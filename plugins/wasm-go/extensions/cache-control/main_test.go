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
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

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

// expiresValidationCase 是 expires 输入契约的一个用例。
// config 是原始 JSON 文本，保证数字/字符串/非法 JSON 值的写法精确可控。
type expiresValidationCase struct {
	name   string
	config string
	valid  bool
}

// expiresValidationCases 覆盖 expires 的全部输入类别：
// 字符串特殊值、JSON 整数字面量、数字字符串（含前导零）、上下边界，
// 以及必须在配置阶段拒绝的缺失/null/空串/布尔/数组/对象/非数字/负数/
// 加号/空白包围/浮点/指数/越界输入。
var expiresValidationCases = []expiresValidationCase{
	// 字符串特殊值：保持原有两种头部行为
	{name: "string max", config: `{"suffix":"jpg|png|jpeg","expires":"max"}`, valid: true},
	{name: "string epoch", config: `{"suffix":"html|htm","expires":"epoch"}`, valid: true},
	// JSON 整数字面量或数字字符串
	{name: "json integer literal", config: `{"suffix":"jpg","expires":3600}`, valid: true},
	{name: "numeric string", config: `{"expires":"3600"}`, valid: true},
	{name: "numeric string with leading zeros", config: `{"expires":"0003600"}`, valid: true},
	// 两个边界值
	{name: "lower boundary zero", config: `{"expires":0}`, valid: true},
	{name: "upper boundary", config: `{"expires":9223372036}`, valid: true},
	// 缺失、null、空串、布尔、数组、对象：配置失败
	{name: "missing expires", config: `{"suffix":"jpg"}`, valid: false},
	{name: "null expires", config: `{"expires":null}`, valid: false},
	{name: "empty string expires", config: `{"expires":""}`, valid: false},
	{name: "boolean true expires", config: `{"expires":true}`, valid: false},
	{name: "boolean false expires", config: `{"expires":false}`, valid: false},
	{name: "array expires", config: `{"expires":[]}`, valid: false},
	{name: "object expires", config: `{"expires":{}}`, valid: false},
	// 非数字、负数、加号、空白包围、浮点、指数：配置失败，不做截断或隐式修正
	{name: "alphabetic string", config: `{"expires":"abc"}`, valid: false},
	{name: "negative json number", config: `{"expires":-5}`, valid: false},
	{name: "negative string", config: `{"expires":"-5"}`, valid: false},
	{name: "explicit plus sign", config: `{"expires":"+1"}`, valid: false},
	{name: "whitespace surrounded string", config: `{"expires":" 3600 "}`, valid: false},
	{name: "float json number", config: `{"expires":36.5}`, valid: false},
	{name: "float string", config: `{"expires":"3.14"}`, valid: false},
	{name: "exponent json number", config: `{"expires":3.6e3}`, valid: false},
	{name: "exponent string", config: `{"expires":"1e3"}`, valid: false},
	// 越界：上限加一与 int64/uint64 溢出字符串
	{name: "upper boundary overflow by one", config: `{"expires":9223372037}`, valid: false},
	{name: "int64 max string", config: `{"expires":"9223372036854775807"}`, valid: false},
	{name: "uint64 overflow string", config: `{"expires":"99999999999999999999"}`, valid: false},
}

// TestParseConfigExpiresValidation 通过真实插件启动流程断言 expires 输入契约：
// 合法配置启动成功，非法配置在插件启动阶段被拒绝。
func TestParseConfigExpiresValidation(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		for _, tc := range expiresValidationCases {
			t.Run(tc.name, func(t *testing.T) {
				host, status := test.NewTestHost(json.RawMessage(tc.config))
				defer host.Reset()
				if tc.valid {
					require.Equal(t, types.OnPluginStartStatusOK, status)
				} else {
					require.Equal(t, types.OnPluginStartStatusFailed, status)
				}
			})
		}
	})
}

// expiresResponseCase 是合法 expires 配置下响应头行为的一个用例。
type expiresResponseCase struct {
	name             string
	config           string
	path             string
	wantCacheControl string
	// wantExpires 非空时期望固定的 Expires 头；为空时按 wantSeconds 校验时间方向。
	wantExpires string
	wantSeconds int64
	// expectNoHeaders 表示响应不应携带任何缓存控制头（后缀不匹配）。
	expectNoHeaders bool
}

