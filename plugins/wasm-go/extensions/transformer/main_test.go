// Copyright (c) 2022 Alibaba Group Holding Ltd.
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
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// headersToMap turns mock host headers (sorted [][2]string) into a name -> values map.
func headersToMap(hs [][2]string) map[string][]string {
	out := map[string][]string{}
	for _, h := range hs {
		out[h[0]] = append(out[h[0]], h[1])
	}
	return out
}

// configJSON marshals a config map into a json.RawMessage.
func configJSON(cfg map[string]any) json.RawMessage {
	b, _ := json.Marshal(cfg)
	return b
}

// --- parseConfig ---

func TestParseConfig_RequestRulesOnly(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{
					"operate": "add",
					"headers": []map[string]any{{"key": "X-Add", "value": "v"}},
				},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		cfg, err := host.GetMatchConfig()
		require.NoError(t, err)
		tcfg := cfg.(*TransformerConfig)
		require.True(t, tcfg.reroute, "reroute defaults to true")
		require.NotNil(t, tcfg.reqTrans)
		require.Nil(t, tcfg.respTrans)
	})
}

func TestParseConfig_RerouteFalse(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reroute": false,
			"reqRules": []map[string]any{{
				"operate": "remove",
				"headers": []map[string]any{{"key": "x-y"}},
			}},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		cfg, err := host.GetMatchConfig()
		require.NoError(t, err)
		require.False(t, cfg.(*TransformerConfig).reroute)
	})
}

func TestParseConfig_NoRulesRejected(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// --- Request × headers ---

func TestRequest_Headers_RemoveAndAddAndRename(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "remove", "headers": []map[string]any{{"key": "X-Drop"}}},
				{"operate": "add", "headers": []map[string]any{{"key": "X-New", "value": "v1"}}},
				{"operate": "rename", "headers": []map[string]any{{"oldKey": "X-Old", "newKey": "X-Renamed"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{":method", "GET"},
			{"X-Drop", "gone"},
			{"X-Old", "old-value"},
		})
		require.Equal(t, types.ActionContinue, action)

		got := headersToMap(host.GetRequestHeaders())
		_, hasDrop := got["x-drop"]
		require.False(t, hasDrop, "X-Drop should be removed")
		require.Equal(t, []string{"v1"}, got["x-new"])
		require.Equal(t, []string{"old-value"}, got["x-renamed"])
		_, hasOld := got["x-old"]
		require.False(t, hasOld, "X-Old should be deleted after rename")

		host.CompleteHttp()
	})
}

func TestRequest_Headers_AddSkipsWhenPresent(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "add", "headers": []map[string]any{{"key": "X-K", "value": "fresh"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{"X-K", "existing"},
		})
		require.Equal(t, types.ActionContinue, action)

		got := headersToMap(host.GetRequestHeaders())
		require.Equal(t, []string{"existing"}, got["x-k"], "add must be a no-op when key already exists")
		host.CompleteHttp()
	})
}

func TestRequest_Headers_ReplaceActsAsAddWhenMissing(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "replace", "headers": []map[string]any{{"key": "X-R", "newValue": "v"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
		})
		require.Equal(t, types.ActionContinue, action)

		got := headersToMap(host.GetRequestHeaders())
		require.Equal(t, []string{"v"}, got["x-r"], "replace acts as add when target missing")
		host.CompleteHttp()
	})
}

func TestRequest_Headers_AppendExtendsExisting(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "append", "headers": []map[string]any{{"key": "X-Multi", "appendValue": "added"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{"X-Multi", "first"},
		})
		require.Equal(t, types.ActionContinue, action)

		got := headersToMap(host.GetRequestHeaders())
		require.ElementsMatch(t, []string{"first", "added"}, got["x-multi"])
		host.CompleteHttp()
	})
}

