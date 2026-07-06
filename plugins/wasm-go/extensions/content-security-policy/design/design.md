# Design: content-security-policy plugin

## Goal

Provide a Higress wasm-go plugin that injects the `Content-Security-Policy` (CSP)
response header so that gateway operators can centrally manage CSP for their
services. CSP is a defense-in-depth mitigation for XSS and data injection: the
browser restricts which resources a document may load based on the policy.

Resolves issue #1706.

## Non-goals

- Parsing or validating individual CSP directives (the browser is the authority;
  we treat each policy as an opaque string). Invalid directives are surfaced by
  the browser's console, not by the gateway.
- Generating nonces or hashes for `'nonce-...'` / `'sha256-...'` sources. Those
  require per-request collaboration with the backend and are out of scope for a
  header-injection plugin.

## Configuration schema

```jsonc
{
  "policies":   ["default-src 'self'", "script-src 'self' https://cdn.example.com"],
  "policy":     "default-src 'self'",          // convenience alias (single policy)
  "report_only": false                          // emit Content-Security-Policy-Report-Only instead
}
```

- `policies` (string array) and `policy` (string) are mutually-compatible aliases.
  If both are present, the entries are merged (single `policy` appended after the
  `policies` list). Blank / whitespace-only entries are dropped.
- At least one non-empty policy is required; otherwise plugin start fails. This
  matches the pattern used by `request-block` (reject configurations that would
  be no-ops so misconfigurations surface early).
- `report_only` selects the header name.

## Behavior

Implemented as a response-headers-only plugin (`ProcessResponseHeadersBy`):

1. Choose the header name: `Content-Security-Policy`, or
   `Content-Security-Policy-Report-Only` when `report_only` is true.
2. Remove any existing response header with that name (replace semantics — the
   upstream's own CSP should not leak through and weaken/conflict with the
   gateway-owned policy). This mirrors how the `cors` plugin strips upstream
   CORS headers before emitting its own.
3. Combine all configured policy blocks into a single header value, joining them
   with `"; "`. CSP directives are `;`-separated, so joining policy blocks with
   `"; "` yields one valid `Content-Security-Policy` header. This is the standard
   form (one header, multiple directives). (Multiple same-named CSP headers are
   also spec-valid with intersection semantics, but a single combined header is
   the common real-world form and avoids duplicate-header merging quirks in the
   proxy-wasm host ABI.)
4. Return `ActionContinue`.

The plugin does not register request or body hooks; it neither inspects nor
buffers the body.

## Header-name selection rationale

Keeping `report_only` purely a header-name switch (rather than, say, also
stripping the enforcing header) keeps the behavior predictable: an operator who
sets `report_only: true` gets *only* the report-only header added, and any
enforcing CSP that happens to exist upstream is left alone. This avoids
surprising loosening of an existing policy.

## Edge cases

- **No policy configured**: `parseConfig` returns an error; plugin start fails
  (`OnPluginStartStatusFailed`). Tested.
- **Blank/empty policy entries**: trimmed and dropped, never emitted as empty
  headers. Tested.
- **Upstream already set CSP**: removed then re-emitted with the configured
  policy. Tested.
- **Report-only mode**: only the report-only header is managed; the enforcing
  header is untouched. Tested.

## Test plan

Unit tests (`main_test.go`) use the `wasm-go/pkg/test` host harness, mirroring
`cors` and `request-block`:

- `TestParseConfig`: enforcing policies, report-only single `policy` field,
  blank-entry dropping, and the missing-policy failure case.
- `TestOnHttpResponseHeadersEnforcing`: both policy blocks combined into a single
  header value (`"default-src 'self'; script-src 'self' https://cdn.example.com"`);
  report-only header absent.
- `TestOnHttpResponseHeadersReportOnly`: report-only header emitted with the
  right value; enforcing header absent.
- `TestOnHttpResponseHeadersReplacesUpstreamHeader`: an upstream `default-src
  'unsafe-inline'` is replaced by the configured `default-src 'self'`.

Coverage target: ≥ 30% (CI-enforced for new wasm plugins); these cases exercise
every branch of `parseConfig` and `onHttpResponseHeaders`, so coverage should be
well above the threshold.

## Out of scope / future work

- Per-route policy generation with nonces/hashes for strict CSP (would need
  response-body rewriting or backend cooperation).
- Integration with the Reporting API (`report-to`) beyond passing the directive
  through in the policy string (already supported — it is just text).
