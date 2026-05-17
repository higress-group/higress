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
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// 将 multipart 字节按 part name 排序后重新序列化（使用 \r\n）
func sortMultipartBody(data []byte, boundary string) []byte {
	reader := multipart.NewReader(bytes.NewReader(data), boundary)
	type Part struct {
		Name        string
		Filename    string
		ContentType string
		Content     []byte
	}
	var parts []Part

	for {
		p, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		content, _ := io.ReadAll(p)
		parts = append(parts, Part{
			Name:        p.FormName(),
			Filename:    p.FileName(),
			ContentType: p.Header.Get("Content-Type"),
			Content:     content,
		})
	}

	// 按 Name 排序
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].Name < parts[j].Name
	})

	// 重新组装（严格用 \r\n）
	var buf bytes.Buffer
	for _, p := range parts {
		buf.WriteString("--" + boundary + "\r\n")
		if p.Filename != "" {
			buf.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"%s\"; filename=\"%s\"\r\n", p.Name, p.Filename))
			if p.ContentType != "" {
				buf.WriteString("Content-Type: " + p.ContentType + "\r\n")
			}
		} else {
			buf.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"%s\"\r\n", p.Name))
		}
		buf.WriteString("\r\n")
		buf.Write(p.Content)
		buf.WriteString("\r\n")
	}
	buf.WriteString("--" + boundary + "--\r\n")
	return buf.Bytes()
}

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
		{
			name:      "multipart/form-data with file",
			mediaType: "multipart/form-data; boundary=--------------------------180978275079165582161528",
			body:      []byte("----------------------------180978275079165582161528\r\nContent-Disposition: form-data; name=\"file1\"; filename=\"test.txt\"\r\nContent-Type: text/plain\r\n\r\n这是一个txt文件\r\n----------------------------180978275079165582161528\r\nContent-Disposition: form-data; name=\"file2\"; filename=\"test.txt\"\r\nContent-Type: text/plain\r\n\r\n这是一个txt文件\r\n----------------------------180978275079165582161528--\r\n"),
			expected: map[string][]string{
				"file1":              {""},
				"file1.content":      {"6L+Z5piv5LiA5LiqdHh05paH5Lu2"},
				"file1.content-type": {"text/plain"},
				"file1.filename":     {"test.txt"},
				"file2":              {""},
				"file2.content":      {"6L+Z5piv5LiA5LiqdHh05paH5Lu2"},
				"file2.content-type": {"text/plain"},
				"file2.filename":     {"test.txt"},
			},
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
			name:      "multipart/form-data with file",
			mediaType: "multipart/form-data; boundary=--------------------------180978275079165582161528",
			body: map[string][]string{
				"X-process":          {"wasm"},
				"file1":              {},
				"file1.content":      {"6L+Z5piv5LiA5LiqdHh05paH5Lu2"},
				"file1.content-type": {"text/plain"},
				"file1.filename":     {"test.txt"},
				"file2":              {},
				"file2.content":      {"6L+Z5piv5LiA5LiqdHh05paH5Lu2"},
				"file2.content-type": {"text/plain"},
				"file2.filename":     {"test.txt"},
			},
			expected: []byte(
				"----------------------------180978275079165582161528\r\n" +
					"Content-Disposition: form-data; name=\"X-process\"\r\n" +
					"\r\n" +
					"wasm\r\n" +
					"----------------------------180978275079165582161528\r\n" +
					"Content-Disposition: form-data; name=\"file1\"; filename=\"test.txt\"\r\n" +
					"Content-Type: text/plain\r\n" +
					"\r\n" +
					"这是一个txt文件\r\n" +
					"----------------------------180978275079165582161528\r\n" +
					"Content-Disposition: form-data; name=\"file2\"; filename=\"test.txt\"\r\n" +
					"Content-Type: text/plain\r\n" +
					"\r\n" +
					"这是一个txt文件\r\n" +
					"----------------------------180978275079165582161528--\r\n",
			),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := constructBody(c.mediaType, c.body)
			if c.errMsg != "" {
				require.EqualError(t, err, c.errMsg)
				return
			}

			if "multipart/form-data with file" == c.name {
				boundary := "--------------------------180978275079165582161528"
				// 进行排序，解决map顺序不确定导致比较失败的问题
				actual = sortMultipartBody(actual, boundary)
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
