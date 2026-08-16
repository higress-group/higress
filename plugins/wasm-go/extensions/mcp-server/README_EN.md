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

`protocolStrategy` describes only the upstream profile and is explicit in this milestone:

| Downstream | Upstream | Current status |
| --- | --- | --- |
| modern | registered / REST / composed | Supported |
| modern | `protocolStrategy: modern` | Supported as one stateless request per exchange |
| modern | `protocolStrategy: legacy` | Supported with an isolated legacy handshake inside one downstream exchange |
| legacy | legacy upstream | Existing behavior retained |
| legacy | modern-only upstream | Unsupported and deferred |

Outbound headers are rebuilt for every RPC. `Authorization` is generated or forwarded only through an explicit proxy authentication policy. Cookies, downstream sessions, `Last-Event-ID`, internal routing headers, and unrelated credentials are not forwarded by default. A well-formed but unrecognized `Mcp-Param-*` header is forwarded only for the current Tool RPC on a modern-to-modern path; it must not enter discover, initialize, or a legacy RPC.

### Migration and defaults

An existing `mcp-proxy` without `protocolStrategy` continues to default to `legacy`; its legacy downstream-to-legacy upstream initialize/session/transport path is unchanged. Set `modern` explicitly only after confirming that the upstream supports `2026-07-28`. This milestone does not auto-detect, fall back, or retry another profile, and runtime session IDs must not be placed in configuration or test evidence.

### Explicitly deferred scope

| Level | Capabilities outside this milestone |
| --- | --- |
| Deferred P1 | `protocolStrategy: auto`, the `2025-11-25` profile, full JSON Schema 2020-12 and output validation, large tools pagination/cursors, and the legacy downstream-to-modern-only bridge |
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
