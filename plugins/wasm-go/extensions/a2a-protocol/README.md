# A2A Protocol

The `a2a-protocol` plugin extracts bounded, trusted metadata from A2A 1.0
JSON-RPC requests and unary/SSE responses. It removes client-provided
`x-higress-a2a-*` metadata before publishing canonical values for later
Higress policy plugins. Optional compatibility recognizes A2A 0.3 method
aliases without translating payloads.

The upstream Agent remains authoritative for task state. The plugin never
copies Parts, artifacts, credentials, or callback URLs into headers or logs.
Normative A2A 1.0 JSON-RPC requests use `Content-Type: application/json`.
When an Agent Card interface declares `tenant`, the plugin preserves it and
publishes the value echoed in request parameters as trusted A2A metadata.

```yaml
protocolVersion: "1.0"
mode: enforce
legacy03:
  enabled: false
agent:
  id: weather-agent
  externalBaseURL: https://agents.example.com/a2a
agentCard:
  path: /.well-known/agent-card.json
  rewrite: true
  signatureMode: preserve
  maxResponseBytes: 262144
jsonrpc:
  maxRequestBytes: 4194304
  maxSSEEventBytes: 262144
authorization:
  exposeInternalHeaders: true
```

GET responses from the canonical Agent Card path and the legacy
`/.well-known/agent.json` path are validated before forwarding. The plugin
preserves unknown fields, advertises only the JSON-RPC 1.0 interface supported
by the route, and rewrites its declared endpoint to the explicitly trusted
`agent.externalBaseURL`. The plugin never derives this URL from request
authority or forwarding headers. Cards fail closed if this setting is absent,
if the response is compressed, or if a preserved endpoint does not exactly
match the configured public HTTPS endpoint.

Signed Cards with structurally valid signatures are passed through unchanged
in `preserve` mode only when they already advertise that configured endpoint.
Use `hgctl agent add --type a2a --a2a-external-base-url
https://agents.example.com/a2a --a2a-plugin-url
oci://registry.example.com/plugins/a2a-protocol:TAG ...` when publishing
through `hgctl`. The explicit plugin URL is required because the alpha image is
not published by this change and plugin selection belongs to the operator.
