---
title: APISIX HMAC 认证
keywords: [higress,hmac auth,apisix]
description: APISIX HMAC 认证插件配置参考
---

## 功能说明

`hmac-auth-apisix` 插件兼容 Apache APISIX 的 HMAC 认证机制，通过 HMAC 算法为 HTTP 请求生成防篡改的数字签名，实现请求的身份认证和权限控制。该插件完全兼容 Apache APISIX HMAC 认证插件的配置和签名算法，签名生成方法可参考 [Apache APISIX HMAC 认证文档](https://apisix.apache.org/docs/apisix/plugins/hmac-auth/)

## 运行属性

插件执行阶段：`认证阶段`
插件执行优先级：`330`

## 配置字段

**注意：**

- 在一个规则里，鉴权配置和认证配置不可同时存在
- 对于通过认证鉴权的请求，请求的 header 会被添加一个 `X-Mse-Consumer` 字段，用以标识调用者的名称
  该字段由网关**覆盖写入**而非追加：客户端请求中自带的 `X-Mse-Consumer` 会先被移除，再写入本次认证得到的 Consumer 名称。
  因此下游读取到的始终是网关的认证结果，调用方无法通过自带该请求头伪造身份。

### 认证配置

| 名称                    | 数据类型        | 填写要求                   | 默认值                                      | 描述                                                         |
| ----------------------- | --------------- | -------------------------- | ------------------------------------------- | ------------------------------------------------------------ |
| `global_auth`           | bool            | 选填（**仅实例级别配置**） | -                                           | 只能在实例级别配置，若配置为 true，则全局生效认证机制；若配置为 false，则只对做了配置的域名和路由生效认证机制，若不配置则仅当没有域名和路由配置时全局生效（兼容老用户使用习惯） |
| `consumers`             | array of object | 必填                       | -                                           | 配置服务的调用者，用于对请求进行认证                         |
| `allowed_algorithms`    | array of string | 选填                       | ["hmac-sha1", "hmac-sha256", "hmac-sha512"] | 允许的 HMAC 算法列表。有效值为 "hmac-sha1"、"hmac-sha256" 和 "hmac-sha512" 的组合 |
| `clock_skew`            | number          | 选填                       | 300                                         | 客户端请求的时间戳与 Higress 服务器当前时间之间允许的最大时间差（以秒为单位）。这有助于解决客户端和服务器之间的时间同步差异，并防止重放攻击。时间戳将根据 Date 头中的时间（必须为 GMT 格式）进行计算。如果配置为0，会跳过该校验 |
| `signed_headers`        | array of string | 选填                       | -                                           | 客户端请求的 HMAC 签名中应包含的 HMAC 签名头列表             |
| `validate_request_body` | boolean         | 选填                       | false                                       | 如果为 true，则验证请求正文的完整性，以确保在传输过程中没有被篡改。具体来说，插件会创建一个 SHA-256 的 base64 编码 digest，并将其与 `Digest` 头进行比较。如果 `Digest` 头丢失或 digest 不匹配，验证将失败 |
| `hide_credentials`      | boolean         | 选填                       | false                                       | 如果为 true，则不会将授权请求头传递给上游服务                |
| `anonymous_consumer`    | string          | 选填                       | -                                           | 匿名消费者名称。如果已配置，则允许匿名用户绕过身份验证       |


`consumers`中每一项的配置字段说明如下：

| 名称         | 数据类型 | 填写要求 | 默认值       | 描述                                           |
| ------------ | -------- | -------- | ------------ | ---------------------------------------------- |
| `access_key` | string   | 必填     | -            | 消费者的唯一标识符，用于标识相关配置，例如密钥 |
| `secret_key` | string   | 必填     | -            | 用于生成 HMAC 的密钥                           |
| `name`       | string   | 选填     | `access_key` | 配置该 consumer 的名称                         |

### 鉴权配置（非必需）

| 名称    | 数据类型        | 填写要求                 | 默认值 | 描述                                                         |
| ------- | --------------- | ------------------------ | ------ | ------------------------------------------------------------ |
| `allow` | array of string | 选填(**非实例级别配置**) | -      | 只能在路由或域名等细粒度规则上配置，对于符合匹配条件的请求，配置允许访问的 consumer，从而实现细粒度的权限控制。**在规则上启用了本插件却未配置 `allow`（或配置为空列表）时，视为没有任何 consumer 被授权，匹配该规则的请求会被直接拒绝（fail closed）** |

**关于 `allow` 的 fail closed 语义：**

- 细粒度规则（`_rules_`）一旦命中当前请求，就表示插件已在该域名或路由上生效。此时 `allow` 缺失或为空列表**不再表示“插件未生效”**，而是表示没有任何 consumer 被授权，所有匹配该规则的请求都会返回 `401`，错误信息为 `{"message":"client request can't be validated: no consumer is allowed"}`，不会跳过认证直接放行。
- 配置了 `anonymous_consumer` 时同样如此：匿名身份也不能绕过上述拒绝。
- `global_auth: true` 时认证全局生效，`allow` 只是附加的细粒度限制，因此规则上未配置 `allow` 表示不做额外限制，请求仍需通过签名校验后才能访问。
- `global_auth: false` 时，只有命中规则的域名或路由才会认证；未命中任何规则的请求直接放行。

## 配置示例

### 全局配置认证和路由粒度鉴权

以下配置用于对网关特定路由或域名开启 Hmac Auth 认证和鉴权。**注意：access_key 字段不可重复**

#### 示例1：基础路由与域名鉴权配置

**实例级别插件配置**：
```yaml
global_auth: false
consumers:
- name: consumer1
  access_key: consumer1-key
  secret_key: 2bda943c-ba2b-11ec-ba07-00163e1250b5
- name: consumer2
  access_key: consumer2-key
  secret_key: c8c8e9ca-558e-4a2d-bb62-e700dcc40e35
```

**路由级配置**（适用于 route-a 和 route-b）：
```yaml
allow: 
- consumer1  # 仅允许consumer1访问
```

**域名级配置**（适用于 `*.example.com` 和 `test.com`）：
```yaml
allow:
- consumer2  # 仅允许consumer2访问
```

**配置说明**：

- 路由名称（如 route-a、route-b）对应网关路由创建时定义的名称，匹配时仅允许consumer1访问
- 域名匹配（如 `*.example.com`、`test.com`）用于过滤请求域名，匹配时仅允许consumer2访问
- 未在allow列表中的调用者将被拒绝访问
- 如果在某个路由或域名上启用了本插件，却没有配置 `allow`（或配置为空列表），则该路由或域名上的**所有**请求都会被拒绝（返回 `401`），即“失败关闭”（fail closed），不会跳过认证直接放行

**生成签名，可以使用以下 Go 代码片段或其他技术栈**：

```go
package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"hash"
	"strings"
	"time"
)

// SignedHeader 定义签名头的结构
type SignedHeader struct {
	Name  string
	Value string
}

func main() {
	// 配置参数
	keyID := "consumer1-key"                            // key id
	secretKey := "2bda943c-ba2b-11ec-ba07-00163e1250b5" // secret key
	requestMethod := "POST"                             // HTTP method
	requestPath := "/foo"                               // Route URI
	algorithm := "hmac-sha256"                          // algorithm
	validateRequestBody := false                        // 是否验证请求体，设置为true时会添加Digest头部

	// 如果配置了 signed_headers，则需要按照顺序添加
	signedHeaders := []SignedHeader{
		//{Name: "x-custom-header-a", Value: "test1"},
		//{Name: "x-custom-header-b", Value: "test2"},
	}

	body := []byte("{}") // request body

	// 获取当前 GMT 时间
	gmtTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")

	// 动态构造签名字符串（有序）
	signingStringBuilder := strings.Builder{}
	signingStringBuilder.WriteString(fmt.Sprintf("%s\n%s %s\ndate: %s\n",
		keyID,
		requestMethod,
		requestPath,
		gmtTime))

	// 按照signedHeaders中的顺序添加header
	for _, header := range signedHeaders {
		signingStringBuilder.WriteString(fmt.Sprintf("%s: %s\n", header.Name, header.Value))
	}

	signingString := signingStringBuilder.String()

	// 创建签名
	signature, err := generateHmacSignature(secretKey, algorithm, signingString)
	if err != nil {
		fmt.Printf("Error generating signature: %v\n", err)
		return
	}

	// 动态构建headers字段内容
	headersField := "@request-target date"
	for _, header := range signedHeaders {
		headersField += " " + header.Name
	}

	// 构造请求头部
	headers := map[string]string{
		"Date": gmtTime,
		"Authorization": fmt.Sprintf(`Signature keyId="%s",algorithm="%s",headers="%s",signature="%s"`,
			keyID,
			algorithm,
			headersField,
			signature,
		),
	}

	// 如果需要验证请求体，则添加Digest头部
	if validateRequestBody {
		headers["Digest"] = calculateBodyDigest(body)
	}

	// 添加签名的请求头
	for _, header := range signedHeaders {
		formattedHeaderName := formatHeaderName(header.Name)
		headers[formattedHeaderName] = header.Value
	}

	// 打印签名字符串
	fmt.Printf("signingString: %s\n", signingString)
	// 打印请求头
	fmt.Println("Headers:")
	for key, value := range headers {
		fmt.Printf("%s: %s\n", key, value)
	}
}

// generateHmacSignature 生成HMAC签名
func generateHmacSignature(secretKey, algorithm, message string) (string, error) {
	var mac hash.Hash

	switch algorithm {
	case "hmac-sha1":
		mac = hmac.New(sha1.New, []byte(secretKey))
	case "hmac-sha256":
		mac = hmac.New(sha256.New, []byte(secretKey))
	case "hmac-sha512":
		mac = hmac.New(sha512.New, []byte(secretKey))
	default:
		return "", fmt.Errorf("unsupported algorithm: %s", algorithm)
	}

	mac.Write([]byte(message))
	signature := mac.Sum(nil)
	return base64.StdEncoding.EncodeToString(signature), nil
}

// calculateBodyDigest 计算body的摘要
func calculateBodyDigest(body []byte) string {
	hash := sha256.Sum256(body)
	encodedDigest := base64.StdEncoding.EncodeToString(hash[:])
	return "SHA-256=" + encodedDigest
}

// formatHeaderName 将header name转换为标准HTTP头格式
func formatHeaderName(headerName string) string {
	parts := strings.Split(headerName, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	return strings.Join(parts, "-")
}
```

**请求与响应示例**：

1. **验证通过场景**
```shell
curl -X POST 'http://localhost:8082/foo' \
-H 'Authorization:Signature keyId="consumer1-key",algorithm="hmac-sha256",headers="@request-target date",signature="746z4VISwZehUwZdzTV486ZMMbBtakmMHKPfs/A4RdU="' \
-H 'Date:Fri, 12 Sep 2025 23:53:18 GMT' \
-H 'Content-Type: application/json' \
-d '{}'
```
- 响应：返回后端服务正常响应
- 附加信息：认证通过后会自动添加请求头 `X-Mse-Consumer: consumer1` 传递给后端

2. **请求方法修改导致验签失败**
```shell
curl -X PUT 'http://localhost:8082/foo' \  # 此处将POST改为PUT
-H 'Authorization:Signature keyId="consumer1-key",algorithm="hmac-sha256",headers="@request-target date",signature="746z4VISwZehUwZdzTV486ZMMbBtakmMHKPfs/A4RdU="' \
-H 'Date:Fri, 12 Sep 2025 23:53:18 GMT' \
-H 'Content-Type: application/json' \
-d '{}'
```
- 响应：`401 Unauthorized`
- 错误信息：`{"message":"client request can't be validated: Invalid signature"}`

3. **不在允许列表中的调用者**
```shell
curl -X POST 'http://localhost:8082/foo' \
-H 'Authorization:Signature keyId="consumer2-key",algorithm="hmac-sha256",headers="@request-target date",signature="dltotPwd4iWGGz//kuehPJlHXZemR5WKwCPAJD/KPhE="' \
-H 'Date:Fri, 12 Sep 2025 23:59:01 GMT' \
-H 'Content-Type: application/json' \
-d '{}'
```
- 响应：`401 Unauthorized`
- 错误信息：`{"message":"client request can't be validated: consumer 'consumer2' is not allowed"}`

4. **时间戳过期**
```shell
curl -X POST 'http://localhost:8082/foo' \
-H 'Authorization:Signature keyId="consumer1-key",algorithm="hmac-sha256",headers="@request-target date",signature="746z4VISwZehUwZdzTV486ZMMbBtakmMHKPfs/A4RdU="' \
-H 'Date:Fri, 12 Sep 2025 23:53:18 GMT' \  # 过期的时间戳
-H 'Content-Type: application/json' \
-d '{}'
```
- 响应：`401 Unauthorized`
- 错误信息：`{"message":"client request can't be validated: Clock skew exceeded"}`

#### 示例2：带自定义签名头与请求体验证的配置

**实例级别插件配置**：
```yaml
global_auth: false
consumers:
- name: consumer1
  access_key: consumer1-key
  secret_key: 2bda943c-ba2b-11ec-ba07-00163e1250b5
- name: consumer2
  access_key: consumer2-key
  secret_key: c8c8e9ca-558e-4a2d-bb62-e700dcc40e35
signed_headers:  # 需要纳入签名的自定义请求头
- X-Custom-Header-A
- X-Custom-Header-B
validate_request_body: true  # 启用请求体签名校验
```

**请求与响应示例**：

1. **验证通过场景**
```shell
curl -X POST 'http://localhost:8082/foo' \
-H 'Authorization:Signature keyId="consumer1-key",algorithm="hmac-sha256",headers="@request-target date x-custom-header-a x-custom-header-b",signature="KoOlbkDIR/JzlKK47eURewnIpmhpkQU+KIyBUhqVfmo="' \
-H 'Date:Sat, 13 Sep 2025 00:04:34 GMT' \
-H 'Digest:SHA-256=RBNvo1WzZ4oRRq0W9+hknpT7T8If536DEMBg9hyq/4o=' \  # 请求体摘要
-H 'X-Custom-Header-A:test1' \
-H 'X-Custom-Header-B:test2' \
-H 'Content-Type: application/json' \
-d '{}'
```

- 响应：返回后端服务正常响应

2. **缺少签名头**
```shell
curl -X POST 'http://localhost:8082/foo' \
-H 'Authorization:Signature keyId="consumer1-key",algorithm="hmac-sha256",headers="@request-target date x-custom-header-b",signature="KoOlbkDIR/JzlKK47eURewnIpmhpkQU+KIyBUhqVfmo="' \
-H 'Date:Sat, 13 Sep 2025 00:04:34 GMT' \
-H 'Digest:SHA-256=RBNvo1WzZ4oRRq0W9+hknpT7T8If536DEMBg9hyq/4o=' \
-H 'X-Custom-Header-B:test2' \  # 缺少X-Custom-Header-A
-H 'Content-Type: application/json' \
-d '{}'
```

- 响应：`401 Unauthorized`
- 错误信息：`{"message":"client request can't be validated: expected header "X-Custom-Header-A" missing in signing"}`

3. **请求体被篡改**
```shell
curl -X POST 'http://localhost:8082/foo' \
-H 'Authorization:Signature keyId="consumer1-key",algorithm="hmac-sha256",headers="@request-target date x-custom-header-a x-custom-header-b",signature="NcA+44FFtl2rjNvV28wSn8Rln02i4i2tFXKp3/ahyYA="' \
-H 'Date:Sat, 13 Sep 2025 00:09:40 GMT' \
-H 'Digest:SHA-256=RBNvo1WzZ4oRRq0W9+hknpT7T8If536DEMBg9hyq/4o=' \
-H 'X-Custom-Header-A:test1' \
-H 'X-Custom-Header-B:test2' \
-H 'Content-Type: application/json' \
-d '{"key":"value"}'  # 篡改后的请求体
```
- 响应：`401 Unauthorized`
- 错误信息：`{"message":"client request can't be validated: Invalid digest"}`

### 网关实例级别开启全局认证

以下配置将在网关实例级别开启 Hmac Auth 认证，**所有请求必须经过认证才能访问**：

```yaml
global_auth: true  # 开启全局认证
consumers:
- name: consumer1
  access_key: consumer1-key
  secret_key: 2bda943c-ba2b-11ec-ba07-00163e1250b5
- name: consumer2
  access_key: consumer2-key
  secret_key: c8c8e9ca-558e-4a2d-bb62-e700dcc40e35
```

**说明**：当 `global_auth: true` 时，所有访问网关的请求都需要携带有效的认证信息，未认证的请求将被直接拒绝