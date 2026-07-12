// Copyright (c) 2025 Alibaba Group Holding Ltd.
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
	// headerContentSecurityPolicy is the enforcing CSP response header.
	headerContentSecurityPolicy = "Content-Security-Policy"
	// headerContentSecurityPolicyReportOnly is the report-only CSP response header.
	headerContentSecurityPolicyReportOnly = "Content-Security-Policy-Report-Only"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"csp",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
	)
}

// CSPConfig holds the Content Security Policy values to apply to responses.
type CSPConfig struct {
	contentSecurityPolicy           string
	contentSecurityPolicyReportOnly string
}

func parseConfig(json gjson.Result, config *CSPConfig) error {
	config.contentSecurityPolicy = strings.TrimSpace(json.Get("content_security_policy").String())
	config.contentSecurityPolicyReportOnly = strings.TrimSpace(json.Get("content_security_policy_report_only").String())

	if config.contentSecurityPolicy == "" && config.contentSecurityPolicyReportOnly == "" {
		return errors.New("at least one of content_security_policy or content_security_policy_report_only must be configured")
	}

	log.Infof("content_security_policy: %q, content_security_policy_report_only: %q",
		config.contentSecurityPolicy, config.contentSecurityPolicyReportOnly)
	return nil
}

func onHttpResponseHeaders(_ wrapper.HttpContext, config CSPConfig) types.Action {
	applyResponseHeader(headerContentSecurityPolicy, config.contentSecurityPolicy)
	applyResponseHeader(headerContentSecurityPolicyReportOnly, config.contentSecurityPolicyReportOnly)
	return types.ActionContinue
}

// applyResponseHeader removes any pre-existing header with the given name and
// sets a single value. This makes the gateway-configured policy authoritative
// and avoids duplicate or conflicting CSP headers (multiple CSP headers are
// intersected by browsers, which is rarely the desired behavior).
func applyResponseHeader(name, value string) {
	if value == "" {
		return
	}
	if err := proxywasm.RemoveHttpResponseHeader(name); err != nil {
		log.Errorf("failed to remove existing %s header: %v", name, err)
	}
	if err := proxywasm.AddHttpResponseHeader(name, value); err != nil {
		log.Errorf("failed to add %s header: %v", name, err)
	}
}
