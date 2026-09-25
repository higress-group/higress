---
title: External Authentication
keywords: [higress, auth]
description: The Ext Authentication plugin implements the capability to call external authorization services for authentication and authorization.
---

## Feature Description

The `ext-auth` plugin sends an authorization request to an external authorization service to check if the client request is authorized. When implementing this plugin, it refers to the native [ext_authz filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_authz_filter) of Envoy, and realizes part of the capabilities of the native filter to connect to an HTTP service.

## Operating Attributes

Plugin Execution Phase: `Authentication Phase`
Plugin Execution Priority: `360`


## Configuration Fields

| Name | Data Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| `http_service` | object | Yes | - | Configuration for the external authorization service |
| `match_type` | string | No |  | Can be `whitelist` or `blacklist` |
| `match_list` | array of MatchRule | No |  | Request matching rules for domains, methods, paths, and request-header presence |
| `failure_mode_allow` | bool | No | false | When set to true, client requests will be accepted even if the communication with the authorization service fails or the authorization service returns an HTTP 5xx error |
| `failure_mode_allow_header_add` | bool | No | false | When both `failure_mode_allow` and `failure_mode_allow_header_add` are set to true, if the communication with the authorization service fails or the authorization service returns an HTTP 5xx error, the `x-envoy-auth-failure-mode-allowed: true` header will be added to the request header |
| `status_on_error` | int | No | 403 | Sets the HTTP status code returned to the client when the authorization service is inaccessible or has a 5xx status code. The default status code is `403` |
| `cache` | object | No | - | Auth-result cache configuration, disabled by default. When enabled, allowed requests that hit the cache no longer call the authorization service |

Configuration fields for each item in `http_service`

| Name | Data Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| `endpoint_mode` | string | No | envoy | Can be `envoy` or `forward_auth` |
| `endpoint` | object | Yes | - | Information about the HTTP service to which the authentication request is sent |
| `timeout` | int | No | 1000 | The connection timeout for the `ext-auth` service in milliseconds |
| `authorization_request` | object | No | - | Configuration for sending the authentication request |
| `authorization_response` | object | No | - | Configuration for handling the authentication response |
| `success_condition` | array of Condition | No | - | After the authorization service returns HTTP 200, further validate its response with a set of conditions; the request is allowed only when all conditions match, otherwise it is rejected with the `status_on_error` status code |

Configuration fields for each item of `Condition` type in `success_condition`

| Name | Data Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| `source` | string | Yes | - | Where the value is extracted from: `status_code` (the authorization response status code), `header` (a response header), or `body_json` (the response body, by gjson path) |
| `key` | string | Required when `source` is `header` or `body_json` | - | For `header`, the response-header name (case-insensitive); for `body_json`, a gjson path such as `data.code`; ignored for `status_code` |
| `op` | string | Yes | - | Comparison operator: `eq`, `ne`, `in`, `not_in`, `exists`, `not_exists`, `gt`, or `lt` |
| `value` | string or array of string | Required except for `exists` and `not_exists` | - | The target value. For `in` and `not_in` it is the list of candidates; other operators use the first element or a plain scalar |

`Condition` evaluation semantics:

- Conditions are ANDed: the request is allowed only when every condition matches; otherwise it is rejected with the `status_on_error` status code.
- It only takes effect when the authorization service returns HTTP 200; non-200 responses follow the original handling.
- `header` names are matched case-insensitively, but `eq`, `ne`, `in`, and `not_in` compare the extracted value case-sensitively (exact match).
- `exists` reflects whether a value can be extracted: a `header` with an empty value counts as absent; a `body_json` path that is explicitly `null` counts as present with an empty-string value.
- `gt` and `lt` compare both sides as floating-point numbers; if either side fails to parse, the condition does not match.
- When no value can be extracted (unknown source, missing header, non-existent gjson path, or empty body), every operator except `not_exists` does not match.

Configuration fields for each item in `endpoint`

