---
title: JWT 认证
keywords: [higress,jwt auth]
description: JWT 认证插件配置参考
---

## 功能说明
`jwt-auth`插件实现了基于JWT(JSON Web Tokens)进行认证鉴权的功能，支持从HTTP请求的URL参数、请求头、Cookie字段解析JWT，同时验证该Token是否有权限访问。

本插件和`安全能力->认证鉴权`中JWT认证的区别是，额外提供了调用方身份识别的能力，支持对不同调用方配置不同的JWT凭证。

## 运行属性

插件执行阶段：`认证阶段`
插件执行优先级：`340`

## 配置字段

**注意：**

- 在一个规则里，鉴权配置和认证配置不可同时存在
- 对于通过认证鉴权的请求，请求的header会被添加一个`X-Mse-Consumer`字段，用以标识调用者的名称。

### 认证配置

| 名称        | 数据类型        | 填写要求                                    | 默认值 | 描述                                                        |
| ----------- | --------------- | ------------------------------------------- | ------ | ----------------------------------------------------------- |
| `global_auth` | bool            | 选填（**仅实例级别配置**）                  | -      | 只能在实例级别配置，若配置为true，则全局生效认证机制; 若配置为false，则只对做了配置的域名和路由生效认证机制，若不配置则仅当没有域名和路由配置时全局生效（兼容老用户使用习惯）。 |
| `consumers` | array of object | 必填                                        | -      | 配置服务的调用者，用于对请求进行认证                        |

`consumers`中每一项的配置字段说明如下：

| 名称                    | 数据类型          | 填写要求 | 默认值                                            | 描述                     |
| ----------------------- | ----------------- | -------- | ------------------------------------------------- | ------------------------ |
| `name`                  | string            | 必填     | -                                                 | 配置该consumer的名称     |
| `jwks`                  | string            | wasm-cpp 必填；wasm-go 未配置 `remote_jwks` 时必填 | -             | https://www.rfc-editor.org/rfc/rfc7517 指定的json格式字符串，是由验证JWT中签名的公钥（或对称密钥）组成的Json Web Key Set  |
| `remote_jwks`           | object            | wasm-go 实现选填 | -                                      | 远程 JWKS 服务引用。引用的服务需要已由 Higress 配置或发现，例如通过 McpBridge |
| `jwks_cache_duration`   | number            | wasm-go 实现选填 | 600                                   | 远程 JWKS 缓存时间，单位为秒，最大值为 604800 |
| `jwks_fetch_timeout`    | number            | wasm-go 实现选填 | 1500                                  | 远程 JWKS 拉取超时时间，单位为毫秒，最大值为 10000 |
| `issuer`                | string            | 必填     | -                                                 | JWT的签发者，需要和payload中的iss字段保持一致              |
| `claims_to_headers`     | array of object   | 选填     | -                                                 | 抽取JWT的payload中指定字段，设置到指定的请求头中转发给后端 |
| `from_headers`          | array of object   | 选填     | {"name":"Authorization","value_prefix":"Bearer "} | 从指定的请求头中抽取JWT |
| `from_params`           | array of string   | 选填     | access_token                                      | 从指定的URL参数中抽取JWT                                   |
| `from_cookies`          | array of string   | 选填     | -                                                 | 从指定的cookie中抽取JWT                                    |
| `clock_skew_seconds`    | number            | 选填     | 60                                                | 校验JWT的exp和iat字段时允许的时钟偏移量，单位为秒          |
| `keep_token`            | bool              | 选填     | true                                              | 转发给后端时是否保留JWT                                    |

**注意：** 
- 只有当`from_headers`,`from_params`,`from_cookies`均未配置时，才会使用默认值
- `remote_jwks`、`jwks_cache_duration`、`jwks_fetch_timeout` 仅适用于 wasm-go 实现；wasm-cpp 实现仍需要配置 `jwks`。
- 在 wasm-go 实现中，`jwks` 与 `remote_jwks` 只能配置其中一个。
- 使用 `remote_jwks` 时，远程 JWKS 拉取失败、返回非法 JWKS、刷新被限流或在多 key 集合中找不到匹配 `kid` 时会拒绝请求。无 `kid` 的 token 仅在拉取到的 JWKS 恰好只有一个 key 时被接受。
- 远程 JWKS 的失败重试按远程服务引用限流 30 秒；冷启动首次拉取未完成期间，以及拉取失败后的限流窗口内，请求会被本地拒绝。过期缓存仅在刷新请求已发起且尚未完成时继续使用。
- 超过 64 KiB 的远程 JWKS 响应会被拒绝。
- `jwks_cache_duration` 必须至少为 30 秒，避免缓存过期时间短于远程 JWKS 失败重试窗口。
- 在 wasm-go 实现中，空的内联 `jwks` key 集合会在配置解析阶段被拒绝。`keep_token` 为 `false` 时，从 `from_params` 提取的 token 会从转发请求路径中移除。

