package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"strings"
	"testing"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 生成有效的 HMAC 签名 - 与插件中的 generateSigningString 方法保持一致
func generateValidSignature(accessKey, secretKey, algorithm, requestMethod, requestURI string, headers []string, headerValues map[string]string) string {
	// 构造签名字符串，严格按照插件中的 generateSigningString 方法实现
	var signingStringItems []string
	// 第一行是 keyId
	signingStringItems = append(signingStringItems, accessKey)

	if len(headers) > 0 {
		for _, h := range headers {
			if h == "@request-target" {
				// 注意：这里使用原始请求方法和URI
				requestTarget := fmt.Sprintf("%s %s", requestMethod, requestURI)
				signingStringItems = append(signingStringItems, requestTarget)
			} else {
				if value, ok := headerValues[h]; ok {
					signingStringItems = append(signingStringItems, fmt.Sprintf("%s: %s", h, value))
				}
			}
		}
	}

	// 签名字符串需要以换行符结尾
	signingString := strings.Join(signingStringItems, "\n") + "\n"

	// 生成 HMAC 签名
	var mac hash.Hash
	switch algorithm {
	case "hmac-sha1":
		mac = hmac.New(sha1.New, []byte(secretKey))
	case "hmac-sha256":
		mac = hmac.New(sha256.New, []byte(secretKey))
	case "hmac-sha512":
		mac = hmac.New(sha512.New, []byte(secretKey))
	default:
		mac = hmac.New(sha256.New, []byte(secretKey))
	}

	mac.Write([]byte(signingString))
	signature := mac.Sum(nil)
	return base64.StdEncoding.EncodeToString(signature)
}

// 生成 Authorization 头
func generateAuthorizationHeader(accessKey, secretKey, algorithm, requestMethod, requestURI string, headers []string, headerValues map[string]string) string {
	signature := generateValidSignature(accessKey, secretKey, algorithm, requestMethod, requestURI, headers, headerValues)
	header := fmt.Sprintf(`Signature keyId="%s",algorithm="%s",signature="%s"`, accessKey, algorithm, signature)
	if len(headers) > 0 {
		header += fmt.Sprintf(`,headers="%s"`, strings.Join(headers, " "))
	}
	return header
}

// 辅助函数：生成有效的认证头
func generateValidAuthHeaderWithDate(dateStr, method, path string) string {
	headerValues := map[string]string{
		"date": dateStr,
	}
	return generateAuthorizationHeader("ak1", "sk1", "hmac-sha256", method, path, []string{"@request-target", "date"}, headerValues)
}

// 通用测试配置生成函数
func createConfig(consumers []map[string]interface{}, extra map[string]interface{}) json.RawMessage {
	config := map[string]interface{}{
		"consumers": consumers,
		// 设置 clock_skew 为 0 来跳过时钟偏差校验
		"clock_skew": 0,
	}

	for k, v := range extra {
		config[k] = v
	}

	data, _ := json.Marshal(config)
	return data
}

func TestParseGlobalConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		tests := []struct {
			name     string
			config   json.RawMessage
			expected bool
		}{
			{
				"basic global config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
						{
							"name":       "consumer2",
							"access_key": "ak2",
							"secret_key": "sk2",
						},
					},
					map[string]interface{}{},
				),
				true,
			},
			{
				"global auth true config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
					},
					map[string]interface{}{
						"global_auth": true,
					},
				),
				true,
			},
			{
				"global auth false config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
					},
					map[string]interface{}{
						"global_auth": false,
					},
				),
				true,
			},
			{
				"clock skew config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
					},
					map[string]interface{}{
						"clock_skew": 600,
					},
				),
				true,
			},
			{
				"algorithm config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
					},
					map[string]interface{}{
						"allowed_algorithms": []string{"hmac-sha256"},
					},
				),
				true,
			},
			{
				"signed headers config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
					},
					map[string]interface{}{
						"signed_headers": []string{"host", "date"},
					},
				),
				true,
			},
			{
				"anonymous consumer config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
					},
					map[string]interface{}{
						"anonymous_consumer": "anonymous",
					},
				),
				true,
			},
			{
				"invalid config - missing consumers",
				createConfig(
					[]map[string]interface{}{},
					map[string]interface{}{
						"global_auth": false,
					},
				),
				false,
			},
			{
				"invalid config - empty consumers",
				createConfig(
					[]map[string]interface{}{},
					map[string]interface{}{
						"consumers": []map[string]interface{}{},
					},
				),
				false,
			},
			{
				"invalid config - missing access_key",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"secret_key": "sk1",
						},
					},
					map[string]interface{}{},
				),
				false,
			},
			{
				"invalid config - missing secret_key",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
						},
					},
					map[string]interface{}{},
				),
				false,
			},
			{
				"invalid config - duplicate access_key",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
						{
							"name":       "consumer2",
							"access_key": "ak1",
							"secret_key": "sk2",
						},
					},
					map[string]interface{}{},
				),
				false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				host, status := test.NewTestHost(tt.config)
				defer host.Reset()

				if tt.expected {
					require.Equal(t, types.OnPluginStartStatusOK, status)
				} else {
					require.Equal(t, types.OnPluginStartStatusFailed, status)
				}
			})
		}
	})
}

func TestParseOverrideRuleConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		tests := []struct {
			name     string
			config   json.RawMessage
			expected bool
		}{
			{
				"route auth config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
						{
							"name":       "consumer2",
							"access_key": "ak2",
							"secret_key": "sk2",
						},
					},
					map[string]interface{}{
						"allow": []string{"consumer1"},
					},
				),
				true,
			},
			{
				"domain auth config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
						{
							"name":       "consumer2",
							"access_key": "ak2",
							"secret_key": "sk2",
						},
					},
					map[string]interface{}{
						"allow": []string{"consumer2"},
					},
				),
				true,
			},
			{
				"invalid config - empty allow list",
				createConfig(
					[]map[string]interface{}{},
					map[string]interface{}{
						"allow": []string{},
					},
				),
				false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				host, status := test.NewTestHost(tt.config)
				defer host.Reset()

				if tt.expected {
					require.Equal(t, types.OnPluginStartStatusOK, status)
				} else {
					require.Equal(t, types.OnPluginStartStatusFailed, status)
				}
			})
		}
	})
}

func TestParseRuleConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		tests := []struct {
			name     string
			config   json.RawMessage
			expected bool
		}{
			{
				"route level config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
						{
							"name":       "consumer2",
							"access_key": "ak2",
							"secret_key": "sk2",
						},
					},
					map[string]interface{}{
						"_rules_": []map[string]interface{}{
							{
								"_match_route_": []string{"route-a", "route-b"},
								"allow":         []string{"consumer1"},
							},
							{
								"_match_route_": []string{"route-c"},
								"allow":         []string{"consumer2"},
							},
						},
					},
				),
				true,
			},
			{
				"domain level config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
						{
							"name":       "consumer2",
							"access_key": "ak2",
							"secret_key": "sk2",
						},
					},
					map[string]interface{}{
						"_rules_": []map[string]interface{}{
							{
								"_match_domain_": []string{"*.example.com", "test.com"},
								"allow":          []string{"consumer2"},
							},
							{
								"_match_domain_": []string{"api.example.com"},
								"allow":          []string{"consumer1"},
							},
						},
					},
				),
				true,
			},
			{
				"service level config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
						{
							"name":       "consumer2",
							"access_key": "ak2",
							"secret_key": "sk2",
						},
					},
					map[string]interface{}{
						"_rules_": []map[string]interface{}{
							{
								"_match_service_": []string{"service-a:8080", "service-b"},
								"allow":           []string{"consumer1"},
							},
							{
								"_match_service_": []string{"service-b:9090"},
								"allow":           []string{"consumer2"},
							},
						},
					},
				),
				true,
			},
			{
				"route prefix level config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
						{
							"name":       "consumer2",
							"access_key": "ak2",
							"secret_key": "sk2",
						},
					},
					map[string]interface{}{
						"_rules_": []map[string]interface{}{
							{
								"_match_route_prefix_": []string{"api-", "web-"},
								"allow":                []string{"consumer1"},
							},
							{
								"_match_route_prefix_": []string{"admin-", "internal-"},
								"allow":                []string{"consumer2"},
							},
						},
					},
				),
				true,
			},
			{
				"route and service level config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
						{
							"name":       "consumer2",
							"access_key": "ak2",
							"secret_key": "sk2",
						},
					},
					map[string]interface{}{
						"_rules_": []map[string]interface{}{
							{
								"_match_route_":   []string{"route-a"},
								"_match_service_": []string{"service-a:8080"},
								"allow":           []string{"consumer1"},
							},
							{
								"_match_route_":   []string{"route-b"},
								"_match_service_": []string{"service-b:9090"},
								"allow":           []string{"consumer2"},
							},
						},
					},
				),
				true,
			},
			{
				"mixed level config",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
						{
							"name":       "consumer2",
							"access_key": "ak2",
							"secret_key": "sk2",
						},
						{
							"name":       "consumer3",
							"access_key": "ak3",
							"secret_key": "sk3",
						},
					},
					map[string]interface{}{
						"_rules_": []map[string]interface{}{
							{
								"_match_route_": []string{"api-route"},
								"allow":         []string{"consumer1"},
							},
							{
								"_match_domain_": []string{"*.example.com"},
								"allow":          []string{"consumer2"},
							},
							{
								"_match_service_": []string{"internal-service:8080"},
								"allow":           []string{"consumer3"},
							},
							{
								"_match_route_prefix_": []string{"web-"},
								"allow":                []string{"consumer1", "consumer2"},
							},
						},
					},
				),
				true,
			},
			{
				"invalid rule config - missing match conditions",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
					},
					map[string]interface{}{
						"_rules_": []map[string]interface{}{
							{
								"allow": []string{"consumer1"},
								// 缺少匹配条件
							},
						},
					},
				),
				false,
			},
			{
				"invalid rule config - empty match conditions",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
					},
					map[string]interface{}{
						"_rules_": []map[string]interface{}{
							{
								"_match_route_": []string{},
								"allow":         []string{"consumer1"},
							},
						},
					},
				),
				false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				host, status := test.NewTestHost(tt.config)
				defer host.Reset()

				if tt.expected {
					require.Equal(t, types.OnPluginStartStatusOK, status)
				} else {
					require.Equal(t, types.OnPluginStartStatusFailed, status)
				}
			})
		}
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		tests := []struct {
			name           string
			config         json.RawMessage
			headers        [][2]string
			expectContinue bool
			expectError    string
		}{
			{
				"missing authorization header",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
						{
							"name":       "consumer2",
							"access_key": "ak2",
							"secret_key": "sk2",
						},
					},
					map[string]interface{}{},
				),
				[][2]string{{":authority", "example.com"}, {":path", "/api/test"}, {":method", "GET"}},
				true,
				`{"message":"client request can't be validated: missing Authorization header"}`,
			},
			{
				"empty authorization header",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
						{
							"name":       "consumer2",
							"access_key": "ak2",
							"secret_key": "sk2",
						},
					},
					map[string]interface{}{},
				),
				[][2]string{{":authority", "example.com"}, {":path", "/api/test"}, {":method", "GET"}, {"authorization", ""}},
				true,
				`{"message":"client request can't be validated: missing Authorization header"}`,
			},
			{
				"invalid authorization format - missing signature prefix",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
					},
					map[string]interface{}{
						"global_auth": true,
					},
				),
				[][2]string{{":authority", "example.com"}, {":path", "/api/test"}, {":method", "GET"}, {"authorization", "Bearer token123"}},
				true,
				`{"message":"client request can't be validated: Authorization header does not start with 'Signature '"}`,
			},
			{
				"invalid authorization format - invalid signature format",
				createConfig(
					[]map[string]interface{}{
						{
							"name":       "consumer1",
							"access_key": "ak1",
							"secret_key": "sk1",
						},
					},
					map[string]interface{}{
						"global_auth": true,
					},
				),
				[][2]string{{":authority", "example.com"}, {":path", "/api/test"}, {":method", "GET"}, {"authorization", "Signature invalid-format"}},
				true,
				`{"message":"client request can't be validated: keyId or signature missing"}`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				host, status := test.NewTestHost(tt.config)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				action := host.CallOnHttpRequestHeaders(tt.headers)
				require.Equal(t, types.ActionContinue, action)

				localResponse := host.GetLocalResponse()
				require.NotNil(t, localResponse)
				require.Equal(t, uint32(401), localResponse.StatusCode)
				require.Equal(t, tt.expectError, string(localResponse.Data))

				host.CompleteHttp()
			})
		}

		// 测试有效的凭证情况
		t.Run("valid credentials - global auth true, no allow config", func(t *testing.T) {
			host, status := test.NewTestHost(createConfig(
				[]map[string]interface{}{
					{
						"name":       "consumer1",
						"access_key": "ak1",
						"secret_key": "sk1",
					},
				},
				map[string]interface{}{
					"global_auth": true,
				},
			))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			fixedTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
			headerValues := map[string]string{
				"date": fixedTime,
			}

			authHeader := generateAuthorizationHeader("ak1", "sk1", "hmac-sha256", "GET", "/api/test", []string{"@request-target", "date"}, headerValues)
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", authHeader},
				{"date", fixedTime},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Nil(t, host.GetLocalResponse(), "Valid credentials should be accepted")

			host.CompleteHttp()
		})

		// 测试无效的凭证（未配置的 access_key）
		t.Run("invalid credential - not configured access_key", func(t *testing.T) {
			host, status := test.NewTestHost(createConfig(
				[]map[string]interface{}{
					{
						"name":       "consumer1",
						"access_key": "ak1",
						"secret_key": "sk1",
					},
				},
				map[string]interface{}{
					"global_auth": true,
				},
			))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			fixedTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
			headerValues := map[string]string{
				"date": fixedTime,
			}

			authHeader := generateAuthorizationHeader("unknown_ak", "sk", "hmac-sha256", "GET", "/api/test", []string{"@request-target", "date"}, headerValues)
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", authHeader},
				{"date", fixedTime},
			})

			require.Equal(t, types.ActionContinue, action)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse, "Invalid credentials should be rejected")
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Equal(t, `{"message":"client request can't be validated: Invalid keyId"}`, string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试无效的签名（错误的签名）
		t.Run("invalid signature - wrong signature", func(t *testing.T) {
			host, status := test.NewTestHost(createConfig(
				[]map[string]interface{}{
					{
						"name":       "consumer1",
						"access_key": "ak1",
						"secret_key": "sk1",
					},
				},
				map[string]interface{}{
					"global_auth": true,
				},
			))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			fixedTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
			headerValues := map[string]string{
				"date": fixedTime,
			}

			authHeader := generateAuthorizationHeader("ak1", "sk1", "hmac-sha256", "GET", "/api/test", []string{"@request-target", "date"}, headerValues)
			// 故意修改签名
			authHeader = strings.Replace(authHeader, "signature=", "signature=wrong", 1)
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", authHeader},
				{"date", fixedTime},
			})

			require.Equal(t, types.ActionContinue, action)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse, "Invalid signature should be rejected")
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Equal(t, `{"message":"client request can't be validated: keyId or signature missing"}`, string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试有效的凭证（全局认证关闭，有 allow 配置）
		t.Run("valid credentials - global auth false, with allow config", func(t *testing.T) {
			host, status := test.NewTestHost(createConfig(
				[]map[string]interface{}{
					{
						"name":       "consumer1",
						"access_key": "ak1",
						"secret_key": "sk1",
					},
					{
						"name":       "consumer2",
						"access_key": "ak2",
						"secret_key": "sk2",
					},
				},
				map[string]interface{}{
					"allow": []string{"consumer1"},
				},
			))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			fixedTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
			headerValues := map[string]string{
				"date": fixedTime,
			}

			authHeader := generateAuthorizationHeader("ak1", "sk1", "hmac-sha256", "GET", "/api/test", []string{"@request-target", "date"}, headerValues)
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", authHeader},
				{"date", fixedTime},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Nil(t, host.GetLocalResponse(), "Valid credentials should be accepted")

			host.CompleteHttp()
		})

		// 测试有效的凭证但不在 allow 列表中的情况
		t.Run("valid credentials but not in allow list", func(t *testing.T) {
			host, status := test.NewTestHost(createConfig(
				[]map[string]interface{}{
					{
						"name":       "consumer1",
						"access_key": "ak1",
						"secret_key": "sk1",
					},
					{
						"name":       "consumer2",
						"access_key": "ak2",
						"secret_key": "sk2",
					},
				},
				map[string]interface{}{
					"allow": []string{"consumer1"},
				},
			))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			fixedTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
			headerValues := map[string]string{
				"date": fixedTime,
			}

			authHeader := generateAuthorizationHeader("ak2", "sk2", "hmac-sha256", "GET", "/api/test", []string{"@request-target", "date"}, headerValues)
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", authHeader},
				{"date", fixedTime},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Nil(t, host.GetLocalResponse(), "Valid credentials should be accepted when not in allow list")

			host.CompleteHttp()
		})

		// 测试匿名消费者配置
		t.Run("anonymous consumer config", func(t *testing.T) {
			host, status := test.NewTestHost(createConfig(
				[]map[string]interface{}{
					{
						"name":       "consumer1",
						"access_key": "ak1",
						"secret_key": "sk1",
					},
				},
				map[string]interface{}{
					"anonymous_consumer": "anonymous",
				},
			))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Nil(t, host.GetLocalResponse(), "Request without credentials should be accepted with anonymous consumer")

			host.CompleteHttp()
		})
	})
}