| Name | Data Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| `service_name` | string | Yes | - | Enter the name of the authorization service, the full FQDN name with service type, e.g., `ext-auth.dns`, `ext-auth.my-ns.svc.cluster.local` |
| `service_port` | int | No | 80 | Enter the service port of the authorization service |
| `service_host` | string | No | - | The Host header set when requesting the authorization service. If not filled, it will be the same as the FQDN |
| `path_prefix` | string | Required when `endpoint_mode` is `envoy` | - | When `endpoint_mode` is `envoy`, the request path prefix for the client to send a request to the authorization service |
| `request_method` | string | No | GET | When `endpoint_mode` is `forward_auth`, the HTTP Method for the client to send a request to the authorization service |
| `path` | string | Required when `endpoint_mode` is `forward_auth` | - | When `endpoint_mode` is `forward_auth`, the request path for the client to send a request to the authorization service |

Configuration fields for each item in `authorization_request`

| Name | Data Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| `allowed_headers` | array of StringMatcher | No | - | After setting, the client request headers that match the items will be added to the request headers in the authorization service request. In addition to the user-defined header matching rules, the `Authorization` HTTP header will be automatically included in the authorization service request (when `endpoint_mode` is `forward_auth`, the `X-Forwarded-*` request headers will be added) |
| `allowed_properties` | array of AllowedProperty | No | - | When set, Envoy filter state properties will be mapped to HTTP headers and sent to the authorization service.<br>Check out following documents for the property list supported by Envoy:<br><ul><li>Envoy 1.27 (Higress < 2.2.0): https://www.envoyproxy.io/docs/envoy/v1.27.0/intro/arch_overview/advanced/attributes</li><li>Envoy 1.36 (Higress >= 2.2.0): https://www.envoyproxy.io/docs/envoy/v1.36.0/intro/arch_overview/advanced/attributes</li></ul> |

| `headers_to_add` | map[string]string | No | - | Sets the list of request headers to be included in the authorization service request. Please note that the client request headers with the same name will be overwritten |
| `with_request_body` | bool | No | false | Buffer the client request body and send it to the authentication request (not effective for HTTP Method GET, OPTIONS, HEAD requests) |
| `max_request_body_bytes` | int | No | 10MB | Sets the maximum size of the client request body to be saved in memory. When the client request body reaches the value set in this field, an HTTP 413 status code will be returned and the authorization process will not be started. Note that this setting takes precedence over the `failure_mode_allow` configuration |

Configuration fields for each item of `AllowedProperty` type

| Name | Data Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| `path` | array of string | Yes | - | Property path, e.g., `["route_name"]` or `["metadata", "user_id"]` |
| `header` | string | Yes | - | The request header name to map the property to |

Configuration fields for each item in `authorization_response`

| Name | Data Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| `allowed_upstream_headers` | array of StringMatcher | No | - | The response headers of the authentication request that match the items will be added to the original client request headers. Please note that the request headers with the same name will be overwritten |
| `allowed_client_headers` | array of StringMatcher | No | - | If not set, when the request is rejected, all the response headers of the authentication request will be added to the client's response headers. When set, when the request is rejected, the response headers of the authentication request that match the items will be added to the client's response headers |
| `mapped_upstream_headers` | array of HeaderMapping | No | - | Extract a value from the authorization response by source and forward it to the upstream request under a new header name. Complements `allowed_upstream_headers`, which forwards same-name headers |

Configuration fields for each item of `HeaderMapping` type in `mapped_upstream_headers`

| Name | Data Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| `source` | string | Yes | - | Where the value is extracted from: `status_code`, `header`, or `body_json`, with the same meaning as in `Condition` |
| `key` | string | Required when `source` is `header` or `body_json` | - | For `header`, the response-header name (case-insensitive); for `body_json`, a gjson path; ignored for `status_code` |
| `to_header` | string | Yes | - | The request-header name used when forwarding to the upstream |

`HeaderMapping` forwarding semantics:

- It only takes effect when the request is allowed (after `success_condition` passes), and runs after `allowed_upstream_headers`.
- When no value can be extracted or the value is an empty string, the header is skipped and not injected.
- The header is set with an overwrite, so an existing upstream request header with the same name is replaced.

