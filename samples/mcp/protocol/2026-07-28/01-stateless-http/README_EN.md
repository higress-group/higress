# Demo 01: Stateless HTTP Tools

[中文](./README.md) | [Back to version index](../README_EN.md)

This demo proves that an MCP `2026-07-28` client can call `server/discover`, `tools/list`, and `tools/call` through independent HTTP requests without `initialize` or a protocol session.

## Prerequisites

From the Higress repository root:

```bash
cd samples/mcp
./protocol/2026-07-28/plugin/build.sh
./environment/scripts/up.sh
```

Enter this demo:

```bash
cd protocol/2026-07-28/01-stateless-http
export GATEWAY_URL=http://127.0.0.1:18080/mcp
export MCP_HOST=stateless.mcp.demo
```

## Step 1: Deploy

```bash
kubectl apply -f resources.yaml
kubectl get ingress -n mcp-demo mcp-2026-stateless-http
kubectl get wasmplugin -n higress-system mcp-2026-stateless-http
```

Wait a few seconds for the Higress Controller to push configuration to the Gateway.

## Step 2: Discover the server

```bash
curl -sS "$GATEWAY_URL" \
  -H "Host: $MCP_HOST" \
  -H "Origin: http://$MCP_HOST" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: server/discover' \
  -H 'X-Request-ID: demo-stateless-discover' \
  --data-binary @requests/discover.json | jq
```

Expect `resultType: complete`, tools-only capabilities, `ttlMs: 0`, `cacheScope: private`, and server name `stateless-demo`.

## Step 3: List tools

```bash
curl -sS "$GATEWAY_URL" \
  -H "Host: $MCP_HOST" \
  -H "Origin: http://$MCP_HOST" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: tools/list' \
  -H 'X-Request-ID: demo-stateless-list' \
  --data-binary @requests/list-tools.json | jq '.result'
```

The list should contain only `say_hello` and retain the complete-result and cache wire fields.

## Step 4: Call the tool and inspect session headers

```bash
curl -sS -D /tmp/mcp-stateless-headers \
  "$GATEWAY_URL" \
  -H "Host: $MCP_HOST" \
  -H "Origin: http://$MCP_HOST" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: tools/call' \
  -H 'Mcp-Name: say_hello' \
  -H 'X-Request-ID: demo-stateless-call' \
  --data-binary @requests/call-tool.json | jq
```

Expect `hello Higress`. The response must not contain an MCP session:

```bash
if grep -qi '^Mcp-Session-Id:' /tmp/mcp-stateless-headers; then
  echo 'unexpected MCP session'
else
  echo 'no MCP session returned'
fi
```

## Step 5: Inspect gateway evidence

```bash
kubectl logs -n higress-system deployment/higress-gateway --tail=200 |
  grep 'demo-stateless' || true
```

## Clean up

```bash
kubectl delete -f resources.yaml
```
