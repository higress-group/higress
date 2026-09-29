---
title: APISIX HMAC Authentication
keywords: [higress, hmac auth, apisix]
description: Configuration Reference for the APISIX HMAC Authentication Plugin
---

## Feature Description
The `hmac-auth-apisix` plugin is compatible with Apache APISIX's HMAC authentication mechanism. It generates tamper-proof digital signatures for HTTP requests using the HMAC algorithm, enabling request identity authentication and permission control. This plugin is fully compatible with the configuration and signature algorithm of the Apache APISIX HMAC Authentication Plugin. For signature generation methods, please refer to the [Apache APISIX HMAC Authentication Documentation](https://apisix.apache.org/docs/apisix/plugins/hmac-auth/).


## Operational Attributes
- Plugin Execution Phase: `Authentication Phase`
- Plugin Execution Priority: `330`


## Configuration Fields
**Note:**
- In a single rule, authentication configuration and authorization configuration cannot coexist.
- For requests that pass authentication and authorization, a `X-Mse-Consumer` field will be added to the request header to identify the caller's name.
  The gateway **replaces** this header instead of appending to it: any client-supplied `X-Mse-Consumer` is removed
  before the consumer name from this authentication is set. Downstream therefore always reads the gateway's own
  assertion, so a caller cannot forge its identity by sending the header itself.


### Authentication Configuration

| Name                    | Data Type        | Requirements                              | Default Value                                      | Description                                                                                                                                                                                                 |
| ----------------------- | ---------------- | ----------------------------------------- | --------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `global_auth`           | bool             | Optional (**Instance-level configuration only**) | -                                                   | Can only be configured at the instance level. If set to `true`, the authentication mechanism takes effect globally; if set to `false`, authentication only applies to domains and routes with specific configurations. If not configured, it takes effect globally only when there are no domain or route configurations (to maintain compatibility with legacy user habits). |
| `consumers`             | array of object  | Required                                  | -                                                   | Configures service callers for request authentication.                                                                                                                                                     |
| `allowed_algorithms`    | array of string  | Optional                                  | ["hmac-sha1", "hmac-sha256", "hmac-sha512"]        | List of allowed HMAC algorithms. Valid values are combinations of "hmac-sha1", "hmac-sha256", and "hmac-sha512".                                                                                           |
| `clock_skew`            | number           | Optional                                  | 300                                                 | Maximum allowed time difference (in seconds) between the timestamp of the client request and the current time of the Higress server. This helps resolve time synchronization differences between the client and server and prevents replay attacks. The timestamp is calculated based on the time in the `Date` header (must be in GMT format). If set to `0`, this check is skipped. |
| `signed_headers`        | array of string  | Optional                                  | -                                                   | List of HTTP headers that should be included in the HMAC signature of the client request.                                                                                                                  |
| `validate_request_body` | boolean          | Optional                                  | false                                               | If set to `true`, the integrity of the request body is verified to ensure no tampering during transmission. Specifically, the plugin creates a SHA-256 base64-encoded digest and compares it with the `Digest` header. Verification fails if the `Digest` header is missing or the digest does not match. |
| `hide_credentials`      | boolean          | Optional                                  | false                                               | If set to `true`, the authorization request header will not be passed to the upstream service.                                                                                                              |
| `anonymous_consumer`    | string           | Optional                                  | -                                                   | Name of the anonymous consumer. If configured, anonymous users are allowed to bypass identity authentication.                                                                                              |


### Configuration Fields for Each Item in `consumers`

| Name         | Data Type | Requirements | Default Value | Description                                                                 |
|--------------|-----------|--------------|---------------|-----------------------------------------------------------------------------|
| `access_key` | string    | Required     | -             | A unique identifier for the consumer, used to reference configurations such as the secret key. |
| `secret_key` | string    | Required     | -             | Secret key used to generate the HMAC signature.                             |
| `name`       | string    | Optional     | `access_key`  | Name of the consumer.                                                       |


### Authorization Configuration (Non-essential)

