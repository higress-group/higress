---
title: Basic 认证
keywords: [higress,basic auth]
description: Basic 认证插件配置参考
---

## 功能说明
`basic-auth`插件实现了基于 HTTP Basic Auth 标准进行认证鉴权的功能

## 运行属性

插件执行阶段：`认证阶段`
插件执行优先级：`320`

## 配置字段

**注意：**

- 在一个规则里，鉴权配置和认证配置不可同时存在
- 对于通过认证鉴权的请求，请求的header会被添加一个`X-Mse-Consumer`字段，用以标识调用者的名称。

### 认证配置

| 名称          | 数据类型        | 填写要求                   | 默认值 | 描述                                                                                                                                                                            |
| -----------   | --------------- | --------                   | ------ | ----------------------------------------------------                                                                                                                            |
| `global_auth` | bool            | 选填（**仅实例级别配置**） | -      | 只能在实例级别配置，若配置为true，则全局生效认证机制; 若配置为false，则只对做了配置的域名和路由生效认证机制，若不配置则仅当没有域名和路由配置时全局生效（兼容老用户使用习惯）。 |
| `consumers`   | array of object | 必填                       | -      | 配置服务的调用者，用于对请求进行认证                                                                                                                                            |

`consumers`中每一项的配置字段说明如下：

| 名称         | 数据类型 | 填写要求 | 默认值 | 描述                     |
| ------------ | -------- | -------- | ------ | ------------------------ |
| `credential` | string   | 必填     | -      | 配置该consumer的访问凭证 |
| `name`       | string   | 必填     | -      | 配置该consumer的名称     |

### 鉴权配置（非必需）

| 名称             | 数据类型        | 填写要求                                          | 默认值 | 描述                                               |
| ---------------- | --------------- | ------------------------------------------------- | ------ | -------------------------------------------------- |
| `allow`          | array of string | 必填                                              | -      | 对于符合匹配条件的请求，配置允许访问的consumer名称 |

## 配置示例

### 全局配置认证和路由粒度进行鉴权

以下配置将对网关特定路由或域名开启 Basic Auth 认证和鉴权，注意凭证信息中的用户名和密码之间使用":"分隔，`credential`字段不能重复


在实例级别做如下插件配置：

```yaml
consumers:
- credential: 'admin:123456'
  name: consumer1
- credential: 'guest:abc'
  name: consumer2
global_auth: false
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

若是在控制台进行配置，此例指定的 `route-a` 和 `route-b` 即在控制台创建路由时填写的路由名称，当匹配到这两个路由时，将允许`name`为`consumer1`的调用者访问，其他调用者不允许访问；

此例指定的 `*.example.com` 和 `test.com` 用于匹配请求的域名，当发现域名匹配时，将允许`name`为`consumer2`的调用者访问，其他调用者不允许访问。

根据该配置，下列请求可以允许访问：

**请求指定用户名密码**

```bash
# 假设以下请求将会匹配到route-a路由
# 使用 curl 的 -u 参数指定
curl -u admin:123456  http://xxx.hello.com/test
# 或者直接指定 Authorization 请求头，用户名密码使用 base64 编码
curl -H 'Authorization: Basic YWRtaW46MTIzNDU2'  http://xxx.hello.com/test
```

认证鉴权通过后，请求的header中会被添加一个`X-Mse-Consumer`字段，在此例中其值为`consumer1`，用以标识调用方的名称

下列请求将拒绝访问：

**请求未提供用户名密码，返回401**
```bash
curl  http://xxx.hello.com/test
```
**请求提供的用户名密码错误，返回401**
```bash
curl -u admin:abc  http://xxx.hello.com/test
```
**根据请求的用户名和密码匹配到的调用者无访问权限，返回403**
```bash
# consumer2不在route-a的allow列表里
curl -u guest:abc  http://xxx.hello.com/test
```

## 相关错误码

| HTTP 状态码 | 出错信息                                                                           | 原因说明               |
| ----------- |--------------------------------------------------------------------------------| ---------------------- |
| 401         | Request denied by Basic Auth check. No Basic Authentication information found. | 请求未提供凭证         |
| 401         | Request denied by Basic Auth check. Invalid username and/or password.          | 请求凭证无效           |
| 403         | Request denied by Basic Auth check. Unauthorized consumer.                     | 请求的调用方无访问权限 |
