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

	// DefaultRedisKeyPrefix 默认 Redis key 前缀
	DefaultRedisKeyPrefix = "chat_quota"

	// QuotaKeyFormat Redis key 格式：{redis_key_prefix}:<targetName>
	// 使用 {} 包裹前缀作为 hash tag，确保 group/consumer quota key 在 Redis Cluster 中落在同一 slot
	// 例：prefix="chat_quota" targetName="team-a" → "{chat_quota}:team-a"
	QuotaKeyFormat = "{%s}:%s"

	// RequestPhaseQuotaReadScript 阶段 1 严格模式读
	// **仅 group != "" 时调用**group == "" 走 plugin 端 Get，与老 ai-quota 一致。
	// KEYS[1]=group_quota_key      KEYS[2]=consumer_quota_key
	// ARGV=（无；脚本不读任何 ARGV）
	// 返回: {group_remaining, consumer_remaining}
	RequestPhaseQuotaReadScript = `
local ng = tonumber(redis.call("GET", KEYS[1]) or "0")
local nc = tonumber(redis.call("GET", KEYS[2]) or "0")
return {ng, nc}
`

	// ResponsePhaseQuotaDecrbyScript 阶段 2 原子 DECRBY
	// **仅 group != "" 时调用**group == "" 走 plugin 端 DecrBy。
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
	RedisKeyPrefix     string            `yaml:"redis_key_prefix"`
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
	config.RedisKeyPrefix = json.Get("redis_key_prefix").String()
	if config.RedisKeyPrefix == "" {
		config.RedisKeyPrefix = DefaultRedisKeyPrefix
	}
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

	consumerKey := fmt.Sprintf(QuotaKeyFormat, config.RedisKeyPrefix, consumer)
	// 读取 group header。
	group, err := proxywasm.GetHttpRequestHeader("x-mse-consumer-group")

	// 新路径——Lua Eval，原子读 group + consumer 两池
	if err == nil && group != "" {
		context.SetContext("group", group)
		groupKey := fmt.Sprintf(QuotaKeyFormat, config.RedisKeyPrefix, group)
		keys := []interface{}{groupKey, consumerKey}
		_ = config.redisClient.Eval(RequestPhaseQuotaReadScript, 2, keys, nil, func(response resp.Value) {
			isDenied := false
			if err := response.Error(); err != nil {
				isDenied = true
			}
			arr := response.Array()
			if len(arr) != 2 {
				isDenied = true
			}
			gRem := arr[0].Integer()
			cRem := arr[1].Integer()
			if gRem <= 0 || cRem <= 0 {
				isDenied = true
			}
			if isDenied {
				util.SendResponse(http.StatusForbidden, "ai-quota.noquota", "text/plain", "Request denied by ai quota check, ai-quota.noquota: consumer quota exhausted")
				return
			}
			proxywasm.ResumeHttpRequest()
		})
		return types.HeaderStopAllIterationAndWatermark
	}

	// 老路径——单池 GET，行为与 ai-quota 升级前完全一致
	_ = config.redisClient.Get(consumerKey, func(response resp.Value) {
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
			util.SendResponse(http.StatusForbidden, "ai-quota.noquota", "text/plain", "Request denied by ai quota check, ai-quota.noquota: consumer quota exhausted")
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
	consumerKey := fmt.Sprintf(QuotaKeyFormat, config.RedisKeyPrefix, consumer)

	// - group == ""：走老 ai-quota 路径（单 DECRBY）
	if group == "" {
		log.Debugf("update consumer:%s, totalToken:%d", consumer, totalToken)
		config.redisClient.DecrBy(consumerKey, totalToken, nil)
		return data
	}

	// - group != ""：走 Lua Eval，原子双 key DECRBY
	groupKey := fmt.Sprintf(QuotaKeyFormat, config.RedisKeyPrefix, group)
	keys := []interface{}{groupKey, consumerKey}
	args := []interface{}{totalToken}
	log.Debugf("update consumer:%s and group:%s, totalToken:%d", consumer, group, totalToken)
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
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check, ai-quota.unauthorized: consumer or group must be set (exactly one).")
		return types.ActionContinue
	}
	if err != nil {
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check, ai-quota.unauthorized: quota must be an integer.")
		return types.ActionContinue
	}

	targetName := queryConsumer
	if queryGroup != "" {
		targetName = queryGroup
	}

	quotaKey := fmt.Sprintf(QuotaKeyFormat, config.RedisKeyPrefix, targetName)

	err2 := config.redisClient.Set(quotaKey, quota, func(response resp.Value) {
		log.Debugf("Redis set key = %s quota = %d", quotaKey, quota)
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
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check, ai-quota.unauthorized: consumer or group must be set (exactly one).")
		return types.ActionContinue
	}
	targetName := queryConsumer
	if queryGroup != "" {
		targetName = queryGroup
	}
	err := config.redisClient.Get(fmt.Sprintf(QuotaKeyFormat, config.RedisKeyPrefix, targetName), func(response resp.Value) {
		quota := 0
		if err := response.Error(); err != nil {
			util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis error:%v", err))
			return
		} else if response.IsNull() {
			quota = 0
		} else {
			quota = response.Integer()
		}
		result := make(map[string]any)
		if queryGroup != "" {
			result["group"] = targetName
		} else {
			result["consumer"] = targetName
		}
		result["quota"] = quota
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
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check, ai-quota.unauthorized: consumer or group must be set (exactly one).")
		return types.ActionContinue
	}
	if err != nil {
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check, ai-quota.unauthorized: value must be an integer.")
		return types.ActionContinue
	}

	targetName := queryConsumer
	if queryGroup != "" {
		targetName = queryGroup
	}

	quotaKey := fmt.Sprintf(QuotaKeyFormat, config.RedisKeyPrefix, targetName)

	if value >= 0 {
		err := config.redisClient.IncrBy(quotaKey, value, func(response resp.Value) {
			log.Debugf("Redis Incr key = %s value = %d", quotaKey, value)
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
		err := config.redisClient.DecrBy(quotaKey, 0-value, func(response resp.Value) {
			log.Debugf("Redis Incr key = %s value = %d", quotaKey, value)
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
