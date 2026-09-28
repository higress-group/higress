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
| `match_list`                    | array of MatchRule | 否   |        | 请求匹配规则列表，支持按域名、方法、路径和请求头是否存在进行匹配 |
| `failure_mode_allow`            | bool               | 否   | false  | 当设置为 true 时，即使与授权服务的通信失败，或者授权服务返回了 HTTP 5xx 错误，仍会接受客户端请求 |
| `failure_mode_allow_header_add` | bool               | 否   | false  | 当 `failure_mode_allow` 和 `failure_mode_allow_header_add` 都设置为 true 时，若与授权服务的通信失败，或授权服务返回了 HTTP 5xx 错误，那么请求头中将会添加 `x-envoy-auth-failure-mode-allowed: true` |
| `status_on_error`               | int                | 否   | 403    | 当授权服务无法访问或状态码为 5xx 时，设置返回给客户端的 HTTP 状态码。默认状态码是 `403` |
| `cache`                         | object             | 否   | -      | 认证结果缓存配置，默认关闭。开启后被缓存命中的放行请求不再回源授权服务 |

`http_service` 中每一项的配置字段说明

| 名称                     | 数据类型 | 必填 | 默认值 | 描述                                  |
|--------------------------|----------|------|--------|---------------------------------------|
| `endpoint_mode`          | string   | 否   | envoy  | 可选 `envoy` 或 `forward_auth`        |
| `endpoint`               | object   | 是   | -      | 发送鉴权请求的 HTTP 服务信息          |
| `timeout`                | int      | 否   | 1000   | `ext-auth` 服务连接超时时间，单位毫秒 |
| `authorization_request`  | object   | 否   | -      | 发送鉴权请求配置                      |
| `authorization_response` | object   | 否   | -      | 处理鉴权响应配置                      |
| `success_condition`      | array of Condition | 否 | -      | 鉴权服务返回 HTTP 200 后，再用一组条件校验其响应内容；全部满足才放行，否则以 `status_on_error` 状态码拒绝客户端请求 |

`success_condition` 中每一项（`Condition`）的配置字段说明

| 名称     | 数据类型 | 必填 | 默认值 | 描述                                                         |
|----------|----------|------|--------|--------------------------------------------------------------|
| `source` | string   | 是   | -      | 取值来源，可选 `status_code`（鉴权响应状态码）、`header`（响应头）、`body_json`（响应体，按 gjson 路径取值） |
| `key`    | string   | `source` 为 `header` 或 `body_json` 时必填 | -      | `header` 时为响应头名称（忽略大小写）；`body_json` 时为 gjson 路径，例如 `data.code`；`status_code` 时忽略 |
| `op`     | string   | 是   | -      | 比较运算符，可选 `eq`、`ne`、`in`、`not_in`、`exists`、`not_exists`、`gt`、`lt` |
| `value`  | string 或 array of string | 除 `exists`、`not_exists` 外必填 | -      | 比较目标值。`in`、`not_in` 时为候选值数组，其余运算符取首值或直接写标量 |

`Condition` 判定语义：

- 多个条件之间为 AND，全部满足才放行；任一不满足即以 `status_on_error` 状态码拒绝。
- 仅在鉴权服务返回 HTTP 200 时生效；非 200 响应仍按原有逻辑处理。
- `header` 名称匹配忽略大小写，但 `eq`、`ne`、`in`、`not_in` 对取到的值做大小写敏感的精确比较。
- `exists` 以“能否取到值”为准：`header` 值为空字符串视为取不到；`body_json` 路径显式为 `null` 视为存在，其值为空字符串。
- `gt`、`lt` 将两侧按浮点数比较，任一侧无法解析为数字时该条件判为不满足。
- 取不到值时（来源未知、响应头缺失、gjson 路径不存在、响应体为空），除 `not_exists` 外的运算符均判为不满足。

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
| `allowed_properties`     | array of AllowedProperty | 否   | -      | 设置后将把 Envoy filter state 中的 property 映射为 HTTP header 发送给授权服务。<br>Envoy 支持的 property 列表参见下方文档：<br><ul><li>Envoy 1.27（Higress < 2.2.0）：https://www.envoyproxy.io/docs/envoy/v1.27.0/intro/arch_overview/advanced/attributes</li><li>Envoy 1.36（Higress >= 2.2.0）：https://www.envoyproxy.io/docs/envoy/v1.36.0/intro/arch_overview/advanced/attributes</li></ul> |
| `headers_to_add`         | map[string]string      | 否   | -      | 设置将包含在授权服务请求中的请求头列表。请注意，同名的客户端请求头将被覆盖 |
| `with_request_body`      | bool                   | 否   | false  | 缓冲客户端请求体，并将其发送至鉴权请求中（HTTP Method为GET、OPTIONS、HEAD请求时不生效） |
| `max_request_body_bytes` | int                    | 否   | 10MB   | 设置在内存中保存客户端请求体的最大尺寸。当客户端请求体达到在此字段中设置的数值时，将会返回HTTP 413状态码，并且不会启动授权过程。注意，这个设置会优先于 `failure_mode_allow` 的配置 |