func TestRequest_Headers_DedupeStrategies(t *testing.T) {
	tests := []struct {
		strategy string
		input    []string
		// for SPLIT_*, input has a single value containing commas.
		want []string
	}{
		{"", []string{"a", "b", "a", "c"}, []string{"a"}},                                // default = RETAIN_FIRST
		{"RETAIN_FIRST", []string{"a", "b", "a", "c"}, []string{"a"}},                    // explicit
		{"RETAIN_LAST", []string{"a", "b", "a", "c"}, []string{"c"}},                     // last only
		{"RETAIN_UNIQUE", []string{"a", "b", "a", "c"}, []string{"a", "b", "c"}},         // dedup preserving order
		{"SPLIT_AND_RETAIN_FIRST", []string{"x,y,z"}, []string{"x"}},                     // split first value, keep first part
		{"SPLIT_AND_RETAIN_LAST", []string{"x,y,z"}, []string{"z"}},                      // split first value, keep last part
	}
	for _, tc := range tests {
		t.Run(tc.strategy, func(t *testing.T) {
			test.RunTest(t, func(t *testing.T) {
				headers := [][2]string{{":authority", "test.com"}, {":path", "/p"}}
				for _, v := range tc.input {
					headers = append(headers, [2]string{"X-Dup", v})
				}

				host, status := test.NewTestHost(configJSON(map[string]any{
					"reqRules": []map[string]any{
						{"operate": "dedupe", "headers": []map[string]any{{"key": "X-Dup", "strategy": tc.strategy}}},
					},
				}))
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)
				require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders(headers))

				got := headersToMap(host.GetRequestHeaders())
				require.Equal(t, tc.want, got["x-dup"])
				host.CompleteHttp()
			})
		})
	}
}

// --- Request × querys ---

func TestRequest_Querys_AddRebuildsPath(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "add", "querys": []map[string]any{{"key": "trace", "value": "1"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/api?foo=bar"},
		})
		require.Equal(t, types.ActionContinue, action)

		got := headersToMap(host.GetRequestHeaders())
		path := got[":path"][0]
		require.True(t, strings.HasPrefix(path, "/api?"), "path should retain /api?")
		require.Contains(t, path, "foo=bar")
		require.Contains(t, path, "trace=1")
		host.CompleteHttp()
	})
}

func TestRequest_Querys_Remove(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "remove", "querys": []map[string]any{{"key": "secret"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/api?secret=xxx&keep=yes"},
		}))

		path := headersToMap(host.GetRequestHeaders())[":path"][0]
		require.NotContains(t, path, "secret")
		require.Contains(t, path, "keep=yes")
		host.CompleteHttp()
	})
}

// --- Request × body (JSON) ---

func TestRequest_BodyJson_AddAndRemove(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "add", "body": []map[string]any{{"key": "added", "value": "v"}}},
				{"operate": "remove", "body": []map[string]any{{"key": "drop"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{"content-type", "application/json"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{"drop":"x","keep":"y"}`)))

		body := host.GetRequestBody()
		// pretty.Pretty inserts whitespace, so check JSON content semantically.
		require.NotContains(t, string(body), `"drop"`, "drop field should be removed")
		require.Contains(t, string(body), `"keep"`)
		require.Contains(t, string(body), `"added"`)
		require.Contains(t, string(body), `"v"`)
		host.CompleteHttp()
	})
}

func TestRequest_BodyJson_ValueTypeNumber(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "add", "body": []map[string]any{
					{"key": "limit", "value": "10", "value_type": "number"},
				}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{"content-type", "application/json"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{}`)))

		body := host.GetRequestBody()
		// number type writes 10 not "10"
		require.Contains(t, string(body), `"limit": 10`)
		require.NotContains(t, string(body), `"limit": "10"`)
		host.CompleteHttp()
	})
}

func TestRequest_BodyForm_Urlencoded(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "add", "body": []map[string]any{{"key": "extra", "value": "x"}}},
				{"operate": "remove", "body": []map[string]any{{"key": "drop"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{"content-type", "application/x-www-form-urlencoded"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`drop=1&keep=yes`)))

		body := string(host.GetRequestBody())
		require.NotContains(t, body, "drop=")
		require.Contains(t, body, "keep=yes")
		require.Contains(t, body, "extra=x")
		host.CompleteHttp()
	})
}

func TestRequest_Body_NonStructuredContentTypeSkipped(t *testing.T) {
	// content-type=text/plain → body phase is skipped (DontReadRequestBody);
	// header transform still runs in onHttpRequestHeaders.
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "add", "headers": []map[string]any{{"key": "X-Tag", "value": "y"}}},
				{"operate": "add", "body": []map[string]any{{"key": "should-not-apply", "value": "v"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{"content-type", "text/plain"},
		}))

		got := headersToMap(host.GetRequestHeaders())
		require.Equal(t, []string{"y"}, got["x-tag"], "header transform still applies for unsupported content-type")
		host.CompleteHttp()
	})
}

// --- mapSource = body: header transform delayed to body phase ---

