---
title: 跨域资源共享
keywords: [higress,cors]
description: 跨域资源共享插件配置参考
---

## 功能说明

`cors` 插件可以为服务端启用 CORS（Cross-Origin Resource Sharing，跨域资源共享）的返回 http 响应头。

## 运行属性

插件执行阶段：`认证阶段`
插件执行优先级：`2000`

## 版本说明

### 2.0.1

相比 `2.0.0`，本版本对齐浏览器 CORS 语义，并将插件执行优先级调整为 `2000`。

具体变更和升级注意事项：

* 实际 CORS 请求的 Origin 或 Method 不匹配时，插件不再直接返回 `403`，而是继续转发到后端；插件不会添加 CORS 允许响应头，并会移除后端返回的 CORS policy 响应头，由浏览器按 CORS 规则拦截。若已有监控或客户端逻辑依赖网关直接返回 `403`，升级后需要相应调整。
* CORS 预检请求统一由插件直接返回 `204 No Content`。非法预检请求不会返回 `Access-Control-Allow-*`、`Access-Control-Expose-Headers`、`Access-Control-Allow-Credentials` 或 `Access-Control-Max-Age`，浏览器会判定预检失败；若已有逻辑依赖非法预检返回 `403`，升级后需要相应调整。
* 同源 `OPTIONS` 请求即使携带类似预检的请求头，也会继续转发到后端，避免被 CORS 插件误拦截。
* `allow_methods: ["*"]` 会在预检响应中回显本次请求的 `Access-Control-Request-Method`；`allow_headers: ["*"]` 会回显规范化后的 `Access-Control-Request-Headers`，没有请求头时不返回 `Access-Control-Allow-Headers`。
* 默认 Method/Header 解析会按逗号拆分并去除空格，避免默认值被当作单个不可匹配的条目。
* Origin 模式匹配会锚定完整 Origin，避免类似 `http://api.example.com.evil.com` 误匹配 `http://*.example.com`；如果历史配置依赖非完整匹配，需要调整为明确的 Origin 模式。
* 当 `Access-Control-Allow-Origin` 返回具体 Origin 而不是 `*` 时，插件会合并返回 `Vary: Origin`，避免缓存复用错误的跨域响应。
* `expose_headers: ["*"]` 在携带凭据的请求中仍按兼容方式接受配置，但浏览器会把 `Access-Control-Expose-Headers: *` 当作字面量 Header 名称，而不是暴露所有 Header。

## 配置字段

| 名称                  | 数据类型        | 填写要求 | 默认值                                                                                                                     | 描述                                                                                                                                                                                                                                         |
|-----------------------|-----------------|----------|----------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| allow_origins         | array of string | 选填     | *                                                                                                                          | 允许跨域访问的 Origin，格式为 scheme://host:port，示例如 http://example.com:8081。当 allow_credentials 为 false 时，可以使用 * 来表示允许所有 Origin 通过                                                                                    |
| allow_origin_patterns | array of string | 选填     | -                                                                                                                          | 允许跨域访问的 Origin 模式匹配， 用 * 匹配域名或者端口， <br/>比如 http://*.example.com -- 匹配域名， http://*.example.com:[8080,9090] -- 匹配域名和指定端口， http://*.example.com:[*] -- 匹配域名和所有端口。单独 * 表示匹配所有域名和端口 |
| allow_methods         | array of string | 选填     | GET, PUT, POST, DELETE, PATCH, OPTIONS                                                                                     | 允许跨域访问的 Method，比如：GET，POST 等。可以使用 * 来表示允许所有 Method；预检响应会回显本次请求的 `Access-Control-Request-Method`。                                                                                                      |
| allow_headers         | array of string | 选填     | DNT,X-CustomHeader,Keep-Alive,User-Agent,X-Requested-With，<br/>If-Modified-Since,Cache-Control,Content-Type,Authorization | 允许跨域访问时请求方携带哪些非 CORS 规范以外的 Header。可以使用 * 来表示允许任意 Header；预检响应会回显规范化后的 `Access-Control-Request-Headers`，没有请求头时不返回 `Access-Control-Allow-Headers`。                                      |
| expose_headers        | array of string | 选填     | -                                                                                                                          | 允许跨域访问时响应方携带哪些非 CORS 规范以外的 Header。可以使用 *；但当请求携带凭据时，浏览器会把 `Access-Control-Expose-Headers: *` 当作字面量 Header 名称，而不是暴露所有 Header。                                                          |
| allow_credentials     | bool            | 选填     | false                                                                                                                      | 是否允许跨域访问的请求方携带凭据（如 Cookie 等）。根据 CORS 规范，如果设置该选项为 true，在 allow_origins 不能使用 *， 替换成使用 allow_origin_patterns *                                                                                    |
| max_age               | number          | 选填     | 86400秒                                                                                                                    | 浏览器缓存 CORS 结果的最大时间，单位为秒。<br/>在这个时间范围内，浏览器会复用上一次的检查结果                                                                                                                                                |

