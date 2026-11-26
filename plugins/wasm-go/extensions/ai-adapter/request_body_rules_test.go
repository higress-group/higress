package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/proxytest"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
)

const liteModel = "doubao-seedream-5-0-lite-260128"
const proModel = "doubao-seedream-5-0-pro-260628"
const taskURI = "/wj-open/v2/open-platform/task/task_create_sync"

func ruleConfig(t *testing.T) ([]byte, PluginConfig) {
	t.Helper()
	raw, err := os.ReadFile("config-seedream-model-rewrite.json")
	if err != nil {
		t.Fatal(err)
	}
	var config PluginConfig
	if err := parseConfig(gjson.ParseBytes(raw), &config, nil); err != nil {
		t.Fatal(err)
	}
	return raw, config
}

func taskBody(encoded bool) []byte {
	param := `{"model":"` + liteModel + `","prompt":"生成一只狗","size":"1728x2304","watermark":false,"id":9007199254740993}`
	if encoded {
		b, _ := json.Marshal(param)
		param = string(b)
	}
	return []byte(`{"taskCode":2511,"id":9007199254740993,"param":` + param + `}`)
}

func TestRequestBodyRules(t *testing.T) {
	_, config := ruleConfig(t)
	for _, encoded := range []bool{false, true} {
		t.Run(strconv.FormatBool(encoded), func(t *testing.T) {
			body := taskBody(encoded)
			updated, changed, err := applyRequestBodyRules(body, config.RequestBodyRules)
			if err != nil || !changed {
				t.Fatalf("rewrite failed: changed=%v err=%v", changed, err)
			}
			var before, after map[string]json.RawMessage
			_ = json.Unmarshal(body, &before)
			_ = json.Unmarshal(updated, &after)
			if !bytes.Equal(before["id"], after["id"]) || !bytes.Equal(before["taskCode"], after["taskCode"]) {
				t.Fatal("unrelated top-level fields changed")
			}
			var param []byte = after["param"]
			if encoded {
				var value string
				if err := json.Unmarshal(param, &value); err != nil {
					t.Fatal("param must remain a JSON string", err)
				}
				param = []byte(value)
			}
			if !gjson.ValidBytes(param) || gjson.GetBytes(param, "model").String() != proModel ||
				gjson.GetBytes(param, "id").Raw != "9007199254740993" ||
				gjson.GetBytes(param, "prompt").String() != "生成一只狗" ||
				gjson.GetBytes(param, "size").String() != "1728x2304" ||
				gjson.GetBytes(param, "watermark").Raw != "false" {
				t.Fatalf("unexpected rewritten param: %s", param)
			}
		})
	}
	for _, body := range []string{
		strings.Replace(string(taskBody(true)), "2511", "2512", 1),
		strings.Replace(string(taskBody(true)), "2511", `"2511"`, 1),
		strings.Replace(string(taskBody(true)), liteModel, proModel, 1),
		`{"taskCode":2511,"param":"invalid"}`, `{"taskCode":2511,"param":null}`,
		`{"taskCode":2511,"param":{}}`, `{"param":{"model":"` + liteModel + `"}}`,
		`{"taskCode":2511`, `null`, `[]`,
	} {
		updated, changed, err := applyRequestBodyRules([]byte(body), config.RequestBodyRules)
		if err != nil || changed || string(updated) != body {
			t.Fatalf("nonmatching body changed: %s (%v)", body, err)
		}
	}
	// 所有条件均来自原始 body；首条命中后不继续匹配下一条。
	rules := append([]RequestBodyRule{}, config.RequestBodyRules...)
	next := rules[0]
	next.Conditions = []BodyRuleCondition{{Path: []string{"param", "model"}, Equals: json.RawMessage(`"` + proModel + `"`)}}
	next.Set = []BodyRuleSet{{Path: []string{"param", "model"}, Value: json.RawMessage(`"unexpected"`)}}
	rules = append(rules, next)
	updated, _, err := applyRequestBodyRules(taskBody(true), rules)
	if err != nil || strings.Contains(string(updated), "unexpected") {
		t.Fatal("rules must not cascade", err)
	}
	// 第二个写入失败时，第一个写入也不生效。
	rules[0].Set = append(append([]BodyRuleSet{}, rules[0].Set...), BodyRuleSet{Path: []string{"missing"}, Value: json.RawMessage(`1`)})
	body := taskBody(true)
	updated, changed, err := applyRequestBodyRules(body, rules)
	if err == nil || changed || !bytes.Equal(updated, body) {
		t.Fatal("partial update escaped")
	}
	// taskCode 也可以按配置放在 param 内部，字段名中的点按字面处理。
	nested, _ := parseRequestBodyRules(gjson.Parse(`[{"uri":"/task","conditions":[{"path":["param","taskCode"],"equals":2511}],"set":[{"path":["param","a.b"],"value":"new"}]}]`))
	updated, changed, err = applyRequestBodyRules([]byte(`{"param":"{\"taskCode\":2511,\"a.b\":\"old\"}"}`), nested)
	if err != nil || !changed || !strings.Contains(string(updated), `\"a.b\":\"new\"`) {
		t.Fatalf("nested literal-key rewrite failed: %s %v", updated, err)
	}
}

