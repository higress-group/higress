---
title: CSP Content Security Policy
keywords: [higress, csp, content security policy, security]
description: CSP Content Security Policy plugin configuration reference
---

## Description

The `csp` plugin injects a [Content-Security-Policy](https://developer.mozilla.org/en-US/docs/Web/HTTP/CSP) response header into HTTP responses, which significantly reduces the risk of XSS (cross-site scripting) and other injection attacks.

It supports both enforcing mode and report-only mode, can validate a stricter candidate policy in parallel while the current one stays enforced, and can optionally preserve a CSP header already set by the upstream service.

## Runtime Properties

Plugin execution phase: `default phase`

Plugin execution priority: `330`

## Configuration Fields

| Name | Type | Requirement | Default | Description |
| -------- | -------- | -------- | -------- | -------- |
| policy | string | Required | - | The CSP directive string, e.g. `default-src 'self'; img-src *` |
| report_only | bool | Optional | false | When `true`, uses the `Content-Security-Policy-Report-Only` header so violations are reported but not enforced, which is useful for validating a policy before rollout |
| report_only_policy | string | Optional | - | Optional. When set, injects an extra `Content-Security-Policy-Report-Only` header alongside the enforced policy, so a stricter candidate policy can be validated while the current one stays enforced. Cannot be combined with `report_only: true` |
| override | bool | Optional | true | When `true`, removes any CSP header already set by the upstream (both the enforcing and the report-only variant) before injecting the configured policy; when `false`, keeps an existing upstream header of the **same variant** untouched, while still injecting its own variant if the upstream set only the other one |

> When `report_only: true` and `override: true` (default), the plugin also strips the upstream's enforcing `Content-Security-Policy`, so report-only mode never leaves an upstream enforcing policy in effect.

> An empty `policy` fails the plugin configuration validation and the plugin will not take effect.

> The plugin only injects headers; it does not validate the CSP syntax of `policy` / `report_only_policy`. The strings are emitted as-is, so make sure they are spelled correctly.

> The CSP header is injected on every response of the matched routes (browsers only apply it to HTML documents; it is harmless on other responses). To scope it to specific routes, narrow the plugin with a route-level matchRule.

## Configuration Examples

### Enforcing mode (default)

```yaml
policy: "default-src 'self'; img-src *; script-src 'self'"
```

The response will carry:

```
Content-Security-Policy: default-src 'self'; img-src *; script-src 'self'
```

### Report-only mode (validate a policy without blocking)

```yaml
policy: "default-src 'self'; report-to csp-endpoint"
report_only: true
```

The response will carry:

```
Content-Security-Policy-Report-Only: default-src 'self'; report-to csp-endpoint
```

> Without a `report-to` / `report-uri` directive, report-only violations only appear in the browser console and are not sent to a collection endpoint.

### Enforce the current policy while validating a stricter one

The standard CSP rollout: keep the current policy enforced while observing a stricter candidate via `Content-Security-Policy-Report-Only`, then switch it to enforcing once validated.

```yaml
policy: "default-src 'self'; img-src *"
report_only_policy: "default-src 'self'; img-src 'self'"
```

The response will carry both:

```
Content-Security-Policy: default-src 'self'; img-src *
Content-Security-Policy-Report-Only: default-src 'self'; img-src 'self'
```

### Keep the upstream CSP header if present

```yaml
policy: "default-src 'self'"
override: false
```

If the upstream response already contains a CSP header of the **same variant**, its value is preserved and that variant is not injected; otherwise the configured policy is injected. If the upstream set only the other variant, the plugin still injects its own.
