// Copyright (c) 2025 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// A limit_by_param rule is resolved from the request query string. url.ParseQuery
// reports an error for the first malformed percent escape but still returns every
// pair it could decode, so discarding the map on error silently drops the rule:
// a caller appends &x=%zz to a request that is otherwise within the rule and the
// limiter stops seeing it at all.
func TestParamRuleSurvivesMalformedQueryEscape(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("well formed query still evaluates the rule", func(t *testing.T) {
			host, status := test.NewTestHost(paramLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test?apikey=param-key-456"},
				{":method", "POST"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			host.CallOnRedisCall(0, multiRuleResp([3]int{50, 1, 60}))
			host.CompleteHttp()
		})

		t.Run("malformed escape in another param must not disable the rule", func(t *testing.T) {
			host, status := test.NewTestHost(paramLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test?apikey=param-key-456&x=%zz"},
				{":method", "POST"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action,
				"a malformed escape in an unrelated param must not skip the limit_by_param rule")

			host.CallOnRedisCall(0, multiRuleResp([3]int{50, 1, 60}))
			host.CompleteHttp()
		})

		t.Run("malformed escape before the rule param is still limited", func(t *testing.T) {
			host, status := test.NewTestHost(paramLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test?x=%zz&apikey=param-key-456"},
				{":method", "POST"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action,
				"pair ordering must not change rule resolution")

			host.CallOnRedisCall(0, multiRuleResp([3]int{50, 1, 60}))
			host.CompleteHttp()
		})

		t.Run("query without the rule param is not limited", func(t *testing.T) {
			host, status := test.NewTestHost(paramLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test?x=%zz"},
				{":method", "POST"},
			})
			require.Equal(t, types.ActionContinue, action,
				"a query that never carries the rule param must be forwarded")
			host.CompleteHttp()
		})
	})
}
