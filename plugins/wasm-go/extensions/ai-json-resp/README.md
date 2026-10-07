---
title: AI JSON 格式化
keywords: [ AI网关, AI JSON 格式化 ]
description: AI JSON 格式化插件配置参考
---

## 功能说明

LLM响应结构化插件，用于根据默认或用户配置的Json Schema对AI的响应进行结构化，以便后续插件处理。注意目前只支持 `非流式响应`。

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`150`

### 配置说明

| Name | Type | Requirement | Default | **Description** |
| --- | --- | --- | --- | --- |
| serviceName | str |  required | - | AI服务或支持AI-Proxy的网关服务名称 |
| serviceDomain | str |  optional | - | AI服务或支持AI-Proxy的网关服务域名/IP地址 |
| servicePath | str |  optional | '/v1/chat/completions' | AI服务或支持AI-Proxy的网关服务基础路径 |
| serviceUrl | str |  optional | - | AI服务或支持 AI-Proxy 的网关服务URL, 插件将自动提取Domain 和 Path, 用于填充未配置的 serviceDomain 或 servicePath |
| servicePort | int |  optional | 443 | 网关服务端口 |
| serviceTimeout | int |  optional | 50000 | 默认请求超时时间 |
| maxRetry | int |  optional | 3 | 若回答无法正确提取格式化时重试次数 |
| contentPath | str |  optional | "choices.0.message.content” | 从LLM回答中提取响应结果的gpath路径 |
| jsonSchema | str (json) |  optional | - | 验证请求所参照的 jsonSchema, 为空只验证并返回合法Json格式响应 |
| enableSwagger | bool |  optional | false | 是否启用 Swagger 协议进行验证 |
| enableOas3 | bool |  optional | true | 是否启用 Oas3 协议进行验证 |
| enableContentDisposition | bool | optional | true | 是否启用 Content-Disposition 头部, 若启用则会在响应头中添加 `Content-Disposition: attachment; filename="response.json"` |

> 出于性能考虑，默认支持的最大 Json Schema 深度为 6。超过此深度的 Json Schema 将不用于验证响应，插件只会检查返回的响应是否为合法的 Json 格式。


### 请求和返回参数说明

- **请求参数**: 本插件请求格式为openai请求格式，包含`model`和`messages`字段，其中`model`为AI模型名称，`messages`为对话消息列表，每个消息包含`role`和`content`字段，`role`为消息角色，`content`为消息内容。
  ```json
  {
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "give me a api doc for add the variable x to x+5"}
    ]
  }
  ```
  其他请求参数需参考配置的ai服务或网关服务的相应文档。
- **返回参数**: 
  - 返回满足定义的Json Schema约束的 `Json格式响应`
  - 若未定义Json Schema，则返回合法的`Json格式响应`
  - 若出现内部错误，则返回 `{ "Code": 10XX, "Msg": "错误信息提示" }`。

## 请求示例

```bash
curl -X POST "http://localhost:8001/v1/chat/completions" \
-H "Content-Type: application/json" \
-d '{
  "model": "gpt-4",
  "messages": [
    {"role": "user", "content": "give me a api doc for add the variable x to x+5"}
  ]
}'

```

## 返回示例
### 正常返回
在正常情况下，系统应返回经过 JSON Schema 验证的 JSON 数据。如果未配置 JSON Schema，系统将返回符合 JSON 标准的合法 JSON 数据。
```json
{
  "apiVersion": "1.0",
  "request": {
    "endpoint": "/add_to_five",
    "method": "POST",
    "port": 8080,
    "headers": {
      "Content-Type": "application/json"
    },
    "body": {
      "x": 7
    }
  }
}
```

### 异常返回
在发生错误时，返回状态码为 `500`，返回内容为 JSON 格式的错误信息。包含错误码 `Code` 和错误信息 `Msg` 两个字段。
```json
{
  "Code": 1006,
  "Msg": "retry count exceed max retry count"
}
```

### 错误码说明
| 错误码 | 说明 |
| --- | --- |
| 1001 | 配置的Json Schema不是合法Json格式|
| 1002 | 配置的Json Schema编译失败，不是合法的Json Schema 格式或深度超出 jsonSchemaMaxDepth 且 rejectOnDepthExceeded 为true|
| 1003 | 无法在响应中提取合法的Json|
| 1004 | 响应为空字符串|
| 1005 | 响应不符合Json Schema定义|
| 1006 | 重试次数超过最大限制|
| 1007 | 无法获取响应内容，可能是上游服务配置错误或获取内容的ContentPath路径错误|
| 1008 | serviceDomain 为空，请注意 serviceDomain 或 serviceUrl 不能同时为空。|

## 服务配置说明
本插件需要配置上游服务来支持出现异常时的自动重试机制, 支持的配置主要包括`支持openai接口的AI服务`或`本地网关服务`

### 支持openai接口的AI服务
以qwen为例，基本配置如下：

```yaml
serviceName: qwen
serviceDomain: dashscope.aliyuncs.com
apiKey: [Your API Key]
servicePath: /compatible-mode/v1/chat/completions
jsonSchema:
  title: ReasoningSchema
  type: object
  properties:
    reasoning_steps:
      type: array
      items:
        type: string
      description: The reasoning steps leading to the final conclusion.
    answer:
      type: string
      description: The final answer, taking into account the reasoning steps.
  required:
    - reasoning_steps
    - answer
  additionalProperties: false
```

### 本地网关服务
为了能复用已经配置好的服务，本插件也支持配置本地网关服务。例如，若网关已经配置好了AI-proxy服务，则可以直接配置如下：
1. 创建一个固定IP地址为127.0.0.1:80的服务，例如localservice.static

2. 配置文件中添加localservice.static的服务配置
```yaml
serviceName: localservice
serviceDomain: 127.0.0.1
servicePort: 80
```
3. 自动提取请求的Path，Header等信息
插件会自动提取请求的Path，Header等信息，从而避免对AI服务的重复配置。
