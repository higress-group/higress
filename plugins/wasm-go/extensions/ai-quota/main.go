package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/tokenusage"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/resp"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-quota/util"
)

const (
	pluginName = "ai-quota"

	// RedisKeyPrefix 集群限流插件在 Redis 中 key 的统一前缀
	// 使用 {} 包裹作为 hash tag，确保 group/consumer quota key 在 Redis Cluster 中落在同一 slot
	RedisKeyPrefix = "{chat_quota}"

	// QuotaKeyFormat Redis key 格式：{chat_quota}:<subject>
	// subject 可为 consumer name 或 group name
	// 例：{chat_quota}:alice, {chat_quota}:team-a
	QuotaKeyFormat = RedisKeyPrefix + ":%s"

	// RequestPhaseQuotaReadScript 阶段 1 严格模式读
	// **仅 group != "" 时调用**——group == "" 走 plugin 端 Get，与老 ai-quota 字节级一致。
	// plugin Go 端用 `if group != ""` 决定走 Lua 还是非 Lua 路径，不引入显式 hasGroup 变量。
	// 用 2 次 GET 而非 1 次 MGET：Lua 脚本在 Redis 单线程上原子执行，
	// 脚本内两次 GET 与 MGET 等价但更可读。
	// KEYS[1]=group_quota_key      KEYS[2]=consumer_quota_key
	// ARGV=（无；脚本不读任何 ARGV）
	// 返回: {group_remaining, consumer_remaining}
	RequestPhaseQuotaReadScript = `
local ng = tonumber(redis.call("GET", KEYS[1]) or "0")
local nc = tonumber(redis.call("GET", KEYS[2]) or "0")
return {ng, nc}
`

	// ResponsePhaseQuotaDecrbyScript 阶段 2 原子 DECRBY
	// **仅 group != "" 时调用**——group == "" 走 plugin 端 DecrBy。
	// 两把 key 都 DECRBY，原子执行避免双池扣减顺序竞争。
	// KEYS[1]=group_quota_key      KEYS[2]=consumer_quota_key
	// ARGV[1]=cost
	// 返回: {group_remaining, consumer_remaining}（扣减后，可为负）
	ResponsePhaseQuotaDecrbyScript = `
local cost = tonumber(ARGV[1])
local ng = redis.call("DECRBY", KEYS[1], cost)
local nc = redis.call("DECRBY", KEYS[2], cost)
return {ng, nc}
`
)

type ChatMode string

const (
	ChatModeCompletion ChatMode = "completion"
	ChatModeAdmin      ChatMode = "admin"
	ChatModeNone       ChatMode = "none"
)

type AdminMode string

const (
	AdminModeRefresh AdminMode = "refresh"
	AdminModeQuery   AdminMode = "query"
	AdminModeDelta   AdminMode = "delta"
	AdminModeNone    AdminMode = "none"
)

func main() {}

func init() {
	wrapper.SetCtx(
		pluginName,
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessStreamingResponseBody(onHttpStreamingResponseBody),
	)
}

type QuotaConfig struct {
	redisInfo          RedisInfo         `yaml:"redis"`
	AdminConsumer      string            `yaml:"admin_consumer"`
	AdminPath          string            `yaml:"admin_path"`
	EnablePathSuffixes []string          `yaml:"enable_path_suffixes"`
	credential2Name    map[string]string `yaml:"-"`
	redisClient        wrapper.RedisClient
}

type Consumer struct {
	Name       string `yaml:"name"`
	Credential string `yaml:"credential"`
}

type RedisInfo struct {
	ServiceName string `required:"true" yaml:"service_name" json:"service_name"`
	ServicePort int    `required:"false" yaml:"service_port" json:"service_port"`
	Username    string `required:"false" yaml:"username" json:"username"`
	Password    string `required:"false" yaml:"password" json:"password"`
	Timeout     int    `required:"false" yaml:"timeout" json:"timeout"`
	Database    int    `required:"false" yaml:"database" json:"database"`
}