// expiresResponseCases 覆盖 0、两种 3600 写法、前导零规范化、上限、
// max、epoch 与后缀不匹配。
var expiresResponseCases = []expiresResponseCase{
	{name: "zero expires", config: `{"suffix":"jpg","expires":0}`,
		path: "/images/photo.jpg", wantCacheControl: "max-age=0", wantSeconds: 0},
	{name: "json number 3600", config: `{"suffix":"jpg","expires":3600}`,
		path: "/images/photo.jpg", wantCacheControl: "max-age=3600", wantSeconds: 3600},
	{name: "numeric string 3600", config: `{"suffix":"jpg","expires":"3600"}`,
		path: "/images/photo.jpg", wantCacheControl: "max-age=3600", wantSeconds: 3600},
	{name: "leading zeros normalized", config: `{"suffix":"jpg","expires":"0003600"}`,
		path: "/images/photo.jpg", wantCacheControl: "max-age=3600", wantSeconds: 3600},
	{name: "upper boundary without duration overflow", config: `{"suffix":"jpg","expires":9223372036}`,
		path: "/images/photo.jpg", wantCacheControl: "max-age=9223372036", wantSeconds: 9223372036},
	{name: "string max", config: `{"suffix":"css","expires":"max"}`,
		path: "/static/main.css", wantCacheControl: "max-age=315360000",
		wantExpires: "Thu, 31 Dec 2037 23:55:55 GMT"},
	{name: "string epoch", config: `{"suffix":"html","expires":"epoch"}`,
		path: "/page.html", wantCacheControl: "no-cache",
		wantExpires: "Thu, 01 Jan 1970 00:00:01 GMT"},
	{name: "suffix mismatch adds no headers", config: `{"suffix":"jpg","expires":"3600"}`,
		path: "/api/data.json", expectNoHeaders: true},
}

// requireExpiresInSeconds 校验 Expires 头存在、可按 HTTP 时间格式解析，
// 且过期时刻符合配置的秒数。
//
// go 模式下插件使用真实时钟，断言过期时刻距当前约 wantSeconds 秒。
// wasm 模式下 wazero 的 WASI 实现默认挂载 FakeWalltime（自 2022-01-01 UTC
// 起步、每次读取 +1ms 的假时钟），因此使用该纪元作为时钟基准。
// 两种模式都只允许 15 秒误差，不能在断言失败后退化为宽窗口。
func requireExpiresInSeconds(t *testing.T, headers [][2]string, wantSeconds int64) {
	t.Helper()
	value, ok := test.GetHeaderValue(headers, "expires")
	require.True(t, ok, "expires header must exist")
	expireTime, err := http.ParseTime(value)
	require.NoError(t, err)
	clockBase := time.Now()
	// RunTest names its runtime subtests "go" and "wasm". The pinned
	// proxytest/wazero version uses the default fake clock in wasm mode.
	if strings.Contains(t.Name(), "/wasm/") {
		clockBase = time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	require.InDelta(t, float64(wantSeconds), expireTime.Sub(clockBase).Seconds(), 15,
		"expires must reflect the configured %d seconds", wantSeconds)
}

// TestOnHttpResponseHeadersExpiresContract 覆盖合法 expires 配置的响应头行为。
func TestOnHttpResponseHeadersExpiresContract(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		for _, tc := range expiresResponseCases {
			t.Run(tc.name, func(t *testing.T) {
				host, status := test.NewTestHost(json.RawMessage(tc.config))
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				host.CallOnHttpRequestHeaders([][2]string{
					{":authority", "example.com"},
					{":path", tc.path},
					{":method", "GET"},
				})
				action := host.CallOnHttpResponseHeaders([][2]string{
					{":status", "200"},
					{"content-type", "text/plain"},
				})
				require.Equal(t, types.ActionContinue, action)

				headers := host.GetResponseHeaders()
				if tc.expectNoHeaders {
					require.False(t, test.HasHeader(headers, "expires"))
					require.False(t, test.HasHeader(headers, "cache-control"))
					return
				}
				require.True(t, test.HasHeaderWithValue(headers, "cache-control", tc.wantCacheControl))
				if tc.wantExpires != "" {
					require.True(t, test.HasHeaderWithValue(headers, "expires", tc.wantExpires))
					return
				}
				requireExpiresInSeconds(t, headers, tc.wantSeconds)
			})
		}
	})
}

// TestParseConfigExpiresStruct 校验解析后的配置结构（依赖 Go 反射，仅 go 模式）。
// 用反射按字段名取值，保证测试在引入 maxAgeSeconds 字段之前也可编译运行。
func TestParseConfigExpiresStruct(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("numeric config stores validated seconds", func(t *testing.T) {
			host, status := test.NewTestHost([]byte(`{"suffix":"jpg|png","expires":3600}`))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			cfgValue := reflect.ValueOf(config).Elem()
			seconds := cfgValue.FieldByName("maxAgeSeconds")
			require.True(t, seconds.IsValid(), "config must carry validated maxAgeSeconds")
			require.Equal(t, int64(3600), seconds.Int())
			require.Empty(t, cfgValue.FieldByName("expires").String())
			require.Equal(t, "[jpg png]", fmt.Sprint(cfgValue.FieldByName("suffix")))
		})

		t.Run("leading zeros normalize to seconds", func(t *testing.T) {
			host, status := test.NewTestHost([]byte(`{"expires":"0003600"}`))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			seconds := reflect.ValueOf(config).Elem().FieldByName("maxAgeSeconds")
			require.True(t, seconds.IsValid(), "config must carry validated maxAgeSeconds")
			require.Equal(t, int64(3600), seconds.Int())
		})

		t.Run("special values keep expires marker", func(t *testing.T) {
			host, status := test.NewTestHost([]byte(`{"suffix":"css","expires":"max"}`))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.Equal(t, "max", reflect.ValueOf(config).Elem().FieldByName("expires").String())
		})
	})
}
