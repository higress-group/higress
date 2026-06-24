# 功能说明
`OAuth2`插件实现了基于JWT(JSON Web Tokens)进行OAuth2 Access Token签发的能力, 遵循[RFC9068](https://datatracker.ietf.org/doc/html/rfc9068)规范

# 插件配置说明

## 配置字段

| 名称                 | 数据类型        | 填写要求                                    | 默认值          | 描述                                                                   |
| -----------          | --------------- | ------------------------------------------- | ------          | -----------------------------------------------------------            |
| `consumers`          | array of object | 必填                                        | -               | 配置服务的调用者，用于对请求进行认证                                   |
| `issuer`             | string          | 选填                                        | Higress-Gateway | 用于填充JWT中的issuer                                                  |
| `auth_path`          | string          | 选填                                        | /oauth2/token   | 指定路径后缀用于签发Token，路由级配置时，要确保首先能匹配对应的路由    |
| `global_credentials` | bool            | 选填                                        | ture            | 是否开启全局凭证，即允许路由A下的auth_path签发的Token可以用于访问路由B |
| `auth_header_name`   | string          | 选填                                        | Authorization   | 用于指定从哪个请求头获取JWT                                            |
| `token_ttl`          | number          | 选填                                        | 7200            | token从签发后多久内有效，单位为秒                      |
| `clock_skew_seconds` | number          | 选填                                        | 60              | 校验JWT的exp和iat字段时允许的时钟偏移量，单位为秒                      |
| `keep_token`         | bool            | 选填                                        | ture            | 转发给后端时是否保留JWT                                                |

`consumers`中每一项的配置字段说明如下：

| 名称                    | 数据类型          | 填写要求 | 默认值                                            | 描述                     |
| ----------------------- | ----------------- | -------- | ------------------------------------------------- | ------------------------ |
| `name`                  | string            | 必填     | -                                                 | 配置该consumer的名称     |
| `client_id`             | string            | 必填     | -                                                 | OAuth2 client id         |
| `client_secret`         | string            | 必填     | -                                                 | OAuth2 client secret     |


**注意：**
- 对于开启该配置的路由，如果路径后缀和`auth_path`匹配，则该路由到原目标服务，而是用于生成Token
- 如果关闭`global_credentials`,请确保启用此插件的路由不是精确匹配路由，此时若存在另一条前缀匹配路由，则可能导致预期外行为
- 对于通过认证鉴权的请求，请求的header会被添加一个`X-Mse-Consumer`字段，用以标识调用者的名称。

## 配置示例

```yaml
consumers:
  - name: consumer1
    client_id: 9515b564-0b1d-11ee-9c4c-00163e1250b5
    client_secret: 9e55de56-0b1d-11ee-b8ec-00163e1250b5
  - name: consumer2
    client_id: 8521b564-0b1d-11ee-9c4c-00163e1250b5
    client_secret: 8520b564-0b1d-11ee-9c4c-00163e1250b5
issuer: Higress-Gateway
auth_path: /oauth2/token
global_credentials: true
auth_header_name: Authorization
token_ttl: 7200
clock_skew_seconds: 3153600000
keep_token: true
```

#### 使用 Client Credential 授权模式

**获取 AccessToken**

```bash

# 通过 GET 方法获取

curl 'http://test.com/oauth2/token?grant_type=client_credentials&client_id=12345678-xxxx-xxxx-xxxx-xxxxxxxxxxxx&client_secret=abcdefgh-xxxx-xxxx-xxxx-xxxxxxxxxxxx'

# 通过 POST 方法获取 (需要先匹配到有真实目标服务的路由)

curl 'http://test.com/oauth2/token' -H 'content-type: application/x-www-form-urlencoded' -d 'grant_type=client_credentials&client_id=12345678-xxxx-xxxx-xxxx-xxxxxxxxxxxx&client_secret=abcdefgh-xxxx-xxxx-xxxx-xxxxxxxxxxxx'

# 获取响应中的 access_token 字段即可:
{
  "token_type": "bearer",
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uXC9hdCtqd3QifQ.eyJhdWQiOiJkZWZhdWx0IiwiY2xpZW50X2lkIjoiMTIzNDU2NzgteHh4eC14eHh4LXh4eHgteHh4eHh4eHh4eHh4IiwiZXhwIjoxNjg3OTUxNDYzLCJpYXQiOjE2ODc5NDQyNjMsImlzcyI6IkhpZ3Jlc3MtR2F0ZXdheSIsImp0aSI6IjEwOTU5ZDFiLThkNjEtNGRlYy1iZWE3LTk0ODEwMzc1YjYzYyIsInN1YiI6ImNvbnN1bWVyMSJ9.NkT_rG3DcV9543vBQgneVqoGfIhVeOuUBwLJJ4Wycb0",
  "expires_in": 7200
}

```

**使用 AccessToken 请求**

```bash

curl 'http://test.com' -H 'Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uXC9hdCtqd3QifQ.eyJhdWQiOiJkZWZhdWx0IiwiY2xpZW50X2lkIjoiMTIzNDU2NzgteHh4eC14eHh4LXh4eHgteHh4eHh4eHh4eHh4IiwiZXhwIjoxNjg3OTUxNDYzLCJpYXQiOjE2ODc5NDQyNjMsImlzcyI6IkhpZ3Jlc3MtR2F0ZXdheSIsImp0aSI6IjEwOTU5ZDFiLThkNjEtNGRlYy1iZWE3LTk0ODEwMzc1YjYzYyIsInN1YiI6ImNvbnN1bWVyMSJ9.NkT_rG3DcV9543vBQgneVqoGfIhVeOuUBwLJJ4Wycb0'

```
因为 test.com 仅授权了 consumer2，但这个 Access Token 是基于 consumer1 的 `client_id`，`client_secret` 获取的，因此将返回 `403 Access Denied`


# 常见错误码说明

| HTTP 状态码 | 出错信息               | 原因说明                                                                         |
| ----------- | ---------------------- | -------------------------------------------------------------------------------- |
| 401         | Invalid Jwt token      | 请求头未提供JWT, 或者JWT格式错误，或过期等原因                                   |
| 403         | Access Denied          | 无权限访问当前路由                                                               |

