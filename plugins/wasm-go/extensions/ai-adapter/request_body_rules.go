package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime"
	"reflect"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const ctxKeyBodyRules = "ai_adapter_request_body_rules"

type RequestBodyRule struct {
	URI        string              `json:"uri"`
	Method     string              `json:"method"`
	Conditions []BodyRuleCondition `json:"conditions"`
	Set        []BodyRuleSet       `json:"set"`
}

type BodyRuleCondition struct {
	Path   []string        `json:"path"`
	Equals json.RawMessage `json:"equals"`
}

type BodyRuleSet struct {
	Path  []string        `json:"path"`
	Value json.RawMessage `json:"value"`
}

func parseRequestBodyRules(raw gjson.Result) ([]RequestBodyRule, error) {
	if !raw.Exists() {
		return nil, nil
	}
	if !raw.IsArray() {
		return nil, fmt.Errorf("requestBodyRules must be an array")
	}
	var rules []RequestBodyRule
	decoder := json.NewDecoder(strings.NewReader(raw.Raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rules); err != nil {
		return nil, fmt.Errorf("requestBodyRules: %w", err)
	}
	validPath := func(path []string) bool {
		if len(path) == 0 || len(path) > 32 {
			return false
		}
		for _, key := range path {
			if key == "" {
				return false
			}
		}
		return true
	}
	for i := range rules {
		rule := &rules[i]
		if !strings.HasPrefix(rule.URI, "/") || strings.ContainsAny(rule.URI, "?# \t\r\n") {
			return nil, fmt.Errorf("requestBodyRules[%d].uri must be an exact path without query", i)
		}
		switch rule.Method {
		case "":
			rule.Method = "POST"
		case "POST", "PUT", "PATCH", "DELETE", "GET", "HEAD", "OPTIONS":
		default:
			return nil, fmt.Errorf("requestBodyRules[%d].method is invalid", i)
		}
		if len(rule.Conditions) == 0 || len(rule.Set) == 0 {
			return nil, fmt.Errorf("requestBodyRules[%d] requires conditions and set", i)
		}
		for _, condition := range rule.Conditions {
			if !validPath(condition.Path) || !json.Valid(condition.Equals) {
				return nil, fmt.Errorf("requestBodyRules[%d] has invalid condition path or equals", i)
			}
		}
		for _, update := range rule.Set {
			if !validPath(update.Path) || !json.Valid(update.Value) {
				return nil, fmt.Errorf("requestBodyRules[%d] has invalid set path or value", i)
			}
		}
	}
	return rules, nil
}

// 原有头转换仍正常运行，但不能关闭新规则所需的 body 读取。
type bodyRuleContext struct{ wrapper.HttpContext }

func (bodyRuleContext) DontReadRequestBody() {}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config PluginConfig, logger log.Log) types.Action {
	var candidates []RequestBodyRule
	if config.EnableRequestTransform && len(config.RequestBodyRules) > 0 {
		path, _ := proxywasm.GetHttpRequestHeader(":path")
		path, _, _ = strings.Cut(path, "?")
		method, _ := proxywasm.GetHttpRequestHeader(":method")
		contentType, _ := proxywasm.GetHttpRequestHeader("content-type")
		mediaType, _, _ := mime.ParseMediaType(contentType)
		encoding, _ := proxywasm.GetHttpRequestHeader("content-encoding")
		// 不尝试修改压缩或非 JSON 内容。
		if encoding == "" &&
			(mediaType == "application/json" || strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json")) {
			for _, rule := range config.RequestBodyRules {
				if rule.URI == path && rule.Method == method {
					candidates = append(candidates, rule)
				}
			}
		}
	}
	if len(candidates) == 0 {
		return onLegacyRequestHeaders(ctx, config, logger)
	}
	action := onLegacyRequestHeaders(bodyRuleContext{ctx}, config, logger)
	if action != types.ActionContinue && action != types.HeaderStopIteration {
		return action
	}
	ctx.SetContext(ctxKeyBodyRules, candidates)
	ctx.SetRequestBodyBufferLimit(defaultMaxBodyBytes)
	// 必须在头阶段删除旧长度，交由代理重新确定修改后的消息边界。
	_ = proxywasm.RemoveHttpRequestHeader("content-length")
	return types.HeaderStopIteration
}

// 每层 path 都是对象的字面字段名；遇到 JSON 字符串时透明解码，并在写回时恢复字符串类型。
func visitBodyField(raw json.RawMessage, path []string, replacement json.RawMessage) (json.RawMessage, error) {
	if len(path) == 0 {
		if replacement != nil {
			return replacement, nil
		}
		return raw, nil
	}
	trimmed := bytes.TrimSpace(raw)
	quoted := len(trimmed) > 0 && trimmed[0] == '"'
	if quoted {
		var embedded string
		if err := json.Unmarshal(raw, &embedded); err != nil {
			return nil, err
		}
		raw = json.RawMessage(embedded)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("path traverses a non-object or invalid JSON")
	}
	value, exists := object[path[0]]
	if !exists {
		return nil, fmt.Errorf("path does not exist")
	}
	value, err := visitBodyField(value, path[1:], replacement)
	if err != nil || replacement == nil {
		return value, err
	}
	object[path[0]] = value
	encoded, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	if quoted {
		return json.Marshal(string(encoded))
	}
	return encoded, nil
}

func equalBodyValues(a, b json.RawMessage) bool {
	decode := func(raw json.RawMessage) (any, error) {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber() // 避免大整数被 float64 舍入后误匹配。
		var value any
		err := decoder.Decode(&value)
		return value, err
	}
	left, err := decode(a)
	if err != nil {
		return false
	}
	right, err := decode(b)
	return err == nil && reflect.DeepEqual(left, right)
}

func applyRequestBodyRules(body []byte, rules []RequestBodyRule) ([]byte, bool, error) {
	if !json.Valid(body) {
		return body, false, nil
	}
	for _, rule := range rules {
		matched := true
		for _, condition := range rule.Conditions {
			value, err := visitBodyField(body, condition.Path, nil)
			if err != nil || !equalBodyValues(value, condition.Equals) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		// 只执行首条命中规则，所有修改成功后才写回，避免半更新。
		updated := json.RawMessage(body)
		for _, update := range rule.Set {
			var err error
			updated, err = visitBodyField(updated, update.Path, update.Value)
			if err != nil {
				return body, false, err
			}
		}
		return updated, !bytes.Equal(body, updated), nil
	}
	return body, false, nil
}
