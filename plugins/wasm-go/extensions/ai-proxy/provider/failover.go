package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/google/uuid"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

type failover struct {
	// @Title zh-CN 是否启用 apiToken 的 failover 机制
	enabled bool `required:"false" yaml:"enabled" json:"enabled"`
	// @Title zh-CN 触发 failover 连续请求失败的阈值
	failureThreshold int64 `required:"false" yaml:"failureThreshold" json:"failureThreshold"`
	// @Title zh-CN 健康检测的成功阈值
	successThreshold int64 `required:"false" yaml:"successThreshold" json:"successThreshold"`
	// @Title zh-CN 健康检测的间隔时间，单位毫秒
	healthCheckInterval int64 `required:"false" yaml:"healthCheckInterval" json:"healthCheckInterval"`
	// @Title zh-CN 健康检测的超时时间，单位毫秒
	healthCheckTimeout int64 `required:"false" yaml:"healthCheckTimeout" json:"healthCheckTimeout"`
	// @Title zh-CN 健康检测使用的模型
	healthCheckModel string `required:"false" yaml:"healthCheckModel" json:"healthCheckModel"`
	// @Title zh-CN 需要进行 failover 的原始请求的状态码，支持正则表达式匹配
	failoverOnStatus []string `required:"false" yaml:"failoverOnStatus" json:"failoverOnStatus"`
	// @Title zh-CN 本次请求使用的 apiToken
	ctxApiTokenInUse string
	// @Title zh-CN 记录本次请求时所有可用的 apiToken
	ctxAvailableApiTokensInRequest string
	// @Title zh-CN 记录 apiToken 请求失败的次数，key 为 apiToken，value 为失败次数
	ctxApiTokenRequestFailureCount string
	// @Title zh-CN 记录 apiToken 健康检测成功的次数，key 为 apiToken，value 为成功次数
	ctxApiTokenRequestSuccessCount string
	// @Title zh-CN 记录所有可用的 apiToken 列表
	ctxApiTokens string
	// @Title zh-CN 记录所有不可用的 apiToken 列表
	ctxUnavailableApiTokens string
	// @Title zh-CN 记录请求的 cluster, host 和 path，用于在健康检测时构建请求
	ctxHealthCheckEndpoint string
	// @Title zh-CN 健康检测选主，只有选到主的 Wasm VM 才执行健康检测
	ctxVmLease string
}

type Lease struct {
	VMID      string `json:"vmID"`
	Timestamp int64  `json:"timestamp"`
}

type HealthCheckEndpoint struct {
	Host    string `json:"host"`
	Path    string `json:"path"`
	Cluster string `json:"cluster"`
}

const (
	casMaxRetries                      = 10
	addApiTokenOperation               = "addApiToken"
	removeApiTokenOperation            = "removeApiToken"
	addApiTokenRequestCountOperation   = "addApiTokenRequestCount"
	resetApiTokenRequestCountOperation = "resetApiTokenRequestCount"
	CtxRequestHost                     = "requestHost"
	CtxRequestPath                     = "requestPath"
	CtxRequestBody                     = "requestBody"
)

var (
	healthCheckClient wrapper.HttpClient
)

func (f *failover) FromJson(json gjson.Result) {
	f.enabled = json.Get("enabled").Bool()
	f.failureThreshold = json.Get("failureThreshold").Int()
	if f.failureThreshold == 0 {
		f.failureThreshold = 3
	}
	f.successThreshold = json.Get("successThreshold").Int()
	if f.successThreshold == 0 {
		f.successThreshold = 1
	}
	f.healthCheckInterval = json.Get("healthCheckInterval").Int()
	if f.healthCheckInterval == 0 {
		f.healthCheckInterval = 5000
	}
	f.healthCheckTimeout = json.Get("healthCheckTimeout").Int()
	if f.healthCheckTimeout == 0 {
		f.healthCheckTimeout = 5000
	}
	f.healthCheckModel = json.Get("healthCheckModel").String()

	for _, status := range json.Get("failoverOnStatus").Array() {
		f.failoverOnStatus = append(f.failoverOnStatus, status.String())
	}
	// If failoverOnStatus is empty, default to retry on 4xx and 5xx
	if len(f.failoverOnStatus) == 0 {
		f.failoverOnStatus = []string{"4.*", "5.*"}
	}
}

