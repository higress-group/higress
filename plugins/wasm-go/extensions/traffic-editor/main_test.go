package main

import (
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/stretchr/testify/require"
)

func TestSample(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("default config only", func(t *testing.T) {
			host, status := test.NewTestHost([]byte(`
{
  "defaultConfig": {
    "commands": [
      {
        "type": "set",
        "target": {
          "type": "request_header",
          "name": "x-test"
        },
        "value": "123456"
      }
    ]
  }
}
				`))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/get"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)

			expectedNewHeaders := [][2]string{
				{":authority", "example.com"},
				{":path", "/get"},
				{":method", "POST"},
				{"x-test", "123456"},
				{"Content-Type", "application/json"},
			}
			newHeaders := host.GetRequestHeaders()
			require.True(t, compareHeaders(expectedNewHeaders, newHeaders), "expected headers: %v, got: %v", expectedNewHeaders, newHeaders)
		})
	})
}

func TestSetMultipleRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost([]byte(`{
	  "defaultConfig": {
	    "commands": [
	      {"type": "set", "target": {"type": "request_header", "name": "x-a"}, "value": "aaa"},
	      {"type": "set", "target": {"type": "request_header", "name": "x-b"}, "value": "bbb"}
	    ]
	  }
	}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		originalHeaders := [][2]string{
			{":authority", "example.com"},
			{":path", "/get"},
			{":method", "POST"},
			{"Content-Type", "application/json"},
			{"x-c", "ccc"},
		}
		action := host.CallOnHttpRequestHeaders(originalHeaders)
		require.Equal(t, types.ActionContinue, action)
		expectedHeaders := [][2]string{
			{":authority", "example.com"},
			{":path", "/get"},
			{":method", "POST"},
			{"Content-Type", "application/json"},
			{"x-a", "aaa"},
			{"x-b", "bbb"},
			{"x-c", "ccc"},
		}
		newHeaders := host.GetRequestHeaders()
		require.True(t, compareHeaders(expectedHeaders, newHeaders))
	})
}

func TestConditionalConfigMatch(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost([]byte(`{
	  "defaultConfig": {
	    "commands": [
	      {"type": "set", "target": {"type": "request_header", "name": "x-def"}, "value": "default"}
	    ]
	  },
	  "conditionalConfigs": [
	    {
	      "conditions": [
	        {"type": "equals", "value1": {"type": "request_header", "name": "x-cond"}, "value2": "match"}
	      ],
	      "commands": [
	        {"type": "set", "target": {"type": "request_header", "name": "x-special"}, "value": "special"}
	      ]
	    }
	  ]
	}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		originalHeaders := [][2]string{
			{":authority", "example.com"},
			{":path", "/data"},
			{":method", "POST"},
			{"Content-Type", "application/json"},
			{"x-cond", "match"},
		}
		action := host.CallOnHttpRequestHeaders(originalHeaders)
		require.Equal(t, types.ActionContinue, action)
		expectedHeaders := [][2]string{
			{":authority", "example.com"},
			{":path", "/data"},
			{":method", "POST"},
			{"Content-Type", "application/json"},
			{"x-cond", "match"},
			{"x-special", "special"},
		}
		newHeaders := host.GetRequestHeaders()
		require.True(t, compareHeaders(expectedHeaders, newHeaders))
	})
}

func TestConditionalConfigNoMatch(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost([]byte(`{
	  "defaultConfig": {
	    "commands": [
	      {"type": "set", "target": {"type": "request_header", "name": "x-def"}, "value": "default"}
	    ]
	  },
	  "conditionalConfigs": [
	    {
	      "conditions": [
	        {"type": "equals", "value1": {"type": "request_header", "name": "x-cond"}, "value2": "match"}
	      ],
	      "commands": [
	        {"type": "set", "target": {"type": "request_header", "name": "x-special"}, "value": "special"}
	      ]
	    }
	  ]
	}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		originalHeaders := [][2]string{
			{":authority", "example.com"},
			{":path", "/get"},
			{":method", "POST"},
			{"Content-Type", "application/json"},
			{"x-cond", "notmatch"},
		}
		action := host.CallOnHttpRequestHeaders(originalHeaders)
		require.Equal(t, types.ActionContinue, action)
		expectedHeaders := [][2]string{
			{":authority", "example.com"},
			{":path", "/get"},
			{":method", "POST"},
			{"Content-Type", "application/json"},
			{"x-cond", "notmatch"},
			{"x-def", "default"},
		}
		newHeaders := host.GetRequestHeaders()
		require.True(t, compareHeaders(expectedHeaders, newHeaders))
	})
}

func TestSetResponseHeader(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost([]byte(`{
	  "defaultConfig": {
	    "commands": [
	      {"type": "set", "target": {"type": "response_header", "name": "x-res"}, "value": "respval"}
	    ]
	  }
	}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		action := host.CallOnHttpResponseHeaders([][2]string{{"x-origin", "originval"}})
		require.Equal(t, types.ActionContinue, action)
		newHeaders := host.GetResponseHeaders()
		require.True(t, compareHeaders([][2]string{{"x-origin", "originval"}, {"x-res", "respval"}}, newHeaders))
	})
}