Configuration fields for each item of `StringMatcher` type. When using `array of StringMatcher`, the StringMatchers defined in the array will be configured in order.

| Name | Data Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| `exact` | string | No, one of `exact`, `prefix`, `suffix`, `contains`, `regex` must be selected | - | Exact match |
| `prefix` | string | No, one of `exact`, `prefix`, `suffix`, `contains`, `regex` must be selected | - | Prefix match |
| `suffix` | string | No, one of `exact`, `prefix`, `suffix`, `contains`, `regex` must be selected | - | Suffix match |
| `contains` | string | No, one of `exact`, `prefix`, `suffix`, `contains`, `regex` must be selected | - | Contains |
| `regex` | string | No, one of `exact`, `prefix`, `suffix`, `contains`, `regex` must be selected | - | Regular expression match |

Configuration fields for each item of `MatchRule` type. When using `array of MatchRule`, the MatchRules defined in the array will be configured in order.

| Name | Data Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| `match_rule_domain` | string | No | - | The domain of the matching rule, supports wildcard patterns, e.g., `*.bar.com` |
| `match_rule_method` | []string | No | - | Matching rule for the request method |
| `match_rule_path` | string | No | - | The rule for matching the request path |
| `match_rule_type` | string | No | - | The type of the rule for matching the request path, can be `exact`, `prefix`, `suffix`, `contains`, `regex` |
| `match_rule_headers` | array of HeaderPresenceCondition | No | - | Matches request-header presence; the array must not be empty and every condition in the rule must match |

Configuration fields for each item of `HeaderPresenceCondition` type:

| Name | Data Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| `name` | string | Yes | - | An HTTP request-header name, matched case-insensitively. Case-insensitive duplicates within one rule are not allowed |
| `exists` | bool | Yes | - | `true` requires the header to be present and `false` requires it to be absent. A header with an empty value is still present |

Configuration fields for each item in `cache`

| Name | Data Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| `enabled` | bool | No | false | Whether to enable the auth-result cache. Disabled by default; when disabled no Redis client is created and every request goes straight to the authorization service |
| `ttl` | int | Required when enabled | - | Cache entry lifetime in seconds, in the range `[1, 600]`; values above the maximum are clamped to `600`; when missing or non-positive the cache stays off (treated as disabled, not an error) |
| `key_fields` | array | No | - | The list of fields that compose a custom cache key. When omitted the default key is used (method + path + all request headers forwarded to the authorization service); when set, the key is composed only of the method, the query-free path, and the fields listed here. An invalid value (not an array, an item missing `source`/`key`, an unsupported `source`, or an empty `key`) causes the whole plugin configuration to be rejected |
| `redis` | object | Required when enabled | - | The Redis service used by the cache |

Configuration fields for each item in `cache.key_fields`

| Name | Data Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| `source` | string | Yes | - | Where the field is read from: `header` (a request header) or `query` (a query parameter) |
| `key` | string | Yes | - | The request header name or query parameter name; must not be empty |

Configuration fields for each item in `cache.redis`

| Name | Data Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| `service_name` | string | Yes | - | The Redis service name, a full FQDN with the service type, such as `redis.dns` or `redis.my-ns.svc.cluster.local` |
| `service_port` | int | No | 6379 | The Redis service port; defaults to `80` when `service_name` ends with `.static`, otherwise `6379` |
| `timeout` | int | No | 1000 | The Redis call timeout in milliseconds |
| `username` | string | No | - | The Redis username |
| `password` | string | No | - | The Redis password |
| `database` | int | No | 0 | The Redis database number |

`cache` semantics:

