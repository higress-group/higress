// Copyright (c) 2024-2025
// Beijing Baolande Software Corporation
// All Rights Reserved

package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var userQuotaTestConfig = json.RawMessage(`{
	"redis": {"service_name": "redis.static", "service_port": 6379},
	"skip_route_prefixes": []
}`)

func newMeteredResponseHost(t *testing.T, responseStatus string) test.TestHost {
	t.Helper()
	host, status := test.NewTestHost(userQuotaTestConfig)
	require.Equal(t, types.OnPluginStartStatusOK, status)

	action := host.CallOnHttpRequestHeaders([][2]string{
		{":authority", "example.com"},
		{":path", "/v1/chat/completions"},
		{":method", "POST"},
		{"x-mse-consumer", "u-1"},
	})
	require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)
	host.CallOnRedisCall(0, test.CreateRedisRespArray([]interface{}{10000, "2026-01-01", 2}))
	require.Len(t, host.GetRedisCalloutAttributes(), 1)
	host.CallOnRedisCall(0, test.CreateRedisRespInt(0))
	require.Empty(t, host.GetRedisCalloutAttributes())

	action = host.CallOnHttpResponseHeaders([][2]string{{":status", responseStatus}})
	require.Equal(t, types.ActionContinue, action)
	return host
}

func TestOnStreamingResponseBodyMarksDirtyWithoutIncrResponse(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cases := []struct {
			name string
			body []byte
		}{
			{
				name: "non-streaming response",
				body: []byte(`{"choices":[],"usage":{"prompt_tokens":13,"completion_tokens":8,"total_tokens":21}}`),
			},
			{
				name: "streaming response",
				body: []byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":13,\"completion_tokens\":8,\"total_tokens\":21}}\n\n"),
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				host := newMeteredResponseHost(t, "200")
				defer host.Reset()

				action := host.CallOnHttpStreamingResponseBody(tc.body, true)
				require.Equal(t, types.ActionContinue, action)

				attrs := host.GetRedisCalloutAttributes()
				require.Len(t, attrs, 2)
				incrQuery := string(attrs[0].Query)
				require.Contains(t, incrQuery, "INCRBY")
				usedKeyPattern := regexp.MustCompile(`user_token_quota:\{u-1\}:used:2:\d{4}-\d{2}-\d{2}`)
				actualUsedKey := usedKeyPattern.FindString(incrQuery)
				require.NotEmpty(t, actualUsedKey)
				cycleStart, err := time.Parse("2006-01-02", actualUsedKey[len("user_token_quota:{u-1}:used:2:"):])
				require.NoError(t, err)
				saddQuery := string(attrs[1].Query)
				require.Contains(t, strings.ToUpper(saddQuery), "SADD")
				require.Contains(t, saddQuery, dirtyCurrentKey())
				require.Contains(t, saddQuery, dirtyMember("u-1", 2, cycleStart))
				host.CompleteHttp()
			})
		}
	})
}

func TestOnStreamingResponseBodySkipsDirtyForInvalidUsage(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cases := []struct {
			name           string
			responseStatus string
			body           []byte
		}{
			{
				name:           "zero token usage",
				responseStatus: "200",
				body:           []byte(`{"choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`),
			},
			{
				name:           "non-200 response",
				responseStatus: "500",
				body:           []byte(`{"choices":[],"usage":{"prompt_tokens":13,"completion_tokens":8,"total_tokens":21}}`),
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				host := newMeteredResponseHost(t, tc.responseStatus)
				defer host.Reset()

				action := host.CallOnHttpStreamingResponseBody(tc.body, true)
				require.Equal(t, types.ActionContinue, action)
				require.Empty(t, host.GetRedisCalloutAttributes())
				host.CompleteHttp()
			})
		}
	})
}
