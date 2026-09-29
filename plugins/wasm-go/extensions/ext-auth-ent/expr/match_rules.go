package expr

import (
	"strings"

	"ext-auth/util"
	"regexp"
)

const (
	ModeWhitelist = "whitelist"
	ModeBlacklist = "blacklist"
)

type MatchRules struct {
	Mode     string
	RuleList []Rule
}

type Rule struct {
	Domain string
	Method []string
	Path   Matcher
}

func MatchRulesDefaults() MatchRules {
	return MatchRules{
		Mode:     ModeWhitelist,
		RuleList: []Rule{},
	}
}

// IsAllowedByMode checks if the given domain, method and path are allowed based on the configuration mode.
func (config *MatchRules) IsAllowedByMode(domain, method, path string) bool {
	switch config.Mode {
	case ModeWhitelist:
		for _, rule := range config.RuleList {
			if rule.matchesAllConditions(domain, method, path) {
				return true
			}
		}
		return false
	case ModeBlacklist:
		for _, rule := range config.RuleList {
			if rule.matchesAllConditions(domain, method, path) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// matchesAllConditions checks if the given domain, method and path match all conditions of the rule.
func (rule *Rule) matchesAllConditions(domain, method, path string) bool {
	// If all conditions are empty, return false
	if rule.Domain == "" && rule.Path == nil && len(rule.Method) == 0 {
		return false
	}

	// Check domain and path matching
	domainMatch := rule.Domain == "" || matchDomain(domain, rule.Domain)
	pathMatch := rule.Path == nil || rule.Path.Match(path)

	// Check HTTP method matching: if no methods are specified, any method is allowed
	methodMatch := len(rule.Method) == 0 || util.ContainsString(rule.Method, method)

	return domainMatch && pathMatch && methodMatch
}

// matchDomain checks if the given domain matches the pattern.
func matchDomain(domain string, pattern string) bool {
	// Convert wildcard pattern to regex pattern
	regexPattern := convertWildcardToRegex(pattern)
	matched, _ := regexp.MatchString(regexPattern, domain)
	return matched
}

// convertWildcardToRegex converts a wildcard pattern to a regex pattern.
func convertWildcardToRegex(pattern string) string {
	pattern = regexp.QuoteMeta(pattern)
	pattern = "^" + strings.ReplaceAll(pattern, "\\*", ".*") + "$"
	return pattern
}