func TestRequest_MapFromBody_DelaysHeaderTransform(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{
					"operate":   "map",
					"mapSource": "body",
					"headers": []map[string]any{
						{"fromKey": "user.id", "toKey": "X-User-Id"},
					},
				},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// In headers phase, we expect Pause (HeaderStopIteration) because the rule needs body data.
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{":method", "POST"},
			{"content-type", "application/json"},
		})
		require.Equal(t, types.ActionPause, action, "mapSource=body should pause header iteration until body arrives")

		// Body phase: header should now be set from body json field.
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{"user":{"id":"alice"}}`)))

		got := headersToMap(host.GetRequestHeaders())
		require.Equal(t, []string{"alice"}, got["x-user-id"])
		host.CompleteHttp()
	})
}

// --- regex template ---

func TestRequest_Headers_AddWithHostPattern(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "add", "headers": []map[string]any{
					{"key": "X-From-Host", "value": "from-$1", "host_pattern": `^(.+)\.example\.com$`},
				}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "shop.example.com"},
			{":path", "/p"},
		}))

		got := headersToMap(host.GetRequestHeaders())
		require.Equal(t, []string{"from-shop"}, got["x-from-host"])
		host.CompleteHttp()
	})
}

func TestRequest_Headers_AddWithPathPattern_NoMatchKeepsValue(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "add", "headers": []map[string]any{
					{"key": "X-V", "value": "literal", "path_pattern": `^/api/v(\d+)/`},
				}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// path does NOT match the pattern → value is the literal "literal"
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/static/css"},
		}))

		got := headersToMap(host.GetRequestHeaders())
		require.Equal(t, []string{"literal"}, got["x-v"])
		host.CompleteHttp()
	})
}

// --- Response × headers + body ---

func TestResponse_Headers_AddAndRemove(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"respRules": []map[string]any{
				{"operate": "add", "headers": []map[string]any{{"key": "X-Out", "value": "o"}}},
				{"operate": "remove", "headers": []map[string]any{{"key": "Server"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"Server", "envoy"},
			{"Content-Type", "text/plain"},
		}))

		got := headersToMap(host.GetResponseHeaders())
		require.Equal(t, []string{"o"}, got["x-out"])
		_, hasServer := got["server"]
		require.False(t, hasServer)
		host.CompleteHttp()
	})
}

func TestResponse_BodyJson_Replace(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"respRules": []map[string]any{
				{"operate": "replace", "body": []map[string]any{
					{"key": "status", "newValue": "filtered"},
				}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"content-type", "application/json"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseBody([]byte(`{"status":"raw","other":1}`)))

		body := string(host.GetResponseBody())
		require.Contains(t, body, `"status": "filtered"`)
		require.Contains(t, body, `"other"`)
		host.CompleteHttp()
	})
}

func TestResponse_NonJsonBodyUnchanged(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"respRules": []map[string]any{
				{"operate": "add", "body": []map[string]any{{"key": "ignored", "value": "x"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"content-type", "text/plain"},
		}))
		// Body callback shouldn't even run with DontReadResponseBody set, but if invoked it must not crash.
		host.CompleteHttp()
	})
}

// --- request without reqRules: body callback returns Continue without changes ---

func TestRequest_NoReqRules_BodyCallbackIsNoop(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"respRules": []map[string]any{
				{"operate": "add", "headers": []map[string]any{{"key": "X-Marker", "value": "1"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
		}))
		// Should be Continue and a no-op since reqRules == nil
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{"x":1}`)))
		host.CompleteHttp()
	})
}

// --- JSON body: full operation matrix ---

func TestRequest_BodyJson_Rename(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "rename", "body": []map[string]any{{"oldKey": "old", "newKey": "fresh"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"}, {"content-type", "application/json"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{"old":"v","keep":1}`)))

		body := string(host.GetRequestBody())
		require.NotContains(t, body, `"old"`)
		require.Contains(t, body, `"fresh"`)
		require.Contains(t, body, `"keep"`)
		host.CompleteHttp()
	})
}

func TestRequest_BodyJson_Replace(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "replace", "body": []map[string]any{{"key": "status", "newValue": "filtered"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"}, {"content-type", "application/json"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{"status":"raw"}`)))

		require.Contains(t, string(host.GetRequestBody()), `"status": "filtered"`)
		host.CompleteHttp()
	})
}