| Name    | Data Type        | Requirements                              | Default Value | Description                                                                                                                                 |
|---------|------------------| ----------------------------------------- |---------------|---------------------------------------------------------------------------------------------------------------------------------------------|
| `allow` | array of string  | Optional (**Non-instance-level configuration only**) | -             | Can only be configured in fine-grained rules such as routes or domains. For requests that match the criteria, it configures the consumers allowed to access, enabling fine-grained permission control. **When a rule enables the plugin but `allow` is missing (or an empty list), no consumer is authorized and matching requests are rejected (fail closed).** |

**Fail-closed semantics of `allow`:**

- Once a fine-grained rule (`_rules_`) matches the current request, the plugin is in effect on that domain or route. A missing or empty `allow` list there **no longer means "the plugin is not in effect"**; it means no consumer is authorized, so every request matching that rule is rejected with `401` and the message `{"message":"client request can't be validated: no consumer is allowed"}` instead of being forwarded without authentication.
- This also holds when `anonymous_consumer` is configured: the anonymous identity cannot bypass the rejection above.
- With `global_auth: true`, authentication applies globally and `allow` is only an additional fine-grained restriction, so a rule without `allow` adds no restriction and requests still have to pass signature verification.
- With `global_auth: false`, only domains and routes matched by a rule are authenticated; requests that match no rule are passed through.


## Configuration Examples

### Global Configuration Authentication and Route-Level Authorization

The following configuration is used to enable Hmac Auth authentication and authorization for specific routes or domains of the gateway. **Note: The `access_key` field must be unique.**


#### Example 1: Basic Route and Domain Authorization Configuration

**Instance-Level Plugin Configuration**:
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

**Route-Level Configuration** (applicable to `route-a` and `route-b`):
```yaml
allow: 
- consumer1  # Only consumer1 is allowed access
```  

**Domain-Level Configuration** (applicable to `*.example.com` and `test.com`):
```yaml
allow:
- consumer2  # Only consumer2 is allowed access
```  


#### Configuration Instructions:
- **Route Names** (e.g., `route-a`, `route-b`): Correspond to the names defined when creating gateway routes. Only `consumer1` is allowed access when matched.
- **Domain Matching** (e.g., `*.example.com`, `test.com`): Used to filter request domains. Only `consumer2` is allowed access when matched.
- Callers not in the `allow` list will be denied access.
- If the plugin is enabled on a route or domain without an `allow` list (or with an empty one), then **all** requests on that route or domain are rejected with `401` — the plugin fails closed instead of skipping authentication.


#### To Generate a Signature, Use the Following Go Code Snippet or Other Tech Stacks:

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

// SignedHeader defines the structure of signed headers
type SignedHeader struct {
	Name  string
	Value string
}

func main() {
	// Configuration parameters
	keyID := "consumer1-key"                            // Key ID
	secretKey := "2bda943c-ba2b-11ec-ba07-00163e1250b5" // Secret key
	requestMethod := "POST"                             // HTTP method
	requestPath := "/foo"                               // Route URI
	algorithm := "hmac-sha256"                          // Algorithm
	validateRequestBody := false                        // Whether to validate the request body; set to true to add the Digest header

	// If signed_headers is configured, add them in order
	signedHeaders := []SignedHeader{
		//{Name: "x-custom-header-a", Value: "test1"},
		//{Name: "x-custom-header-b", Value: "test2"},
	}

	body := []byte("{}") // Request body

	// Get current GMT time
	gmtTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")

	// Dynamically construct the signing string (in order)
	signingStringBuilder := strings.Builder{}
	signingStringBuilder.WriteString(fmt.Sprintf("%s\n%s %s\ndate: %s\n",
		keyID,
		requestMethod,
		requestPath,
		gmtTime))

	// Add headers in the order specified in signedHeaders
	for _, header := range signedHeaders {
		signingStringBuilder.WriteString(fmt.Sprintf("%s: %s\n", header.Name, header.Value))
	}

	signingString := signingStringBuilder.String()

	// Generate signature
	signature, err := generateHmacSignature(secretKey, algorithm, signingString)
	if err != nil {
		fmt.Printf("Error generating signature: %v\n", err)
		return
	}

	// Dynamically build the content of the headers field
	headersField := "@request-target date"
	for _, header := range signedHeaders {
		headersField += " " + header.Name
	}

	// Construct request headers
	headers := map[string]string{
		"Date": gmtTime,
		"Authorization": fmt.Sprintf(`Signature keyId="%s",algorithm="%s",headers="%s",signature="%s"`,
			keyID,
			algorithm,
			headersField,
			signature,
		),
	}

	// Add Digest header if request body validation is required
	if validateRequestBody {
		headers["Digest"] = calculateBodyDigest(body)
	}

	// Add signed request headers
	for _, header := range signedHeaders {
		formattedHeaderName := formatHeaderName(header.Name)
		headers[formattedHeaderName] = header.Value
	}

	// Print the signing string
	fmt.Printf("signingString: %s\n", signingString)
	// Print request headers
	fmt.Println("Headers:")
	for key, value := range headers {
		fmt.Printf("%s: %s\n", key, value)
	}
}

