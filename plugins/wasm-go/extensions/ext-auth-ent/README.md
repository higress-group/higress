---
title: 外部认证
keywords: [higress, auth]
description: Ext 认证插件实现了调用外部授权服务进行认证鉴权的功能。
---

## 功能说明

`ext-auth` 插件实现了向外部授权服务发送鉴权请求，以检查客户端请求是否得到授权。该插件实现时参考了Envoy原生的[ext_authz filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_authz_filter)，实现了原生filter中对接HTTP服务的部分能力

## 运行属性

插件执行阶段：`认证阶段`
插件执行优先级：`360`


## 配置字段

| 名称                            | 数据类型           | 必填 | 默认值 | 描述                                                         |
| ------------------------------- | ------------------ | ---- | ------ | ------------------------------------------------------------ |
| `http_service`                  | object             | 是   | -      | 外部授权服务配置                                             |
| `match_type`                    | string             | 否   |        | 可选 `whitelist` 或 `blacklist`                              |
| `match_list`                    | array of MatchRule | 否   |        | 一个包含 (`match_rule_domain`, `match_rule_path`, `match_rule_type`) 的列表 |
| `failure_mode_allow`            | bool               | 否   | false  | 当设置为 true 时，即使与授权服务的通信失败，或者授权服务返回了 HTTP 5xx 错误，仍会接受客户端请求 |
| `failure_mode_allow_header_add` | bool               | 否   | false  | 当 `failure_mode_allow` 和 `failure_mode_allow_header_add` 都设置为 true 时，若与授权服务的通信失败，或授权服务返回了 HTTP 5xx 错误，那么请求头中将会添加 `x-envoy-auth-failure-mode-allowed: true` |
| `status_on_error`               | int                | 否   | 403    | 当授权服务无法访问或状态码为 5xx 时，设置返回给客户端的 HTTP 状态码。默认状态码是 `403` |

`http_service` 中每一项的配置字段说明

| 名称                     | 数据类型 | 必填 | 默认值 | 描述                                  |
|--------------------------|----------|------|--------|---------------------------------------|
| `endpoint_mode`          | string   | 否   | envoy  | 可选 `envoy` 或 `forward_auth`        |
| `endpoint`               | object   | 是   | -      | 发送鉴权请求的 HTTP 服务信息          |
| `timeout`                | int      | 否   | 1000   | `ext-auth` 服务连接超时时间，单位毫秒 |
| `authorization_request`  | object   | 否   | -      | 发送鉴权请求配置                      |
| `authorization_response` | object   | 否   | -      | 处理鉴权响应配置                      |

`endpoint` 中每一项的配置字段说明

| 名称             | 数据类型 | 必填                                   | 默认值 | 描述                                                         |
|------------------|----------|----------------------------------------|--------|--------------------------------------------------------------|
| `service_name`   | string   | 是                                     | -      | 输入授权服务名称，带服务类型的完整 FQDN 名称，例如 `ext-auth.dns` 、`ext-auth.my-ns.svc.cluster.local` |
| `service_port`   | int      | 否                                     | 80     | 输入授权服务的服务端口                                       |
| `service_host`   | string   | 否                                     | -      | 请求授权服务时设置的 Host 头，不填时和 FQDN 保持一致         |
| `path_prefix`    | string   | `endpoint_mode` 为 `envoy` 时必填      | -      | `endpoint_mode` 为 `envoy` 时，客户端向授权服务发送请求的请求路径前缀 |
| `request_method` | string   | 否                                     | GET    | `endpoint_mode` 为 `forward_auth` 时，客户端向授权服务发送请求的 HTTP Method |
| `path`           | string   | `endpoint_mode` 为 `forward_auth` 时必填 | -      | `endpoint_mode` 为 `forward_auth` 时，客户端向授权服务发送请求的请求路径 |

`authorization_request` 中每一项的配置字段说明

| 名称                     | 数据类型               | 必填 | 默认值 | 描述                                                         |
|--------------------------|------------------------|------|--------|--------------------------------------------------------------|
| `allowed_headers`        | array of StringMatcher | 否   | -      | 设置后，匹配项的客户端请求头将添加到授权服务请求中的请求头中。除了用户自定义的头部匹配规则外，授权服务请求中会自动包含 `Authorization` 这个HTTP头（`endpoint_mode` 为 `forward_auth` 时，会添加 `X-Forwarded-*` 的请求头） |
| `headers_to_add`         | map[string]string      | 否   | -      | 设置将包含在授权服务请求中的请求头列表。请注意，同名的客户端请求头将被覆盖 |
| `with_request_body`      | bool                   | 否   | false  | 缓冲客户端请求体，并将其发送至鉴权请求中（HTTP Method为GET、OPTIONS、HEAD请求时不生效） |
| `max_request_body_bytes` | int                    | 否   | 10MB   | 设置在内存中保存客户端请求体的最大尺寸。当客户端请求体达到在此字段中设置的数值时，将会返回HTTP 413状态码，并且不会启动授权过程。注意，这个设置会优先于 `failure_mode_allow` 的配置 |

