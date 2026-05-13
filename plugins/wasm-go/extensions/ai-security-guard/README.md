---
title: AI内容安全
keywords: [higress, AI, security]
description: 阿里云内容安全检测
---

## 功能说明
通过对接阿里云内容安全检测大模型的输入输出，保障AI应用内容合法合规。

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`300`

## 配置说明
| Name | Type | Requirement | Default | Description |
| ------------ | ------------ | ------------ | ------------ | ------------ |
| `serviceName` | string | requried | - | 服务名 |
| `servicePort` | string | requried | - | 服务端口 |
| `serviceHost` | string | requried | - | 阿里云内容安全endpoint的域名 |
| `accessKey` | string | requried | - | 阿里云AK |
| `secretKey` | string | requried | - | 阿里云SK |
| `action` | string | requried | - | 阿里云ai安全业务接口 |
| `securityToken` | string | optional | - | 阿里云安全令牌（用于临时凭证） |
| `checkRequest` | bool | optional | false | 检查提问内容是否合规 |
| `checkResponse` | bool | optional | false | 检查大模型的回答内容是否合规，生效时会使流式响应变为非流式 |
| `requestCheckService` | string | optional | llm_query_moderation | 指定阿里云内容安全用于检测输入内容的服务 |
| `responseCheckService` | string | optional | llm_response_moderation | 指定阿里云内容安全用于检测输出内容的服务 |
| `requestContentJsonPath` | string | optional | `messages.@reverse.0.content` | 指定要检测内容在请求body中的jsonpath |
| `responseContentJsonPath` | string | optional | `choices.0.message.content` | 指定要检测内容在响应body中的jsonpath |
| `responseStreamContentJsonPath` | string | optional | `choices.0.delta.content` | 指定要检测内容在流式响应body中的jsonpath |
| `responseContentFallbackJsonPaths` | array | optional | [`choices.0.message.content`, `content.#(type=="text")#.text`] | 当 `responseContentJsonPath` 提取为空时，按顺序尝试这些兜底路径；与主路径相同的项会自动跳过；显式配置为空数组 `[]` 可禁用兜底 |
| `responseStreamContentFallbackJsonPaths` | array | optional | [`choices.0.delta.content`, `delta.text`] | 当 `responseStreamContentJsonPath` 提取为空时，按顺序尝试这些流式兜底路径；与主路径相同的项会自动跳过；显式配置为空数组 `[]` 可禁用兜底 |
| `denyCode` | int | optional | 200 | 指定内容非法时的响应状态码 |
| `denyMessage` | string | optional | openai格式的流式/非流式响应 | 指定内容非法时的响应内容 |
| `protocol` | string | optional | openai | 协议格式，非openai协议填`original` |
| `contentModerationLevelBar` | string | optional | max | 内容合规检测拦截风险等级，取值为 `max`, `high`, `medium` or `low` |
| `promptAttackLevelBar` | string | optional | max | 提示词攻击检测拦截风险等级，取值为 `max`, `high`, `medium` or `low` |
| `sensitiveDataLevelBar` | string | optional | S4 | 敏感内容检测拦截风险等级，取值为  `S4`, `S3`, `S2` or `S1` |
| `customLabelLevelBar` | string | optional | max | 自定义检测拦截风险等级，取值为 max, high, medium, low |
| `riskAction` | string | optional | block | 风险处置动作，取值为 `block` 或 `mask`。`block` 表示按风险等级阈值拦截请求，`mask` 表示当 API 返回脱敏建议时使用脱敏内容替换敏感字段。注意：脱敏功能仅适用于 MultiModalGuard 模式 |
| `timeout` | int | optional | 2000 | 调用内容安全服务时的超时时间 |
| `bufferLimit` | int | optional | 1000 | 调用内容安全服务时每段文本的长度限制 |
| `consumerRequestCheckService` | map | optional | - | 为不同消费者指定特定的请求检测服务 |
| `consumerResponseCheckService` | map | optional | - | 为不同消费者指定特定的响应检测服务 |
| `consumerRiskLevel` | map | optional | - | 为不同消费者指定各维度的拦截风险等级 |

### 拒绝响应结构

内容被拦截时，插件（`MultiModalGuard` action）统一返回以下结构化 JSON 对象，各协议的承载位置如下：

```json
{
  "blockedDetails": [
    {
      "Type": "contentModeration",
      "Level": "high",
      "Suggestion": "block"
    }
  ],
  "requestId": "AAAAAA-BBBB-CCCC-DDDD-EEEEEEE****",
  "guardCode": 200
}
```

