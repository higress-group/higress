package main

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
)

const duplicateModelRejectBody = `duplicate "model" member in request body`

var endpointMetricsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"lb_type":   "endpoint",
		"lb_policy": "endpoint_metrics",
	})
	return data
}()

// callLoadBalancerRequestBody runs the request through the plugin and returns
// the filter action together with any locally generated response.
func callLoadBalancerRequestBody(t *testing.T, body string) (types.Action, uint32, string) {
	t.Helper()
	host, status := test.NewTestHost(endpointMetricsConfig)
	defer host.Reset()
	if status != types.OnPluginStartStatusOK {
		t.Fatalf("plugin start status = %v, want %v", status, types.OnPluginStartStatusOK)
	}

	host.CallOnHttpRequestHeaders([][2]string{
		{":authority", "example.com"},
		{":path", "/v1/chat/completions"},
		{":method", "POST"},
		{"content-type", "application/json"},
	})

	action := host.CallOnHttpRequestBody([]byte(body))
	if local := host.GetLocalResponse(); local != nil {
		return action, local.StatusCode, string(local.Data)
	}
	return action, 0, ""
}

func assertNotRejected(t *testing.T, body string) {
	t.Helper()
	action, statusCode, data := callLoadBalancerRequestBody(t, body)
	if statusCode != 0 {
		t.Fatalf("body %s was rejected with %d %q, want no local response", body, statusCode, data)
	}
	if action != types.ActionContinue {
		t.Errorf("body %s action = %v, want %v", body, action, types.ActionContinue)
	}
}

func TestDuplicateModelMemberRejected(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		action, statusCode, data := callLoadBalancerRequestBody(t, `{"model":"qwen-max","model":"gpt-4o","messages":[]}`)
		if action != types.ActionPause {
			t.Errorf("action = %v, want %v", action, types.ActionPause)
		}
		if statusCode != 400 {
			t.Errorf("status = %d, want 400", statusCode)
		}
		if data != duplicateModelRejectBody {
			t.Errorf("body = %q, want %q", data, duplicateModelRejectBody)
		}
	})
}

func TestSingleModelMemberUnaffected(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		assertNotRejected(t, `{"model":"qwen-max","messages":[{"role":"user","content":"hi"}]}`)
	})
}

func TestModelWordInsideStringValueNotRejected(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// The count must be structural: "model" spelled inside a string value is
		// prompt text, not a member name.
		assertNotRejected(t, `{"model":"qwen-max","messages":[{"role":"user","content":"repeat \"model\" and \"model\" twice"}]}`)
	})
}

func TestNestedModelMemberNotRejected(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// Only the top level is counted, so a tool definition carrying its own
		// "model" keys stays valid.
		assertNotRejected(t, `{"model":"qwen-max","tools":[{"type":"function","function":{"name":"model","parameters":{"model":"a","model":"b"}}}]}`)
	})
}
