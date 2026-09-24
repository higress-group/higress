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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"custom-response",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
	)
}

type CustomResponseConfig struct {
	rules                 []CustomResponseRule
	defaultRule           *CustomResponseRule
	enableOnStatusRuleMap map[string]*CustomResponseRule
}

type CustomResponseRule struct {
	statusCode     uint32
	headers        [][2]string
	body           string
	enableOnStatus []string
	contentType    string
}

func parseConfig(gjson gjson.Result, config *CustomResponseConfig) error {
	rules := gjson.Get("rules")
	rulesVersion := rules.Exists() && rules.IsArray()
	if rulesVersion {
		for _, cf := range gjson.Get("rules").Array() {
			item := new(CustomResponseRule)
			if err := parseRuleItem(cf, item); err != nil {
				return err
			}
			// the first rule item which enableOnStatus is empty to be set default
			if len(item.enableOnStatus) == 0 && config.defaultRule == nil {
				config.defaultRule = item
			}
			config.rules = append(config.rules, *item)
		}
	} else {
		rule := new(CustomResponseRule)
		if err := parseRuleItem(gjson, rule); err != nil {
			return err
		}
		config.rules = append(config.rules, *rule)
		config.defaultRule = rule
	}
	config.enableOnStatusRuleMap = make(map[string]*CustomResponseRule)
	for i, configItem := range config.rules {
		for _, statusCode := range configItem.enableOnStatus {
			if v, ok := config.enableOnStatusRuleMap[statusCode]; ok {
				log.Errorf("enable_on_status code used in %v, want to add %v", v, statusCode)
				return errors.New("enableOnStatus can only use once")
			}
			config.enableOnStatusRuleMap[statusCode] = &config.rules[i]
		}
	}
	if rulesVersion && config.defaultRule == nil && len(config.enableOnStatusRuleMap) == 0 {
		return errors.New("no valid config is found")
	}
	return nil
}

func parseRuleItem(gjson gjson.Result, rule *CustomResponseRule) error {
	headersArray := gjson.Get("headers").Array()
	rule.headers = make([][2]string, 0, len(headersArray))
	for _, v := range headersArray {
		kv := strings.SplitN(v.String(), "=", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			if strings.EqualFold(key, "content-type") {
				rule.contentType = value
			} else if strings.EqualFold(key, "content-length") {
				continue
			} else {
				rule.headers = append(rule.headers, [2]string{key, value})
			}
		} else {
			return fmt.Errorf("invalid header pair format: %s", v.String())
		}
	}

	rule.body = gjson.Get("body").String()
	if rule.contentType == "" && rule.body != "" {
		if json.Valid([]byte(rule.body)) {
			rule.contentType = "application/json; charset=utf-8"
		} else {
			rule.contentType = "text/plain; charset=utf-8"
		}
	}
	rule.headers = append(rule.headers, [2]string{"content-type", rule.contentType})

	rule.statusCode = 200
	if gjson.Get("status_code").Exists() {
		statusCode := gjson.Get("status_code")
		parsedStatusCode, err := strconv.ParseUint(statusCode.String(), 10, 32)
		if err != nil || parsedStatusCode < 100 || parsedStatusCode > 599 {
			return fmt.Errorf("invalid status code value: %s", statusCode.String())
		}
		rule.statusCode = uint32(parsedStatusCode)
	}

	enableOnStatusArray := gjson.Get("enable_on_status").Array()
	rule.enableOnStatus = make([]string, 0, len(enableOnStatusArray))
	for _, v := range enableOnStatusArray {
		s := v.String()
		_, err := strconv.Atoi(s)
		if err != nil {
			matchString, err := isValidFuzzyMatchString(s)
			if err != nil {
				return err
			}
			rule.enableOnStatus = append(rule.enableOnStatus, matchString)
			continue
		}
		rule.enableOnStatus = append(rule.enableOnStatus, s)
	}
	return nil
}

func isValidFuzzyMatchString(s string) (string, error) {
	const requiredLength = 3
	if len(s) != requiredLength {
		return "", fmt.Errorf("invalid enable_on_status %q: length must be %d", s, requiredLength)
	}

	lower := strings.ToLower(s)
	hasX := false
	hasDigit := false

	for _, c := range lower {
		switch {
		case c == 'x':
			hasX = true
		case c >= '0' && c <= '9':
			hasDigit = true
		default:
			return "", fmt.Errorf("invalid enable_on_status %q: must contain only digits and x/X", s)
		}
	}

	if !hasX {
		return "", fmt.Errorf("invalid enable_on_status %q: fuzzy match must contain x/X (use enable_on_status for exact statusCode matching)", s)
	}
	if !hasDigit {
		return "", fmt.Errorf("invalid enable_on_status %q: must contain at least one digit", s)
	}

	return lower, nil
}

func onHttpRequestHeaders(_ wrapper.HttpContext, config CustomResponseConfig) types.Action {
	if len(config.enableOnStatusRuleMap) != 0 {
		return types.ActionContinue
	}
	log.Infof("use default rule %+v", config.defaultRule)
	err := proxywasm.SendHttpResponseWithDetail(config.defaultRule.statusCode, "custom-response", config.defaultRule.headers, []byte(config.defaultRule.body), -1)
	if err != nil {
		log.Errorf("send http response failed: %v", err)
	}

	return types.ActionPause
}

func onHttpResponseHeaders(_ wrapper.HttpContext, config CustomResponseConfig) types.Action {
	// enableOnStatusRuleMap is not empty, compare the status code.
	// if match the status code, mock the response.
	statusCodeStr, err := proxywasm.GetHttpResponseHeader(":status")
	if err != nil {
		log.Errorf("get http response status code failed: %v", err)
		return types.ActionContinue
	}
	if rule, ok := config.enableOnStatusRuleMap[statusCodeStr]; ok {
		err = proxywasm.SendHttpResponseWithDetail(rule.statusCode, "custom-response", rule.headers, []byte(rule.body), -1)
		if err != nil {
			log.Errorf("send http response failed: %v", err)
		}
		return types.ActionContinue
	}

	if rule, match := fuzzyMatchCode(config.enableOnStatusRuleMap, statusCodeStr); match {
		err = proxywasm.SendHttpResponseWithDetail(rule.statusCode, "custom-response", rule.headers, []byte(rule.body), -1)
		if err != nil {
			log.Errorf("send http response failed: %v", err)
		}
		return types.ActionContinue
	}
	return types.ActionContinue
}

func fuzzyMatchCode(statusRuleMap map[string]*CustomResponseRule, statusCode string) (*CustomResponseRule, bool) {
	if len(statusRuleMap) == 0 || statusCode == "" {
		return nil, false
	}
	codeLen := len(statusCode)
	for pattern, rule := range statusRuleMap {
		// 规则1：模式长度必须与状态码一致
		if len(pattern) != codeLen {
			continue
		}
		// 纯数字的enableOnStatus已经判断过，跳过
		if !strings.Contains(pattern, "x") {
			continue
		}
		// 规则2：所有数字位必须精确匹配
		match := true
		for i, c := range pattern {
			// 如果是数字位需要校验
			if c >= '0' && c <= '9' {
				// 边界检查防止panic
				if i >= codeLen || statusCode[i] != byte(c) {
					match = false
					break
				}
			}
			// 非数字位（如x）自动匹配
		}
		if match {
			return rule, true
		}
	}
	return nil, false
}
