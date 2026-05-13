package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/tokenusage"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	// Envoy log levels
	LogLevelTrace = iota
	LogLevelDebug
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelCritical
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-statistics",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBody(onHttpStreamingBody),
		wrapper.ProcessResponseBody(onHttpResponseBody),
		wrapper.WithRebuildAfterRequests[AIStatisticsConfig](1000),
		wrapper.WithRebuildMaxMemBytes[AIStatisticsConfig](200*1024*1024),
	)
}

const (
	defaultMaxBodyBytes uint32 = 100 * 1024 * 1024
	// Context consts
	StatisticsRequestStartTime = "ai-statistics-request-start-time"
	StatisticsFirstTokenTime   = "ai-statistics-first-token-time"
	CtxGeneralAtrribute        = "attributes"
	CtxLogAtrribute            = "logAttributes"
	CtxStreamingBodyBuffer     = "streamingBodyBuffer"
	RouteName                  = "route"
	ClusterName                = "cluster"
	APIName                    = "api"
	ConsumerKey                = "x-mse-consumer"
	RequestPath                = "request_path"
	SkipProcessing             = "skip_processing"

	// Session ID related
	SessionID = "session_id"

	// AI API Paths
	PathOpenAIChatCompletions       = "/v1/chat/completions"
	PathOpenAICompletions           = "/v1/completions"
	PathOpenAIEmbeddings            = "/v1/embeddings"
	PathOpenAIModels                = "/v1/models"
	PathGeminiGenerateContent       = "/generateContent"
	PathGeminiStreamGenerateContent = "/streamGenerateContent"

	// Source Type
	FixedValue            = "fixed_value"
	RequestHeader         = "request_header"
	RequestBody           = "request_body"
	ResponseHeader        = "response_header"
	ResponseStreamingBody = "response_streaming_body"
	ResponseBody          = "response_body"

	// Inner metric & log attributes
	LLMFirstTokenDuration  = "llm_first_token_duration"
	LLMServiceDuration     = "llm_service_duration"
	LLMDurationCount       = "llm_duration_count"
	LLMStreamDurationCount = "llm_stream_duration_count"
	ResponseType           = "response_type"
	ChatID                 = "chat_id"
	ChatRound              = "chat_round"

	// Inner span attributes
	ArmsSpanKind     = "gen_ai.span.kind"
	ArmsModelName    = "gen_ai.model_name"
	ArmsRequestModel = "gen_ai.request.model"
	ArmsInputToken   = "gen_ai.usage.input_tokens"
	ArmsOutputToken  = "gen_ai.usage.output_tokens"
	ArmsTotalToken   = "gen_ai.usage.total_tokens"

	// Extract Rule
	RuleFirst   = "first"
	RuleReplace = "replace"
	RuleAppend  = "append"

	// Built-in attributes
	BuiltinQuestionKey        = "question"
	BuiltinAnswerKey          = "answer"
	BuiltinToolCallsKey       = "tool_calls"
	BuiltinReasoningKey       = "reasoning"
	BuiltinSystemKey          = "system"
	BuiltinReasoningTokens    = "reasoning_tokens"
	BuiltinCachedTokens       = "cached_tokens"
	BuiltinInputTokenDetails  = "input_token_details"
	BuiltinOutputTokenDetails = "output_token_details"

	// Built-in attribute paths
	// Question paths (from request body)
	QuestionPathOpenAI = "messages.@reverse.0.content"
	QuestionPathClaude = "messages.@reverse.0.content" // Claude uses same format

	// System prompt paths (from request body)
	SystemPathClaude = "system" // Claude /v1/messages has system as a top-level field

	// Answer paths (from response body - non-streaming)
	AnswerPathOpenAINonStreaming = "choices.0.message.content"
	AnswerPathClaudeNonStreaming = "content.0.text"

	// Answer paths (from response streaming body)
	AnswerPathOpenAIStreaming = "choices.0.delta.content"
	AnswerPathClaudeStreaming = "delta.text"

	// Tool calls paths (OpenAI format)
	ToolCallsPathNonStreaming = "choices.0.message.tool_calls"
	ToolCallsPathStreaming    = "choices.0.delta.tool_calls"

	// Claude/Anthropic tool calls paths (streaming)
	ClaudeEventType         = "type"
	ClaudeContentBlockType  = "content_block.type"
	ClaudeContentBlockID    = "content_block.id"
	ClaudeContentBlockName  = "content_block.name"
	ClaudeContentBlockInput = "content_block.input"
	ClaudeDeltaPartialJSON  = "delta.partial_json"
	ClaudeIndex             = "index"

	// Reasoning paths
	ReasoningPathNonStreaming = "choices.0.message.reasoning_content"
	ReasoningPathStreaming    = "choices.0.delta.reasoning_content"

	// Context key for streaming tool calls buffer
	CtxStreamingToolCallsBuffer = "streamingToolCallsBuffer"
)

// getDefaultAttributes returns the default attributes configuration for empty config
// This includes all attributes but may consume significant memory for large conversations
func getDefaultAttributes() []Attribute {
	return []Attribute{
		// Extract complete conversation history from request body
		{
			Key:         "messages",
			ValueSource: RequestBody,
			Value:       "messages",
			ApplyToLog:  true,
		},
		// Built-in attributes (no value_source needed, will be auto-extracted)
		{
			Key:        BuiltinQuestionKey,
			ApplyToLog: true,
		},
		{
			Key:        BuiltinSystemKey,
			ApplyToLog: true,
		},
		{
			Key:        BuiltinAnswerKey,
			ApplyToLog: true,
			Rule:       RuleAppend, // Streaming responses need to append content from all chunks
		},
		{
			Key:        BuiltinReasoningKey,
			ApplyToLog: true,
			Rule:       RuleAppend, // Streaming responses need to append content from all chunks
		},
		{
			Key:        BuiltinToolCallsKey,
			ApplyToLog: true,
		},
		// Token statistics (auto-extracted from response)
		{
			Key:        BuiltinReasoningTokens,
			ApplyToLog: true,
		},
		{
			Key:        BuiltinCachedTokens,
			ApplyToLog: true,
		},
		// Detailed token information
		{
			Key:        BuiltinInputTokenDetails,
			ApplyToLog: true,
		},
		{
			Key:        BuiltinOutputTokenDetails,
			ApplyToLog: true,
		},
	}
}

