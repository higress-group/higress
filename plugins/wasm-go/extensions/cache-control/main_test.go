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
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：基本配置（数字过期时间）
var basicConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"suffix":  "jpg|png|jpeg",
		"expires": "3600",
	})
	return data
}()

// 测试配置：最大缓存时间配置
var maxExpiresConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"suffix":  "css|js",
		"expires": "max",
	})
	return data
}()

// 测试配置：不缓存配置
var epochExpiresConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"suffix":  "html|htm",
		"expires": "epoch",
	})
	return data
}()

// 测试配置：无后缀限制配置
var noSuffixConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"expires": "7200",
	})
	return data
}()

// 测试配置：单后缀配置
var singleSuffixConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"suffix":  "pdf",
		"expires": "1800",
	})
	return data
}()

// 测试配置：空后缀配置
var emptySuffixConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]string{
		"suffix":  "",
		"expires": "3600",
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本配置解析
		t.Run("basic config", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试最大缓存时间配置解析
		t.Run("max expires config", func(t *testing.T) {
			host, status := test.NewTestHost(maxExpiresConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试不缓存配置解析
		t.Run("epoch expires config", func(t *testing.T) {
			host, status := test.NewTestHost(epochExpiresConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试无后缀限制配置解析
		t.Run("no suffix config", func(t *testing.T) {
			host, status := test.NewTestHost(noSuffixConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试单后缀配置解析
		t.Run("single suffix config", func(t *testing.T) {
			host, status := test.NewTestHost(singleSuffixConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试空后缀配置解析
		t.Run("empty suffix config", func(t *testing.T) {
			host, status := test.NewTestHost(emptySuffixConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试基本请求头处理（带查询参数）
		t.Run("request headers with query params", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含查询参数
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/images/photo.jpg?size=large"},
				{":method", "GET"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试请求头处理（无查询参数）
		t.Run("request headers without query params", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，不包含查询参数
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/images/photo.png"},
				{":method", "GET"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试请求头处理（复杂路径）
		t.Run("request headers with complex path", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含复杂路径
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/static/css/main.css?v=1.0.0&theme=dark"},
				{":method", "GET"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}

func TestOnHttpResponseHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试匹配后缀的响应头处理（数字过期时间）
		t.Run("matching suffix with numeric expires", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/images/photo.jpg"},
				{":method", "GET"},
			})

			// 处理响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "image/jpeg"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了缓存控制头
			responseHeaders := host.GetResponseHeaders()
			require.True(t, test.HasHeader(responseHeaders, "expires"))
			require.True(t, test.HasHeaderWithValue(responseHeaders, "cache-control", "max-age=3600"))

			host.CompleteHttp()
		})

		// 测试匹配后缀的响应头处理（最大缓存时间）
		t.Run("matching suffix with max expires", func(t *testing.T) {
			host, status := test.NewTestHost(maxExpiresConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/static/main.css"},
				{":method", "GET"},
			})

			// 处理响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/css"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了缓存控制头
			responseHeaders := host.GetResponseHeaders()
			require.True(t, test.HasHeader(responseHeaders, "expires"))
			require.True(t, test.HasHeaderWithValue(responseHeaders, "cache-control", "max-age=315360000"))

			host.CompleteHttp()
		})

		// 测试匹配后缀的响应头处理（不缓存）
		t.Run("matching suffix with epoch expires", func(t *testing.T) {
			host, status := test.NewTestHost(epochExpiresConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/page.html"},
				{":method", "GET"},
			})

			// 处理响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/html"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了缓存控制头
			responseHeaders := host.GetResponseHeaders()
			require.True(t, test.HasHeader(responseHeaders, "expires"))
			require.True(t, test.HasHeaderWithValue(responseHeaders, "cache-control", "no-cache"))

			host.CompleteHttp()
		})

		// 测试不匹配后缀的响应头处理
		t.Run("non-matching suffix", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data.json"},
				{":method", "GET"},
			})

			// 处理响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 验证是否没有添加缓存控制头
			responseHeaders := host.GetResponseHeaders()
			require.False(t, test.HasHeader(responseHeaders, "expires"))
			require.False(t, test.HasHeader(responseHeaders, "cache-control"))

			host.CompleteHttp()
		})

		// 测试无后缀限制的响应头处理
		t.Run("no suffix restriction", func(t *testing.T) {
			host, status := test.NewTestHost(noSuffixConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/any/file.txt"},
				{":method", "GET"},
			})

			// 处理响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/plain"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了缓存控制头
			responseHeaders := host.GetResponseHeaders()
			require.True(t, test.HasHeader(responseHeaders, "expires"))
			require.True(t, test.HasHeaderWithValue(responseHeaders, "cache-control", "max-age=7200"))

			host.CompleteHttp()
		})

		// 测试单后缀匹配
		t.Run("single suffix match", func(t *testing.T) {
			host, status := test.NewTestHost(singleSuffixConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/documents/report.pdf"},
				{":method", "GET"},
			})

			// 处理响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/pdf"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了缓存控制头
			responseHeaders := host.GetResponseHeaders()
			require.True(t, test.HasHeader(responseHeaders, "expires"))
			require.True(t, test.HasHeaderWithValue(responseHeaders, "cache-control", "max-age=1800"))

			host.CompleteHttp()
		})

		// 测试空后缀配置
		t.Run("empty suffix config", func(t *testing.T) {
			host, status := test.NewTestHost(emptySuffixConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/any/file.xyz"},
				{":method", "GET"},
			})

			// 处理响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/octet-stream"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了缓存控制头
			responseHeaders := host.GetResponseHeaders()
			require.True(t, test.HasHeader(responseHeaders, "expires"))
			require.True(t, test.HasHeaderWithValue(responseHeaders, "cache-control", "max-age=3600"))

			host.CompleteHttp()
		})
	})
}

func TestCompleteFlow(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("complete cache control flow", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 处理请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/images/logo.png"},
				{":method", "GET"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 2. 处理响应头
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "image/png"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 3. 验证完整的缓存控制流程
			responseHeaders := host.GetResponseHeaders()

			// 验证是否添加了必要的缓存控制响应头
			require.True(t, test.HasHeader(responseHeaders, "expires"))
			require.True(t, test.HasHeaderWithValue(responseHeaders, "cache-control", "max-age=3600"))

			host.CompleteHttp()
		})
	})
}
