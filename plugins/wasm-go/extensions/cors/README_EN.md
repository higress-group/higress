---
title: Cross-Origin Resource Sharing
keywords: [higress,cors]
description: Cross-Origin Resource Sharing plugin configuration reference
---
## Function Description
The `cors` plugin can enable CORS (Cross-Origin Resource Sharing) HTTP response headers for the server.

## Execution Attributes
Plugin execution phase: `Authorization Phase`  
Plugin execution priority: `340`

## Configuration Fields
| Name                  | Data Type        | Required | Default Value                                                                                                                | Description                                                                                                                                                                                                                                       |
|-----------------------|------------------|----------|-----------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| allow_origins         | array of string  | Optional | *                                                                                                                           | Allowed Origins for cross-origin access, formatted as scheme://host:port, for example, http://example.com:8081. When allow_credentials is false, * can be used to allow all Origins through.                                                |
| allow_origin_patterns | array of string  | Optional | -                                                                                                                           | Patterns for matching allowed Origins for cross-origin access, using * to match domain or port, <br/>for example http://*.example.com -- matches domain, http://*.example.com:[8080,9090] -- matches domain and specified ports, http://*.example.com:[*] -- matches domain and all ports. A single * indicates matching all domains and ports. |
| allow_methods         | array of string  | Optional | GET, PUT, POST, DELETE, PATCH, OPTIONS                                                                                     | Allowed Methods for cross-origin access, for example: GET, POST, etc. * can be used to indicate all Methods are allowed.                                                                                                                       |
| allow_headers         | array of string  | Optional | DNT,X-CustomHeader,Keep-Alive,User-Agent,X-Requested-With,<br/>If-Modified-Since,Cache-Control,Content-Type,Authorization | Allowed Headers for the requester to carry that are not part of CORS specifications during cross-origin access. * can be used to indicate any Header is allowed.                                                                                 |
| expose_headers        | array of string  | Optional | -                                                                                                                           | Allowed Headers for the responder to carry that are not part of CORS specifications during cross-origin access. * can be used to indicate any Header is allowed.                                                                                 |
| allow_credentials     | bool             | Optional | false                                                                                                                       | Whether to allow the requester to carry credentials (e.g. Cookies) during cross-origin access. According to CORS specifications, if this option is set to true, * cannot be used for allow_origins, replace it with allow_origin_patterns.  |
| max_age               | number           | Optional | 86400 seconds                                                                                                              | Maximum time for browsers to cache CORS results, in seconds. <br/>Within this time frame, browsers will reuse the previous inspection results.                                                                                                 |
> Note
> * allow_credentials is a very sensitive option, please enable it with caution. Once enabled, allow_credentials and allow_origins cannot both be *, if both are set, the allow_origins value of "*" takes effect.
> * allow_origins and allow_origin_patterns can be set simultaneously. First, check if allow_origins matches, then check if allow_origin_patterns matches.
> * Illegal CORS requests will return HTTP status code 403, with the response body content as "Invalid CORS request".

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
  url: oci://higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/cors:1.0.0
  imagePullPolicy: Always
```

### Request Testing
#### Simple Request
```shell
curl -v -H "Origin: http://httpbin2.example.org:9090" -H "Host: httpbin.example.com" http://127.0.0.1/anything/get\?foo\=1
< HTTP/1.1 200 OK
> x-cors-version: 1.0.0
> access-control-allow-origin: http://httpbin2.example.org:9090
> access-control-expose-headers: X-Custom-Header,X-Env-UTM
> access-control-allow-credentials: true
```

#### Preflight Request
```shell
curl -v -X OPTIONS -H "Origin: http://httpbin2.example.org:9090" -H  "Host: httpbin.example.com" -H "Access-Control-Request-Method: POST"  -H "Access-Control-Request-Headers: Content-Type,Token" http://127.0.0.1/anything/get\?foo\=1

< HTTP/1.1 200 OK
< x-cors-version: 1.0.0
< access-control-allow-origin: http://httpbin2.example.org:9090
< access-control-allow-methods: GET,POST,PATCH
< access-control-allow-headers: Content-Type,Token,Authorization
< access-control-expose-headers: X-Custom-Header,X-Env-UTM
< access-control-allow-credentials: true
< access-control-max-age: 3600
< date: Tue, 23 May 2023 11:41:28 GMT
< server: istio-envoy
< content-length: 0
<
* Connection #0 to host 127.0.0.1 left intact
* Closing connection 0
```

#### Illegal CORS Origin Preflight Request
```shell
curl -v -X OPTIONS -H "Origin: http://httpbin2.example.org" -H  "Host: httpbin.example.com" -H "Access-Control-Request-Method: GET"  -H "Access-Control-Request-Headers: Content-Type,Token" http://127.0.0.1/anything/get\?foo\=1

 < HTTP/1.1 200 OK
< x-cors-version: 1.0.0
< vary: Origin
< access-control-allow-methods: GET,POST,PATCH
< access-control-allow-headers: Content-Type,Token,Authorization
< access-control-expose-headers: X-Custom-Header,X-Env-UTM
< access-control-allow-credentials: true
< access-control-max-age: 3600
< date: Tue, 15 Apr 2025 15:26:52 GMT
< server: envoy
< content-length: 0
< 
* Connection #0 to host 127.0.0.1 left intact
```

#### Illegal CORS Method Preflight Request
```shell
curl -v -X OPTIONS -H "Origin: http://httpbin2.example.org:9090" -H  "Host: httpbin.example.com" -H "Access-Control-Request-Method: DELETE"  -H "Access-Control-Request-Headers: Content-Type,Token" http://127.0.0.1/anything/get\?foo\=1

< HTTP/1.1 200 OK
< x-cors-version: 1.0.0
< vary: Origin
< access-control-allow-origin: http://httpbin2.example.org:9090
< access-control-allow-methods: GET,POST,PATCH
< access-control-allow-headers: Content-Type,Token,Authorization
< access-control-expose-headers: X-Custom-Header,X-Env-UTM
< access-control-allow-credentials: true
< access-control-max-age: 3600
< date: Tue, 15 Apr 2025 15:27:44 GMT
< server: envoy
< content-length: 0
< 
* Connection #0 to host 127.0.0.1 left intact
```

#### Illegal CORS Header Preflight Request
```shell
curl -v -X OPTIONS -H "Origin: http://httpbin2.example.org:9090" -H  "Host: httpbin.example.com" -H "Access-Control-Request-Method: GET" -H "Access-Control-Request-Headers: TokenView"  http://127.0.0.1/anything/get\?foo\=1

< HTTP/1.1 200 OK
< x-cors-version: 1.0.0
< vary: Origin
< access-control-allow-origin: http://httpbin2.example.org:9090
< access-control-allow-methods: GET,POST,PATCH
< access-control-allow-headers: Content-Type,Token,Authorization
< access-control-expose-headers: X-Custom-Header,X-Env-UTM
< access-control-allow-credentials: true
< access-control-max-age: 3600
< date: Tue, 15 Apr 2025 15:28:34 GMT
< server: envoy
< content-length: 0
< 
* Connection #0 to host 127.0.0.1 left intact
```

## Reference Documents
- https://www.ruanyifeng.com/blog/2016/04/cors.html
- https://fetch.spec.whatwg.org/#http-cors-protocol
- https://www.cnblogs.com/sunmmi/articles/5956554.html
