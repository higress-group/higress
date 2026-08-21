package mcp_session

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/handler"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	RedisNotEnabledResponseBody = "Redis is not enabled, SSE connection is not supported"
)

// The callbacks in the filter, like `DecodeHeaders`, can be implemented on demand.
// Because api.PassThroughStreamFilter provides a default implementation.
type filter struct {
	api.PassThroughStreamFilter

	callbacks api.FilterCallbackHandler
	path      string
	config    *config
	stopChan  chan struct{}

	req                *http.Request
	serverName         string
	proxyURL           *url.URL
	matchedRule        common.MatchRule
	needProcess        bool
	skipRequestBody    bool
	skipResponseBody   bool
	cachedResponseBody []byte
	sseServer          *common.SSEServer // SSE server instance for this filter (per-request, not shared)

	userLevelConfig     bool
	mcpConfigHandler    *handler.MCPConfigHandler
	ratelimit           bool
	mcpRatelimitHandler *handler.MCPRatelimitHandler
}

// Callbacks which are called in request path
// The endStream is true if the request doesn't have body
func (f *filter) DecodeHeaders(header api.RequestHeaderMap, endStream bool) api.StatusType {
	requestUrl := common.NewRequestURL(header)
	if requestUrl == nil {
		return api.Continue
	}
	f.path = requestUrl.ParsedURL.Path

	// Check if request matches any rule in match_list
	matched, matchedRule := common.IsMatch(f.config.matchList, requestUrl.Host, f.path)
	if !matched {
		api.LogDebugf("Request does not match any rule in match_list: %s", requestUrl.ParsedURL.String())
		return api.Continue
	}
	f.needProcess = true
	f.matchedRule = matchedRule

	f.req = &http.Request{
		Method: requestUrl.Method,
		URL:    requestUrl.ParsedURL,
	}

	if strings.HasSuffix(f.path, ConfigPathSuffix) && f.config.enableUserLevelServer {
		if !requestUrl.InternalIP {
			api.LogWarnf("Access denied: non-Internal IP address %s", requestUrl.ParsedURL.String())
			f.callbacks.DecoderFilterCallbacks().SendLocalReply(http.StatusForbidden, "", nil, 0, "")
			return api.LocalReply
		}
		if strings.HasSuffix(f.path, ConfigPathSuffix) && requestUrl.Method == http.MethodGet {
			api.LogDebugf("Handling config request: %s", f.path)
			f.mcpConfigHandler.HandleConfigRequest(f.req, []byte{})
			return api.LocalReply
		}
		f.userLevelConfig = true
		if endStream {
			return api.Continue
		} else {
			return api.StopAndBuffer
		}
	}

	return f.processMcpRequestHeaders(header, endStream)
}

func (f *filter) processMcpRequestHeaders(header api.RequestHeaderMap, endStream bool) api.StatusType {
	switch f.matchedRule.UpstreamType {
	case common.RestUpstream, common.StreamableUpstream:
		return f.processMcpRequestHeadersForRestUpstream(header, endStream)
	case common.SSEUpstream:
		return f.processMcpRequestHeadersForSSEUpstream(header, endStream)
	}
	f.needProcess = false
	return api.Continue
}

