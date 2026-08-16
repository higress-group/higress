package wasmplugin

import (
	"errors"
	"strconv"
	"strings"

	"github.com/corazawaf/coraza/v3"
	"github.com/corazawaf/coraza/v3/debuglog"
	ctypes "github.com/corazawaf/coraza/v3/types"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func PluginStart() {
	wrapper.SetCtx(
		"waf-plugin-go",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
		wrapper.ProcessResponseBodyBy(onHttpResponseBody),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
		wrapper.ProcessStreamDoneBy(onHttpStreamDone),
	)
}

type WafConfig struct {
	waf coraza.WAF
	// tx  ctypes.Transaction
}

func parseConfig(json gjson.Result, config *WafConfig, log log.Log) error {
	var secRules []string
	var value gjson.Result
	value = json.Get("useCRS")
	if value.Exists() {
		if value.Bool() {
			secRules = append(secRules, "Include @demo-conf")
			secRules = append(secRules, "Include @crs-setup-demo-conf")
			secRules = append(secRules, "Include @owasp_crs/*.conf")
			secRules = append(secRules, "SecRuleEngine On")
		}
	}
	value = json.Get("secRules")
	if value.Exists() {
		for _, item := range json.Get("secRules").Array() {
			rule := item.String()
			secRules = append(secRules, rule)
		}
	}

	conf := coraza.NewWAFConfig().
		WithErrorCallback(logError).
		WithDebugLogger(debuglog.DefaultWithPrinterFactory(logPrinterFactory)).
		WithRootFS(root)
	// error: Failed to load Wasm module due to a missing import: wasi_snapshot_preview1.fd_filestat_get
	// because without fs.go
	waf, err := coraza.NewWAF(conf.WithDirectives(strings.Join(secRules, "\n")))

	config.waf = waf
	if err != nil {
		log.Errorf("Failed to create waf conf: %v", err)
		return errors.New("failed to create waf conf")
	}

	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config WafConfig, log log.Log) types.Action {
	ctx.SetContext("skipwaf", false)

	if ignoreBody() {
		ctx.DontReadRequestBody()
		ctx.DontReadResponseBody()
		ctx.SetContext("skipwaf", true)
		return types.ActionContinue
	}

	ctx.SetContext("interruptionHandled", false)
	ctx.SetContext("processedRequestBody", false)
	ctx.SetContext("processedResponseBody", false)
	ctx.SetContext("tx", config.waf.NewTransaction())

	tx := ctx.GetContext("tx").(ctypes.Transaction)

	protocol, err := proxywasm.GetProperty([]string{"request", "protocol"})
	if err != nil {
		// TODO(anuraaga): HTTP protocol is commonly required in WAF rules, we should probably
		// fail fast here, but proxytest does not support properties yet.
		protocol = []byte("HTTP/2.0")
	}

	ctx.SetContext("httpProtocol", string(protocol))

	// Note the pseudo-header :path includes the query.
	// See https://httpwg.org/specs/rfc9113.html#rfc.section.8.3.1
	uri, err := proxywasm.GetHttpRequestHeader(":path")
	if err != nil {
		log.Error("Failed to get :path")
		return types.ActionContinue
	}

	// This currently relies on Envoy's behavior of mapping all requests to HTTP/2 semantics
	// and its request properties, but they may not be true of other proxies implementing
	// proxy-wasm.

	if tx.IsRuleEngineOff() {
		return types.ActionContinue
	}
	// OnHttpRequestHeaders does not terminate if IP/Port retrieve goes wrong
	srcIP, srcPort := retrieveAddressInfo(log, "source")
	dstIP, dstPort := retrieveAddressInfo(log, "destination")

	tx.ProcessConnection(srcIP, srcPort, dstIP, dstPort)

	method, err := proxywasm.GetHttpRequestHeader(":method")
	if err != nil {
		log.Error("Failed to get :method")
		return types.ActionContinue
	}

	tx.ProcessURI(uri, method, string(protocol))

	hs, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		log.Error("Failed to get request headers")
		return types.ActionContinue
	}

	for _, h := range hs {
		tx.AddRequestHeader(h[0], h[1])
	}

	// CRS rules tend to expect Host even with HTTP/2
	authority, err := proxywasm.GetHttpRequestHeader(":authority")
	if err == nil {
		tx.AddRequestHeader("Host", authority)
		tx.SetServerName(parseServerName(log, authority))
	}

	interruption := tx.ProcessRequestHeaders()
	if interruption != nil {
		return handleInterruption(ctx, "http_request_headers", interruption, log)
	}

	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config WafConfig, body []byte, log log.Log) types.Action {
	if ctx.GetBoolContext("interruptionHandled", false) {
		return types.ActionContinue
	}

	tx, ok := ctx.GetContext("tx").(ctypes.Transaction)
	if !ok {
		return types.ActionContinue
	}

	if tx.IsRuleEngineOff() {
		return types.ActionContinue
	}

	// Do not perform any action related to request body data if SecRequestBodyAccess is set to false
	if !tx.IsRequestBodyAccessible() {
		log.Info("Skipping request body inspection, SecRequestBodyAccess is off.")
		// ProcessRequestBody is still performed for phase 2 rules, checking already populated variables
		ctx.SetContext("processedRequestBody", true)
		interruption, err := tx.ProcessRequestBody()
		if err != nil {
			log.Error("Failed to process request body")
			return types.ActionContinue
		}

		if interruption != nil {
			return handleInterruption(ctx, "http_request_body", interruption, log)
		}

		return types.ActionContinue
	}

	interruption, _, err := tx.WriteRequestBody(body)
	if err != nil {
		log.Error("Failed to write request body")
		return types.ActionContinue
	}

	if interruption != nil {
		return handleInterruption(ctx, "http_request_body", interruption, log)
	}

	ctx.SetContext("processedRequestBody", true)
	interruption, err = tx.ProcessRequestBody()
	if err != nil {
		log.Error("Failed to process request body")
		return types.ActionContinue
	}
	if interruption != nil {
		return handleInterruption(ctx, "http_request_body", interruption, log)
	}

	return types.ActionContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config WafConfig, log log.Log) types.Action {
	if ctx.GetBoolContext("skipwaf", false) {
		return types.ActionContinue
	}

	if ctx.GetBoolContext("interruptionHandled", false) {
		return types.ActionContinue
	}

	tx, ok := ctx.GetContext("tx").(ctypes.Transaction)
	if !ok {
		return types.ActionContinue
	}

	if tx.IsRuleEngineOff() {
		return types.ActionContinue
	}

	// Requests without body won't call OnHttpRequestBody, but there are rules in the request body
	// phase that still need to be executed. If they haven't been executed yet, now is the time.
	if !ctx.GetBoolContext("processedRequestBody", false) {
		ctx.SetContext("processedRequestBody", true)
		interruption, err := tx.ProcessRequestBody()
		if err != nil {
			log.Error("Failed to process request body")
			return types.ActionContinue
		}
		if interruption != nil {
			return handleInterruption(ctx, "http_response_headers", interruption, log)
		}
	}

	status, err := proxywasm.GetHttpResponseHeader(":status")
	if err != nil {
		log.Error("Failed to get :status")
		return types.ActionContinue
	}
	code, err := strconv.Atoi(status)
	if err != nil {
		code = 0
	}

	hs, err := proxywasm.GetHttpResponseHeaders()
	if err != nil {
		log.Error("Failed to get response headers")
		return types.ActionContinue
	}

	for _, h := range hs {
		tx.AddResponseHeader(h[0], h[1])
	}

	httpProtocol, _ := ctx.GetContext("httpProtocol").(string)
	interruption := tx.ProcessResponseHeaders(code, httpProtocol)
	if interruption != nil {
		return handleInterruption(ctx, "http_response_headers", interruption, log)
	}

	return types.ActionContinue
}

