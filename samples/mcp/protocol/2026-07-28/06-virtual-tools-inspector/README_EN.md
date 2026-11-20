# Demo 06: Inspect a virtual tool set

[中文](./README.md) | [Back to the version demos](../README_EN.md)

Call two source servers' `echo` tools through one virtual MCP endpoint. Inspector uses MCP `2026-07-28` stateless HTTP. Higress runs `mcp-router` before `mcp-server` and dispatches each composed tool to a Legacy `2025-03-26` or Modern `2026-07-28` upstream.

## Run the verification

You need Docker with host networking, Go 1.24.4, Python 3.8+, Node.js 22.19.0+, and npm. The script builds both Wasm filters from the current checkout and starts a pinned Higress Envoy image and a local echo fixture. The demo provides its own gateway configuration.

From the repository root, run the following commands.

```bash
cd samples/mcp/protocol/2026-07-28/06-virtual-tools-inspector
python3 verify.py
```

The script runs eight operations against the live gateway using **MCP Inspector CLI 2.12.0**. Each uses `--transport http --protocol-era modern`.

| Path | Inspector operations | Expected result |
| --- | --- | --- |
| `/virtual` | Discover, list, call both tools | `capabilities.tools: {}`, with `legacy___echo` and `modern___echo` in the list. Calls return `legacy: hello` and `modern: hello`. |
| `/empty` | Discover, list | `capabilities.tools: {}` and `tools: []`. |
| `/disabled` | Discover | The router rule disables dispatch. Discovery omits `tools`. |
| `/unmatched` | Discover | The route matches no dispatch rule. Discovery omits `tools`. |

Inspector's CLI method called `initialize` runs the connection probe. For the Modern era it sends `server/discover`. The verifier checks the protocol version reported by Inspector and matches the four discoveries to Envoy access logs. The upstream capture shows `initialize`, `notifications/initialized`, and `tools/call` at the Legacy source. The Modern source receives only `tools/call`, with both `Mcp-Name` and `params.name` set to `echo`.

[plugins.json](./plugins.json) contains the dispatch rules and `servers` for the router and `toolSet.serverTools` for the server. Both sources implement an `echo` tool, and the virtual endpoint prefixes their names to distinguish them.

The terminal prints the evidence directory. Use `--evidence` to choose a new directory. It contains the Inspector JSON results, `gateway.log`, `upstream.json`, and `summary.json` with the source revision, Wasm hashes, and verification status. The script cleans up the demo gateway and fixture when it exits.

## Try the Inspector UI

Start the demo in one terminal, leave it running, and copy the Inspector URL it prints.

```bash
python3 verify.py --serve
```

In another terminal, start the Web Inspector with the printed URL.

```bash
MCP_URL='http://127.0.0.1:<port>/virtual'
npx --yes @modelcontextprotocol/inspector@2.12.0 --web \
  --server-url "$MCP_URL" --transport http --protocol-era modern
```

Open the page shown by Inspector, connect, and go to **Tools**. List the tools and call either `___echo` tool with `hello` as `message`. For the empty catalog, change the path from `/virtual` to `/empty`, reconnect, and list again. Press Ctrl+C to stop the gateway and Inspector.