- Only allow decisions are cached: the headers injected upstream are written to the cache only when the authorization service returns HTTP 200 and `success_condition` (if configured) passes; rejections are never cached.
- The cache key has two modes:
  - Default (`key_fields` not set): derived with SHA-256 from the method and path sent to the authorization service plus all request headers forwarded to the authorization service (including credentials), so any change lands on a different entry and avoids cross-contamination. Suited to credentials that stay stable and reusable for a period of time.
  - Custom (`key_fields` set): derived with SHA-256 only from the method, the query-free path, and the values of the fields listed in `key_fields` in declaration order. In this mode the query string is no longer folded into the key wholesale, and `Authorization` and the headers automatically added by `forward_auth` are not included unless explicitly listed. Suited to credentials that are unique per request (such as a per-request-signed `Authorization`), where the default key would never hit the cache.
  - In custom mode the method and the query-free path are always part of the key as a floor, and a listed field that is absent from the request contributes an empty value. Make sure the listed fields are enough to tell callers apart: inputs that are not listed do not participate in the key, so requests landing on the same key share one cached allow decision until its `ttl` expires.
- On a cache hit, the headers injected by the previous allow are replayed and the request is allowed without calling the authorization service again.
- Fail-open: when Redis is not ready, a connection or call fails, or the cached value is corrupt, the request falls back to a real authorization call; a cache problem never blocks a request.
- The cache is skipped when `authorization_request.with_request_body` is `true`, because the body influences the decision but is not part of the key, which would risk cross-contamination.
- The cache gate requires `enabled: true`, a positive `ttl`, and no forwarded request body; if any is unmet the cache is treated as off — no Redis client is created, no error is raised, and the request follows the original cache-free flow.
- The TTL is capped at `600` seconds so a stale allow can never outlive it.

### Differences between the two `endpoint_mode`

When `endpoint_mode` is `envoy`, the authentication request will use the original request's HTTP Method and the configured `path_prefix` as the request path prefix, concatenated with the original request path.

When `endpoint_mode` is `forward_auth`, the authentication request will use the configured `request_method` as the HTTP Method and the configured `path` as the request path. Higress will automatically generate and send the following headers to the authorization service:

| Header | Description |
| --- | --- |
| `x-forwarded-proto` | The scheme of the original request, such as http/https |
| `x-forwarded-method` | The method of the original request, such as get/post/delete/patch |
| `x-forwarded-host` | The host of the original request |
| `x-forwarded-uri` | The path of the original request, including path parameters, e.g., `/v1/app?test=true` |

### Blacklist and Whitelist Modes

Supports blacklist and whitelist modes. Whitelist is the default, and an empty whitelist sends every request to external authorization. A matching `whitelist` rule bypasses authorization, while a miss executes it; a matching `blacklist` rule executes authorization, while a miss bypasses it. Domain, method, path, and header conditions within one rule are ANDed, while entries in `match_list` are ORed. Domains support wildcards such as `*.bar.com`, and paths support `exact`, `prefix`, `suffix`, `contains`, and `regex`. `authorization_request.allowed_headers` only controls which headers are forwarded to the authorization service; it does not control whether that service is called.

**Whitelist Mode**

```yaml
# Configuration for the whitelist mode. Requests that match the whitelist rules do not need verification.
match_type: 'whitelist'
match_list:
  # Requests with the domain name api.example.com and a path prefixed with /public do not need verification.
  - match_rule_domain: 'api.example.com'
    match_rule_path: '/public'
    match_rule_type: 'prefix'
  # For the image resource server images.example.com, all GET requests do not need verification.
  - match_rule_domain: 'images.example.com'
    match_rule_method: ["GET"]
  # For all domains, HEAD requests with an exact path match of /health-check do not need verification.
  - match_rule_method: ["HEAD"]
    match_rule_path: '/health-check'
    match_rule_type: 'exact'
```

**Blacklist Mode**

```yaml
# Configuration for the blacklist mode. Requests that match the blacklist rules need verification.
match_type: 'blacklist'
match_list:
  # Requests with the domain name admin.example.com and a path prefixed with /sensitive need verification.
  - match_rule_domain: 'admin.example.com'
    match_rule_path: '/sensitive'
    match_rule_type: 'prefix'
  # For all domains, DELETE requests with an exact path match of /user need verification.
  - match_rule_method: ["DELETE"]
    match_rule_path: '/user'
    match_rule_type: 'exact'
  # For the domain legacy.example.com, all POST requests need verification.
  - match_rule_domain: 'legacy.example.com'
    match_rule_method: ["POST"]
  # Requests containing the x-custom-auth header need verification.
  - match_rule_headers:
      - name: 'x-custom-auth'
        exists: true
```