func (f *filter) processMcpRequestHeadersForRestUpstream(header api.RequestHeaderMap, endStream bool) api.StatusType {
	method := f.req.Method
	requestUrl := f.req.URL
	if !strings.HasSuffix(requestUrl.Path, GlobalSSEPathSuffix) {
		f.proxyURL = requestUrl
		if f.config.enableUserLevelServer {
			parts := strings.Split(requestUrl.Path, "/")
			if len(parts) >= 3 {
				serverName := parts[1]
				uid := parts[2]
				// Get encoded config
				encodedConfig, _ := f.mcpConfigHandler.GetEncodedConfig(serverName, uid)
				if encodedConfig != "" {
					header.Set("x-higress-mcpserver-config", encodedConfig)
					api.LogDebugf("Set x-higress-mcpserver-config Header for %s:%s", serverName, uid)
				}
			}
			f.ratelimit = true
		}
		if endStream {
			return api.Continue
		}
		// The request body only needs to be buffered when user-level rate
		// limiting is enabled, since that path inspects the JSON-RPC method in
		// the body (see DecodeData). For plain REST/streamable MCP proxying we
		// never touch the body, so skip buffering it and stream it directly to
		// the upstream. Buffering the full body would otherwise cause large
		// requests to be rejected with a 413 once the body exceeds Envoy's
		// buffer limit, which cannot be enlarged from the Go filter. See #3238.
		if !f.ratelimit {
			f.skipRequestBody = true
			return api.Continue
		}
		return api.StopAndBuffer
	}

	if method != http.MethodGet {
		f.callbacks.DecoderFilterCallbacks().SendLocalReply(http.StatusMethodNotAllowed, "Method not allowed", nil, 0, "")
	} else {
		// to support the query param in Message Endpoint
		trimmed := strings.TrimSuffix(requestUrl.Path, GlobalSSEPathSuffix)
		if rq := requestUrl.RawQuery; rq != "" {
			trimmed += "?" + rq
		}

		// Create SSE server instance for this filter (per-request, not shared)
		// MCPServer is shared (thread-safe), but SSEServer must be per-request (contains request-specific messageEndpoint)
		f.sseServer = common.NewSSEServer(f.config.sharedMCPServer,
			common.WithSSEEndpoint(GlobalSSEPathSuffix),
			common.WithMessageEndpoint(trimmed),
			common.WithRedisClient(f.config.redisClient))
		f.serverName = f.sseServer.GetServerName()
		body := "SSE connection create"
		f.callbacks.DecoderFilterCallbacks().SendLocalReply(http.StatusOK, body, nil, 0, "")
	}
	return api.LocalReply
}

func (f *filter) processMcpRequestHeadersForSSEUpstream(header api.RequestHeaderMap, endStream bool) api.StatusType {
	// We don't need to process the request body for SSE upstream.
	f.skipRequestBody = true
	// Remove Accept-Encoding header to avoid gzip encoding,
	// which our response body handling logic doesn't support.
	header.Del("Accept-Encoding")
	return api.Continue
}

// DecodeData might be called multiple times during handling the request body.
// The endStream is true when handling the last piece of the body.
func (f *filter) DecodeData(buffer api.BufferInstance, endStream bool) api.StatusType {
	if !f.needProcess || f.skipRequestBody {
		return api.Continue
	}
	if f.matchedRule.UpstreamType != common.RestUpstream && f.matchedRule.UpstreamType != common.StreamableUpstream {
		return api.Continue
	}
	if !endStream {
		return api.StopAndBuffer
	}
	if f.userLevelConfig {
		// Handle config POST request
		api.LogDebugf("Handling config request: %s", f.path)
		f.mcpConfigHandler.HandleConfigRequest(f.req, buffer.Bytes())
		return api.LocalReply
	} else if f.ratelimit {
		if checkJSONRPCMethod(buffer.Bytes(), "tools/list") {
			api.LogDebugf("Not a tools call request, skipping ratelimit")
			return api.Continue
		}
		parts := strings.Split(f.req.URL.Path, "/")
		if len(parts) < 3 {
			api.LogWarnf("Access denied: no valid uid found")
			f.callbacks.DecoderFilterCallbacks().SendLocalReply(http.StatusForbidden, "", nil, 0, "")
			return api.LocalReply
		}
		serverName := parts[1]
		uid := parts[2]
		encodedConfig, err := f.mcpConfigHandler.GetEncodedConfig(serverName, uid)
		if err != nil {
			api.LogWarnf("Access denied: no valid config found for uid %s", uid)
			f.callbacks.DecoderFilterCallbacks().SendLocalReply(http.StatusForbidden, "", nil, 0, "")
			return api.LocalReply
		} else if encodedConfig == "" && checkJSONRPCMethod(buffer.Bytes(), "tools/call") {
			api.LogDebugf("Empty config found for %s:%s", serverName, uid)
			if !f.mcpRatelimitHandler.HandleRatelimit(f.req, buffer.Bytes()) {
				return api.LocalReply
			}
		}
	}
	return api.Continue
}

