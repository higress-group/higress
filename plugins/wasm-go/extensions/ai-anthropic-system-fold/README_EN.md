---
title: AI Anthropic System Fold
keywords: [higress, ai, anthropic, claude code]
description: Normalize inline `role: system` messages on the Anthropic Messages API into the top-level `system` field.
---

## Function Description

On the Anthropic Messages API endpoint (`/v1/messages`), this plugin folds any
inline `role: "system"` messages found inside the `messages` array into the
top-level `system` field, then removes them from `messages`.

This restores compatibility with backends whose Anthropic protocol layer
strictly validates `messages[*].role` as only `user` / `assistant` (e.g. some
vLLM / SGLang builds), which otherwise reject such requests with:

```
400 1 validation error:
{'type': 'literal_error', 'loc': ('body', 'messages', N, 'role'),
 'msg': "Input should be 'user' or 'assistant'", 'input': 'system'}
```

Some clients — notably **Claude Code CLI (>= 2.1.154)** — place system prompt
content inside the `messages` array as `{"role": "system", ...}` entries rather
than in the Anthropic-standard top-level `system` field. Folding them into
`system` is the standard, spec-aligned normalization (the same approach taken by
backends such as vLLM and SGLang).

## Behavior

- Only acts on the Anthropic `/messages` endpoint. The OpenAI
  `/chat/completions` endpoint is explicitly excluded, since the OpenAI protocol
  legitimately allows `system` messages inside the array.
- Removes every `role: "system"` entry from `messages` and appends its text to
  the top-level `system`:
  - existing top-level `system` content is preserved first, the folded content
    is appended after it;
  - if `system` is a string (or absent), the result is a string;
  - if `system` is a content-block array, a `{"type":"text","text":...}` block
    is appended.
- Both string content and content-block (`[{"type":"text","text":...}]`) message
  content are supported.
- Requests without any `role: "system"` message in `messages` are passed through
  unchanged.
- Other non-standard roles are left untouched and still rejected by the
  backend's strict validation.

## Configuration

This plugin has no configuration items.

| Name | Type | Required | Default | Description |
| ---- | ---- | -------- | ------- | ----------- |
| -    | -    | -        | -       | No configuration needed. |

## Example

Request to `/v1/messages`:

```json
{
  "model": "claude-3-5-sonnet",
  "max_tokens": 1024,
  "system": "Be concise.",
  "messages": [
    { "role": "user", "content": "Hi" },
    { "role": "system", "content": "Always answer in English." }
  ]
}
```

After the plugin processes it, the request forwarded to the backend becomes:

```json
{
  "model": "claude-3-5-sonnet",
  "max_tokens": 1024,
  "system": "Be concise.\n\nAlways answer in English.",
  "messages": [
    { "role": "user", "content": "Hi" }
  ]
}
```