## Configuration Examples

Assume that in Kubernetes, the `ext-auth` service has a `serviceName` of `ext-auth`, a port of `8090`, a path of `/auth`, and is in the `backend` namespace.

### When endpoint_mode is envoy

#### Example 1

Configuration of the `ext-auth` plugin:

```yaml
http_service:
  endpoint_mode: envoy
  endpoint:
    service_name: ext-auth.backend.svc.cluster.local
    service_port: 8090
    path_prefix: /auth
  timeout: 1000
```

When using the following request to the gateway after enabling the `ext-auth` plugin:

```shell
curl -X POST http://localhost:8082/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 -X GET -H "foo: bar" -H "Authorization: xxx"
```

**When the request to the `ext-auth` service is successful**:

The `ext-auth` service will receive the following authorization request:

```
POST /auth/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 HTTP/1.1
Host: ext-auth.backend.svc.cluster.local
Authorization: xxx
Content-Length: 0
```

**When the request to the `ext-auth` service fails**:

When the response from the `ext-auth` service is 5xx, the client will receive an HTTP response code of 403 and all the response headers returned by the `ext-auth` service.

If the `ext-auth` service returns response headers of `x-auth-version: 1.0` and `x-auth-failed: true`, they will be passed to the client.

```
HTTP/1.1 403 Forbidden
x-auth-version: 1.0
x-auth-failed: true
date: Tue, 16 Jul 2024 00:19:41 GMT
server: istio-envoy
content-length: 0
```

When the `ext-auth` service is inaccessible or the status code is 5xx, the client request will be rejected with the status code configured in `status_on_error`.

When the `ext-auth` service returns other HTTP status codes, the client request will be rejected with the returned status code. If `allowed_client_headers` is configured, the response headers with corresponding matching items will be added to the client's response.

#### Example 2

Configuration of the `ext-auth` plugin:

```yaml
http_service:
  authorization_request:
    allowed_headers:
      - exact: x-auth-version
    headers_to_add:
      x-envoy-header: true
  authorization_response:
    allowed_upstream_headers:
      - exact: x-user-id
      - exact: x-auth-version
  endpoint_mode: envoy
  endpoint:
    service_name: ext-auth.backend.svc.cluster.local
    service_host: my-domain.local
    service_port: 8090
    path_prefix: /auth
  timeout: 1000
```

When using the following request to the gateway after enabling the `ext-auth` plugin:

```shell
curl -X POST http://localhost:8082/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 -X GET -H "foo: bar" -H "Authorization: xxx"
```

The `ext-auth` service will receive the following authorization request:

```
POST /auth/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 HTTP/1.1
Host: my-domain.local
Authorization: xxx
X-Auth-Version: 1.0
x-envoy-header: true
Content-Length: 0
```

If the response headers returned by the `ext-auth` service contain `x-user-id` and `x-auth-version`, these two headers will be included in the request when the gateway calls the upstream.

#### Example 3: Passing Route Name to Authorization Service

Configuration of the `ext-auth` plugin:

```yaml
http_service:
  authorization_request:
    allowed_headers:
      - exact: x-auth-version
    allowed_properties:
      - path: [route_name]
        header: x-route-name
    headers_to_add:
      x-envoy-header: true
  authorization_response:
    allowed_upstream_headers:
      - exact: x-user-id
      - exact: x-auth-version
  endpoint_mode: envoy
  endpoint:
    service_name: ext-auth.backend.svc.cluster.local
    service_host: my-domain.local
    service_port: 8090
    path_prefix: /auth
  timeout: 1000
```

When using the following request to the gateway after enabling the `ext-auth` plugin:

```shell
curl -X POST http://localhost:8082/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 -X GET -H "foo: bar" -H "Authorization: xxx"
```

The `ext-auth` service will receive the following authorization request:

```
POST /auth/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 HTTP/1.1
Host: my-domain.local
Authorization: xxx
X-Auth-Version: 1.0
x-envoy-header: true
Content-Length: 0
X-Route-Name: your-route-name
```

