# Design: ai-anthropic-system-fold

## Problem

The Anthropic Messages API carries the system prompt in a top-level `system`
field, and the `messages` array is restricted to `user` / `assistant` roles.

Some clients — notably **Claude Code CLI (>= 2.1.154)** — instead place system
prompt content inside the `messages` array as `{"role": "system", ...}` entries
(a mid-conversation system-message style). Backends whose Anthropic protocol
layer strictly validates `messages[*].role` reject such requests during request
validation, before inference:

```
400 1 validation error:
{'type': 'literal_error', 'loc': ('body', 'messages', N, 'role'),
 'msg': "Input should be 'user' or 'assistant'", 'input': 'system'}
```

This affects gateways that front backends with a strict Anthropic layer (e.g.
some vLLM / SGLang builds), even when the gateway itself is healthy.

## Goals / Non-goals

- **Goal:** make such requests succeed against strict backends, with a
  spec-aligned transformation, transparently to the client.
- **Non-goal:** handle other non-standard roles (e.g. `ctx` / `msg`). They are
  left to the backend's strict validation. (Inspection of Claude Code bundles in
  the referenced upstream PRs found only `system` is actually emitted.)

## Approach

Normalize at the gateway: for Anthropic `/messages` requests, remove every
`role: "system"` entry from `messages` and fold its text into the top-level
`system` field.

This mirrors the normalization that backends and peer proxies independently
converged on:

- vLLM — vllm-project/vllm#44283 (merged)
- Xinference — xorbitsai/inference#5049 (merged)
- CodeRouter — zephel01/CodeRouter#23 (merged)

### Why fold into top-level `system` (vs. alternatives)

- **Expanding the role enum** to allow `system` in `messages` was rejected
  upstream — it would diverge from the Anthropic spec.
- **Coercing `system` → `user`** keeps position but changes semantics (a system
  instruction becomes a user turn). Folding into `system` is more faithful: it
  is exactly where the Anthropic spec expects system content to live.

### Merge rules

- Existing top-level `system` content is preserved first; folded content is
  appended after it.
- If `system` is a string (or absent) → result is a string (`\n\n`-joined).
- If `system` is a content-block array → a `{"type":"text","text":...}` block is
  appended.
- Both string and content-block message `content` are supported (text blocks are
  concatenated).

## Scope

- Acts only on the Anthropic `/messages` endpoint.
- Explicitly excludes `/chat/completions` — the OpenAI protocol legitimately
  allows `system` inside `messages`, so it must not be touched.

## Implementation notes

- JSON is read/rewritten with `gjson` / `sjson` (no full unmarshal).
- The endpoint is determined from `:path` in the **request-headers phase** and
  cached on the context; the request-body phase reads the cached decision. This
  is required because reading `:path` in the body phase fails in the wasm-go host
  (`get request path failed: bad argument`).
- `content-length` is removed in the headers phase so the proxy recomputes it
  after the body rewrite. Non-`/messages` requests skip body buffering.

## Limitations

- Only `role: "system"` is folded; other non-standard roles still fail backend
  validation (by design — see Non-goals).
- Folding moves mid-conversation system content to the top-level system prompt,
  i.e. it no longer applies "from that position onward" — acceptable for the
  compatibility goal and consistent with the upstream backend fixes.
