---
title: CSP 内容安全策略
keywords: [higress, csp, content security policy, 安全]
description: CSP 内容安全策略插件配置参考
---

## 功能说明

`csp` 插件用于为 HTTP 响应统一注入 [Content-Security-Policy（内容安全策略）](https://developer.mozilla.org/zh-CN/docs/Web/HTTP/CSP)响应头，可显著降低 XSS（跨站脚本）等注入类攻击的风险。

支持强制模式与仅上报（Report-Only）模式，可在强制现有策略的同时并行灰度更严格的候选策略，并可选择是否覆盖上游服务已设置的 CSP 响应头。

## 运行属性

插件执行阶段：`默认阶段`

插件执行优先级：`330`

## 配置字段

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
| -------- | -------- | -------- | -------- | -------- |
| policy | string | 必填 | - | CSP 策略字符串，例如 `default-src 'self'; img-src *` |
| report_only | bool | 选填 | false | 为 `true` 时使用 `Content-Security-Policy-Report-Only` 响应头，仅上报违规而不强制拦截，便于灰度验证策略 |
| report_only_policy | string | 选填 | - | 可选。设置后在强制策略之外额外注入一个 `Content-Security-Policy-Report-Only` 响应头，用于强制现有策略的同时灰度验证更严格的候选策略。不可与 `report_only: true` 同时使用 |
| override | bool | 选填 | true | 为 `true` 时先清除上游已设置的 CSP 响应头（强制与仅上报两种变体都会被清除）再注入本插件策略；为 `false` 时若响应中已存在**同名**（同一变体）的 CSP 响应头则保留原值，不注入该变体；上游仅设置了另一变体时，本插件仍会注入自己的变体 |

> 在 `report_only: true` 且 `override: true`（默认）时，插件会一并移除上游的强制 `Content-Security-Policy`，确保仅上报模式不会残留上游的强制拦截。

> `policy` 为空会导致插件配置校验失败，插件不会生效。

> 插件仅注入响应头，不校验 `policy` / `report_only_policy` 的 CSP 语法，策略字符串原样下发，请自行确保拼写正确。

> CSP 响应头会注入到所有匹配路由的响应上（浏览器仅对 HTML 文档生效，其余响应无害）。如需仅作用于特定路由，请通过路由级 matchRule 收窄插件生效范围。

## 配置示例

### 强制模式（默认）

```yaml
policy: "default-src 'self'; img-src *; script-src 'self'"
```

响应将携带：

```
Content-Security-Policy: default-src 'self'; img-src *; script-src 'self'
```

### 仅上报模式（灰度验证策略，不拦截）

```yaml
policy: "default-src 'self'; report-to csp-endpoint"
report_only: true
```

响应将携带：

```
Content-Security-Policy-Report-Only: default-src 'self'; report-to csp-endpoint
```

> 仅上报模式若不配置 `report-to` / `report-uri` 指令，违规信息只会出现在浏览器控制台，不会上报到收集端点。

### 强制现有策略 + 灰度验证更严格的新策略

CSP 上线的标准做法：保持当前策略强制生效，同时用 `Content-Security-Policy-Report-Only` 观察更严格的候选策略，验证无误后再切换为强制。

```yaml
policy: "default-src 'self'; img-src *"
report_only_policy: "default-src 'self'; img-src 'self'"
```

响应将同时携带：

```
Content-Security-Policy: default-src 'self'; img-src *
Content-Security-Policy-Report-Only: default-src 'self'; img-src 'self'
```

### 不覆盖上游已有的 CSP 响应头

```yaml
policy: "default-src 'self'"
override: false
```

若上游响应已包含**同名**（同一变体）的 CSP 响应头，则保留其原值、不注入该变体；否则注入本插件配置的策略。上游仅设置了另一变体时，本插件仍会注入自己的变体。