`remote_jwks` 的配置字段说明如下：
| 名称           | 数据类型 | 填写要求 | 默认值 | 描述 |
| -------------- | -------- | -------- | ------ | ---- |
| `service_name` | string   | 必填     | -      | 用于构造出站集群的 Higress 服务名称 |
| `service_host` | string   | 必填     | -      | JWKS 请求使用的 Host 或 `:authority` 请求头，不包含端口；端口通过 `service_port` 配置 |
| `service_port` | number   | 选填     | 443    | JWKS 请求使用的服务端口 |
| `path`         | string   | 必填     | -      | JWKS 请求路径，例如 `/.well-known/jwks.json` |

`from_headers` 中每一项的配置字段说明如下：

| 名称             | 数据类型        | 填写要求| 默认值 | 描述                                                      |
| ---------------- | --------------- | ------- | ------ | --------------------------------------------------------- |
| `name`           | string          | 必填    | -      | 抽取JWT的请求header                                       |
| `value_prefix`   | string          | 必填    | -      | 对请求header的value去除此前缀，剩余部分作为JWT            |

`claims_to_headers` 中每一项的配置字段说明如下：

| 名称             | 数据类型        | 填写要求| 默认值 | 描述                                                      |
| ---------------- | --------------- | ------- | ------ | --------------------------------------------------------- |
| `claim`          | string          | 必填    | -      | JWT payload中的指定字段，要求必须是字符串或无符号整数类型 |
| `header`         | string          | 必填    | -      | 从payload取出字段的值设置到这个请求头中，转发给后端       |
| `override`       | bool            | 选填    | true   | true时，存在同名请求头会进行覆盖；false时，追加同名请求头 |


### 鉴权配置（非必需）

| 名称        | 数据类型        | 填写要求                                    | 默认值 | 描述                                                                                                                                                           |
| ----------- | --------------- | ------------------------------------------- | ------ | -----------------------------------------------------------                                                                                                    |
| `allow`     | array of string | 选填(**非实例级别配置**)                    | -      | 只能在路由或域名等细粒度规则上配置，对于符合匹配条件的请求，配置允许访问的 consumer，从而实现细粒度的权限控制 |

## 配置示例

### 全局配置认证和路由粒度进行鉴权

注意如果一个JWT能匹配多个`jwks`，则按照配置顺序命中第一个匹配的`consumer`

在实例级别做如下插件配置：

```yaml
global_auth: false
consumers:
- name: consumer1
  issuer: abcd
  jwks: |
    {
      "keys": [
        {
          "kty": "oct",
          "kid": "123",
          "k": "hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew",
          "alg": "HS256"
        }
      ]
    }
- name: consumer2
  issuer: abc
  jwks: |
    {
      "keys": [
        {
          "kty": "RSA",
          "e": "AQAB",
          "use": "sig",
          "kid": "123",
          "alg": "RS256",
          "n": "i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw"
        }
      ]
    }
```

对 route-a 和 route-b 这两个路由做如下配置：

```yaml
allow: 
- consumer1
```

对 *.example.com 和 test.com 在这两个域名做如下配置:

```yaml
allow:
- consumer2
```

**说明：**

此例指定的route-a和route-b即在创建网关路由时填写的路由名称，当匹配到这两个路由时，将允许name为consumer1的调用者访问，其他调用者不允许访问。

此例指定的*.example.com和test.com用于匹配请求的域名，当发现域名匹配时，将允许name为consumer2的调用者访问，其他调用者不被允许访问。

根据该配置，下列请求可以允许访问：

假设以下请求会匹配到route-a这条路由

**将 JWT 设置在 url 参数中**
```bash
curl  'http://xxx.hello.com/test?access_token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxODY1NjczODE5fQ.-vBSV0bKeDwQcuS6eeSZN9dLTUnSnZVk8eVCXdooCQ4'
```
**将 JWT 设置在 http 请求头中**
```bash
curl  http://xxx.hello.com/test -H 'Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxODY1NjczODE5fQ.-vBSV0bKeDwQcuS6eeSZN9dLTUnSnZVk8eVCXdooCQ4'
```

认证鉴权通过后，请求的header中会被添加一个`X-Mse-Consumer`字段，在此例中其值为`consumer1`，用以标识调用方的名称

下列请求将拒绝访问：

