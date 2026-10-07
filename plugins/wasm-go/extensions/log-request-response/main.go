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

// Constants for log keys in Filter State
const (
	pluginName            = "log-request-response"
	logKeyRequestHeaders  = "log-request-headers"
	logKeyRequestBody     = "log-request-body"
	logKeyResponseHeaders = "log-response-headers"
	logKeyResponseBody    = "log-response-body"
)

// Constants for context keys
const (
	contextKeyRequestBodyBuffer  = "request_body_buffer"
	contextKeyResponseBodyBuffer = "response_body_buffer"
)

// HTTP/2 header name mapping
var http2HeaderMap = map[string]string{
	":authority": "authority",
	":method":    "method",
	":path":      "path",
	":scheme":    "scheme",
	":status":    "status",
}

func main() {}

func init() {
	wrapper.SetCtx(
		// Plugin name
		pluginName,
		// Set custom function for parsing plugin configuration
		wrapper.ParseConfig(parseConfig),
		// Set custom function for processing request headers
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		// Set custom function for processing streaming request body
		wrapper.ProcessStreamingRequestBody(onStreamingRequestBody),
		// Set custom function for processing response headers
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		// Set custom function for processing streaming response body
		wrapper.ProcessStreamingResponseBody(onStreamingResponseBody),
		// Set the maximum memory limit for WASM plugin rebuild to 200MB
		wrapper.WithRebuildMaxMemBytes[PluginConfig](200*1024*1024),
	)
}

// PluginConfig Custom plugin configuration
type PluginConfig struct {
	// Request configuration
	Request struct {
		// Headers configuration
		Headers struct {
			// Whether to enable request headers logging
			Enabled bool
		}
		// Body configuration
		Body struct {
			// Whether to enable request body logging
			Enabled bool
			// Maximum size limit for logging (bytes)
			MaxSize int
			// Content types to be logged
			ContentTypes []string
		}
	}
	// Response configuration
	Response struct {
		// Headers configuration
		Headers struct {
			// Whether to enable response headers logging
			Enabled bool
		}
		// Body configuration
		Body struct {
			// Whether to enable response body logging
			Enabled bool
			// Maximum size limit for logging (bytes)
			MaxSize int
			// Content types to be logged
			ContentTypes []string
		}
	}
}

// The YAML configuration filled in the console will be automatically converted to JSON,
// so we can directly parse the configuration from this JSON parameter
func parseConfig(json gjson.Result, config *PluginConfig) error {
	// Parse request headers configuration
	config.Request.Headers.Enabled = json.Get("request.headers.enabled").Bool()

	// Parse request body configuration
	config.Request.Body.Enabled = json.Get("request.body.enabled").Bool()
	config.Request.Body.MaxSize = int(json.Get("request.body.maxSize").Int())

	// Set default maximum size for request body
	if config.Request.Body.MaxSize <= 0 {
		config.Request.Body.MaxSize = 10 * 1024 // Default 10KB
	}

	// Parse request body content types
	if contentTypes := json.Get("request.body.contentTypes").Array(); len(contentTypes) > 0 {
		for _, ct := range contentTypes {
			config.Request.Body.ContentTypes = append(config.Request.Body.ContentTypes, ct.String())
		}
	} else {
		// Default content types
		config.Request.Body.ContentTypes = []string{
			"application/json",
			"application/xml",
			"application/x-www-form-urlencoded",
			"text/plain",
		}
	}

	// Parse response headers configuration
	config.Response.Headers.Enabled = json.Get("response.headers.enabled").Bool()

	// Parse response body configuration
	config.Response.Body.Enabled = json.Get("response.body.enabled").Bool()
	config.Response.Body.MaxSize = int(json.Get("response.body.maxSize").Int())

	// Set default maximum size for response body
	if config.Response.Body.MaxSize <= 0 {
		config.Response.Body.MaxSize = 10 * 1024 // Default 10KB
	}

	// Parse response body content types
	if contentTypes := json.Get("response.body.contentTypes").Array(); len(contentTypes) > 0 {
		for _, ct := range contentTypes {
			config.Response.Body.ContentTypes = append(config.Response.Body.ContentTypes, ct.String())
		}
	} else {
		// Default content types
		config.Response.Body.ContentTypes = []string{
			"application/json",
			"application/xml",
			"text/plain",
			"text/html",
		}
	}

	return nil
}

