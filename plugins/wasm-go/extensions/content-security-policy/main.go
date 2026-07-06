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
	// headerContentSecurityPolicy is the enforcing CSP header. Resources that
	// violate the policy are blocked by the user agent.
	headerContentSecurityPolicy = "Content-Security-Policy"
	// headerContentSecurityPolicyReport is the report-only CSP header.
	// Violations are reported (via report-uri / report-to / the Reporting API)
	// but not enforced, which is useful for rolling out a policy safely.
	headerContentSecurityPolicyReport = "Content-Security-Policy-Report-Only"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"content-security-policy",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
	)
}

// CSPConfig holds the parsed configuration for the content-security-policy plugin.
type CSPConfig struct {
	// policies is the list of Content-Security-Policy directive strings. Each
	// entry is emitted as a separate response header; per the CSP spec a browser
	// enforces the intersection (most restrictive) of all delivered policies.
	policies []string
	// reportOnly switches the emitted header to Content-Security-Policy-Report-Only,
	// so violations are reported rather than enforced. Useful for validating a
	// policy before switching it to enforcing mode.
	reportOnly bool
}

func parseConfig(json gjson.Result, config *CSPConfig, log log.Log) error {
	config.reportOnly = json.Get("report_only").Bool()

	for _, item := range json.Get("policies").Array() {
		if policy := strings.TrimSpace(item.String()); policy != "" {
			config.policies = append(config.policies, policy)
		}
	}

	// Convenience alias: a single "policy" field is treated as a one-element list.
	if single := strings.TrimSpace(json.Get("policy").String()); single != "" {
		config.policies = append(config.policies, single)
	}

	if len(config.policies) == 0 {
		return errors.New("no content security policy configured: at least one policy is required")
	}

	return nil
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config CSPConfig, log log.Log) types.Action {
	headerName := headerContentSecurityPolicy
	if config.reportOnly {
		headerName = headerContentSecurityPolicyReport
	}

	// Drop any CSP header already set by the upstream so the gateway-owned
	// policy is authoritative (replace semantics, matching the cors plugin).
	if err := proxywasm.RemoveHttpResponseHeader(headerName); err != nil {
		log.Debugf("failed to remove existing %s header: %v", headerName, err)
	}

	// Combine all configured policy blocks into a single header value. CSP
	// directives are separated by ";", so joining blocks with "; " produces one
	// valid Content-Security-Policy header. This is the standard form (one header,
	// multiple directives) and avoids the ambiguity of multiple same-named headers.
	combined := strings.Join(config.policies, "; ")
	if err := proxywasm.AddHttpResponseHeader(headerName, combined); err != nil {
		log.Warnf("failed to add %s header: %v", headerName, err)
	}

	return types.ActionContinue
}
