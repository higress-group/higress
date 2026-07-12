---
title: 内容安全策略
keywords: [higress, 内容安全策略, content security policy, csp]
description: 内容安全策略插件配置参考
---

## 功能说明
`csp` 插件用于在响应中添加 Content Security Policy（内容安全策略，CSP）相关的响应头。CSP 是一个额外的安全层，用于降低跨站脚本攻击（XSS）和数据注入攻击等风险。浏览器会根据 CSP 中声明的策略，限制页面可以加载的资源来源。

本插件支持两种 CSP 响应头：

- `Content-Security-Policy`：浏览器会强制执行该策略，违反策略的资源会被阻止加载。
- `Content-Security-Policy-Report-Only`：浏览器不会强制执行该策略，仅当资源违反策略时，向指定地址上报违规信息，便于在完全启用策略前进行观察和调试。

如果上游响应中已经存在同名的响应头，插件会先移除原有的头再写入配置的策略，保证网关配置的策略是最终生效的策略，避免出现多个相互冲突的 CSP 头。

## 运行属性

插件执行阶段：`认证阶段`
插件执行优先级：`400`

## 配置字段

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
| --- | --- | --- | --- | --- |
 | content_security_policy | string | 选填（与 `content_security_policy_report_only` 至少配置一项） | - | 配置 `Content-Security-Policy` 响应头的值，浏览器会强制执行该策略。 |
 | content_security_policy_report_only | string | 选填（与 `content_security_policy` 至少配置一项） | - | 配置 `Content-Security-Policy-Report-Only` 响应头的值，浏览器仅上报违规而不强制执行。 |

注意：`content_security_policy` 和 `content_security_policy_report_only` 至少需要配置一项，否则插件配置校验失败。

## 配置示例

1. 为响应添加强制执行的 CSP 头，只允许加载同源资源
```yaml
content_security_policy: "default-src 'self'"
```

根据该配置，响应头中将包含：
```
Content-Security-Policy: default-src 'self'
```

2. 添加报告模式的 CSP 头，违规上报到 `/csp-report`
```yaml
content_security_policy_report_only: "default-src 'self'; report-uri /csp-report"
```

根据该配置，响应头中将包含：
```
Content-Security-Policy-Report-Only: default-src 'self'; report-uri /csp-report
```

3. 同时配置强制执行和报告模式的 CSP 头
```yaml
content_security_policy: "default-src 'self'"
content_security_policy_report_only: "img-src 'self'; report-uri /csp-report"
```

根据该配置，响应头中将同时包含 `Content-Security-Policy` 和 `Content-Security-Policy-Report-Only` 两个头。

## 参考

- [Content Security Policy (CSP) - MDN](https://developer.mozilla.org/zh-CN/docs/Web/HTTP/CSP)
