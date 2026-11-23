---
title: 自定义应答
keywords: [higress,custom response]
description: 自定义应答插件配置参考
---


## 功能说明
`custom-response`插件支持配置自定义的响应，包括自定义 HTTP 应答状态码、HTTP 应答头，以及 HTTP 应答 Body。可以用于 Mock 响应，也可以用于判断特定状态码后给出自定义应答，例如在触发网关限流策略时实现自定义响应。

## 运行属性

插件执行阶段：`认证阶段`
插件执行优先级：`910`

## 配置字段

| 名称 | 数据类型 | 填写要求 |  默认值 | 描述 |
| -------- | -------- | -------- | -------- | -------- |
|  status_code    |  number     |  选填      |   200  |  自定义 HTTP 应答状态码   |
|  headers     |  array of string      |  选填     |   -  |  自定义 HTTP 应答头，key 和 value 用`=`分隔   |
|  body      |  string    |  选填     |   -   |  自定义 HTTP 应答 Body  |
|  enable_on_status   |  array of number    |   选填     |  -  | 匹配原始状态码，生成自定义响应，不填写时，不判断原始状态码   |

## 配置示例

### Mock 应答场景

```yaml
status_code: 200
headers:
- Content-Type=application/json
- Hello=World
body: "{\"hello\":\"world\"}"

```

根据该配置，请求将返回自定义应答如下：

```text
HTTP/1.1 200 OK
Content-Type: application/json
Hello: World
Content-Length: 17

{"hello":"world"}
```

### 触发限流时自定义响应

```yaml
enable_on_status: 
- 429
status_code: 302
headers:
- Location=https://example.com
```

触发网关限流时一般会返回 `429` 状态码，这时请求将返回自定义应答如下：

```text
HTTP/1.1 302 Found
Location: https://example.com
```

从而实现基于浏览器 302 重定向机制，将限流后的用户引导到其他页面，比如可以是一个 CDN 上的静态页面。

如果希望触发限流时，正常返回其他应答，参考 Mock 应答场景配置相应的字段即可。
