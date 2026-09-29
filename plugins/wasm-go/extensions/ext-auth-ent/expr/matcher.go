package expr

import (
	"errors"
	"strings"

	"regexp"
)

const (
	MatchPatternExact    string = "exact"
	MatchPatternPrefix   string = "prefix"
	MatchPatternSuffix   string = "suffix"
	MatchPatternContains string = "contains"
	MatchPatternRegex    string = "regex"

	MatchIgnoreCase string = "ignore_case"
)

type Matcher interface {
	Match(s string) bool
}

type stringExactMatcher struct {
	target     string
	ignoreCase bool
}

func (m *stringExactMatcher) Match(s string) bool {
	if m.ignoreCase {
		return strings.ToLower(s) == m.target
	}
	return s == m.target
}

type stringPrefixMatcher struct {
	target     string
	ignoreCase bool
}

func (m *stringPrefixMatcher) Match(s string) bool {
	if m.ignoreCase {
		return strings.HasPrefix(strings.ToLower(s), m.target)
	}
	return strings.HasPrefix(s, m.target)
}

type stringSuffixMatcher struct {
	target     string
	ignoreCase bool
}

func (m *stringSuffixMatcher) Match(s string) bool {
	if m.ignoreCase {
		return strings.HasSuffix(strings.ToLower(s), m.target)
	}
	return strings.HasSuffix(s, m.target)
}

type stringContainsMatcher struct {
	target     string
	ignoreCase bool
}

func (m *stringContainsMatcher) Match(s string) bool {
	if m.ignoreCase {
		return strings.Contains(strings.ToLower(s), m.target)
	}
	return strings.Contains(s, m.target)
}

type stringRegexMatcher struct {
	regex *regexp.Regexp
}

func (m *stringRegexMatcher) Match(s string) bool {
	return m.regex.MatchString(s)
}

type MatcherConstructor func(string, bool) (Matcher, error)

var matcherConstructors = map[string]MatcherConstructor{
	MatchPatternExact:    newStringExactMatcher,
	MatchPatternPrefix:   newStringPrefixMatcher,
	MatchPatternSuffix:   newStringSuffixMatcher,
	MatchPatternContains: newStringContainsMatcher,
	MatchPatternRegex:    newStringRegexMatcher,
}

func newStringExactMatcher(target string, ignoreCase bool) (Matcher, error) {
	if ignoreCase {
		target = strings.ToLower(target)
	}
	return &stringExactMatcher{target: target, ignoreCase: ignoreCase}, nil
}

func newStringPrefixMatcher(target string, ignoreCase bool) (Matcher, error) {
	if ignoreCase {
		target = strings.ToLower(target)
	}
	return &stringPrefixMatcher{target: target, ignoreCase: ignoreCase}, nil
}

func newStringSuffixMatcher(target string, ignoreCase bool) (Matcher, error) {
	if ignoreCase {
		target = strings.ToLower(target)
	}
	return &stringSuffixMatcher{target: target, ignoreCase: ignoreCase}, nil
}

func newStringContainsMatcher(target string, ignoreCase bool) (Matcher, error) {
	if ignoreCase {
		target = strings.ToLower(target)
	}
	return &stringContainsMatcher{target: target, ignoreCase: ignoreCase}, nil
}

func newStringRegexMatcher(target string, ignoreCase bool) (Matcher, error) {
	if ignoreCase && !strings.HasPrefix(target, "(?i)") {
		target = "(?i)" + target
	}
	re, err := regexp.Compile(target)
	if err != nil {
		return nil, err
	}
	return &stringRegexMatcher{regex: re}, nil
}

func BuildStringMatcher(matchType, target string, ignoreCase bool) (Matcher, error) {
	for constructorType, constructor := range matcherConstructors {
		if constructorType == matchType {
			return constructor(target, ignoreCase)
		}
	}
	return nil, errors.New("unknown string matcher type")
}
