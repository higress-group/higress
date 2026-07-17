# ai-context-limit 设计文档

## 背景

大模型网关经常需要在请求到达上游模型之前拦截超长 prompt。现有 token 相关插件更多依赖模型响应中的 usage 字段进行事后统计，适合做计量和配额，但无法阻止超限请求进入模型。

`ai-context-limit` 提供请求侧上下文窗口保护能力，面向 OpenAI Chat Completions、Anthropic Messages 等协议兼容请求，在转发前估算输入 token 数，并在估算结果超过配置阈值时直接返回错误响应。

## 目标

- 在请求发送到上游模型前拦截超限文本输入。
- 支持通过 Higress WasmPlugin 在路由、服务、域名和 MCP Server 等粒度配置。
- 首个版本保持数据面自包含，运行时不依赖网络下载分词资源。
- 返回 OpenAI 兼容错误响应，方便常见 SDK 识别异常。

## 非目标

- 为每个模型系列精确适配专属分词器。
- 根据模型名自动选择分词器。
- 统计多模态内容的 token。
- 做响应侧 token 统计或配额管理。

如果后续特定模型族对精度有更高要求，可以在该插件基础上继续扩展。

## 配置

```yaml
max_context_tokens: 128000
buffer_ratio: 1.10
error_status_code: 400
```

| 字段 | 类型 | 默认值 | 校验规则 | 含义 |
|---|---|---|---|---|
| `max_context_tokens` | int | - | `>= 0` | 最大输入 token 估算阈值。设为 `0` 表示禁用拦截。 |
| `buffer_ratio` | float | `1.10` | `0 <= value <= 10` | 安全缓冲系数，估算 token 数会先乘以该系数再与阈值比较。设为 `0` 使用默认值。 |
| `error_status_code` | int | `400` | `400 <= value <= 599` | 请求被拦截时返回的 HTTP 状态码。 |

## 请求处理流程

1. 请求头阶段，非 JSON 请求和未启用配置直接放行，不读取请求体。
2. JSON 请求会把请求体 buffer 上限调到 8MB，并等待请求体阶段处理。
3. 插件会自动识别请求协议（OpenAI 或 Anthropic），并抽取对应字段的文本：
   - **OpenAI Chat Completions**：
     - `messages[].content` 字符串或 text parts 数组；
     - `messages[].role` 和 `messages[].name`；
     - `messages[].tool_calls[].function.name` 和 `arguments`；
     - `tools[].function.name`、`description`、`parameters`；
     - `response_format.json_schema.name`、`description`、`schema`；
     - 顶层 `system` 字段。
   - **Anthropic Messages**（通过 `tools[].input_schema` / `tool_use` / `tool_result` / `thinking` / `redacted_thinking` / `document` / `search_result` 等特有字段识别）：
     - `messages[].content` 字符串或 content block 数组；
     - `messages[].role`；
     - `text` block 的 `text` 字段；
     - `tool_use` block 的 `name` 和 `input`（raw JSON）；
     - `tool_result` block 的 `content`（字符串或 content block 数组，递归处理）；
     - `thinking` block 的 `thinking` 字段；
     - `redacted_thinking` block 的 `data` 字段（保守计入）；
     - `document` block：`source.type=text` 时计入 `title` + `source.data`，其他视为多模态；
     - `search_result` block 的 `title` + `source` + `content[]` 中的 text blocks；
     - `tools[].name`、`description`、`type`、`input_schema`（raw JSON）；
     - 顶层 `system`（字符串或 text block 数组）。
4. 如果检测到非文本二进制内容（如图片、音频、base64/url/file document），整个请求直接放行。插件会统计所有文本承载字段，未知非文本 block 视为多模态并放行。
5. 插件对抽取文本进行 token 估算，并将结果乘以 `buffer_ratio` 后与 `max_context_tokens` 比较。
6. 当 `estimated_tokens > max_context_tokens` 时，插件返回 OpenAI 兼容的 `context_length_exceeded` 错误。

## Token 估算策略

首个版本使用单一内嵌 BPE 词表，并在插件启动时初始化 tokenizer。这样可以避免 WASM 沙箱运行时下载资源，使请求处理完全在本地完成。

默认 `buffer_ratio` 为 `1.10`。基于长中文文档、混合 RAG、代码、多轮对话等文本的验证结果，该缓冲系数可以覆盖已观察到的低估场景，同时保持实现简单、确定。

## 错误响应

```json
{
  "error": {
    "message": "This model's maximum context length is 128000 tokens. Your request had approximately 140000 tokens.",
    "type": "invalid_request_error",
    "code": "context_length_exceeded"
  }
}
```

## 验证

实现已包含单元测试，覆盖：

