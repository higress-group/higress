package expr

import (
	"regexp"
	"strings"

	"ext-auth/util"
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
	Domain  string
	Method  []string
	Path    Matcher
	Headers []HeaderCondition
}

type HeaderCondition struct {
	Name   string
	Exists bool
}

type HeaderNameSet map[string]struct{}

type RequestAttributes struct {
	Domain      string
	Method      string
	Path        string
	HeaderNames HeaderNameSet
}

func NewHeaderNameSet(headers [][2]string) HeaderNameSet {
	headerNames := make(HeaderNameSet, len(headers))
	for _, header := range headers {
		headerNames[strings.ToLower(header[0])] = struct{}{}
	}
	return headerNames
}

func MatchRulesDefaults() MatchRules {
	return MatchRules{
		Mode:     ModeWhitelist,
		RuleList: []Rule{},
	}
}

func (config *MatchRules) RequiresRequestHeaders() bool {
	for _, rule := range config.RuleList {
		if len(rule.Headers) > 0 {
			return true
		}
	}
	return false
}

// Matches reports whether the request is within the external authorization scope.
func (config *MatchRules) Matches(request RequestAttributes) bool {
	switch config.Mode {
	case ModeWhitelist:
		return !config.matchesAnyRule(request)
	case ModeBlacklist:
		return config.matchesAnyRule(request)
	default:
		return true
	}
}

func (config *MatchRules) matchesAnyRule(request RequestAttributes) bool {
	for _, rule := range config.RuleList {
		if rule.matchesAllConditions(request) {
			return true
		}
	}
	return false
}

// matchesAllConditions checks if the given domain, method, path and headers match all conditions of the rule.
func (rule *Rule) matchesAllConditions(request RequestAttributes) bool {
	// If all conditions are empty, return false
	if rule.Domain == "" && rule.Path == nil && len(rule.Method) == 0 && len(rule.Headers) == 0 {
		return false
	}

	// Check domain and path matching
	domainMatch := rule.Domain == "" || matchDomain(request.Domain, rule.Domain)
	pathMatch := rule.Path == nil || rule.Path.Match(request.Path)

	// Check HTTP method matching: if no methods are specified, any method is allowed
	methodMatch := len(rule.Method) == 0 || util.ContainsString(rule.Method, request.Method)

	headerMatch := true
	for _, condition := range rule.Headers {
		_, present := request.HeaderNames[condition.Name]
		if present != condition.Exists {
			headerMatch = false
			break
		}
	}

	return domainMatch && pathMatch && methodMatch && headerMatch
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
