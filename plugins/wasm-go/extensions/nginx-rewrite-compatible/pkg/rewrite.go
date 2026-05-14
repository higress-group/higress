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
		if value != "" {
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