func parseConfig(json gjson.Result, config *QuotaConfig) error {
	log.Debugf("parse config()")
	// admin
	config.AdminPath = json.Get("admin_path").String()
	config.AdminConsumer = json.Get("admin_consumer").String()
	if config.AdminPath == "" {
		config.AdminPath = "/quota"
	}
	suffixResult := json.Get("enable_path_suffixes")
	if !suffixResult.Exists() {
		config.EnablePathSuffixes = []string{"/v1/chat/completions", "/v1/messages"}
	} else if !suffixResult.IsArray() {
		return errors.New("enable_path_suffixes must be an array")
	} else {
		pathSuffixes := suffixResult.Array()
		config.EnablePathSuffixes = make([]string, 0, len(pathSuffixes))
		for _, suffix := range pathSuffixes {
			suffixStr := strings.TrimSpace(suffix.String())
			if suffixStr == "" {
				continue
			}
			config.EnablePathSuffixes = append(config.EnablePathSuffixes, suffixStr)
		}
	}
	if len(config.EnablePathSuffixes) == 0 {
		return errors.New("enable_path_suffixes must not be empty")
	}
	if config.AdminConsumer == "" {
		return errors.New("missing admin_consumer in config")
	}
	// Redis
	redisConfig := json.Get("redis")
	if !redisConfig.Exists() {
		return errors.New("missing redis in config")
	}
	serviceName := redisConfig.Get("service_name").String()
	if serviceName == "" {
		return errors.New("redis service name must not be empty")
	}
	servicePort := int(redisConfig.Get("service_port").Int())
	if servicePort == 0 {
		if strings.HasSuffix(serviceName, ".static") {
			// use default logic port which is 80 for static service
			servicePort = 80
		} else {
			servicePort = 6379
		}
	}
	username := redisConfig.Get("username").String()
	password := redisConfig.Get("password").String()
	timeout := int(redisConfig.Get("timeout").Int())
	if timeout == 0 {
		timeout = 1000
	}
	database := int(redisConfig.Get("database").Int())
	config.redisInfo.ServiceName = serviceName
	config.redisInfo.ServicePort = servicePort
	config.redisInfo.Username = username
	config.redisInfo.Password = password
	config.redisInfo.Timeout = timeout
	config.redisInfo.Database = database
	config.redisClient = wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
		FQDN: serviceName,
		Port: int64(servicePort),
	})

	return config.redisClient.Init(username, password, int64(timeout), wrapper.WithDataBase(database))
}

