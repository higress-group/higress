package utils

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// 原子表达式描述:
// eq arg1 arg2: arg1 == arg2时为true
// ne arg1 arg2: arg1 != arg2时为true
// lt arg1 arg2: arg1 < arg2时为true
// le arg1 arg2: arg1 <= arg2时为true
// gt arg1 arg2: arg1 > arg2时为true
// ge arg1 arg2: arg1 >= arg2时为true
// and arg1 arg2: arg1 && arg2
// or arg1 arg2: arg1 || arg2
// contain arg1 arg2: arg1 包含 arg2时为true
var conditionalExpressionRegex = regexp.MustCompile(`\(([^()]*)\)`)
var templateExpressionRegex = regexp.MustCompile(`\{\{(.*?)\}\}`)

var operators = map[string]interface{}{
	"eq": func(a, b interface{}) bool {
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	},
	"ge":      func(a, b float64) bool { return a >= b },
	"le":      func(a, b float64) bool { return a <= b },
	"gt":      func(a, b float64) bool { return a > b },
	"lt":      func(a, b float64) bool { return a < b },
	"and":     func(a, b bool) bool { return a && b },
	"or":      func(a, b bool) bool { return a || b },
	"contain": func(a, b string) bool { return strings.Contains(a, b) },
}

// 执行判断条件
func ExecConditionalStr(conditionalStr string) (bool, error) {
	// 正则表达式匹配括号内的表达式
	matches := conditionalExpressionRegex.FindAllStringSubmatch(conditionalStr, -1)
	// 找到最里面的(原子表达式)
	for _, match := range matches {
		subCondition := match[1]
		result, err := ExecConditionalStr(subCondition)
		if err != nil {
			return false, err
		}
		// 用结果替换原子表达式
		conditionalStr = strings.ReplaceAll(conditionalStr, match[0], fmt.Sprintf("%t", result))
	}

	fields := strings.Fields(conditionalStr)
	// 执行原子表达式
	if len(fields) == 3 {
		compareFunc := operators[fields[0]]
		switch fc := compareFunc.(type) {
		default:
			return false, fmt.Errorf("invalid conditional func %v", compareFunc)
		case func(a, b float64) bool:
			a, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				return false, fmt.Errorf("invalid conditional str %s", conditionalStr)
			}
			b, err := strconv.ParseFloat(fields[2], 64)
			if err != nil {
				return false, fmt.Errorf("invalid conditional str %s", conditionalStr)
			}
			return fc(a, b), nil
		case func(a, b bool) bool:
			a, err := strconv.ParseBool(fields[1])
			if err != nil {
				return false, fmt.Errorf("invalid conditional str %s", conditionalStr)
			}
			b, err := strconv.ParseBool(fields[2])
			if err != nil {
				return false, fmt.Errorf("invalid conditional str %s", conditionalStr)
			}
			return fc(a, b), nil
		case func(a, b string) bool:
			a := fields[1]
			b := fields[2]
			return fc(a, b), nil
		case func(a, b interface{}) bool:
			a := fields[1]
			b := fields[2]
			return fc(a, b), nil
		}
		// 继续获取上一层的(原子表达式)
	} else if strings.Contains(conditionalStr, "(") || strings.Contains(conditionalStr, ")") {
		return ExecConditionalStr(conditionalStr)
		// 原子表达式有问题，返回
	} else {
		return false, fmt.Errorf("invalid conditional str %s", conditionalStr)
	}

}

// 通过正则表达式寻找模板中的 ｛｛foo｝｝ 字符串foo
// 返回 {{foo}} : foo
func ParseTmplStr(tmpl string) map[string]string {
	result := make(map[string]string)
	matches := templateExpressionRegex.FindAllStringSubmatch(tmpl, -1)
	for _, match := range matches {
		result[match[0]] = match[1]
	}
	return result
}

// 使用kv替换模板中的字符
// 例如 模板是`hello,{{foo}}` 使用{"{{foo}}":"bot"} 替换后为`hello,bot`
func ReplacedStr(tmpl string, kvs map[string]string) string {

	for k, v := range kvs {
		tmpl = strings.Replace(tmpl, k, v, -1)
	}

	return tmpl
}