func (f *failover) Validate() error {
	if f.healthCheckModel == "" {
		return errors.New("missing healthCheckModel in failover config")
	}
	return nil
}

func (c *ProviderConfig) initVariable() {
	// Set provider name as prefix to differentiate shared data
	provider := c.GetType()
	id := c.GetId()
	c.failover.ctxApiTokenInUse = provider + "-" + id + "-apiTokenInUse"
	c.failover.ctxApiTokenRequestFailureCount = provider + "-" + id + "-apiTokenRequestFailureCount"
	c.failover.ctxApiTokenRequestSuccessCount = provider + "-" + id + "-apiTokenRequestSuccessCount"
	c.failover.ctxApiTokens = provider + "-" + id + "-apiTokens"
	c.failover.ctxUnavailableApiTokens = provider + "-" + id + "-unavailableApiTokens"
	c.failover.ctxHealthCheckEndpoint = provider + "-" + id + "-requestHostAndPath"
	c.failover.ctxVmLease = provider + "-" + id + "-vmLease"
}

func parseConfig(json gjson.Result, config *any) error {
	return nil
}

func (c *ProviderConfig) SetApiTokensFailover(activeProvider Provider) error {
	c.initVariable()
	// Reset shared data in case plugin configuration is updated
	log.Debugf("ai-proxy plugin configuration is updated, reset shared data")
	c.resetSharedData()

	if c.isFailoverEnabled() {
		log.Debugf("ai-proxy plugin failover is enabled")

		vmID := generateVMID()
		err := c.initApiTokens()

		if err != nil {
			return fmt.Errorf("failed to init apiTokens: %v", err)
		}

		wrapper.RegisterTickFunc(c.failover.healthCheckInterval, func() {
			// Only the Wasm VM that successfully acquires the lease will perform health check
			if c.isFailoverEnabled() && c.tryAcquireOrRenewLease(vmID) {
				log.Debugf("Successfully acquired or renewed lease for %v: %v", vmID, c.GetType())
				unavailableTokens, _, err := getApiTokens(c.failover.ctxUnavailableApiTokens)
				if err != nil {
					log.Errorf("Failed to get unavailable tokens: %v", err)
					return
				}
				if len(unavailableTokens) > 0 {
					for _, apiToken := range unavailableTokens {
						log.Debugf("Perform health check for unavailable apiTokens: %s", strings.Join(unavailableTokens, ", "))
						healthCheckEndpoint, headers, body := c.generateRequestHeadersAndBody()
						healthCheckClient = wrapper.NewClusterClient(wrapper.TargetCluster{
							Cluster: healthCheckEndpoint.Cluster,
						})

						ctx := createHttpContext()
						ctx.SetContext(c.failover.ctxApiTokenInUse, apiToken)

						modifiedHeaders, modifiedBody, err := c.transformRequestHeadersAndBody(ctx, activeProvider, headers, body)
						if err != nil {
							log.Errorf("Failed to transform request headers and body: %v", err)
						}

						// The apiToken for ChatCompletion and Embeddings can be the same, so we only need to health check ChatCompletion
						err = healthCheckClient.Post(generateUrl(modifiedHeaders), util.HeaderToSlice(modifiedHeaders), modifiedBody, func(statusCode int, responseHeaders http.Header, responseBody []byte) {
							if statusCode == 200 {
								c.handleAvailableApiToken(apiToken)
							}
						}, uint32(c.failover.healthCheckTimeout))
						if err != nil {
							log.Errorf("Failed to perform health check request: %v", err)
						}
					}
				}
			}
		})
	}
	return nil
}

func generateUrl(header http.Header) string {
	return fmt.Sprintf("https://%s%s", header.Get(":authority"), header.Get(":path"))
}

