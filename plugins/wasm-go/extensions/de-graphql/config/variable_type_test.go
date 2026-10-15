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

package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const typedGql = "query FetchSomeIDQuery($someId: String! $num: Int $price: Float! $need: Boolean!) {\n  human(id: $someId) {\n    name\n  }\n}"

// TestParseGqlFromUrlRejectsMistypedVariables covers variable values that do not match
// their declared GraphQL type. They used to be concatenated into the variables object
// verbatim, which let a caller inject extra variables or send a body that is not valid
// JSON at all.
func TestParseGqlFromUrlRejectsMistypedVariables(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "int variable carrying injected json", url: `/api?someId=a&num=1,"isAdmin":true&price=1.0&need=false`},
		{name: "int variable that is not a number", url: `/api?someId=a&num=notanumber&price=1.0&need=false`},
		{name: "int variable with json string", url: `/api?someId=a&num="10"&price=1.0&need=false`},
		{name: "float variable that is not a number", url: `/api?someId=a&num=1&price=Inf&need=false`},
		{name: "float variable with plus sign", url: `/api?someId=a&num=1&price=+1&need=false`},
		{name: "float variable with trailing dot", url: `/api?someId=a&num=1&price=1.&need=false`},
		{name: "boolean variable that is not a boolean", url: `/api?someId=a&num=1&price=1.0&need=yes`},
		{name: "boolean variable with quotes", url: `/api?someId=a&num=1&price=1.0&need="false"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DeGraphQLConfig{}
			require.NoError(t, d.SetGql(typedGql))

			body, err := d.ParseGqlFromUrl(tt.url)
			require.Error(t, err, "a mistyped variable must be rejected instead of reaching the upstream")
			assert.Empty(t, body)
		})
	}
}

// TestParseGqlFromUrlKeepsValuesForValidInput pins the exact bodies produced for values
// that were already accepted, including the caller's numeric notation.
func TestParseGqlFromUrlKeepsValuesForValidInput(t *testing.T) {
	d := &DeGraphQLConfig{}
	require.NoError(t, d.SetGql(typedGql))

	body, err := d.ParseGqlFromUrl("/api?someId=a&num=10&price=12.0&need=false&hee=1")
	require.NoError(t, err)
	assert.Equal(t,
		"{\"query\":\"query FetchSomeIDQuery($someId: String! $num: Int $price: Float! $need: Boolean!) {\\n  human(id: $someId) {\\n    name\\n  }\\n}\",\"variables\":{\"someId\":\"a\",\"num\":10,\"price\":12.0,\"need\":false}}",
		body)
}

// TestParseGqlFromUrlEscapesStringValues covers String variables whose value would break
// the hand-rolled escaping: a trailing backslash used to make the body invalid JSON, and
// an embedded quote used to let the value close its own string and add variables.
func TestParseGqlFromUrlEscapesStringValues(t *testing.T) {
	cases := []struct {
		name      string
		url       string
		wantValue string
	}{
		{name: "trailing backslash", url: `/api?someId=a\&num=1&price=1.0&need=false`, wantValue: `a\`},
		{name: "backslash before quote", url: `/api?someId=a\"&num=1&price=1.0&need=false`, wantValue: `a\"`},
		{name: "quote and injected field", url: `/api?someId=a","injected":"x&num=1&price=1.0&need=false`, wantValue: `a","injected":"x`},
		{name: "newline", url: "/api?someId=a%0Ab&num=1&price=1.0&need=false", wantValue: "a\nb"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &DeGraphQLConfig{}
			require.NoError(t, d.SetGql(typedGql))

			body, err := d.ParseGqlFromUrl(tc.url)
			require.NoError(t, err)

			var parsed struct {
				Variables map[string]any `json:"variables"`
			}
			require.NoError(t, json.Unmarshal([]byte(body), &parsed), "the produced body must be valid JSON: %s", body)
			assert.Equal(t, tc.wantValue, parsed.Variables["someId"])
			assert.Len(t, parsed.Variables, 4, "no extra variable may be injected: %s", body)
		})
	}
}
