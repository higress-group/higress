package main

import (
	"net/http"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"sni-misdirect",
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
	)
}

type Config struct {
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config Config, log log.Log) types.Action {
	// no need to check HTTP/1.0 and HTTP/1.1
	protocol, err := proxywasm.GetProperty([]string{"request", "protocol"})
	if err != nil {
		log.Errorf("failed to get request protocol: %v", err)
		return types.ActionContinue
	}
	if strings.HasPrefix(string(protocol), "HTTP/1") {
		return types.ActionContinue
	}
	// no need to check http scheme
	scheme := ctx.Scheme()
	if scheme != "https" {
		return types.ActionContinue
	}
	// no need to check grpc
	contentType, err := proxywasm.GetHttpRequestHeader("content-type")
	if err != nil {
		log.Errorf("failed to get request content-type: %v", err)
		return types.ActionContinue
	}
	if strings.HasPrefix(contentType, "application/grpc") {
		return types.ActionContinue
	}
	// get sni
	sni, err := proxywasm.GetProperty([]string{"connection", "requested_server_name"})
	if err != nil {
		log.Errorf("failed to get requested_server_name: %v", err)
		return types.ActionContinue
	}
	// get authority
	host, err := proxywasm.GetHttpRequestHeader(":authority")
	if err != nil {
		log.Errorf("failed to get request authority: %v", err)
		return types.ActionContinue
	}
	host = stripPortFromHost(host)
	if string(sni) == host {
		return types.ActionContinue
	}
	if !strings.HasPrefix(string(sni), "*.") {
		proxywasm.SendHttpResponseWithDetail(http.StatusMisdirectedRequest, "sni-misdirect.mismatched.non_wildcard", nil, []byte("Misdirected Request"), -1)
		return types.ActionPause
	}
	if !strings.Contains(host, string(sni)[1:]) {
		proxywasm.SendHttpResponseWithDetail(http.StatusMisdirectedRequest, "sni-misdirect.mismatched.wildcard", nil, []byte("Misdirected Request"), -1)
		return types.ActionPause
	}
	return types.ActionContinue
}

func stripPortFromHost(requestHost string) string {
	// Find the last occurrence of ':' to locate the port.
	portStart := strings.LastIndex(requestHost, ":")

	// Check if ':' is found.
	if portStart != -1 {
		// According to RFC3986, IPv6 address is always enclosed in "[]".
		// section 3.2.2.
		v6EndIndex := strings.LastIndex(requestHost, "]")

		// Check if ']' is found and its position is after the ':'.
		if v6EndIndex == -1 || v6EndIndex < portStart {
			// Check if there are characters after ':'.
			if portStart+1 <= len(requestHost) {
				// Return the substring without the port.
				return requestHost[:portStart]
			}
		}
	}

	// If no port is found or the conditions are not met, return the original requestHost.
	return requestHost
}