// EncodeHeaders Callbacks which are called in response path.
// The endStream is true if the response doesn't have body.
func (f *filter) EncodeHeaders(header api.ResponseHeaderMap, endStream bool) api.StatusType {
	if !f.needProcess {
		return api.Continue
	}
	if f.matchedRule.UpstreamType != common.RestUpstream && f.matchedRule.UpstreamType != common.StreamableUpstream {
		if contentType, ok := header.Get("content-type"); !ok || !strings.HasPrefix(contentType, "text/event-stream") {
			api.LogDebugf("Skip response body for non-SSE upstream. Content-Type: %s", contentType)
			f.skipResponseBody = true
		}
		return api.Continue
	}
	if f.serverName != "" {
		if f.config.redisClient != nil {
			header.Set("Content-Type", "text/event-stream")
			header.Set("Cache-Control", "no-cache")
			header.Set("Connection", "keep-alive")
			header.Set("Access-Control-Allow-Origin", "*")
			header.Del("Content-Length")
		} else {
			header.Set("Content-Length", strconv.Itoa(len(RedisNotEnabledResponseBody)))
		}
		return api.Continue
	}
	return api.Continue
}

// EncodeData might be called multiple times during handling the response body.
// The endStream is true when handling the last piece of the body.
func (f *filter) EncodeData(buffer api.BufferInstance, endStream bool) api.StatusType {
	if !f.needProcess || f.skipResponseBody {
		return api.Continue
	}

	ret := api.Continue
	api.LogDebugf("Upstream Type: %s", f.matchedRule.UpstreamType)
	switch f.matchedRule.UpstreamType {
	case common.RestUpstream:
		api.LogDebugf("Encoding data from Rest upstream")
		ret = f.encodeDataFromRestUpstream(buffer, endStream)
	case common.SSEUpstream:
		api.LogDebugf("Encoding data from SSE upstream")
		ret = f.encodeDataFromSSEUpstream(buffer, endStream)
		if endStream {
			// Always continue as long as the stream has ended.
			ret = api.Continue
		}
	case common.StreamableUpstream:
		// Do nothing for streamable upstream
	}
	return ret
}

func (f *filter) encodeDataFromRestUpstream(buffer api.BufferInstance, endStream bool) api.StatusType {
	if !f.needProcess {
		return api.Continue
	}
	if !endStream {
		return api.StopAndBuffer
	}
	if f.proxyURL != nil && f.config.redisClient != nil {
		sessionID := f.proxyURL.Query().Get("sessionId")
		if sessionID != "" {
			channel := common.GetSSEChannelName(sessionID)
			eventData := fmt.Sprintf("event: message\ndata: %s\n\n", buffer.String())
			publishErr := f.config.redisClient.Publish(channel, eventData)
			if publishErr != nil {
				api.LogErrorf("Failed to publish wasm mcp server message to Redis: %v", publishErr)
			}
		}
	}

	if f.serverName != "" {
		if f.config.redisClient != nil {
			// handle SSE server for this filter instance
			buffer.Reset()
			f.sseServer.HandleSSE(f.callbacks, f.stopChan)
			return api.Running
		} else {
			_ = buffer.SetString(RedisNotEnabledResponseBody)
			return api.Continue
		}
	}
	return api.Continue
}

