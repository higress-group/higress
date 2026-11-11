package main

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
)

const clusterHashTarget = "outbound|443||llm-a.internal.dns"

func clusterHashConfig(key map[string]interface{}) json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"lb_type":   "cluster",
		"lb_policy": "cluster_hash",
		"lb_config": map[string]interface{}{
			"cluster_header": "x-test-target-cluster",
			"key":            key,
			"clusters": []map[string]interface{}{
				{"cluster": clusterHashTarget, "weight": 100},
			},
		},
	})
	return data
}

func TestClusterHashRuntime(t *testing.T) {
	wasmtest.RunTest(t, func(t *testing.T) {
		t.Run("routes by header key", func(t *testing.T) {
			host, status := wasmtest.NewTestHost(clusterHashConfig(map[string]interface{}{
				"source": "header",
				"name":   "x-session-id",
			}))
			defer host.Reset()
			requireEqual(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-session-id", "session-a"},
			})
			requireEqual(t, types.ActionContinue, action)
			requireTargetCluster(t, host)
		})

		t.Run("routes by cookie key", func(t *testing.T) {
			host, status := wasmtest.NewTestHost(clusterHashConfig(map[string]interface{}{
				"source": "cookie",
				"name":   "llm_session",
			}))
			defer host.Reset()
			requireEqual(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"cookie", "theme=dark; llm_session=session-b"},
			})
			requireEqual(t, types.ActionContinue, action)
			requireTargetCluster(t, host)
		})

		t.Run("routes by body key", func(t *testing.T) {
			host, status := wasmtest.NewTestHost(clusterHashConfig(map[string]interface{}{
				"source":   "body",
				"jsonPath": "$.callOptions.stickySessionId",
			}))
			defer host.Reset()
			requireEqual(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"content-length", "51"},
			})
			requireEqual(t, types.HeaderStopIteration, action)

			action = host.CallOnHttpRequestBody([]byte(`{"callOptions":{"stickySessionId":"session-c"}}`))
			requireEqual(t, types.ActionContinue, action)
			requireTargetCluster(t, host)
		})

		t.Run("rejects body key when post request has no body indicators", func(t *testing.T) {
			host, status := wasmtest.NewTestHost(clusterHashConfig(map[string]interface{}{
				"source":   "body",
				"jsonPath": "$.callOptions.stickySessionId",
			}))
			defer host.Reset()
			requireEqual(t, types.OnPluginStartStatusOK, status)

			contextID := host.InitializeHttpContext()
			action := host.CallOnRequestHeaders(contextID, [][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			}, true)
			requireEqual(t, types.ActionPause, action)
			localResponse := host.GetSentLocalResponse(contextID)
			if localResponse == nil {
				t.Fatal("expected local response")
			}
			requireEqual(t, uint32(403), localResponse.StatusCode)
			if string(localResponse.Data) != "hash key required" {
				t.Fatalf("local response body = %q, want hash key required", string(localResponse.Data))
			}
		})

		t.Run("routes by metadata key", func(t *testing.T) {
			host, status := wasmtest.NewTestHost(clusterHashConfig(map[string]interface{}{
				"source":       "metadata",
				"propertyPath": []string{"metadata", "sticky_session"},
			}))
			defer host.Reset()
			requireEqual(t, types.OnPluginStartStatusOK, status)
			err := host.SetProperty([]string{"metadata", "sticky_session"}, []byte("session-d"))
			if err != nil {
				t.Fatalf("SetProperty() error = %v", err)
			}

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})
			requireEqual(t, types.ActionContinue, action)
			requireTargetCluster(t, host)
		})

		t.Run("rejects missing key", func(t *testing.T) {
			host, status := wasmtest.NewTestHost(clusterHashConfig(map[string]interface{}{
				"source": "header",
				"name":   "x-session-id",
			}))
			defer host.Reset()
			requireEqual(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})
			requireEqual(t, types.ActionPause, action)
			localResponse := host.GetLocalResponse()
			if localResponse == nil {
				t.Fatal("expected local response")
			}
			requireEqual(t, uint32(403), localResponse.StatusCode)
			if string(localResponse.Data) != "hash key required" {
				t.Fatalf("local response body = %q, want hash key required", string(localResponse.Data))
			}
		})
	})
}

func requireTargetCluster(t *testing.T, host wasmtest.TestHost) {
	t.Helper()
	target, ok := wasmtest.GetHeaderValue(host.GetRequestHeaders(), "x-test-target-cluster")
	if !ok {
		t.Fatal("target cluster header should exist")
	}
	requireEqual(t, clusterHashTarget, target)
	if localResponse := host.GetLocalResponse(); localResponse != nil {
		t.Fatalf("unexpected local response: %+v", localResponse)
	}
}

func requireEqual[T comparable](t *testing.T, want, got T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}