func TestSignedHeadersDefault(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		consumers := []map[string]interface{}{
			{
				"name":       "consumer1",
				"access_key": "ak1",
				"secret_key": "sk1",
			},
		}
		tests := []struct {
			name                   string
			configSignedHeaders    []string
			configureSignedHeaders bool
			requestSignedHeaders   []string
			clockSkew              int
			dateOffset             time.Duration
			signatureDateOffset    time.Duration
			expectedError          string
		}{
			{
				name:          "omitted list rejects keyId only with clock skew disabled",
				clockSkew:     0,
				expectedError: "headers missing",
			},
			{
				name:                 "omitted list accepts signed date",
				requestSignedHeaders: []string{"date"},
				clockSkew:            300,
			},
			{
				name:                 "omitted list rejects tampered date",
				requestSignedHeaders: []string{"date"},
				clockSkew:            300,
				signatureDateOffset:  time.Minute,
				expectedError:        "Invalid signature",
			},
			{
				name:                 "omitted list rejects expired date",
				requestSignedHeaders: []string{"date"},
				clockSkew:            300,
				dateOffset:           -time.Hour,
				expectedError:        "Clock skew exceeded",
			},
			{
				name:                   "explicit empty list accepts keyId only",
				configSignedHeaders:    []string{},
				configureSignedHeaders: true,
				clockSkew:              0,
			},
			{
				name:                   "explicit non-empty list remains unchanged",
				configSignedHeaders:    []string{"@request-target"},
				configureSignedHeaders: true,
				requestSignedHeaders:   []string{"@request-target"},
				clockSkew:              300,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				extra := map[string]interface{}{"clock_skew": tt.clockSkew}
				if tt.configureSignedHeaders {
					extra["signed_headers"] = tt.configSignedHeaders
				}
				host, status := test.NewTestHost(createConfig(consumers, extra))
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				requestDate := time.Now().UTC().Add(tt.dateOffset)
				signatureDate := requestDate
				if tt.signatureDateOffset != 0 {
					signatureDate = time.Now().UTC().Add(tt.signatureDateOffset)
				}
				requestDateString := requestDate.Format("Mon, 02 Jan 2006 15:04:05 GMT")
				signatureDateString := signatureDate.Format("Mon, 02 Jan 2006 15:04:05 GMT")
				authHeader := generateAuthorizationHeader(
					"ak1", "sk1", "hmac-sha256", "GET", "/api/test",
					tt.requestSignedHeaders, map[string]string{"date": signatureDateString},
				)
				action := host.CallOnHttpRequestHeaders([][2]string{
					{":authority", "example.com"},
					{":path", "/api/test"},
					{":method", "GET"},
					{"authorization", authHeader},
					{"date", requestDateString},
				})
				require.Equal(t, types.ActionContinue, action)

				localResponse := host.GetLocalResponse()
				if tt.expectedError == "" {
					require.Nil(t, localResponse)
				} else {
					require.NotNil(t, localResponse)
					require.Equal(t, uint32(401), localResponse.StatusCode)
					require.Contains(t, string(localResponse.Data), tt.expectedError)
				}
				host.CompleteHttp()
			})
		}
	})
}

