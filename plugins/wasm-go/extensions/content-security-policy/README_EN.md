# Introduction

The `content-security-policy` plugin automatically adds the `Content-Security-Policy` (CSP) response header to responses. CSP is an additional defense-in-depth layer that greatly reduces the risk of Cross-Site Scripting (XSS) and data-injection attacks by letting the browser restrict which resources (scripts, styles, images, fonts, APIs, ...) a page may load.

The plugin supports both *enforce* and *report-only* modes, and allows multiple policies to be delivered at once.

# Configuration

| Name | Type | Required |  Default | Description |
| -------- | -------- | -------- | -------- | -------- |
|  policies     |  array of string     | Optional, at least one of `policies`/`policy` is required     |   -  |  A list of CSP policy strings. The entries are joined with `; ` into a single `Content-Security-Policy` header, making it convenient to split a policy into multiple directive blocks. |
|  policy     |  string     | Optional, at least one of `policies`/`policy` is required     |   -  |  A single CSP policy string. Equivalent to a one-element `policies`, provided as a convenience for simple cases. |
|  report_only     |  bool     | Optional     |   false  |  When `true`, the `Content-Security-Policy-Report-Only` header is emitted instead, so violations are reported but not blocked. Useful for safely rolling out a policy. |

# Examples

## Basic enforcing policy
```yaml
policy: "default-src 'self'; script-src 'self' https://cdn.example.com"
```
The response will carry `Content-Security-Policy: default-src 'self'; script-src 'self' https://cdn.example.com`.

## Multiple directive blocks (combined into one header)
```yaml
policies:
- "default-src 'self'"
- "script-src 'self' https://cdn.example.com"
```
The entries are joined with `; ` and delivered as a single response header: `Content-Security-Policy: default-src 'self'; script-src 'self' https://cdn.example.com`.

## Report-only mode (safe rollout)
```yaml
policy: "default-src 'self'; report-uri /csp-report"
report_only: true
```
The response will carry `Content-Security-Policy-Report-Only: ...`; violations are reported but not blocked, which helps validate a policy without affecting traffic.

## Enable for specific routes or domains
```yaml
# Fine-grained rules via the _rules_ field
_rules_:
# Rule 1: match by route name
- _match_route_:
  - route-a
  - route-b
  policy: "default-src 'self'"
# Rule 2: match by domain
- _match_domain_:
  - "*.example.com"
  - test.com
  policies:
  - "default-src 'self'"
  - "img-src 'self' data:"
  report_only: true
```
`route-a` and `route-b` in `_match_route_` are the route names configured when creating the gateway routes; when matched, this rule's config takes effect.
The domains in `_match_domain_` (`*.example.com` and `test.com`) match the request host; when matched, this rule's config takes effect.
Rules are evaluated in order; the first match wins and later rules are ignored.

# Behavior notes

- The plugin uses *replace* semantics: if the upstream already set a CSP header with the same name, it is removed first and then re-emitted with the gateway-owned policy, so the gateway-side policy is authoritative (consistent with the `cors` plugin).
- When `report_only` is `true`, only the `Content-Security-Policy-Report-Only` header is managed; the enforcing `Content-Security-Policy` header is left untouched.
