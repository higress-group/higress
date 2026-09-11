// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/a2a"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

const (
	modeEnforce = "enforce"
	modeAudit   = "audit"
)

var trustedHeaderFields = []struct {
	name string
	get  func(a2a.Metadata) string
}{
	{"version", func(m a2a.Metadata) string { return m.Version }},
	{"binding", func(m a2a.Metadata) string { return m.Binding }},
	{"method", func(m a2a.Metadata) string { return m.Method }},
	{"request-id", func(m a2a.Metadata) string { return m.RequestID }},
	{"task-id", func(m a2a.Metadata) string { return m.TaskID }},
	{"context-id", func(m a2a.Metadata) string { return m.ContextID }},
	{"message-id", func(m a2a.Metadata) string { return m.MessageID }},
	{"tenant", func(m a2a.Metadata) string { return m.Tenant }},
	{"task-state", func(m a2a.Metadata) string { return m.TaskState }},
	{"stream-event-type", func(m a2a.Metadata) string { return m.StreamEventType }},
	{"error-code", func(m a2a.Metadata) string { return m.ErrorCode }},
	{"parse-status", func(m a2a.Metadata) string { return m.ParseStatus }},
}

type pluginConfig struct {
	ProtocolVersion string `json:"protocolVersion"`
	Mode            string `json:"mode"`
	Legacy03        struct {
		Enabled bool `json:"enabled"`
	} `json:"legacy03"`
	Agent struct {
		ID              string `json:"id"`
		ExternalBaseURL string `json:"externalBaseURL"`
	} `json:"agent"`
	AgentCard struct {
		Path             string `json:"path"`
		Rewrite          *bool  `json:"rewrite"`
		SignatureMode    string `json:"signatureMode"`
		MaxResponseBytes int    `json:"maxResponseBytes"`

		rewrite bool
	} `json:"agentCard"`
	JSONRPC struct {
		MaxRequestBytes  int      `json:"maxRequestBytes"`
		MaxSSEEventBytes int      `json:"maxSSEEventBytes"`
		AllowedMethods   []string `json:"allowedMethods"`
	} `json:"jsonrpc"`
	Authorization struct {
		ExposeInternalHeaders *bool `json:"exposeInternalHeaders"`
	} `json:"authorization"`

	exposeHeaders bool
}

func init() {
	wrapper.SetCtx(
		"a2a-protocol",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBody(onHttpStreamingResponseBody),
		wrapper.ProcessResponseBody(onHttpResponseBody),
	)
}