func (f *filter) encodeDataFromSSEUpstream(buffer api.BufferInstance, endStream bool) api.StatusType {
	bufferBytes := buffer.Bytes()
	bufferData := string(bufferBytes)
	api.LogDebugf("Received SSE data: %q, length: %d, endStream: %v", bufferData, len(bufferData), endStream)

	// Combine cached data with new data
	var combinedData string
	if len(f.cachedResponseBody) > 0 {
		combinedData = string(f.cachedResponseBody) + bufferData
		api.LogDebugf("Combined with cached data: %q, total length: %d", combinedData, len(combinedData))
	} else {
		combinedData = bufferData
	}

	err, endpointUrl := f.findEndpointUrl(combinedData)
	if err != nil {
		api.LogWarnf("Failed to find endpoint URL in SSE data: %v", err)
		f.needProcess = false
		return api.Continue
	}
	if endpointUrl == "" {
		// No endpoint URL found. Need to buffer and check again.
		f.cachedResponseBody = []byte(combinedData)
		buffer.Reset()
		return api.Continue
	}
	// Clear cached data
	f.cachedResponseBody = nil

	// Remove query string since we don't need to change it.
	queryStringIndex := strings.IndexAny(endpointUrl, "?")
	if queryStringIndex != -1 {
		endpointUrl = endpointUrl[:queryStringIndex]
	}

	if changed, newEndpointUrl := f.rewriteEndpointUrl(endpointUrl); changed {
		api.LogDebugf("The endpoint URL is changed.\n  Old: %s\n  New: %s", endpointUrl, newEndpointUrl)

		endpointUrlIndex := strings.Index(combinedData, endpointUrl)
		if endpointUrlIndex == -1 {
			api.LogWarnf("Something wrong, the previously found endpoint URL %s not found in the SSE data now", endpointUrl)
		} else {
			newBufferData := combinedData[:endpointUrlIndex] + newEndpointUrl + combinedData[endpointUrlIndex+len(endpointUrl):]
			_ = buffer.SetString(newBufferData)
		}
	} else {
		api.LogDebugf("The endpoint URL %s is not changed", endpointUrl)
	}

	f.needProcess = false
	return api.Continue
}

func (f *filter) rewriteEndpointUrl(endpointUrl string) (bool, string) {
	if !f.matchedRule.EnablePathRewrite {
		return false, ""
	}

	if schemeIndex := strings.Index(endpointUrl, "://"); schemeIndex != -1 {
		endpointUrl = endpointUrl[schemeIndex+3:]
		if slashIndex := strings.Index(endpointUrl, "/"); slashIndex != -1 {
			endpointUrl = endpointUrl[slashIndex:]
		} else {
			endpointUrl = "/"
		}
	}

	if !strings.HasPrefix(endpointUrl, f.matchedRule.PathRewritePrefix) {
		// The endpoint URL does not match the path rewrite prefix. We are unable to rewrite it back.
		api.LogWarnf("The endpoint URL %s does not match the path rewrite prefix %s", endpointUrl, f.matchedRule.PathRewritePrefix)
		return false, ""
	}

	suffix := endpointUrl[len(f.matchedRule.PathRewritePrefix):]

	if len(suffix) == 0 {
		endpointUrl = f.matchedRule.MatchRulePath
	} else {
		matchPathHasTrailingSlash := strings.HasSuffix(f.matchedRule.MatchRulePath, "/")
		suffixHasLeadingSlash := strings.HasPrefix(suffix, "/")
		if matchPathHasTrailingSlash != suffixHasLeadingSlash {
			// One has, the other doesn't have.
			endpointUrl = f.matchedRule.MatchRulePath + suffix
		} else if matchPathHasTrailingSlash {
			// Both have.
			endpointUrl = f.matchedRule.MatchRulePath + suffix[1:]
		} else {
			// Neither have.
			endpointUrl = f.matchedRule.MatchRulePath + "/" + suffix
		}
	}

	return true, endpointUrl
}

func (f *filter) findNextLineBreak(bufferData string) (error, string) {
	// See https://html.spec.whatwg.org/multipage/server-sent-events.html
	crIndex := strings.IndexAny(bufferData, "\r")
	lfIndex := strings.IndexAny(bufferData, "\n")
	if crIndex == -1 && lfIndex == -1 {
		// No line break found.
		return nil, ""
	}
	lineBreak := ""
	if crIndex != -1 && lfIndex != -1 {
		if crIndex < lfIndex {
			if crIndex+1 == lfIndex {
				lineBreak = "\r\n"
			} else {
				lineBreak = "\r"
			}
		} else {
			if crIndex == lfIndex+1 {
				// Found unexpected "\n\r". Skip body processing.
				return errors.New("found unexpected LF+CR"), ""
			} else {
				lineBreak = "\n"
			}
		}
	} else if crIndex != -1 {
		lineBreak = "\r"
	} else {
		lineBreak = "\n"
	}
	return nil, lineBreak
}

