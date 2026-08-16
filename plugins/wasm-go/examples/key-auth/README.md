---
title: Key 认证
keywords: [higress,key auth]
description: Key 认证插件配置参考
---

## 功能说明
`key-auth`插件实现了基于 API Key 进行认证鉴权的功能，支持从 HTTP 请求的 URL 参数或者请求头解析 API Key，同时验证该 API Key 是否有权限访问。

## 运行属性

插件执行阶段：`认证阶段`
插件执行优先级：`310`

## 配置字段

**注意：**

- 在一个规则里，鉴权配置和认证配置不可同时存在
- 对于通过认证鉴权的请求，请求的header会被添加一个`X-Mse-Consumer`字段，用以标识调用者的名称。

### 认证配置
| 名称          | 数据类型        | 填写要求                                    | 默认值 | 描述                                                                                                                                                                            |
| -----------   | --------------- | ------------------------------------------- | ------ | -----------------------------------------------------------                                                                                                                     |
| `global_auth` | bool            | 选填（**仅实例级别配置**）                  | -      | 只能在实例级别配置，若配置为true，则全局生效认证机制; 若配置为false，则只对做了配置的域名和路由生效认证机制，若不配置则仅当没有域名和路由配置时全局生效（兼容老用户使用习惯）。 |
| `consumers`   | array of object | 必填                                        | -      | 配置服务的调用者，用于对请求进行认证                                                                                                                                            |
| `keys`        | array of string | 必填                                        | -      | API Key 的来源字段名称，可以是 URL 参数或者 HTTP 请求头名称                                                                                                                     |
| `in_query`    | bool            | `in_query` 和 `in_header` 至少有一个为 true | true   | 配置 true 时，网关会尝试从 URL 参数中解析 API Key                                                                                                                               |
| `in_header`   | bool            | `in_query` 和 `in_header` 至少有一个为 true | true   | 配置 true 时，网关会尝试从 HTTP 请求头中解析 API Key                                                                                                                            |

`consumers`中每一项的配置字段说明如下：

| 名称         | 数据类型 | 填写要求 | 默认值 | 描述                     |
| ------------ | -------- | -------- | ------ | ------------------------ |
| `credential` | string   | `credential` 和 `credentials` 二选一 | -      | 配置该consumer的一个访问凭证 |
| `credentials` | array of string | `credential` 和 `credentials` 二选一 | -      | 配置该consumer的多个访问凭证，不能与 `credential` 同时配置 |
| `name`       | string   | 必填     | -      | 配置该consumer的名称     |

### 鉴权配置（非必需）

| 名称        | 数据类型        | 填写要求                                    | 默认值 | 描述                                                                                                                                                           |
| ----------- | --------------- | ------------------------------------------- | ------ | -----------------------------------------------------------                                                                                                    |
| `allow`     | array of string | 选填(**非实例级别配置**)                    | -      | 只能在路由或域名等细粒度规则上配置，对于符合匹配条件的请求，配置允许访问的 consumer，从而实现细粒度的权限控制 |

## 配置示例

### 全局配置认证和路由粒度进行鉴权

以下配置将对网关特定路由或域名开启Key Auth认证和鉴权。credential 或 credentials 中的访问凭证不能重复。

在实例级别做如下插件配置：

```yaml
global_auth: false
consumers:
- credential: 2bda943c-ba2b-11ec-ba07-00163e1250b5
  name: consumer1
- credential: c8c8e9ca-558e-4a2d-bb62-e700dcc40e35
  name: consumer2
keys:
- apikey
- x-api-key
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
n
**将 API Key 设置在 url 参数中**
```bash
curl  http://xxx.hello.com/test?apikey=2bda943c-ba2b-11ec-ba07-00163e1250b5
```
**将 API Key 设置在 http 请求头中**
```bash
curl  http://xxx.hello.com/test -H 'x-api-key: 2bda943c-ba2b-11ec-ba07-00163e1250b5'
```

认证鉴权通过后，请求的header中会被添加一个`X-Mse-Consumer`字段，在此例中其值为`consumer1`，用以标识调用方的名称

下列请求将拒绝访问：

**请求未提供 API Key，返回401**
```bash
curl  http://xxx.hello.com/test
```
**请求提供的 API Key 无权访问，返回401**
```bash
curl  http://xxx.hello.com/test?apikey=926d90ac-ba2e-11ec-ab68-00163e1250b5
```

**根据请求提供的 API Key匹配到的调用者无访问权限，返回403**
```bash
# consumer2不在route-a的allow列表里
curl  http://xxx.hello.com/test?apikey=c8c8e9ca-558e-4a2d-bb62-e700dcc40e35
```

### 网关实例级别开启

以下配置将对网关实例级别开启 Basic Auth 认证，所有请求均需要经过认证后才能访问。

```yaml
global_auth: true
consumers:
- credential: 2bda943c-ba2b-11ec-ba07-00163e1250b5
  name: consumer1
- credential: c8c8e9ca-558e-4a2d-bb62-e700dcc40e35
  name: consumer2
keys:
- apikey
- x-api-key
```


## 相关错误码

| HTTP 状态码 | 出错信息                                                  | 原因说明                |
| ----------- | --------------------------------------------------------- | ----------------------- |
| 401         | Request denied by Key Auth check. Muti API key found in request | 请求提供多个 API Key      |
| 401         | Request denied by Key Auth check. No API key found in request | 请求未提供 API Key      |
| 401         | Request denied by Key Auth check. Invalid API key         | 不允许当前 API Key 访问 |
| 403         | Request denied by Key Auth check. Unauthorized consumer   | 请求的调用方无访问权限  |