func onHttpRequestHeaders(context wrapper.HttpContext, config QuotaConfig) types.Action {
	context.DisableReroute()
	log.Debugf("onHttpRequestHeaders()")
	// get tokens
	consumer, err := proxywasm.GetHttpRequestHeader("x-mse-consumer")
	if err != nil {
		return deniedNoKeyAuthData()
	}
	if consumer == "" {
		return deniedUnauthorizedConsumer()
	}

	rawPath := context.Path()
	path, _ := url.Parse(rawPath)
	chatMode, adminMode := getOperationMode(path.Path, config.AdminPath, config.EnablePathSuffixes)
	context.SetContext("chatMode", chatMode)
	context.SetContext("adminMode", adminMode)
	context.SetContext("consumer", consumer)
	log.Debugf("chatMode:%s, adminMode:%s, consumer:%s", chatMode, adminMode, consumer)
	if chatMode == ChatModeNone {
		return types.ActionContinue
	}
	if chatMode == ChatModeAdmin {
		// query quota
		if adminMode == AdminModeQuery {
			return queryQuota(context, config, consumer, path)
		}
		if adminMode == AdminModeRefresh || adminMode == AdminModeDelta {
			context.BufferRequestBody()
			return types.HeaderStopIteration
		}
		return types.ActionContinue
	}

	// there is no need to read request body when it is on chat completion mode
	context.DontReadRequestBody()

	// 读取 group header（不校验格式——与 name 一致）。
	group := ""
	if rawGroup, gErr := proxywasm.GetHttpRequestHeader("x-mse-consumer-group"); gErr == nil && rawGroup != "" {
		group = rawGroup
	}
	context.SetContext("group", group)

	// 按 group 是否非空走两条路径：
	// - group == ""：走老 ai-quota 路径（Get + DecrBy），字节级一致
	// - group != ""：走 Lua Eval，原子双 key 操作
	consumerKey := fmt.Sprintf(QuotaKeyFormat, consumer)
	if group == "" {
		// 老路径——单池 GET，行为与 ai-quota 升级前完全一致
		config.redisClient.Get(consumerKey, func(response resp.Value) {
			isDenied := false
			if err := response.Error(); err != nil {
				isDenied = true
			}
			if response.IsNull() {
				isDenied = true
			}
			if response.Integer() <= 0 {
				isDenied = true
			}
			if isDenied {
				util.SendResponse(http.StatusTooManyRequests, "ai-quota.consumer_exhausted", "text/plain", "Request denied by ai quota check, ai-quota.consumer_exhausted: consumer quota exhausted")
				return
			}
			proxywasm.ResumeHttpRequest()
		})
		return types.HeaderStopAllIterationAndWatermark
	}

	// 新路径——Lua Eval，原子读 group + consumer 两池
	groupKey := fmt.Sprintf(QuotaKeyFormat, group)
	keys := []interface{}{groupKey, consumerKey}
	_ = config.redisClient.Eval(RequestPhaseQuotaReadScript, 2, keys, nil, func(response resp.Value) {
		if err := response.Error(); err != nil {
			util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis error:%v", err))
			return
		}
		arr := response.Array()
		if len(arr) != 2 {
			log.Errorf("phase1 unexpected response: %v", response)
			util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", "quota check response parse error")
			return
		}
		gRem := arr[0].Integer()
		cRem := arr[1].Integer()

		// 严格模式：任一池 ≤ 0 即拒
		var code, detail string
		switch {
		case gRem <= 0 && cRem <= 0:
			code, detail = "ai-quota.both_exhausted", "group and consumer quota exhausted"
		case gRem <= 0:
			code, detail = "ai-quota.group_exhausted", "group quota exhausted"
		case cRem <= 0:
			code, detail = "ai-quota.consumer_exhausted", "consumer quota exhausted"
		}
		if code != "" {
			util.SendResponse(http.StatusTooManyRequests, code, "text/plain", "Request denied by ai quota check, "+code+": "+detail)
			return
		}
		proxywasm.ResumeHttpRequest()
	})
	return types.HeaderStopAllIterationAndWatermark
}

func onHttpRequestBody(ctx wrapper.HttpContext, config QuotaConfig, body []byte) types.Action {
	log.Debugf("onHttpRequestBody()")
	chatMode, ok := ctx.GetContext("chatMode").(ChatMode)
	if !ok {
		return types.ActionContinue
	}
	if chatMode == ChatModeNone || chatMode == ChatModeCompletion {
		return types.ActionContinue
	}
	adminMode, ok := ctx.GetContext("adminMode").(AdminMode)
	if !ok {
		return types.ActionContinue
	}
	adminConsumer, ok := ctx.GetContext("consumer").(string)
	if !ok {
		return types.ActionContinue
	}

	if adminMode == AdminModeRefresh {
		return refreshQuota(ctx, config, adminConsumer, string(body))
	}
	if adminMode == AdminModeDelta {
		return deltaQuota(ctx, config, adminConsumer, string(body))
	}

	return types.ActionContinue
}

func onHttpStreamingResponseBody(ctx wrapper.HttpContext, config QuotaConfig, data []byte, endOfStream bool) []byte {
	chatMode, ok := ctx.GetContext("chatMode").(ChatMode)
	if !ok {
		return data
	}
	if chatMode == ChatModeNone || chatMode == ChatModeAdmin {
		return data
	}
	if usage := tokenusage.GetTokenUsage(ctx, data); usage.TotalToken > 0 {
		ctx.SetContext(tokenusage.CtxKeyInputToken, usage.InputToken)
		ctx.SetContext(tokenusage.CtxKeyOutputToken, usage.OutputToken)
	}

	// chat completion mode
	if !endOfStream {
		return data
	}

	if ctx.GetContext(tokenusage.CtxKeyInputToken) == nil || ctx.GetContext(tokenusage.CtxKeyOutputToken) == nil || ctx.GetContext("consumer") == nil {
		return data
	}

	inputToken := ctx.GetContext(tokenusage.CtxKeyInputToken).(int64)
	outputToken := ctx.GetContext(tokenusage.CtxKeyOutputToken).(int64)
	consumer := ctx.GetContext("consumer").(string)
	group, _ := ctx.GetContext("group").(string)
	totalToken := int(inputToken + outputToken)

	// 按 group 是否非空走两条路径：
	// - group == ""：走老 ai-quota 路径（单 DECRBY），字节级一致
	// - group != ""：走 Lua Eval，原子双 key DECRBY
	consumerKey := fmt.Sprintf(QuotaKeyFormat, consumer)
	if group == "" {
		log.Debugf("update consumer:%s, totalToken:%d", consumer, totalToken)
		config.redisClient.DecrBy(consumerKey, totalToken, nil)
		return data
	}

	// 新路径——Lua Eval，原子双池 DECRBY
	groupKey := fmt.Sprintf(QuotaKeyFormat, group)
	keys := []interface{}{groupKey, consumerKey}
	args := []interface{}{totalToken}
	log.Debugf("phase2 decrby consumer:%s group:%s cost:%d", consumer, group, totalToken)
	_ = config.redisClient.Eval(ResponsePhaseQuotaDecrbyScript, 2, keys, args, nil)
	return data
}

