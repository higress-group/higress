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