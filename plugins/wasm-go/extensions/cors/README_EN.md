---
title: Cross-Origin Resource Sharing
keywords: [higress,cors]
description: Cross-Origin Resource Sharing plugin configuration reference
---
## Function Description
The `cors` plugin can enable CORS (Cross-Origin Resource Sharing) HTTP response headers for the server.

## Execution Attributes
Plugin execution phase: `AUTHN`
Plugin execution priority: `2000`

## Release Notes

### 2.0.1

Compared with `2.0.0`, this version aligns the plugin behavior with browser CORS semantics and changes the plugin execution priority to `2000`.

Changes and upgrade notes:

* When the Origin or Method of an actual CORS request does not match the configuration, the plugin no longer returns a direct `403`. The request continues upstream; the plugin does not add CORS allow response headers and removes upstream CORS policy response headers, so browsers block the result according to CORS rules. If monitoring or client logic depends on a gateway-level `403`, adjust it before upgrading.
* CORS preflight requests are answered directly with `204 No Content`. Invalid preflight responses omit `Access-Control-Allow-*`, `Access-Control-Expose-Headers`, `Access-Control-Allow-Credentials`, and `Access-Control-Max-Age`, so browsers treat the preflight as failed. If existing logic depends on invalid preflights returning `403`, adjust it before upgrading.
* Same-origin `OPTIONS` requests continue upstream even when they carry preflight-like request headers, preventing the CORS plugin from intercepting them incorrectly.
* `allow_methods: ["*"]` echoes the current `Access-Control-Request-Method` in preflight responses; `allow_headers: ["*"]` echoes normalized `Access-Control-Request-Headers` and omits `Access-Control-Allow-Headers` when no request headers were requested.
* Default Method/Header values are split by comma and trimmed, preventing defaults from being treated as one unmatchable item.
* Origin pattern matching is anchored to the full Origin value, avoiding accidental matches such as `http://api.example.com.evil.com` for `http://*.example.com`. If an existing configuration relied on partial matching, update it to an explicit Origin pattern.
* When `Access-Control-Allow-Origin` is a specific Origin instead of `*`, the plugin merges `Vary: Origin` into the response to prevent cache reuse with the wrong CORS headers.
* `expose_headers: ["*"]` remains accepted for compatibility when credentials are allowed, but browsers treat `Access-Control-Expose-Headers: *` as a literal header name for credentialed requests instead of exposing all headers.

## Configuration Fields
| Name                  | Data Type        | Required | Default Value                                                                                                                | Description                                                                                                                                                                                                                                       |
|-----------------------|------------------|----------|-----------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| allow_origins         | array of string  | Optional | *                                                                                                                           | Allowed Origins for cross-origin access, formatted as scheme://host:port, for example, http://example.com:8081. When allow_credentials is false, * can be used to allow all Origins through.                                                |
| allow_origin_patterns | array of string  | Optional | -                                                                                                                           | Patterns for matching allowed Origins for cross-origin access, using * to match domain or port, <br/>for example http://*.example.com -- matches domain, http://*.example.com:[8080,9090] -- matches domain and specified ports, http://*.example.com:[*] -- matches domain and all ports. A single * indicates matching all domains and ports. |
| allow_methods         | array of string  | Optional | GET, PUT, POST, DELETE, PATCH, OPTIONS                                                                                     | Allowed Methods for cross-origin access, for example: GET, POST, etc. `*` can be used to indicate all Methods are allowed; preflight responses echo the current `Access-Control-Request-Method`.                                               |
| allow_headers         | array of string  | Optional | DNT,X-CustomHeader,Keep-Alive,User-Agent,X-Requested-With,<br/>If-Modified-Since,Cache-Control,Content-Type,Authorization | Allowed Headers for the requester to carry that are not part of CORS specifications during cross-origin access. `*` can be used to indicate any Header is allowed; preflight responses echo normalized `Access-Control-Request-Headers`, and omit `Access-Control-Allow-Headers` when no request headers were requested. |
| expose_headers        | array of string  | Optional | -                                                                                                                           | Allowed Headers for the responder to expose during cross-origin access. `*` can be used, but for credentialed requests browsers treat `Access-Control-Expose-Headers: *` as a literal header name instead of exposing all headers.                |
| allow_credentials     | bool             | Optional | false                                                                                                                       | Whether to allow the requester to carry credentials (e.g. Cookies) during cross-origin access. According to CORS specifications, if this option is set to true, * cannot be used for allow_origins, replace it with allow_origin_patterns.  |
| max_age               | number           | Optional | 86400 seconds                                                                                                              | Maximum time for browsers to cache CORS results, in seconds. <br/>Within this time frame, browsers will reuse the previous inspection results.                                                                                                 |
> Note
> * allow_credentials is a very sensitive option, please enable it with caution. Once enabled, allow_credentials and allow_origins cannot both be *, if both are set, the allow_origins value of "*" takes effect.
> * allow_origins and allow_origin_patterns can be set simultaneously. First, check if allow_origins matches, then check if allow_origin_patterns matches.
> * For actual CORS requests, if the Origin or Method does not match the configuration, the plugin continues the request upstream, does not add CORS response headers, and removes upstream CORS policy response headers. Browsers then block the result according to CORS rules.
> * For CORS preflight requests, valid requests are answered directly with `204 No Content` and the configured CORS response headers. Invalid requests are answered directly with `204 No Content`, but without `Access-Control-Allow-*`, `Access-Control-Expose-Headers`, `Access-Control-Allow-Credentials`, or `Access-Control-Max-Age`; browsers treat the preflight as failed.
> * When `Access-Control-Allow-Origin` is a specific Origin instead of `*`, the plugin merges `Vary: Origin` into the response to prevent cache reuse with the wrong CORS headers.

