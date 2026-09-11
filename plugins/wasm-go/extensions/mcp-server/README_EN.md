# mcp-server

[中文](./README.md)

## Overview

`mcp-server` hosts MCP tool servers in the gateway. The current binary includes Quark Search and Amap tools, and it can also expose configured REST tools or proxy an upstream MCP server. MCP server plugins require Higress 2.1.0 or later.

## MCP 2026 Tools baseline

The plugin retains its legacy profile and adds the stateless HTTP Tools baseline from MCP `2026-07-28`:

| Profile | Exact supported versions | Lifecycle and transport |
| --- | --- | --- |
| legacy | `2024-11-05`, `2025-03-26`, `2025-06-18` | Retains `initialize` / `notifications/initialized` and the existing session and HTTP/SSE compatibility behavior |
| modern | `2026-07-28` | Per-request `_meta`, no initialize and no protocol session; no GET/DELETE/Last-Event-ID recovery |

The current modern profile implements only `server/discover`, `tools/list`, and `tools/call`. It validates Content-Type, Accept, same-origin Origin, single-message JSON-RPC boundaries, mirrored identity headers, and resource bounds. Batches, response envelopes, and trailing JSON are rejected.

A modern request must carry both the transport headers and request metadata:

```http
MCP-Protocol-Version: 2026-07-28
Mcp-Method: tools/call
Mcp-Name: get_weather
Content-Type: application/json
Accept: application/json, text/event-stream
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "_meta": {
      "io.modelcontextprotocol/protocolVersion": "2026-07-28",
      "io.modelcontextprotocol/clientInfo": {"name": "example", "version": "1.0.0"},
      "io.modelcontextprotocol/clientCapabilities": {}
    },
    "name": "get_weather",
    "arguments": {"location": "Hangzhou"}
  }
}
```

### Capabilities and result contract

- `server/discover` advertises only the effective `tools: {}` capability. It does not advertise `tools.listChanged`, MRTR, subscriptions, resources, prompts, or any other unimplemented capability.
- Every successful modern result uses `resultType: complete` and carries server identity in `_meta.io.modelcontextprotocol/serverInfo`.
- `server/discover` and `tools/list` additionally return `ttlMs: 0` and `cacheScope: private`. These are wire-contract fields only: this milestone has no response/descriptor cache engine, shared cache, or active invalidation.

### Proxy profile matrix

`protocolStrategy` selects the upstream behavior; omitted, empty and explicit `legacy` retain existing behavior:

| Downstream | Upstream | Current status |
| --- | --- | --- |
| modern | registered / REST / composed | Supported |
| modern | `protocolStrategy: modern` | Supported as one stateless request per exchange |
| modern | `protocolStrategy: legacy` | Supported with an isolated legacy handshake inside one downstream exchange |
| modern | `auto` + `http` | Discover before each forwarded Tool request, then modern or one legacy handshake |
| legacy | `auto` + `http` | Existing legacy path, no modern probe |
| Any supported downstream | `auto` + `sse` | Existing legacy SSE path; no transport or URL guessing |
| legacy | legacy upstream | Existing behavior retained |
| legacy | modern-only upstream | Unsupported and deferred |

Outbound headers are rebuilt for every RPC. `Authorization` is generated or forwarded only through an explicit proxy authentication policy. Cookies, downstream sessions, `Last-Event-ID`, internal routing headers, and unrelated credentials are not forwarded by default. A well-formed but unrecognized `Mcp-Param-*` header is forwarded only for the current Tool RPC on a modern-to-modern path; it must not enter discover, initialize, or a legacy RPC.

### Migration and defaults

Existing configurations need no migration and still default to `legacy`. A known modern upstream can use explicit `modern` to avoid discovery overhead. Enable `auto` explicitly for request-scoped detection; it does not change the downstream protocol. Before rolling back to an older plugin that rejects auto, switch the configuration to `legacy` and confirm it is effective, then roll back the image. Never put runtime session IDs in configuration or test credentials.

### Opt-in auto detection

```yaml
server:
  name: upstream-tools
  type: mcp-proxy
  transport: http
  protocolStrategy: auto
  mcpServerURL: https://mcp.example.com/mcp
  timeout: 5000
  autoDetection:
    probeTimeoutMs: 1000
```

`autoDetection.probeTimeoutMs` is consumed only by `auto + http`. It defaults to 1000 ms and must be a positive uint32 integer. The probe uses the smaller of this value and the effective timeout. This section alone does not enable auto. The existing timeout still applies per callout, so total request latency may exceed it.

Each forwarded tools/list or tools/call completes existing authorization checks and prepares effective upstream credentials once before discovering. Modern success makes 2 calls: discover → Tool RPC. Legacy success makes 4: discover → initialize → initialized → Tool RPC. There is no cache, cross-request session reuse or coalescing, and a tool call does not require a prior list. Downstream discover remains local to the gateway.