func deniedNoKeyAuthData() types.Action {
	util.SendResponse(http.StatusUnauthorized, "ai-quota.no_key", "text/plain", "Request denied by ai quota check. No Key Authentication information found.")
	return types.ActionContinue
}

func deniedUnauthorizedConsumer() types.Action {
	util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. Unauthorized consumer.")
	return types.ActionContinue
}

func getOperationMode(path string, adminPath string, pathSuffixes []string) (ChatMode, AdminMode) {
	fullAdminPath := "/v1/chat/completions" + adminPath
	if strings.HasSuffix(path, fullAdminPath+"/refresh") {
		return ChatModeAdmin, AdminModeRefresh
	}
	if strings.HasSuffix(path, fullAdminPath+"/delta") {
		return ChatModeAdmin, AdminModeDelta
	}
	if strings.HasSuffix(path, fullAdminPath) {
		return ChatModeAdmin, AdminModeQuery
	}
	for _, suffix := range pathSuffixes {
		if strings.HasSuffix(path, suffix) {
			return ChatModeCompletion, AdminModeNone
		}
	}
	return ChatModeNone, AdminModeNone
}

func refreshQuota(ctx wrapper.HttpContext, config QuotaConfig, adminConsumer string, body string) types.Action {
	// check consumer
	if adminConsumer != config.AdminConsumer {
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. Unauthorized admin consumer.")
		return types.ActionContinue
	}

	queryValues, _ := url.ParseQuery(body)
	values := make(map[string]string, len(queryValues))
	for k, v := range queryValues {
		values[k] = v[0]
	}
	queryConsumer := values["consumer"]
	queryGroup := values["group"]
	quota, err := strconv.Atoi(values["quota"])

	// 互斥校验：consumer 与 group 必须恰好设置一个
	if (queryConsumer == "" && queryGroup == "") || (queryConsumer != "" && queryGroup != "") {
		util.SendResponse(http.StatusBadRequest, "ai-quota.invalid_param", "text/plain", "ai-quota.invalid_param: exactly one of 'consumer' or 'group' must be set")
		return types.ActionContinue
	}
	if err != nil {
		util.SendResponse(http.StatusBadRequest, "ai-quota.invalid_param", "text/plain", "ai-quota.invalid_param: quota must be an integer")
		return types.ActionContinue
	}

	subject := queryConsumer
	if queryGroup != "" {
		subject = queryGroup
	}

	sourceAddr := string(getSourceAddress())
	subjectKey := fmt.Sprintf(QuotaKeyFormat, subject)

	err2 := config.redisClient.Set(subjectKey, quota, func(response resp.Value) {
		log.Infof("admin refresh quota: admin=%s subject=%s key=%s quota=%d from=%s", adminConsumer, subject, subjectKey, quota, sourceAddr)
		if err := response.Error(); err != nil {
			util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis error:%v", err))
			return
		}
		util.SendResponse(http.StatusOK, "ai-quota.refreshquota", "text/plain", "refresh quota successful")
	})

	if err2 != nil {
		util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis error:%v", err2))
		return types.ActionContinue
	}

	return types.ActionPause
}