字段说明：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `blockedDetails` | array | 命中拦截的维度明细；若安全服务未返回明细，则根据顶层风险信号自动合成 |
| `blockedDetails[].Type` | string | 风险类型：`contentModeration` / `promptAttack` / `sensitiveData` / `maliciousUrl` / `modelHallucination` |
| `blockedDetails[].Level` | string | 风险等级：`high` / `medium` / `low` 等 |
| `blockedDetails[].Suggestion` | string | 安全服务建议操作，通常为 `block` |
| `requestId` | string | 安全服务的请求 ID，用于追踪 |
| `guardCode` | int | 安全服务返回的业务码（非 HTTP 状态码，成功检测时为 `200`） |

各协议承载位置：

- **`text_generation`（OpenAI 非流式）**：上述结构体序列化为 JSON 字符串后放入 `choices[0].message.content`
- **`text_generation`（OpenAI 流式 SSE）**：同上，放入首个 chunk 的 `delta.content`
- **`text_generation`（`protocol=original`）**：上述结构体直接作为 JSON 响应 body 返回
- **`image_generation`**：上述结构体直接作为 JSON 响应 body 返回（HTTP 403）
- **`mcp`（JSON-RPC）**：上述结构体序列化为 JSON 字符串后放入 `error.message`
- **`mcp`（SSE）**：同上，通过 SSE 事件返回

补充说明一下内容合规检测、提示词攻击检测、敏感内容检测三种风险的四个等级：

- 对于内容合规检测、提示词攻击检测：
    - `max`: 检测请求/响应内容，但是不会产生拦截行为
    - `high`: 内容安全检测/提示词攻击检测 结果中风险等级为 `high` 时产生拦截
    - `medium`: 内容安全检测/提示词攻击检测 结果中风险等级 >= `medium` 时产生拦截
    - `low`: 内容安全检测/提示词攻击检测 结果中风险等级 >= `low` 时产生拦截

- 对于敏感内容检测：
    - `S4`: 检测请求/响应内容，但是不会产生拦截行为
    - `S3`: 敏感内容检测结果中风险等级为 `S3` 时产生拦截
    - `S2`: 敏感内容检测结果中风险等级 >= `S2` 时产生拦截
    - `S1`: 敏感内容检测结果中风险等级 >= `S1` 时产生拦截

- 对于自定义检测（customLabel）：
    - `max`: 检测请求/响应内容，但是不会产生拦截行为
    - `high`: 自定义检测结果中风险等级为 `high` 时产生拦截
    - 注意：阿里云 API 对 customLabel 维度仅返回 `high` 和 `none` 两个等级，不同于其他维度的四级划分。配置为 `high` 即可在检测命中时拦截，配置为 `max` 则不拦截。`medium` 和 `low` 为配置兼容性保留，但 API 不会返回这些等级。

- 对于风险处置动作（riskAction）：
    - `block`: 按各维度的风险等级阈值判断是否拦截
    - `mask`: 当 API 返回 `Suggestion=mask` 时使用脱敏内容替换敏感字段，当 `Suggestion=block` 时仍会拦截
    - 注意：脱敏功能仅适用于 MultiModalGuard 模式（action 配置为 MultiModalGuard），其他模式不支持脱敏

## 配置示例
### 前提条件
由于插件中需要调用阿里云内容安全服务，所以需要先创建一个DNS类型的服务，例如：

