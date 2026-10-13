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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeGraphQLConfig_SetGql(t *testing.T) {
	tests := []struct {
		name          string
		gql           string
		wantVariables []Variable
		wantErr       error
	}{
		{
			name:    "empty gql",
			gql:     "",
			wantErr: errors.New("gql can't be empty"),
		},
		{
			name:          "no params",
			gql:           "query",
			wantVariables: []Variable{},
			wantErr:       nil,
		},
		{
			name:    "four params",
			gql:     "query ($owner:String $num:Float! $int : Int! $boolean : Boolean  )",
			wantErr: nil,
			wantVariables: []Variable{
				{
					name:  "owner",
					typ:   StringType,
					blank: true,
				},
				{
					name:  "num",
					typ:   FloatType,
					blank: false,
				},
				{
					name:  "int",
					typ:   IntType,
					blank: false,
				},
				{
					name:  "boolean",
					typ:   BooleanType,
					blank: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DeGraphQLConfig{}
			err := d.SetGql(tt.gql)
			assert.Equal(t, tt.wantErr, err)
			if err != nil {
				return
			}
			assert.Equal(t, tt.wantVariables, d.variables)
		})
	}
}

func TestDeGraphQLConfig_ParseGqlFromUrl(t *testing.T) {

	tests := []struct {
		name    string
		gql     string
		url     string
		want    string
		wantErr error
	}{
		{
			name:    "empty url",
			gql:     "query ($owner:String! $name:String!)",
			url:     "",
			want:    "",
			wantErr: errors.New("request url can't be empty"),
		},

		{
			name:    "no params",
			gql:     "query HeroNameQuery {\n  hero {\n    name\n  }\n}",
			url:     "/api?owner=a",
			want:    "{\"query\":\"query HeroNameQuery {\\n  hero {\\n    name\\n  }\\n}\"}",
			wantErr: nil,
		},

		{
			name:    "one string variable",
			gql:     "query FetchSomeIDQuery($someId: String!) {\n  human(id: $someId) {\n    name\n  }\n}",
			url:     "/api?someId=a",
			want:    "{\"query\":\"query FetchSomeIDQuery($someId: String!) {\\n  human(id: $someId) {\\n    name\\n  }\\n}\",\"variables\":{\"someId\":\"a\"}}",
			wantErr: nil,
		},

		{
			name:    "multi variables",
			gql:     "query FetchSomeIDQuery($someId: String! $num: Int $price: Float! $need:Boolean!) {\n  human(id: $someId) {\n    name\n  }\n}",
			url:     "/api?someId=a&num=10&price=12.0&need=false&hee=1",
			want:    "{\"query\":\"query FetchSomeIDQuery($someId: String! $num: Int $price: Float! $need:Boolean!) {\\n  human(id: $someId) {\\n    name\\n  }\\n}\",\"variables\":{\"need\":false,\"num\":10,\"price\":12,\"someId\":\"a\"}}",
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DeGraphQLConfig{}
			d.SetGql(tt.gql)
			body, err := d.ParseGqlFromUrl(tt.url)
			assert.Equal(t, tt.wantErr, err)
			if err != nil {
				return
			}
			assert.Equal(t, tt.want, body)
		})
	}
}

func TestDeGraphQLConfig_SetEndpoint(t *testing.T) {

	tests := []struct {
		name     string
		endPoint string
		wantErr  error
		want     string
	}{
		{
			name:     "empty endpoint",
			endPoint: "",
			wantErr:  nil,
			want:     "/graphql",
		},
		{
			name:     "empty endpoint with blank",
			endPoint: "   ",
			wantErr:  nil,
			want:     "/graphql",
		},

		{
			name:     "with value",
			endPoint: " /graphql2 ",
			wantErr:  nil,
			want:     "/graphql2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DeGraphQLConfig{}
			err := d.SetEndpoint(tt.endPoint)
			assert.Equal(t, tt.wantErr, err)
			if err != nil {
				return
			}
			assert.Equal(t, tt.want, d.endpoint)
		})
	}
}