func (f *filter) findEndpointUrl(bufferData string) (error, string) {
	// Keep searching for events until we find an endpoint event or run out of data
	for {
		eventIndex := strings.Index(bufferData, "event:")
		if eventIndex == -1 {
			// No more events found
			return nil, ""
		}

		// Move to the start of the event
		bufferData = bufferData[eventIndex:]

		// Find the end of the event line
		err, lineBreak := f.findNextLineBreak(bufferData)
		if err != nil {
			return fmt.Errorf("failed to find endpoint URL in SSE data: %v", err), ""
		}
		if lineBreak == "" {
			// No line break found, which means the data is not enough.
			return nil, ""
		}

		api.LogDebugf("event line break sequence: %v", []byte(lineBreak))
		eventEndIndex := strings.Index(bufferData, lineBreak)
		if eventEndIndex == -1 {
			return nil, ""
		}

		eventName := strings.TrimSpace(bufferData[len("event:"):eventEndIndex])

		// Move past the event line
		bufferData = bufferData[eventEndIndex+len(lineBreak):]

		if eventName == "endpoint" {
			// Found endpoint event, now look for the data field
			err, lineBreak = f.findNextLineBreak(bufferData)
			if err != nil {
				return fmt.Errorf("failed to find endpoint URL in SSE data: %v", err), ""
			}
			if lineBreak == "" {
				// No line break found, which means the data is not enough.
				return nil, ""
			}

			api.LogDebugf("data line break sequence: %v", []byte(lineBreak))
			dataEndIndex := strings.Index(bufferData, lineBreak)
			if dataEndIndex == -1 {
				// Data received not enough.
				return nil, ""
			}

			eventData := bufferData[:dataEndIndex]
			if !strings.HasPrefix(eventData, "data:") {
				return fmt.Errorf("an unexpected non-data field found in the event. Skip processing. Field: %s", eventData), ""
			}

			return nil, strings.TrimSpace(eventData[len("data:"):])
		} else {
			// Not an endpoint event, skip to the next event
			api.LogDebugf("Skipping non-endpoint event: %s", eventName)

			// First, we need to skip the data field of this event
			err, lineBreak = f.findNextLineBreak(bufferData)
			if err != nil {
				return fmt.Errorf("failed to find endpoint URL in SSE data: %v", err), ""
			}
			if lineBreak == "" {
				// No line break found, which means the data is not enough.
				return nil, ""
			}

			dataEndIndex := strings.Index(bufferData, lineBreak)
			if dataEndIndex == -1 {
				// Data received not enough.
				return nil, ""
			}

			// Move past the data line
			bufferData = bufferData[dataEndIndex+len(lineBreak):]

			// Skip any additional empty lines that separate events
			for strings.HasPrefix(bufferData, lineBreak) {
				bufferData = bufferData[len(lineBreak):]
			}

			// Continue to look for the next event
		}
	}
}

// OnDestroy stops the goroutine
func (f *filter) OnDestroy(reason api.DestroyReason) {
	api.LogDebugf("OnDestroy: reason=%v", reason)
	f.cachedResponseBody = nil
	if f.serverName != "" && f.stopChan != nil {
		select {
		case <-f.stopChan:
			return
		default:
			api.LogDebug("Stopping SSE connection")
			close(f.stopChan)
		}
	}
}

// check if the request is a tools/call request
func checkJSONRPCMethod(body []byte, method string) bool {
	var request mcp.CallToolRequest
	if err := json.Unmarshal(body, &request); err != nil {
		api.LogWarnf("Failed to unmarshal request body: %v, not a JSON RPC request", err)
		return true
	}

	return request.Method == method
}
