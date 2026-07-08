# CSP Plugin Design

> Design document for the `csp` wasm-go plugin, implementing [#1706](https://github.com/alibaba/higress/issues/1706).
> This document was provided to / produced with an AI coding tool (Claude Code), as required by the
> "Special Requirements for AI Coding Tool Usage" section in CONTRIBUTING_EN.md.

## 1. Problem

Issue #1706 requests a plugin that helps add the `Content-Security-Policy` (CSP) response header,
so that gateway operators can enforce a browser-side content security policy centrally at the
gateway, greatly reducing the risk of XSS attacks — without requiring every upstream application
to implement it.

## 2. Goals / Non-Goals

**Goals**

- Inject a configurable `Content-Security-Policy` header into HTTP responses.
- Support `Content-Security-Policy-Report-Only` mode so a policy can be validated (violations
  reported) before being enforced.
- Allow operators to choose whether the gateway overrides a CSP header already set by the
  upstream, or preserves it.
- Fail fast on invalid configuration (empty policy) so misconfiguration is visible at config
  time, not silently at runtime.

**Non-Goals**

- Parsing or validating CSP directive syntax (the policy string is passed through verbatim;
  directive semantics are the operator's responsibility).
- `report-to` / reporting endpoint management.
- Per-content-type or per-path conditional policies (can be achieved via Higress route-level
  plugin configuration instead).

## 3. Design

### 3.1 Plugin shape

A minimal wasm-go plugin using the `higress-group/wasm-go` wrapper SDK. Only two hooks are
registered — CSP only affects response headers, so no request-phase or body-phase hooks are
needed, and request/response bodies are never read (zero memory overhead):

```
wrapper.SetCtx("csp",
    wrapper.ParseConfig(parseConfig),                 // config JSON -> PluginConfig
    wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
)
```

### 3.2 Configuration model

| Field | Type | Required | Default | Meaning |
|---|---|---|---|---|
| `policy` | string | yes | - | CSP directive string, e.g. `default-src 'self'` |
| `report_only` | bool | no | false | Use `Content-Security-Policy-Report-Only` instead of the enforcing header |
| `report_only_policy` | string | no | - | Extra report-only candidate policy injected alongside the enforced one; mutually exclusive with `report_only: true` |
| `override` | bool | no | true | Overwrite upstream-provided CSP headers, or keep an existing header of the same variant if present |

Decisions:

- **`policy` is required and non-empty** (validated in `parseConfig`, returning an error fails
  plugin start). A CSP plugin without a policy is meaningless; failing fast surfaces the
  misconfiguration immediately.
- **`override` defaults to `true`** — the common intent of installing this plugin is "the gateway
  decides the policy". Operators who trust upstream-set policies can opt out with
  `override: false`. An explicit JSON `null` for `override` (e.g. a YAML `override:` empty
  scalar) is treated the same as an absent field, preserving the default — gjson reports
  `Exists()==true` for a null value, and naively calling `Bool()` on it would silently flip the
  default to `false`.
- **`report_only` maps to a different header name** rather than a separate plugin/field pair,
  mirroring how the two headers work in the CSP spec itself.
- **`report_only_policy` enables the standard CSP rollout pattern** — keep the current policy
  enforced while observing a stricter candidate via the report-only header, then promote it.
  It is rejected in combination with `report_only: true` (validated in `parseConfig`): in that
  mode the primary policy already uses the report-only header, so a second candidate would be
  ambiguous.

### 3.3 Response-phase logic

```
out := [(report_only ? report-only-header : enforcing-header, policy)]
if !report_only && report_only_policy != "":
    out += (report-only-header, report_only_policy)
if override:
    RemoveHttpResponseHeader("Content-Security-Policy")              // drop both variants,
    RemoveHttpResponseHeader("Content-Security-Policy-Report-Only")  // all values
for (name, value) in out:
    if !override && upstream already has a `name` header: skip this variant
    AddHttpResponseHeader(name, value)
return ActionContinue
```

With `override: true`, both CSP header variants are removed first: `Remove` clears *all* values
for a name, while `Replace` would only touch the first, so a duplicated upstream header or the
opposite (enforce vs report-only) variant could otherwise leak through. Removing both variants
also makes report-only mode truly non-enforcing even when the upstream sent an enforcing policy.
Each configured header is then added exactly once, so no duplicate CSP headers are produced
(browsers combine duplicates with intersection semantics, making the effective policy stricter
than intended).

With `override: false`, back-off is per variant: an upstream header of the same variant is kept
and that variant is not injected, while the other configured variant is still added. Presence is
keyed on the header-lookup error alone — only a confirmed not-found allows injection; any other
outcome (including a present-but-empty header, which real Envoy returns as Ok + empty value)
conservatively counts as present, so the plugin never adds a duplicate next to a header it did
not positively confirm absent. The wasm-go test host differs from Envoy here: it surfaces
empty-value headers as not-found, which `TestOverrideFalseEmptyValueTreatedAbsent` documents.

## 4. Testing

Unit tests use the SDK test host (`wasm-go/pkg/test`), which runs each case in both native-Go
and compiled-wasm modes without needing Envoy/K8s:

| Case | Asserts |
|---|---|
| `TestSetsCSPHeader` | header injected with configured value |
| `TestReportOnlyHeader` | report-only header used; enforcing header absent |
| `TestReportOnlyRemovesUpstreamEnforce` | upstream enforcing header dropped in report-only mode with `override: true` |
| `TestOverrideReplacesUpstreamEnforce` | upstream header replaced by configured policy with `override: true` |
| `TestOverrideFalseKeepsExisting` | upstream header preserved when `override: false` |
| `TestOverrideFalseEmptyValueTreatedAbsent` | mock host surfaces empty-value headers as not-found, so the policy is injected (real Envoy keeps them) |
| `TestOverrideFalseAddsWhenAbsent` | policy injected when no upstream CSP header exists and `override: false` |
| `TestOverrideFalseInjectsOwnVariant` | with `override: false`, an upstream header of the other variant does not block injection |
| `TestOverrideFalseDualPolicyYieldsPerVariant` | dual policy with `override: false`: same-variant upstream header kept, report-only candidate still injected |
| `TestOverrideNullDefaultsTrue` | explicit JSON `null` for `override` falls back to the default (`true`) |
| `TestDualPolicyEmitsBoth` | `report_only_policy` injects the report-only candidate alongside the enforced policy |
| `TestReportOnlyPolicyConflictRejected` | plugin start fails when `report_only_policy` is combined with `report_only: true` |
| `TestEmptyPolicyRejected` | plugin start fails on empty policy |

## 5. Alternatives considered

- **Using the `transformer` plugin to add the header**: possible, but a dedicated plugin gives
  first-class semantics (report-only mode, override control, config validation) and a
  discoverable name for a common security need, which is what #1706 asks for.
- **Multiple policy entries / per-directive config**: rejected for v1 — a single opaque policy
  string matches how CSP is authored and audited in practice, and keeps config obvious.
