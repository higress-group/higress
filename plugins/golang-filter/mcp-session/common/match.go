package common

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
)

// RuleType defines the type of matching rule
type RuleType string

// UpstreamType defines the type of matching rule
type UpstreamType string

// HostMatchType defines the type of host matching
type HostMatchType int

const (
	ExactMatch    RuleType = "exact"
	PrefixMatch   RuleType = "prefix"
	SuffixMatch   RuleType = "suffix"
	ContainsMatch RuleType = "contains"
	RegexMatch    RuleType = "regex"

	RestUpstream       UpstreamType = "rest"
	SSEUpstream        UpstreamType = "sse"
	StreamableUpstream UpstreamType = "streamable"

	HostExact HostMatchType = iota
	HostPrefix
	HostSuffix
)

// HostMatcher defines the structure for host matching
type HostMatcher struct {
	matchType HostMatchType
	host      string
}

// MatchRule defines the structure for a matching rule
type MatchRule struct {
	ServerName        string       `json:"server_name"`         // MCP server that owns this rule, when known
	MatchRuleDomain   string       `json:"match_rule_domain"`   // Domain pattern, supports wildcards
	MatchRulePath     string       `json:"match_rule_path"`     // Path pattern to match
	MatchRuleType     RuleType     `json:"match_rule_type"`     // Type of match rule
	UpstreamType      UpstreamType `json:"upstream_type"`       // Type of upstream(s) matched by the rule
	EnablePathRewrite bool         `json:"enable_path_rewrite"` // Enable request path rewrite for matched routes
	PathRewritePrefix string       `json:"path_rewrite_prefix"` // Prefix the request path would be rewritten to.
	HostMatcher       HostMatcher  // Host matcher for efficient matching
}

// ParseMatchList parses the match list from the config
func ParseMatchList(matchListConfig []interface{}) []MatchRule {
	matchList := make([]MatchRule, 0)
	for _, item := range matchListConfig {
		if ruleMap, ok := item.(map[string]interface{}); ok {
			rule := MatchRule{}
			if serverName, ok := ruleMap["server_name"].(string); ok {
				rule.ServerName = serverName
			}
			if domain, ok := ruleMap["match_rule_domain"].(string); ok {
				rule.MatchRuleDomain = domain
			}
			if path, ok := ruleMap["match_rule_path"].(string); ok {
				rule.MatchRulePath = path
			}
			if ruleType, ok := ruleMap["match_rule_type"].(string); ok {
				rule.MatchRuleType = RuleType(ruleType)
			}
			if upstreamType, ok := ruleMap["upstream_type"].(string); ok {
				rule.UpstreamType = UpstreamType(upstreamType)
			}
			if len(rule.UpstreamType) == 0 {
				rule.UpstreamType = RestUpstream
			} else {
				switch rule.UpstreamType {
				case RestUpstream, SSEUpstream, StreamableUpstream:
					break
				default:
					api.LogWarnf("Unknown upstream type: %s", rule.UpstreamType)
				}
			}
			if enablePathRewrite, ok := ruleMap["enable_path_rewrite"].(bool); ok {
				rule.EnablePathRewrite = enablePathRewrite
			}
			if pathRewritePrefix, ok := ruleMap["path_rewrite_prefix"].(string); ok {
				rule.PathRewritePrefix = pathRewritePrefix
			}
			if rule.EnablePathRewrite {
				if rule.UpstreamType != SSEUpstream {
					api.LogWarnf("Path rewrite is only supported for SSE upstream type")
				} else if rule.MatchRuleType != PrefixMatch {
					api.LogWarnf("Path rewrite is only supported for prefix match type")
				} else if !strings.HasPrefix(rule.PathRewritePrefix, "/") {
					rule.PathRewritePrefix = "/" + rule.PathRewritePrefix
				}
			}

			rule.HostMatcher = ParseHostPattern(rule.MatchRuleDomain)

			matchList = append(matchList, rule)
		}
	}
	return matchList
}