func TestRequestBodyRuleConfigValidation(t *testing.T) {
	raw, _ := ruleConfig(t)
	for _, invalid := range []string{
		`{"requestBodyRules":{}}`, `{"requestBodyRules":null}`,
		strings.Replace(string(raw), taskURI, taskURI+"?x=1", 1),
		strings.Replace(string(raw), `"POST"`, `"post"`, 1),
		strings.Replace(string(raw), `"equals": 2511`, `"equalsTypo": 2511`, 1),
		strings.Replace(string(raw), `"path": ["taskCode"]`, `"path": []`, 1),
		strings.Replace(string(raw), `"conditions": [`, `"conditions": null, "unused": [`, 1),
		`{"requestBodyRules":[{"uri":"/task","conditions":[],"set":[]}]}`,
	} {
		var config PluginConfig
		if err := parseConfig(gjson.Parse(invalid), &config, nil); err == nil {
			t.Fatalf("invalid configuration accepted: %s", invalid)
		}
	}
}

func TestRequestBodyRulesHost(t *testing.T) {
	raw, _ := ruleConfig(t)
	vm := proxywasm.GetVMContext()
	t.Cleanup(func() { proxywasm.SetVMContext(vm) })
	for _, tc := range []struct {
		name, path, method, contentType, encoding    string
		disabled, legacy, mismatch, chunked, noRules bool
	}{
		{name: "no model or provider properties"},
		{name: "query ignored", path: taskURI + "?trace=1"},
		{name: "chunked input", chunked: true},
		{name: "different URI", path: "/other"},
		{name: "exact URI only", path: taskURI + "/"},
		{name: "different method", method: "PUT"},
		{name: "non JSON", contentType: "text/plain"},
		{name: "compressed", encoding: "gzip"},
		{name: "disabled", disabled: true},
		{name: "task mismatch", mismatch: true},
		{name: "legacy transform with rule", legacy: true},
		{name: "legacy transform after rule mismatch", legacy: true, mismatch: true},
		{name: "legacy config without rules", legacy: true, noRules: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := string(raw)
			if tc.disabled {
				config = strings.Replace(config, `"enableRequestTransform": true`, `"enableRequestTransform": false`, 1)
			}
			if tc.legacy {
				config = strings.Replace(config, "{", `{"modelProviderMappings":[{"model":"legacy-model","provider":"*","requestTransform":{"type":"body_transform","bodyTransform":{"setValues":{"legacyApplied":true}}}}],`, 1)
			}
			if tc.noRules {
				var object map[string]json.RawMessage
				_ = json.Unmarshal([]byte(config), &object)
				delete(object, "requestBodyRules")
				b, _ := json.Marshal(object)
				config = string(b)
			}
			host, reset := proxytest.NewHostEmulator(proxytest.NewEmulatorOption().WithVMContext(vm).WithPluginConfiguration([]byte(config)))
			defer reset()
			host.RegisterForeignFunction("get_log_level", func([]byte) []byte { return []byte{0, 0, 0, 0} })
			if host.StartPlugin() != types.OnPluginStartStatusOK {
				t.Fatal("plugin start failed")
			}
			id := host.InitializeHttpContext()
			if tc.legacy {
				_ = host.SetProperty([]string{"wasm.requestModel"}, []byte("legacy-model"))
				_ = host.SetProperty([]string{"cluster_name"}, []byte("outbound|443||llm-Test.internal.dns"))
			}
			body := taskBody(true)
			if tc.mismatch {
				body = bytes.Replace(body, []byte("2511"), []byte("2512"), 1)
			}
			path, method, contentType := tc.path, tc.method, tc.contentType
			if path == "" {
				path = taskURI
			}
			if method == "" {
				method = "POST"
			}
			if contentType == "" {
				contentType = "application/json; charset=utf-8"
			}
			headers := [][2]string{{":path", path}, {":method", method}, {":authority", "test.local"}, {"content-type", contentType}, {"content-length", strconv.Itoa(len(body))}}
			if tc.encoding != "" {
				headers = append(headers, [2]string{"content-encoding", tc.encoding})
			}
			action := host.CallOnRequestHeaders(id, headers, false)
			candidate := (path == taskURI || strings.HasPrefix(path, taskURI+"?")) && method == "POST" && tc.contentType == "" && tc.encoding == "" && !tc.disabled && !tc.noRules
			if candidate && action != types.HeaderStopIteration {
				t.Fatalf("body not buffered: %v", action)
			}
			if candidate {
				for _, header := range host.GetCurrentRequestHeaders(id) {
					if header[0] == "content-length" {
						t.Fatal("stale content-length retained")
					}
				}
			}
			if tc.chunked {
				mid := len(body) / 2
				if host.CallOnRequestBody(id, body[:mid], false) != types.ActionPause {
					t.Fatal("partial body not buffered")
				}
				host.CallOnRequestBody(id, body[mid:], true)
			} else {
				host.CallOnRequestBody(id, body, true)
			}
			actual := host.GetCurrentRequestBody(id)
			wantRewrite := candidate && !tc.mismatch
			if strings.Contains(string(actual), proModel) != wantRewrite {
				t.Fatalf("unexpected rewrite result: %s", actual)
			}
			if tc.legacy && !gjson.GetBytes(actual, "legacyApplied").Bool() {
				t.Fatal("legacy transformation was suppressed")
			}
			if !wantRewrite && !tc.legacy && !bytes.Equal(actual, body) {
				t.Fatal("unmatched request body changed")
			}
			host.CompleteHttpContext(id)
		})
	}
}

