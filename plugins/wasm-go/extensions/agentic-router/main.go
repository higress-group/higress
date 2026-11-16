// agentic-router —— Higress Wasm 插件（学习型模型档位路由）
//
// 请求体进入时做任务分类 + 四专家（规则/UCB/LinUCB/k-NN，EXP3 元选择器）决策，
// active 模式下改写 OpenAI 兼容 body 的 "model" 字段（配合 ai-proxy 等转发插件）；
// 响应流结束时按 状态/延迟/usage成本 计算质量奖励喂飞轮，路由随使用越来越准。
//
// 飞轮状态经 proxywasm shared data 在同实例多 worker 间共享（last-writer-wins）。
// 算法规范与三语言对拍实现见 https://github.com/lhwwxy/agentic-router-gateway
package main

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	router "github.com/alibaba/higress/plugins/wasm-go/extensions/agentic-router/core"
)

const (
	sharedStateKey = "agentic_router_state"
	ctxDecisionID  = "ar_decision_id"
	ctxStartMs     = "ar_start_ms"
	ctxStatus      = "ar_status"
	ctxUsageCost   = "ar_usage_cost"
	ctxSettled     = "ar_settled"
)

type Config struct {
	Shadow        bool   `json:"shadow"` // true=只记录不改写（默认）
	CrossProvider bool   `json:"crossProvider"`
	// TierModels: fast/mid/strong → 模型名（OpenAI 兼容 body 的 model 值）
	TierModels map[string]string `json:"tierModels"`
	// Prices: tier → {in, out}（元/百万 token），缺省用内置通用档位价
	Prices map[string]struct {
		In  float64 `json:"in"`
		Out float64 `json:"out"`
	} `json:"prices"`
}

var (
	routerInstance *router.Router
	priceIn        = map[string]float64{"fast": 1, "mid": 2, "strong": 3}
	priceOut       = map[string]float64{"fast": 4, "mid": 8, "strong": 16}
)

func init() {
	wrapper.SetCtx("agentic-router",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBodyBy(onHttpStreamingResponseBody),
		wrapper.ProcessStreamDoneBy(onHttpStreamDone),
	)
}

// Wasm 插件经 init 注册 proxy-wasm 回调，不需要传统 main 入口
func main() {}

func parseConfig(json gjson.Result, config *Config, log wrapper.Log) error {
	config.Shadow = true // 默认影子模式：先观察后接管
	if j := json.Get("shadow"); j.Exists() {
		config.Shadow = j.Bool()
	}
	config.CrossProvider = true
	if j := json.Get("crossProvider"); j.Exists() {
		config.CrossProvider = j.Bool()
	}
	if j := json.Get("tierModels"); j.Exists() {
		config.TierModels = map[string]string{}
		for tier, m := range j.Map() {
			config.TierModels[tier] = m.String()
		}
	}
	if j := json.Get("prices"); j.Exists() {
		for tier, p := range j.Map() {
			if config.Prices == nil {
				config.Prices = map[string]struct {
					In  float64 `json:"in"`
					Out float64 `json:"out"`
				}{}
			}
			config.Prices[tier] = struct {
				In  float64 `json:"in"`
				Out float64 `json:"out"`
			}{In: p.Get("in").Float(), Out: p.Get("out").Float()}
		}
	}
	// 构建 modelTiers（统一 provider 命名 default，条目形如 "default/<model>"）
	providers := []router.ProviderEntry{{Provider: "default"}}
	tiers := map[string][]string{}
	for tier, m := range config.TierModels {
		tiers[tier] = []string{"default/" + m}
	}
	state := loadSharedState(log)
	routerInstance = router.NewRouter(state, router.ModelTiers{Providers: providers, Tiers: tiers}, config.CrossProvider, nil)
	if !config.Shadow {
		routerInstance.SetMode("active")
	}
	log.Infof("agentic-router ready: mode=%s tierModels=%v", routerInstance.State.Mode, config.TierModels)
	return nil
}

// loadSharedState 从 Wasm shared data 恢复飞轮（跨 worker 共享，best-effort）
func loadSharedState(log wrapper.Log) *router.State {
	data, _, err := proxywasm.GetSharedData(sharedStateKey)
	if err != nil || len(data) == 0 {
		return nil
	}
	s := &router.State{}
	if err := json.Unmarshal(data, s); err != nil {
		log.Warnf("agentic-router: shared state corrupt, cold start: %v", err)
		return nil
	}
	return s
}

func saveSharedState(log wrapper.Log) {
	if routerInstance == nil {
		return
	}
	data, err := json.Marshal(routerInstance.State)
	if err != nil {
		return
	}
	// 跨 worker 并发写冲突（CAS 失败）时静默放弃：下一次结算会再写
	_ = proxywasm.SetSharedData(sharedStateKey, data, 0)
}

// extractLastUserText 从 OpenAI 兼容请求体取最后一条非空 user 消息文本
func extractLastUserText(body []byte) string {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return ""
	}
	var last string
	for _, m := range messages.Array() {
		if m.Get("role").String() != "user" {
			continue
		}
		c := m.Get("content")
		if c.Type == gjson.String && strings.TrimSpace(c.String()) != "" {
			last = c.String()
		} else if c.IsArray() {
			var parts []string
			for _, p := range c.Array() {
				if p.Get("type").String() == "text" {
					parts = append(parts, p.Get("text").String())
				}
			}
			if joined := strings.Join(parts, "\n"); strings.TrimSpace(joined) != "" {
				last = joined
			}
		}
	}
	return last
}

