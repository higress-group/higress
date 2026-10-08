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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseQueryByPath(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		expected map[string][]string
		errMsg   string
	}{
		{
			name: "common",
			path: "/get?k1=v1&k2=v2&k3=v3",
			expected: map[string][]string{
				"k1": {"v1"},
				"k2": {"v2"},
				"k3": {"v3"},
			},
		},
		{
			name:     "empty query",
			path:     "www.example.com/get",
			expected: map[string][]string{},
		},
		{
			name: "multiple values",
			path: "www.example.com/get?k1=v11&k1=v12&k2=v2&k1=v13",
			expected: map[string][]string{
				"k1": {"v11", "v12", "v13"},
				"k2": {"v2"},
			},
		},
		{
			name: "encoded url",
			path: "/get%20with%3Freserved%20characters?key=Hello+World",
			expected: map[string][]string{
				"key": {"Hello World"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := parseQueryByPath(c.path)
			if c.errMsg != "" {
				require.EqualError(t, err, c.errMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestConstructPath(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		qs       map[string][]string
		expected string
		errMsg   string
	}{
		{
			name: "common",
			path: "/get",
			qs: map[string][]string{
				"k1": {"v1"},
				"k2": {"v2"},
				"k3": {"v3"},
			},
			expected: "/get?k1=v1&k2=v2&k3=v3",
		},
		{
			name:     "empty query",
			path:     "www.example.com/get",
			qs:       map[string][]string{},
			expected: "www.example.com/get",
		},
		{
			name: "encoded url",
			path: "/get with?",
			qs: map[string][]string{
				"key": {"Hello World"},
			},
			expected: "/get%20with?key=Hello+World",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := constructPath(c.path, c.qs)
			if c.errMsg != "" {
				require.EqualError(t, err, c.errMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestParseBody(t *testing.T) {
	cases := []struct {
		name      string
		mediaType string
		body      []byte
		expected  interface{}
		errMsg    string
	}{
		{
			name:      "application/json",
			mediaType: "application/json",
			body: []byte(`{
  "k1": "v2",
  "k2": 20,
  "k3": true,
  "k4": [1, 2, 3],
  "k5": {
    "k6": "v6"
  }
}`),
			expected: map[string]interface{}{"body": []byte(`{
  "k1": "v2",
  "k2": 20,
  "k3": true,
  "k4": [1, 2, 3],
  "k5": {
    "k6": "v6"
  }
}`)},
		},
		{
			name:      "application/x-www-form-urlencoded",
			mediaType: "application/x-www-form-urlencoded",
			body:      []byte("k1=v11&k1=v12&k2=v2&k3=v3"),
			expected: map[string][]string{
				"k1": {"v11", "v12"},
				"k2": {"v2"},
				"k3": {"v3"},
			},
		},
		{
			name:      "multipart/form-data",
			mediaType: "multipart/form-data; boundary=--------------------------962785348548682888818907",
			body:      []byte("----------------------------962785348548682888818907\r\nContent-Disposition: form-data; name=\"k1\"\r\n\r\nv11\r\n----------------------------962785348548682888818907\r\nContent-Disposition: form-data; name=\"k1\"\r\n\r\nv12\r\n----------------------------962785348548682888818907\r\nContent-Disposition: form-data; name=\"k2\"\r\n\r\nv2\r\n----------------------------962785348548682888818907--\r\n"),
			expected: map[string][]string{
				"k1": {"v11", "v12"},
				"k2": {"v2"},
			},
		},
		{
			name:      "unsupported content type",
			mediaType: "plain/text",
			body:      []byte(`qwe`),
			errMsg:    fmt.Sprintf(errContentTypeFmt, "plain/text"),
		},
		{
			name:      "empty body",
			mediaType: "application/json",
			body:      []byte(``),
			errMsg:    errEmptyBody.Error(),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := parseBody(c.mediaType, c.body)
			if c.errMsg != "" {
				require.EqualError(t, err, c.errMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestConstructBody(t *testing.T) {
	cases := []struct {
		name      string
		mediaType string
		body      interface{}
		expected  []byte
		errMsg    string
	}{
		{
			name:      "application/json",
			mediaType: "application/json",
			body: map[string]interface{}{"body": []byte(`{
  "k1": {
    "k2": [1, 2, 3]
  }
}
`)},
			expected: []byte(`{
  "k1": {
    "k2": [1, 2, 3]
  }
}
`),
		},
		{
			name:      "application/x-www-form-urlencoded",
			mediaType: "application/x-www-form-urlencoded",
			body: map[string][]string{
				"k1": {"v11", "v12"},
			},
			expected: []byte("k1=v11&k1=v12"),
		},
		{
			name:      "multipart/form-data",
			mediaType: "multipart/form-data; boundary=--------------------------962785348548682888818907",
			body: map[string][]string{
				"k1": {"v11", "v12"},
			},
			expected: []byte("----------------------------962785348548682888818907\r\nContent-Disposition: form-data; name=\"k1\"\r\n\r\nv11\r\n----------------------------962785348548682888818907\r\nContent-Disposition: form-data; name=\"k1\"\r\n\r\nv12\r\n----------------------------962785348548682888818907--\r\n"),
		},
		{
			name:      "unsupported media type",
			mediaType: "plain/text",
			body:      []byte(`qwe`),
			errMsg:    fmt.Sprintf(errContentTypeFmt, "plain/text"),
		},
		{
			name:      "empty body",
			mediaType: "application/json",
			body:      map[string]interface{}{"body": []byte{}},
			expected:  []byte{},
		},
		{
			name:      "invalid body type for application/json",
			mediaType: "application/json",
			body:      "invalid string body",
			errMsg:    errBodyType.Error(),
		},
		{
			name:      "invalid body content for application/json",
			mediaType: "application/json",
			body:      map[string]interface{}{"body": 123},
			errMsg:    errBodyType.Error(),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := constructBody(c.mediaType, c.body)
			if c.errMsg != "" {
				require.EqualError(t, err, c.errMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestConvertByJsonType(t *testing.T) {
	cases := []struct {
		name     string
		valueTyp string
		value    string
		expected interface{}
		errMsg   string
	}{
		{
			name:     "object",
			valueTyp: "object",
			value:    "{\"array\": [1, 2, 3],   \"object\": {     \"first\": \"hello\",     \"second\": \"world\"   } }",
			expected: map[string]interface{}{
				"array": []interface{}{float64(1), float64(2), float64(3)},
				"object": map[string]interface{}{
					"first":  "hello",
					"second": "world",
				},
			},
		},
		{
			name:     "boolean",
			valueTyp: "boolean",
			value:    "true",
			expected: true,
		},
		{
			name:     "boolean: failed",
			valueTyp: "boolean",
			value:    "null",
			errMsg:   "strconv.ParseBool: parsing \"null\": invalid syntax",
		},
		{
			name:     "number",
			valueTyp: "number",
			value:    "10",
			expected: float64(10),
		},
		{
			name:     "string",
			valueTyp: "string",
			value:    "hello world",
			expected: "hello world",
		},
		{
			name:     "unsupported type",
			valueTyp: "integer",
			value:    "10",
			expected: "10", // default string
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := convertByJsonType(c.valueTyp, c.value)
			if c.errMsg != "" {
				require.EqualError(t, err, c.errMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.expected, actual)
		})
	}
}
