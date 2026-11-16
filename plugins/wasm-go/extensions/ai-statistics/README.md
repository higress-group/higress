---
title: AI可观测
keywords: [higress, AI, observability]
description: AI可观测配置参考
---

## 介绍

提供 AI 可观测基础能力，包括 metric, log, trace，其后需接 ai-proxy 插件，如果不接 ai-proxy 插件的话，则需要用户进行相应配置才可生效。

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`200`

## 配置说明

插件默认请求符合 openai 协议格式，并提供了以下基础可观测值，用户无需特殊配置：

- metric：提供了输入 token、输出 token、首个 token 的 rt（流式请求）、请求总 rt 等指标，支持在网关、路由、服务、模型四个维度上进行观测
- log：提供了 input_token, output_token, model, llm_service_duration, llm_first_token_duration 等字段

用户还可以通过配置的方式对可观测的值进行扩展：

| 名称             | 数据类型  | 填写要求 | 默认值 | 描述                     |
|----------------|-------|------|-----|------------------------|
| `use_default_attributes` | bool | 非必填  | false   | 是否使用默认完整属性配置，包含 messages、answer、question 等所有字段。适用于调试、审计场景 |
| `use_default_response_attributes` | bool | 非必填  | false   | 是否使用轻量级默认属性配置（推荐），包含 model 和 token 统计，不缓冲流式响应体。适用于高并发生产环境 |
| `attributes` | []Attribute | 非必填  | -   | 用户希望记录在log/span中的信息 |
| `disable_openai_usage` | bool | 非必填  | false   | 非openai兼容协议时，model、token的支持非标，配置为true时可以避免报错 |
| `value_length_limit` | int | 非必填  | 4000   | 记录的单个value的长度限制 |
| `enable_path_suffixes` | []string    | 非必填   | []     | 只对这些特定路径后缀的请求生效，可以配置为 "\*" 以匹配所有路径（通配符检查会优先进行以提高性能）。如果为空数组，则对所有路径生效 |
| `enable_content_types` | []string    | 非必填   | []     | 只对这些内容类型的响应进行缓冲处理。如果为空数组，则对所有内容类型生效                                                           |
| `session_id_header` | string | 非必填  | -   | 指定读取 session ID 的 header 名称。如果不配置，将按以下优先级自动查找：`x-openclaw-session-key`、`x-clawdbot-session-key`、`x-moltbot-session-key`、`x-agent-session`。session ID 可用于追踪多轮 Agent 对话 |

Attribute 配置说明:

| 名称                    | 数据类型 | 填写要求 | 默认值 | 描述                                                                                                                                        |
| ----------------------- | -------- | -------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `key`                   | string   | 必填     | -      | attribute 名称                                                                                                                              |
| `value_source`          | string   | 必填     | -      | attribute 取值来源，可选值为 `fixed_value`, `request_header`, `request_body`, `response_header`, `response_body`, `response_streaming_body` |
| `value`                 | string   | 必填     | -      | attribute 取值 key value/path                                                                                                               |
| `default_value`         | string   | 非必填   | -      | attribute 默认值                                                                                                                            |
| `rule`                  | string   | 非必填   | -      | 从流式响应中提取 attribute 的规则，可选值为 `first`, `replace`, `append`                                                                    |
| `apply_to_log`          | bool     | 非必填   | false  | 是否将提取的信息记录在日志中                                                                                                                |
| `apply_to_span`         | bool     | 非必填   | false  | 是否将提取的信息记录在链路追踪 span 中                                                                                                      |
| `trace_span_key`        | string   | 非必填   | -      | 链路追踪 attribute key，默认会使用`key`的设置                                                                                               |
| `as_separate_log_field` | bool     | 非必填   | false  | 记录日志时是否作为单独的字段，日志字段名使用`key`的设置                                                                                     |

`value_source` 的各种取值含义如下：

- `fixed_value`：固定值
- `request_header` ： attribute 值通过 http 请求头获取，value 配置为 header key
- `request_body` ：attribute 值通过请求 body 获取，value 配置格式为 gjson 的 jsonpath
- `response_header` ：attribute 值通过 http 响应头获取，value 配置为 header key
- `response_body` ：attribute 值通过响应 body 获取，value 配置格式为 gjson 的 jsonpath
- `response_streaming_body` ：attribute 值通过流式响应 body 获取，value 配置格式为 gjson 的 jsonpath

