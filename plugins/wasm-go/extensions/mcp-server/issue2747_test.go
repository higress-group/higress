package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var issue2747Config = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"server": map[string]interface{}{
			"name": "issue-2747",
			"type": "rest",
		},
		"tools": []map[string]interface{}{
			{
				"name": "lookup",
				"args": []map[string]interface{}{
					{
						"name":     "location",
						"type":     "string",
						"required": true,
					},
				},
				"requestTemplate": map[string]interface{}{
					"url":            "https://httpbin.org/get",
					"method":         "GET",
					"argsToUrlParam": true,
				},
			},
		},
	})
	return data
}()

func requestHeader(headers [][2]string, name string) string {
	for _, header := range headers {
		if strings.EqualFold(header[0], name) {
			return header[1]
		}
	}
	return ""
}

func TestIssue2747MissingRequiredArgument(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("legacy rejects missing required argument", func(t *testing.T) {
			host, status := test.NewTestHost(issue2747Config)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "mcp.example.com"},
				{":method", "POST"},
				{":path", "/mcp"},
				{"content-type", "application/json"},
				{"accept", "application/json, text/event-stream"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			body := []byte(`{
				"jsonrpc":"2.0",
				"id":1,
				"method":"tools/call",
				"params":{
					"name":"lookup",
					"arguments":{}
				}
			}`)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))

			response := host.GetLocalResponse()
			require.NotNil(t, response)
			require.Contains(t, string(response.Data), "location")
			require.Equal(t, "/mcp",
				requestHeader(host.GetRequestHeaders(), ":path"))
		})

		t.Run("modern rejects missing required argument", func(t *testing.T) {
			host, status := test.NewTestHost(issue2747Config)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "mcp.example.com"},
				{":method", "POST"},
				{":path", "/mcp"},
				{"content-type", "application/json"},
				{"accept", "application/json, text/event-stream"},
				{"MCP-Protocol-Version", "2026-07-28"},
				{"Mcp-Method", "tools/call"},
				{"Mcp-Name", "lookup"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			body := []byte(`{
				"jsonrpc":"2.0",
				"id":2,
				"method":"tools/call",
				"params":{
					"name":"lookup",
					"arguments":{},
					"_meta":{
						"io.modelcontextprotocol/protocolVersion":"2026-07-28",
						"io.modelcontextprotocol/clientCapabilities":{}
					}
				}
			}`)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))

			response := host.GetLocalResponse()
			require.NotNil(t, response)
			require.Contains(t, string(response.Data), "location")
			require.Equal(t, "/mcp",
				requestHeader(host.GetRequestHeaders(), ":path"))
		})

		t.Run("legacy routes when required argument is present", func(t *testing.T) {
			host, status := test.NewTestHost(issue2747Config)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "mcp.example.com"},
				{":method", "POST"},
				{":path", "/mcp"},
				{"content-type", "application/json"},
				{"accept", "application/json, text/event-stream"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			body := []byte(`{
				"jsonrpc":"2.0",
				"id":3,
				"method":"tools/call",
				"params":{
					"name":"lookup",
					"arguments":{
						"location":"hangzhou"
					}
				}
			}`)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))

			require.Nil(t, host.GetLocalResponse())
			require.Equal(t, "/get?location=hangzhou",
				requestHeader(host.GetRequestHeaders(), ":path"))
		})
	})
}