`AllowedProperty` 类型每一项的配置字段说明

| 名称       | 数据类型 | 必填 | 默认值 | 描述                                                         |
|------------|----------|------|--------|--------------------------------------------------------------|
| `path`     | array of string | 是   | -      | 属性路径，如 `["route_name"]` 或 `["metadata", "user_id"]` |
| `header`   | string   | 是   | -      | 映射到的请求头名称                                           |

`authorization_response` 中每一项的配置字段说明

| 名称                       | 数据类型               | 必填 | 默认值 | 描述                                                         |
|----------------------------|------------------------|------|--------|--------------------------------------------------------------|
| `allowed_upstream_headers` | array of StringMatcher | 否   | -      | 匹配项的鉴权请求的响应头将添加到原始的客户端请求头中。请注意，同名的请求头将被覆盖 |
| `allowed_client_headers`   | array of StringMatcher | 否   | -      | 如果不设置，在请求被拒绝时，所有的鉴权请求的响应头将添加到客户端的响应头中。当设置后，在请求被拒绝时，匹配项的鉴权请求的响应头将添加到客户端的响应头中 |
| `mapped_upstream_headers`  | array of HeaderMapping | 否   | -      | 从鉴权响应中按来源取值，改名后透传到上游请求头。与 `allowed_upstream_headers`（同名透传）互补 |

`mapped_upstream_headers` 中每一项（`HeaderMapping`）的配置字段说明

| 名称        | 数据类型 | 必填 | 默认值 | 描述                                                         |
|-------------|----------|------|--------|--------------------------------------------------------------|
| `source`    | string   | 是   | -      | 取值来源，可选 `status_code`、`header`、`body_json`，含义同 `Condition` |
| `key`       | string   | `source` 为 `header` 或 `body_json` 时必填 | -      | `header` 时为响应头名称（忽略大小写）；`body_json` 时为 gjson 路径；`status_code` 时忽略 |
| `to_header` | string   | 是   | -      | 透传到上游请求时使用的请求头名称                             |

`HeaderMapping` 透传语义：

- 仅在请求被放行时生效（`success_condition` 校验通过后），在 `allowed_upstream_headers` 之后执行。
- 取不到值或值为空字符串时跳过，不注入该请求头。
- 以覆盖写方式设置，上游请求中同名的原有请求头会被覆盖。

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
| `match_rule_headers` | array of HeaderPresenceCondition | 否 | - | 按请求头是否存在进行匹配；数组不能为空，同一规则中的所有条件必须同时满足 |

`HeaderPresenceCondition` 类型每一项的配置字段说明：

| 名称     | 数据类型 | 必填 | 默认值 | 描述 |
|----------|----------|------|--------|------|
| `name`   | string   | 是   | -      | HTTP 请求头名称，忽略大小写；同一规则中不能配置大小写不同的重复名称 |
| `exists` | bool     | 是   | -      | `true` 表示请求头存在，`false` 表示请求头不存在；请求头值为空字符串时仍视为存在 |

`cache` 中每一项的配置字段说明