// TestDeGraphQLConfig_ParseGqlFromUrl_CoercesDeclaredTypes checks that the value
// supplied for each variable is rendered as the JSON scalar for the type the
// gql operation declares, instead of being copied into the body verbatim.
func TestDeGraphQLConfig_ParseGqlFromUrl_CoercesDeclaredTypes(t *testing.T) {
	tests := []struct {
		name string
		gql  string
		url  string
		want string
	}{
		{
			name: "int",
			gql:  "query ($id: Int!) { item(id: $id) { id } }",
			url:  "/api?id=42",
			want: "{\"query\":\"query ($id: Int!) { item(id: $id) { id } }\",\"variables\":{\"id\":42}}",
		},
		{
			name: "float",
			gql:  "query ($score: Float!) { item(score: $score) { id } }",
			url:  "/api?score=1.5",
			want: "{\"query\":\"query ($score: Float!) { item(score: $score) { id } }\",\"variables\":{\"score\":1.5}}",
		},
		{
			name: "boolean",
			gql:  "query ($enabled: Boolean!) { item(enabled: $enabled) { id } }",
			url:  "/api?enabled=true",
			want: "{\"query\":\"query ($enabled: Boolean!) { item(enabled: $enabled) { id } }\",\"variables\":{\"enabled\":true}}",
		},
		{
			name: "string keeps separators as data",
			gql:  "query ($title: String!) { item(title: $title) { id } }",
			url:  `/api?title=x,"admin":true`,
			want: `{"query":"query ($title: String!) { item(title: $title) { id } }","variables":{"title":"x,\"admin\":true"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DeGraphQLConfig{}
			require.NoError(t, d.SetGql(tt.gql))
			body, err := d.ParseGqlFromUrl(tt.url)
			require.NoError(t, err)
			assert.True(t, json.Valid([]byte(body)), "body must be valid JSON: %s", body)
			assert.Equal(t, tt.want, body)
		})
	}
}

// TestDeGraphQLConfig_ParseGqlFromUrl_RejectsUnrepresentableValues covers the
// values the previous implementation copied into the body unchanged. A variable
// declared as Int, Float or Boolean must only accept a value that really is
// that scalar, otherwise a client could append extra members to, or close, the
// variables object and rewrite the operation the operator configured.
func TestDeGraphQLConfig_ParseGqlFromUrl_RejectsUnrepresentableValues(t *testing.T) {
	const intGql = "query ($id: Int! $enabled: Boolean!) { item(id: $id, enabled: $enabled) { id } }"
	const floatGql = "query ($score: Float! $enabled: Boolean!) { item(score: $score, enabled: $enabled) { id } }"

	tests := []struct {
		name string
		gql  string
		url  string
	}{
		{name: "int carrying an extra member", gql: intGql, url: `/api?id=1,"admin":true&enabled=true`},
		{name: "int closing the variables object", gql: intGql, url: `/api?id=1}&enabled=true`},
		{name: "int that is not numeric", gql: intGql, url: `/api?id=abc&enabled=true`},
		{name: "int joined from repeated parameters", gql: intGql, url: `/api?id=1&id=2&enabled=true`},
		{name: "boolean carrying an extra member", gql: intGql, url: `/api?id=1&enabled=true,"admin":true`},
		{name: "boolean that is not parseable", gql: intGql, url: `/api?id=1&enabled=yes`},
		{name: "float that is not numeric", gql: floatGql, url: `/api?score=abc&enabled=true`},
		{name: "float carrying an extra member", gql: floatGql, url: `/api?score=1.0,"admin":true&enabled=true`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DeGraphQLConfig{}
			require.NoError(t, d.SetGql(tt.gql))
			body, err := d.ParseGqlFromUrl(tt.url)
			require.Error(t, err)
			assert.Empty(t, body)
		})
	}
}

// TestDeGraphQLConfig_ParseGqlFromUrl_OnlyEmitsDeclaredVariables asserts that
// the generated document keeps exactly the declared variables. A String
// variable accepts the bytes below, but only as string data: the quotes, commas
// and braces stay inside the value instead of ending the string and adding
// members to the surrounding objects.
func TestDeGraphQLConfig_ParseGqlFromUrl_OnlyEmitsDeclaredVariables(t *testing.T) {
	const gql = "query ($title: String!) { item(title: $title) { id } }"

	d := &DeGraphQLConfig{}
	require.NoError(t, d.SetGql(gql))
	body, err := d.ParseGqlFromUrl(`/api?title=x","admin":true,"z":"`)
	require.NoError(t, err)
	require.True(t, json.Valid([]byte(body)), "body must be valid JSON: %s", body)

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(body), &top))
	require.Len(t, top, 2)
	require.Contains(t, top, "query")
	require.Contains(t, top, "variables")

	var variables map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["variables"], &variables))
	require.Len(t, variables, 1)
	require.Contains(t, variables, "title")

	var title string
	require.NoError(t, json.Unmarshal(variables["title"], &title))
	assert.Equal(t, `x","admin":true,"z":"`, title)
}

// TestDeGraphQLConfig_ParseGqlFromUrl_EscapesStringValues keeps a regression
// test for the backslash case: the old getJsonStr only escaped double quotes
// and newlines, so a value ending in a backslash turned the appended closing
// quote into an escaped quote and produced a malformed document.
func TestDeGraphQLConfig_ParseGqlFromUrl_EscapesStringValues(t *testing.T) {
	const gql = "query ($title: String!) { item(title: $title) { id } }"

	tests := []struct {
		name  string
		url   string
		value string
	}{
		{name: "plain", url: "/api?title=plain", value: "plain"},
		{name: "trailing backslash", url: `/api?title=foo\`, value: `foo\`},
		{name: "embedded backslash and colon", url: `/api?title=C:\temp\new`, value: `C:\temp\new`},
		{name: "double quote", url: `/api?title=a"b`, value: `a"b`},
		{name: "newline", url: "/api?title=line1%0Aline2", value: "line1\nline2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DeGraphQLConfig{}
			require.NoError(t, d.SetGql(gql))
			body, err := d.ParseGqlFromUrl(tt.url)
			require.NoError(t, err)
			require.True(t, json.Valid([]byte(body)), "body must be valid JSON: %s", body)

			var decoded struct {
				Query     string         `json:"query"`
				Variables map[string]any `json:"variables"`
			}
			require.NoError(t, json.Unmarshal([]byte(body), &decoded))
			assert.Equal(t, gql, decoded.Query)
			assert.Equal(t, map[string]any{"title": tt.value}, decoded.Variables)
		})
	}
}
