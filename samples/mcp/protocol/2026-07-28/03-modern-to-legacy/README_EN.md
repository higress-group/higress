# Demo 03: A Modern Client Calls a Legacy MCP Server

[中文](./README.md) | [Back to version index](../README_EN.md)

This demo proves that a modern stateless client can call an MCP `2025-03-26` server through Higress. Higress performs an isolated legacy handshake inside each downstream request, adapts the modern result contract, and prevents sensitive downstream headers from crossing the protocol boundary.

## Prerequisites

From the Higress repository root:

```bash
cd samples/mcp
./protocol/2026-07-28/plugin/build.sh
./environment/scripts/up.sh
cd protocol/2026-07-28/03-modern-to-legacy
export GATEWAY_URL=http://127.0.0.1:18080/mcp
export MCP_HOST=legacy-bridge.mcp.demo
```

## Step 1: Deploy the legacy fixture

```bash
kubectl -n mcp-demo create configmap legacy-mcp-fixture \
  --from-file=server.py=fixture/legacy_server.py \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f fixture/deployment.yaml
kubectl -n mcp-demo rollout status deployment/legacy-mcp --timeout=180s
kubectl apply -f resources.yaml
```

Reset its event ledger:

```bash
kubectl -n mcp-demo exec deployment/legacy-mcp -- python -c \
  'import urllib.request; urllib.request.urlopen(urllib.request.Request("http://127.0.0.1:8080/__reset", data=b"", method="POST"))'
```

## Step 2: List legacy tools with a modern request

```bash
curl -sS -D /tmp/mcp-bridge-list-headers \
  "$GATEWAY_URL" \
  -H "Host: $MCP_HOST" \
  -H "Origin: http://$MCP_HOST" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: tools/list' \
  -H 'Cookie: downstream-cookie=secret' \
  -H 'Mcp-Session-Id: downstream-session' \
  -H 'Last-Event-ID: downstream-event' \
  -H 'Authorization: Bearer downstream-not-policy' \
  -H 'x-unrelated-credential: should-not-pass' \
  -H 'Mcp-Param-Future: must-not-cross-era' \
  -H 'X-Request-ID: demo-bridge-list' \
  --data-binary @requests/list-tools.json | jq
```

Expect `proxy_echo`, `resultType: complete`, `ttlMs: 0`, and `cacheScope: private`. The response must not expose the upstream session:

```bash
grep -i '^Mcp-Session-Id:' /tmp/mcp-bridge-list-headers || echo 'upstream session is hidden'
```

## Step 3: Call the legacy Tool

```bash
curl -sS -D /tmp/mcp-bridge-call-headers \
  "$GATEWAY_URL" \
  -H "Host: $MCP_HOST" \
  -H "Origin: http://$MCP_HOST" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: tools/call' \
  -H 'Mcp-Name: proxy_echo' \
  -H 'Cookie: downstream-cookie=secret' \
  -H 'Mcp-Session-Id: downstream-session' \
  -H 'Last-Event-ID: downstream-event' \
  -H 'Authorization: Bearer downstream-not-policy' \
  -H 'x-unrelated-credential: should-not-pass' \
  -H 'Mcp-Param-Future: must-not-cross-era' \
  -H 'X-Request-ID: demo-bridge-call' \
  --data-binary @requests/call-tool.json | jq
```

Expect `echo:bridge`, `isError: false`, and `resultType: complete`.

## Step 4: Inspect the exact handshake sequence

```bash
kubectl -n mcp-demo exec deployment/legacy-mcp -- python -c \
  'import urllib.request; print(urllib.request.urlopen("http://127.0.0.1:8080/__state").read().decode())' |
  jq
```

The two downstream requests must produce exactly:

```text
initialize
notifications/initialized
tools/list
initialize
notifications/initialized
tools/call
```

`cookiePresent`, `lastEventIDPresent`, `authorizationPresent`, and `unrelatedCredentialPresent` must be `false`; `futureParam` must be `null`. The session created by the legacy handshake remains upstream-only.

## Clean up

```bash
kubectl delete -f resources.yaml
kubectl delete -f fixture/deployment.yaml
kubectl -n mcp-demo delete configmap legacy-mcp-fixture
```