// 复现生产配置：旧模型映射和新任务规则共用一个 WasmPlugin。
func TestRequestBodyRuleRouteIsolation(t *testing.T) {
	raw, _ := ruleConfig(t)
	var taskConfig map[string]any
	if err := json.Unmarshal(raw, &taskConfig); err != nil {
		t.Fatal(err)
	}
	taskConfig["_match_route_"] = []string{"task-route"}
	legacyConfig := map[string]any{
		"_match_route_":          []string{"legacy-route"},
		"enableRequestTransform": true,
		"modelProviderMappings": []any{map[string]any{
			"model": "legacy-model", "provider": "Test",
			"requestTransform": map[string]any{"type": "body_transform", "bodyTransform": map[string]any{"setValues": map[string]any{"legacyApplied": true}}},
		}},
	}
	config, _ := json.Marshal(map[string]any{"_rules_": []any{legacyConfig, taskConfig}})
	vm := proxywasm.GetVMContext()
	host, reset := proxytest.NewHostEmulator(proxytest.NewEmulatorOption().WithVMContext(vm).WithPluginConfiguration(config))
	defer func() { reset(); proxywasm.SetVMContext(vm) }()
	host.RegisterForeignFunction("get_log_level", func([]byte) []byte { return []byte{0, 0, 0, 0} })
	if host.StartPlugin() != types.OnPluginStartStatusOK {
		t.Fatal("plugin start failed")
	}
	for _, route := range []string{"legacy-route", "task-route", "legacy-route"} {
		id := host.InitializeHttpContext()
		_ = host.SetProperty([]string{"route_name"}, []byte(route))
		_ = host.SetProperty([]string{"wasm.requestModel"}, []byte("legacy-model"))
		_ = host.SetProperty([]string{"cluster_name"}, []byte("outbound|443||llm-Test.internal.dns"))
		host.CallOnRequestHeaders(id, [][2]string{{":path", taskURI}, {":method", "POST"}, {":authority", "test.local"}, {"content-type", "application/json"}}, false)
		host.CallOnRequestBody(id, taskBody(true), true)
		actual := host.GetCurrentRequestBody(id)
		if route == "legacy-route" {
			if !gjson.GetBytes(actual, "legacyApplied").Bool() || strings.Contains(string(actual), proModel) {
				t.Fatalf("legacy route configuration lost: %s", actual)
			}
		} else if !strings.Contains(string(actual), proModel) || gjson.GetBytes(actual, "legacyApplied").Exists() {
			t.Fatalf("task route used another route's configuration: %s", actual)
		}
		host.CompleteHttpContext(id)
	}
}
