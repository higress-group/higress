---
title: AI Statistics
keywords: [higress, AI, observability]
description: AI Statistics plugin configuration reference
---

## Introduction

Provides basic AI observability capabilities, including metric, log, and trace. The ai-proxy plug-in needs to be connected afterwards. If the ai-proxy plug-in is not connected, the user needs to configure it accordingly to take effect.

## Runtime Properties

Plugin Phase: `CUSTOM`
Plugin Priority: `200`

## Configuration instructions

The default request of the plug-in conforms to the openai protocol format and provides the following basic observable values. Users do not need special configuration:

- metric: It provides indicators such as input token, output token, rt of the first token (streaming request), total request rt, etc., and supports observation in the four dimensions of gateway, routing, service, and model.
- log: Provides input_token, output_token, model, llm_service_duration, llm_first_token_duration and other fields

Users can also expand observable values ​​through configuration:

| Name             | Type  | Required | Default | Description |
|----------------|-------|------|-----|------------------------|
| `attributes` | []Attribute | optional  | -   | Information that the user wants to record in log/span |
| `disable_openai_usage` | bool | optional  | false   | When using a non-OpenAI-compatible protocol, the support for model and token is non-standard. Setting the configuration to true can prevent errors. |
| `value_length_limit` | int | optional  | 4000   | length limit for each value |
| `enable_path_suffixes`   | []string    | optional | ["/v1/chat/completions","/v1/completions","/v1/embeddings","/v1/models","/generateContent","/streamGenerateContent"] | Only effective for requests with these specific path suffixes, can be configured as "\*" to match all paths                                         |
| `enable_content_types` | []string    | optional | ["text/event-stream","application/json"]                                                                             | Only buffer response body for these content types                                                                                                   |
| `session_id_header` | string | optional  | -   | Specify the header name to read session ID from. If not configured, it will automatically search in the following priority: `x-openclaw-session-key`, `x-clawdbot-session-key`, `x-moltbot-session-key`, `x-agent-session`. Session ID can be used to trace multi-turn Agent conversations |

Attribute Configuration instructions:

| Name                    | Type   | Required | Default | Description                                                                                                                                                  |
| ----------------------- | ------ | -------- | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `key`                   | string | required | -       | attribute key                                                                                                                                                |
| `value_source`          | string | required | -       | attribute value source, optional values ​​are `fixed_value`, `request_header`, `request_body`, `response_header`, `response_body`, `response_streaming_body` |
| `value`                 | string | required | -       | how to get attribute value                                                                                                                                   |
| `default_value`         | string | optional | -       | default value for attribute                                                                                                                                  |
| `rule`                  | string | optional | -       | Rule to extract attribute from streaming response, optional values ​​are `first`, `replace`, `append`                                                        |
| `apply_to_log`          | bool   | optional | false   | Whether to record the extracted information in the log                                                                                                       |
| `apply_to_span`         | bool   | optional | false   | Whether to record the extracted information in the link tracking span                                                                                        |
| `trace_span_key`        | string | optional | -       | span attribute key, default is the value of `key`                                                                                                            |
| `as_separate_log_field` | bool   | optional | false   | Whether to use a separate log field, the field name is equal to the value of `key`                                                                           |

The meanings of various values for `value_source` ​​are as follows:

- `fixed_value`: fixed value
- `request_header`: The attribute is obtained through the http request header
- `request_body`: The attribute is obtained through the http request body
- `response_header`: The attribute is obtained through the http response header
- `response_body`: The attribute is obtained through the http response body
- `response_streaming_body`: The attribute is obtained through the http streaming response body

When `value_source` is `response_streaming_body`, `rule` should be configured to specify how to obtain the specified value from the streaming body. The meaning of the value is as follows:

- `first`: extract value from the first valid chunk
- `replace`: extract value from the last valid chunk
- `append`: join value pieces from all valid chunks

### Built-in Attributes