![](https://img.alicdn.com/imgextra/i4/O1CN013AbDcn1slCY19inU2_!!6000000005806-0-tps-1754-1320.jpg)

阿里云内容安全配置示例：

```yaml
requestCheckService: llm_query_moderation
responseCheckService: llm_response_moderation
```

阿里云AI安全护栏配置示例：

```yaml
requestCheckService: query_security_check
responseCheckService: response_security_check
```

### 检测输入内容是否合规

```yaml
serviceName: safecheck.dns
servicePort: 443
serviceHost: "green-cip.cn-shanghai.aliyuncs.com"
accessKey: "XXXXXXXXX"
secretKey: "XXXXXXXXXXXXXXX"
checkRequest: true
```

### 检测输入与输出是否合规

```yaml
serviceName: safecheck.dns
servicePort: 443
serviceHost: green-cip.cn-shanghai.aliyuncs.com
accessKey: "XXXXXXXXX"
secretKey: "XXXXXXXXXXXXXXX"
checkRequest: true
checkResponse: true
```

### 使用临时安全凭证

```yaml
serviceName: safecheck.dns
servicePort: 443
serviceHost: "green-cip.cn-shanghai.aliyuncs.com"
accessKey: "XXXXXXXXX"
secretKey: "XXXXXXXXXXXXXXX"
securityToken: "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
checkRequest: true
```

### 为不同消费者指定不同的检测服务

```yaml
serviceName: safecheck.dns
servicePort: 443
serviceHost: "green-cip.cn-shanghai.aliyuncs.com"
accessKey: "XXXXXXXXX"
secretKey: "XXXXXXXXXXXXXXX"
checkRequest: true
consumerSpecificRequestCheckService:
  consumerA: llm_query_moderation_strict
  consumerB: llm_query_moderation_relaxed
consumerSpecificResponseCheckService:
  consumerA: llm_response_moderation_strict
  consumerB: llm_response_moderation_relaxed
```

### 指定自定义内容安全检测服务
用户可能需要根据不同的场景配置不同的检测规则，该问题可通过为不同域名/路由/服务配置不同的内容安全检测服务实现。如下图所示，我们创建了一个名为 llm_query_moderation_01 的检测服务，其中的检测规则在 llm_query_moderation 之上做了一些改动：

![](https://img.alicdn.com/imgextra/i4/O1CN01bAtcvn1N9sB16iiZR_!!6000000001528-0-tps-2728-822.jpg)

接下来在目标域名/路由/服务级别进行以下配置，指定使用我们自定义的 llm_query_moderation_01 中的规则进行检测：

```yaml
serviceName: safecheck.dns
servicePort: 443
serviceHost: "green-cip.cn-shanghai.aliyuncs.com"
accessKey: "XXXXXXXXX"
secretKey: "XXXXXXXXXXXXXXX"
checkRequest: true
requestCheckService: llm_query_moderation_01
```

### 配置非openai协议（例如百炼App）

```yaml
serviceName: safecheck.dns
servicePort: 443
serviceHost: "green-cip.cn-shanghai.aliyuncs.com"
accessKey: "XXXXXXXXX"
secretKey: "XXXXXXXXXXXXXXX"
checkRequest: true
checkResponse: true
requestContentJsonPath: "input.prompt"
responseContentJsonPath: "output.text"
denyCode: 200
denyMessage: "很抱歉，我无法回答您的问题"
protocol: original
```

### 配置响应内容兜底提取路径

当主路径提取不到内容时，可按优先级顺序配置兜底路径，兼容多种返回协议：

```yaml
serviceName: safecheck.dns
servicePort: 443
serviceHost: "green-cip.cn-shanghai.aliyuncs.com"
accessKey: "XXXXXXXXX"
secretKey: "XXXXXXXXXXXXXXX"
checkResponse: true
responseContentJsonPath: "choices.0.message.content"
responseStreamContentJsonPath: "choices.0.delta.content"
responseContentFallbackJsonPaths:
  - "output.text"
  - 'content.#(type=="text")#.text'
responseStreamContentFallbackJsonPaths:
  - "payload.delta"
  - "delta.text"
```

如需严格模式（主路径未命中即跳过，不走兜底），可显式关闭兜底：

```yaml
responseContentFallbackJsonPaths: []
responseStreamContentFallbackJsonPaths: []
```

## 可观测
### Metric
ai-security-guard 插件提供了以下监控指标：
- `ai_sec_request_deny`: 请求内容安全检测失败请求数
- `ai_sec_response_deny`: 模型回答安全检测失败请求数

### Trace
如果开启了链路追踪，ai-security-guard 插件会在请求 span 中添加以下 attributes:
- `ai_sec_risklabel`: 表示请求命中的风险类型
- `ai_sec_deny_phase`: 表示请求被检测到风险的阶段（取值为request或者response）

## 请求示例
```bash
curl http://localhost/v1/chat/completions \
-H "Content-Type: application/json" \
-d '{
  "model": "gpt-4o-mini",
  "messages": [
    {
      "role": "user",
      "content": "这是一段非法内容"
    }
  ]
}'
```

请求内容会被发送到阿里云内容安全服务进行检测，如果请求内容检测结果为非法，网关将返回形如以下的回答：

```json
{
  "id": "chatcmpl-AAy3hK1dE4ODaegbGOMoC9VY4Sizv",
  "object": "chat.completion",
  "created": 1677652288,
  "model": "gpt-4o-mini",
  "system_fingerprint": "fp_44709d6fcb",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "作为一名人工智能助手，我不能提供涉及色情、暴力、政治等敏感话题的内容。如果您有其他相关问题，欢迎您提问。",
      },
      "logprobs": null,
      "finish_reason": "stop"
    }
  ]
}
```
