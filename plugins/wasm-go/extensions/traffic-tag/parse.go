// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"

	"github.com/tidwall/gjson"
)

var regexCache = map[string]*regexp.Regexp{}

func parseContentConfig(json gjson.Result, config *TrafficTagConfig, log log.Log) error {
	var parseError error
	config.ConditionGroups = []ConditionGroup{}

	json.Get(ConditionGroups).ForEach(func(_, group gjson.Result) bool {
		groupResults := gjson.GetMany(group.Raw, HeaderName, HeaderValue, MatchLogic, Conditions)
		cg := ConditionGroup{
			HeaderName:  groupResults[0].String(),
			HeaderValue: groupResults[1].String(),
			Logic:       strings.ToLower(groupResults[2].String()),
			Conditions:  []ConditionRule{},
		}
		if cg.HeaderName == "" || cg.HeaderValue == "" || cg.Logic == "" || (cg.Logic != "and" && cg.Logic != "or") {
			parseError = fmt.Errorf("invalid condition group: %s, HeaderName: %s, HeaderValue: %s, Logic: %s", group.String(), cg.HeaderName, cg.HeaderValue, cg.Logic)
			return false
		}

		groupResults[3].ForEach(func(_, cond gjson.Result) bool {
			results := gjson.GetMany(cond.Raw, CondKeyType, CondKey, CondMatchType, CondValue)
			c := ConditionRule{
				ConditionType: strings.ToLower(results[0].String()),
				Key:           results[1].String(),
				Operator:      strings.ToLower(results[2].String()),
				Value:         extractStringArray(results[3]),
			}
			parseError = c.validate()
			if parseError != nil {
				return false
			}

			// precompile regex
			if c.Operator == Op_Regex {
				err := compileRegex(c.Value[0])
				if err != nil {
					parseError = err
					return false
				}
			}
			cg.Conditions = append(cg.Conditions, c)
			return true
		})

		config.ConditionGroups = append(config.ConditionGroups, cg)
		return true
	})

	log.Infof("Completed parsing condition config: %v", config.ConditionGroups)
	return parseError
}

func parseWeightConfig(json gjson.Result, config *TrafficTagConfig, log log.Log) error {
	var parseError error
	var accumulatedWeight int64
	config.WeightGroups = []WeightGroup{}

	// parse default tag key and value
	if k, v := json.Get(DefaultTagKey), json.Get(DefaultTagVal); k.Exists() && v.Exists() {
		config.DefaultTagKey = k.String()
		config.DefaultTagVal = v.String()
		log.Debugf("Default tag key: %s, value: %s", config.DefaultTagKey, config.DefaultTagVal)
	}

	json.Get(WeightGroups).ForEach(func(_, header gjson.Result) bool {
		results := gjson.GetMany(header.Raw, HeaderName, HeaderValue, Weight)
		wh := WeightGroup{
			HeaderName:  results[0].String(),
			HeaderValue: results[1].String(),
			Weight:      results[2].Int(),
		}
		if wh.HeaderName == "" || wh.HeaderValue == "" || wh.Weight < 0 || wh.Weight > TotalWeight {
			parseError = errors.New("invalid weight config: " + header.String())
			return false
		}

		if accumulatedWeight += wh.Weight; accumulatedWeight > TotalWeight {
			parseError = errors.New("total weight exceeds: " + strconv.Itoa(TotalWeight))
			return false
		}
		wh.Accumulate = accumulatedWeight
		config.WeightGroups = append(config.WeightGroups, wh)
		return true
	})
	if len(config.WeightGroups) > 0 {
		log.Infof("Completed parsing weight config: %v", config.WeightGroups)
	} else {
		log.Infof("No weight config configured")
	}

	return parseError
}

func compileRegex(pattern string) error {
	if _, exists := regexCache[pattern]; !exists {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return err
		}
		regexCache[pattern] = compiled
		proxywasm.LogDebug("compiled regex: " + pattern)
	}
	return nil
}

func extractStringArray(result gjson.Result) []string {
	var values []string
	for _, v := range result.Array() {
		values = append(values, v.String())
	}
	return values
}

func (c ConditionRule) String() string {
	return fmt.Sprintf("ConditionType: %s, Key: %s, Operator: %s, Value: %v", c.ConditionType, c.Key, c.Operator, c.Value)
}

func (c ConditionRule) validate() error {
	if c.ConditionType == "" {
		return errors.New("conditionType cannot be empty")
	}
	if c.Key == "" {
		return errors.New("key cannot be empty")
	}
	if c.Operator == "" {
		return errors.New("operator cannot be empty")
	}

	var validOperators = map[string]bool{
		Op_Equal:     true,
		Op_NotEqual:  true,
		Op_Prefix:    true,
		Op_In:        true,
		Op_NotIn:     true,
		Op_Regex:     true,
		Op_Percent:   true,
		Op_Exists:    true,
		Op_NotExists: true,
	}
	if !validOperators[c.Operator] {
		return fmt.Errorf("invalid operator: '%s'", c.Operator)
	}

	if c.ConditionType != Type_Header && c.ConditionType != Type_Parameter && c.ConditionType != Type_Cookie {
		return fmt.Errorf("invalid conditionType: '%s'", c.ConditionType)
	}

	switch c.Operator {
	case Op_Exists, Op_NotExists:
		// 不校验Value
		return nil
	case Op_In, Op_NotIn:
		// 至少一个值
		if len(c.Value) < 1 {
			return errors.New("value must contain at least one element for 'in' or 'not_in' operators")
		}
	case Op_Percent:
		// 'percentage' 有且只有一个值，且为0-100之间的整数
		if len(c.Value) != 1 {
			return errors.New("value for 'percentage' must contain exactly one element")
		}
		percent, err := strconv.Atoi(c.Value[0])
		if err != nil {
			return fmt.Errorf("value for 'percentage' must be a valid integer")
		}
		if percent < 0 || percent > 100 {
			return fmt.Errorf("value for 'percentage' must be greater than 0 and less than 100")
		}
	default:
		// 其他操作符只能有一个值
		if len(c.Value) != 1 {
			return fmt.Errorf("value must contain exactly one element for '%s' operator", c.Operator)
		}
	}

	return nil
}
