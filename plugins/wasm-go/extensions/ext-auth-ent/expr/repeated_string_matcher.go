package expr

import (
	"errors"

	"github.com/tidwall/gjson"
)

type repeatedStringMatcher struct {
	matchers []Matcher
}

func (rsm *repeatedStringMatcher) Match(s string) bool {
	for _, m := range rsm.matchers {
		if m.Match(s) {
			return true
		}
	}
	return false
}

func buildRepeatedStringMatcher(matchers []gjson.Result, allIgnoreCase bool) (Matcher, error) {
	builtMatchers := make([]Matcher, len(matchers))

	createMatcher := func(json gjson.Result, targetKey string, ignoreCase bool, constructor MatcherConstructor) (Matcher, error) {
		result := json.Get(targetKey)
		if result.Exists() && result.String() != "" {
			target := result.String()
			return constructor(target, ignoreCase)
		}
		return nil, nil
	}

	for i, item := range matchers {
		var matcher Matcher
		var err error

		// If allIgnoreCase is true, it takes precedence over any user configuration,
		// forcing case-insensitive matching regardless of individual item settings.
		ignoreCase := allIgnoreCase
		if !allIgnoreCase {
			ignoreCaseResult := item.Get(MatchIgnoreCase)
			if ignoreCaseResult.Exists() && ignoreCaseResult.Bool() {
				ignoreCase = true
			}
		}

		for key, creator := range matcherConstructors {
			if matcher, err = createMatcher(item, key, ignoreCase, creator); err != nil {
				return nil, err
			}
			if matcher != nil {
				break
			}
		}

		if matcher == nil {
			return nil, errors.New("unknown string matcher type")
		}

		builtMatchers[i] = matcher

	}

	return &repeatedStringMatcher{
		matchers: builtMatchers,
	}, nil
}

func BuildRepeatedStringMatcherIgnoreCase(matchers []gjson.Result) (Matcher, error) {
	return buildRepeatedStringMatcher(matchers, true)
}

func BuildRepeatedStringMatcher(matchers []gjson.Result) (Matcher, error) {
	return buildRepeatedStringMatcher(matchers, false)
}