**请求未提供JWT，返回401**
```bash
curl  http://xxx.hello.com/test
```

**根据请求提供的JWT匹配到的调用者无访问权限，返回403**
```bash
# consumer1不在*.example.com的allow列表里
curl  'http://xxx.example.com/test' -H 'Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxODY1NjczODE5fQ.-vBSV0bKeDwQcuS6eeSZN9dLTUnSnZVk8eVCXdooCQ4'
```

### 使用远程 JWKS（仅 wasm-go 实现）

如果签名公钥由身份提供方维护，wasm-go 实现可以通过已配置的服务引用拉取和缓存远程 JWKS。该服务需要已由 Higress 配置或发现。

```yaml
global_auth: false
consumers:
- name: remote-consumer
  issuer: https://issuer.example.com
  remote_jwks:
    service_name: issuer.example.com.dns
    service_host: issuer.example.com
    service_port: 443
    path: /.well-known/jwks.json
  jwks_cache_duration: 600
  jwks_fetch_timeout: 1500
```

配置远程 JWKS 时需要确保网关数据面能够访问引用的服务。远程 JWKS 响应体超过 64 KiB 会被视为非法响应。

#### 网关实例级别开启

以下配置将对网关实例级别开启 JWT Auth 认证，所有请求均需要经过认证后才能访问。

```yaml
global_auth: true
consumers:
- name: consumer1
  issuer: abcd
  jwks: |
    {
      "keys": [
        {
          "kty": "oct",
          "kid": "123",
          "k": "hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew",
          "alg": "HS256"
        }
      ]
    }
- name: consumer2
  issuer: abc
  jwks: |
    {
      "keys": [
        {
          "kty": "RSA",
          "e": "AQAB",
          "use": "sig",
          "kid": "123",
          "alg": "RS256",
          "n": "i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw"
        }
      ]
    }
```

## 常见错误码说明

| HTTP 状态码 | 出错信息               | 原因说明                                                                         |
| ----------- | ---------------------- | -------------------------------------------------------------------------------- |
| 401         | Jwt missing            | 请求头未提供JWT                                                                  |
| 401         | Jwt expired            | JWT已经过期                                                                      |
| 401         | Jwt verification fails | JWT payload校验失败，如iss不匹配                                                 |
| 403         | Access Denied          | 无权限访问当前路由                                                               |

## 详细说明

### 1、基于token的认证

#### 1.1 简介

很多对外开放的API需要识别请求者的身份，并据此判断所请求的资源是否可以返回给请求者。token就是一种用于身份验证的机制，基于这种机制，应用不需要在服务端保留用户的认证信息或者会话信息，可实现无状态、分布式的Web应用授权，为应用的扩展提供了便利。

#### 1.2 流程描述

