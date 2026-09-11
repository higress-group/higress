// Copyright (c) 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.

package server

import (
	"fmt"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"testing"
)

func TestAutoConfiguration(t *testing.T) {
	for _, tc := range []struct {
		strategy, transport, timeout string
		want                         uint32
		invalid                      bool
	}{
		{"", "http", "", 1000, false}, {"legacy", "http", "0", 1000, false}, {"modern", "http", "false", 1000, false},
		{"auto", "sse", "0", 1000, false}, {"auto", "http", "", 1000, false}, {"auto", "http", "42", 42, false},
		{"auto", "http", "0", 0, true}, {"auto", "http", "-1", 0, true}, {"auto", "http", "1.5", 0, true},
		{"auto", "http", `"42"`, 0, true}, {"auto", "http", "null", 0, true}, {"auto", "http", "4294967296", 0, true},
		{"auto", "http", "4294967295", 4294967295, false},
	} {
		t.Run(tc.strategy+tc.transport+tc.timeout, func(t *testing.T) {
			extra := ""
			if tc.timeout != "" {
				extra = `,"autoDetection":{"probeTimeoutMs":` + tc.timeout + `}`
			}
			config := gjson.Parse(fmt.Sprintf(`{"transport":%q,"protocolStrategy":%q,"mcpServerURL":"/mcp"%s}`, tc.transport, tc.strategy, extra))
			s, err := setupMcpProxyServer("proxy", config, "")
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, s.GetAutoDetectionConfig().ProbeTimeoutMs)
			clone := s.Clone().(*McpProxyServer)
			assert.Equal(t, s.GetProtocolStrategy(), clone.GetProtocolStrategy())
			assert.Equal(t, s.GetAutoDetectionConfig(), clone.GetAutoDetectionConfig())
		})
	}
}

func TestAutoOperationHeadersDoNotInheritBusinessIdentity(t *testing.T) {
	p := &PreparedProxyRequest{toolNameHeader: "encoded-tool", authHeaders: [][2]string{{"Cookie", "upstream=explicit"}}, forwardHeaders: [][2]string{{"traceparent", "trace"}, {"Mcp-Param-Region", "region"}, {"Cookie", "downstream=secret"}, {"Mcp-Session-Id", "old"}}}
	for _, method := range []string{"server/discover", "initialize", "notifications/initialized", "tools/list", "tools/call"} {
		version := protocol.Version20260728
		if method == "initialize" || method == "notifications/initialized" {
			version = protocol.Version20250618
		}
		headers := p.headers(OutboundOperation{method: method, version: version})
		assert.Equal(t, "upstream=explicit", mustHeaderValue(t, headers, "Cookie"))
		assert.Equal(t, "trace", mustHeaderValue(t, headers, "traceparent"))
		_, param := findHeader(headers, "Mcp-Param-Region")
		assert.Equal(t, method == "tools/call", param)
		_, name := findHeader(headers, protocol.HeaderName)
		assert.Equal(t, method == "tools/call", name)
		if version == protocol.Version20260728 {
			assert.Equal(t, method, mustHeaderValue(t, headers, protocol.HeaderMethod))
		}
	}
}