func TestRequest_BodyJson_AppendNewExistingScalarAndArray(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "append", "body": []map[string]any{
					{"key": "fresh", "appendValue": "v1"},
					{"key": "scalar", "appendValue": "v2"},
					{"key": "arr", "appendValue": "v3"},
					{"key": "emptyArr", "appendValue": "v4"},
				}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"}, {"content-type", "application/json"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(
			`{"scalar":"old","arr":["a","b"],"emptyArr":[]}`)))

		body := string(host.GetRequestBody())
		// branch 1: key not present → becomes scalar v1
		require.Contains(t, body, `"fresh": "v1"`)
		// branch 2: old scalar → becomes [old, v2]
		require.Contains(t, body, `"scalar": ["old", "v2"]`)
		// branch 3: existing non-empty array → values appended
		require.Contains(t, body, `"arr": ["a", "b", "v3"]`)
		// branch 4: existing empty array → [v4]
		require.Contains(t, body, `"emptyArr": ["v4"]`)
		host.CompleteHttp()
	})
}

func TestRequest_BodyJson_DedupeStrategies(t *testing.T) {
	// JSON dedupe semantics: RETAIN_FIRST/RETAIN_LAST collapse to a scalar (not array);
	// RETAIN_UNIQUE keeps an array only when more than one unique value remains.
	cases := []struct {
		strategy string
		body     string
		key      string
		contains string
	}{
		{"RETAIN_FIRST", `{"k":["a","b","a"]}`, "k", `"k": "a"`},
		{"RETAIN_LAST", `{"k":["a","b","c"]}`, "k", `"k": "c"`},
		{"RETAIN_UNIQUE", `{"k":["a","b","a","c"]}`, "k", `"k": ["a", "b", "c"]`},
		{"RETAIN_UNIQUE", `{"k":["a","a","a"]}`, "k", `"k": "a"`}, // collapses to scalar when 1 unique
	}
	for _, tc := range cases {
		t.Run(tc.strategy, func(t *testing.T) {
			test.RunTest(t, func(t *testing.T) {
				host, status := test.NewTestHost(configJSON(map[string]any{
					"reqRules": []map[string]any{
						{"operate": "dedupe", "body": []map[string]any{{"key": tc.key, "strategy": tc.strategy}}},
					},
				}))
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
					{":authority", "test.com"}, {":path", "/p"}, {"content-type", "application/json"},
				}))
				require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(tc.body)))
				require.Contains(t, string(host.GetRequestBody()), tc.contains)
				host.CompleteHttp()
			})
		})
	}
}

func TestRequest_BodyJson_ValueTypeBooleanObject(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "add", "body": []map[string]any{
					{"key": "flag", "value": "true", "value_type": "boolean"},
					{"key": "meta", "value": `{"a":1}`, "value_type": "object"},
				}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"}, {"content-type", "application/json"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{}`)))

		body := string(host.GetRequestBody())
		require.Contains(t, body, `"flag": true`)
		require.NotContains(t, body, `"flag": "true"`)
		// object value gets parsed and re-encoded (pretty.Pretty inserts whitespace).
		require.Contains(t, body, `"meta"`)
		require.Contains(t, body, `"a": 1`)
	})
}

func TestRequest_BodyJson_NestedPathAndArrayIndex(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "add", "body": []map[string]any{
					{"key": "user.profile.name", "value": "alice"},
				}},
				{"operate": "replace", "body": []map[string]any{
					{"key": "items.0", "newValue": "zero"},
				}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"}, {"content-type", "application/json"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(
			`{"user":{"profile":{}},"items":["a","b"]}`)))

		body := string(host.GetRequestBody())
		require.Contains(t, body, `"name": "alice"`)
		require.Contains(t, body, `"zero"`)
	})
}

func TestRequest_BodyJson_InvalidJsonReturnsAction(t *testing.T) {
	// Invalid JSON body — handler returns errors.New("invalid json body"); plugin must still
	// release the stream (ActionContinue) and not panic.
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "add", "body": []map[string]any{{"key": "x", "value": "y"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"}, {"content-type", "application/json"},
		}))
		// not valid json — handler returns err, plugin logs warn but does not crash.
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{not-json`)))
		host.CompleteHttp()
	})
}

// --- mapSource matrix on body ---

func TestRequest_MapFromHeaders_ToBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{
					"operate":   "map",
					"mapSource": "headers",
					"body": []map[string]any{
						{"fromKey": "X-Tenant", "toKey": "tenant"},
					},
				},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"}, {"content-type", "application/json"},
			{"X-Tenant", "acme"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{}`)))

		require.Contains(t, string(host.GetRequestBody()), `"tenant"`)
		require.Contains(t, string(host.GetRequestBody()), `"acme"`)
	})
}