- 配置默认值和非法值；
- 从 messages、tools、顶层 system 字段抽取文本；
- 多模态检测与放行；
- token 计数基础行为；
- 严格阈值比较逻辑。

插件已通过以下验证：

- `go test ./...`
- `go vet ./...`
- `make local-build PLUGIN_NAME=ai-context-limit`

---

# ai-context-limit Design

## Background

Large language model gateways often need to reject over-sized prompts before they reach the upstream model. Existing token-related plugins mainly rely on response-side usage fields, which is useful for accounting but cannot prevent an over-limit request from reaching the model.

`ai-context-limit` provides request-side context window protection for OpenAI Chat Completions, Anthropic Messages and other compatible traffic. It estimates input tokens before forwarding and returns an error response when the estimated input size exceeds the configured limit.

## Goals

- Block over-limit text requests before they are sent to the upstream model.
- Support route, service, domain, and MCP Server level configuration through Higress WasmPlugin.
- Keep the first version self-contained in the data plane, without runtime network access for tokenizer resources.
- Provide OpenAI-compatible error responses so common SDKs can parse the failure.

## Non-goals

- Exact tokenizer matching for every model family.
- Model-name based tokenizer selection.
- Multimodal token counting.
- Response-side token accounting or quota management.

These can be added later if there is a stronger precision requirement for specific model families.

## Configuration

```yaml
max_context_tokens: 128000
buffer_ratio: 1.10
error_status_code: 400
```

| Field | Type | Default | Validation | Meaning |
|---|---|---|---|---|
| `max_context_tokens` | int | - | `>= 0` | Maximum estimated input tokens. `0` disables blocking. |
| `buffer_ratio` | float | `1.10` | `0 <= value <= 10` | Safety multiplier applied to estimated tokens before comparison. `0` uses the default. |
| `error_status_code` | int | `400` | `400 <= value <= 599` | HTTP status code for blocked requests. |

## Request Processing

1. In the request header phase, non-JSON requests and disabled configs are passed through without reading the body.
2. For JSON requests, the plugin raises the request body buffer limit to 8MB and waits for the body phase.
3. The plugin auto-detects the request protocol (OpenAI or Anthropic) and extracts text from the corresponding fields:
   - **OpenAI Chat Completions**:
     - `messages[].content` string or text parts array;
     - `messages[].role` and `messages[].name`;
     - `messages[].tool_calls[].function.name` and `arguments`;
     - `tools[].function.name`, `description`, and `parameters`;
     - `response_format.json_schema.name`, `description`, and `schema`;
     - top-level `system`.
   - **Anthropic Messages** (detected via `tools[].input_schema` / `tool_use` / `tool_result` / `thinking` / `redacted_thinking` / `document` / `search_result`):
     - `messages[].content` string or content block array;
     - `messages[].role`;
     - `text` block `text` field;
     - `tool_use` block `name` and `input` (raw JSON);
     - `tool_result` block `content` (string or content block array, recursively processed);
     - `thinking` block `thinking` field;
     - `redacted_thinking` block `data` field (conservatively counted);
     - `document` block: `source.type=text` counts `title` + `source.data`, others treated as multimodal;
     - `search_result` block `title` + `source` + `content[]` text blocks;
     - `tools[].name`, `description`, `type`, and `input_schema` (raw JSON);
     - top-level `system` (string or text block array).
4. If non-text binary content is detected (e.g., images, audio, base64/url/file documents), the request is passed through. The plugin counts all text-bearing fields; unknown non-text blocks are treated as multimodal and bypassed.
5. The extracted text is tokenized, multiplied by `buffer_ratio`, and compared with `max_context_tokens`.
6. If `estimated_tokens > max_context_tokens`, the plugin returns an OpenAI-compatible `context_length_exceeded` response.

## Token Estimation Strategy

The first version uses a single embedded BPE vocabulary and initializes the tokenizer once during plugin startup. This avoids runtime downloads in the WASM sandbox and keeps request processing fully local.

The default `buffer_ratio` is `1.10`. Internal validation on long Chinese documents, mixed RAG content, code, and multi-turn conversations showed that the buffer covers observed under-estimation cases while keeping the implementation simple and deterministic.

## Error Response

```json
{
  "error": {
    "message": "This model's maximum context length is 128000 tokens. Your request had approximately 140000 tokens.",
    "type": "invalid_request_error",
    "code": "context_length_exceeded"
  }
}
```

## Validation

The implementation includes unit tests for:

- configuration defaults and invalid values;
- text extraction from messages, tools, and top-level system fields;
- multimodal detection and pass-through behavior;
- token counting basics;
- strict threshold comparison.

The plugin has also been verified with:

- `go test ./...`
- `go vet ./...`
- `make local-build PLUGIN_NAME=ai-context-limit`