| 名称      | 数据类型 | 必填       | 默认值 | 描述                                                         |
|-----------|----------|------------|--------|--------------------------------------------------------------|
| `enabled`    | bool     | 否         | false  | 是否开启认证结果缓存。默认关闭；关闭时不创建 Redis 客户端，每个请求都直接回源授权服务 |
| `ttl`        | int      | 开启时必填 | -      | 缓存条目存活时间，单位秒，取值范围 `[1, 600]`；超过上限会被截断为 `600`；未配置或非正值时缓存不生效（按未开启处理，不报错） |
| `key_fields` | array    | 否         | -      | 自定义缓存 key 的组成字段列表。不配置时使用默认 key（方法 + 路径 + 转发给授权服务的全部请求头）；配置后缓存 key 只由方法、去掉 query 的路径与这里列出的字段组成。字段非法（不是数组、元素缺 `source`/`key`、`source` 取值不对、`key` 为空）时整个插件配置拒绝加载 |
| `redis`      | object   | 开启时必填 | -      | 缓存使用的 Redis 服务配置                                     |

`cache.key_fields` 中每一项的配置字段说明

| 名称     | 数据类型 | 必填 | 默认值 | 描述                                                         |
|----------|----------|------|--------|--------------------------------------------------------------|
| `source` | string   | 是   | -      | 字段来源，取值 `header`（请求头）或 `query`（查询参数）       |
| `key`    | string   | 是   | -      | 请求头名或查询参数名，不能为空                                |

`cache.redis` 中每一项的配置字段说明

| 名称           | 数据类型 | 必填 | 默认值 | 描述                                                         |
|----------------|----------|------|--------|--------------------------------------------------------------|
| `service_name` | string   | 是   | -      | Redis 服务名称，带服务类型的完整 FQDN 名称，例如 `redis.dns`、`redis.my-ns.svc.cluster.local` |
| `service_port` | int      | 否   | 6379   | Redis 服务端口；`service_name` 以 `.static` 结尾时默认 `80`，否则默认 `6379` |
| `timeout`      | int      | 否   | 1000   | Redis 调用超时时间，单位毫秒                                 |
| `username`     | string   | 否   | -      | Redis 用户名                                                 |
| `password`     | string   | 否   | -      | Redis 密码                                                   |
| `database`     | int      | 否   | 0      | Redis 数据库序号                                             |

`cache` 缓存语义：

- 只缓存「放行」决策：仅当授权服务返回 HTTP 200 且 `success_condition`（若配置）校验通过时，才把注入上游的请求头写入缓存；拒绝结果不缓存。
- 缓存 key 有两种模式：
  - 默认（未配置 `key_fields`）：由发往授权服务的方法、路径，以及转发给授权服务的全部请求头（含凭据）经 SHA-256 派生，任一变化都会落到不同条目，避免串号。适合凭据在一段时间内稳定复用的场景。
  - 自定义（配置了 `key_fields`）：只由方法、去掉 query 的路径，以及 `key_fields` 按声明顺序列出的字段值经 SHA-256 派生。此时 query 串不再整体进入 key，`Authorization` 与 `forward_auth` 自动附加的请求头也不会被自动纳入，只有显式列出的字段才参与。适合凭据每次请求都不同（如按请求签名的 `Authorization`）、默认模式下永远命不中缓存的场景。
  - 自定义模式下，方法与去 query 的路径始终是 key 的兜底部分；列出的字段在请求上缺失时贡献一个空值。请自行确保列出的字段足以区分不同调用方——未列入的输入不参与 key，落在同一 key 上的请求会共用同一条缓存放行结果，直到 `ttl` 到期。
- 命中缓存时直接复用上次放行所注入的请求头并放行，不再回源授权服务。
- 失败放行（fail-open）：Redis 未就绪、连接或调用出错、缓存值损坏，都会回源到真实的授权调用，缓存问题不会阻断请求。
- 当 `authorization_request.with_request_body` 为 `true` 时跳过缓存：请求体参与决策但不进入 key，缓存会带来串号风险。
- 缓存开关需同时满足 `enabled: true`、`ttl` 为正、未转发请求体；任一不满足都按未开启处理，不创建 Redis 客户端、不报错，请求走原有无缓存流程。
- TTL 上限为 `600` 秒，避免过期的放行决策长期存活。

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