func parseConfig(raw gjson.Result, config *pluginConfig) error {
	if err := json.Unmarshal([]byte(raw.Raw), config); err != nil {
		return fmt.Errorf("failed to parse a2a-protocol config: %w", err)
	}
	if config.ProtocolVersion == "" {
		config.ProtocolVersion = "1.0"
	}
	if config.ProtocolVersion != "1.0" {
		return fmt.Errorf("protocolVersion must be 1.0")
	}
	if config.Mode == "" {
		config.Mode = modeEnforce
	}
	if config.Mode != modeEnforce && config.Mode != modeAudit {
		return fmt.Errorf("mode must be enforce or audit")
	}
	if config.Agent.ID == "" || len(config.Agent.ID) > a2a.MaxMetadataValueBytes {
		return fmt.Errorf("agent.id must contain 1-%d bytes", a2a.MaxMetadataValueBytes)
	}
	if config.AgentCard.Path == "" {
		config.AgentCard.Path = canonicalAgentCardPath
	}
	if config.AgentCard.Path[0] != '/' {
		return fmt.Errorf("agentCard.path must be absolute")
	}
	config.AgentCard.rewrite = config.AgentCard.Rewrite == nil || *config.AgentCard.Rewrite
	if config.AgentCard.SignatureMode == "" {
		config.AgentCard.SignatureMode = cardSignaturePreserve
	}
	if config.AgentCard.SignatureMode != cardSignaturePreserve && config.AgentCard.SignatureMode != cardSignatureResign {
		return fmt.Errorf("agentCard.signatureMode must be preserve or resign")
	}
	if config.AgentCard.MaxResponseBytes <= 0 {
		config.AgentCard.MaxResponseBytes = defaultMaxCardBytes
	}
	if config.AgentCard.MaxResponseBytes > hardMaxCardBytes {
		return fmt.Errorf("agentCard.maxResponseBytes exceeds hard limit %d", hardMaxCardBytes)
	}
	if config.Agent.ExternalBaseURL != "" {
		if err := validateHTTPSURL(config.Agent.ExternalBaseURL); err != nil {
			return fmt.Errorf("configured agent.externalBaseURL: %w", err)
		}
	}
	if config.JSONRPC.MaxRequestBytes <= 0 {
		config.JSONRPC.MaxRequestBytes = a2a.DefaultMaxRequestBytes
	}
	if config.JSONRPC.MaxRequestBytes > a2a.HardMaxRequestBytes {
		return fmt.Errorf("jsonrpc.maxRequestBytes exceeds hard limit %d", a2a.HardMaxRequestBytes)
	}
	if config.JSONRPC.MaxSSEEventBytes <= 0 {
		config.JSONRPC.MaxSSEEventBytes = a2a.DefaultMaxSSEEventBytes
	}
	if config.JSONRPC.MaxSSEEventBytes > a2a.HardMaxRequestBytes {
		return fmt.Errorf("jsonrpc.maxSSEEventBytes exceeds hard limit %d", a2a.HardMaxRequestBytes)
	}
	config.exposeHeaders = config.Authorization.ExposeInternalHeaders == nil || *config.Authorization.ExposeInternalHeaders
	for i, method := range config.JSONRPC.AllowedMethods {
		canonical, _, err := a2a.CanonicalMethod(method, config.Legacy03.Enabled)
		if err != nil {
			return fmt.Errorf("jsonrpc.allowedMethods[%d]: %w", i, err)
		}
		config.JSONRPC.AllowedMethods[i] = canonical
	}
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config pluginConfig) types.Action {
	ctx.DisableReroute()
	removeSpoofedHeaders()
	if ctx.Method() == "GET" && isAgentCardPath(ctx.Path(), config.AgentCard.Path) {
		ctx.DontReadRequestBody()
		ctx.SetContext("a2a.card", true)
		ctx.SetContext("a2a.card.legacy", isLegacyAgentCardPath(ctx.Path()))
		externalURL, err := deriveExternalAgentURL(config)
		if err != nil {
			ctx.SetContext("a2a.card.external_error", true)
		} else {
			ctx.SetContext("a2a.card.external_url", externalURL)
		}
		return types.ActionContinue
	}
	contentType, _ := proxywasm.GetHttpRequestHeader("content-type")
	if ctx.Method() != "POST" || !isJSONRPCContentType(contentType) {
		ctx.DontReadRequestBody()
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	ctx.SetRequestBodyBufferLimit(uint32(config.JSONRPC.MaxRequestBytes))
	ctx.SetContext("a2a.candidate", true)
	if contentEncoding, _ := proxywasm.GetHttpRequestHeader("content-encoding"); strings.TrimSpace(contentEncoding) != "" {
		ctx.DontReadRequestBody()
		return rejectOrAudit(ctx, config, a2a.Metadata{Binding: "jsonrpc", ParseStatus: "invalid"}, -32600, "A2A Content-Encoding is not supported")
	}
	if !ctx.HasRequestBody() {
		ctx.DontReadRequestBody()
		return rejectOrAudit(ctx, config, a2a.Metadata{Binding: "jsonrpc", ParseStatus: "invalid"}, -32600, "A2A request body is required")
	}
	if contentLengthOversized(config) {
		meta := a2a.Metadata{Binding: "jsonrpc", ParseStatus: "oversized"}
		version, _ := proxywasm.GetHttpRequestHeader("a2a-version")
		meta.Version = version
		publishMetadata(ctx, config, meta, true, true)
		if config.Mode == modeAudit {
			ctx.DontReadRequestBody()
			return types.ActionContinue
		}
		_ = proxywasm.SendHttpResponse(413, [][2]string{{"content-type", "application/json"}}, []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"A2A request exceeds configured limit"}}`), -1)
		return types.ActionPause
	}
	// Keep request headers in the filter chain until the JSON-RPC body has
	// been validated and canonical metadata headers have been published.
	return types.HeaderStopIteration
}

func onHttpRequestBody(ctx wrapper.HttpContext, config pluginConfig, body []byte) types.Action {
	version, allowLegacy, versionStatus := effectiveVersion(config)
	if versionStatus != "" {
		return rejectOrAudit(ctx, config, a2a.Metadata{Version: version, Binding: "jsonrpc", ParseStatus: "invalid"}, -32009, versionStatus)
	}
	meta, err := a2a.ParseRequest(body, config.JSONRPC.MaxRequestBytes, version, allowLegacy)
	if err != nil {
		code := -32600
		if errors.Is(err, a2a.ErrUnknownMethod) {
			code = -32601
		}
		return rejectOrAudit(ctx, config, meta, code, err.Error())
	}
	if len(config.JSONRPC.AllowedMethods) > 0 && !slices.Contains(config.JSONRPC.AllowedMethods, meta.Method) {
		return rejectOrAudit(ctx, config, meta, -32601, "A2A method is not allowed")
	}
	publishMetadata(ctx, config, meta, true, true)
	ctx.SetContext("a2a.active", true)
	ctx.SetContext("a2a.version", meta.Version)
	ctx.SetContext("a2a.method", meta.Method)
	return types.ActionContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config pluginConfig) types.Action {
	removeSpoofedResponseHeaders()
	if ctx.GetBoolContext("a2a.card", false) {
		status, _ := proxywasm.GetHttpResponseHeader(":status")
		statusCode, _ := strconv.Atoi(status)
		if statusCode < 200 || statusCode >= 300 {
			ctx.DontReadResponseBody()
			return types.ActionContinue
		}
		if !ctx.HasResponseBody() {
			markAgentCardRejection(ctx)
			return types.ActionContinue
		}
		contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
		if !isAgentCardContentType(contentType) {
			markAgentCardRejection(ctx)
			return types.ActionContinue
		}
		if contentEncoding, _ := proxywasm.GetHttpResponseHeader("content-encoding"); strings.TrimSpace(contentEncoding) != "" {
			markAgentCardRejection(ctx)
			return types.ActionContinue
		}
		if ctx.GetBoolContext("a2a.card.external_error", false) || responseContentLengthOversized(config.AgentCard.MaxResponseBytes) {
			markAgentCardRejection(ctx)
			return types.ActionContinue
		}
		ctx.BufferResponseBody()
		ctx.SetResponseBodyBufferLimit(uint32(config.AgentCard.MaxResponseBytes))
		// Hold response headers until the bounded body has been validated so a
		// malformed Card can still be converted into an HTTP 502 response.
		return types.HeaderStopIteration
	}
	if !ctx.GetBoolContext("a2a.active", false) {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	if !ctx.HasResponseBody() || ctx.IsBinaryResponseBody() {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
	if strings.EqualFold(mediaType(contentType), "text/event-stream") && isStreamingMethod(ctx.GetStringContext("a2a.method", "")) {
		ctx.SetContext("a2a.sse", a2a.NewSSEParser(config.JSONRPC.MaxSSEEventBytes))
		return types.ActionContinue
	}
	ctx.BufferResponseBody()
	ctx.SetResponseBodyBufferLimit(uint32(config.JSONRPC.MaxRequestBytes))
	// Unary response metadata is derived from the body, so retain response
	// headers until the body callback has published the trusted fields.
	return types.HeaderStopIteration
}

func onHttpStreamingResponseBody(ctx wrapper.HttpContext, config pluginConfig, data []byte, endOfStream bool) []byte {
	if rejection := ctx.GetByteSliceContext("a2a.card.rejection", nil); rejection != nil {
		if ctx.GetBoolContext("a2a.card.rejection_sent", false) {
			return nil
		}
		ctx.SetContext("a2a.card.rejection_sent", true)
		return rejection
	}
	value := ctx.GetContext("a2a.sse")
	parser, ok := value.(*a2a.SSEParser)
	if !ok {
		return data
	}
	events := parser.Feed(data, endOfStream, ctx.GetStringContext("a2a.version", config.ProtocolVersion), ctx.GetStringContext("a2a.method", ""))
	for _, event := range events {
		publishMetadata(ctx, config, event.Metadata, false, false)
	}
	return data
}

func onHttpResponseBody(ctx wrapper.HttpContext, config pluginConfig, body []byte) types.Action {
	if ctx.GetBoolContext("a2a.card", false) {
		result, err := processAgentCard(body, config, ctx.GetBoolContext("a2a.card.legacy", false), ctx.GetStringContext("a2a.card.external_url", ""))
		if err != nil {
			replaceWithAgentCardRejection()
			return types.ActionContinue
		}
		if result.rewritten {
			_ = proxywasm.ReplaceHttpResponseBody(result.body)
			_ = proxywasm.RemoveHttpResponseHeader("content-length")
			_ = proxywasm.ReplaceHttpResponseHeader("etag", rewrittenETag(result.body))
		}
		return types.ActionContinue
	}
	if !ctx.GetBoolContext("a2a.active", false) {
		return types.ActionContinue
	}
	meta, err := a2a.ParseResponse(body, config.JSONRPC.MaxRequestBytes, ctx.GetStringContext("a2a.version", config.ProtocolVersion), ctx.GetStringContext("a2a.method", ""))
	if err != nil {
		meta.ParseStatus = "invalid"
		if errors.Is(err, a2a.ErrOversized) {
			meta.ParseStatus = "oversized"
		}
	}
	publishMetadata(ctx, config, meta, false, true)
	return types.ActionContinue
}

func effectiveVersion(config pluginConfig) (string, bool, string) {
	version, _ := proxywasm.GetHttpRequestHeader("a2a-version")
	if version == "" {
		version = "0.3"
	}
	if version == config.ProtocolVersion {
		return version, false, ""
	}
	if version == "0.3" && config.Legacy03.Enabled {
		return version, true, ""
	}
	return version, false, "A2A VersionNotSupportedError"
}

func rejectOrAudit(ctx wrapper.HttpContext, config pluginConfig, meta a2a.Metadata, code int, message string) types.Action {
	publishMetadata(ctx, config, meta, true, true)
	if config.Mode == modeAudit {
		ctx.SetContext("a2a.active", true)
		ctx.SetContext("a2a.version", meta.Version)
		return types.ActionContinue
	}
	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      nil,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	})
	_ = proxywasm.SendHttpResponse(400, [][2]string{{"content-type", "application/json"}}, body, -1)
	return types.ActionPause
}

func publishMetadata(ctx wrapper.HttpContext, config pluginConfig, meta a2a.Metadata, request, headers bool) {
	publishField(config, "agent-id", config.Agent.ID, request, headers)
	for _, field := range trustedHeaderFields {
		value := field.get(meta)
		publishField(config, field.name, value, request, headers)
	}
}

func publishField(config pluginConfig, field, value string, request, headers bool) {
	if value == "" {
		return
	}
	if len(value) > a2a.MaxMetadataValueBytes {
		value = value[:a2a.MaxMetadataValueBytes]
	}
	_ = proxywasm.SetProperty([]string{"a2a", strings.ReplaceAll(field, "-", "_")}, []byte(value))
	if !headers || !config.exposeHeaders {
		return
	}
	name := "x-higress-a2a-" + field
	if request {
		_ = proxywasm.ReplaceHttpRequestHeader(name, value)
	} else {
		_ = proxywasm.ReplaceHttpResponseHeader(name, value)
	}
}

func removeSpoofedHeaders() {
	_ = proxywasm.RemoveHttpRequestHeader("x-higress-a2a-agent-id")
	for _, field := range trustedHeaderFields {
		_ = proxywasm.RemoveHttpRequestHeader("x-higress-a2a-" + field.name)
	}
}

func removeSpoofedResponseHeaders() {
	_ = proxywasm.RemoveHttpResponseHeader("x-higress-a2a-agent-id")
	_ = proxywasm.RemoveHttpResponseHeader("x-higress-a2a-card-cache-key")
	for _, field := range trustedHeaderFields {
		_ = proxywasm.RemoveHttpResponseHeader("x-higress-a2a-" + field.name)
	}
}

func isJSONRPCContentType(contentType string) bool {
	typeName := strings.ToLower(mediaType(contentType))
	return typeName == "application/json"
}

func mediaType(contentType string) string {
	if idx := strings.IndexByte(contentType, ';'); idx >= 0 {
		contentType = contentType[:idx]
	}
	return strings.TrimSpace(contentType)
}

func isStreamingMethod(method string) bool {
	return method == "SendStreamingMessage" || method == "SubscribeToTask"
}

func contentLengthOversized(config pluginConfig) bool {
	value, _ := proxywasm.GetHttpRequestHeader("content-length")
	length, err := strconv.Atoi(value)
	return err == nil && length > config.JSONRPC.MaxRequestBytes
}

func isAgentCardPath(requestPath, configuredPath string) bool {
	path := pathWithoutQuery(requestPath)
	return strings.HasSuffix(path, configuredPath) || strings.HasSuffix(path, canonicalAgentCardPath) || strings.HasSuffix(path, legacyAgentCardPath)
}

func isLegacyAgentCardPath(requestPath string) bool {
	return strings.HasSuffix(pathWithoutQuery(requestPath), legacyAgentCardPath)
}

func pathWithoutQuery(path string) string {
	if index := strings.IndexByte(path, '?'); index >= 0 {
		return path[:index]
	}
	return path
}

func isAgentCardContentType(contentType string) bool {
	typeName := mediaType(contentType)
	return strings.EqualFold(typeName, "application/json") || strings.EqualFold(typeName, "application/a2a+json")
}

func responseContentLengthOversized(maxBytes int) bool {
	value, _ := proxywasm.GetHttpResponseHeader("content-length")
	length, err := strconv.Atoi(value)
	return err == nil && length > maxBytes
}

func markAgentCardRejection(ctx wrapper.HttpContext) {
	_ = proxywasm.ReplaceHttpResponseHeader(":status", "502")
	_ = proxywasm.ReplaceHttpResponseHeader("content-type", "application/json")
	_ = proxywasm.RemoveHttpResponseHeader("content-length")
	_ = proxywasm.RemoveHttpResponseHeader("etag")
	_ = proxywasm.ReplaceHttpResponseHeader("cache-control", "no-store")
	ctx.SetContext("a2a.card.rejection", []byte(`{"error":"invalid A2A Agent Card"}`))
}

func replaceWithAgentCardRejection() {
	_ = proxywasm.ReplaceHttpResponseHeader(":status", "502")
	_ = proxywasm.ReplaceHttpResponseHeader("content-type", "application/json")
	_ = proxywasm.RemoveHttpResponseHeader("content-length")
	_ = proxywasm.RemoveHttpResponseHeader("etag")
	_ = proxywasm.ReplaceHttpResponseHeader("cache-control", "no-store")
	_ = proxywasm.ReplaceHttpResponseBody([]byte(`{"error":"invalid A2A Agent Card"}`))
}