func TestRequest_MapFromQuerys_ToHeader(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{
					"operate":   "map",
					"mapSource": "querys",
					"headers": []map[string]any{
						{"fromKey": "trace", "toKey": "X-Trace"},
					},
				},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p?trace=t1"},
		}))
		got := headersToMap(host.GetRequestHeaders())
		require.Equal(t, []string{"t1"}, got["x-trace"])
	})
}

// --- response body JSON op matrix ---

func TestResponse_BodyJson_AddRemoveAppend(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"respRules": []map[string]any{
				{"operate": "add", "body": []map[string]any{{"key": "added", "value": "v"}}},
				{"operate": "remove", "body": []map[string]any{{"key": "drop"}}},
				{"operate": "append", "body": []map[string]any{{"key": "tags", "appendValue": "extra"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"}, {"content-type", "application/json"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseBody([]byte(
			`{"drop":1,"tags":["a"]}`)))

		body := string(host.GetResponseBody())
		require.NotContains(t, body, `"drop"`)
		require.Contains(t, body, `"added"`)
		require.Contains(t, body, `"tags": ["a", "extra"]`)
	})
}

// --- multiple rules: order preserved ---

func TestRequest_MultipleRules_OrderIsPreserved(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "add", "headers": []map[string]any{{"key": "X-Step", "value": "1"}}},
				{"operate": "append", "headers": []map[string]any{{"key": "X-Step", "appendValue": "2"}}},
				{"operate": "rename", "headers": []map[string]any{{"oldKey": "X-Step", "newKey": "X-Final"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"},
		}))
		got := headersToMap(host.GetRequestHeaders())
		require.ElementsMatch(t, []string{"1", "2"}, got["x-final"])
		_, hasOrig := got["x-step"]
		require.False(t, hasOrig)
	})
}

// --- config validation regressions (the 4 bugs we fixed) ---

func TestParseConfig_InvalidOperateRejected(t *testing.T) {
	// Regression for bug at main.go ~712: errors.Wrapf(nil,...) returned nil, silently
	// accepting invalid operate. Must now return a non-nil error and fail start.
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "bogus", "headers": []map[string]any{{"key": "X", "value": "y"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_InvalidMapSourceRejected(t *testing.T) {
	// Regression for bug at main.go ~724: invalid mapSource silently accepted.
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{
					"operate":   "map",
					"mapSource": "bogus",
					"headers":   []map[string]any{{"fromKey": "x", "toKey": "y"}},
				},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_InvalidValueTypeRejected(t *testing.T) {
	// Regression for bug at main.go ~742: invalid value_type silently accepted.
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "add", "body": []map[string]any{
					{"key": "x", "value": "y", "value_type": "bogus"},
				}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// --- delayed-header rewrites driven by body data (need_head_trans pathway) ---

func TestRequest_MapFromBody_DelaysQueryTransform(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{
					"operate":   "map",
					"mapSource": "body",
					"querys": []map[string]any{
						{"fromKey": "tenant", "toKey": "t"},
					},
				},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// Headers phase pauses (need body).
		require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"}, {":method", "POST"},
			{"content-type", "application/json"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(
			`{"tenant":"acme"}`)))

		// Path is rebuilt to include the mapped query param.
		path := headersToMap(host.GetRequestHeaders())[":path"][0]
		require.Contains(t, path, "t=acme")
	})
}

func TestResponse_MapFromBody_DelaysHeaderTransform(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"respRules": []map[string]any{
				{
					"operate":   "map",
					"mapSource": "body",
					"headers": []map[string]any{
						{"fromKey": "status", "toKey": "X-Resp-Status"},
					},
				},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"},
		}))
		// Response headers phase pauses until body arrives.
		require.Equal(t, types.ActionPause, host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"}, {"content-type", "application/json"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseBody([]byte(
			`{"status":"ok"}`)))

		got := headersToMap(host.GetResponseHeaders())
		require.Equal(t, []string{"ok"}, got["x-resp-status"])
	})
}

// --- response form-urlencoded body is not a structured-body content type → skipped ---

func TestResponse_BodyNonJsonSkipped(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"respRules": []map[string]any{
				{"operate": "add", "body": []map[string]any{{"key": "ignored", "value": "x"}}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"},
		}))
		// form-urlencoded isn't supported for response body → DontReadResponseBody.
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"}, {"content-type", "application/x-www-form-urlencoded"},
		}))
		host.CompleteHttp()
	})
}

func TestParseConfig_InvalidRegexRejected(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON(map[string]any{
			"reqRules": []map[string]any{
				{"operate": "add", "headers": []map[string]any{
					{"key": "X", "value": "v", "host_pattern": `([`},
				}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}
