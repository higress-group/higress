---
title: OAuth2 认证
keywords: [higress,oauth2]
description: OAuth2 认证插件配置参考
---

## 功能说明
`OAuth2`插件实现了基于JWT(JSON Web Tokens)进行OAuth2 Access Token签发的能力, 遵循[RFC9068](https://datatracker.ietf.org/doc/html/rfc9068)规范

## 运行属性

插件执行阶段：`认证阶段`
插件执行优先级：`350`

## 配置字段

### 认证配置

| 名称                 | 数据类型        | 填写要求                                    | 默认值          | 描述                                                                                                                                                                          |
| -----------          | --------------- | ------------------------------------------- | ------          | -----------------------------------------------------------                                                                                                                   |
| `consumers`          | array of object | 必填                                        | -               | 配置服务的调用者，用于对请求进行认证                                                                                                                                          |
| `issuer`             | string          | 选填                                        | Higress-Gateway | 用于填充JWT中的issuer                                                                                                                                                         |
| `auth_path`          | string          | 选填                                        | /oauth2/token   | 指定路径后缀用于签发Token，路由级配置时，要确保首先能匹配对应的路由, 使用 API 管理时，需要创建相同路径的接口                                                                  |
| `global_credentials` | bool            | 选填                                        | ture            | 在通过 consumer 认证的前提下，允许任意路由签发的凭证访问                                                                                                                      |
| `auth_header_name`   | string          | 选填                                        | Authorization   | 用于指定从哪个请求头获取JWT                                                                                                                                                   |
| `token_ttl`          | number          | 选填                                        | 7200            | token从签发后多久内有效，单位为秒                                                                                                                                             |
| `clock_skew_seconds` | number          | 选填                                        | 60              | 校验JWT的exp和iat字段时允许的时钟偏移量，单位为秒                                                                                                                             |
| `keep_token`         | bool            | 选填                                        | ture            | 转发给后端时是否保留JWT                                                                                                                                                       |
| `global_auth`        | array of string | 选填(**仅实例级别配置**)                    | -               | 只能在实例级别配置，若配置为true，则全局生效认证机制; 若配置为false，则只对做了配置的域名和路由生效认证机制; 若不配置则仅当没有域名和路由配置时全局 生效（兼容老用户使用习惯） |

`consumers`中每一项的配置字段说明如下：

| 名称                    | 数据类型          | 填写要求 | 默认值                                            | 描述                     |
| ----------------------- | ----------------- | -------- | ------------------------------------------------- | ------------------------ |
| `name`                  | string            | 必填     | -                                                 | 配置该consumer的名称     |
| `client_id`             | string            | 必填     | -                                                 | OAuth2 client id         |
| `client_secret`         | string            | 必填     | -                                                 | OAuth2 client secret     |

**注意：**
- 对于开启该配置的路由，如果路径后缀和`auth_path`匹配，则该路由不会到原目标服务，而是用于生成Token
- 如果关闭`global_credentials`,请确保启用此插件的路由不是精确匹配路由，此时若存在另一条前缀匹配路由，则可能导致预期外行为
- 对于通过认证鉴权的请求，请求的header会被添加一个`X-Mse-Consumer`字段，用以标识调用者的名称。

### 鉴权配置（非必需）

| 名称        | 数据类型        | 填写要求                                    | 默认值 | 描述                                                                                                                                                           |
| ----------- | --------------- | ------------------------------------------- | ------ | -----------------------------------------------------------                                                                                                    |
| `allow`     | array of string | 选填(**非实例级别配置**)                    | -      | 只能在路由或域名等细粒度规则上配置，对于符合匹配条件的请求，配置允许访问的 consumer，从而实现细粒度的权限控制 |

**注意：**
- 在一个规则里，鉴权配置和认证配置不可同时存在

## 配置示例

### 路由粒度配置认证

在`route-a`和`route-b`两个路由做如下插件配置：

```yaml
consumers:
- name: consumer1
  client_id: 12345678-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  client_secret: abcdefgh-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

此时虽然使用同一份配置，但`route-a` 下签发的凭证无法用于访问 `route-b`，反之亦然。

如果希望同一份配置共享凭证访问权限，可以做如下配置:

```yaml
global_credentials: true
consumers:
- name: consumer1
  client_id: 12345678-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  client_secret: abcdefgh-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

### 全局配置认证，路由粒度进行鉴权

以下配置将对网关特定路由或域名开启 Jwt Auth 认证和鉴权，注意如果一个JWT能匹配多个`jwks`，则按照配置顺序命中第一个匹配的`consumer`

在实例级别做如下插件配置：

```yaml
global_auth: false
consumers:
- name: consumer1
  client_id: 12345678-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  client_secret: abcdefgh-xxxx-xxxx-xxxx-xxxxxxxxxxxx
- name: consumer2
  client_id: 87654321-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  client_secret: hgfedcba-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

在`route-a`和`route-b`两个路由做如下插件配置：

```yaml
allow:
- consumer1
```

在`*.example.com`和`test.com`两个域名做如下插件配置：

```yaml
allow:
- consumer2
```

此例指定的 `route-a` 和 `route-b` 即在创建网关路由时填写的路由名称，当匹配到这两个路由时，将允许`name`为`consumer1`的调用者访问，其他调用者不允许访问；

此例指定的 `*.example.com` 和 `test.com` 用于匹配请求的域名，当发现域名匹配时，将允许`name`为`consumer2`的调用者访问，其他调用者不允许访问。

### 网关实例级别开启

以下配置将对网关实例级别开启 OAuth2 认证，所有请求均需要经过认证后才能访问

```yaml
global_auth: true
consumers:
- name: consumer1
  client_id: 12345678-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  client_secret: abcdefgh-xxxx-xxxx-xxxx-xxxxxxxxxxxx
- name: consumer2
  client_id: 87654321-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  client_secret: hgfedcba-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

# 请求示例

## 使用 Client Credential 授权模式

### 获取 AccessToken

```bash

# 通过 GET 方法获取（推荐）

curl 'http://test.com/oauth2/token?grant_type=client_credentials&client_id=12345678-xxxx-xxxx-xxxx-xxxxxxxxxxxx&client_secret=abcdefgh-xxxx-xxxx-xxxx-xxxxxxxxxxxx'

# 通过 POST 方法获取（需要先匹配到有真实目标服务的路由，否则网关不会读取请求 Body）

curl 'http://test.com/oauth2/token' -H 'content-type: application/x-www-form-urlencoded' -d 'grant_type=client_credentials&client_id=12345678-xxxx-xxxx-xxxx-xxxxxxxxxxxx&client_secret=abcdefgh-xxxx-xxxx-xxxx-xxxxxxxxxxxx'

# 获取响应中的 access_token 字段即可:
{
  "token_type": "bearer",
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uXC9hdCtqd3QifQ.eyJhdWQiOiJkZWZhdWx0IiwiY2xpZW50X2lkIjoiMTIzNDU2NzgteHh4eC14eHh4LXh4eHgteHh4eHh4eHh4eHh4IiwiZXhwIjoxNjg3OTUxNDYzLCJpYXQiOjE2ODc5NDQyNjMsImlzcyI6IkhpZ3Jlc3MtR2F0ZXdheSIsImp0aSI6IjEwOTU5ZDFiLThkNjEtNGRlYy1iZWE3LTk0ODEwMzc1YjYzYyIsInN1YiI6ImNvbnN1bWVyMSJ9.NkT_rG3DcV9543vBQgneVqoGfIhVeOuUBwLJJ4Wycb0",
  "expires_in": 7200
}

```

### 使用 AccessToken 请求

```bash

curl 'http://test.com' -H 'Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uXC9hdCtqd3QifQ.eyJhdWQiOiJkZWZhdWx0IiwiY2xpZW50X2lkIjoiMTIzNDU2NzgteHh4eC14eHh4LXh4eHgteHh4eHh4eHh4eHh4IiwiZXhwIjoxNjg3OTUxNDYzLCJpYXQiOjE2ODc5NDQyNjMsImlzcyI6IkhpZ3Jlc3MtR2F0ZXdheSIsImp0aSI6IjEwOTU5ZDFiLThkNjEtNGRlYy1iZWE3LTk0ODEwMzc1YjYzYyIsInN1YiI6ImNvbnN1bWVyMSJ9.NkT_rG3DcV9543vBQgneVqoGfIhVeOuUBwLJJ4Wycb0'

```

# 常见错误码说明

| HTTP 状态码 | 出错信息               | 原因说明                                                                         |
| ----------- | ---------------------- | -------------------------------------------------------------------------------- |
| 401         | Invalid Jwt token      | 请求头未提供JWT, 或者JWT格式错误，或过期等原因                                   |
| 403         | Access Denied          | 无权限访问当前路由                                                               |