当 `value_source` 为 `response_streaming_body` 时，应当配置 `rule`，用于指定如何从流式 body 中获取指定值，取值含义如下：

- `first`：多个 chunk 中取第一个有效 chunk 的值
- `replace`：多个 chunk 中取最后一个有效 chunk 的值
- `append`：拼接多个有效 chunk 中的值，可用于获取回答内容

### 内置属性 (Built-in Attributes)

插件提供了一些内置属性键（key），可以直接使用而无需配置 `value_source` 和 `value`。这些内置属性会自动从请求/响应中提取相应的值：

| 内置属性键 | 说明 | 适用场景 |
|---------|------|---------|
| `question` | 用户提问内容 | 支持 OpenAI/Claude 消息格式 |
| `system` | 系统提示词 | 支持 Claude `/v1/messages` 的顶层 system 字段 |
| `answer` | AI 回答内容 | 支持 OpenAI/Claude 消息格式，流式和非流式 |
| `tool_calls` | 工具调用信息 | OpenAI/Claude 工具调用 |
| `reasoning` | 推理过程 | OpenAI o1 等推理模型 |
| `reasoning_tokens` | 推理 token 数（如 o1 模型） | OpenAI Chat Completions，从 `output_token_details.reasoning_tokens` 提取 |
| `cached_tokens` | 缓存命中的 token 数 | OpenAI Chat Completions，从 `input_token_details.cached_tokens` 提取 |
| `input_token_details` | 输入 token 详细信息（完整对象） | OpenAI/Gemini/Anthropic，包含缓存、工具使用等详情 |
| `output_token_details` | 输出 token 详细信息（完整对象） | OpenAI/Gemini/Anthropic，包含推理 token、生成图片数等详情 |

使用内置属性时，只需设置 `key`、`apply_to_log` 等参数，无需设置 `value_source` 和 `value`。

**注意**：
- `reasoning_tokens` 和 `cached_tokens` 是从 token details 中提取的便捷字段，适用于 OpenAI Chat Completions API
- `input_token_details` 和 `output_token_details` 会以 JSON 字符串形式记录完整的 token 详情对象

## 配置示例

如果希望在网关访问日志中记录 ai-statistic 相关的统计值，需要修改 log_format，在原 log_format 基础上添加一个新字段，示例如下：

```yaml
'{"ai_log":"%FILTER_STATE(wasm.ai_log:PLAIN)%"}'
```

如果字段设置了 `as_separate_log_field`，例如：

```yaml
attributes:
  - key: consumer
    value_source: request_header
    value: x-mse-consumer
    apply_to_log: true
    as_separate_log_field: true
```

那么要在日志中打印，需要额外设置 log_format：

```
'{"consumer":"%FILTER_STATE(wasm.consumer:PLAIN)%"}'
```

### 空配置

#### 监控

```
# counter 类型，输入 token 数量的累加值
route_upstream_model_consumer_metric_input_token{ai_route="ai-route-aliyun.internal",ai_cluster="outbound|443||llm-aliyun.internal.dns",ai_model="qwen-turbo",ai_consumer="none"} 24

# counter 类型，输出 token 数量的累加值
route_upstream_model_consumer_metric_output_token{ai_route="ai-route-aliyun.internal",ai_cluster="outbound|443||llm-aliyun.internal.dns",ai_model="qwen-turbo",ai_consumer="none"} 507

# counter 类型，流式请求和非流式请求消耗总时间的累加值
route_upstream_model_consumer_metric_llm_service_duration{ai_route="ai-route-aliyun.internal",ai_cluster="outbound|443||llm-aliyun.internal.dns",ai_model="qwen-turbo",ai_consumer="none"} 6470

# counter 类型，流式请求和非流式请求次数的累加值
route_upstream_model_consumer_metric_llm_duration_count{ai_route="ai-route-aliyun.internal",ai_cluster="outbound|443||llm-aliyun.internal.dns",ai_model="qwen-turbo",ai_consumer="none"} 2

# counter 类型，流式请求首个 token 延时的累加值
route_upstream_model_consumer_metric_llm_first_token_duration{ai_route="ai-route-aliyun.internal",ai_cluster="outbound|443||llm-aliyun.internal.dns",ai_model="qwen-turbo",ai_consumer="none"} 340

# counter 类型，流式请求次数的累加值
route_upstream_model_consumer_metric_llm_stream_duration_count{ai_route="ai-route-aliyun.internal",ai_cluster="outbound|443||llm-aliyun.internal.dns",ai_model="qwen-turbo",ai_consumer="none"} 1
```