## Configuration Examples
### Allow all cross-origin access, without allowing the requester to carry credentials
```yaml
allow_origins:
  - '*'
allow_methods:
  - '*'
allow_headers:
  - '*'
expose_headers:
  - '*'
allow_credentials: false
max_age: 7200
```

### Allow all cross-origin access, while allowing the requester to carry credentials
```yaml
allow_origin_patterns:
  - '*'
allow_methods:
  - '*'
allow_headers:
  - '*'
expose_headers:
  - '*'
allow_credentials: true
max_age: 7200
```

### Allow specific subdomains, specific methods, and specific request headers for cross-origin access, while allowing the requester to carry credentials
```yaml
allow_origin_patterns:
  - http://*.example.com
  - http://*.example.org:[8080,9090]
allow_methods:
  - GET
  - PUT
  - POST
  - DELETE
allow_headers:
  - Token
  - Content-Type
  - Authorization
expose_headers:
  - '*'
allow_credentials: true
max_age: 7200
```

## Testing
### Test Configuration
```yaml
apiVersion: networking.higress.io/v1
kind: McpBridge
metadata:
  name: mcp-cors-httpbin
  namespace: higress-system
spec:
  registries:
    - domain: httpbin.org
      name: httpbin
      port: 80
      type: dns
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    higress.io/destination: httpbin.dns
    higress.io/upstream-vhost: "httpbin.org"
    higress.io/backend-protocol: HTTP
  name: ingress-cors-httpbin
  namespace: higress-system
spec:
  ingressClassName: higress
  rules:
    - host: httpbin.example.com
      http:
        paths:
          - backend:
              resource:
                apiGroup: networking.higress.io
                kind: McpBridge
                name: mcp-cors-httpbin
            path: /
            pathType: Prefix
---
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: wasm-cors-httpbin
  namespace: higress-system
spec:
  defaultConfigDisable: true
  matchRules:
    - config:
        allow_origins:
          - http://httpbin.example.net
        allow_origin_patterns:
          - http://*.example.com:[*]
          - http://*.example.org:[9090,8080]
        allow_methods:
          - GET
          - POST
          - PATCH
        allow_headers:
          - Content-Type
          - Token
          - Authorization
        expose_headers:
          - X-Custom-Header
          - X-Env-UTM
        allow_credentials: true
        max_age: 3600
      configDisable: false
      ingress:
        - ingress-cors-httpbin
  url: oci://higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/cors:2.0.1
  imagePullPolicy: Always
```

### Request Testing
#### Simple Request
```shell
curl -v -H "Origin: http://httpbin2.example.org:9090" -H "Host: httpbin.example.com" http://127.0.0.1/anything/get\?foo\=1
< HTTP/1.1 200 OK
> x-cors-version: 2.0.1
> access-control-allow-origin: http://httpbin2.example.org:9090
> access-control-expose-headers: X-Custom-Header,X-Env-UTM
> access-control-allow-credentials: true
> vary: Origin
```

#### Preflight Request
```shell
curl -v -X OPTIONS -H "Origin: http://httpbin2.example.org:9090" -H "Host: httpbin.example.com" -H "Access-Control-Request-Method: POST" -H "Access-Control-Request-Headers: Content-Type, Token" http://127.0.0.1/anything/get\?foo\=1
< HTTP/1.1 204 No Content
< x-cors-trace: trace
< access-control-allow-origin: http://httpbin2.example.org:9090
< access-control-allow-methods: GET,POST,PATCH
< access-control-allow-headers: Content-Type,Token,Authorization
< access-control-expose-headers: X-Custom-Header,X-Env-UTM
< access-control-allow-credentials: true
< access-control-max-age: 3600
< vary: Origin
< date: Tue, 23 May 2023 11:41:28 GMT
< server: istio-envoy
< content-length: 0
<
* Connection #0 to host 127.0.0.1 left intact
* Closing connection 0
```

#### Illegal CORS Origin Preflight Request
Invalid preflight requests return `204 No Content` without `access-control-allow-*` CORS policy response headers, so browsers treat the preflight as failed.

```shell
curl -v -X OPTIONS -H "Origin: http://httpbin2.example.org" -H "Host: httpbin.example.com" -H "Access-Control-Request-Method: GET" http://127.0.0.1/anything/get\?foo\=1
< HTTP/1.1 204 No Content
< x-cors-trace: trace
< date: Tue, 23 May 2023 11:27:01 GMT
< server: istio-envoy
< content-length: 0
<
* Connection #0 to host 127.0.0.1 left intact
```

#### Illegal CORS Method Preflight Request
```shell
curl -v -X OPTIONS -H "Origin: http://httpbin2.example.org:9090" -H "Host: httpbin.example.com" -H "Access-Control-Request-Method: DELETE" http://127.0.0.1/anything/get\?foo\=1
< HTTP/1.1 204 No Content
< x-cors-trace: trace
< date: Tue, 23 May 2023 11:28:51 GMT
< server: istio-envoy
< content-length: 0
<
* Connection #0 to host 127.0.0.1 left intact
```

#### Illegal CORS Header Preflight Request
```shell
curl -v -X OPTIONS -H "Origin: http://httpbin2.example.org:9090" -H "Host: httpbin.example.com" -H "Access-Control-Request-Method: GET" -H "Access-Control-Request-Headers: TokenView" http://127.0.0.1/anything/get\?foo\=1
< HTTP/1.1 204 No Content
< x-cors-trace: trace
< date: Tue, 23 May 2023 11:31:03 GMT
< server: istio-envoy
< content-length: 0
<
* Connection #0 to host 127.0.0.1 left intact
```

## Reference Documents
- https://www.ruanyifeng.com/blog/2016/04/cors.html
- https://fetch.spec.whatwg.org/#http-cors-protocol