// generateHmacSignature generates an HMAC signature
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

// calculateBodyDigest calculates the digest of the request body
func calculateBodyDigest(body []byte) string {
	hash := sha256.Sum256(body)
	encodedDigest := base64.StdEncoding.EncodeToString(hash[:])
	return "SHA-256=" + encodedDigest
}

// formatHeaderName converts the header name to standard HTTP header format
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


#### Request and Response Examples:

1. **Validation Passed Scenario**
```shell
curl -X POST 'http://localhost:8082/foo' \
-H 'Authorization:Signature keyId="consumer1-key",algorithm="hmac-sha256",headers="@request-target date",signature="746z4VISwZehUwZdzTV486ZMMbBtakmMHKPfs/A4RdU="' \
-H 'Date:Fri, 12 Sep 2025 23:53:18 GMT' \
-H 'Content-Type: application/json' \
-d '{}'
```  
- **Response**: Returns a normal response from the backend service.
- **Additional Info**: After successful authentication, the request header `X-Mse-Consumer: consumer1` is automatically added and passed to the backend.


2. **Signature Verification Failure Due to Modified Request Method**
```shell
curl -X PUT 'http://localhost:8082/foo' \  # POST is modified to PUT here
-H 'Authorization:Signature keyId="consumer1-key",algorithm="hmac-sha256",headers="@request-target date",signature="746z4VISwZehUwZdzTV486ZMMbBtakmMHKPfs/A4RdU="' \
-H 'Date:Fri, 12 Sep 2025 23:53:18 GMT' \
-H 'Content-Type: application/json' \
-d '{}'
```  
- **Response**: `401 Unauthorized`
- **Error Message**: `{"message":"client request can't be validated: Invalid signature"}`


3. **Caller Not in Allow List**
```shell
curl -X POST 'http://localhost:8082/foo' \
-H 'Authorization:Signature keyId="consumer2-key",algorithm="hmac-sha256",headers="@request-target date",signature="dltotPwd4iWGGz//kuehPJlHXZemR5WKwCPAJD/KPhE="' \
-H 'Date:Fri, 12 Sep 2025 23:59:01 GMT' \
-H 'Content-Type: application/json' \
-d '{}'
```  
- **Response**: `401 Unauthorized`
- **Error Message**: `{"message":"client request can't be validated: consumer 'consumer2' is not allowed"}`


4. **Expired Timestamp**
```shell
curl -X POST 'http://localhost:8082/foo' \
-H 'Authorization:Signature keyId="consumer1-key",algorithm="hmac-sha256",headers="@request-target date",signature="746z4VISwZehUwZdzTV486ZMMbBtakmMHKPfs/A4RdU="' \
-H 'Date:Fri, 12 Sep 2025 23:53:18 GMT' \  # Expired timestamp
-H 'Content-Type: application/json' \
-d '{}'
```  
- **Response**: `401 Unauthorized`
- **Error Message**: `{"message":"client request can't be validated: Clock skew exceeded"}`