// getDefaultResponseAttributes returns a lightweight default attributes configuration
// for production environments with high concurrency and high latency.
// - Buffers request body for model extraction (small, essential field)
// - Does NOT extract large fields like question, system, messages
// - Does NOT buffer streaming response body (no answer, reasoning, tool_calls)
// - Only extracts token statistics from response context
func getDefaultResponseAttributes() []Attribute {
	return []Attribute{
		// Token statistics (extracted from context, no body buffering needed)
		{
			Key:        BuiltinReasoningTokens,
			ApplyToLog: true,
		},
		{
			Key:        BuiltinCachedTokens,
			ApplyToLog: true,
		},
		{
			Key:        BuiltinInputTokenDetails,
			ApplyToLog: true,
		},
		{
			Key:        BuiltinOutputTokenDetails,
			ApplyToLog: true,
		},
	}
}

// Default session ID headers in priority order
var defaultSessionHeaders = []string{
	"x-openclaw-session-key",
	"x-clawdbot-session-key",
	"x-moltbot-session-key",
	"x-agent-session",
}

// extractSessionId extracts session ID from request headers
// If customHeader is configured, it takes priority; otherwise falls back to default headers
func extractSessionId(customHeader string) string {
	// If custom header is configured, try it first
	if customHeader != "" {
		if sessionId, _ := proxywasm.GetHttpRequestHeader(customHeader); sessionId != "" {
			return sessionId
		}
	}
	// Fall back to default session headers in priority order
	for _, header := range defaultSessionHeaders {
		if sessionId, _ := proxywasm.GetHttpRequestHeader(header); sessionId != "" {
			return sessionId
		}
	}
	return ""
}