func (c *ProviderConfig) transformRequestHeadersAndBody(ctx wrapper.HttpContext, activeProvider Provider, headers [][2]string, body []byte) (http.Header, []byte, error) {
	modifiedHeaders := util.SliceToHeader(headers)
	if handler, ok := activeProvider.(TransformRequestHeadersHandler); ok {
		handler.TransformRequestHeaders(ctx, ApiNameChatCompletion, modifiedHeaders)
	}

	// Apply providerBasePath if configured
	if c.providerBasePath != "" {
		modifiedHeaders.Set(":path", c.applyProviderBasePath(modifiedHeaders.Get(":path")))
	}

	var err error
	if handler, ok := activeProvider.(TransformRequestBodyHandler); ok {
		body, err = handler.TransformRequestBody(ctx, ApiNameChatCompletion, body)
	} else if handler, ok := activeProvider.(TransformRequestBodyHeadersHandler); ok {
		body, err = handler.TransformRequestBodyHeaders(ctx, ApiNameChatCompletion, body, modifiedHeaders)
	} else {
		body, err = c.defaultTransformRequestBody(ctx, ApiNameChatCompletion, body)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to transform request body: %v", err)
	}

	return modifiedHeaders, body, nil
}

func createHttpContext() *wrapper.CommonHttpCtx[any] {
	setParseConfig := wrapper.ParseConfig[any](parseConfig)
	vmCtx := wrapper.NewCommonVmCtx[any]("health-check", setParseConfig)
	pluginCtx := vmCtx.NewPluginContext(rand.Uint32())
	ctx := pluginCtx.NewHttpContext(rand.Uint32()).(*wrapper.CommonHttpCtx[any])
	return ctx
}

func (c *ProviderConfig) generateRequestHeadersAndBody() (HealthCheckEndpoint, [][2]string, []byte) {
	data, _, err := proxywasm.GetSharedData(c.failover.ctxHealthCheckEndpoint)
	if err != nil {
		log.Errorf("Failed to get request host and path: %v", err)
	}
	var healthCheckEndpoint HealthCheckEndpoint
	err = json.Unmarshal(data, &healthCheckEndpoint)
	if err != nil {
		log.Errorf("Failed to unmarshal request host and path: %v", err)
	}

	headers := [][2]string{
		{"content-type", "application/json"},
		{":authority", healthCheckEndpoint.Host},
		{":path", healthCheckEndpoint.Path},
	}
	body := []byte(fmt.Sprintf(`{
                      "model": "%s",
                      "messages": [
                        {
                          "role": "user",
                          "content": "who are you?"
                        }
                      ]
                    }`, c.failover.healthCheckModel))
	return healthCheckEndpoint, headers, body
}

func (c *ProviderConfig) tryAcquireOrRenewLease(vmID string) bool {
	now := time.Now().Unix()

	data, cas, err := proxywasm.GetSharedData(c.failover.ctxVmLease)
	if err != nil {
		if errors.Is(err, types.ErrorStatusNotFound) {
			return c.setLease(vmID, now, cas)
		} else {
			log.Errorf("Failed to get lease: %v", err)
			return false
		}
	}
	if data == nil {
		return c.setLease(vmID, now, cas)
	}

	var lease Lease
	err = json.Unmarshal(data, &lease)
	if err != nil {
		log.Errorf("Failed to unmarshal lease data: %v", err)
		return false
	}
	// If vmID is itself, try to renew the lease directly
	// If the lease is expired (60s), try to acquire the lease
	if lease.VMID == vmID || now-lease.Timestamp > 60 {
		lease.VMID = vmID
		lease.Timestamp = now
		return c.setLease(vmID, now, cas)
	}

	return false
}

func (c *ProviderConfig) setLease(vmID string, timestamp int64, cas uint32) bool {
	lease := Lease{
		VMID:      vmID,
		Timestamp: timestamp,
	}
	leaseByte, err := json.Marshal(lease)
	if err != nil {
		log.Errorf("Failed to marshal lease data: %v", err)
		return false
	}

	if err := proxywasm.SetSharedData(c.failover.ctxVmLease, leaseByte, cas); err != nil {
		log.Errorf("Failed to set or renew lease: %v", err)
		return false
	}
	return true
}

func generateVMID() string {
	return uuid.New().String()
}