The plugin provides several built-in attribute keys that can be used directly without configuring `value_source` and `value`. These built-in attributes automatically extract corresponding values from requests/responses:

| Built-in Key | Description | Use Case |
|--------------|-------------|----------|
| `question` | User's question content | Supports OpenAI/Claude message formats |
| `answer` | AI's answer content | Supports OpenAI/Claude message formats, both streaming and non-streaming |
| `tool_calls` | Tool call information | OpenAI/Claude tool calls |
| `reasoning` | Reasoning process | OpenAI o1 and other reasoning models |
| `reasoning_tokens` | Number of reasoning tokens (e.g., o1 model) | OpenAI Chat Completions, extracted from `output_token_details.reasoning_tokens` |
| `cached_tokens` | Number of cached tokens | OpenAI Chat Completions, extracted from `input_token_details.cached_tokens` |
| `input_token_details` | Complete input token details (object) | OpenAI/Gemini/Anthropic, includes cache, tool usage, etc. |
| `output_token_details` | Complete output token details (object) | OpenAI/Gemini/Anthropic, includes reasoning tokens, generated images, etc. |

When using built-in attributes, you only need to set `key`, `apply_to_log`, etc., without setting `value_source` and `value`.

**Notes**:
- `reasoning_tokens` and `cached_tokens` are convenience fields extracted from token details, applicable to OpenAI Chat Completions API
- `input_token_details` and `output_token_details` will record the complete token details object as a JSON string

## Configuration example

If you want to record ai-statistic related statistical values in the gateway access log, you need to modify log_format and add a new field based on the original log_format. The example is as follows:

```yaml
'{"ai_log":"%FILTER_STATE(wasm.ai_log:PLAIN)%"}'
```

If the field is set with `as_separate_log_field`, for example:

```yaml
attributes:
  - key: consumer
    value_source: request_header
    value: x-mse-consumer
    apply_to_log: true
    as_separate_log_field: true
```

Then to print in the log, you need to set log_format additionally:

```
'{"consumer":"%FILTER_STATE(wasm.consumer:PLAIN)%"}'
```

### Empty

#### Metric

```
# counter, cumulative count of input tokens
route_upstream_model_consumer_metric_input_token{ai_route="ai-route-aliyun.internal",ai_cluster="outbound|443||llm-aliyun.internal.dns",ai_model="qwen-turbo",ai_consumer="none"} 24

# counter, cumulative count of output tokens
route_upstream_model_consumer_metric_output_token{ai_route="ai-route-aliyun.internal",ai_cluster="outbound|443||llm-aliyun.internal.dns",ai_model="qwen-turbo",ai_consumer="none"} 507

# counter, cumulative total duration of both streaming and non-streaming requests
route_upstream_model_consumer_metric_llm_service_duration{ai_route="ai-route-aliyun.internal",ai_cluster="outbound|443||llm-aliyun.internal.dns",ai_model="qwen-turbo",ai_consumer="none"} 6470

# counter, cumulative count of both streaming and non-streaming requests
route_upstream_model_consumer_metric_llm_duration_count{ai_route="ai-route-aliyun.internal",ai_cluster="outbound|443||llm-aliyun.internal.dns",ai_model="qwen-turbo",ai_consumer="none"} 2

# counter, cumulative latency of the first token in streaming requests
route_upstream_model_consumer_metric_llm_first_token_duration{ai_route="ai-route-aliyun.internal",ai_cluster="outbound|443||llm-aliyun.internal.dns",ai_model="qwen-turbo",ai_consumer="none"} 340

# counter, cumulative count of streaming requests
route_upstream_model_consumer_metric_llm_stream_duration_count{ai_route="ai-route-aliyun.internal",ai_cluster="outbound|443||llm-aliyun.internal.dns",ai_model="qwen-turbo",ai_consumer="none"} 1
```

Below are some example usages of these metrics:

Average latency of the first token in streaming requests:

```
irate(route_upstream_model_consumer_metric_llm_first_token_duration[2m])
/
irate(route_upstream_model_consumer_metric_llm_stream_duration_count[2m])
```

Average process duration of both streaming and non-streaming requests:

```
irate(route_upstream_model_consumer_metric_llm_service_duration[2m])
/
irate(route_upstream_model_consumer_metric_llm_duration_count[2m])
```

#### Log

```json
{
  "ai_log": "{\"model\":\"qwen-turbo\",\"input_token\":\"10\",\"output_token\":\"69\",\"llm_first_token_duration\":\"309\",\"llm_service_duration\":\"1955\"}"
}
```

If the request contains a session ID header, the log will automatically include a `session_id` field:

```json
{
  "ai_log": "{\"session_id\":\"sess_abc123\",\"model\":\"qwen-turbo\",\"input_token\":\"10\",\"output_token\":\"69\",\"llm_first_token_duration\":\"309\",\"llm_service_duration\":\"1955\"}"
}
```

#### Trace

When the configuration is empty, no additional attributes will be added to the span.

### Record Token Details

Use built-in attributes to record token details for OpenAI Chat Completions:

```yaml
attributes:
  # Use convenient built-in attributes to extract specific fields
  - key: reasoning_tokens  # Reasoning tokens (o1 and other reasoning models)
    apply_to_log: true
  - key: cached_tokens  # Cached tokens from prompt caching
    apply_to_log: true
  # Record complete token details objects
  - key: input_token_details
    apply_to_log: true
  - key: output_token_details
    apply_to_log: true
```

#### Log Example

For requests using prompt caching and reasoning models, the log might look like:

```json
{
  "ai_log": "{\"model\":\"gpt-4o\",\"input_token\":\"100\",\"output_token\":\"50\",\"reasoning_tokens\":\"25\",\"cached_tokens\":\"80\",\"input_token_details\":\"{\\\"cached_tokens\\\":80}\",\"output_token_details\":\"{\\\"reasoning_tokens\\\":25}\",\"llm_service_duration\":\"2000\"}"
}
```

Where:
- `reasoning_tokens`: 25 - Number of tokens generated during reasoning
- `cached_tokens`: 80 - Number of tokens read from cache
- `input_token_details`: Complete input token details (JSON format)
- `output_token_details`: Complete output token details (JSON format)

These details are useful for:
1. **Cost optimization**: Understanding cache hit rates to optimize prompt caching strategy
2. **Performance analysis**: Analyzing reasoning token ratio to evaluate actual overhead of reasoning models
3. **Usage statistics**: Fine-grained statistics of various token types

## Debugging

### Verifying ai_log Content

During testing or debugging, you can enable Higress debug logging to verify the ai_log content:

```bash
# Log format example
2026/01/31 23:29:30 proxy_debug_log: [ai-statistics] [nil] [test-request-id] [ai_log] attributes to be written: {"question":"What is 2+2?","answer":"4","reasoning":"...","tool_calls":[...],"session_id":"sess_123","model":"gpt-4","input_token":20,"output_token":10}
```

This debug log allows you to verify:
- Whether question/answer/reasoning are correctly extracted
- Whether tool_calls are properly concatenated (especially arguments in streaming scenarios)
- Whether session_id is correctly identified
- Whether all fields match expectations

### Extract token usage information from non-openai protocols

When setting the protocol to original in ai-proxy, taking Alibaba Cloud Bailian as an example, you can make the following configuration to specify how to extract `model`, `input_token`, `output_token`

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

#### Metric

```
route_upstream_model_consumer_metric_input_token{ai_route="bailian",ai_cluster="qwen",ai_model="qwen-max"} 343
route_upstream_model_consumer_metric_output_token{ai_route="bailian",ai_cluster="qwen",ai_model="qwen-max"} 153
route_upstream_model_consumer_metric_llm_service_duration{ai_route="bailian",ai_cluster="qwen",ai_model="qwen-max"} 3725
route_upstream_model_consumer_metric_llm_duration_count{ai_route="bailian",ai_cluster="qwen",ai_model="qwen-max"} 1
```

