---
title: AI Anthropic System Fold
keywords: [higress, ai, anthropic, claude code]
description: 将 Anthropic Messages API 中内联的 `role: system` 消息折叠进顶层 `system` 字段。
---

## 功能说明

在 Anthropic Messages API 端点(`/v1/messages`)上,本插件把 `messages` 数组里内联的
`role: "system"` 消息折叠进顶层 `system` 字段,并将其从 `messages` 中移除。

这是为了兼容那些 Anthropic 协议层严格校验 `messages[*].role` 只能为 `user` / `assistant`
的后端(如部分 vLLM / SGLang 版本)——否则这类请求会在校验阶段被拒:

```
400 1 validation error:
{'type': 'literal_error', 'loc': ('body', 'messages', N, 'role'),
 'msg': "Input should be 'user' or 'assistant'", 'input': 'system'}
```

部分客户端——尤其是 **Claude Code CLI(>= 2.1.154)**——会把系统提示以
`{"role": "system", ...}` 的形式放进 `messages` 数组,而不是放在 Anthropic 规范的顶层
`system` 字段里。把它们折叠进 `system` 是符合规范的标准归一化做法(vLLM、SGLang 等后端
也采用同样的处理方式)。

## 行为

- 仅作用于 Anthropic `/messages` 端点;明确**排除** OpenAI `/chat/completions`
  (OpenAI 协议本就允许 `messages` 里带 `system`)。
- 移除 `messages` 中所有 `role: "system"` 条目,并把其文本追加进顶层 `system`:
  - 已有的顶层 `system` 内容保留在前,折叠内容追加在后;
  - 若 `system` 为字符串(或不存在),结果为字符串;
  - 若 `system` 为内容块数组,则追加一个 `{"type":"text","text":...}` 块。
- 同时支持字符串内容与内容块(`[{"type":"text","text":...}]`)形式的消息内容。
- `messages` 中没有任何 `role: "system"` 消息的请求原样放行。
- 其它非标准角色不处理,仍由后端的严格校验拒绝。

## 配置项

本插件无需配置。

| 名称 | 数据类型 | 必填 | 默认值 | 描述 |
| ---- | -------- | ---- | ------ | ---- |
| -    | -        | -    | -      | 无需配置 |

## 示例

发往 `/v1/messages` 的请求:

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

经插件处理后,转发给后端的请求变为:

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
