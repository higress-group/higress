---
title: AI 会话头整形
keywords: [ AI网关, AI会话, session, header ]
description: 提取各类 AI 客户端的会话标识 header，确定性归一为统一的 session 头
---

## 功能说明

`ai-header-session` 插件用于 **AI Header 整形**：在请求头处理阶段识别不同的 AI 编程客户端（Claude Code、Cursor、Cline、Continue、GitHub Copilot……），从每个客户端各自携带的、零散的会话标识 header 中，**确定性地**派生出一个统一的会话 header（默认 `X-AI-Session-Id`），供下游做会话统计、限流、审计、链路聚合等。

核心特性：

- **客户端分类匹配**：按客户端类型分别采集不同的 header 头，规则可配置。
- **统一且可自定义的输出 header 名**：默认 `X-AI-Session-Id`，可改。
- **可复现（确定性）的会话 ID 生成规则**：相同输入 header 永远产生相同的会话 ID（见下文「会话 ID 生成规则」）。
- **匹配失败兜底**：未识别到客户端 / 逐级提取均未命中时，打印当前全部请求头，便于排查。
- **日志可配置**：通过 `dump_unmatched` 开关控制是否在匹配失败时打印全量 header。

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`500`

## 处理流程

```
1. 若统一 header（session_header）已存在 → 幂等跳过，直接放行
2. 读取全部请求头，按 match_mode 选择的方案识别：
   - clients 方案：按 clients 规则匹配客户端
     2a. 未匹配任何客户端 → 匹配失败，打印全部请求头（dump_unmatched），放行
     2b. 匹配到客户端，但其 session_headers 有缺失 → 打印全部请求头（诊断），仍继续生成
   - header 方案：按 header_rules 列表逐级提取，命中第一个后取「匹配之后的内容」
     2c. 逐级均未命中 → 匹配失败，打印全部请求头（dump_unmatched），放行
3. 按「会话 ID 生成规则」计算唯一会话 ID，写入 session_header 并放行
```

全程不会 panic、不会阻断正常请求；任一环节出错都仅记录日志后放行。

## 会话 ID 生成规则（可复现）

会话 ID 是其输入的**纯函数**，因此可复现、可校验：

1. 按客户端配置的 `session_headers` **固定顺序**取出各 header 的值（缺失值以空串占位）。
2. 归一化：header 名小写、值 `trim`。
3. 拼接成规范串：`<name>|<h1>=<v1>|<h2>=<v2>|...`。
4. 对规范串做哈希（`fnv` 默认，或 `sha256`），取 16 位十六进制摘要。
5. 输出 `<clientName>-<hash16>`，例如 `claude-code-3f2a9c41b7d0e58a`。

> 说明：对鉴权类 header（如 `authorization` / `x-api-key`）做哈希是安全的——摘要不可逆，得到的是稳定、匿名的「按凭证维度」的会话 key。`session_headers` 的**顺序是契约的一部分**，改变顺序会改变生成的 ID。

**header 方案**下，会话 ID 同样确定性派生：逐级命中后得到 `(header 名, 提取值)`，规范串为 `<header>|<提取值>`，输出 `<header>-<hash16>`，例如 `authorization-3f2a9c41b7d0e58a`。

## 配置说明

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|------|----------|----------|--------|------|
| `session_header` | string | optional | `X-AI-Session-Id` | 统一输出的会话 header 名 |
| `hash_algorithm` | string | optional | `fnv` | 摘要算法，`fnv` 或 `sha256` |
| `match_mode` | string | optional | `clients` | 识别方案：`clients`（按客户端）或 `header`（按 header 逐级提取） |
| `clients` | array of client rule | optional | 内置默认规则 | `clients` 方案的客户端识别与会话头采集规则 |
| `header_rules` | array of header rule | `header` 方案必填 | - | `header` 方案的逐级提取规则列表 |
| `log` | log object | optional | - | 日志配置 |

### client rule（`match_mode: clients`）

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|------|----------|----------|--------|------|
| `name` | string | required | - | 客户端名，同时作为会话 ID 前缀 |
| `match_header` | string | optional | `user-agent` | 用于识别客户端的 header 名 |
| `match_pattern` | string | required | - | 在 `match_header` 值上匹配的 Go RE2 正则 |
| `session_headers` | array of string | required | - | 用于派生会话 ID 的 header 名**有序**列表 |

### header rule（`match_mode: header`）

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|------|----------|----------|--------|------|
| `header` | string | required | - | 要提取的 header 名 |
| `pattern` | string | optional | - | 在 header 值上匹配的 Go RE2 正则；命中后取**匹配结束位置之后**的子串；为空则取整个值 |

> 逐级语义：按列表顺序尝试每条规则，第一个「header 存在且提取值（trim 后）非空」的规则胜出；提取值为空则继续向后尝试。例如 `header: authorization, pattern: "(?i)^Bearer\\s+"`，值 `Bearer sk-abc123` 提取出 `sk-abc123`。

### log object

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|------|----------|----------|--------|------|
| `dump_unmatched` | bool | optional | `true` | 匹配失败（未识别到客户端 / 逐级提取未命中 / 缺失源 header）时是否打印全部请求头 |

## 内置默认客户端规则

未配置 `clients` 时使用下列内置规则（均可被配置覆盖）：

| 客户端 | 匹配（user-agent 正则） | session_headers（有序） |
|--------|--------------------------|--------------------------|
| `claude-code` | `(?i)claude` | `authorization`, `x-api-key`, `user-agent` |
| `cursor` | `(?i)cursor` | `authorization`, `x-cursor-checksum`, `user-agent` |
| `cline` | `(?i)cline` | `authorization`, `user-agent` |
| `continue` | `(?i)continue` | `authorization`, `user-agent` |
| `github-copilot` | `(?i)(githubcopilot|copilot|vscode)` | `authorization`, `x-request-id`, `user-agent` |

## 配置示例

### 最简配置（全部默认）

```yaml
{}
```

请求带 `user-agent: claude-cli/1.0.42` 时，会写入形如 `X-AI-Session-Id: claude-code-3f2a9c41b7d0e58a` 的头。

### 自定义 header 名 + sha256 + 自定义客户端

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

### header 方案（逐级提取，取匹配之后的内容）

```yaml
match_mode: "header"
header_rules:
  - header: "authorization"        # 优先用 Bearer Token 之后的内容
    pattern: "(?i)^Bearer\\s+"
  - header: "x-session-id"         # 兜底：直接取整个值
```

请求带 `authorization: Bearer sk-abc123` 时，会提取出 `sk-abc123` 并写入形如 `X-AI-Session-Id: authorization-3f2a9c41b7d0e58a` 的头。

### 显式会话头逐级提取（推荐）

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
  dump_unmatched: true   # 6 个头全都没有时，打印全部请求头便于排查
```

## 编译

```bash
cd plugins/wasm-go
PLUGIN_NAME=ai-header-session make build
# 产物：extensions/ai-header-session/plugin.wasm
```
