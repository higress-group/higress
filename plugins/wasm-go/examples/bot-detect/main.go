/*
 * Copyright (c) 2022 Alibaba Group Holding Ltd.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"bot-detect/config"

	"regexp"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"bot-detect",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
	)
}

func parseConfig(json gjson.Result, botDetectConfig *config.BotDetectConfig, log log.Log) error {
	log.Debug("parseConfig()")

	if json.Get("blocked_code").Exists() {
		botDetectConfig.BlockedCode = uint32(int(json.Get("blocked_code").Int()))
	}

	if json.Get("blocked_message").Exists() {
		botDetectConfig.BlockedMessage = json.Get("blocked_message").String()
	}

	allowRules := make([]gjson.Result, 0)
	denyRules := make([]gjson.Result, 0)

	allowRulesValue := json.Get("allow")
	if allowRulesValue.Exists() && allowRulesValue.IsArray() {
		allowRules = json.Get("allow").Array()
	}

	denyRulesValue := json.Get("deny")
	if denyRulesValue.Exists() && denyRulesValue.IsArray() {
		denyRules = json.Get("deny").Array()
	}

	for _, allowRule := range allowRules {
		c, err := regexp.Compile(allowRule.String())
		if err != nil {
			return err
		}
		botDetectConfig.Allow = append(botDetectConfig.Allow, c)
	}

	for _, denyRule := range denyRules {
		c, err := regexp.Compile(denyRule.String())
		if err != nil {
			return err
		}
		botDetectConfig.Deny = append(botDetectConfig.Deny, c)
	}

	// Fill default values
	botDetectConfig.FillDefaultValue()
	log.Debugf("botDetectConfig:%+v", botDetectConfig)
	return nil

}

func onHttpRequestHeaders(ctx wrapper.HttpContext, botDetectConfig config.BotDetectConfig, log log.Log) types.Action {
	log.Debug("onHttpRequestHeaders()")
	//// Get user-agent header
	ua, err := proxywasm.GetHttpRequestHeader("user-agent")
	if err != nil {
		log.Warnf("failed to get user-agent: %v", err)
		return types.ActionPause
	}
	host := ctx.Host()
	scheme := ctx.Scheme()
	path := ctx.Path()
	method := ctx.Method()

	if ok, rule := botDetectConfig.Process(ua); !ok {
		proxywasm.SendHttpResponseWithDetail(botDetectConfig.BlockedCode, "bot-detect.blocked", nil, []byte(botDetectConfig.BlockedMessage), -1)
		log.Debugf("scheme:%s, host:%s, method:%s, path:%s user-agent:%s has been blocked by rule:%s", scheme, host, method, path, ua, rule)
		return types.ActionPause
	}

	log.Debugf("scheme:%s, host:%s, method:%s, path:%s user-agent:%s has been passed", scheme, host, method, path, ua)
	return types.ActionContinue
}