以下是使用指标的几个示例：

流式请求首个 token 的平均延时：

```
irate(route_upstream_model_consumer_metric_llm_first_token_duration[2m])
/
irate(route_upstream_model_consumer_metric_llm_stream_duration_count[2m])
```

流式请求和非流式请求平均消耗的总时长：

```
irate(route_upstream_model_consumer_metric_llm_service_duration[2m])
/
irate(route_upstream_model_consumer_metric_llm_duration_count[2m])
```

#### 日志

```json
{
  "ai_log": "{\"model\":\"qwen-turbo\",\"input_token\":\"10\",\"output_token\":\"69\",\"llm_first_token_duration\":\"309\",\"llm_service_duration\":\"1955\"}"
}
```

如果请求中携带了 session ID header，日志中会自动添加 `session_id` 字段：

```json
{
  "ai_log": "{\"session_id\":\"sess_abc123\",\"model\":\"qwen-turbo\",\"input_token\":\"10\",\"output_token\":\"69\",\"llm_first_token_duration\":\"309\",\"llm_service_duration\":\"1955\"}"
}
```

#### 链路追踪

配置为空时，不会在 span 中添加额外的 attribute

### 从非 openai 协议提取 token 使用信息

在 ai-proxy 中设置协议为 original 时，以百炼为例，可作如下配置指定如何提取 model, input_token, output_token

```yaml
attributes:
  - key: model
    value_source: response_body
    value: usage.models.0.model_id
    apply_to_log: true
    apply_to_span: false
  - key: input_token
    value_source: response_body
    value: usage.models.0.input_tokens
    apply_to_log: true
    apply_to_span: false
  - key: output_token
    value_source: response_body
    value: usage.models.0.output_tokens
    apply_to_log: true
    apply_to_span: false
```

#### 监控

```
route_upstream_model_consumer_metric_input_token{ai_route="bailian",ai_cluster="qwen",ai_model="qwen-max"} 343
route_upstream_model_consumer_metric_output_token{ai_route="bailian",ai_cluster="qwen",ai_model="qwen-max"} 153
route_upstream_model_consumer_metric_llm_service_duration{ai_route="bailian",ai_cluster="qwen",ai_model="qwen-max"} 3725
route_upstream_model_consumer_metric_llm_duration_count{ai_route="bailian",ai_cluster="qwen",ai_model="qwen-max"} 1
```

#### 日志

此配置下日志效果如下：

```json
{
  "ai_log": "{\"model\":\"qwen-max\",\"input_token\":\"343\",\"output_token\":\"153\",\"llm_service_duration\":\"19110\"}"
}
```

#### 链路追踪

链路追踪的 span 中可以看到 model, input_token, output_token 三个额外的 attribute

### 配合认证鉴权记录 consumer

举例如下：

```yaml
attributes:
  - key: consumer # 配合认证鉴权记录consumer
    value_source: request_header
    value: x-mse-consumer
    apply_to_log: true
```

### 记录问题与回答

#### 仅记录当前轮次的问题与回答

```yaml
attributes:
  - key: question # 记录当前轮次的问题（最后一条用户消息）
    value_source: request_body
    value: messages.@reverse.0.content
    apply_to_log: true
  - key: answer # 在流式响应中提取大模型的回答
    value_source: response_streaming_body
    value: choices.0.delta.content
    rule: append
    apply_to_log: true
  - key: answer # 在非流式响应中提取大模型的回答
    value_source: response_body
    value: choices.0.message.content
    apply_to_log: true
```

#### 记录完整的多轮对话历史（推荐配置）

对于多轮 Agent 对话场景，使用内置属性可以大幅简化配置：

```yaml
session_id_header: "x-session-id"  # 可选，指定 session ID header
attributes:
  - key: messages     # 完整对话历史
    value_source: request_body
    value: messages
    apply_to_log: true
  - key: question     # 内置属性，自动提取最后一条用户消息
    apply_to_log: true
  - key: answer       # 内置属性，自动提取回答
    apply_to_log: true
  - key: reasoning    # 内置属性，自动提取思考过程
    apply_to_log: true
  - key: tool_calls   # 内置属性，自动提取工具调用
    apply_to_log: true
```

**内置属性说明：**

插件提供以下内置属性 key，无需配置 `value_source` 和 `value` 字段即可自动提取：