// normalizeHeaderName standardizes HTTP/2 header names by removing the colon prefix
// or mapping them to more standard names
func normalizeHeaderName(name string) string {
	// If it's a known HTTP/2 header, map it to a standard name
	if standardName, exists := http2HeaderMap[name]; exists {
		return standardName
	}

	// For other headers that might start with colon, just remove the colon
	if strings.HasPrefix(name, ":") {
		return name[1:]
	}

	// Return the original name for regular headers
	return name
}

// processStreamingBody common function to process streaming body
func processStreamingBody(
	ctx wrapper.HttpContext,
	enabled bool,
	maxSize int,
	bufferKey string,
	logKey string,
	chunk []byte,
	isEndStream bool,
) []byte {
	// If body logging is not enabled or max size is <= 0, just return the chunk as is
	if !enabled || maxSize <= 0 {
		return chunk
	}

	// Get the buffer from context
	buffer, _ := ctx.GetContext(bufferKey).([]byte)

	// If we haven't reached max size yet, append chunk to buffer
	if len(buffer) < maxSize {
		// Calculate how much of this chunk we can add
		remainingCapacity := maxSize - len(buffer)
		if remainingCapacity > 0 {
			if len(chunk) <= remainingCapacity {
				buffer = append(buffer, chunk...)
				ctx.SetContext(bufferKey, buffer)
			} else {
				buffer = append(buffer, chunk[:remainingCapacity]...)
				// reach max size, record and clear
				bodyStr := string(buffer)
				setPropertyWithMarshal(logKey, bodyStr)
				// clear buffer
				ctx.SetContext(bufferKey, []byte{})
			}
		}
	}

	// When we reach the end of stream, create log entry
	if isEndStream && len(buffer) > 0 {
		bodyStr := string(buffer)
		setPropertyWithMarshal(logKey, bodyStr)
		// clear buffer
		ctx.SetContext(bufferKey, []byte{})
	}

	// Always return the original chunk unmodified
	return chunk
}

// setPropertyWithMarshal marshals the given string value into a JSON-safe format
// and sets it as a property in the Envoy filter state with the specified key.
// This ensures proper escaping of special characters when the value is included in JSON.
func setPropertyWithMarshal(key string, value string) {
	// Create a helper map to properly escape the string using JSON marshaling
	helper := map[string]string{
		"placeholder": value,
	}

	// Marshal the helper map to JSON
	marshalledHelper, _ := json.Marshal(helper)

	// Extract the properly escaped value using gjson
	marshalledRaw := gjson.GetBytes(marshalledHelper, "placeholder").Raw

	var marshalledStr string
	if len(marshalledRaw) >= 2 {
		// Remove the surrounding quotes from the JSON string
		marshalledStr = marshalledRaw[1 : len(marshalledRaw)-1]
	} else {
		log.Errorf("failed to marshal json string, raw string is: %s", value)
		marshalledStr = ""
	}

	// Set the property with the marshaled string
	if err := proxywasm.SetProperty([]string{key}, []byte(marshalledStr)); err != nil {
		log.Errorf("failed to set %s in filter state, err: %v, raw:\n%s", key, err, value)
	}
}

