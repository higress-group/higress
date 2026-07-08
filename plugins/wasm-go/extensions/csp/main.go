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
	"errors"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	// headerCSP is the standard Content-Security-Policy response header.
	headerCSP = "Content-Security-Policy"
	// headerCSPReportOnly only reports violations without enforcing them.
	headerCSPReportOnly = "Content-Security-Policy-Report-Only"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"csp",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
	)
}

// @Name csp
// @Category security
// @Phase UNSPECIFIED_PHASE
// @Priority 330
// @Title zh-CN CSP 内容安全策略
// @Description zh-CN 为 HTTP 响应注入 Content-Security-Policy 响应头，降低 XSS 等注入类攻击风险。
// @Title en-US CSP Content Security Policy
// @Description en-US Injects the Content-Security-Policy response header to reduce XSS and other injection risks.
// @Version 1.0.0
//
// @Example
// policy: "default-src 'self'; img-src *; script-src 'self'"
// report_only: false
// override: true
//
// @End
type PluginConfig struct {
	// @Title zh-CN CSP 策略
	// @Description zh-CN CSP 指令字符串，例如 default-src 'self'; img-src *
	// @Title en-US CSP policy
	// @Description en-US CSP directive string, for example default-src 'self'; img-src *
	Policy string `required:"true" minLength:"1" yaml:"policy" json:"policy"`
	// @Title zh-CN 仅上报模式
	// @Description zh-CN 为 true 时使用 Content-Security-Policy-Report-Only 响应头，仅上报违规而不强制拦截。
	// @Title en-US Report-only mode
	// @Description en-US When true, uses Content-Security-Policy-Report-Only so violations are reported but not enforced.
	ReportOnly bool `required:"false" yaml:"report_only" json:"report_only"`
	// @Title zh-CN 仅上报附加策略
	// @Description zh-CN 可选。设置后在强制策略之外额外注入一个 Content-Security-Policy-Report-Only 响应头，用于强制现有策略的同时灰度验证更严格的候选策略。不可与 report_only: true 同时使用。
	// @Title en-US Additional report-only policy
	// @Description en-US Optional. Injects an extra Content-Security-Policy-Report-Only header alongside the enforced policy, so a stricter candidate policy can be validated while the current one stays enforced. Cannot be combined with report_only: true.
	ReportOnlyPolicy string `required:"false" minLength:"1" yaml:"report_only_policy" json:"report_only_policy"`
	// @Title zh-CN 覆盖上游响应头
	// @Description zh-CN 为 true 时先清除上游已设置的 CSP 响应头再注入；为 false 时若响应中已存在同名 CSP 响应头则保留原值。
	// @Title en-US Override upstream header
	// @Description en-US When true, clears upstream CSP headers before injecting; when false, keeps an existing upstream header of the same variant.
	Override bool `required:"false" yaml:"override" json:"override"`
}

// headerName returns the response header name to use based on reportOnly.
func (c *PluginConfig) headerName() string {
	if c.ReportOnly {
		return headerCSPReportOnly
	}
	return headerCSP
}

func parseConfig(json gjson.Result, config *PluginConfig) error {
	config.Policy = strings.TrimSpace(json.Get("policy").String())
	if config.Policy == "" {
		return errors.New("csp: `policy` must be configured and non-empty")
	}
	config.ReportOnly = json.Get("report_only").Bool()
	config.ReportOnlyPolicy = strings.TrimSpace(json.Get("report_only_policy").String())
	if config.ReportOnly && config.ReportOnlyPolicy != "" {
		return errors.New("csp: `report_only_policy` cannot be combined with `report_only: true`")
	}
	if override := json.Get("override"); override.Exists() && override.Type != gjson.Null {
		config.Override = override.Bool()
	} else {
		config.Override = true
	}
	return nil
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config PluginConfig) types.Action {
	// Header(s) to emit: the primary policy, plus an optional report-only
	// candidate that runs alongside an enforced policy.
	out := [][2]string{{config.headerName(), config.Policy}}
	if !config.ReportOnly && config.ReportOnlyPolicy != "" {
		out = append(out, [2]string{headerCSPReportOnly, config.ReportOnlyPolicy})
	}

	if config.Override {
		// Drop both variants; Replace only touches the first value.
		_ = proxywasm.RemoveHttpResponseHeader(headerCSP)
		_ = proxywasm.RemoveHttpResponseHeader(headerCSPReportOnly)
	}

	for _, h := range out {
		// Without override, keep an existing upstream header of the same variant.
		// Presence is keyed on the error alone: Envoy returns Ok+empty for a
		// present-but-empty header; anything but a confirmed not-found counts as present.
		if !config.Override {
			if _, err := proxywasm.GetHttpResponseHeader(h[0]); !errors.Is(err, types.ErrorStatusNotFound) {
				continue
			}
		}
		if err := proxywasm.AddHttpResponseHeader(h[0], h[1]); err != nil {
			log.Errorf("csp: failed to set %s header: %v", h[0], err)
		}
	}
	return types.ActionContinue
}