| 内置 Key | 说明 | 默认 value_source |
|---------|------|-------------------|
| `question` | 自动提取最后一条用户消息 | `request_body` |
| `answer` | 自动提取回答内容（支持 OpenAI/Claude 协议） | `response_streaming_body` / `response_body` |
| `tool_calls` | 自动提取并拼接工具调用（流式场景自动按 index 拼接 arguments） | `response_streaming_body` / `response_body` |
| `reasoning` | 自动提取思考过程（reasoning_content，如 DeepSeek-R1） | `response_streaming_body` / `response_body` |

> **注意**：如果配置了 `value_source` 和 `value`，将优先使用配置的值，以保持向后兼容。

日志输出示例：

```json
{
  "ai_log": "{\"session_id\":\"sess_abc123\",\"messages\":[{\"role\":\"user\",\"content\":\"北京天气怎么样？\"}],\"question\":\"北京天气怎么样？\",\"reasoning\":\"用户想知道北京的天气，我需要调用天气查询工具。\",\"tool_calls\":[{\"index\":0,\"id\":\"call_abc123\",\"type\":\"function\",\"function\":{\"name\":\"get_weather\",\"arguments\":\"{\\\"location\\\":\\\"Beijing\\\"}\"}}],\"model\":\"deepseek-reasoner\"}"
}
```

**流式响应中的 tool_calls 处理：**

插件会自动按 `index` 字段识别每个独立的工具调用，拼接分片返回的 `arguments` 字符串，最终输出完整的工具调用列表。

### 记录 Token 详情

使用内置属性记录 OpenAI Chat Completions 的 token 详细信息：

```yaml
attributes:
  # 使用便捷的内置属性提取特定字段
  - key: reasoning_tokens  # 推理token数（o1等推理模型）
    apply_to_log: true
  - key: cached_tokens  # 缓存命中的token数
    apply_to_log: true
  # 记录完整的token详情对象
  - key: input_token_details
    apply_to_log: true
  - key: output_token_details
    apply_to_log: true
```

#### 日志示例

对于使用了 prompt caching 和推理模型的请求，日志可能如下：

```json
{
  "ai_log": "{\"model\":\"gpt-4o\",\"input_token\":\"100\",\"output_token\":\"50\",\"reasoning_tokens\":\"25\",\"cached_tokens\":\"80\",\"input_token_details\":\"{\\\"cached_tokens\\\":80}\",\"output_token_details\":\"{\\\"reasoning_tokens\\\":25}\",\"llm_service_duration\":\"2000\"}"
}
```

其中：
- `reasoning_tokens`: 25 - 推理过程产生的 token 数
- `cached_tokens`: 80 - 从缓存中读取的 token 数
- `input_token_details`: 完整的输入 token 详情（JSON 格式）
- `output_token_details`: 完整的输出 token 详情（JSON 格式）

这些详情对于：
1. **成本优化**：了解缓存命中率，优化 prompt caching 策略
2. **性能分析**：分析推理 token 占比，评估推理模型的实际开销
3. **使用统计**：细粒度统计各类 token 的使用情况

## 流式响应观测能力

流式（Streaming）响应是 AI 对话的常见场景，插件提供了完善的流式观测支持，能够正确拼接和提取流式响应中的关键信息。

### 流式响应的挑战

流式响应将完整内容拆分为多个 SSE chunk 逐步返回，例如：

```
data: {"choices":[{"delta":{"content":"Hello"}}]}
data: {"choices":[{"delta":{"content":" 👋"}}]}
data: {"choices":[{"delta":{"content":"!"}}]}
data: [DONE]
```

要获取完整的回答内容，需要将各个 chunk 中的 `delta.content` 拼接起来。

### 自动拼接机制

插件针对不同类型的内容提供了自动拼接能力：

| 内容类型 | 拼接方式 | 说明 |
|---------|---------|------|
| `answer` | 文本追加（append） | 将各 chunk 的 `delta.content` 按顺序拼接成完整回答 |
| `reasoning` | 文本追加（append） | 将各 chunk 的 `delta.reasoning_content` 按顺序拼接 |
| `tool_calls` | 按 index 组装 | 识别每个工具调用的 `index`，分别拼接各自的 `arguments` |

#### answer 和 reasoning 拼接示例

流式响应：
```
data: {"choices":[{"delta":{"content":"你好"}}]}
data: {"choices":[{"delta":{"content":"，我是"}}]}
data: {"choices":[{"delta":{"content":"AI助手"}}]}
```