func TestPathQueryParseAndHeaderChange(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost([]byte(`{
	  "defaultConfig": {
	    "commands": [
	      {"type": "set", "target": {"type": "request_query", "name": "foo"}, "value": "bar"}
	    ]
	  }
	}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":method", "POST"},
			{"Content-Type", "application/json"},
			{":path", "/get?foo=old&baz=1"},
		})
		require.Equal(t, types.ActionContinue, action)
		newHeaders := host.GetRequestHeaders()
		found := false
		for _, h := range newHeaders {
			if h[0] == ":path" && strings.Contains(h[1], "foo=bar") {
				found = true
			}
		}
		require.True(t, found, "path header should be updated with foo=bar")
	})
}

func TestConditionSetMultiStage(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost([]byte(`{
	  "conditionalConfigs": [
	    {
	      "conditions": [
	        {"type": "equals", "value1": {"type": "request_header", "name": "x-a"}, "value2": "aaa"}
	      ],
	      "commands": [
	        {"type": "set", "target": {"type": "response_header", "name": "x-b"}, "value": "bbb"}
	      ]
	    }
	  ]
	}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		actionReq := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":method", "POST"},
			{"Content-Type", "application/json"},
			{":path", "/get?foo=old&baz=1"},
			{"x-a", "aaa"},
		})
		require.Equal(t, types.ActionContinue, actionReq)
		actionResp := host.CallOnHttpResponseHeaders([][2]string{{"content-type", "application/json"}})
		require.Equal(t, types.ActionContinue, actionResp)
		newHeaders := host.GetResponseHeaders()
		require.True(t, compareHeaders([][2]string{{"x-b", "bbb"}, {"content-type", "application/json"}}, newHeaders))
	})
}

func TestConditionSetMultiStage2(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost([]byte(`{
	  "conditionalConfigs": [
	    {
	      "conditions": [
	        {"type": "equals", "value1": {"type": "request_header", "name": "x-a"}, "value2": "aaa"}
	      ],
	      "commands": [
	        {"type": "copy", "source": {"type": "request_header", "name": "x-b"}, "target": {"type": "response_header", "name": "x-c"}}
	      ]
	    }
	  ]
	}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		actionReq := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":method", "POST"},
			{"Content-Type", "application/json"},
			{":path", "/get?foo=old&baz=1"},
			{"x-a", "aaa"},
			{"x-b", "bbb"},
		})
		require.Equal(t, types.ActionContinue, actionReq)
		actionResp := host.CallOnHttpResponseHeaders([][2]string{{"content-type", "application/json"}})
		require.Equal(t, types.ActionContinue, actionResp)
		newHeaders := host.GetResponseHeaders()
		require.True(t, compareHeaders([][2]string{{"x-c", "bbb"}, {"content-type", "application/json"}}, newHeaders))
	})
}

type mockContext struct {
	wrapper.HttpContext
	ctx map[string]interface{}
}

func (m *mockContext) GetContext(key string) interface{} {
	if m.ctx == nil {
		return nil
	}
	return m.ctx[key]
}

func (m *mockContext) Path() string {
	return "/test"
}

func TestOnHttpResponseHeadersNilContext(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost([]byte(`{}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		ctx := &mockContext{}
		require.NotPanics(t, func() {
			action := onHttpResponseHeaders(ctx, PluginConfig{})
			require.Equal(t, types.ActionContinue, action)
		})
	})
}
func compareHeaders(headers1, headers2 [][2]string) bool {
	if len(headers1) != len(headers2) {
		return false
	}
	m1 := make(map[string]string, len(headers1))
	m2 := make(map[string]string, len(headers2))
	for _, h := range headers1 {
		m1[strings.ToLower(h[0])] = h[1]
	}
	for _, h := range headers2 {
		m2[strings.ToLower(h[0])] = h[1]
	}
	if len(m1) != len(m2) {
		return false
	}
	for k, v := range m1 {
		if mv, ok := m2[k]; !ok || mv != v {
			return false
		}
	}
	return true
}
