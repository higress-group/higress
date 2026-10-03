// Copyright (c) 2022 Alibaba Group Holding Ltd.
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
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
)

func TestRequestHeadersBodyDetection(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		tests := []struct {
			name   string
			config json.RawMessage
		}{
			{
				name: "endpoint metrics",
				config: json.RawMessage(`{
					"lb_type": "endpoint",
					"lb_policy": "endpoint_metrics",
					"lb_config": {}
				}`),
			},
			{
				name: "global least request",
				config: json.RawMessage(`{
					"lb_type": "endpoint",
					"lb_policy": "global_least_request",
					"lb_config": {
						"serviceFQDN": "redis.default.svc.cluster.local",
						"servicePort": 6379
					}
				}`),
			},
			{
				name: "prefix cache",
				config: json.RawMessage(`{
					"lb_type": "endpoint",
					"lb_policy": "prefix_cache",
					"lb_config": {
						"serviceFQDN": "redis.default.svc.cluster.local",
						"servicePort": 6379
					}
				}`),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Run("bodyless request continues in headers phase", func(t *testing.T) {
					host, status := test.NewTestHost(tt.config)
					defer host.Reset()
					if status != types.OnPluginStartStatusOK {
						t.Fatalf("plugin start status = %v, want %v", status, types.OnPluginStartStatusOK)
					}

					action := host.CallOnHttpRequestHeaders([][2]string{
						{":authority", "example.com"},
						{":method", "GET"},
						{":path", "/health"},
					}, test.WithEndOfStream(true))

					if action != types.ActionContinue {
						t.Fatalf("header action = %v, want %v", action, types.ActionContinue)
					}

					host.CompleteHttp()
					for _, message := range host.GetErrorLogs() {
						if strings.Contains(message, "get host_selected failed") {
							t.Fatalf("bodyless request emitted a spurious host selection error: %q", message)
						}
					}
				})

				t.Run("request with body waits for body callback", func(t *testing.T) {
					host, status := test.NewTestHost(tt.config)
					defer host.Reset()
					if status != types.OnPluginStartStatusOK {
						t.Fatalf("plugin start status = %v, want %v", status, types.OnPluginStartStatusOK)
					}

					action := host.CallOnHttpRequestHeaders([][2]string{
						{":authority", "example.com"},
						{":method", "POST"},
						{":path", "/v1/chat/completions"},
						{"content-type", "application/json"},
					}, test.WithEndOfStream(false))

					if action != types.HeaderStopIteration {
						t.Fatalf("header action = %v, want %v", action, types.HeaderStopIteration)
					}
				})
			})
		}
	})
}