// When number of request successes exceeds the threshold during health check,
// add the apiToken back to the available list and remove it from the unavailable list
func (c *ProviderConfig) handleAvailableApiToken(apiToken string) {
	successApiTokenRequestCount, _, err := getApiTokenRequestCount(c.failover.ctxApiTokenRequestSuccessCount)
	if err != nil {
		log.Errorf("Failed to get successApiTokenRequestCount: %v", err)
		return
	}

	successCount := successApiTokenRequestCount[apiToken] + 1
	if successCount >= c.failover.successThreshold {
		log.Infof("healthcheck after failover: apiToken %s is available now, add it back to the apiTokens list", apiToken)
		removeApiToken(c.failover.ctxUnavailableApiTokens, apiToken)
		addApiToken(c.failover.ctxApiTokens, apiToken)
		resetApiTokenRequestCount(c.failover.ctxApiTokenRequestSuccessCount, apiToken)
	} else {
		log.Debugf("apiToken %s is still unavailable, the number of health check passed: %d, continue to health check...", apiToken, successCount)
		addApiTokenRequestCount(c.failover.ctxApiTokenRequestSuccessCount, apiToken)
	}
}

// When number of request failures exceeds the threshold,
// remove the apiToken from the available list and add it to the unavailable list
func (c *ProviderConfig) handleUnavailableApiToken(ctx wrapper.HttpContext, apiToken string) {
	failureApiTokenRequestCount, _, err := getApiTokenRequestCount(c.failover.ctxApiTokenRequestFailureCount)
	if err != nil {
		log.Errorf("Failed to get failureApiTokenRequestCount: %v", err)
		return
	}

	availableTokens, _, err := getApiTokens(c.failover.ctxApiTokens)
	if err != nil {
		log.Errorf("Failed to get available apiToken: %v", err)
		return
	}
	// unavailable apiToken has been removed from the available list
	if !containsElement(availableTokens, apiToken) {
		return
	}

	failureCount := failureApiTokenRequestCount[apiToken] + 1
	if failureCount >= c.failover.failureThreshold {
		log.Infof("failover: apiToken %s is unavailable now, remove it from apiTokens list", apiToken)
		removeApiToken(c.failover.ctxApiTokens, apiToken)
		addApiToken(c.failover.ctxUnavailableApiTokens, apiToken)
		resetApiTokenRequestCount(c.failover.ctxApiTokenRequestFailureCount, apiToken)
		// Set the request host and path to shared data in case they are needed in apiToken health check
		c.setHealthCheckEndpoint(ctx)
	} else {
		log.Debugf("apiToken %s is still available as it has not reached the failure threshold, the number of failed request: %d", apiToken, failureCount)
		addApiTokenRequestCount(c.failover.ctxApiTokenRequestFailureCount, apiToken)
	}
}

func addApiToken(key, apiToken string) {
	modifyApiToken(key, apiToken, addApiTokenOperation)
}

func removeApiToken(key, apiToken string) {
	modifyApiToken(key, apiToken, removeApiTokenOperation)
}

func modifyApiToken(key, apiToken, op string) {
	for attempt := 1; attempt <= casMaxRetries; attempt++ {
		apiTokens, cas, err := getApiTokens(key)
		if err != nil {
			log.Errorf("Failed to get %s: %v", key, err)
			continue
		}

		exists := containsElement(apiTokens, apiToken)
		if op == addApiTokenOperation && exists {
			log.Debugf("%s already exists in %s", apiToken, key)
			return
		} else if op == removeApiTokenOperation && !exists {
			log.Debugf("%s does not exist in %s", apiToken, key)
			return
		}

		if op == addApiTokenOperation {
			apiTokens = append(apiTokens, apiToken)
		} else {
			apiTokens = removeElement(apiTokens, apiToken)
		}

		if err := setApiTokens(key, apiTokens, cas); err == nil {
			log.Debugf("Successfully updated %s in %s", apiToken, key)
			return
		} else if !errors.Is(err, types.ErrorStatusCasMismatch) {
			log.Errorf("Failed to set %s after %d attempts: %v", key, attempt, err)
			return
		}

		log.Errorf("CAS mismatch when setting %s, retrying...", key)
	}
}

func getApiTokens(key string) ([]string, uint32, error) {
	data, cas, err := proxywasm.GetSharedData(key)
	if err != nil {
		if errors.Is(err, types.ErrorStatusNotFound) {
			return []string{}, cas, nil
		}
		return nil, 0, err
	}
	if data == nil {
		return []string{}, cas, nil
	}

	var apiTokens []string
	if err = json.Unmarshal(data, &apiTokens); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal tokens: %v", err)
	}

	return apiTokens, cas, nil
}

