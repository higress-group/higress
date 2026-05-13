package main

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	DefaultMaxBodyBytes     = 100 * 1024 * 1024 // 100MB
	responseRewriteModeNone = ""
	responseRewriteModeJSON = "json"
	responseRewriteModeSSE  = "sse"

	contextResponseRewriteEnabled = "model_mapper.response_rewrite_enabled"
	contextResponseRewriteMode    = "model_mapper.response_rewrite_mode"
	contextResponseModelKey       = "model_mapper.response_model_key"
	contextResponseClientModel    = "model_mapper.response_client_model"
	contextResponseUpstreamModel  = "model_mapper.response_upstream_model"
	contextResponsePendingData    = "model_mapper.response_pending_data"
)

var defaultResponseModelPaths = []string{
	"model",
	"body.model",
	"response.model",
	"message.model",
	"modelVersion",
}

func main() {}

func init() {
	wrapper.SetCtx(
		"model-mapper",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBody(onHttpStreamingResponseBody),
		wrapper.ProcessResponseBody(onHttpResponseBody),
		wrapper.WithRebuildAfterRequests[Config](1000),
		wrapper.WithRebuildMaxMemBytes[Config](200*1024*1024),
	)
}

type ModelMapping struct {
	Prefix string
	Target string
}

type Config struct {
	modelKey              string
	exactModelMapping     map[string]string
	prefixModelMapping    []ModelMapping
	defaultModel          string
	enableOnPathSuffix    []string
	enableResponseMapping bool
	responseModelPaths    []string
}