By configuring `allowed_properties`, you can map Envoy filter state properties like `route_name` to HTTP headers and send them to the authorization service, enabling the authorization service to make decisions based on routing information.

#### Example 4: Validate the authorization response and forward fields to the upstream

Configuration of the `ext-auth` plugin:

```yaml
http_service:
  endpoint_mode: envoy
  endpoint:
    service_name: ext-auth.backend.svc.cluster.local
    service_port: 8090
    path_prefix: /auth
  timeout: 1000
  # After the authorization service returns 200, also require response body data.code == "0" to allow
  success_condition:
    - source: body_json
      key: data.code
      op: eq
      value: "0"
  authorization_response:
    # Extract fields from the authorization response and forward them to the upstream under new names
    mapped_upstream_headers:
      - source: body_json
        key: data.uid
        to_header: x-auth-user-id
      - source: header
        key: x-user-token
        to_header: x-auth-token
```

When the authorization service returns HTTP 200 with the body `{"data": {"code": "0", "uid": "1001"}}` and the response header `x-user-token: abc`, the request is allowed and the request sent to the upstream carries `x-auth-user-id: 1001` and `x-auth-token: abc`. If `data.code` is not `"0"`, the client request is rejected with `status_on_error` (403 by default) even though the status code is 200.

#### Example 5: Enable the auth-result cache

Configuration of the `ext-auth` plugin:

```yaml
http_service:
  endpoint_mode: envoy
  endpoint:
    service_name: ext-auth.backend.svc.cluster.local
    service_port: 8090
    path_prefix: /auth
  timeout: 1000
  authorization_response:
    allowed_upstream_headers:
      - exact: x-user-id
# Enable the auth-result cache; allow decisions are cached in Redis for 60 seconds
cache:
  enabled: true
  ttl: 60
  redis:
    service_name: redis.dns
    service_port: 6379
```

Once enabled, for the same request method, path, and request headers forwarded to the authorization service (including credentials), the first request calls the authorization service; after it is allowed, the headers injected upstream (such as `x-user-id`) are written to Redis and live for `ttl` seconds. Subsequent requests that hit the cache replay that allow decision and no longer call the authorization service. When Redis is not ready, a call fails, or the cached value is corrupt, the request always fails open (falling back to a real authorization call). The cache is not used when `authorization_request.with_request_body` is `true`.

If the credential is unique per request (for example an `Authorization` signed per request), the default key treats every request as a new entry and almost never hits the cache. In that case use `key_fields` to specify which fields compose the key — for example telling callers apart only by the query parameter `userId` and the request header `x-app-key`:

```yaml
http_service:
  endpoint_mode: envoy
  endpoint:
    service_name: ext-auth.backend.svc.cluster.local
    service_port: 8090
    path_prefix: /auth
  timeout: 1000
  authorization_response:
    allowed_upstream_headers:
      - exact: x-user-id
cache:
  enabled: true
  ttl: 60
  # The cache key is composed only of the method, the query-free path, and the
  # fields listed below; Authorization and unlisted query parameters no longer
  # take part in the key
  key_fields:
    - source: query
      key: userId
    - source: header
      key: x-app-key
  redis:
    service_name: redis.dns
    service_port: 6379
```

Note: inputs not listed in `key_fields` (including `Authorization` and unlisted query parameters) do not take part in the key, so requests landing on the same key share one cached allow decision until its `ttl` expires. Make sure the listed fields are enough to tell callers apart.

### When endpoint_mode is forward_auth

#### Example 1

Configuration of the `ext-auth` plugin:

```yaml
http_service:
  endpoint_mode: forward_auth
  endpoint:
    service_name: ext-auth.backend.svc.cluster.local
    service_port: 8090
    path: /auth
    request_method: POST
  timeout: 1000
```

When using the following request to the gateway after enabling the `ext-auth` plugin:

```shell
curl -i http://localhost:8082/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 -X GET -H "foo: bar" -H "Authorization: xxx" -H "Host: foo.bar.com"
```

