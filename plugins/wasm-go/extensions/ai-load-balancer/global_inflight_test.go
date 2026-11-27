package main

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/cluster_metrics"
)

var adaptiveScoreGlobalInflightConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"lb_type":   "cluster",
		"lb_policy": "cluster_metrics",
		"lb_config": map[string]interface{}{
			"mode":                    "AdaptiveScore",
			"p2c_choices":             2,
			"service_list":            []string{"svc-a", "svc-b"},
			"global_inflight_enabled": true,
			"serviceFQDN":             "redis.static",
			"servicePort":             6379,
			"global_inflight_timeout": 100,
		},
	})
	return data
}()

func TestGlobalInflightAsyncDispatchResumesRequest(t *testing.T) {
	testCases := []struct {
		name          string
		status        int32
		response      []byte
		wantCandidate string
		rollback      bool
	}{
		{
			name:          "redis success",
			response:      test.CreateRedisRespArray([]interface{}{"svc-a", 1, 1.0}),
			wantCandidate: "svc-a",
		},
		{
			name:     "redis error",
			response: test.CreateRedisRespError("redis unavailable"),
		},
		{
			name:   "redis timeout",
			status: 1,
		},
		{
			name:     "empty result",
			response: test.CreateRedisRespArray(nil),
		},
		{
			name:     "invalid result",
			response: test.CreateRedisRespString("unexpected"),
		},
		{
			name:     "empty candidate",
			response: test.CreateRedisRespArray([]interface{}{"", 1, 1.0}),
		},
		{
			name:     "unknown candidate",
			response: test.CreateRedisRespArray([]interface{}{"svc-unknown", 1, 1.0}),
			rollback: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			test.RunGoTest(t, func(t *testing.T) {
				host, status := test.NewTestHost(adaptiveScoreGlobalInflightConfig)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)
				require.NoError(t, host.SetRouteName("route-a"))

				action := host.CallOnHttpRequestHeaders([][2]string{
					{":authority", "example.com"},
					{":path", "/v1/chat/completions"},
					{":method", "POST"},
				})
				require.Equal(t, types.HeaderStopAllIterationAndWatermark, action,
					"async global inflight dispatch must not permanently pause the request")

				host.CallOnRedisCall(tc.status, tc.response)
				if tc.rollback {
					// The unknown candidate is rolled back with HINCRBY before the
					// callback resumes the request.
					host.CallOnRedisCall(0, test.CreateRedisRespInt(1))
				}

				require.Equal(t, types.ActionContinue, host.GetHttpStreamAction(),
					"request must resume after the async callback")

				lb := loadGlobalInflightLB(t, host)
				selected, ongoing := selectedService(lb)
				require.Equal(t, 1, ongoing, "exactly one request should be in flight")
				if tc.wantCandidate != "" {
					require.Equal(t, tc.wantCandidate, selected)
				}
				require.Equal(t, selected, requestHeaderValue(host.GetRequestHeaders(), lb.ClusterHeader),
					"selected service header must match the in-flight service")

				host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}})
				host.CallOnHttpStreamingResponseBody([]byte("chunk"), true)
				host.CompleteHttp()

				_, ongoing = selectedService(lb)
				require.Zero(t, ongoing, "in-flight counters must not leak after the stream completes")
				require.Equal(t, 1, totalRequestCount(lb), "request counters must not leak after the stream completes")
			})
		})
	}
}

func loadGlobalInflightLB(t *testing.T, host test.TestHost) cluster_metrics.ClusterEndpointLoadBalancer {
	t.Helper()
	rawConfig, err := host.GetMatchConfig()
	require.NoError(t, err)
	config, ok := rawConfig.(*Config)
	require.True(t, ok)
	lb, ok := config.lb.(cluster_metrics.ClusterEndpointLoadBalancer)
	require.True(t, ok)
	return lb
}

func selectedService(lb cluster_metrics.ClusterEndpointLoadBalancer) (string, int) {
	selected := ""
	ongoing := 0
	for _, svc := range lb.ServiceList {
		if lb.ServiceRequestOngoing[svc] > 0 {
			selected = svc
		}
		ongoing += lb.ServiceRequestOngoing[svc]
	}
	return selected, ongoing
}

func totalRequestCount(lb cluster_metrics.ClusterEndpointLoadBalancer) int {
	total := 0
	for _, svc := range lb.ServiceList {
		total += lb.ServiceRequestCount[svc]
	}
	return total
}

func requestHeaderValue(headers [][2]string, key string) string {
	for _, header := range headers {
		if header[0] == key {
			return header[1]
		}
	}
	return ""
}
