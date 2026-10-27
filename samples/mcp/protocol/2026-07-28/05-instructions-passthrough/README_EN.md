# Demo 05: Read Upstream Instructions with the Native SDK

[中文](./README.md) | [Back to version index](../README_EN.md)

The official Go SDK `v1.7.0` supplies both a native MCP upstream and the automatic client. The upstream uses `ServerOptions.Instructions` for deterministic multiline Chinese guidance and exposes `get_weather`. The client uses SDK Connect/ListTools/CallTool and compares the complete instructions string read from the SDK object. The SDK generates all MCP messages.

## Prerequisites

Use Go >=1.25, Python 3, curl, kubectl, Kind, Helm, and a running Docker or Podman engine. Optional Inspector commands require Node >=22.19.0. No real credentials or external business service is required.

From the repository root, build the pinned plugin and start the shared environment:

```bash
cd samples/mcp
./protocol/2026-07-28/plugin/build.sh
./environment/scripts/up.sh
cd protocol/2026-07-28/05-instructions-passthrough
export MCP_DEMO_EVIDENCE="$HOME/higress-instructions-evidence/$(date +%Y%m%d-%H%M%S)"
```

The default canonical Higress source is `1268f41d5de0ab07ecd9759c8217b7815dfc655e`. The shared plugin builder remains Go 1.24.4; the native fixture uses `golang:1.25.0-alpine` and `alpine:3.20`. Compiler differences can change the Wasm hash, so use the current build record. Re-run demos 01–04 when upgrading the source pin.

## Step 1: Run all four SDK phases

```bash
./verify.sh
```

The script builds and loads `localhost/mcp-demo/instructions-sdk:1.0.0`, deploys this case's ConfigMap, Deployment, Service, Ingress and WasmPlugin, starts its upstream port-forward, and runs four independent SDK clients. It reuses the public environment's engine and ownership checks.

There is one upstream. Instructions S come from [fixture/instructions.txt](./fixture/instructions.txt), including the trailing newline. Two gateway paths differ only by `server.passthroughInstructions`; no Host override or hosts-file edit is needed:

| Phase | SDK endpoint | SDK instructions | Actual upstream methods |
| --- | --- | --- | --- |
| direct | `http://127.0.0.1:18083/mcp` | Exact S | discover, list, call |
| on | `http://127.0.0.1:18080/instructions/on` | Exact S | discover, list, call |
| off | `http://127.0.0.1:18080/instructions/off` | Empty value; omitted from SDK JSON | list, call; no upstream discover |
| changed | Enabled path, fresh SDK client | Exact S' | discover, list, call |

These methods are `server/discover`, `tools/list` and `tools/call`. Every phase must negotiate `2026-07-28` and must never send a legacy `initialize`. The Go SDK stores modern discovery in an object named `InitializeResult`; its name does not imply a legacy handshake. Standalone SSE and subscriptions are disabled.

The changed phase applies [instructions-changed.txt](./fixture/instructions-changed.txt), rolls the same upstream deployment, then creates a fresh client so discovery state cannot be reused. On exit the script restores the case's previous configuration, or S on a first run, and stops its own port-forward. Resources stay deployed for manual repetition. See `completion.txt` and `restore.log` for restoration and process cleanup.

## Step 2: Inspect SDK and real proxy evidence

```bash
python3 -m json.tool "$MCP_DEMO_EVIDENCE/on.json"
python3 -m json.tool "$MCP_DEMO_EVIDENCE/off.json"
python3 -m json.tool "$MCP_DEMO_EVIDENCE/changed.json"
cat "$MCP_DEMO_EVIDENCE/completion.txt"
```

Each JSON contains `sdkResult`, `listResult`, `callResult` and `upstream.events` observed by SDK middleware. The automatic client requires:

- Complete string equality with the corresponding UTF-8 file, including whitespace and the final newline; the off phase requires an empty SDK instructions value.
- Negotiated `2026-07-28`; direct `serverInfo` is `native-instructions-upstream`, while gateway discovery remains `instructions-gateway`, version `1.0.0`.
- Exactly one listed `get_weather` tool, and a successful call returning `杭州：晴，25°C`.
- The exact upstream sequence and modern method/protocol headers, without legacy initialize. The off phase must not discover upstream.
- Correlated `demo05-<phase>-<n>` gateway records for on/off/changed. Ingress and WasmPlugin snapshots retain the actual routes.

