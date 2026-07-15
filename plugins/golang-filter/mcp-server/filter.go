package mcp_server

import (
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
)

type filter struct {
	api.PassThroughStreamFilter

	callbacks api.FilterCallbackHandler

	config  *config
	req     *http.Request
	message bool
	path    string
	host    string
}

func (f *filter) DecodeHeaders(header api.RequestHeaderMap, endStream bool) api.StatusType {
	url := common.NewRequestURL(header)
	if url == nil {
		return api.Continue
	}
	f.path = url.ParsedURL.Path
	f.host = url.Host

	for _, server := range f.config.servers {
		if common.MatchDomainWithMatchers(f.host, server.HostMatchers) && strings.HasPrefix(f.path, server.BaseServer.GetMessageEndpoint()) {
			// Enforce HTTP Basic auth for servers that require it, before any
			// further processing of the request.
			if server.AuthUsername != "" {
				authHeader, _ := header.Get("authorization")
				if !common.CheckBasicAuth(authHeader, server.AuthUsername, server.AuthPassword) {
					f.callbacks.DecoderFilterCallbacks().SendLocalReply(
						http.StatusUnauthorized,
						"Unauthorized",
						map[string][]string{"WWW-Authenticate": {`Basic realm="MCP Server"`}},
						0, "")
					return api.LocalReply
				}
			}
			if url.Method != http.MethodPost {
				f.callbacks.DecoderFilterCallbacks().SendLocalReply(http.StatusMethodNotAllowed, "Method not allowed", nil, 0, "")
				return api.LocalReply
			}
			// Create a new http.Request object
			f.req = &http.Request{
				Method: url.Method,
				URL:    url.ParsedURL,
				Header: make(http.Header),
			}
			api.LogDebugf("Message request: %v", url.ParsedURL)
			// Copy headers from api.RequestHeaderMap to http.Header
			header.Range(func(key, value string) bool {
				f.req.Header.Add(key, value)
				return true
			})
			f.message = true
			if endStream {
				return api.Continue
			} else {
				return api.StopAndBuffer
			}
		}
	}

	return api.Continue
}

func (f *filter) DecodeData(buffer api.BufferInstance, endStream bool) api.StatusType {
	if f.message {
		for _, server := range f.config.servers {
			if common.MatchDomainWithMatchers(f.host, server.HostMatchers) && strings.HasPrefix(f.path, server.BaseServer.GetMessageEndpoint()) {
				if !endStream {
					return api.StopAndBuffer
				}
				// Create a response recorder to capture the response
				recorder := httptest.NewRecorder()
				// Call the handleMessage method of SSEServer with complete body
				httpStatus := server.BaseServer.HandleMessage(recorder, f.req, buffer.Bytes())
				f.message = false
				f.callbacks.DecoderFilterCallbacks().SendLocalReply(httpStatus, recorder.Body.String(), recorder.Header(), 0, "")
				return api.LocalReply
			}
		}
	}
	return api.Continue
}

func (f *filter) EncodeHeaders(header api.ResponseHeaderMap, endStream bool) api.StatusType {
	return api.Continue
}

func (f *filter) EncodeData(buffer api.BufferInstance, endStream bool) api.StatusType {
	return api.Continue
}
