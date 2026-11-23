// Copyright (c) 2024 Alibaba Group Holding Ltd.
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
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// legacyEscapeCacheValue is the previous implementation, kept here so the
// compatibility assertions below describe the exact output that deployments
// observe today.
func legacyEscapeCacheValue(value string) string {
	return strings.Trim(strconv.Quote(value), "\"")
}

func TestEscapeCacheValue(t *testing.T) {
	// Inputs without a double quote or a control character must render exactly
	// as before, so a cache hit is byte-identical to the pre-fix output.
	byteIdentical := []string{
		"",
		"plain text",
		"中文内容与标点：，。",
		"a < b && c > d",
		`back\slash`,
		"emoji 🙂 and combining é",
		"tab\tand newline\n",
	}
	for _, value := range byteIdentical {
		require.Equal(t, legacyEscapeCacheValue(value), escapeCacheValue(value),
			"input %q must stay byte-identical", value)
	}

	// Every input must produce a JSON string literal that decodes back to the
	// original value.
	roundTrip := []string{
		"",
		"plain text",
		"中文内容与标点：，。",
		"a < b && c > d",
		`back\slash`,
		`他说："你好"`,
		`"`,
		`""`,
		`a"b"c"`,
		`"quoted start`,
		"line1\nline2\ttab",
		"\x00\x01\x1f",
		"emoji 🙂",
	}
	for _, value := range roundTrip {
		escaped := escapeCacheValue(value)
		var decoded string
		require.NoErrorf(t, json.Unmarshal([]byte(`"`+escaped+`"`), &decoded),
			"input %q produced a non-JSON fragment: %q", value, escaped)
		require.Equal(t, value, decoded, "input %q must round-trip", value)
	}
}

func TestEscapeCacheValueControlCharactersUseJSONEscapes(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  string
	}{
		{"\x00", `\u0000`},
		{"\x01", `\u0001`},
		{"\x1f", `\u001f`},
		{"a\x00b", `a\u0000b`},
		{`"`, `\"`},
		{`\`, `\\`},
		{"\n", `\n`},
		{"\r", `\r`},
		{"\t", `\t`},
	} {
		require.Equal(t, tc.want, escapeCacheValue(tc.value),
			"input %q must use JSON escapes", tc.value)
	}
}