#### Example 2: Configuration with Custom Signed Headers and Request Body Validation

**Instance-Level Plugin Configuration**:
```yaml
global_auth: false
consumers:
- name: consumer1
  access_key: consumer1-key
  secret_key: 2bda943c-ba2b-11ec-ba07-00163e1250b5
- name: consumer2
  access_key: consumer2-key
  secret_key: c8c8e9ca-558e-4a2d-bb62-e700dcc40e35
signed_headers:  # Custom request headers to be included in the signature
- X-Custom-Header-A
- X-Custom-Header-B
validate_request_body: true  # Enable request body signature verification
```  


#### Request and Response Examples:

1. **Validation Passed Scenario**
```shell
curl -X POST 'http://localhost:8082/foo' \
-H 'Authorization:Signature keyId="consumer1-key",algorithm="hmac-sha256",headers="@request-target date x-custom-header-a x-custom-header-b",signature="KoOlbkDIR/JzlKK47eURewnIpmhpkQU+KIyBUhqVfmo="' \
-H 'Date:Sat, 13 Sep 2025 00:04:34 GMT' \
-H 'Digest:SHA-256=RBNvo1WzZ4oRRq0W9+hknpT7T8If536DEMBg9hyq/4o=' \  # Request body digest
-H 'X-Custom-Header-A:test1' \
-H 'X-Custom-Header-B:test2' \
-H 'Content-Type: application/json' \
-d '{}'
```  
- **Response**: Returns a normal response from the backend service.


2. **Missing Signed Header**
```shell
curl -X POST 'http://localhost:8082/foo' \
-H 'Authorization:Signature keyId="consumer1-key",algorithm="hmac-sha256",headers="@request-target date x-custom-header-b",signature="KoOlbkDIR/JzlKK47eURewnIpmhpkQU+KIyBUhqVfmo="' \
-H 'Date:Sat, 13 Sep 2025 00:04:34 GMT' \
-H 'Digest:SHA-256=RBNvo1WzZ4oRRq0W9+hknpT7T8If536DEMBg9hyq/4o=' \
-H 'X-Custom-Header-B:test2' \  # X-Custom-Header-A is missing
-H 'Content-Type: application/json' \
-d '{}'
```  
- **Response**: `401 Unauthorized`
- **Error Message**: `{"message":"client request can't be validated: expected header \"X-Custom-Header-A\" missing in signing"}`


3. **Tampered Request Body**
```shell
curl -X POST 'http://localhost:8082/foo' \
-H 'Authorization:Signature keyId="consumer1-key",algorithm="hmac-sha256",headers="@request-target date x-custom-header-a x-custom-header-b",signature="NcA+44FFtl2rjNvV28wSn8Rln02i4i2tFXKp3/ahyYA="' \
-H 'Date:Sat, 13 Sep 2025 00:09:40 GMT' \
-H 'Digest:SHA-256=RBNvo1WzZ4oRRq0W9+hknpT7T8If536DEMBg9hyq/4o=' \
-H 'X-Custom-Header-A:test1' \
-H 'X-Custom-Header-B:test2' \
-H 'Content-Type: application/json' \
-d '{"key":"value"}'  # Tampered request body
```  
- **Response**: `401 Unauthorized`
- **Error Message**: `{"message":"client request can't be validated: Invalid digest"}`


### Enable Global Authentication at the Gateway Instance Level

The following configuration enables Hmac Auth authentication at the gateway instance level. **All requests must be authenticated to access the gateway**:

```yaml
global_auth: true  # Enable global authentication
consumers:
- name: consumer1
  access_key: consumer1-key
  secret_key: 2bda943c-ba2b-11ec-ba07-00163e1250b5
- name: consumer2
  access_key: consumer2-key
  secret_key: c8c8e9ca-558e-4a2d-bb62-e700dcc40e35
```  

**Description**: When `global_auth: true`, all requests to the gateway must carry valid authentication information. Unauthenticated requests will be rejected directly.