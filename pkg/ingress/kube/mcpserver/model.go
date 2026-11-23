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

package mcpserver

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"

	"istio.io/istio/pkg/config"
)

var (
	GvkMcpServer = config.GroupVersionKind{Group: "networking.higress.io", Version: "v1alpha1", Kind: "McpServer"}
)

const (
	UpstreamTypeRest       string = "rest"
	UpstreamTypeSSE        string = "sse"
	UpstreamTypeStreamable string = "streamable"

	ExactMatchType    string = "exact"
	PrefixMatchType   string = "prefix"
	SuffixMatchType   string = "suffix"
	ContainsMatchType string = "contains"
	RegexMatchType    string = "regex"
)

var (
	ValidUpstreamTypes = map[string]bool{
		UpstreamTypeRest:       true,
		UpstreamTypeSSE:        true,
		UpstreamTypeStreamable: true,
	}
	ValidPathMatchTypes = map[string]bool{
		ExactMatchType:    true,
		PrefixMatchType:   true,
		SuffixMatchType:   true,
		ContainsMatchType: true,
		RegexMatchType:    true,
	}
)

type McpServer struct {
	Name              string   `json:"name,omitempty"`
	Domains           []string `json:"domains,omitempty"`
	PathMatchType     string   `json:"path_match_type,omitempty"`
	PathMatchValue    string   `json:"path_match_value,omitempty"`
	UpstreamType      string   `json:"upstream_type,omitempty"`
	EnablePathRewrite bool     `json:"enable_path_rewrite,omitempty"`
	PathRewritePrefix string   `json:"path_rewrite_prefix,omitempty"`
}

// ValidateRuleContent rejects values that cannot be represented faithfully in
// the mcp-session filter config. Control characters never appear in a
// legitimate domain, path or prefix, and invalid UTF-8 would be lossily
// replaced when the config is marshaled.
//
// An empty domain is accepted because it matches any host. The
// path-rewrite/upstream-type pairing is deliberately not checked here:
// mcp-session only logs a warning and ignores an unsupported rewrite, so
// dropping the rule would disable MCP for the route entirely.
func ValidateRuleContent(domains []string, pathMatchValue, pathRewritePrefix string) error {
	for _, domain := range domains {
		if err := ValidateRuleValue("match_rule_domain", domain); err != nil {
			return err
		}
	}
	if err := ValidateRuleValue("match_rule_path", pathMatchValue); err != nil {
		return err
	}
	return ValidateRuleValue("path_rewrite_prefix", pathRewritePrefix)
}

// ValidateRuleValue checks a single rule field.
func ValidateRuleValue(field, value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", field)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s cannot contain control characters", field)
		}
	}
	return nil
}

// ValidateMatchRule checks an annotation-derived server at parse time so that a
// malformed annotation is rejected instead of flowing into the mcp-session
// filter config. The enum constraints mirror the ones applied to ConfigMap
// MatchList entries.
func ValidateMatchRule(server *McpServer) error {
	if server == nil {
		return errors.New("mcp server cannot be nil")
	}

	if len(server.Domains) == 0 {
		return errors.New("domains cannot be empty")
	}
	for _, domain := range server.Domains {
		if domain == "" {
			return errors.New("domains cannot contain an empty entry")
		}
	}

	if server.PathMatchType == "" {
		return errors.New("path_match_type cannot be empty, must be one of: exact, prefix, suffix, contains, regex")
	}
	if !ValidPathMatchTypes[server.PathMatchType] {
		return fmt.Errorf("invalid path_match_type: %s, must be one of: exact, prefix, suffix, contains, regex", server.PathMatchType)
	}

	if server.UpstreamType != "" && !ValidUpstreamTypes[server.UpstreamType] {
		return fmt.Errorf("invalid upstream_type: %s, must be one of: rest, sse, streamable", server.UpstreamType)
	}

	if server.PathMatchValue == "" {
		return errors.New("path_match_value cannot be empty")
	}

	return ValidateRuleContent(server.Domains, server.PathMatchValue, server.PathRewritePrefix)
}
