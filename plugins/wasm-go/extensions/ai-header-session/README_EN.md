---
title: AI Header Session
keywords: [ AI Gateway, AI session, session, header ]
description: Extract session headers from various AI clients and deterministically normalize them into a unified session header
---

## Overview

The `ai-header-session` plugin reshapes **AI headers**: during the request-header phase it recognizes different AI coding clients (Claude Code, Cursor, Cline, Continue, GitHub Copilot, ...) and, from the scattered session-identifying headers each client carries, **deterministically** derives a single unified session header (default `X-AI-Session-Id`). Downstream components can use it for session statistics, rate limiting, auditing, trace aggregation, etc.

Key features:

- **Per-client matching**: collect different headers per client type; rules are configurable.
- **Unified, customizable output header name**: defaults to `X-AI-Session-Id`.
- **Reproducible (deterministic) session-id rule**: identical input headers always produce the same session id (see "Session ID rule").
- **Match-failure fallback**: when no client is recognized / no header rule hits, all request headers are dumped to the log for troubleshooting.
- **Configurable logging**: the `dump_unmatched` switch controls whether all headers are logged on match failure.

## Runtime attributes

Plugin execution phase: `Default Phase`
Plugin execution priority: `500`

## Processing flow

```
1. If the unified header (session_header) already exists -> idempotent skip, pass through
2. Read all request headers and recognize per the scheme chosen by match_mode:
   - clients scheme: match the client against the clients rules
     2a. No client matched -> match failure, dump all request headers (dump_unmatched), pass through
     2b. Client matched but some session_headers are missing -> dump all request headers (diagnostic), still continue
   - header scheme: walk header_rules level by level, take "content after the match" on the first hit
     2c. No rule hit -> match failure, dump all request headers (dump_unmatched), pass through
3. Compute the unique session id per the "Session ID rule", write it to session_header, pass through
```

The plugin never panics and never blocks a normal request; any error is logged and the request is passed through.

## Session ID rule (reproducible)

The session id is a **pure function** of its inputs, so it is reproducible and verifiable:

1. Pull each header value in the client's configured `session_headers` **fixed order** (missing values padded as empty strings).
2. Normalize: lower-case header names, trim values.
3. Build the canonical string: `<name>|<h1>=<v1>|<h2>=<v2>|...`.
4. Hash the canonical string (`fnv` by default, or `sha256`) and take a 16-char hex digest.
5. Output `<clientName>-<hash16>`, e.g. `claude-code-3f2a9c41b7d0e58a`.

> Hashing auth headers (e.g. `authorization` / `x-api-key`) is safe — the digest is one-way, yielding a stable, anonymous per-credential session key. The order of `session_headers` is **part of the contract**: changing it changes the derived id.

Under the **header scheme**, the id is derived deterministically too: a hit yields `(header name, extracted value)`, the canonical string is `<header>|<value>`, and the output is `<header>-<hash16>`, e.g. `authorization-3f2a9c41b7d0e58a`.

## Configuration

| Name | Type | Requirement | Default | Description |
|------|------|-------------|---------|-------------|
| `session_header` | string | optional | `X-AI-Session-Id` | Unified output session header name |
| `hash_algorithm` | string | optional | `fnv` | Digest algorithm, `fnv` or `sha256` |
| `match_mode` | string | optional | `clients` | Recognition scheme: `clients` (by client) or `header` (by header, level-by-level extraction) |
| `clients` | array of client rule | optional | built-in defaults | Client recognition & header-collection rules for the `clients` scheme |
| `header_rules` | array of header rule | required for `header` | - | Level-by-level extraction rules for the `header` scheme |
| `log` | log object | optional | - | Logging configuration |

### client rule (`match_mode: clients`)

| Name | Type | Requirement | Default | Description |
|------|------|-------------|---------|-------------|
| `name` | string | required | - | Client name; also used as the session-id prefix |
| `match_header` | string | optional | `user-agent` | Header used to recognize the client |
| `match_pattern` | string | required | - | Go RE2 regex matched against `match_header` value |
| `session_headers` | array of string | required | - | **Ordered** list of header names used to derive the id |

### header rule (`match_mode: header`)

| Name | Type | Requirement | Default | Description |
|------|------|-------------|---------|-------------|
| `header` | string | required | - | Header name to extract |
| `pattern` | string | optional | - | Go RE2 regex matched on the header value; on a hit the substring **after the match** is taken; empty means take the whole value |

> Level-by-level semantics: rules are tried in order; the first rule whose header exists and yields a non-empty (trimmed) extracted value wins; an empty extraction falls through to the next rule. E.g. `header: authorization, pattern: "(?i)^Bearer\\s+"` extracts `sk-abc123` from `Bearer sk-abc123`.

### log object

| Name | Type | Requirement | Default | Description |
|------|------|-------------|---------|-------------|
| `dump_unmatched` | bool | optional | `true` | Whether to dump all request headers on match failure (no client / no rule hit / missing source headers) |

## Built-in default clients

When `clients` is not configured, the following built-in rules are used (all overridable):

| Client | Match (user-agent regex) | session_headers (ordered) |
|--------|---------------------------|----------------------------|
| `claude-code` | `(?i)claude` | `authorization`, `x-api-key`, `user-agent` |
| `cursor` | `(?i)cursor` | `authorization`, `x-cursor-checksum`, `user-agent` |
| `cline` | `(?i)cline` | `authorization`, `user-agent` |
| `continue` | `(?i)continue` | `authorization`, `user-agent` |
| `github-copilot` | `(?i)(githubcopilot|copilot|vscode)` | `authorization`, `x-request-id`, `user-agent` |

## Examples

### Minimal (all defaults)

```yaml
{}
```

A request with `user-agent: claude-cli/1.0.42` gets a header like `X-AI-Session-Id: claude-code-3f2a9c41b7d0e58a`.

### Custom header name + sha256 + custom client

```yaml
session_header: "X-Session"
hash_algorithm: "sha256"
clients:
  - name: "my-agent"
    match_header: "user-agent"
    match_pattern: "(?i)myagent"
    session_headers:
      - "authorization"
      - "x-device-id"
```

### Header scheme (level-by-level extraction, take content after the match)

```yaml
match_mode: "header"
header_rules:
  - header: "authorization"        # prefer the content after the Bearer token
    pattern: "(?i)^Bearer\\s+"
  - header: "x-session-id"         # fallback: take the whole value
```

A request with `authorization: Bearer sk-abc123` extracts `sk-abc123` and writes a header like `X-AI-Session-Id: authorization-3f2a9c41b7d0e58a`.

### Explicit session headers, level-by-level (recommended)

```yaml
match_mode: "header"
header_rules:
  - header: "x-session-id"
  - header: "x-conversation-id"
  - header: "x-request-id"
  - header: "x-client-request-id"
  - header: "openai-conversation-id"
  - header: "anthropic-request-id"
log:
  dump_unmatched: true   # when none of the headers exist, dump all request headers for troubleshooting
```

## Build

```bash
cd plugins/wasm-go
PLUGIN_NAME=ai-header-session make build
# output: extensions/ai-header-session/plugin.wasm
```