Health, `GET /__state` and `POST /__reset` are auxiliary observation/reset endpoints; they do not implement MCP. The binary, SDK results, source/plugin/image identities, ConfigMap snapshots and checksums go to external `MCP_DEMO_EVIDENCE`. Do not commit them. `verify.sh` does not run Inspector.

## Step 3: Repeat an SDK call manually

After `verify.sh` restores the upstream configuration, restart this case's port-forward:

```bash
source ../../../environment/scripts/common.sh
start_port_forward instructions-upstream mcp-demo service/instructions-mcp 18083:8080
wait_http http://127.0.0.1:18083/healthz
"$MCP_DEMO_EVIDENCE/client" \
  --phase manual-on \
  --endpoint http://127.0.0.1:18080/instructions/on \
  --admin-url http://127.0.0.1:18083 \
  --instructions-file fixture/instructions.txt
```

For the off path add `--instructions-absent --expected-methods tools/list,tools/call`. For a direct connection use `/mcp` and `--server-name native-instructions-upstream`. Each invocation creates a new SDK client and resets the upstream ledger; avoid concurrent clients during counted verification.

To deploy separately, run `fixture/build.sh`; create the ConfigMap with `kubectl -n mcp-demo create configmap mcp-instructions-config --from-file=instructions.txt=fixture/instructions.txt --dry-run=client -o yaml | kubectl apply -f -`, then run `kubectl label -n mcp-demo configmap mcp-instructions-config mcp.higress.io/demo=instructions --overwrite`, `kubectl apply -f fixture/deployment.yaml` and `kubectl apply -f resources.yaml`. Wait for the deployment rollout, then use the SDK command above.

## Step 4: Optional manual Inspector 2.12.0 reproduction

Use the pinned official version with explicit Modern and HTTP. These paths are not named `/mcp`, so transport auto-detection is insufficient. Keep its writable catalog in the evidence directory:

```bash
node --version  # >=22.19.0
npx --yes @modelcontextprotocol/inspector@2.12.0 --cli \
  http://127.0.0.1:18080/instructions/on \
  --transport http --protocol-era modern \
  --catalog "$MCP_DEMO_EVIDENCE/inspector-catalog.json" \
  --method initialize --format json
npx --yes @modelcontextprotocol/inspector@2.12.0 --cli \
  http://127.0.0.1:18080/instructions/on \
  --transport http --protocol-era modern \
  --catalog "$MCP_DEMO_EVIDENCE/inspector-catalog.json" \
  --method tools/list --format json
npx --yes @modelcontextprotocol/inspector@2.12.0 --cli \
  http://127.0.0.1:18080/instructions/on \
  --transport http --protocol-era modern \
  --catalog "$MCP_DEMO_EVIDENCE/inspector-catalog.json" \
  --method tools/call --tool-name get_weather --tool-arg city=杭州
curl -fsS http://127.0.0.1:18083/__state | python3 -m json.tool
```

The CLI's `initialize` method is a connect-only probe. With explicit modern mode, inspect the real discovery instructions and tool results, then confirm the upstream ledger has no legacy `initialize`. These are optional manual commands, not evidence of execution by themselves. The automatic acceptance path uses the native Go SDK.

## Boundary and cleanup

This case demonstrates ordinary successful responses from one explicit-modern HTTP proxy. The gateway merges only a non-empty instructions string of at most 32768 decoded UTF-8 bytes, retains its own identity/capabilities/cache hints, and does not cache instructions. Guidance is text, does not authorize tools, and is not executed here. Error/SSE/authentication/invalid-field matrices belong to the feature tests.

```bash
source ../../../environment/scripts/common.sh
stop_port_forward instructions-upstream
kubectl delete -f resources.yaml
kubectl delete -f fixture/deployment.yaml
kubectl -n mcp-demo delete configmap mcp-instructions-config
# After all labs, run ./environment/scripts/down.sh from samples/mcp.
```
