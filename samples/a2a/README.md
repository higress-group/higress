# A2A protocol-aware route

`quickstart.yaml` shows the existing Service, Ingress, and WasmPlugin resources
needed to publish one A2A 1.0 JSON-RPC Agent. Replace the public and upstream
host names and replace `REGISTRY/REPOSITORY:TAG` with an operator-approved,
published `a2a-protocol` image before applying it:

```shell
kubectl apply -f samples/a2a/quickstart.yaml
```

The plugin is attached through an operator-managed WasmPlugin. Ingress authors
cannot choose plugin code, Redis destinations, or credentials. `hgctl agent add
--type a2a` follows the same boundary and requires `--a2a-plugin-url` with an
explicit tagged OCI image.

Clients send A2A 1.0 JSON-RPC requests with `Content-Type: application/json`
and `A2A-Version: 1.0`. The plugin recognizes the canonical 1.0 methods,
extracts bounded protocol metadata, and removes spoofed `x-higress-a2a-*`
headers before publishing trusted values. Slash-style 0.3 aliases are accepted
only when `legacy03.enabled` is explicitly set.

Agent Card responses at `/.well-known/agent-card.json` are bounded and
validated. The preference-ordered `supportedInterfaces` list is reduced to the
JSON-RPC 1.0 interfaces this route supports; URLs are rewritten to the trusted
`agent.externalBaseURL`, and an optional interface `tenant` is preserved for
clients to echo in requests. Signed Cards are preserved only when their URL
already equals the configured public URL.

The upstream Agent remains responsible for task state, task ownership, and
callback delivery. This first change does not add endpoint affinity or accept
Redis configuration from Ingress annotations.