func parseConfig(json gjson.Result, config *Config) error {
	config.modelKey = json.Get("modelKey").String()
	if config.modelKey == "" {
		config.modelKey = "model"
	}
	config.enableResponseMapping = false
	if enableResponseMapping := json.Get("enableResponseMapping"); enableResponseMapping.Exists() {
		if !enableResponseMapping.IsBool() {
			return errors.New("enableResponseMapping must be a boolean")
		}
		config.enableResponseMapping = enableResponseMapping.Bool()
	}

	config.responseModelPaths = make([]string, len(defaultResponseModelPaths))
	copy(config.responseModelPaths, defaultResponseModelPaths)
	if userPaths := json.Get("responseModelPaths"); userPaths.Exists() {
		if !userPaths.IsArray() {
			return errors.New("responseModelPaths must be an array of strings")
		}
		seen := make(map[string]bool)
		for _, p := range config.responseModelPaths {
			seen[p] = true
		}
		for _, p := range userPaths.Array() {
			path := p.String()
			if !seen[path] {
				config.responseModelPaths = append(config.responseModelPaths, path)
				seen[path] = true
			}
		}
	}

	modelMapping := json.Get("modelMapping")
	if modelMapping.Exists() && !modelMapping.IsObject() {
		return errors.New("modelMapping must be an object")
	}

	config.exactModelMapping = make(map[string]string)
	config.prefixModelMapping = make([]ModelMapping, 0)

	// To replicate C++ behavior (nlohmann::json iterates keys alphabetically),
	// we collect entries and sort them by key.
	type mappingEntry struct {
		key   string
		value string
	}
	var entries []mappingEntry
	modelMapping.ForEach(func(key, value gjson.Result) bool {
		entries = append(entries, mappingEntry{
			key:   key.String(),
			value: value.String(),
		})
		return true
	})
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].key < entries[j].key
	})

	for _, entry := range entries {
		key := entry.key
		value := entry.value
		if key == "*" {
			config.defaultModel = value
		} else if strings.HasSuffix(key, "*") {
			prefix := strings.TrimSuffix(key, "*")
			config.prefixModelMapping = append(config.prefixModelMapping, ModelMapping{
				Prefix: prefix,
				Target: value,
			})
		} else {
			config.exactModelMapping[key] = value
		}
	}

	enableOnPathSuffix := json.Get("enableOnPathSuffix")
	if enableOnPathSuffix.Exists() {
		if !enableOnPathSuffix.IsArray() {
			return errors.New("enableOnPathSuffix must be an array")
		}
		for _, item := range enableOnPathSuffix.Array() {
			config.enableOnPathSuffix = append(config.enableOnPathSuffix, item.String())
		}
	} else {
		config.enableOnPathSuffix = []string{
			"/completions",
			"/embeddings",
			"/images/generations",
			"/audio/speech",
			"/fine_tuning/jobs",
			"/moderations",
			"/image-synthesis",
			"/video-synthesis",
			"/rerank",
			"/messages",
			"/responses",
		}
	}

	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config Config) types.Action {
	resetResponseRewriteContext(ctx)

	// Check path suffix
	path, err := proxywasm.GetHttpRequestHeader(":path")
	if err != nil {
		return types.ActionContinue
	}

	// Strip query parameters
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	matched := false
	for _, suffix := range config.enableOnPathSuffix {
		if strings.HasSuffix(path, suffix) {
			matched = true
			break
		}
	}

	if !matched || !ctx.HasRequestBody() {
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	contentType, _ := proxywasm.GetHttpRequestHeader("content-type")
	if !strings.Contains(strings.ToLower(contentType), "application/json") {
		return types.ActionContinue
	}

	// Prepare for body processing
	proxywasm.RemoveHttpRequestHeader("content-length")
	// 100MB buffer limit
	ctx.SetRequestBodyBufferLimit(DefaultMaxBodyBytes)

	return types.HeaderStopIteration
}

func onHttpRequestBody(ctx wrapper.HttpContext, config Config, body []byte) types.Action {
	if len(body) == 0 {
		return types.ActionContinue
	}

	if !json.Valid(body) {
		log.Error("invalid json body")
		return types.ActionContinue
	}

	oldModel := gjson.GetBytes(body, config.modelKey).String()

	newModel := config.defaultModel
	if newModel == "" {
		newModel = oldModel
	}

	// Exact match
	if target, ok := config.exactModelMapping[oldModel]; ok {
		newModel = target
	} else {
		// Prefix match
		for _, mapping := range config.prefixModelMapping {
			if strings.HasPrefix(oldModel, mapping.Prefix) {
				newModel = mapping.Target
				break
			}
		}
	}

	if newModel != "" && newModel != oldModel {
		if config.enableResponseMapping && oldModel != "" {
			ctx.SetContext(contextResponseRewriteEnabled, true)
			ctx.SetContext(contextResponseClientModel, oldModel)
			ctx.SetContext(contextResponseUpstreamModel, newModel)
		}
		newBody, err := sjson.SetBytes(body, config.modelKey, newModel)
		if err != nil {
			log.Errorf("failed to update model: %v", err)
			return types.ActionContinue
		}
		proxywasm.ReplaceHttpRequestBody(newBody)
		log.Debugf("model mapped, before: %s, after: %s", oldModel, newModel)
	}

	return types.ActionContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config Config) types.Action {
	if !ctx.GetBoolContext(contextResponseRewriteEnabled, false) || !ctx.HasResponseBody() {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
	contentType = strings.ToLower(contentType)

	if strings.Contains(contentType, "application/json") {
		ctx.SetContext(contextResponseRewriteMode, responseRewriteModeJSON)
		ctx.BufferResponseBody()
		ctx.SetResponseBodyBufferLimit(DefaultMaxBodyBytes)
		proxywasm.RemoveHttpResponseHeader("content-length")
		return types.ActionContinue
	}

	if strings.Contains(contentType, "text/event-stream") {
		ctx.SetContext(contextResponseRewriteMode, responseRewriteModeSSE)
		proxywasm.RemoveHttpResponseHeader("content-length")
		return types.ActionContinue
	}

	ctx.DontReadResponseBody()
	ctx.SetContext(contextResponseRewriteEnabled, false)
	return types.ActionContinue
}

func onHttpStreamingResponseBody(ctx wrapper.HttpContext, config Config, chunk []byte, isLastChunk bool) []byte {
	if !ctx.GetBoolContext(contextResponseRewriteEnabled, false) {
		return chunk
	}
	if ctx.GetStringContext(contextResponseRewriteMode, responseRewriteModeNone) != responseRewriteModeSSE {
		return chunk
	}

	clientModel := ctx.GetStringContext(contextResponseClientModel, "")
	upstreamModel := ctx.GetStringContext(contextResponseUpstreamModel, "")
	if clientModel == "" || upstreamModel == "" {
		return chunk
	}

	pendingData := ctx.GetStringContext(contextResponsePendingData, "")
	pendingData += string(chunk)

	var output strings.Builder
	for {
		eventPos, sepSize := findSseEventSeparator(pendingData)
		if eventPos == -1 {
			break
		}
		rawEvent := pendingData[:eventPos]
		output.WriteString(rewriteSseEvent(rawEvent, config.responseModelPaths, upstreamModel, clientModel))
		output.WriteString(pendingData[eventPos : eventPos+sepSize])
		pendingData = pendingData[eventPos+sepSize:]
	}

	if isLastChunk && pendingData != "" {
		output.WriteString(rewriteSseEvent(pendingData, config.responseModelPaths, upstreamModel, clientModel))
		pendingData = ""
	}
	ctx.SetContext(contextResponsePendingData, pendingData)

	return []byte(output.String())
}

func onHttpResponseBody(ctx wrapper.HttpContext, config Config, body []byte) types.Action {
	if !ctx.GetBoolContext(contextResponseRewriteEnabled, false) {
		return types.ActionContinue
	}
	if ctx.GetStringContext(contextResponseRewriteMode, responseRewriteModeNone) != responseRewriteModeJSON {
		return types.ActionContinue
	}

	clientModel := ctx.GetStringContext(contextResponseClientModel, "")
	upstreamModel := ctx.GetStringContext(contextResponseUpstreamModel, "")
	if clientModel == "" || upstreamModel == "" || len(body) == 0 {
		return types.ActionContinue
	}

	newBody, rewritten, err := rewriteModelFieldInJSONBytes(body, config.responseModelPaths, upstreamModel, clientModel)
	if err != nil {
		log.Errorf("failed to rewrite response model: %v", err)
		return types.ActionContinue
	}
	if rewritten {
		proxywasm.ReplaceHttpResponseBody(newBody)
	}
	return types.ActionContinue
}

func resetResponseRewriteContext(ctx wrapper.HttpContext) {
	ctx.SetContext(contextResponseRewriteEnabled, false)
	ctx.SetContext(contextResponseRewriteMode, responseRewriteModeNone)
	ctx.SetContext(contextResponseModelKey, "")
	ctx.SetContext(contextResponseClientModel, "")
	ctx.SetContext(contextResponseUpstreamModel, "")
	ctx.SetContext(contextResponsePendingData, "")
}

func rewriteModelFieldInJSONBytes(payload []byte, paths []string, upstreamModel, clientModel string) ([]byte, bool, error) {
	if !json.Valid(payload) {
		return payload, false, nil
	}

	for _, path := range paths {
		if gjson.GetBytes(payload, path).String() == upstreamModel {
			newPayload, err := sjson.SetBytes(payload, path, clientModel)
			if err != nil {
				return payload, false, err
			}
			return newPayload, true, nil
		}
	}
	return payload, false, nil
}

func findSseEventSeparator(data string) (eventPos int, separatorSize int) {
	lfPos := strings.Index(data, "\n\n")
	crlfPos := strings.Index(data, "\r\n\r\n")
	if lfPos == -1 {
		if crlfPos == -1 {
			return -1, 0
		}
		return crlfPos, 4
	}
	if crlfPos == -1 || lfPos < crlfPos {
		return lfPos, 2
	}
	return crlfPos, 4
}

func rewriteSseEvent(rawEvent string, paths []string, upstreamModel, clientModel string) string {
	var result strings.Builder
	lineStart := 0

	for lineStart <= len(rawEvent) {
		lineEnd := strings.Index(rawEvent[lineStart:], "\n")
		hasNewline := lineEnd != -1
		var line string
		if hasNewline {
			lineEnd += lineStart
			line = rawEvent[lineStart:lineEnd]
			lineStart = lineEnd + 1
		} else {
			line = rawEvent[lineStart:]
			lineStart = len(rawEvent) + 1
		}

		lineNoCR := strings.TrimSuffix(line, "\r")
		if !strings.HasPrefix(lineNoCR, "data:") {
			result.WriteString(line)
			if hasNewline {
				result.WriteString("\n")
			}
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(lineNoCR, "data:"))
		if payload == "[DONE]" {
			result.WriteString(line)
			if hasNewline {
				result.WriteString("\n")
			}
			continue
		}

		rewrittenPayload, rewritten, err := rewriteModelFieldInJSONBytes([]byte(payload), paths, upstreamModel, clientModel)
		if err != nil || !rewritten {
			result.WriteString(line)
			if hasNewline {
				result.WriteString("\n")
			}
			continue
		}

		result.WriteString("data: ")
		result.Write(rewrittenPayload)
		if hasNewline {
			result.WriteString("\n")
		}
	}

	return result.String()
}
