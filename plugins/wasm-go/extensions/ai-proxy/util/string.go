package util

import (
	"regexp"
)

func StripPrefix(s string, prefix string) string {
	if len(prefix) != 0 && len(s) >= len(prefix) && s[0:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

func MatchStatus(status string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, _ := regexp.MatchString(pattern, status)
		if matched {
			return true
		}
	}
	return false
}