![](https://help-static-aliyun-doc.aliyuncs.com/assets/img/zh-CN/2336348951/p135822.png)

上图是网关利用JWT实现认证的整个业务流程时序图，下面我们用文字来详细描述图中标注的步骤：

1. 客户端向API网关发起认证请求，请求中一般会携带终端用户的用户名和密码；

2. 网关将请求直接转发给后端服务；

3. 后端服务读取请求中的验证信息（比如用户名、密码）进行验证，验证通过后使用私钥生成标准的token，返回给网关；

4. 网关将携带token的应答返回给客户端，客户端需要将这个token缓存到本地；

5. 客户端向API网关发送业务请求，请求中携带token；

6. 网关使用用户设定的公钥对请求中的token进行验证，验证通过后，将请求透传给后端服务；

7. 后端服务进行业务处理后应答；

8. 网关将业务应答返回给客户端。

在这个整个过程中, 网关利用token认证机制，实现了用户使用自己的用户体系对自己API进行授权的能力。下面我们就要介绍网关实现token认证所使用的结构化令牌Json Web Token(JWT)。

#### 1.3 JWT

##### 1.3.1 简介

Json Web Toke（JWT），是为了在网络应用环境间传递声明而执行的一种基于JSON的开放标准RFC7519。JWT一般可以用作独立的身份验证令牌，可以包含用户标识、用户角色和权限等信息，以便于从资源服务器获取资源，也可以增加一些额外的其它业务逻辑所必须的声明信息，特别适用于分布式站点的登录场景。

##### 1.3.2 JWT的构成

`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYWRtaW4iOnRydWV9.TJVA95OrM7E2cBab30RMHrHDcEfxjoYZgeFONFh7HgQ`

如上面的例子所示，JWT就是一个字符串，由三部分构成：

- Header（头部）
- Payload（数据）
- Signature（签名）

**Header**

JWT的头部承载两个信息：

- 声明类型，这里是JWT
- 声明加密的算法

网关支持的加密算法如下：

```text
ES256, ES384, ES512,
HS256, HS384, HS512,
RS256, RS384, RS512,
PS256, PS384, PS512,
EdDSA
```

完整的头部就像下面这样的JSON：

```js
{
  'typ': 'JWT',
  'alg': 'HS256'
}
```

然后将头部进行Base64编码（该编码是可以对称解码的），构成了第一部分。

`eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9`

**Payload**

载荷就是存放有效信息的地方。定义细节如下：

```text
iss：令牌颁发者。表示该令牌由谁创建，该声明是一个字符串
sub: Subject Identifier，iss提供的终端用户的标识，在iss范围内唯一，最长为255个ASCII个字符，区分大小写
aud：Audience(s)，令牌的受众，分大小写的字符串数组
exp：Expiration time，令牌的过期时间戳。超过此时间的token会作废， 该声明是一个整数，是1970年1月1日以来的秒数
iat: 令牌的颁发时间，该声明是一个整数，是1970年1月1日以来的秒数
jti: 令牌的唯一标识，该声明的值在令牌颁发者创建的每一个令牌中都是唯一的，为了防止冲突，它通常是一个密码学随机值。这个值相当于向结构化令牌中加入了一个攻击者无法获得的随机熵组件，有利于防止令牌猜测攻击和重放攻击。
```

也可以新增用户系统需要使用的自定义字段，比如下面的例子添加了name 用户昵称：

```js
{
  "sub": "1234567890",
  "name": "John Doe"
}
```

然后将其进行Base64编码，得到JWT的第二部分：

`JTdCJTBBJTIwJTIwJTIyc3ViJTIyJTNBJTIwJTIyMTIzNDU2Nzg5MCUyMiUyQyUwQSUyMCUyMCUyMm5hbWUlMjIlM0ElMjAlMjJKb2huJTIwRG9lJTIyJTBBJTdE`

**Signature**

这个部分需要Base64编码后的Header和Base64编码后的Payload使用 . 连接组成的字符串，然后通过Header中声明的加密方式进行加密（$secret 表示用户的私钥），然后就构成了jwt的第三部分。

```js
var encodedString = base64UrlEncode(header) + '.' + base64UrlEncode(payload);
var signature = HMACSHA256(encodedString, '$secret');
```

将这三部分用 . 连接成一个完整的字符串，就构成了 1.3.2 节最开始的JWT示例。


##### 1.3.3 时效

网关会验证token中的exp字段，一旦这个字段过期了，网关会认为这个token无效而将请求直接打回。过期时间这个值必须设置。

##### 1.3.4 JWT的几个特点

1. JWT 默认是不加密，不能将秘密数据写入 JWT。
2. JWT 不仅可以用于认证，也可以用于交换信息。有效使用 JWT，可以降低服务器查询数据库的次数。
3. JWT 的最大缺点是，由于服务器不保存 session 状态，因此无法在使用过程中废止某个 token，或者更改 token 的权限。也就是说，一旦 JWT 签发了，在到期之前就会始终有效，除非服务器部署额外的逻辑。
4. JWT 本身包含了认证信息，一旦泄露，任何人都可以获得该令牌的所有权限。为了减少盗用，JWT 的有效期应该设置得比较短。对于一些比较重要的权限，使用时应该再次对用户进行认证。
5. 为了减少盗用，JWT 不应该使用 HTTP 协议明码传输，要使用HTTPS 协议传输。

### 2、用户系统如何应用JWT插件保护API

#### 2.1 生成一对JWK（JSON Web 密钥）

**方法一、在线生成：**

用户可以在这个站点https://mkjwk.org 生成用于token生成与验证的私钥与公钥， 私钥用于授权服务签发JWT，公钥配置到JWT插件中用于网关对请求验签，注意网关使用的jwks格式配置，下图中Public Key需要放到keys结构体中，如：`{"keys":[{"kty":"RSA","e":"AQAB",...}]}`


<img src="https://help-static-aliyun-doc.aliyuncs.com/assets/img/zh-CN/2336348951/p135823.png" style="zoom:50%" />

**方法二、本地生成：**

本文应用Java示例说明，其他语言用户也可以找到相关的工具生成密钥对。 新建一个Maven项目，加入如下依赖：

```xml
<dependency>
     <groupId>org.bitbucket.b_c</groupId>
     <artifactId>jose4j</artifactId>
     <version>0.7.0</version>
</dependency>
```

使用如下的代码生成一对RSA密钥：

```java
RsaJsonWebKey rsaJsonWebKey = RsaJwkGenerator.generateJwk(2048);
final String publicKeyString = rsaJsonWebKey.toJson(JsonWebKey.OutputControlLevel.PUBLIC_ONLY);
final String privateKeyString = rsaJsonWebKey.toJson(JsonWebKey.OutputControlLevel.INCLUDE_PRIVATE);
```

#### 2.2 使用JWK中的私钥实现颁发token 的认证服务

需要使用2.1节中在线生成的 Keypair JSON字符串（三个方框内的第一个）或者本地生成的 privateKeyString JSON字符串作为私钥来颁发token，用于授权可信的用户访问受保护的API，具体实现可以参考下方示例。 向客户颁发token的形式由用户根据具体的业务场景决定，可以将颁发token的功能部署到生产环境，配置成普通API后由访问者通过用户名密码获得，也可以直接在本地环境生成token 后，直接拷贝给指定用户使用。

```java
import java.security.PrivateKey; 
import org.jose4j.json.JsonUtil;
import org.jose4j.jwk.RsaJsonWebKey;
import org.jose4j.jwk.RsaJwkGenerator;
import org.jose4j.jws.AlgorithmIdentifiers;
import org.jose4j.jws.JsonWebSignature;
import org.jose4j.jwt.JwtClaims;
import org.jose4j.jwt.NumericDate;
import org.jose4j.lang.JoseException;
public class GenerateJwtDemo {
    public static void main(String[] args) throws JoseException  {
        String keyId = "uniq_key";
          //使用本文2.1节生成的Keypair
        String privateKeyJson = "{\n"
            + "  \"kty\": \"RSA\",\n"
            + "  \"d\": "
            +
            "\"O9MJSOgcjjiVMNJ4jmBAh0mRHF_TlaVva70Imghtlgwxl8BLfcf1S8ueN1PD7xV6Cnq8YenSKsfiNOhC6yZ_fjW1syn5raWfj68eR7cjHWjLOvKjwVY33GBPNOvspNhVAFzeqfWneRTBbga53Agb6jjN0SUcZdJgnelzz5JNdOGaLzhacjH6YPJKpbuzCQYPkWtoZHDqWTzCSb4mJ3n0NRTsWy7Pm8LwG_Fd3pACl7JIY38IanPQDLoighFfo-Lriv5z3IdlhwbPnx0tk9sBwQBTRdZ8JkqqYkxUiB06phwr7mAnKEpQJ6HvhZBQ1cCnYZ_nIlrX9-I7qomrlE1UoQ\",\n"
            + "  \"e\": \"AQAB\",\n"
            + "  \"alg\": \"RS256\",\n"
            + "  \"n\": \"vCuB8MgwPZfziMSytEbBoOEwxsG7XI3MaVMoocziP4SjzU4IuWuE_DodbOHQwb_thUru57_Efe"
            +
            "--sfATHEa0Odv5ny3QbByqsvjyeHk6ZE4mSAV9BsHYa6GWAgEZtnDceeeDc0y76utXK2XHhC1Pysi2KG8KAzqDa099Yh7s31AyoueoMnrYTmWfEyDsQL_OAIiwgXakkS5U8QyXmWicCwXntDzkIMh8MjfPskesyli0XQD1AmCXVV3h2Opm1Amx0ggSOOiINUR5YRD6mKo49_cN-nrJWjtwSouqDdxHYP-4c7epuTcdS6kQHiQERBd1ejdpAxV4c0t0FHF7MOy9kw\"\n"
            + "}";
        JwtClaims claims = new JwtClaims();
        claims.setGeneratedJwtId();
        claims.setIssuedAtToNow();
        //过期时间一定要设置
        NumericDate date = NumericDate.now();
        date.addSeconds(120*60);
        claims.setExpirationTime(date);
        claims.setNotBeforeMinutesInThePast(1);
        claims.setSubject("YOUR_SUBJECT");
        claims.setAudience("YOUR_AUDIENCE");
        //添加自定义参数，所有值请都使用String类型
        claims.setClaim("userId", "1213234");
        claims.setClaim("email", "userEmail@youapp.com");
        JsonWebSignature jws = new JsonWebSignature();
        jws.setAlgorithmHeaderValue(AlgorithmIdentifiers.RSA_USING_SHA256);
        jws.setKeyIdHeaderValue(keyId);
        jws.setPayload(claims.toJson());
        PrivateKey privateKey = new RsaJsonWebKey(JsonUtil.parseJson(privateKeyJson)).getPrivateKey();
     
        jws.setKey(privateKey);
        String jwtResult = jws.getCompactSerialization();
        System.out.println("Generate Json Web token , result is " + jwtResult);
    }
}
```
