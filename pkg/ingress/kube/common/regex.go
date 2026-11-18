// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import "fmt"

// ValidateRegexPath rejects adjacent wildcard operators in a regex ingress
// path. Envoy's C++ RE2 parser reports these as "bad repetition operator: **",
// causing the complete route configuration to be rejected.
//
// This check is intentionally lexical instead of using Go's regexp package:
// Go and Envoy support different RE2 syntax, so compiling with Go can reject
// valid Envoy expressions such as \C.
func ValidateRegexPath(pathType PathType, path string) error {
	switch pathType {
	case PrefixRegex, FullPathRegex:
		if hasAdjacentWildcardOperators(path) {
			return fmt.Errorf("invalid adjacent wildcard operators in Envoy RE2 expression")
		}
	}
	return nil
}

func hasAdjacentWildcardOperators(expression string) bool {
	previousWasWildcard := false
	for i := 0; i < len(expression); {
		switch expression[i] {
		case '\\':
			if i+1 < len(expression) && expression[i+1] == 'Q' {
				i = indexAfterQuotedLiteral(expression, i+2)
			} else if i+1 < len(expression) {
				i += 2
			} else {
				i++
			}
			previousWasWildcard = false
		case '[':
			i = indexAfterCharacterClass(expression, i+1)
			previousWasWildcard = false
		case '*':
			if previousWasWildcard {
				return true
			}
			previousWasWildcard = true
			i++
		default:
			previousWasWildcard = false
			i++
		}
	}
	return false
}

func indexAfterQuotedLiteral(expression string, start int) int {
	for i := start; i < len(expression); i++ {
		if expression[i] == '\\' && i+1 < len(expression) && expression[i+1] == 'E' {
			return i + 2
		}
	}
	return len(expression)
}

func indexAfterCharacterClass(expression string, start int) int {
	i := start
	if i < len(expression) && expression[i] == '^' {
		i++
	}
	if i < len(expression) && expression[i] == ']' {
		i++
	}

	for i < len(expression) {
		switch expression[i] {
		case '\\':
			if i+1 < len(expression) {
				i += 2
			} else {
				i++
			}
		case '[':
			if i+1 < len(expression) && expression[i+1] == ':' {
				for i += 2; i+1 < len(expression); i++ {
					if expression[i] == ':' && expression[i+1] == ']' {
						i += 2
						break
					}
				}
			} else {
				i++
			}
		case ']':
			return i + 1
		default:
			i++
		}
	}
	return len(expression)
}
