# 功能说明
`simple-jwt-auth`插件基于wasm-go实现了Token解析认证功能，可以判断Token是否有效，如果Token有效则继续访问后端微服务，Token无效或不存在直接拒绝并返回401

# 配置字段
|  名称 |  数据类型 | 填写要求  | 描述  |
| ------------ | ------------ | ------------ | ------------ |
|  token_secret_key | string  | 必填  |   配置Token解析使用的SecretKey|
|  token_headers | string  | 必填  |   配置获取Token请求头名称|

# 配置示例
```yaml
token_secret_key: Dav7kfq3iA8S!JUj8&CUkdnQe72E@Cw6
token_headers: token
```
此例`token_secret_key`中指定的是认证服务生成Token的SecretKey;`token_headers`是携带Token访问的请求头名称；

# Token 格式说明
`token_headers` 指定的请求头支持以下两种写法，插件会先去掉首尾空白再做解析：

- 裸 Token：`<header>.<payload>.<signature>`
- Bearer 方案（前缀大小写不敏感）：`Bearer <header>.<payload>.<signature>`

除剥离可选的 `Bearer ` 前缀外，插件不会对请求头做任何其它改写。请求头缺失、值为空、或者值不是恰好由两个 `.` 分隔的三段式 JWT 时，插件都会返回 `401`（响应头 detail 为 `simple-jwt-auth.auth_failed`），并在网关日志中记录解析失败的原因，而不是让异常输入把插件实例打挂。

# 签名算法白名单
插件把签名算法限制为 `HS256`、`HS384`、`HS512`：声明了其它算法（例如 `none`）的 Token 会在验签之前被直接拒绝，因此 `token_secret_key` 只会被当作 HMAC 对称密钥使用，不会被 Token 头部声明的算法改写用途。

# 错误返回
|  HTTP 状态码 | 响应头 detail  | 触发场景  |
| ------------ | ------------ | ------------ |
|  401 | simple-jwt-auth.bad_config | `token_headers` 或 `token_secret_key` 未配置（响应体中的 `code` 为 400） |
|  401 | simple-jwt-auth.auth_failed | 请求头缺失、值为空、不是合法 JWT、算法不在白名单内或签名校验失败 |