| Probe outcome | Action |
| --- | --- |
| Valid discovery supports 2026-07-28 and Tools | Modern |
| -32022 explicitly advertises compatible 2025-06-18 / 2025-03-26 | Prefer 2025-06-18 and complete one HTTP legacy handshake |
| HTTP 200 matching-ID -32601 without modern evidence; ordinary 400/404/405 without a modern error | Try one 2025-03-26 initialize |
| Modern errors, valid HTTP 404 -32601, 401/403/429, 5xx, network failure or timeout | Stop without downgrade; retain applicable authentication challenges and Retry-After |
| Wrong ID, malformed JSON/SSE, batch, trailing JSON, or probe body over 1 MiB | Stop; parsing failure never guesses legacy |

The auto handshake validates version, Tools capability, serverInfo and the initialized acknowledgement. A session belongs only to that request. Business errors, disconnects or version changes never trigger replay. Modern requestState/inputResponses cannot be silently converted to legacy; cursor errors never restart pagination.

Credentials must permit discover. Fixed, tool-level and passthrough authentication apply to every phase of the same request. Explicit Cookie authentication can use `apiKey` with `in: header, name: Cookie`; the existing `in: cookie` setting is unsupported. Probes carry no Mcp-Name/Mcp-Param headers; auto forwards applicable parameter headers only on actual modern tools/call requests.

Upstream pool instances must have consistent protocol capabilities because probe and business calls may reach different instances. JSON/SSE is parsed after the complete callout response, without live progress or subscriptions. The 1 MiB probe guard prevents copying oversized bodies into Wasm; it does not bound all Envoy receive buffers or Tool results. Cancellation suppresses later phases and callbacks but cannot retract a submitted hostcall. Retry policies in other layers are outside this plugin's single-dispatch guarantee. The final RPC retains the existing forwarding path; [#4597](https://github.com/higress-group/higress/issues/4597) is independent.

See the separate [auto.yaml example](../../../../samples/mcp/protocol/2026-07-28/auto.yaml).

### Explicitly deferred scope

| Level | Capabilities outside this milestone |
| --- | --- |
| Deferred P1 | The `2025-11-25` profile, full JSON Schema 2020-12 and output validation, large tools pagination/cursors, and the legacy downstream-to-modern-only bridge |
| Deferred P2 | MRTR / generation of `input_required`, subscriptions/listen and `tools.listChanged`, and generic state/requestState TTL, persistence, and recovery |
| Separate Proposal | Complete OAuth resource server/client behavior, Tasks, MCP Apps, Resources, Prompts, Completion, and protocol synchronization for the independent native `plugins/golang-filter/mcp-server` |

## Configuration

The compiled-in Quark and Amap servers normally need no additional
`defaultConfig`; their tools and authentication are defined by their server
implementations. To customize the service version returned by MCP `initialize`,
set `server.version`; it defaults to `1.0.0`. See the
[MCP server development guide](../../mcp-servers/README.md).

### REST server example

```yaml
server:
  name: weather
  type: rest
tools:
  - name: get_weather
    description: Query weather
    args:
      - name: location
        type: string
        required: true
    responseTemplate:
      body: "weather for {{.args.location}}"
```

### Custom server version example

```yaml
# Customize serverInfo.version in the initialize response.
server:
  name: quark-search
  version: 2.5.0
```

### MCP proxy example

```yaml
server:
  name: upstream-tools
  type: mcp-proxy
  transport: http
  protocolStrategy: modern # or legacy
  mcpServerURL: https://mcp.example.com/mcp
```

## WasmPlugin resource

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: mcp-server
  namespace: higress-system
spec:
  selector:
    matchLabels:
      higress: higress-system-higress-gateway
  url: oci://higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/mcp-server:<version>
```

## Reproducible verification

Official examples are pinned to `modelcontextprotocol/modelcontextprotocol@f817239f4d6b1efff2c4dfc2f7af85c985d73076`. The official clients are locked to Go SDK `v1.7.0` and TypeScript client `2.0.0`; tests never fetch a moving `latest` version.

```bash
# Unit, official-example, and compatibility coverage
go test -count=1 ./...

# Go 1.25+ and Node.js 20+; exercises discover/list/call on all three modes
./testdata/interop/run.sh

# Explicit e2e WASM build, independent of VERSION/-alpha scanning
make -C ../../../.. build-mcp-server-wasmplugin
```

The complete kind/Envoy e2e requires Docker, kind, kubectl, and Helm:

```bash
PLUGIN_TYPE=GO PLUGIN_NAME=mcp-server TEST_SHORTNAME=WasmPluginsMCP20260728 make higress-wasmplugin-test
```

Local TestHost verification does not replace this real data-plane step; CI runs it. `plugin.wasm` is a build artifact and must not be committed.

## Related documentation

- [MCP quick start](https://higress.cn/en/ai/mcp-quick-start/)
- [MCP server development guide](../../mcp-servers/README.md)
- [Wasm plugin marketplace](https://higress.cn/en/plugin/)
