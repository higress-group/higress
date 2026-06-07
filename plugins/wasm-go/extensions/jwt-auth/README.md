# 功能说明

`jwt-auth` 插件基于 JWT 对请求进行认证鉴权，支持内联 JWKS 或远程 JWKS 拉取，可从请求头、URL 参数或 Cookie 中提取 Token，校验通过后可把 Payload 中的 Claim 写入请求头转发给后端。

更完整的说明见官方文档：[JWT 认证插件](https://higress.io/zh-cn/docs/plugins/jwt-auth)。

# 运行属性

插件执行阶段：认证阶段

# 配置字段

## 全局配置

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
| -------- | -------- | -------- | -------- | -------- |
| consumers | array of object | 必填 | - | 调用方（Consumer）列表，至少配置一项 |
| global_auth | bool | 选填 | - | `true` 全局生效；`false` 仅对配置了 `_rules_` 的域名/路由生效；未配置时无规则则全局生效 |

### consumers 项

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
| -------- | -------- | -------- | -------- | -------- |
| name | string | 必填 | - | Consumer 名称 |
| issuer | string | 必填 | - | JWT 签发者，需与 payload 中 `iss` 一致 |
| jwks | string | 与 remote_jwks 二选一 | - | 内联 JWKS JSON 字符串 |
| remote_jwks | object | 与 jwks 二选一 | - | 远程 JWKS 端点（需 Higress 可发现的服务） |
| jwks_cache_duration | number | 选填 | 600 | 远程 JWKS 缓存时间（秒） |
| jwks_fetch_timeout | number | 选填 | 1500 | 远程 JWKS 拉取超时（毫秒） |
| claims_to_headers | array of object | 选填 | - | 将 Claim 写入请求头 |
| from_headers | array of object | 选填 | Authorization: Bearer | 从请求头提取 JWT |
| from_params | array of string | 选填 | access_token | 从 URL 参数提取 JWT |
| from_cookies | array of string | 选填 | - | 从 Cookie 提取 JWT |
| clock_skew_seconds | number | 选填 | 60 | 校验 `exp`/`iat` 的时钟偏移（秒） |
| keep_token | bool | 选填 | true | 转发时是否保留原 JWT |

### remote_jwks

| 名称 | 数据类型 | 填写要求 | 描述 |
| -------- | -------- | -------- | -------- |
| service_name | string | 必填 | Higress 服务名（如 `issuer.example.com.dns`） |
| service_host | string | 选填 | JWKS 请求的 Host |
| service_port | number | 选填 | 端口，默认 443 |
| path | string | 必填 | JWKS 路径，如 `/.well-known/jwks.json` |

### claims_to_headers 项

| 名称 | 数据类型 | 描述 |
| -------- | -------- | -------- |
| claim | string | JWT payload 中的字段名 |
| header | string | 写入的请求头名 |
| override | bool | 是否覆盖同名请求头，默认 true |

## 路由/域名级配置（_rules_）

| 名称 | 数据类型 | 填写要求 | 描述 |
| -------- | -------- | -------- | -------- |
| allow | array of string | 必填 | 允许访问的 consumer 名称列表 |

# 配置示例

```yaml
consumers:
  - name: example-consumer
    issuer: https://issuer.example.com
    remote_jwks:
      service_name: issuer.example.com.dns
      service_host: issuer.example.com
      service_port: 443
      path: /.well-known/jwks.json
    jwks_cache_duration: 600
    jwks_fetch_timeout: 1500
  - name: inline-consumer
    issuer: https://issuer.example.com
    jwks: '{"keys":[...]}'
```

```yaml
_rules_:
  - _match_route_:
      - route-a
    allow:
      - example-consumer
```