#### Log

```json
{
  "ai_log": "{\"model\":\"qwen-max\",\"input_token\":\"343\",\"output_token\":\"153\",\"llm_service_duration\":\"19110\"}"
}
```

#### Trace

Three additional attributes `model`, `input_token`, and `output_token` can be seen in the trace spans.

### Cooperate with authentication and authentication record consumer

```yaml
attributes:
  - key: consumer
    value_source: request_header
    value: x-mse-consumer
    apply_to_log: true
```

### Record questions and answers

#### Record only current turn's question and answer

```yaml
attributes:
  - key: question # Record the current turn's question (last user message)
    value_source: request_body
    value: messages.@reverse.0.content
    apply_to_log: true
  - key: answer
    value_source: response_streaming_body
    value: choices.0.delta.content
    rule: append
    apply_to_log: true
  - key: answer
    value_source: response_body
    value: choices.0.message.content
    apply_to_log: true
```

#### Record complete multi-turn conversation history (Recommended)

For multi-turn Agent conversation scenarios, using built-in attributes greatly simplifies the configuration:

```yaml
session_id_header: "x-session-id"  # Optional, specify session ID header
attributes:
  - key: messages     # Complete conversation history
    value_source: request_body
    value: messages
    apply_to_log: true
  - key: question     # Built-in, auto-extracts last user message
    apply_to_log: true
  - key: answer       # Built-in, auto-extracts answer
    apply_to_log: true
  - key: reasoning    # Built-in, auto-extracts reasoning process
    apply_to_log: true
  - key: tool_calls   # Built-in, auto-extracts tool calls
    apply_to_log: true
```

**Built-in Attributes:**

The plugin provides the following built-in attribute keys that automatically extract values without configuring `value_source` and `value` fields:

| Built-in Key | Description | Default value_source |
|-------------|-------------|----------------------|
| `question` | Automatically extracts the last user message | `request_body` |
| `answer` | Automatically extracts answer content (supports OpenAI/Claude protocols) | `response_streaming_body` / `response_body` |
| `tool_calls` | Automatically extracts and assembles tool calls (streaming scenarios auto-concatenate arguments by index) | `response_streaming_body` / `response_body` |
| `reasoning` | Automatically extracts reasoning process (reasoning_content, e.g., DeepSeek-R1) | `response_streaming_body` / `response_body` |

> **Note**: If `value_source` and `value` are configured, the configured values take priority for backward compatibility.

Example log output:

```json
{
  "ai_log": "{\"session_id\":\"sess_abc123\",\"messages\":[{\"role\":\"user\",\"content\":\"What's the weather in Beijing?\"}],\"question\":\"What's the weather in Beijing?\",\"reasoning\":\"The user wants to know the weather in Beijing, I need to call the weather query tool.\",\"tool_calls\":[{\"index\":0,\"id\":\"call_abc123\",\"type\":\"function\",\"function\":{\"name\":\"get_weather\",\"arguments\":\"{\\\"location\\\":\\\"Beijing\\\"}\"}}],\"model\":\"deepseek-reasoner\"}"
}
```

**Streaming tool_calls handling:**

The plugin automatically identifies each independent tool call by the `index` field, concatenates fragmented `arguments` strings, and outputs the complete tool call list.

### Path and Content Type Filtering Configuration Examples

#### Process Only Specific AI Paths

```yaml
enable_path_suffixes:
  - "/v1/chat/completions"
  - "/v1/embeddings"
  - "/generateContent"
```

#### Process Only Specific Content Types

```yaml
enable_content_types:
  - "text/event-stream"
  - "application/json"
```

#### Process All Paths (Wildcard)

```yaml
enable_path_suffixes:
  - "*"
```

#### Complete Configuration Example

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
