---
title: Content Security Policy
keywords: [higress, content security policy, csp]
description: Content Security Policy plugin configuration reference
---

## Function Description
The `csp` plugin is used to add Content Security Policy (CSP) related response headers to responses. CSP is an added layer of security that helps mitigate cross-site scripting (XSS) and data injection attacks. Browsers enforce the policy declared in CSP to restrict the sources from which a page is allowed to load resources.

This plugin supports two CSP response headers:

- `Content-Security-Policy`: The browser enforces this policy. Resources that violate the policy are blocked from loading.
- `Content-Security-Policy-Report-Only`: The browser does not enforce this policy. Instead, it reports violations to the specified endpoint, which is useful for observation and debugging before fully enabling the policy.

If the upstream response already contains a header with the same name, the plugin removes the existing header before writing the configured policy. This guarantees that the policy configured on the gateway is the one that takes effect, avoiding multiple conflicting CSP headers.

## Runtime Attributes

Plugin execution phase: `Authentication Phase`
Plugin execution priority: `400`

## Configuration Fields

| Name | Data Type | Requirements | Default Value | Description |
| --- | --- | --- | --- | --- |
 | content_security_policy | string | Optional (at least one of this and `content_security_policy_report_only` must be configured) | - | The value of the `Content-Security-Policy` response header, which is enforced by the browser. |
 | content_security_policy_report_only | string | Optional (at least one of this and `content_security_policy` must be configured) | - | The value of the `Content-Security-Policy-Report-Only` response header. The browser only reports violations without enforcing the policy. |

Note: At least one of `content_security_policy` and `content_security_policy_report_only` must be configured; otherwise the plugin configuration validation fails.

## Configuration Example

1. Add an enforcing CSP header that only allows loading same-origin resources
```yaml
content_security_policy: "default-src 'self'"
```

With this configuration, the response header will contain:
```
Content-Security-Policy: default-src 'self'
```

2. Add a report-only CSP header that reports violations to `/csp-report`
```yaml
content_security_policy_report_only: "default-src 'self'; report-uri /csp-report"
```

With this configuration, the response header will contain:
```
Content-Security-Policy-Report-Only: default-src 'self'; report-uri /csp-report
```

3. Configure both enforcing and report-only CSP headers
```yaml
content_security_policy: "default-src 'self'"
content_security_policy_report_only: "img-src 'self'; report-uri /csp-report"
```

With this configuration, the response headers will contain both `Content-Security-Policy` and `Content-Security-Policy-Report-Only`.

## References

- [Content Security Policy (CSP) - MDN](https://developer.mozilla.org/en-US/docs/Web/HTTP/CSP)