func onHttpResponseBody(ctx wrapper.HttpContext, config WafConfig, body []byte, log log.Log) types.Action {
	if ctx.GetBoolContext("interruptionHandled", false) {
		// At response body phase, proxy-wasm currently relies on emptying the response body as a way of
		// interruption the response. See https://github.com/corazawaf/coraza-proxy-wasm/issues/26.
		// If OnHttpResponseBody is called again and an interruption has already been raised, it means that
		// we have to keep going with the sanitization of the response, emptying it.
		// Sending the crafted HttpResponse with empty body, we don't expect to trigger OnHttpResponseBody
		// log.Warn("Response body interruption already handled, keeping replacing the body")
		// Interruption happened, we don't want to send response body data
		return replaceResponseBodyWhenInterrupted(log, replaceResponseBody)
	}

	tx, ok := ctx.GetContext("tx").(ctypes.Transaction)
	if !ok {
		return types.ActionContinue
	}

	if tx.IsRuleEngineOff() {
		return types.ActionContinue
	}

	// Do not perform any action related to response body data if SecResponseBodyAccess is set to false
	if !tx.IsResponseBodyAccessible() {
		log.Debug("Skipping response body inspection, SecResponseBodyAccess is off.")
		// ProcessResponseBody is performed for phase 4 rules, checking already populated variables
		ctx.SetContext("processedResponseBody", true)
		interruption, err := tx.ProcessResponseBody()
		if err != nil {
			log.Error("Failed to process response body")
			return types.ActionContinue
		}

		if interruption != nil {
			// Proxy-wasm can not anymore deny the response. The best interruption is emptying the body
			// Coraza Multiphase evaluation will help here avoiding late interruptions
			return handleInterruption(ctx, "http_response_body", interruption, log)
		}
		return types.ActionContinue
	}

	interruption, _, err := tx.WriteResponseBody(body)
	if err != nil {
		log.Error("Failed to write response body")
		return types.ActionContinue
	}
	if interruption != nil {
		return handleInterruption(ctx, "http_response_body", interruption, log)
	}

	// We have already sent response headers, an unauthorized response can not be sent anymore,
	// but we can still drop the response to prevent leaking sensitive content.
	// The error will also be logged by Coraza.
	ctx.SetContext("processedResponseBody", true)
	interruption, err = tx.ProcessResponseBody()
	if err != nil {
		log.Error("Failed to process response body")
		return types.ActionContinue
	}
	if interruption != nil {
		return handleInterruption(ctx, "http_response_body", interruption, log)
	}
	return types.ActionContinue
}

func onHttpStreamDone(ctx wrapper.HttpContext, config WafConfig, log log.Log) {
	if ctx.GetBoolContext("skipwaf", false) {
		return
	}

	tx, ok := ctx.GetContext("tx").(ctypes.Transaction)
	if !ok {
		return
	}

	if !tx.IsRuleEngineOff() {
		// Responses without body won't call OnHttpResponseBody, but there are rules in the response body
		// phase that still need to be executed. If they haven't been executed yet, now is the time.
		if !ctx.GetBoolContext("processedResponseBody", false) {
			ctx.SetContext("processedResponseBody", true)
			_, err := tx.ProcessResponseBody()
			if err != nil {
				log.Error("Failed to process response body")
			}
		}
	}

	tx.ProcessLogging()

	_ = tx.Close()
}