最终提取的 `answer`：`"你好，我是AI助手"`

#### tool_calls 拼接示例

流式响应（多个并行工具调用）：
```
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_001","function":{"name":"get_weather"}}]}}]}
data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_002","function":{"name":"get_time"}}]}}]}
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Beijing\"}"}}]}}]}
data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"city\":\"Shanghai\"}"}}]}}]}
```

最终提取的 `tool_calls`：
```json
[
  {"index":0,"id":"call_001","function":{"name":"get_weather","arguments":"{\"city\":\"Beijing\"}"}},
  {"index":1,"id":"call_002","function":{"name":"get_time","arguments":"{\"city\":\"Shanghai\"}"}}
]
```

### 使用默认配置快速启用

插件提供两种默认配置模式：

#### 轻量模式（推荐用于生产环境）

通过 `use_default_response_attributes: true` 启用轻量模式：

```yaml
use_default_response_attributes: true
```

此配置是**推荐的生产环境配置**，特别适合高并发、高延迟的场景：

| 字段 | 说明 |
|------|------|
| `model` | 模型名称（从请求体提取） |
| `reasoning_tokens` | 推理 token 数 |
| `cached_tokens` | 缓存命中 token 数 |
| `input_token_details` | 输入 token 详情 |
| `output_token_details` | 输出 token 详情 |

**为什么推荐轻量模式？**

LLM 请求有两个显著特点：**延迟高**（通常数秒到数十秒）和**请求体大**（多轮对话可能达到数百 KB 甚至 MB 级别）。

在高并发场景下，如果请求体和响应体都被缓存在内存中，积压的请求会占用大量内存：
- 假设 QPS=100，平均延迟=10秒，请求体=500KB
- 同时在处理的请求数 ≈ 100 × 10 = 1000 个
- 如果缓存完整请求体+响应体：1000 × 1.5MB ≈ **1.5GB 内存**

轻量模式通过以下方式降低内存占用：
- **缓冲请求体**：仅用于提取 `model` 字段（很小），不提取 `question`、`system`、`messages` 等大字段
- **不缓冲流式响应体**：不提取 `answer`、`reasoning`、`tool_calls` 等需要完整响应的字段
- **只统计 token**：从响应的 usage 字段提取 token 信息

**内存对比：**

| 场景 | 完整模式 | 轻量模式 |
|------|----------|----------|
| 单次请求 (1MB 请求 + 500KB 响应) | ~1.5MB | ~1MB（请求体） |
| 高并发 (100 QPS, 10s 延迟) | ~1.5GB | ~1GB |
| 超高并发 (1000 QPS, 10s 延迟) | ~15GB | ~10GB |

**注意**：轻量模式下 `chat_round` 字段会正常计算，`model` 会从请求体正常提取。

#### 完整模式

通过 `use_default_attributes: true` 可以一键启用完整的流式观测能力：

```yaml
use_default_attributes: true
```

此配置会自动记录以下字段，**但会缓冲完整的请求体和流式响应体**：

| 字段 | 说明 | 内存影响 |
|------|------|----------|
| `messages` | 完整对话历史 | ⚠️ 可能很大 |
| `question` | 最后一条用户消息 | 需要缓冲请求体 |
| `system` | 系统提示词 | 需要缓冲请求体 |
| `answer` | AI 回答（自动拼接流式 chunk） | ⚠️ 需要缓冲响应体 |
| `reasoning` | 推理过程（自动拼接流式 chunk） | ⚠️ 需要缓冲响应体 |
| `tool_calls` | 工具调用（自动按 index 组装） | 需要缓冲响应体 |
| `reasoning_tokens` | 推理 token 数 | 无 |
| `cached_tokens` | 缓存命中 token 数 | 无 |
| `input_token_details` | 输入 token 详情 | 无 |
| `output_token_details` | 输出 token 详情 | 无 |

**注意**：完整模式适用于调试、审计等需要完整对话记录的场景，但在高并发生产环境可能消耗大量内存。

### 流式日志示例

启用默认配置后，一个流式请求的日志输出示例：

```json
{
  "answer": "2 plus 2 equals 4.",
  "question": "What is 2+2?",
  "response_type": "stream",
  "tool_calls": null,
  "reasoning": null,
  "model": "glm-4-flash",
  "input_token": 10,
  "output_token": 8,
  "llm_first_token_duration": 425,
  "llm_service_duration": 985,
  "chat_id": "chat_abc123"
}
```

