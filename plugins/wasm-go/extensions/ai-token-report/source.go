package main

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

// Same source configuration contract as ai-route-monitor. Kept local to this
// independently built plugin so attribution needs no new shared runtime artifact.
type sourceConfig struct {
	headerName    string
	allowedValues map[string]bool
	fallbackValue string
}

func parseSourceConfig(value gjson.Result) (sourceConfig, error) {
	c := sourceConfig{headerName: "x-wujie-source", allowedValues: make(map[string]bool), fallbackValue: "unknown"}
	if value.Exists() && !value.IsObject() {
		return c, fmt.Errorf("must be an object")
	}
	if header := value.Get("headerName"); header.Exists() {
		if header.Type != gjson.String || !validSourceHeader(header.String()) {
			return c, fmt.Errorf("headerName must be an HTTP header token")
		}
		c.headerName = strings.ToLower(header.String())
	}
	if fallback := value.Get("fallbackValue"); fallback.Exists() {
		if fallback.Type != gjson.String || !validSourceValue(fallback.String()) {
			return c, fmt.Errorf("fallbackValue must match [A-Za-z0-9_-]{1,64}")
		}
		c.fallbackValue = fallback.String()
	}
	if allowed := value.Get("allowedValues"); allowed.Exists() {
		if !allowed.IsArray() || len(allowed.Array()) > 16 {
			return c, fmt.Errorf("allowedValues must be an array of at most 16 values")
		}
		for _, item := range allowed.Array() {
			if item.Type != gjson.String || !validSourceValue(item.String()) {
				return c, fmt.Errorf("allowedValues must match [A-Za-z0-9_-]{1,64}")
			}
			c.allowedValues[item.String()] = true
		}
	}
	return c, nil
}

func (c sourceConfig) normalize(raw string) string {
	if c.allowedValues[raw] {
		return raw
	}
	return c.fallbackValue
}

func validSourceValue(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, ch := range s {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}

func validSourceHeader(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", ch)) {
			return false
		}
	}
	return true
}
