// Copyright (c) 2024 Alibaba Group Holding Ltd.
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
	"encoding/json"
	"net/http"
	"testing"

	"ext-auth/config"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// successConditionConfig requires the authorization response body to carry data.code == "0".
var successConditionConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  "/auth",
			},
			"timeout": 1000,
			"success_condition": []map[string]interface{}{
				{"source": "body_json", "key": "data.code", "op": "eq", "value": "0"},
			},
		},
	})
	return data
}()

func TestCompareNumeric(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  int
	}{
		{"less", "1", "2", -1},
		{"greater", "3", "2", 1},
		{"equal", "2", "2", 0},
		{"floats", "2.5", "2.4999", 1},
		{"whitespace trimmed", " 7 ", "3", 1},
		{"left non-numeric", "abc", "2", 0},
		{"right non-numeric", "2", "xyz", 0},
		{"both non-numeric", "a", "b", 0},
		{"empty left", "", "1", 0},
		{"negative", "-5", "-3", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, compareNumeric(tt.left, tt.right))
		})
	}
}

func TestMatchCondition(t *testing.T) {
	headers := http.Header{}
	headers.Set("x-role", "admin")
	headers.Set("x-empty", "")
	body := []byte(`{"data": {"code": "0", "count": 42}, "ok": true, "nil": null}`)

	tests := []struct {
		name      string
		condition config.Condition
		want      bool
	}{
		{"status_code eq match", config.Condition{Source: "status_code", Op: "eq", Value: []string{"200"}}, true},
		{"status_code eq mismatch", config.Condition{Source: "status_code", Op: "eq", Value: []string{"403"}}, false},
		{"status_code gt", config.Condition{Source: "status_code", Op: "gt", Value: []string{"199"}}, true},
		{"header eq case-insensitive key", config.Condition{Source: "header", Key: "X-Role", Op: "eq", Value: []string{"admin"}}, true},
		{"header in match", config.Condition{Source: "header", Key: "x-role", Op: "in", Value: []string{"admin", "owner"}}, true},
		{"header in case-sensitive mismatch", config.Condition{Source: "header", Key: "x-role", Op: "in", Value: []string{"ADMIN"}}, false},
		{"header not_in match", config.Condition{Source: "header", Key: "x-role", Op: "not_in", Value: []string{"guest"}}, true},
		{"header ne match", config.Condition{Source: "header", Key: "x-role", Op: "ne", Value: []string{"guest"}}, true},
		{"header missing value op", config.Condition{Source: "header", Key: "x-absent", Op: "eq", Value: []string{"x"}}, false},
		{"header exists missing", config.Condition{Source: "header", Key: "x-absent", Op: "exists"}, false},
		{"header not_exists missing", config.Condition{Source: "header", Key: "x-absent", Op: "not_exists"}, true},
		{"header present-but-empty not extracted", config.Condition{Source: "header", Key: "x-empty", Op: "exists"}, false},
		{"body_json eq match", config.Condition{Source: "body_json", Key: "data.code", Op: "eq", Value: []string{"0"}}, true},
		{"body_json eq mismatch", config.Condition{Source: "body_json", Key: "data.code", Op: "eq", Value: []string{"1"}}, false},
		{"body_json ne match", config.Condition{Source: "body_json", Key: "data.code", Op: "ne", Value: []string{"1"}}, true},
		{"body_json number gt", config.Condition{Source: "body_json", Key: "data.count", Op: "gt", Value: []string{"41"}}, true},
		{"body_json number lt", config.Condition{Source: "body_json", Key: "data.count", Op: "lt", Value: []string{"43"}}, true},
		{"body_json bool eq", config.Condition{Source: "body_json", Key: "ok", Op: "eq", Value: []string{"true"}}, true},
		{"body_json missing path not_exists", config.Condition{Source: "body_json", Key: "data.absent", Op: "not_exists"}, true},
		{"body_json missing path eq", config.Condition{Source: "body_json", Key: "data.absent", Op: "eq", Value: []string{"x"}}, false},
		{"body_json explicit null exists", config.Condition{Source: "body_json", Key: "nil", Op: "exists"}, true},
		{"body_json explicit null eq empty", config.Condition{Source: "body_json", Key: "nil", Op: "eq", Value: []string{""}}, true},
		{"unknown op returns false", config.Condition{Source: "status_code", Op: "bogus", Value: []string{"x"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, matchCondition(tt.condition, 200, headers, body))
		})
	}
}