包含工具调用的流式日志示例：

```json
{
  "answer": null,
  "question": "What's the weather in Beijing?",
  "response_type": "stream",
  "tool_calls": [
    {
      "id": "call_abc123",
      "type": "function",
      "function": {
        "name": "get_weather",
        "arguments": "{\"location\": \"Beijing\"}"
      }
    }
  ],
  "model": "glm-4-flash",
  "input_token": 50,
  "output_token": 15,
  "llm_first_token_duration": 300,
  "llm_service_duration": 500
}
```

### 流式特有指标

流式响应会额外记录以下指标：

- `llm_first_token_duration`：从请求发出到收到首个 token 的时间（首字延迟）
- `llm_stream_duration_count`：流式请求次数

可用于监控流式响应的用户体验：

```promql
# 平均首字延迟
irate(route_upstream_model_consumer_metric_llm_first_token_duration[5m])
/
irate(route_upstream_model_consumer_metric_llm_stream_duration_count[5m])
```

## 调试

### 验证 ai_log 内容

在测试或调试过程中，可以通过开启 Higress 的 debug 日志来验证 ai_log 的内容：

```bash
# 日志格式示例
2026/01/31 23:29:30 proxy_debug_log: [ai-statistics] [nil] [test-request-id] [ai_log] attributes to be written: {"question":"What is 2+2?","answer":"4","reasoning":"...","tool_calls":[...],"session_id":"sess_123","model":"gpt-4","input_token":20,"output_token":10}
```

通过这个debug日志可以验证：
- question/answer/reasoning 是否正确提取
- tool_calls 是否正确拼接（特别是流式场景下的arguments）
- session_id 是否正确识别
- 各个字段是否符合预期

## 进阶

配合阿里云 SLS 数据加工，可以将 ai 相关的字段进行提取加工，例如原始日志为：

````
ai_log:{"question":"用python计算2的3次方","answer":"你可以使用 Python 的乘方运算符 `**` 来计算一个数的次方。计算2的3次方，即2乘以自己2次，可以用以下代码表示：\n\n```python\nresult = 2 ** 3\nprint(result)\n```\n\n运行这段代码，你会得到输出结果为8，因为2乘以自己两次等于8。","model":"qwen-max","input_token":"16","output_token":"76","llm_service_duration":"5913"}
````

使用如下数据加工脚本，可以提取出 question 和 answer：

```
e_regex("ai_log", grok("%{EXTRACTJSON}"))
e_set("question", json_select(v("json"), "question", default="-"))
e_set("answer", json_select(v("json"), "answer", default="-"))
```

提取后，SLS 中会添加 question 和 answer 两个字段，示例如下：

````
ai_log:{"question":"用python计算2的3次方","answer":"你可以使用 Python 的乘方运算符 `**` 来计算一个数的次方。计算2的3次方，即2乘以自己2次，可以用以下代码表示：\n\n```python\nresult = 2 ** 3\nprint(result)\n```\n\n运行这段代码，你会得到输出结果为8，因为2乘以自己两次等于8。","model":"qwen-max","input_token":"16","output_token":"76","llm_service_duration":"5913"}

question:用python计算2的3次方

answer:你可以使用 Python 的乘方运算符 `**` 来计算一个数的次方。计算2的3次方，即2乘以自己2次，可以用以下代码表示：

result = 2 ** 3
print(result)

运行这段代码，你会得到输出结果为8，因为2乘以自己两次等于8。

````

### 路径和内容类型过滤配置示例

#### 只处理特定 AI 路径

```yaml
enable_path_suffixes:
  - "/v1/chat/completions"
  - "/v1/embeddings"
  - "/generateContent"
```

#### 只处理特定内容类型

```yaml
enable_content_types:
  - "text/event-stream"
  - "application/json"
```

#### 处理所有路径（通配符）

```yaml
enable_path_suffixes:
  - "*"
```

#### 处理所有内容类型（空数组）

```yaml
enable_content_types: []
```

#### 完整配置示例

```yaml
enable_path_suffixes:
  - "/v1/chat/completions"
  - "/v1/embeddings"
  - "/generateContent"
enable_content_types:
  - "text/event-stream"
  - "application/json"
attributes:
  - key: model
    value_source: request_body
    value: model
    apply_to_log: true
  - key: consumer
    value_source: request_header
    value: x-mse-consumer
    apply_to_log: true
```
