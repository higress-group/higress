package provider

import (
	"net/http"
	"strconv"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// genericProviderInitializer 用于创建一个不做能力映射的通用 Provider。
type genericProviderInitializer struct{}

// ValidateConfig 通用 Provider 不需要额外的配置校验。
func (m *genericProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	return nil
}

// DefaultCapabilities 返回空映射，表示不会做路径或能力重写。
func (m *genericProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{}
}

// CreateProvider 创建 generic provider，并沿用通用的上下文缓存能力。
func (m *genericProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(m.DefaultCapabilities())
	return &genericProvider{
		config: config,
	}, nil
}

// genericProvider 只负责公共的头部、请求体处理逻辑，不绑定任何厂商。
type genericProvider struct {
	config ProviderConfig
}

func (m *genericProvider) GetProviderType() string {
	return providerTypeGeneric
}

// OnRequestHeaders 复用通用的 handleRequestHeaders，并在配置首包超时时写入相关头部。
func (m *genericProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	m.config.handleRequestHeaders(m, ctx, apiName)
	if m.config.firstByteTimeout > 0 {
		ctx.SetContext(ctxKeyIsStreaming, true)
		m.applyFirstByteTimeout()
	}
	return nil
}

func (m *genericProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	// buffer original request body
	_ = proxywasm.ReplaceHttpRequestBody(body)
	return types.ActionContinue, nil
}

// TransformRequestHeaders 只处理鉴权与 Host 改写，不做路径重写。
func (m *genericProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	if len(m.config.apiTokens) > 0 {
		if token := m.config.GetApiTokenInUse(ctx); token != "" {
			util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+token)
		}
	}
	if m.config.genericHost != "" {
		util.OverwriteRequestHostHeader(headers, m.config.genericHost)
	}
	headers.Del("Content-Length")
}

// applyFirstByteTimeout 在配置了 firstByteTimeout 时，为所有流式请求写入超时头。
func (m *genericProvider) applyFirstByteTimeout() {
	if m.config.firstByteTimeout == 0 {
		return
	}
	err := proxywasm.ReplaceHttpRequestHeader(
		"x-envoy-upstream-rq-first-byte-timeout-ms",
		strconv.FormatUint(uint64(m.config.firstByteTimeout), 10),
	)
	if err != nil {
		log.Errorf("generic provider: failed to set first byte timeout header: %v", err)
		return
	}
	log.Debugf("[generic][firstByteTimeout] %d", m.config.firstByteTimeout)
}
