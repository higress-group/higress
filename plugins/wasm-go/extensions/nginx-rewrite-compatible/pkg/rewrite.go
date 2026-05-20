package pkg

import (
	"fmt"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const (
	propertyNamespace = "nginx_rewrite_compatible"
	headerPrefix      = "x-higress-rewrite-var-"
	argVarPrefix      = "arg_"
	httpVarPrefix     = "http_"
	cookieVarPrefix   = "cookie_"
)

func (c PluginConfig) Apply(ctx wrapper.HttpContext, logger log.Log) (bool, error) {
	originalPath, err := proxywasm.GetHttpRequestHeader(":path")
	if err != nil || originalPath == "" {
		originalPath = ctx.Path()
	}
	if originalPath == "" {
		return false, fmt.Errorf("request path is empty")
	}

	currentPath, currentQuery := splitPathAndQuery(originalPath)
	vars := map[string]string{}
	passHeaders := map[string]bool{}
	requestHeaders := map[string]string{}
	requestCookies := map[string]string{}
	changed := false

	for i, rule := range c.Rules {
		matches := rule.compiled.FindStringSubmatchIndex(currentPath)
		if matches == nil {
			continue
		}

		if !changed {
			ctx.DisableReroute()
		}
		changed = true

		newPath := rule.compiled.ReplaceAllString(currentPath, rule.Replacement)
		if newPath == "" {
			return false, fmt.Errorf("rule %d produced an empty path", i)
		}

		switch {
		case rule.QueryTemplate != "":
			currentQuery = expandTemplate(rule, currentPath, matches, rule.QueryTemplate)
		case rule.QueryAppend != "":
			currentQuery = appendQuery(currentQuery, expandTemplate(rule, currentPath, matches, rule.QueryAppend))
		}

		for _, setVar := range rule.SetVars {
			value := captureGroupValue(currentPath, matches, setVar.CaptureGroup)
			vars[setVar.Name] = value
			passHeaders[setVar.Name] = rule.PassToUpstream
		}

		logger.Debugf("rule %d matched path %q and rewrote it to %q", i, currentPath, newPath)
		currentPath = newPath

		if rule.Mode == ModeBreak {
			break
		}
	}

	if !changed {
		return false, nil
	}

	for name, value := range vars {
		switch {
		case strings.HasPrefix(name, argVarPrefix):
			currentQuery = setQueryParam(currentQuery, strings.TrimPrefix(name, argVarPrefix), value)
		case strings.HasPrefix(name, httpVarPrefix):
			requestHeaders[buildRequestHeaderName(strings.TrimPrefix(name, httpVarPrefix))] = value
		case strings.HasPrefix(name, cookieVarPrefix):
			requestCookies[strings.TrimPrefix(name, cookieVarPrefix)] = value
		case value != "":
			if err := proxywasm.SetProperty([]string{propertyNamespace, "vars", name}, []byte(value)); err != nil {
				return false, fmt.Errorf("failed to set property for var %q: %w", name, err)
			}
		}

		headerName := buildUpstreamHeaderName(name)
		if passHeaders[name] {
			if err := proxywasm.ReplaceHttpRequestHeader(headerName, value); err != nil {
				return false, fmt.Errorf("failed to set upstream header for var %q: %w", name, err)
			}
			continue
		}
		if err := proxywasm.RemoveHttpRequestHeader(headerName); err != nil {
			logger.Warnf("failed to remove upstream header %q: %v", headerName, err)
		}
	}

	for name, value := range requestHeaders {
		if err := proxywasm.ReplaceHttpRequestHeader(name, value); err != nil {
			return false, fmt.Errorf("failed to set request header %q: %w", name, err)
		}
	}

	if len(requestCookies) > 0 {
		currentCookie, err := proxywasm.GetHttpRequestHeader("cookie")
		if err != nil {
			currentCookie = ""
		}
		updatedCookie := currentCookie
		for name, value := range requestCookies {
			updatedCookie = setCookie(updatedCookie, name, value)
		}
		if err := proxywasm.ReplaceHttpRequestHeader("cookie", updatedCookie); err != nil {
			return false, fmt.Errorf("failed to set cookie header: %w", err)
		}
	}

	finalPath := buildPath(currentPath, currentQuery)
	if finalPath != originalPath {
		if err := proxywasm.ReplaceHttpRequestHeader(":path", finalPath); err != nil {
			return false, fmt.Errorf("failed to replace :path header: %w", err)
		}
	}

	return true, nil
}

func splitPathAndQuery(path string) (string, string) {
	pathOnly, query, found := strings.Cut(path, "?")
	if !found {
		return path, ""
	}
	return pathOnly, query
}

func buildPath(path string, query string) string {
	if query == "" {
		return path
	}
	return path + "?" + query
}

func appendQuery(existing string, suffix string) string {
	if suffix == "" {
		return existing
	}
	if existing == "" {
		return suffix
	}
	return existing + "&" + suffix
}

func setQueryParam(existing string, key string, value string) string {
	if key == "" {
		return existing
	}

	parts := []string{}
	replaced := false
	if existing != "" {
		for _, part := range strings.Split(existing, "&") {
			if part == "" {
				continue
			}
			name, _, _ := strings.Cut(part, "=")
			if name != key {
				parts = append(parts, part)
				continue
			}
			if !replaced {
				parts = append(parts, key+"="+value)
				replaced = true
			}
		}
	}
	if !replaced {
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, "&")
}

func expandTemplate(rule Rule, currentPath string, matches []int, template string) string {
	return string(rule.compiled.ExpandString(nil, template, currentPath, matches))
}

func captureGroupValue(currentPath string, matches []int, group int) string {
	index := group * 2
	if index+1 >= len(matches) {
		return ""
	}
	start, end := matches[index], matches[index+1]
	if start < 0 || end < 0 {
		return ""
	}
	return currentPath[start:end]
}

func buildUpstreamHeaderName(name string) string {
	sanitized := strings.ToLower(strings.TrimSpace(name))
	sanitized = strings.ReplaceAll(sanitized, "_", "-")
	sanitized = strings.ReplaceAll(sanitized, " ", "-")
	return headerPrefix + sanitized
}

func buildRequestHeaderName(name string) string {
	sanitized := strings.ToLower(strings.TrimSpace(name))
	sanitized = strings.ReplaceAll(sanitized, "_", "-")
	sanitized = strings.ReplaceAll(sanitized, " ", "-")
	return sanitized
}

func setCookie(existing string, key string, value string) string {
	if key == "" {
		return existing
	}

	parts := []string{}
	replaced := false
	if existing != "" {
		for _, part := range strings.Split(existing, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			name, _, _ := strings.Cut(part, "=")
			if name != key {
				parts = append(parts, part)
				continue
			}
			if !replaced {
				parts = append(parts, key+"="+value)
				replaced = true
			}
		}
	}
	if !replaced {
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, "; ")
}