> 注意
> * allow_credentials 是一个很敏感的选项，请谨慎开启。开启之后，allow_credentials 和 allow_origins 为 * 不能同时使用，同时设置时， allow_origins 值为 "*" 生效。
> * allow_origins 和 allow_origin_patterns 可以同时设置， 先检查 allow_origins 是否匹配，然后再检查 allow_origin_patterns 是否匹配
> * 对于实际 CORS 请求，如果 Origin 或 Method 不匹配配置，插件会继续转发到后端，但不会添加 CORS 响应头，并会移除后端返回的 CORS policy 响应头，由浏览器根据 CORS 规则拦截结果。
> * 对于 CORS 预检请求，合法请求由插件直接返回 `204 No Content` 和配置对应的 CORS 响应头；非法请求由插件直接返回 `204 No Content`，但不返回 `Access-Control-Allow-*`、`Access-Control-Expose-Headers`、`Access-Control-Allow-Credentials` 或 `Access-Control-Max-Age`，浏览器会判定预检失败。
> * 当 `Access-Control-Allow-Origin` 返回具体 Origin 而不是 `*` 时，插件会合并返回 `Vary: Origin`，避免缓存复用错误的跨域响应。

## 配置示例

### 允许所有跨域访问, 不允许请求方携带凭据
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

### 允许所有跨域访问,同时允许请求方携带凭据
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

### 允许特定子域,特定方法，特定请求头跨域访问，同时允许请求方携带凭据
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

## 测试

### 测试配置

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

### 请求测试

#### 简单请求
```shell
curl -v -H "Origin: http://httpbin2.example.org:9090" -H  "Host: httpbin.example.com"  http://127.0.0.1/anything/get\?foo\=1

< HTTP/1.1 200 OK
> x-cors-version: 2.0.1
> access-control-allow-origin: http://httpbin2.example.org:9090
> access-control-expose-headers: X-Custom-Header,X-Env-UTM
> access-control-allow-credentials: true
> vary: Origin
```

#### 预检请求

```shell
curl -v -X OPTIONS -H "Origin: http://httpbin2.example.org:9090" -H  "Host: httpbin.example.com" -H "Access-Control-Request-Method: POST"  -H "Access-Control-Request-Headers: Content-Type, Token" http://127.0.0.1/anything/get\?foo\=1

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

#### 非法 CORS Origin 预检请求

非法预检请求会返回 `204 No Content`，但不包含 `access-control-allow-*` 等 CORS policy 响应头，浏览器会判定预检失败。

```shell
curl -v -X OPTIONS -H "Origin: http://httpbin2.example.org" -H  "Host: httpbin.example.com" -H "Access-Control-Request-Method: GET"  http://127.0.0.1/anything/get\?foo\=1

< HTTP/1.1 204 No Content
< x-cors-trace: trace
< date: Tue, 23 May 2023 11:27:01 GMT
< server: istio-envoy
< content-length: 0
<
* Connection #0 to host 127.0.0.1 left intact
```

#### 非法 CORS Method 预检请求

```shell
curl -v -X OPTIONS -H "Origin: http://httpbin2.example.org:9090" -H  "Host: httpbin.example.com" -H "Access-Control-Request-Method: DELETE"  http://127.0.0.1/anything/get\?foo\=1

< HTTP/1.1 204 No Content
< x-cors-trace: trace
< date: Tue, 23 May 2023 11:28:51 GMT
< server: istio-envoy
< content-length: 0
<
* Connection #0 to host 127.0.0.1 left intact
```

#### 非法 CORS Header 预检请求

```shell
 curl -v -X OPTIONS -H "Origin: http://httpbin2.example.org:9090" -H  "Host: httpbin.example.com" -H "Access-Control-Request-Method: GET" -H "Access-Control-Request-Headers: TokenView"  http://127.0.0.1/anything/get\?foo\=1

< HTTP/1.1 204 No Content
< x-cors-trace: trace
< date: Tue, 23 May 2023 11:31:03 GMT
< server: istio-envoy
< content-length: 0
<
* Connection #0 to host 127.0.0.1 left intact
```

## 参考文档
- https://www.ruanyifeng.com/blog/2016/04/cors.html
- https://fetch.spec.whatwg.org/#http-cors-protocol