支持黑白名单模式配置，默认为白名单模式，白名单为空时所有请求都需要鉴权。`whitelist` 规则匹配时跳过鉴权、不匹配时执行鉴权；`blacklist` 规则匹配时执行鉴权、不匹配时跳过鉴权。一个规则内的域名、方法、路径和请求头条件之间为 AND，`match_list` 中的规则之间为 OR。匹配域名支持 `*.bar.com` 等泛域名，路径支持 `exact`、`prefix`、`suffix`、`contains`、`regex`。`authorization_request.allowed_headers` 仅控制转发给鉴权服务的请求头，与是否调用鉴权服务无关。

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
  # 所有包含 x-custom-auth 请求头的请求需要验证
  - match_rule_headers:
      - name: 'x-custom-auth'
        exists: true
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

#### 示例3：传递路由名称到授权服务

`ext-auth` 插件的配置：

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
X-Route-Name: your-route-name
```

通过 `allowed_properties` 配置，可以将 Envoy filter state 中的 `route_name` 等属性映射为 HTTP header 发送给授权服务，便于授权服务根据路由信息进行鉴权决策。

#### 示例4：校验鉴权响应并透传字段到上游

`ext-auth` 插件的配置：

```yaml
http_service:
  endpoint_mode: envoy
  endpoint:
    service_name: ext-auth.backend.svc.cluster.local
    service_port: 8090
    path_prefix: /auth
  timeout: 1000
  # 鉴权服务返回 200 后，还要求响应体 data.code == "0" 才放行
  success_condition:
    - source: body_json
      key: data.code
      op: eq
      value: "0"
  authorization_response:
    # 从鉴权响应取字段，改名后透传给上游
    mapped_upstream_headers:
      - source: body_json
        key: data.uid
        to_header: x-auth-user-id
      - source: header
        key: x-user-token
        to_header: x-auth-token
```

当鉴权服务返回 HTTP 200、响应体为 `{"data": {"code": "0", "uid": "1001"}}`、响应头含 `x-user-token: abc` 时，请求被放行，且发往上游的请求会带上 `x-auth-user-id: 1001` 和 `x-auth-token: abc`。若 `data.code` 不为 `"0"`，即使状态码为 200，也会以 `status_on_error`（默认 403）拒绝客户端请求。

#### 示例5：开启认证结果缓存

`ext-auth` 插件的配置：

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
# 开启认证结果缓存，放行结果在 Redis 中缓存 60 秒
cache:
  enabled: true
  ttl: 60
  redis:
    service_name: redis.dns
    service_port: 6379
```

开启后，对于相同的请求方法、路径与转发给授权服务的请求头（含凭据），首次请求回源授权服务，放行后把注入上游的请求头（如 `x-user-id`）写入 Redis 并存活 `ttl` 秒；后续命中缓存的请求直接复用该放行结果，不再回源授权服务。Redis 未就绪、调用出错或缓存值损坏时一律失败放行（回源真实授权调用）。当 `authorization_request.with_request_body` 为 `true` 时不启用缓存。

如果凭据每次请求都不同（例如 `Authorization` 是按请求签名的），默认 key 会把每个请求都当成新条目、几乎命不中缓存。此时可以用 `key_fields` 指定缓存 key 只由哪些字段组成，例如只按查询参数 `userId` 和请求头 `x-app-key` 区分：

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
  # 缓存 key 只由方法、去 query 的路径、下面列出的字段组成；
  # Authorization 与未列出的 query 参数不再参与 key
  key_fields:
    - source: query
      key: userId
    - source: header
      key: x-app-key
  redis:
    service_name: redis.dns
    service_port: 6379
```

注意：`key_fields` 未列出的输入（含 `Authorization`、未列出的 query 参数）不参与 key，落在同一 key 上的请求会共用同一条缓存放行结果直到 `ttl` 到期，请确保列出的字段足以区分不同调用方。

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

#### 示例3：传递路由名称到授权服务

`ext-auth` 插件的配置：

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
X-Route-Name: your-route-name
Content-Length: 0
```

通过 `allowed_properties` 配置，可以将 Envoy filter state 中的 `route_name` 等属性映射为 HTTP header 发送给授权服务，便于授权服务根据路由信息进行鉴权决策。
