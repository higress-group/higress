// ai-anthropic-system-fold
//
// 背景:Claude Code(claude-cli)调 Anthropic /v1/messages 时,会在 messages 数组里注入
//
//	{"role":"system"} 的中途系统消息(mid-conversation system,真 Anthropic 模型支持)。
//	但部分自建后端(如 sglang 的 anthropic_v1_messages)严格校验 messages[].role 只能是
//	user / assistant,遇到 system 直接 400(Input should be 'user' or 'assistant')。
//
// 本插件:对 Anthropic 的 /messages 请求,把 messages 里所有 role=="system" 的消息从数组中移除,
//
//	并把它们的文本【折叠进顶层 system 字段】——这是符合 Anthropic 规范的等价表达,
//	后端不再报 400,且语义比"改成 user"更忠实。
//
// 仅作用于 /messages,明确排除 /chat/completions(OpenAI 端点本就允许 messages 里带 system)。
package main

import (
	"encoding/json"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-anthropic-system-fold",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
	)
}

// 本插件无需配置项,保留空结构以符合框架签名。
type Config struct{}

func parseConfig(_ gjson.Result, _ *Config) error {
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, _ Config) types.Action {
	// 必须在 header 阶段读取并缓存路径判定:body 阶段取 :path 会报 "bad argument"。
	path, _ := proxywasm.GetHttpRequestHeader(":path")
	isMsg := isAnthropicMessages(path)
	ctx.SetContext("isMsg", isMsg)
	if !isMsg {
		ctx.DontReadRequestBody() // 非 /messages 请求不缓冲请求体,零开销放行
		return types.ActionContinue
	}
	// 将改写请求体:关掉重路由、移除 content-length 让 Envoy 按新体重算。
	ctx.DisableReroute()
	_ = proxywasm.RemoveHttpRequestHeader("content-length")
	return types.ActionContinue
}

// 只处理 Anthropic 的 /messages(覆盖 /v1/messages 及误配的 /v1/v1/messages),
// 明确排除 /chat/completions —— OpenAI 端点允许 messages 里带 system,不能动。
func isAnthropicMessages(path string) bool {
	if strings.Contains(path, "/chat/completions") {
		return false
	}
	return strings.Contains(path, "/messages")
}

// 从一条消息的 content 取纯文本:content 可能是字符串,也可能是内容块数组。
func extractText(content gjson.Result) string {
	if content.Type == gjson.String {
		return content.String()
	}
	if content.IsArray() {
		var sb strings.Builder
		for _, blk := range content.Array() {
			if blk.Get("type").String() == "text" {
				sb.WriteString(blk.Get("text").String())
			}
		}
		return sb.String()
	}
	return ""
}

func onHttpRequestBody(ctx wrapper.HttpContext, _ Config, body []byte) types.Action {
	// 路径判定在 header 阶段已算好并缓存(body 阶段取 :path 会失败)。
	if !ctx.GetBoolContext("isMsg", false) {
		return types.ActionContinue
	}

	msgs := gjson.GetBytes(body, "messages")
	if !msgs.IsArray() {
		return types.ActionContinue
	}

	kept := make([]string, 0, len(msgs.Array()))
	sysTexts := make([]string, 0)
	foundSystem := false
	for _, m := range msgs.Array() {
		if m.Get("role").String() == "system" {
			foundSystem = true
			if t := extractText(m.Get("content")); t != "" {
				sysTexts = append(sysTexts, t)
			}
		} else {
			kept = append(kept, m.Raw)
		}
	}
	if !foundSystem {
		return types.ActionContinue // 没有 role:system,放行不动
	}

	var err error
	newBody := body

	// 1) 重建 messages,去掉所有 system 条目。
	newMessages := "[" + strings.Join(kept, ",") + "]"
	if newBody, err = sjson.SetRawBytes(newBody, "messages", []byte(newMessages)); err != nil {
		log.Errorf("[anthropic-system-fold] rebuild messages failed: %v", err)
		return types.ActionContinue
	}

	// 2) 把收集到的 system 文本折叠进顶层 system。
	if len(sysTexts) > 0 {
		folded := strings.Join(sysTexts, "\n\n")
		existing := gjson.GetBytes(newBody, "system")
		if existing.IsArray() {
			// 顶层 system 是内容块数组 → 追加一个 text 块。
			block, _ := json.Marshal(map[string]string{"type": "text", "text": folded})
			if newBody, err = sjson.SetRawBytes(newBody, "system.-1", block); err != nil {
				log.Errorf("[anthropic-system-fold] append system block failed: %v", err)
				return types.ActionContinue
			}
		} else {
			// 顶层 system 是字符串或不存在 → 拼成字符串。
			s := folded
			if existing.Type == gjson.String && existing.String() != "" {
				s = existing.String() + "\n\n" + folded
			}
			if newBody, err = sjson.SetBytes(newBody, "system", s); err != nil {
				log.Errorf("[anthropic-system-fold] set system string failed: %v", err)
				return types.ActionContinue
			}
		}
	}

	if err = proxywasm.ReplaceHttpRequestBody(newBody); err != nil {
		log.Errorf("[anthropic-system-fold] replace body failed: %v", err)
		return types.ActionContinue
	}
	log.Infof("[anthropic-system-fold] folded %d system message(s) into top-level system", len(sysTexts))
	return types.ActionContinue
}