func TestEvaluateSuccessCondition(t *testing.T) {
	headers := http.Header{}
	body := []byte(`{"data": {"code": "0"}}`)

	require.True(t, evaluateSuccessCondition(nil, 200, headers, body), "nil list is trivially satisfied")
	require.True(t, evaluateSuccessCondition([]config.Condition{}, 200, headers, body), "empty list is trivially satisfied")

	allSatisfied := []config.Condition{
		{Source: "status_code", Op: "eq", Value: []string{"200"}},
		{Source: "body_json", Key: "data.code", Op: "eq", Value: []string{"0"}},
	}
	require.True(t, evaluateSuccessCondition(allSatisfied, 200, headers, body))

	oneUnsatisfied := []config.Condition{
		{Source: "status_code", Op: "eq", Value: []string{"200"}},
		{Source: "body_json", Key: "data.code", Op: "eq", Value: []string{"1"}},
	}
	require.False(t, evaluateSuccessCondition(oneUnsatisfied, 200, headers, body), "AND fails when any condition is unsatisfied")
}

func TestSuccessConditionGating(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("condition satisfied allows request", func(t *testing.T) {
			host, status := test.NewTestHost(successConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{"data": {"code": "0"}}`))

			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())
			host.CompleteHttp()
		})

		t.Run("condition unsatisfied rejects with status_on_error", func(t *testing.T) {
			host, status := test.NewTestHost(successConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// auth server returns 200 but the body indicates a failure
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{"data": {"code": "500"}}`))

			resp := host.GetLocalResponse()
			require.NotNil(t, resp)
			require.Equal(t, uint32(403), resp.StatusCode)
			require.Equal(t, "ext-auth.denied-by-condition", resp.StatusCodeDetail)
			host.CompleteHttp()
		})

		t.Run("non-200 still routes to error handler", func(t *testing.T) {
			host, status := test.NewTestHost(successConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			host.CallOnHttpCall([][2]string{
				{":status", "401"},
				{"content-type", "application/json"},
			}, []byte(`{"data": {"code": "0"}}`))

			resp := host.GetLocalResponse()
			require.NotNil(t, resp)
			require.Equal(t, uint32(401), resp.StatusCode)
			require.Equal(t, "ext-auth.unauthorized", resp.StatusCodeDetail)
			host.CompleteHttp()
		})
	})
}

// successConditionWithClientHeadersConfig combines a success_condition gate with
// allowed_client_headers so the rejected response carries only the allowed header.
var successConditionWithClientHeadersConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  "/auth",
			},
			"timeout":         1000,
			"status_on_error": 403,
			"success_condition": []map[string]interface{}{
				{"source": "body_json", "key": "data.code", "op": "eq", "value": "0"},
			},
			"authorization_response": map[string]interface{}{
				"allowed_client_headers": []map[string]interface{}{
					{"exact": "x-auth-failed"},
				},
			},
		},
	})
	return data
}()

func TestSuccessConditionRejectFiltersClientHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(successConditionWithClientHeadersConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/users"},
			{":method", "POST"},
		})
		require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

		// 200 but condition fails; response carries an allowed and a disallowed header.
		host.CallOnHttpCall([][2]string{
			{":status", "200"},
			{"x-auth-failed", "true"},
			{"x-secret", "hidden"},
			{"content-type", "application/json"},
		}, []byte(`{"data": {"code": "500"}}`))

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(403), resp.StatusCode)
		require.Equal(t, "ext-auth.denied-by-condition", resp.StatusCodeDetail)

		got := headerMap(resp.Headers)
		require.Equal(t, "true", got["x-auth-failed"], "allowed client header is forwarded on rejection")
		require.NotContains(t, got, "x-secret", "header not in allowed_client_headers must be dropped")

		host.CompleteHttp()
	})
}