func TestCompleteFlow(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("complete hmac auth flow", func(t *testing.T) {
			host, status := test.NewTestHost(createConfig(
				[]map[string]interface{}{
					{
						"name":       "consumer1",
						"access_key": "ak1",
						"secret_key": "sk1",
					},
				},
				map[string]interface{}{
					"global_auth": true,
				},
			))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 测试缺少认证信息的情况
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
			})

			require.Equal(t, types.ActionContinue, action)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse, "Request without credentials should be rejected")
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Equal(t, `{"message":"client request can't be validated: missing Authorization header"}`, string(localResponse.Data))

			host.CompleteHttp()

			// 2. 测试有效认证的情况
			fixedTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
			headerValues := map[string]string{
				"date": fixedTime,
			}

			authHeader := generateAuthorizationHeader("ak1", "sk1", "hmac-sha256", "GET", "/api/test", []string{"@request-target", "date"}, headerValues)
			action = host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", authHeader},
				{"date", fixedTime},
			})

			require.Equal(t, types.ActionContinue, action)

			requestHeaders := host.GetRequestHeaders()
			require.True(t, test.HasHeaderWithValue(requestHeaders, "X-Mse-Consumer", "consumer1"))

			require.Nil(t, host.GetLocalResponse(), "Valid credentials should be accepted")

			host.CompleteHttp()
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试有效的请求体和摘要
		t.Run("valid request body with correct digest", func(t *testing.T) {
			host, status := test.NewTestHost(createConfig(
				[]map[string]interface{}{
					{
						"name":       "consumer1",
						"access_key": "ak1",
						"secret_key": "sk1",
					},
				},
				map[string]interface{}{
					"validate_request_body": true,
				},
			))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 准备空请求体的摘要
			validBody := []byte(`{"name": "test", "value": 123}`)
			correctDigest := calculateBodyDigest(validBody)

			// 使用固定的时间值确保一致性
			fixedTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")

			// 生成与请求头一致的签名
			authHeader := generateValidAuthHeaderWithDate(fixedTime, "POST", "/api/test")

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"authorization", authHeader},
				{"date", fixedTime},
				{"digest", correctDigest}, // 使用正确的摘要
				{"content-type", "application/json"},
			})

			// 测试有效的请求体
			action := host.CallOnHttpRequestBody(validBody)
			require.Equal(t, types.ActionContinue, action, "Valid body with correct digest should be accepted")
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction(), "Stream action should be continue")

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "Valid body with correct digest should not be rejected")

			host.CompleteHttp()
		})

		// 测试无效的摘要
		t.Run("invalid digest", func(t *testing.T) {
			host, status := test.NewTestHost(createConfig(
				[]map[string]interface{}{
					{
						"name":       "consumer1",
						"access_key": "ak1",
						"secret_key": "sk1",
					},
				},
				map[string]interface{}{
					"validate_request_body": true,
				},
			))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 使用固定的时间值确保一致性
			fixedTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")

			// 生成与请求头一致的签名
			authHeader := generateValidAuthHeaderWithDate(fixedTime, "POST", "/api/test")

			// 先调用头部处理
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"authorization", authHeader},
				{"date", fixedTime},
				{"digest", "SHA-256=invalid-digest"},
				{"content-type", "application/json"},
			})

			// 测试请求体但摘要不匹配
			body := []byte(`{"name": "test", "value": 123}`)
			action := host.CallOnHttpRequestBody(body)

			require.Equal(t, types.ActionContinue, action, "Request with invalid digest should be rejected")
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction(), "Stream action should be continue")

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse, "Request with invalid digest should be rejected")
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Contains(t, string(localResponse.Data), "Invalid digest", "Error message should indicate invalid digest")

			host.CompleteHttp()
		})

		// 测试缺少摘要头
		t.Run("missing digest header", func(t *testing.T) {
			host, status := test.NewTestHost(createConfig(
				[]map[string]interface{}{
					{
						"name":       "consumer1",
						"access_key": "ak1",
						"secret_key": "sk1",
					},
				},
				map[string]interface{}{
					"validate_request_body": true,
				},
			))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 使用固定的时间值确保一致性
			fixedTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")

			// 生成与请求头一致的签名
			authHeader := generateValidAuthHeaderWithDate(fixedTime, "POST", "/api/test")

			// 先调用头部处理，但不设置digest头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"authorization", authHeader},
				{"date", fixedTime},
				// 故意不设置digest头
				{"content-type", "application/json"},
			})

			// 测试请求体但缺少摘要头
			body := []byte(`{"name": "test", "value": 123}`)
			action := host.CallOnHttpRequestBody(body)

			require.Equal(t, types.ActionContinue, action, "Request without digest header should be rejected")
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction(), "Stream action should be continue")

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse, "Request without digest header should be rejected")
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Contains(t, string(localResponse.Data), "Invalid digest", "Error message should indicate invalid digest")

			host.CompleteHttp()
		})

		// 测试未启用请求体验证
		t.Run("body validation disabled", func(t *testing.T) {
			host, status := test.NewTestHost(createConfig(
				[]map[string]interface{}{
					{
						"name":       "consumer1",
						"access_key": "ak1",
						"secret_key": "sk1",
					},
				},
				map[string]interface{}{
					"validate_request_body": false,
				},
			))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 使用固定的时间值确保一致性
			fixedTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")

			// 生成与请求头一致的签名
			authHeader := generateValidAuthHeaderWithDate(fixedTime, "POST", "/api/test")

			// 先调用头部处理
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"authorization", authHeader},
				{"date", fixedTime},
				{"content-type", "application/json"},
			})
			// 由于禁用了请求体验证，应该直接继续而不等待请求体
			require.Equal(t, types.ActionContinue, action, "Should continue immediately when validate_request_body is false")

			host.CompleteHttp()
		})

		// 测试空请求体
		t.Run("empty request body", func(t *testing.T) {
			host, status := test.NewTestHost(createConfig(
				[]map[string]interface{}{
					{
						"name":       "consumer1",
						"access_key": "ak1",
						"secret_key": "sk1",
					},
				},
				map[string]interface{}{
					"validate_request_body": true,
				},
			))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 使用固定的时间值确保一致性
			fixedTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")

			// 准备空请求体的摘要
			emptyBody := []byte("")
			correctDigest := calculateBodyDigest(emptyBody)

			// 生成与请求头一致的签名
			authHeader := generateValidAuthHeaderWithDate(fixedTime, "POST", "/api/test")

			// 先调用头部处理
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"authorization", authHeader},
				{"date", fixedTime},
				{"digest", correctDigest}, // 空字符串的SHA-256摘要
				{"content-type", "application/json"},
			})

			// 测试空请求体
			action := host.CallOnHttpRequestBody(emptyBody)

			require.Equal(t, types.ActionContinue, action, "Empty body with correct digest should be accepted")
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction(), "Stream action should be continue")

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "Empty body with correct digest should not be rejected")

			host.CompleteHttp()
		})
	})
}