**When the request to the `ext-auth` service is successful**:

The `ext-auth` service will receive the following authorization request:

```
POST /auth HTTP/1.1
Host: ext-auth.backend.svc.cluster.local
Authorization: xxx
X-Forwarded-Proto: HTTP
X-Forwarded-Host: foo.bar.com
X-Forwarded-Uri: /users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5
X-Forwarded-Method: GET
Content-Length: 0
```

**When the request to the `ext-auth` service fails**:

When the response from the `ext-auth` service is 5xx, the client will receive an HTTP response code of 403 and all the response headers returned by the `ext-auth` service.

If the `ext-auth` service returns response headers of `x-auth-version: 1.0` and `x-auth-failed: true`, they will be passed to the client.

```
HTTP/1.1 403 Forbidden
x-auth-version: 1.0
x-auth-failed: true
date: Tue, 16 Jul 2024 00:19:41 GMT
server: istio-envoy
content-length: 0
```

When the `ext-auth` service is inaccessible or the status code is 5xx, the client request will be rejected with the status code configured in `status_on_error`.

When the `ext-auth` service returns other HTTP status codes, the client request will be rejected with the returned status code. If `allowed_client_headers` is configured, the response headers with corresponding matching items will be added to the client's response.

#### Example 2

Configuration of the `ext-auth` plugin:

```yaml
http_service:
  authorization_request:
    allowed_headers:
      - exact: x-auth-version
    headers_to_add:
      x-envoy-header: true
  authorization_response:
    allowed_upstream_headers:
      - exact: x-user-id
      - exact: x-auth-version
  endpoint_mode: forward_auth
  endpoint:
    service_name: ext-auth.backend.svc.cluster.local
    service_host: my-domain.local
    service_port: 8090
    path: /auth
    request_method: POST
  timeout: 1000
```

When using the following request to the gateway after enabling the `ext-auth` plugin:

```shell
curl -i http://localhost:8082/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 -X GET -H "foo: bar" -H "Authorization: xxx" -H "X-Auth-Version: 1.0" -H "Host: foo.bar.com"
```

The `ext-auth` service will receive the following authorization request:

```
POST /auth HTTP/1.1
Host: my-domain.local
Authorization: xxx
X-Forwarded-Proto: HTTP
X-Forwarded-Host: foo.bar.com
X-Forwarded-Uri: /users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5
X-Forwarded-Method: GET
X-Auth-Version: 1.0
x-envoy-header: true
Content-Length: 0
```

If the response headers returned by the `ext-auth` service contain `x-user-id` and `x-auth-version`, these two headers will be included in the request when the gateway calls the upstream.

#### Example 3: Passing Route Name to Authorization Service

Configuration of the `ext-auth` plugin:

```yaml
http_service:
  authorization_request:
    allowed_headers:
      - exact: x-auth-version
    allowed_properties:
      - path: [route_name]
        header: x-route-name
  authorization_response:
    allowed_upstream_headers:
      - exact: x-mse-consumer
      - exact: x-ext-auth-user
  endpoint_mode: forward_auth
  endpoint:
    service_name: ext-auth.backend.svc.cluster.local
    service_port: 8090
    path: /auth
    request_method: POST
  timeout: 1000
```

When using the following request to the gateway after enabling the `ext-auth` plugin:

```shell
curl -i http://localhost:8082/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 -X GET -H "foo: bar" -H "Authorization: xxx" -H "X-Auth-Version: 1.0" -H "Host: foo.bar.com"
```

The `ext-auth` service will receive the following authorization request:

```
POST /auth HTTP/1.1
Host: my-domain.local
Authorization: xxx
X-Forwarded-Proto: HTTP
X-Forwarded-Host: foo.bar.com
X-Forwarded-Uri: /users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5
X-Forwarded-Method: GET
X-Auth-Version: 1.0
x-envoy-header: true
X-Route-Name: your-route-name
Content-Length: 0
```

By configuring `allowed_properties`, you can map Envoy filter state properties like `route_name` to HTTP headers and send them to the authorization service, enabling the authorization service to make decisions based on routing information.