`authorization_response` 中每一项的配置字段说明

| 名称                       | 数据类型               | 必填 | 默认值 | 描述                                                         |
|----------------------------|------------------------|------|--------|--------------------------------------------------------------|
| `allowed_upstream_headers` | array of StringMatcher | 否   | -      | 匹配项的鉴权请求的响应头将添加到原始的客户端请求头中。请注意，同名的请求头将被覆盖 |
| `allowed_client_headers`   | array of StringMatcher | 否   | -      | 如果不设置，在请求被拒绝时，所有的鉴权请求的响应头将添加到客户端的响应头中。当设置后，在请求被拒绝时，匹配项的鉴权请求的响应头将添加到客户端的响应头中 |

`StringMatcher` 类型每一项的配置字段说明，在使用 `array of StringMatcher` 时会按照数组中定义的 StringMatcher 顺序依次进行配置

| 名称       | 数据类型 | 必填                                                         | 默认值 | 描述     |
|------------|----------|-------------------------------------------------------------|--------|----------|
| `exact`    | string   | 否，`exact` , `prefix` , `suffix`, `contains`, `regex` 中选填一项 | -      | 精确匹配 |
| `prefix`   | string   | 否，`exact` , `prefix` , `suffix`, `contains`, `regex` 中选填一项 | -      | 前缀匹配 |
| `suffix`   | string   | 否，`exact` , `prefix` , `suffix`, `contains`, `regex` 中选填一项 | -      | 后缀匹配 |
| `contains` | string   | 否，`exact` , `prefix` , `suffix`, `contains`, `regex` 中选填一项 | -      | 是否包含 |
| `regex`    | string   | 否，`exact` , `prefix` , `suffix`, `contains`, `regex` 中选填一项 | -      | 正则匹配 |

MatchRule 类型每一项的配置字段说明，在使用 `array of MatchRule` 时会按照数组中定义的 MatchRule 顺序依次进行配置

| 名称                | 数据类型 | 必填 | 默认值 | 描述                                                         |
| ------------------- | -------- | ---- | ------ | ------------------------------------------------------------ |
| `match_rule_domain` | string   | 否   | -      | 匹配规则域名，支持通配符模式，例如 `*.bar.com`               |
| `match_rule_method` | []string | 否   | -      | 匹配请求方法                                                 |
| `match_rule_path`   | string   | 否   | -      | 匹配请求路径的规则                                           |
| `match_rule_type`   | string   | 否   | -      | 匹配请求路径的规则类型，可选 `exact` , `prefix` , `suffix`, `contains`, `regex` |

### 两种 `endpoint_mode` 的区别

`endpoint_mode` 为 `envoy` 时，鉴权请求会使用原始请求的 HTTP Method，和配置的 `path_prefix` 作为请求路径前缀拼接上原始的请求路径

`endpoint_mode` 为 `forward_auth` 时，鉴权请求会使用配置的 `request_method` 作为 HTTP Method，和配置的 `path` 作为请求路径，并且 Higress 会自动生成并发送以下 header 至鉴权服务：

| Header               | 说明                                                   |
| -------------------- | ------------------------------------------------------ |
| `x-forwarded-proto`  | 原始请求的scheme，比如 http/https                      |
| `x-forwarded-method` | 原始请求的方法，比如 get/post/delete/patch             |
| `x-forwarded-host`   | 原始请求的host                                         |
| `x-forwarded-uri`    | 原始请求的path，包含路径参数，比如 `/v1/app?test=true` |

### 黑白名单模式

支持黑白名单模式配置，默认为白名单模式，白名单为空，即所有请求都需要经过验证，匹配域名支持泛域名例如 `*.bar.com` ，匹配规则支持 `exact` , `prefix` , `suffix`, `contains`, `regex`

**白名单模式**

```yaml
# 白名单模式配置，符合白名单规则的请求无需验证
match_type: 'whitelist'
match_list:
  # 所有以 api.example.com 为域名，且路径前缀为 /public 的请求无需验证
  - match_rule_domain: 'api.example.com'
    match_rule_path: '/public'
    match_rule_type: 'prefix'
  # 针对图片资源服务器 images.example.com，所有 GET 请求无需验证
  - match_rule_domain: 'images.example.com'
    match_rule_method: ["GET"]
  # 所有域名下，路径精确匹配 /health-check 的 HEAD 请求无需验证
  - match_rule_method: ["HEAD"]
    match_rule_path: '/health-check'
    match_rule_type: 'exact'
```

**黑名单模式**