func setApiTokens(key string, apiTokens []string, cas uint32) error {
	data, err := json.Marshal(apiTokens)
	if err != nil {
		return fmt.Errorf("failed to marshal tokens: %v", err)
	}
	return proxywasm.SetSharedData(key, data, cas)
}

func removeElement(slice []string, s string) []string {
	for i := 0; i < len(slice); i++ {
		if slice[i] == s {
			slice = append(slice[:i], slice[i+1:]...)
			i--
		}
	}
	return slice
}

func containsElement(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func getApiTokenRequestCount(key string) (map[string]int64, uint32, error) {
	data, cas, err := proxywasm.GetSharedData(key)
	if err != nil {
		if errors.Is(err, types.ErrorStatusNotFound) {
			return make(map[string]int64), cas, nil
		}
		return nil, 0, err
	}

	if data == nil {
		return make(map[string]int64), cas, nil
	}

	var apiTokens map[string]int64
	err = json.Unmarshal(data, &apiTokens)
	if err != nil {
		return nil, 0, err
	}
	return apiTokens, cas, nil
}

func addApiTokenRequestCount(key, apiToken string) {
	modifyApiTokenRequestCount(key, apiToken, addApiTokenRequestCountOperation)
}

func resetApiTokenRequestCount(key, apiToken string) {
	modifyApiTokenRequestCount(key, apiToken, resetApiTokenRequestCountOperation)
}

func (c *ProviderConfig) ResetApiTokenRequestFailureCount(apiTokenInUse string) {
	if c.isFailoverEnabled() {
		failureApiTokenRequestCount, _, err := getApiTokenRequestCount(c.failover.ctxApiTokenRequestFailureCount)
		if err != nil {
			log.Errorf("failed to get failureApiTokenRequestCount: %v", err)
		}
		if _, ok := failureApiTokenRequestCount[apiTokenInUse]; ok {
			log.Infof("Reset apiToken %s request failure count", apiTokenInUse)
			resetApiTokenRequestCount(c.failover.ctxApiTokenRequestFailureCount, apiTokenInUse)
		}
	}
}

func modifyApiTokenRequestCount(key, apiToken string, op string) {
	for attempt := 1; attempt <= casMaxRetries; attempt++ {
		apiTokenRequestCount, cas, err := getApiTokenRequestCount(key)
		if err != nil {
			log.Errorf("Failed to get %s: %v", key, err)
			continue
		}

		if op == resetApiTokenRequestCountOperation {
			delete(apiTokenRequestCount, apiToken)
		} else {
			apiTokenRequestCount[apiToken]++
		}

		apiTokenRequestCountByte, err := json.Marshal(apiTokenRequestCount)
		if err != nil {
			log.Errorf("Failed to marshal apiTokenRequestCount: %v", err)
		}

		if err := proxywasm.SetSharedData(key, apiTokenRequestCountByte, cas); err == nil {
			log.Debugf("Successfully updated the count of %s in %s", apiToken, key)
			return
		} else if !errors.Is(err, types.ErrorStatusCasMismatch) {
			log.Errorf("Failed to set %s after %d attempts: %v", key, attempt, err)
			return
		}

		log.Errorf("CAS mismatch when setting %s, retrying...", key)
	}
}

func (c *ProviderConfig) initApiTokens() error {
	return setApiTokens(c.failover.ctxApiTokens, c.apiTokens, 0)
}

func (c *ProviderConfig) GetGlobalRandomToken() string {
	apiTokens, _, err := getApiTokens(c.failover.ctxApiTokens)
	unavailableApiTokens, _, err := getApiTokens(c.failover.ctxUnavailableApiTokens)
	log.Debugf("apiTokens: %v, unavailableApiTokens: %v", apiTokens, unavailableApiTokens)

	if err != nil {
		return ""
	}
	count := len(apiTokens)
	switch count {
	case 0:
		log.Warn("all tokens are unavailable, will use random one of the unavailable tokens")
		return unavailableApiTokens[rand.Intn(len(unavailableApiTokens))]
	case 1:
		return apiTokens[0]
	default:
		return apiTokens[rand.Intn(count)]
	}
}

func (c *ProviderConfig) GetAvailableApiToken(ctx wrapper.HttpContext) []string {
	apiTokens, _ := ctx.GetContext(c.failover.ctxAvailableApiTokensInRequest).([]string)
	return apiTokens
}

// SetAvailableApiTokens set available apiTokens of current request in the context, will be used in the retryOnFailure
func (c *ProviderConfig) SetAvailableApiTokens(ctx wrapper.HttpContext) {
	var apiTokens []string
	if c.isFailoverEnabled() {
		apiTokens, _, _ = getApiTokens(c.failover.ctxApiTokens)
	} else {
		apiTokens = c.apiTokens
	}
	ctx.SetContext(c.failover.ctxAvailableApiTokensInRequest, apiTokens)
}

func (c *ProviderConfig) isFailoverEnabled() bool {
	return c.failover.enabled
}

func (c *ProviderConfig) resetSharedData() {
	_ = proxywasm.SetSharedData(c.failover.ctxVmLease, nil, 0)
	_ = proxywasm.SetSharedData(c.failover.ctxApiTokens, nil, 0)
	_ = proxywasm.SetSharedData(c.failover.ctxUnavailableApiTokens, nil, 0)
	_ = proxywasm.SetSharedData(c.failover.ctxApiTokenRequestSuccessCount, nil, 0)
	_ = proxywasm.SetSharedData(c.failover.ctxApiTokenRequestFailureCount, nil, 0)
}

func (c *ProviderConfig) OnRequestFailed(activeProvider Provider, ctx wrapper.HttpContext, apiTokenInUse string, apiTokens []string, status string) types.Action {
	if c.isFailoverEnabled() && util.MatchStatus(status, c.failover.failoverOnStatus) {
		log.Warnf("apiToken:%s need failover, error status:%s", apiTokenInUse, status)
		c.handleUnavailableApiToken(ctx, apiTokenInUse)
	}
	if c.IsRetryOnFailureEnabled() && util.MatchStatus(status, c.retryOnFailure.retryOnStatus) {
		log.Warnf("need retry, notice that retry response will be bufferd, error status:%s", status)
		err := c.retryFailedRequest(activeProvider, ctx, apiTokenInUse, apiTokens)
		if err != nil {
			log.Errorf("retryFailedRequest failed, err:%v", err)
			return types.ActionContinue
		}
		return types.HeaderStopAllIterationAndWatermark
	}
	return types.ActionContinue
}

func isNotStreamingResponse(ctx wrapper.HttpContext) bool {
	return ctx.GetContext(ctxKeyIsStreaming) != nil && !ctx.GetContext(ctxKeyIsStreaming).(bool)
}

func (c *ProviderConfig) GetApiTokenInUse(ctx wrapper.HttpContext) string {
	token, _ := ctx.GetContext(c.failover.ctxApiTokenInUse).(string)
	return token
}

func (c *ProviderConfig) SetApiTokenInUse(ctx wrapper.HttpContext) {
	var apiToken string
	// if enable apiToken failover, only use available apiToken from global apiTokens list
	if c.isFailoverEnabled() {
		apiToken = c.GetGlobalRandomToken()
	} else {
		apiToken = c.GetOrSetTokenWithContext(ctx)
	}
	log.Debugf("Use apiToken %s to send request", apiToken)
	ctx.SetContext(c.failover.ctxApiTokenInUse, apiToken)
}

func (c *ProviderConfig) setHealthCheckEndpoint(ctx wrapper.HttpContext) {
	cluster, err := proxywasm.GetProperty([]string{"cluster_name"})
	if err != nil {
		log.Errorf("Failed to get cluster_name: %v", err)
	}

	host := ctx.GetStringContext(CtxRequestHost, "")
	path := ctx.GetStringContext(CtxRequestPath, "")
	if host == "" || path == "" {
		log.Errorf("get host or path failed, host:%s, path:%s", host, path)
		return
	}

	healthCheckEndpoint := HealthCheckEndpoint{
		Host:    host,
		Path:    path,
		Cluster: string(cluster),
	}

	healthCheckEndpointByte, err := json.Marshal(healthCheckEndpoint)
	if err != nil {
		log.Errorf("Failed to marshal request host and path: %v", err)

	}
	err = proxywasm.SetSharedData(c.failover.ctxHealthCheckEndpoint, healthCheckEndpointByte, 0)
	if err != nil {
		log.Errorf("Failed to set request host and path: %v", err)
	}
}