func onHttpRequestBody(ctx wrapper.HttpContext, config Config, body []byte, log wrapper.Log) types.Action {
	if routerInstance == nil {
		return types.ActionContinue
	}
	text := extractLastUserText(body)
	if text == "" {
		return types.ActionContinue
	}
	d := routerInstance.Decide(text, "default", nil)
	ctx.SetContext(ctxDecisionID, d.ID)
	ctx.SetContext(ctxStartMs, time.Now().UnixMilli())

	if routerInstance.State.Mode == "active" && d.Model != "" {
		if newBody, err := sjson.SetBytes(body, "model", d.Model); err == nil {
			if err := proxywasm.ReplaceHttpRequestBody(newBody); err != nil {
				log.Errorf("agentic-router: replace body failed: %v", err)
			} else {
				_ = proxywasm.AddHttpRequestHeader("x-agentic-router-tier", d.Used)
				_ = proxywasm.AddHttpRequestHeader("x-agentic-router-expert", d.Chosen)
			}
		}
	}
	log.Debugf("agentic-router: decide id=%d type=%s complexity=%v tier=%s model=%s expert=%s reason=%s",
		d.ID, d.Type, d.Complexity, d.Tier, d.Model, d.Chosen, d.Reason)
	return types.ActionContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config Config, log wrapper.Log) types.Action {
	headers, err := proxywasm.GetHttpResponseHeaders()
	if err != nil {
		return types.ActionContinue
	}
	for _, h := range headers {
		if h[0] == ":status" {
			ctx.SetContext(ctxStatus, atoiSafe(h[1]))
			break
		}
	}
	return types.ActionContinue
}

// onHttpStreamingResponseBody 逐分片透传（不整包缓冲，SSE 首字延迟不受影响），
// 顺带扫描分片中的 usage 字段累计成本。非流式响应也走这里（单分片）。
func onHttpStreamingResponseBody(ctx wrapper.HttpContext, config Config, chunk []byte, isLastChunk bool, log wrapper.Log) []byte {
	if routerInstance == nil {
		return chunk
	}
	if cost := extractUsageCost(chunk, config); cost > 0 {
		prev, _ := ctx.GetContext(ctxUsageCost).(float64)
		ctx.SetContext(ctxUsageCost, prev+cost)
	}
	return chunk
}

// onHttpStreamDone 流结束（流式/非流式都会触发）：结算奖励
func onHttpStreamDone(ctx wrapper.HttpContext, config Config, log wrapper.Log) {
	if routerInstance == nil {
		return
	}
	if v := ctx.GetContext(ctxSettled); v != nil {
		return
	}
	id, _ := ctx.GetContext(ctxDecisionID).(int)
	if id == 0 {
		return
	}
	startMs, _ := ctx.GetContext(ctxStartMs).(int64)
	if startMs == 0 {
		startMs = time.Now().UnixMilli()
	}
	status, _ := ctx.GetContext(ctxStatus).(int)
	cost, _ := ctx.GetContext(ctxUsageCost).(float64)

	res := routerInstance.Settle(id, router.Outcome{
		Success:  status >= 200 && status < 400,
		DtMs:     time.Now().UnixMilli() - startMs,
		CostYuan: cost,
	})
	if res != nil {
		ctx.SetContext(ctxSettled, true)
		log.Debugf("agentic-router: settle id=%d reward=%v tier=%s dt=%dms cost=%v",
			id, res.Reward, res.Tier, time.Now().UnixMilli()-startMs, cost)
		saveSharedState(log)
	}
}

// extractUsageCost 解析分片中的 usage（OpenAI 兼容）：
// 直接 JSON（非流式/最后一个 data 行）或 SSE "data: {...}" 行
func extractUsageCost(chunk []byte, config Config) float64 {
	usage := findUsageJSON(chunk)
	if !usage.Exists() {
		return 0
	}
	tier := "mid"
	if routerInstance != nil {
		tier = router.TierOf(gjson.GetBytes(chunk, "model").String())
	}
	pin, pout := priceIn[tier], priceOut[tier]
	if p, ok := config.Prices[tier]; ok {
		pin, pout = p.In, p.Out
	}
	prompt := usage.Get("prompt_tokens").Float() + usage.Get("prompt_tokens_details.cached_tokens").Float()
	completion := usage.Get("completion_tokens").Float() + usage.Get("completion_tokens_details.reasoning_tokens").Float()
	return (pin*prompt + pout*completion) / 1e6
}

func findUsageJSON(chunk []byte) gjson.Result {
	if u := gjson.GetBytes(chunk, "usage"); u.Exists() {
		return u
	}
	// SSE 分片：逐行找 "data: " 载荷里的 usage
	for _, line := range strings.Split(string(chunk), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		if u := gjson.Get(payload, "usage"); u.Exists() {
			return u
		}
	}
	return gjson.Result{}
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}