// ToolCall represents a single tool call in the response
type ToolCall struct {
	Index    int              `json:"index,omitempty"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function ToolCallFunction `json:"function,omitempty"`
}

// ToolCallFunction represents the function details in a tool call
type ToolCallFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// StreamingToolCallsBuffer holds the state for assembling streaming tool calls
type StreamingToolCallsBuffer struct {
	ToolCalls       map[int]*ToolCall // keyed by index (OpenAI format)
	InToolBlock     map[int]bool      // tracks which indices are in tool_use blocks (Claude format)
	ArgumentsBuffer map[int]string    // buffers partial JSON arguments (Claude format)
}

// extractStreamingToolCalls extracts and assembles tool calls from streaming response chunks (OpenAI format)
func extractStreamingToolCalls(data []byte, buffer *StreamingToolCallsBuffer) *StreamingToolCallsBuffer {
	if buffer == nil {
		buffer = &StreamingToolCallsBuffer{
			ToolCalls:       make(map[int]*ToolCall),
			InToolBlock:     make(map[int]bool),
			ArgumentsBuffer: make(map[int]string),
		}
	}

	chunks := bytes.Split(bytes.TrimSpace(wrapper.UnifySSEChunk(data)), []byte("\n\n"))
	for _, chunk := range chunks {
		toolCallsResult := gjson.GetBytes(chunk, ToolCallsPathStreaming)
		if !toolCallsResult.Exists() || !toolCallsResult.IsArray() {
			continue
		}

		for _, tcResult := range toolCallsResult.Array() {
			index := int(tcResult.Get("index").Int())

			// Get or create tool call entry
			tc, exists := buffer.ToolCalls[index]
			if !exists {
				tc = &ToolCall{Index: index}
				buffer.ToolCalls[index] = tc
			}

			// Update fields if present
			if id := tcResult.Get("id").String(); id != "" {
				tc.ID = id
			}
			if tcType := tcResult.Get("type").String(); tcType != "" {
				tc.Type = tcType
			}
			if funcName := tcResult.Get("function.name").String(); funcName != "" {
				tc.Function.Name = funcName
			}
			// Append arguments (they come in chunks)
			if args := tcResult.Get("function.arguments").String(); args != "" {
				tc.Function.Arguments += args
			}
		}
	}

	return buffer
}

// extractClaudeStreamingToolCalls extracts and assembles tool calls from Claude/Anthropic streaming response chunks
// Claude format uses events: content_block_start, content_block_delta, content_block_stop
func extractClaudeStreamingToolCalls(data []byte, buffer *StreamingToolCallsBuffer) *StreamingToolCallsBuffer {
	if buffer == nil {
		buffer = &StreamingToolCallsBuffer{
			ToolCalls:       make(map[int]*ToolCall),
			InToolBlock:     make(map[int]bool),
			ArgumentsBuffer: make(map[int]string),
		}
	}

	chunks := bytes.Split(bytes.TrimSpace(wrapper.UnifySSEChunk(data)), []byte("\n\n"))
	for _, chunk := range chunks {
		// Get event type
		eventType := gjson.GetBytes(chunk, ClaudeEventType)
		if !eventType.Exists() {
			continue
		}

		switch eventType.String() {
		case "content_block_start":
			// Check if this is a tool_use block
			contentBlockType := gjson.GetBytes(chunk, ClaudeContentBlockType)
			if contentBlockType.Exists() && contentBlockType.String() == "tool_use" {
				index := int(gjson.GetBytes(chunk, ClaudeIndex).Int())

				// Create tool call entry
				tc := &ToolCall{Index: index}

				// Extract id and name
				if id := gjson.GetBytes(chunk, ClaudeContentBlockID).String(); id != "" {
					tc.ID = id
				}
				if name := gjson.GetBytes(chunk, ClaudeContentBlockName).String(); name != "" {
					tc.Function.Name = name
				}
				tc.Type = "tool_use"

				buffer.ToolCalls[index] = tc
				buffer.InToolBlock[index] = true
				buffer.ArgumentsBuffer[index] = ""

				// Try to extract initial input if present
				if input := gjson.GetBytes(chunk, ClaudeContentBlockInput); input.Exists() {
					if inputMap, ok := input.Value().(map[string]interface{}); ok {
						if jsonBytes, err := json.Marshal(inputMap); err == nil {
							buffer.ArgumentsBuffer[index] = string(jsonBytes)
						}
					}
				}
			}

		case "content_block_delta":
			// Check if we're in a tool block
			index := int(gjson.GetBytes(chunk, ClaudeIndex).Int())
			if buffer.InToolBlock[index] {
				// Accumulate partial JSON arguments
				partialJSON := gjson.GetBytes(chunk, ClaudeDeltaPartialJSON)
				if partialJSON.Exists() {
					buffer.ArgumentsBuffer[index] += partialJSON.String()
				}
			}

		case "content_block_stop":
			// Finalize the tool call if we were in a tool block
			index := int(gjson.GetBytes(chunk, ClaudeIndex).Int())
			if buffer.InToolBlock[index] {
				buffer.InToolBlock[index] = false

				// Parse accumulated arguments and set them
				if tc, exists := buffer.ToolCalls[index]; exists {
					tc.Function.Arguments = buffer.ArgumentsBuffer[index]
				}
			}
		}
	}

	return buffer
}

// getToolCallsFromBuffer converts the buffer to a sorted slice of tool calls
func getToolCallsFromBuffer(buffer *StreamingToolCallsBuffer) []ToolCall {
	if buffer == nil || len(buffer.ToolCalls) == 0 {
		return nil
	}

	// Find max index to create properly sized slice
	maxIndex := 0
	for idx := range buffer.ToolCalls {
		if idx > maxIndex {
			maxIndex = idx
		}
	}

	result := make([]ToolCall, 0, len(buffer.ToolCalls))
	for i := 0; i <= maxIndex; i++ {
		if tc, exists := buffer.ToolCalls[i]; exists {
			result = append(result, *tc)
		}
	}
	return result
}

// TracingSpan is the tracing span configuration.
type Attribute struct {
	Key                string `json:"key"`
	ValueSource        string `json:"value_source"`
	Value              string `json:"value"`
	TraceSpanKey       string `json:"trace_span_key,omitempty"`
	DefaultValue       string `json:"default_value,omitempty"`
	Rule               string `json:"rule,omitempty"`
	ApplyToLog         bool   `json:"apply_to_log,omitempty"`
	ApplyToSpan        bool   `json:"apply_to_span,omitempty"`
	AsSeparateLogField bool   `json:"as_separate_log_field,omitempty"`
}

type AIStatisticsConfig struct {
	// Metrics
	// TODO: add more metrics in Gauge and Histogram format
	counterMetrics map[string]proxywasm.MetricCounter
	// Attributes to be recorded in log & span
	attributes []Attribute
	// If there exist attributes extracted from streaming body, chunks should be buffered
	shouldBufferStreamingBody bool
	// If there exist attributes extracted from request body, request body should be buffered
	shouldBufferRequestBody bool
	// If disableOpenaiUsage is true, model/input_token/output_token logs will be skipped
	disableOpenaiUsage bool
	valueLengthLimit   int
	// Path suffixes to enable the plugin on
	enablePathSuffixes []string
	// Content types to enable response body buffering
	enableContentTypes []string
	// Session ID header name (if configured, takes priority over default headers)
	sessionIdHeader string
}

func generateMetricName(route, cluster, model, consumer, metricName string) string {
	return fmt.Sprintf("route.%s.upstream.%s.model.%s.consumer.%s.metric.%s", route, cluster, model, consumer, metricName)
}

func getRouteName() (string, error) {
	if raw, err := proxywasm.GetProperty([]string{"route_name"}); err != nil {
		return "-", err
	} else {
		return string(raw), nil
	}
}

func getAPIName() (string, error) {
	if raw, err := proxywasm.GetProperty([]string{"route_name"}); err != nil {
		return "-", err
	} else {
		parts := strings.Split(string(raw), "@")
		if len(parts) < 3 {
			return "-", errors.New("not api type")
		} else {
			return strings.Join(parts[:3], "@"), nil
		}
	}
}

func getClusterName() (string, error) {
	if raw, err := proxywasm.GetProperty([]string{"cluster_name"}); err != nil {
		return "-", err
	} else {
		return string(raw), nil
	}
}

func (config *AIStatisticsConfig) incrementCounter(metricName string, inc uint64) {
	if inc == 0 {
		return
	}
	counter, ok := config.counterMetrics[metricName]
	if !ok {
		counter = proxywasm.DefineCounterMetric(metricName)
		config.counterMetrics[metricName] = counter
	}
	counter.Increment(inc)
}

// isPathEnabled checks if the request path matches any of the enabled path suffixes
func isPathEnabled(requestPath string, enabledSuffixes []string) bool {
	if len(enabledSuffixes) == 0 {
		return true // If no path suffixes configured, enable for all
	}

	// Remove query parameters from path
	pathWithoutQuery := requestPath
	if queryPos := strings.Index(requestPath, "?"); queryPos != -1 {
		pathWithoutQuery = requestPath[:queryPos]
	}

	// Check if path ends with any enabled suffix
	for _, suffix := range enabledSuffixes {
		if strings.HasSuffix(pathWithoutQuery, suffix) {
			return true
		}
	}
	return false
}

// isContentTypeEnabled checks if the content type matches any of the enabled content types
func isContentTypeEnabled(contentType string, enabledContentTypes []string) bool {
	if len(enabledContentTypes) == 0 {
		return true // If no content types configured, enable for all
	}

	for _, enabledType := range enabledContentTypes {
		if strings.Contains(contentType, enabledType) {
			return true
		}
	}
	return false
}

func parseConfig(configJson gjson.Result, config *AIStatisticsConfig) error {
	// Check if use_default_attributes is enabled
	useDefaultAttributes := configJson.Get("use_default_attributes").Bool()
	// Check if use_default_response_attributes is enabled (lightweight mode)
	useDefaultResponseAttributes := configJson.Get("use_default_response_attributes").Bool()

	// Parse tracing span attributes setting.
	attributeConfigs := configJson.Get("attributes").Array()

	// Set value_length_limit
	if configJson.Get("value_length_limit").Exists() {
		config.valueLengthLimit = int(configJson.Get("value_length_limit").Int())
	} else {
		config.valueLengthLimit = 32000
	}

	// Parse attributes or use defaults
	if useDefaultAttributes {
		config.attributes = getDefaultAttributes()
		// Update value_length_limit to default when using default attributes
		if !configJson.Get("value_length_limit").Exists() {
			config.valueLengthLimit = 10485760 // 10MB
		}
		log.Infof("Using default attributes configuration")
	} else if useDefaultResponseAttributes {
		config.attributes = getDefaultResponseAttributes()
		// Use a reasonable default for lightweight mode
		if !configJson.Get("value_length_limit").Exists() {
			config.valueLengthLimit = 4000
		}
		log.Infof("Using default response attributes configuration (lightweight mode)")
	} else {
		config.attributes = make([]Attribute, len(attributeConfigs))
		for i, attributeConfig := range attributeConfigs {
			attribute := Attribute{}
			err := json.Unmarshal([]byte(attributeConfig.Raw), &attribute)
			if err != nil {
				log.Errorf("parse config failed, %v", err)
				return err
			}
			if attribute.Rule != "" && attribute.Rule != RuleFirst && attribute.Rule != RuleReplace && attribute.Rule != RuleAppend {
				return errors.New("value of rule must be one of [nil, first, replace, append]")
			}
			config.attributes[i] = attribute
		}
	}

	// Check if any attribute needs request body or streaming body buffering
	for _, attribute := range config.attributes {
		// Check for request body buffering
		if attribute.ValueSource == RequestBody {
			config.shouldBufferRequestBody = true
		}
		// Check for streaming body buffering (explicitly configured)
		if attribute.ValueSource == ResponseStreamingBody {
			config.shouldBufferStreamingBody = true
		}
		// For built-in attributes without explicit ValueSource, check default sources
		if attribute.ValueSource == "" && isBuiltinAttribute(attribute.Key) {
			defaultSources := getBuiltinAttributeDefaultSources(attribute.Key)
			for _, src := range defaultSources {
				if src == RequestBody {
					config.shouldBufferRequestBody = true
				}
				// Only answer/reasoning/tool_calls need actual body buffering
				// Token-related attributes are extracted from context, not from body
				if src == ResponseStreamingBody && needsBodyBuffering(attribute.Key) {
					config.shouldBufferStreamingBody = true
				}
			}
		}
	}
	// Metric settings
	config.counterMetrics = make(map[string]proxywasm.MetricCounter)

	// Parse openai usage config setting.
	config.disableOpenaiUsage = configJson.Get("disable_openai_usage").Bool()

	// Parse path suffix configuration
	pathSuffixes := configJson.Get("enable_path_suffixes").Array()
	config.enablePathSuffixes = make([]string, 0, len(pathSuffixes))

	// If use_default_attributes or use_default_response_attributes is enabled and enable_path_suffixes is not configured, use default path suffixes
	if (useDefaultAttributes || useDefaultResponseAttributes) && !configJson.Get("enable_path_suffixes").Exists() {
		config.enablePathSuffixes = []string{"/completions", "/messages"}
		log.Infof("Using default path suffixes: /completions, /messages")
	} else {
		// Process manually configured path suffixes
		for _, suffix := range pathSuffixes {
			suffixStr := suffix.String()
			if suffixStr == "*" {
				// Clear the suffixes list since * means all paths are enabled
				config.enablePathSuffixes = make([]string, 0)
				break
			}
			config.enablePathSuffixes = append(config.enablePathSuffixes, suffixStr)
		}
	}

	// Parse content type configuration
	contentTypes := configJson.Get("enable_content_types").Array()
	config.enableContentTypes = make([]string, 0, len(contentTypes))

	for _, contentType := range contentTypes {
		contentTypeStr := contentType.String()
		if contentTypeStr == "*" {
			// Clear the content types list since * means all content types are enabled
			config.enableContentTypes = make([]string, 0)
			break
		}
		config.enableContentTypes = append(config.enableContentTypes, contentTypeStr)
	}

	// Parse session ID header configuration
	if sessionIdHeader := configJson.Get("session_id_header"); sessionIdHeader.Exists() {
		config.sessionIdHeader = sessionIdHeader.String()
	}

	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config AIStatisticsConfig) types.Action {
	// Check if request path matches enabled suffixes
	requestPath, _ := proxywasm.GetHttpRequestHeader(":path")
	if !isPathEnabled(requestPath, config.enablePathSuffixes) {
		log.Debugf("ai-statistics: skipping request for path %s (not in enabled suffixes)", requestPath)
		// Set skip processing flag and avoid reading request/response body
		ctx.SetContext(SkipProcessing, true)
		ctx.DontReadRequestBody()
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	ctx.DisableReroute()
	route, _ := getRouteName()
	cluster, _ := getClusterName()
	api, apiError := getAPIName()
	if apiError == nil {
		route = api
	}
	ctx.SetContext(RouteName, route)
	ctx.SetContext(ClusterName, cluster)
	ctx.SetUserAttribute(APIName, api)
	ctx.SetContext(StatisticsRequestStartTime, time.Now().UnixMilli())
	if requestPath, _ := proxywasm.GetHttpRequestHeader(":path"); requestPath != "" {
		ctx.SetContext(RequestPath, requestPath)
	}
	if consumer, _ := proxywasm.GetHttpRequestHeader(ConsumerKey); consumer != "" {
		ctx.SetContext(ConsumerKey, consumer)
	}

	// Always buffer request body to extract model field
	// This is essential for metrics and logging
	ctx.SetRequestBodyBufferLimit(defaultMaxBodyBytes)

	// Extract session ID from headers
	sessionId := extractSessionId(config.sessionIdHeader)
	if sessionId != "" {
		ctx.SetUserAttribute(SessionID, sessionId)
	}

	// Set span attributes for ARMS.
	setSpanAttribute(ArmsSpanKind, "LLM")
	// Set user defined log & span attributes which type is fixed_value
	setAttributeBySource(ctx, config, FixedValue, nil)
	// Set user defined log & span attributes which type is request_header
	setAttributeBySource(ctx, config, RequestHeader, nil)

	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config AIStatisticsConfig, body []byte) types.Action {
	// Check if processing should be skipped
	if ctx.GetBoolContext(SkipProcessing, false) {
		return types.ActionContinue
	}

	// Only process request body if we need to extract attributes from it
	if config.shouldBufferRequestBody && len(body) > 0 {
		// Set user defined log & span attributes.
		setAttributeBySource(ctx, config, RequestBody, body)
	}

	// Extract model from request body if available, otherwise try path
	requestModel := "UNKNOWN"
	if len(body) > 0 {
		if model := gjson.GetBytes(body, "model"); model.Exists() {
			requestModel = model.String()
		}
	}
	// If model not found in body, try to extract from path (Gemini style)
	if requestModel == "UNKNOWN" {
		requestPath := ctx.GetStringContext(RequestPath, "")
		if strings.Contains(requestPath, "generateContent") || strings.Contains(requestPath, "streamGenerateContent") { // Google Gemini GenerateContent
			reg := regexp.MustCompile(`^.*/(?P<api_version>[^/]+)/models/(?P<model>[^:]+):\w+Content$`)
			matches := reg.FindStringSubmatch(requestPath)
			if len(matches) == 3 {
				requestModel = matches[2]
			}
		}
	}
	ctx.SetContext(tokenusage.CtxKeyRequestModel, requestModel)
	setSpanAttribute(ArmsRequestModel, requestModel)

	// Set the number of conversation rounds (only if body is available)
	userPromptCount := 0
	if len(body) > 0 {
		if messages := gjson.GetBytes(body, "messages"); messages.Exists() && messages.IsArray() {
			// OpenAI and Claude/Anthropic format - both use "messages" array with "role" field
			for _, msg := range messages.Array() {
				if msg.Get("role").String() == "user" {
					userPromptCount += 1
				}
			}
		} else if contents := gjson.GetBytes(body, "contents"); contents.Exists() && contents.IsArray() {
			// Google Gemini GenerateContent
			for _, content := range contents.Array() {
				if !content.Get("role").Exists() || content.Get("role").String() == "user" {
					userPromptCount += 1
				}
			}
		}
	}
	ctx.SetUserAttribute(ChatRound, userPromptCount)

	// Write log
	debugLogAiLog(ctx)
	_ = ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
	return types.ActionContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config AIStatisticsConfig) types.Action {
	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")

	if !isContentTypeEnabled(contentType, config.enableContentTypes) {
		log.Debugf("ai-statistics: skipping response for content type %s (not in enabled content types)", contentType)
		// Set skip processing flag and avoid reading response body
		ctx.SetContext(SkipProcessing, true)
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	if !strings.Contains(contentType, "text/event-stream") {
		ctx.BufferResponseBody()
	}

	// Set user defined log & span attributes.
	setAttributeBySource(ctx, config, ResponseHeader, nil)

	return types.ActionContinue
}

func onHttpStreamingBody(ctx wrapper.HttpContext, config AIStatisticsConfig, data []byte, endOfStream bool) []byte {
	// Check if processing should be skipped
	if ctx.GetBoolContext(SkipProcessing, false) {
		return data
	}

	// Buffer stream body for record log & span attributes
	if config.shouldBufferStreamingBody {
		streamingBodyBuffer, ok := ctx.GetContext(CtxStreamingBodyBuffer).([]byte)
		if !ok {
			streamingBodyBuffer = data
		} else {
			streamingBodyBuffer = append(streamingBodyBuffer, data...)
		}
		ctx.SetContext(CtxStreamingBodyBuffer, streamingBodyBuffer)
	}

	ctx.SetUserAttribute(ResponseType, "stream")
	if chatID := wrapper.GetValueFromBody(data, []string{
		"id",
		"response.id",
		"responseId", // Gemini generateContent
		"message.id", // anthropic/claude messages
	}); chatID != nil {
		ctx.SetUserAttribute(ChatID, chatID.String())
	}

	// Get requestStartTime from http context
	requestStartTime, ok := ctx.GetContext(StatisticsRequestStartTime).(int64)
	if !ok {
		log.Error("failed to get requestStartTime from http context")
		return data
	}

	// If this is the first chunk, record first token duration metric and span attribute
	if ctx.GetContext(StatisticsFirstTokenTime) == nil {
		firstTokenTime := time.Now().UnixMilli()
		ctx.SetContext(StatisticsFirstTokenTime, firstTokenTime)
		ctx.SetUserAttribute(LLMFirstTokenDuration, firstTokenTime-requestStartTime)
	}

	// Set information about this request
	if !config.disableOpenaiUsage {
		if usage := tokenusage.GetTokenUsage(ctx, data); usage.TotalToken > 0 {
			// Set span attributes for ARMS.
			setSpanAttribute(ArmsTotalToken, usage.TotalToken)
			setSpanAttribute(ArmsModelName, usage.Model)
			setSpanAttribute(ArmsInputToken, usage.InputToken)
			setSpanAttribute(ArmsOutputToken, usage.OutputToken)

			// Set token details to context for later use in attributes
			if len(usage.InputTokenDetails) > 0 {
				ctx.SetContext(tokenusage.CtxKeyInputTokenDetails, usage.InputTokenDetails)
			}
			if len(usage.OutputTokenDetails) > 0 {
				ctx.SetContext(tokenusage.CtxKeyOutputTokenDetails, usage.OutputTokenDetails)
			}

			// Write once
			_ = ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
		}
	}
	// If the end of the stream is reached, record metrics/logs/spans.
	if endOfStream {
		responseEndTime := time.Now().UnixMilli()
		ctx.SetUserAttribute(LLMServiceDuration, responseEndTime-requestStartTime)

		// Set user defined log & span attributes from streaming body.
		// Always call setAttributeBySource even if shouldBufferStreamingBody is false,
		// because token-related attributes are extracted from context (not buffered body).
		var streamingBodyBuffer []byte
		if config.shouldBufferStreamingBody {
			streamingBodyBuffer, _ = ctx.GetContext(CtxStreamingBodyBuffer).([]byte)
		}
		setAttributeBySource(ctx, config, ResponseStreamingBody, streamingBodyBuffer)

		// Write log
		debugLogAiLog(ctx)
		_ = ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)

		// Write metrics
		writeMetric(ctx, config)
	}
	return data
}

func onHttpResponseBody(ctx wrapper.HttpContext, config AIStatisticsConfig, body []byte) types.Action {
	// Check if processing should be skipped
	if ctx.GetBoolContext(SkipProcessing, false) {
		return types.ActionContinue
	}

	// Get requestStartTime from http context
	requestStartTime, _ := ctx.GetContext(StatisticsRequestStartTime).(int64)

	responseEndTime := time.Now().UnixMilli()
	ctx.SetUserAttribute(LLMServiceDuration, responseEndTime-requestStartTime)

	ctx.SetUserAttribute(ResponseType, "normal")
	if chatID := wrapper.GetValueFromBody(body, []string{
		"id",
		"response.id",
		"responseId", // Gemini generateContent
		"message.id", // anthropic/claude messages
	}); chatID != nil {
		ctx.SetUserAttribute(ChatID, chatID.String())
	}

	// Set information about this request
	if !config.disableOpenaiUsage {
		if usage := tokenusage.GetTokenUsage(ctx, body); usage.TotalToken > 0 {
			// Set span attributes for ARMS.
			setSpanAttribute(ArmsModelName, usage.Model)
			setSpanAttribute(ArmsInputToken, usage.InputToken)
			setSpanAttribute(ArmsOutputToken, usage.OutputToken)
			setSpanAttribute(ArmsTotalToken, usage.TotalToken)

			// Set token details to context for later use in attributes
			if len(usage.InputTokenDetails) > 0 {
				ctx.SetContext(tokenusage.CtxKeyInputTokenDetails, usage.InputTokenDetails)
			}
			if len(usage.OutputTokenDetails) > 0 {
				ctx.SetContext(tokenusage.CtxKeyOutputTokenDetails, usage.OutputTokenDetails)
			}
		}
	}

	// Set user defined log & span attributes.
	setAttributeBySource(ctx, config, ResponseBody, body)

	// Write log
	debugLogAiLog(ctx)
	_ = ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)

	// Write metrics
	writeMetric(ctx, config)

	return types.ActionContinue
}

// fetches the tracing span value from the specified source.

func setAttributeBySource(ctx wrapper.HttpContext, config AIStatisticsConfig, source string, body []byte) {
	for _, attribute := range config.attributes {
		var key string
		var value interface{}
		key = attribute.Key

		// Check if this attribute should be processed for the current source
		// For built-in attributes without value_source configured, use default source matching
		if !shouldProcessBuiltinAttribute(key, attribute.ValueSource, source) {
			continue
		}

		// If value is configured, try to extract using the configured path
		if attribute.Value != "" {
			switch source {
			case FixedValue:
				value = attribute.Value
			case RequestHeader:
				value, _ = proxywasm.GetHttpRequestHeader(attribute.Value)
			case RequestBody:
				value = gjson.GetBytes(body, attribute.Value).Value()
			case ResponseHeader:
				value, _ = proxywasm.GetHttpResponseHeader(attribute.Value)
			case ResponseStreamingBody:
				value = extractStreamingBodyByJsonPath(body, attribute.Value, attribute.Rule)
			case ResponseBody:
				value = gjson.GetBytes(body, attribute.Value).Value()
			default:
			}
		}

		// Handle built-in attributes: use fallback if value is empty or not configured
		if (value == nil || value == "") && isBuiltinAttribute(key) {
			value = getBuiltinAttributeFallback(ctx, config, key, source, body, attribute.Rule)
			if value != nil && value != "" {
				log.Debugf("[attribute] Used built-in extraction for %s: %+v", key, value)
			}
		}

		if (value == nil || value == "") && attribute.DefaultValue != "" {
			value = attribute.DefaultValue
		}

		// Format value for logging/span
		var formattedValue interface{}
		switch v := value.(type) {
		case map[string]int64:
			// For token details maps, convert to JSON string
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				log.Warnf("failed to marshal token details: %v", err)
				formattedValue = fmt.Sprint(v)
			} else {
				formattedValue = string(jsonBytes)
			}
		default:
			formattedValue = value
			if len(fmt.Sprint(value)) > config.valueLengthLimit {
				formattedValue = fmt.Sprint(value)[:config.valueLengthLimit/2] + " [truncated] " + fmt.Sprint(value)[len(fmt.Sprint(value))-config.valueLengthLimit/2:]
			}
		}

		log.Debugf("[attribute] source type: %s, key: %s, value: %+v", source, key, formattedValue)
		if attribute.ApplyToLog {
			if attribute.AsSeparateLogField {
				var marshalledJsonStr string
				if _, ok := value.(map[string]int64); ok {
					// Already marshaled in formattedValue
					marshalledJsonStr = fmt.Sprint(formattedValue)
				} else {
					marshalledJsonStr = wrapper.MarshalStr(fmt.Sprint(formattedValue))
				}
				if err := proxywasm.SetProperty([]string{key}, []byte(marshalledJsonStr)); err != nil {
					log.Warnf("failed to set %s in filter state, raw is %s, err is %v", key, marshalledJsonStr, err)
				}
			} else {
				ctx.SetUserAttribute(key, formattedValue)
			}
		}
		// for metrics
		if key == tokenusage.CtxKeyModel || key == tokenusage.CtxKeyInputToken || key == tokenusage.CtxKeyOutputToken || key == tokenusage.CtxKeyTotalToken {
			ctx.SetContext(key, value)
		}
		if attribute.ApplyToSpan {
			if attribute.TraceSpanKey != "" {
				key = attribute.TraceSpanKey
			}
			setSpanAttribute(key, value)
		}
	}
}

// isBuiltinAttribute checks if the given key is a built-in attribute
func isBuiltinAttribute(key string) bool {
	return key == BuiltinQuestionKey || key == BuiltinAnswerKey || key == BuiltinToolCallsKey || key == BuiltinReasoningKey || key == BuiltinSystemKey ||
		key == BuiltinReasoningTokens || key == BuiltinCachedTokens ||
		key == BuiltinInputTokenDetails || key == BuiltinOutputTokenDetails
}

// needsBodyBuffering checks if a built-in attribute needs body buffering
// Token-related attributes are extracted from context (set by tokenusage.GetTokenUsage),
// so they don't require buffering the response body.
func needsBodyBuffering(key string) bool {
	return key == BuiltinAnswerKey || key == BuiltinToolCallsKey || key == BuiltinReasoningKey
}

// getBuiltinAttributeDefaultSources returns the default value_source(s) for a built-in attribute
// Returns nil if the key is not a built-in attribute
// Note: Token-related attributes are extracted from context (set by tokenusage.GetTokenUsage),
// so they don't require body buffering even though they're processed during response phase.
func getBuiltinAttributeDefaultSources(key string) []string {
	switch key {
	case BuiltinQuestionKey, BuiltinSystemKey:
		return []string{RequestBody}
	case BuiltinAnswerKey, BuiltinToolCallsKey, BuiltinReasoningKey:
		return []string{ResponseStreamingBody, ResponseBody}
	case BuiltinReasoningTokens, BuiltinCachedTokens, BuiltinInputTokenDetails, BuiltinOutputTokenDetails:
		// Token details are extracted from context (set by tokenusage.GetTokenUsage),
		// not from body parsing. We use ResponseStreamingBody/ResponseBody to indicate
		// they should be processed during response phase, but they don't require body buffering.
		return []string{ResponseStreamingBody, ResponseBody}
	default:
		return nil
	}
}

// shouldProcessBuiltinAttribute checks if a built-in attribute should be processed for the given source
func shouldProcessBuiltinAttribute(key, configuredSource, currentSource string) bool {
	// If value_source is configured, use exact match
	if configuredSource != "" {
		return configuredSource == currentSource
	}
	// If value_source is not configured and it's a built-in attribute, check default sources
	defaultSources := getBuiltinAttributeDefaultSources(key)
	for _, src := range defaultSources {
		if src == currentSource {
			return true
		}
	}
	return false
}

// getBuiltinAttributeFallback provides protocol compatibility fallback for built-in attributes
func getBuiltinAttributeFallback(ctx wrapper.HttpContext, config AIStatisticsConfig, key, source string, body []byte, rule string) interface{} {
	switch key {
	case BuiltinQuestionKey:
		if source == RequestBody {
			// Try OpenAI/Claude format (both use same messages structure)
			if value := gjson.GetBytes(body, QuestionPathOpenAI).Value(); value != nil && value != "" {
				return value
			}
		}
	case BuiltinSystemKey:
		if source == RequestBody {
			// Try Claude /v1/messages format (system is a top-level field)
			if value := gjson.GetBytes(body, SystemPathClaude).Value(); value != nil && value != "" {
				return value
			}
		}
	case BuiltinAnswerKey:
		if source == ResponseStreamingBody {
			// Try OpenAI format first
			if value := extractStreamingBodyByJsonPath(body, AnswerPathOpenAIStreaming, rule); value != nil && value != "" {
				return value
			}
			// Try Claude format
			if value := extractStreamingBodyByJsonPath(body, AnswerPathClaudeStreaming, rule); value != nil && value != "" {
				return value
			}
		} else if source == ResponseBody {
			// Try OpenAI format first
			if value := gjson.GetBytes(body, AnswerPathOpenAINonStreaming).Value(); value != nil && value != "" {
				return value
			}
			// Try Claude format
			if value := gjson.GetBytes(body, AnswerPathClaudeNonStreaming).Value(); value != nil && value != "" {
				return value
			}
		}
	case BuiltinToolCallsKey:
		if source == ResponseStreamingBody {
			// Get or create buffer from context
			var buffer *StreamingToolCallsBuffer
			if existingBuffer, ok := ctx.GetContext(CtxStreamingToolCallsBuffer).(*StreamingToolCallsBuffer); ok {
				buffer = existingBuffer
			}
			// Try OpenAI format first
			buffer = extractStreamingToolCalls(body, buffer)
			// Also try Claude format (both formats can be checked)
			buffer = extractClaudeStreamingToolCalls(body, buffer)
			ctx.SetContext(CtxStreamingToolCallsBuffer, buffer)

			// Also set tool_calls to user attributes so they appear in ai_log
			toolCalls := getToolCallsFromBuffer(buffer)
			if len(toolCalls) > 0 {
				ctx.SetUserAttribute(BuiltinToolCallsKey, toolCalls)
				return toolCalls
			}
		} else if source == ResponseBody {
			if value := gjson.GetBytes(body, ToolCallsPathNonStreaming).Value(); value != nil {
				return value
			}
		}
	case BuiltinReasoningKey:
		if source == ResponseStreamingBody {
			if value := extractStreamingBodyByJsonPath(body, ReasoningPathStreaming, RuleAppend); value != nil && value != "" {
				return value
			}
		} else if source == ResponseBody {
			if value := gjson.GetBytes(body, ReasoningPathNonStreaming).Value(); value != nil && value != "" {
				return value
			}
		}
	case BuiltinReasoningTokens:
		// Extract reasoning_tokens from output_token_details (only available after response)
		if source == ResponseBody || source == ResponseStreamingBody {
			if outputTokenDetails, ok := ctx.GetContext(tokenusage.CtxKeyOutputTokenDetails).(map[string]int64); ok {
				if reasoningTokens, exists := outputTokenDetails["reasoning_tokens"]; exists {
					return reasoningTokens
				}
			}
		}
	case BuiltinCachedTokens:
		// Extract cached_tokens from input_token_details (only available after response)
		if source == ResponseBody || source == ResponseStreamingBody {
			if inputTokenDetails, ok := ctx.GetContext(tokenusage.CtxKeyInputTokenDetails).(map[string]int64); ok {
				if cachedTokens, exists := inputTokenDetails["cached_tokens"]; exists {
					return cachedTokens
				}
			}
		}
	case BuiltinInputTokenDetails:
		// Return the entire input_token_details map (only available after response)
		if source == ResponseBody || source == ResponseStreamingBody {
			if inputTokenDetails, ok := ctx.GetContext(tokenusage.CtxKeyInputTokenDetails).(map[string]int64); ok {
				return inputTokenDetails
			}
		}
	case BuiltinOutputTokenDetails:
		// Return the entire output_token_details map (only available after response)
		if source == ResponseBody || source == ResponseStreamingBody {
			if outputTokenDetails, ok := ctx.GetContext(tokenusage.CtxKeyOutputTokenDetails).(map[string]int64); ok {
				return outputTokenDetails
			}
		}
	}
	return nil
}

func extractStreamingBodyByJsonPath(data []byte, jsonPath string, rule string) interface{} {
	chunks := bytes.Split(bytes.TrimSpace(wrapper.UnifySSEChunk(data)), []byte("\n\n"))
	var value interface{}
	if rule == RuleFirst {
		for _, chunk := range chunks {
			jsonObj := gjson.GetBytes(chunk, jsonPath)
			if jsonObj.Exists() {
				value = jsonObj.Value()
				break
			}
		}
	} else if rule == RuleReplace {
		for _, chunk := range chunks {
			jsonObj := gjson.GetBytes(chunk, jsonPath)
			if jsonObj.Exists() {
				value = jsonObj.Value()
			}
		}
	} else if rule == RuleAppend {
		// extract llm response
		var strValue string
		for _, chunk := range chunks {
			jsonObj := gjson.GetBytes(chunk, jsonPath)
			if jsonObj.Exists() {
				strValue += jsonObj.String()
			}
		}
		value = strValue
	} else {
		log.Errorf("unsupported rule type: %s", rule)
	}
	return value
}

// shouldLogDebug returns true if the log level is debug or trace
func shouldLogDebug() bool {
	value, err := proxywasm.CallForeignFunction("get_log_level", nil)
	if err != nil {
		// If we can't get log level, default to not logging debug info
		return false
	}
	if len(value) < 4 {
		// Invalid log level value length
		return false
	}
	envoyLogLevel := binary.LittleEndian.Uint32(value[:4])
	return envoyLogLevel == LogLevelTrace || envoyLogLevel == LogLevelDebug
}

// debugLogAiLog logs the current user attributes that will be written to ai_log
func debugLogAiLog(ctx wrapper.HttpContext) {
	// Only log in debug/trace mode
	if !shouldLogDebug() {
		return
	}

	// Get all user attributes as a map
	userAttrs := make(map[string]interface{})

	// Try to reconstruct from GetUserAttribute (note: this is best-effort)
	// The actual attributes are stored internally, we log what we know
	if question := ctx.GetUserAttribute("question"); question != nil {
		userAttrs["question"] = question
	}
	if system := ctx.GetUserAttribute("system"); system != nil {
		userAttrs["system"] = system
	}
	if answer := ctx.GetUserAttribute("answer"); answer != nil {
		userAttrs["answer"] = answer
	}
	if reasoning := ctx.GetUserAttribute("reasoning"); reasoning != nil {
		userAttrs["reasoning"] = reasoning
	}
	if toolCalls := ctx.GetUserAttribute("tool_calls"); toolCalls != nil {
		userAttrs["tool_calls"] = toolCalls
	}
	if messages := ctx.GetUserAttribute("messages"); messages != nil {
		userAttrs["messages"] = messages
	}
	if sessionId := ctx.GetUserAttribute("session_id"); sessionId != nil {
		userAttrs["session_id"] = sessionId
	}
	if model := ctx.GetUserAttribute("model"); model != nil {
		userAttrs["model"] = model
	}
	if inputToken := ctx.GetUserAttribute("input_token"); inputToken != nil {
		userAttrs["input_token"] = inputToken
	}
	if outputToken := ctx.GetUserAttribute("output_token"); outputToken != nil {
		userAttrs["output_token"] = outputToken
	}
	if totalToken := ctx.GetUserAttribute("total_token"); totalToken != nil {
		userAttrs["total_token"] = totalToken
	}
	if chatId := ctx.GetUserAttribute("chat_id"); chatId != nil {
		userAttrs["chat_id"] = chatId
	}
	if responseType := ctx.GetUserAttribute("response_type"); responseType != nil {
		userAttrs["response_type"] = responseType
	}
	if llmFirstTokenDuration := ctx.GetUserAttribute("llm_first_token_duration"); llmFirstTokenDuration != nil {
		userAttrs["llm_first_token_duration"] = llmFirstTokenDuration
	}
	if llmServiceDuration := ctx.GetUserAttribute("llm_service_duration"); llmServiceDuration != nil {
		userAttrs["llm_service_duration"] = llmServiceDuration
	}
	if reasoningTokens := ctx.GetUserAttribute("reasoning_tokens"); reasoningTokens != nil {
		userAttrs["reasoning_tokens"] = reasoningTokens
	}
	if cachedTokens := ctx.GetUserAttribute("cached_tokens"); cachedTokens != nil {
		userAttrs["cached_tokens"] = cachedTokens
	}
	if inputTokenDetails := ctx.GetUserAttribute("input_token_details"); inputTokenDetails != nil {
		userAttrs["input_token_details"] = inputTokenDetails
	}
	if outputTokenDetails := ctx.GetUserAttribute("output_token_details"); outputTokenDetails != nil {
		userAttrs["output_token_details"] = outputTokenDetails
	}

	// Log the attributes as JSON
	logJson, _ := json.Marshal(userAttrs)
	log.Debugf("[ai_log] attributes to be written: %s", string(logJson))
}

// Set the tracing span with value.
func setSpanAttribute(key string, value interface{}) {
	if value != "" {
		traceSpanTag := wrapper.TraceSpanTagPrefix + key
		if e := proxywasm.SetProperty([]string{traceSpanTag}, []byte(fmt.Sprint(value))); e != nil {
			log.Warnf("failed to set %s in filter state: %v", traceSpanTag, e)
		}
	} else {
		log.Debugf("failed to write span attribute [%s], because it's value is empty", key)
	}
}

func writeMetric(ctx wrapper.HttpContext, config AIStatisticsConfig) {
	// Generate usage metrics
	var ok bool
	var route, cluster, model string
	consumer := ctx.GetStringContext(ConsumerKey, "none")
	route, ok = ctx.GetContext(RouteName).(string)
	if !ok {
		log.Info("RouteName type assert failed, skip metric record")
		return
	}
	cluster, ok = ctx.GetContext(ClusterName).(string)
	if !ok {
		log.Info("ClusterName type assert failed, skip metric record")
		return
	}

	if config.disableOpenaiUsage {
		return
	}

	if ctx.GetUserAttribute(tokenusage.CtxKeyModel) == nil || ctx.GetUserAttribute(tokenusage.CtxKeyInputToken) == nil || ctx.GetUserAttribute(tokenusage.CtxKeyOutputToken) == nil || ctx.GetUserAttribute(tokenusage.CtxKeyTotalToken) == nil {
		log.Info("get usage information failed, skip metric record")
		return
	}
	model, ok = ctx.GetUserAttribute(tokenusage.CtxKeyModel).(string)
	if !ok {
		log.Info("Model type assert failed, skip metric record")
		return
	}
	if inputToken, ok := convertToUInt(ctx.GetUserAttribute(tokenusage.CtxKeyInputToken)); ok {
		config.incrementCounter(generateMetricName(route, cluster, model, consumer, tokenusage.CtxKeyInputToken), inputToken)
	} else {
		log.Info("InputToken type assert failed, skip metric record")
	}
	if outputToken, ok := convertToUInt(ctx.GetUserAttribute(tokenusage.CtxKeyOutputToken)); ok {
		config.incrementCounter(generateMetricName(route, cluster, model, consumer, tokenusage.CtxKeyOutputToken), outputToken)
	} else {
		log.Info("OutputToken type assert failed, skip metric record")
	}
	if totalToken, ok := convertToUInt(ctx.GetUserAttribute(tokenusage.CtxKeyTotalToken)); ok {
		config.incrementCounter(generateMetricName(route, cluster, model, consumer, tokenusage.CtxKeyTotalToken), totalToken)
	} else {
		log.Info("TotalToken type assert failed, skip metric record")
	}

	// Generate duration metrics
	var llmFirstTokenDuration, llmServiceDuration uint64
	// Is stream response
	if ctx.GetUserAttribute(LLMFirstTokenDuration) != nil {
		llmFirstTokenDuration, ok = convertToUInt(ctx.GetUserAttribute(LLMFirstTokenDuration))
		if !ok {
			log.Info("LLMFirstTokenDuration type assert failed")
			return
		}
		config.incrementCounter(generateMetricName(route, cluster, model, consumer, LLMFirstTokenDuration), llmFirstTokenDuration)
		config.incrementCounter(generateMetricName(route, cluster, model, consumer, LLMStreamDurationCount), 1)
	}
	if ctx.GetUserAttribute(LLMServiceDuration) != nil {
		llmServiceDuration, ok = convertToUInt(ctx.GetUserAttribute(LLMServiceDuration))
		if !ok {
			log.Warnf("LLMServiceDuration type assert failed")
			return
		}
		config.incrementCounter(generateMetricName(route, cluster, model, consumer, LLMServiceDuration), llmServiceDuration)
		config.incrementCounter(generateMetricName(route, cluster, model, consumer, LLMDurationCount), 1)
	}
}

func convertToUInt(val interface{}) (uint64, bool) {
	switch v := val.(type) {
	case float32:
		return uint64(v), true
	case float64:
		return uint64(v), true
	case int32:
		return uint64(v), true
	case int64:
		return uint64(v), true
	case uint32:
		return uint64(v), true
	case uint64:
		return v, true
	default:
		return 0, false
	}
}