// SessionServerName returns the identity a session must be bound to for this
// rule. A rule that carries an explicit server name uses it directly. When the
// server name is unavailable, the rule's own match key is used so that distinct
// rules never share sessions. There is deliberately no global fallback: using
// one would make sessions replayable across routes.
func (rule MatchRule) SessionServerName() string {
	if rule.ServerName != "" {
		return rule.ServerName
	}
	return "rule:" +
		encodeKeyComponent(rule.MatchRuleDomain) + ":" +
		encodeKeyComponent(rule.MatchRulePath) + ":" +
		encodeKeyComponent(string(rule.MatchRuleType)) + ":" +
		encodeKeyComponent(string(rule.UpstreamType)) + ":" +
		encodeKeyComponent(strconv.FormatBool(rule.EnablePathRewrite)) + ":" +
		encodeKeyComponent(rule.PathRewritePrefix)
}

// stripPortFromHost removes port from host string
// Port removing code is inspired by
// https://github.com/envoyproxy/envoy/blob/v1.17.0/source/common/http/header_utility.cc#L219
func stripPortFromHost(reqHost string) string {
	portStart := strings.LastIndexByte(reqHost, ':')
	if portStart != -1 {
		// According to RFC3986 v6 address is always enclosed in "[]".
		// section 3.2.2.
		v6EndIndex := strings.LastIndexByte(reqHost, ']')
		if v6EndIndex == -1 || v6EndIndex < portStart {
			if portStart+1 <= len(reqHost) {
				return reqHost[:portStart]
			}
		}
	}
	return reqHost
}

// ParseHostPattern parses a host pattern and returns a HostMatcher
func ParseHostPattern(pattern string) HostMatcher {
	var hostMatcher HostMatcher
	if strings.HasPrefix(pattern, "*") {
		hostMatcher.matchType = HostSuffix
		hostMatcher.host = pattern[1:]
	} else if strings.HasSuffix(pattern, "*") {
		hostMatcher.matchType = HostPrefix
		hostMatcher.host = pattern[:len(pattern)-1]
	} else {
		hostMatcher.matchType = HostExact
		hostMatcher.host = pattern
	}
	return hostMatcher
}

// matchPattern checks if the target matches the pattern based on rule type
func matchPattern(pattern string, target string, ruleType RuleType) bool {
	if pattern == "" {
		return true
	}

	switch ruleType {
	case ExactMatch:
		return pattern == target
	case PrefixMatch:
		return strings.HasPrefix(target, pattern)
	case SuffixMatch:
		return strings.HasSuffix(target, pattern)
	case ContainsMatch:
		return strings.Contains(target, pattern)
	case RegexMatch:
		matched, err := regexp.MatchString(pattern, target)
		if err != nil {
			return false
		}
		return matched
	default:
		return false
	}
}

// matchDomainWithMatcher checks if the domain matches using a pre-parsed HostMatcher
func matchDomainWithMatcher(domain string, hostMatcher HostMatcher) bool {
	// Strip port from domain
	domain = stripPortFromHost(domain)

	// Perform matching based on match type
	switch hostMatcher.matchType {
	case HostSuffix:
		return strings.HasSuffix(domain, hostMatcher.host)
	case HostPrefix:
		return strings.HasPrefix(domain, hostMatcher.host)
	case HostExact:
		return domain == hostMatcher.host
	default:
		return false
	}
}

// matchDomainAndPath checks if both domain and path match the rule
func matchDomainAndPath(domain, path string, rule MatchRule) bool {
	return matchDomainWithMatcher(domain, rule.HostMatcher) &&
		matchPattern(rule.MatchRulePath, path, rule.MatchRuleType)
}

// IsMatch checks if the request matches any rule in the rule list
// Returns true if no rules are specified
func IsMatch(rules []MatchRule, host, path string) (bool, MatchRule) {
	if len(rules) == 0 {
		return true, MatchRule{}
	}

	for _, rule := range rules {
		if matchDomainAndPath(host, path, rule) {
			return true, rule
		}
	}
	return false, MatchRule{}
}

// MatchDomainList checks if the domain matches any of the domains in the list
func MatchDomainWithMatchers(domain string, hostMatchers []HostMatcher) bool {
	for _, hostMatcher := range hostMatchers {
		if matchDomainWithMatcher(domain, hostMatcher) {
			return true
		}
	}
	return false
}