```yaml
# 黑名单模式配置，符合黑名单规则的请求需要验证
match_type: 'blacklist'
match_list:
  # 所有以 admin.example.com 为域名，且路径前缀为 /sensitive 的请求需要验证
  - match_rule_domain: 'admin.example.com'
    match_rule_path: '/sensitive'
    match_rule_type: 'prefix'
  # 所有域名下，路径精确匹配 /user 的 DELETE 请求需要验证
  - match_rule_method: ["DELETE"]
    match_rule_path: '/user'
    match_rule_type: 'exact'
  # 所有以 legacy.example.com 为域名的 POST 请求需要验证
  - match_rule_domain: 'legacy.example.com'
    match_rule_method: ["POST"]
```

## 配置示例

下面假设 `ext-auth` 服务在 Kubernetes 中 serviceName 为 `ext-auth`，端口 `8090`，路径为 `/auth`，命名空间为 `backend`

### endpoint_mode为envoy时

#### 示例1

`ext-auth` 插件的配置：

```yaml
http_service:
  endpoint_mode: envoy
  endpoint:
    service_name: ext-auth.backend.svc.cluster.local
    service_port: 8090
    path_prefix: /auth
  timeout: 1000
```

使用如下请求网关，当开启 `ext-auth` 插件后：

```shell
curl -X POST http://localhost:8082/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 -X GET -H "foo: bar" -H "Authorization: xxx"
```

**请求 `ext-auth` 服务成功：**

`ext-auth` 服务将接收到如下的鉴权请求：

```
POST /auth/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 HTTP/1.1
Host: ext-auth.backend.svc.cluster.local
Authorization: xxx
Content-Length: 0
```

**请求 `ext-auth` 服务失败：**

当调用 `ext-auth` 服务响应为 5xx 时，客户端将接收到HTTP响应码403和 `ext-auth` 服务返回的全量响应头

假如 `ext-auth` 服务返回了 `x-auth-version: 1.0` 和 `x-auth-failed: true` 的响应头，会传递给客户端

```
HTTP/1.1 403 Forbidden
x-auth-version: 1.0
x-auth-failed: true
date: Tue, 16 Jul 2024 00:19:41 GMT
server: istio-envoy
content-length: 0
```

当 `ext-auth` 无法访问或状态码为 5xx 时，将以 `status_on_error` 配置的状态码拒绝客户端请求

当 `ext-auth` 服务返回其他 HTTP 状态码时，将以返回的状态码拒绝客户端请求。如果配置了 `allowed_client_headers`，具有相应匹配项的响应头将添加到客户端的响应中

#### 示例2

`ext-auth` 插件的配置：

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

使用如下请求网关，当开启 `ext-auth` 插件后：

```shell
curl -X POST http://localhost:8082/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 -X GET -H "foo: bar" -H "Authorization: xxx"
```

`ext-auth` 服务将接收到如下的鉴权请求：

```
POST /auth/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 HTTP/1.1
Host: my-domain.local
Authorization: xxx
X-Auth-Version: 1.0
x-envoy-header: true
Content-Length: 0
```

`ext-auth` 服务返回响应头中如果包含 `x-user-id` 和 `x-auth-version`，网关调用upstream时的请求中会带上这两个请求头



### endpoint_mode为forward_auth时

#### 示例1

`ext-auth` 插件的配置：

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

使用如下请求网关，当开启 `ext-auth` 插件后：

```shell
curl -i http://localhost:8082/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 -X GET -H "foo: bar" -H "Authorization: xxx" -H "Host: foo.bar.com"
```

**请求 `ext-auth` 服务成功：**

`ext-auth` 服务将接收到如下的鉴权请求：

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

**请求 `ext-auth` 服务失败：**

当调用 `ext-auth` 服务响应为 5xx 时，客户端将接收到HTTP响应码403和 `ext-auth` 服务返回的全量响应头

假如 `ext-auth` 服务返回了 `x-auth-version: 1.0` 和 `x-auth-failed: true` 的响应头，会传递给客户端

```
HTTP/1.1 403 Forbidden
x-auth-version: 1.0
x-auth-failed: true
date: Tue, 16 Jul 2024 00:19:41 GMT
server: istio-envoy
content-length: 0
```

当 `ext-auth` 无法访问或状态码为 5xx 时，将以 `status_on_error` 配置的状态码拒绝客户端请求

当 `ext-auth` 服务返回其他 HTTP 状态码时，将以返回的状态码拒绝客户端请求。如果配置了 `allowed_client_headers`，具有相应匹配项的响应头将添加到客户端的响应中

#### 示例2

`ext-auth` 插件的配置：

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

使用如下请求网关，当开启 `ext-auth` 插件后：

```shell
curl -i http://localhost:8082/users?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5 -X GET -H "foo: bar" -H "Authorization: xxx" -H "X-Auth-Version: 1.0" -H "Host: foo.bar.com"
```

`ext-auth` 服务将接收到如下的鉴权请求：

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

`ext-auth` 服务返回响应头中如果包含 `x-user-id` 和 `x-auth-version`，网关调用upstream时的请求中会带上这两个请求头