// onHttpRequestHeaders processes the request headers and logs them if enabled
func onHttpRequestHeaders(ctx wrapper.HttpContext, config PluginConfig) types.Action {
	// Get all request headers
	headers, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		log.Errorf("Failed to get request headers: %v", err)
		return types.ActionContinue
	}

	method := ""
	contentType := ""

	// Check if request headers need to be logged
	if config.Request.Headers.Enabled {
		jsonStr := "{}"
		for _, header := range headers {
			var err error
			normalizedName := normalizeHeaderName(header[0])
			jsonStr, err = sjson.Set(jsonStr, normalizedName, header[1])
			if err != nil {
				log.Errorf("Failed to convert request header to JSON: name=%s, value=%s, error=%v", normalizedName, header[1], err)
			}
		}

		setPropertyWithMarshal(logKeyRequestHeaders, jsonStr)
	}

	// Get request method and Content-Type for subsequent processing
	for _, header := range headers {
		if strings.ToLower(header[0]) == ":method" {
			method = header[1]
		} else if strings.ToLower(header[0]) == "content-type" {
			contentType = header[1]
		}
	}

	// For non-POST/PUT/PATCH requests, or if request body logging is not enabled, no need to log the request body
	if !config.Request.Body.Enabled || (method != "POST" && method != "PUT" && method != "PATCH") {
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	// Check if the content type is in the configured list for logging
	shouldLogBody := false
	for _, allowedType := range config.Request.Body.ContentTypes {
		if strings.Contains(contentType, allowedType) {
			shouldLogBody = true
			break
		}
	}

	if !shouldLogBody {
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	// Initialize a buffer to accumulate request body chunks
	ctx.SetContext(contextKeyRequestBodyBuffer, []byte{})

	return types.ActionContinue
}

// onStreamingRequestBody processes each chunk of the request body in streaming mode
// This allows us to log the request body without affecting the original request
func onStreamingRequestBody(ctx wrapper.HttpContext, config PluginConfig, chunk []byte, isEndStream bool) []byte {
	return processStreamingBody(
		ctx,
		config.Request.Body.Enabled,
		config.Request.Body.MaxSize,
		contextKeyRequestBodyBuffer,
		logKeyRequestBody,
		chunk,
		isEndStream,
	)
}

// onHttpResponseHeaders processes the response headers and logs them if enabled
func onHttpResponseHeaders(ctx wrapper.HttpContext, config PluginConfig) types.Action {
	// Get all response headers
	headers, err := proxywasm.GetHttpResponseHeaders()
	if err != nil {
		log.Errorf("Failed to get response headers: %v", err)
		return types.ActionContinue
	}

	// Check if response headers need to be logged
	if config.Response.Headers.Enabled {
		jsonStr := "{}"
		for _, header := range headers {
			var err error
			normalizedName := normalizeHeaderName(header[0])
			jsonStr, err = sjson.Set(jsonStr, normalizedName, header[1])
			if err != nil {
				log.Errorf("Failed to convert response header to JSON: name=%s, value=%s, error=%v", normalizedName, header[1], err)
			}
		}

		setPropertyWithMarshal(logKeyResponseHeaders, jsonStr)
	}

	// Check if response body needs to be logged
	if !config.Response.Body.Enabled {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	// Check Content-Type and Content-Encoding for response body logging
	contentType := ""
	hasContentEncoding := false
	for _, header := range headers {
		if strings.ToLower(header[0]) == "content-type" {
			contentType = header[1]
		} else if strings.ToLower(header[0]) == "content-encoding" {
			hasContentEncoding = true
		}
	}

	// Skip response body logging if content encoding is present (avoid logging compressed content)
	if hasContentEncoding {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	// Skip response body logging if content type is not in the configured list
	if contentType != "" {
		shouldLogBody := false
		for _, allowedType := range config.Response.Body.ContentTypes {
			if strings.Contains(contentType, allowedType) {
				shouldLogBody = true
				break
			}
		}

		if !shouldLogBody {
			ctx.DontReadResponseBody()
			return types.ActionContinue
		}
	}

	// Initialize a buffer to accumulate response body chunks
	ctx.SetContext(contextKeyResponseBodyBuffer, []byte{})

	return types.ActionContinue
}

// onStreamingResponseBody processes each chunk of the response body in streaming mode
// This allows us to log the response body without affecting the original response
func onStreamingResponseBody(ctx wrapper.HttpContext, config PluginConfig, chunk []byte, isEndStream bool) []byte {
	return processStreamingBody(
		ctx,
		config.Response.Body.Enabled,
		config.Response.Body.MaxSize,
		contextKeyResponseBodyBuffer,
		logKeyResponseBody,
		chunk,
		isEndStream,
	)
}