func queryQuota(ctx wrapper.HttpContext, config QuotaConfig, adminConsumer string, url *url.URL) types.Action {
	// check consumer
	if adminConsumer != config.AdminConsumer {
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. Unauthorized admin consumer.")
		return types.ActionContinue
	}
	// check url
	queryValues := url.Query()
	values := make(map[string]string, len(queryValues))
	for k, v := range queryValues {
		values[k] = v[0]
	}
	queryConsumer := values["consumer"]
	queryGroup := values["group"]
	if (queryConsumer == "" && queryGroup == "") || (queryConsumer != "" && queryGroup != "") {
		util.SendResponse(http.StatusBadRequest, "ai-quota.invalid_param", "text/plain", "ai-quota.invalid_param: exactly one of 'consumer' or 'group' must be set")
		return types.ActionContinue
	}
	subject := queryConsumer
	if queryGroup != "" {
		subject = queryGroup
	}
	err := config.redisClient.Get(fmt.Sprintf(QuotaKeyFormat, subject), func(response resp.Value) {
		quota := 0
		if err := response.Error(); err != nil {
			util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis error:%v", err))
			return
		} else if response.IsNull() {
			quota = 0
		} else {
			quota = response.Integer()
		}
		result := struct {
			Name  string `json:"name"`
			Quota int    `json:"quota"`
		}{
			Name:  subject,
			Quota: quota,
		}
		body, _ := json.Marshal(result)
		util.SendResponse(http.StatusOK, "ai-quota.queryquota", "application/json", string(body))
	})
	if err != nil {
		util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis error:%v", err))
		return types.ActionContinue
	}
	return types.ActionPause
}

func deltaQuota(ctx wrapper.HttpContext, config QuotaConfig, adminConsumer string, body string) types.Action {
	// check consumer
	if adminConsumer != config.AdminConsumer {
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. Unauthorized admin consumer.")
		return types.ActionContinue
	}

	queryValues, _ := url.ParseQuery(body)
	values := make(map[string]string, len(queryValues))
	for k, v := range queryValues {
		values[k] = v[0]
	}
	queryConsumer := values["consumer"]
	queryGroup := values["group"]
	value, err := strconv.Atoi(values["value"])
	if (queryConsumer == "" && queryGroup == "") || (queryConsumer != "" && queryGroup != "") {
		util.SendResponse(http.StatusBadRequest, "ai-quota.invalid_param", "text/plain", "ai-quota.invalid_param: exactly one of 'consumer' or 'group' must be set")
		return types.ActionContinue
	}
	if err != nil {
		util.SendResponse(http.StatusBadRequest, "ai-quota.invalid_param", "text/plain", "ai-quota.invalid_param: value must be an integer")
		return types.ActionContinue
	}

	subject := queryConsumer
	if queryGroup != "" {
		subject = queryGroup
	}

	sourceAddr := string(getSourceAddress())
	subjectKey := fmt.Sprintf(QuotaKeyFormat, subject)

	if value >= 0 {
		err := config.redisClient.IncrBy(subjectKey, value, func(response resp.Value) {
			log.Infof("admin delta quota (incr): admin=%s subject=%s key=%s value=%d from=%s", adminConsumer, subject, subjectKey, value, sourceAddr)
			if err := response.Error(); err != nil {
				util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis error:%v", err))
				return
			}
			util.SendResponse(http.StatusOK, "ai-quota.deltaquota", "text/plain", "delta quota successful")
		})
		if err != nil {
			util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis error:%v", err))
			return types.ActionContinue
		}
	} else {
		err := config.redisClient.DecrBy(subjectKey, 0-value, func(response resp.Value) {
			log.Infof("admin delta quota (decr): admin=%s subject=%s key=%s value=%d from=%s", adminConsumer, subject, subjectKey, value, sourceAddr)
			if err := response.Error(); err != nil {
				util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis error:%v", err))
				return
			}
			util.SendResponse(http.StatusOK, "ai-quota.deltaquota", "text/plain", "delta quota successful")
		})
		if err != nil {
			util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis error:%v", err))
			return types.ActionContinue
		}
	}

	return types.ActionPause
}

func getSourceAddress() []byte {
	bs, err := proxywasm.GetProperty([]string{"source", "address"})
	if err != nil {
		return nil
	}
	return bs
}
