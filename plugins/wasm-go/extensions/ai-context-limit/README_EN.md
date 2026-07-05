---
title: AI Context Limit
keywords: [ AI Gateway, Context Window, Token ]
description: AI Context Limit plugin configuration reference
---

## Functional Description

`ai-context-limit` estimates the input token count of OpenAI Chat Completions, Anthropic Messages and other compatible requests before forwarding them to the upstream model service. When the estimated input size exceeds the configured context window limit, the plugin returns an error response directly.

This plugin can be used to control context window size by route, service, domain, or MCP Server. It is suitable for setting independent context limits for different applications, models, or traffic entry points.

## Runtime Properties

Plugin execution phase: `Default Phase`

Plugin execution priority: `1000`

## Build

The plugin requires an embedded BPE vocabulary file. Download it before the first build:

```bash
make build
```

Or step by step:

```bash
make prepare      # Download vocabulary to bpe/o200k_base.tiktoken
make build-go    # Compile WASM
```

## Configuration Fields

| Name | Data Type | Requirement | Default Value | Description |
|------|-----------|-------------|---------------|-------------|
| `max_context_tokens` | int | Required | - | Maximum context token limit. Requests whose estimated input size exceeds this value will be blocked. Set to 0 to disable. |
| `buffer_ratio` | float | Optional | 1.10 | Safety buffer ratio (valid range: 0–10). The estimated token count is multiplied by this ratio before comparison. |
| `error_status_code` | int | Optional | 400 | HTTP status code returned when the request exceeds the context limit (valid range: 400–599). |

## Configuration Example

```yaml
max_context_tokens: 128000
buffer_ratio: 1.10
error_status_code: 400
```

## Response Example

When a request exceeds the configured limit, the plugin returns an error response in the following format:

```json
{
  "error": {
    "message": "This model's maximum context length is 128000 tokens. Your request had approximately 140000 tokens.",
    "type": "invalid_request_error",
    "code": "context_length_exceeded"
  }
}
```

## Notes

- The plugin counts text-bearing fields including text, tool schema, tool arguments, thinking, text document, and search_result. Non-text content such as images, audio, and base64/url/file documents will skip token counting and the entire request is passed through.
- Non-JSON requests and requests that are not in a compatible protocol format will not trigger the context limit.
- The plugin reads up to 8MB of the request body for text estimation; content beyond this limit will not be processed.
