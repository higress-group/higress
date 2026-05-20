package pkg

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

const (
	ModeBreak = "break"
	ModeLast  = "last"
)

type PluginConfig struct {
	Rules []Rule `json:"rules"`
}

type Rule struct {
	Regex          string   `json:"regex"`
	Replacement    string   `json:"replacement"`
	QueryAppend    string   `json:"query_append,omitempty"`
	QueryTemplate  string   `json:"query_template,omitempty"`
	SetVars        []SetVar `json:"set_vars,omitempty"`
	PassToUpstream bool     `json:"pass_to_upstream,omitempty"`
	Mode           string   `json:"mode,omitempty"`

	compiled *regexp.Regexp
}

type SetVar struct {
	Name         string `json:"name"`
	CaptureGroup int    `json:"capture_group"`
}

func (c *PluginConfig) FromJson(json gjson.Result) error {
	rules := json.Get("rules")
	if !rules.Exists() || !rules.IsArray() || len(rules.Array()) == 0 {
		return errors.New("rules must be a non-empty array")
	}

	c.Rules = make([]Rule, 0, len(rules.Array()))
	for i, item := range rules.Array() {
		rule, err := parseRule(item)
		if err != nil {
			return fmt.Errorf("invalid rule %d: %w", i, err)
		}
		c.Rules = append(c.Rules, rule)
	}
	return nil
}

func parseRule(item gjson.Result) (Rule, error) {
	rule := Rule{
		Regex:          item.Get("regex").String(),
		Replacement:    item.Get("replacement").String(),
		QueryAppend:    item.Get("query_append").String(),
		QueryTemplate:  item.Get("query_template").String(),
		PassToUpstream: item.Get("pass_to_upstream").Bool(),
		Mode:           strings.ToLower(item.Get("mode").String()),
	}

	if rule.Regex == "" {
		return Rule{}, errors.New("regex is required")
	}
	if rule.Replacement == "" {
		return Rule{}, errors.New("replacement is required")
	}
	if rule.QueryAppend != "" && rule.QueryTemplate != "" {
		return Rule{}, errors.New("query_append and query_template cannot be used together")
	}
	if rule.Mode == "" {
		rule.Mode = ModeLast
	}
	if rule.Mode != ModeBreak && rule.Mode != ModeLast {
		return Rule{}, fmt.Errorf("unsupported mode %q", rule.Mode)
	}

	compiled, err := regexp.Compile(rule.Regex)
	if err != nil {
		return Rule{}, fmt.Errorf("failed to compile regex: %w", err)
	}
	rule.compiled = compiled

	setVars := item.Get("set_vars")
	if setVars.Exists() {
		if !setVars.IsArray() {
			return Rule{}, errors.New("set_vars must be an array")
		}
		rule.SetVars = make([]SetVar, 0, len(setVars.Array()))
		for i, setVarItem := range setVars.Array() {
			setVar := SetVar{
				Name:         setVarItem.Get("name").String(),
				CaptureGroup: int(setVarItem.Get("capture_group").Int()),
			}
			if setVar.Name == "" {
				return Rule{}, fmt.Errorf("set_vars[%d].name is required", i)
			}
			if setVar.CaptureGroup < 0 || setVar.CaptureGroup > compiled.NumSubexp() {
				return Rule{}, fmt.Errorf("set_vars[%d].capture_group=%d out of range", i, setVar.CaptureGroup)
			}
			rule.SetVars = append(rule.SetVars, setVar)
		}
	}

	return rule, nil
}
