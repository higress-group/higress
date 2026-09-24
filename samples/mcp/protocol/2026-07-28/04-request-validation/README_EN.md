# Demo 04: Request Validation and Early Rejection

[中文](./README.md) | [Back to version index](../README_EN.md)

This demo proves that three invalid cases fail before the REST backend: missing Tool arguments, a hostile cross-origin request, and an `Mcp-Method` header that disagrees with the JSON-RPC body.

## Prerequisites

From the Higress repository root:

```bash
cd samples/mcp
./protocol/2026-07-28/plugin/build.sh
./environment/scripts/up.sh
cd protocol/2026-07-28/04-request-validation
export GATEWAY_URL=http://127.0.0.1:18080/mcp
export BACKEND_URL=http://127.0.0.1:18082
export MCP_HOST=validation.mcp.demo
kubectl apply -f resources.yaml
curl -sS -X POST "$BACKEND_URL/__reset" | jq
```

## Step 1: Missing required Tool argument

```bash
curl -sS -w '\nHTTP %{http_code}\n' "$GATEWAY_URL" \
  -H "Host: $MCP_HOST" \
  -H "Origin: http://$MCP_HOST" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: tools/call' \
  -H 'Mcp-Name: get_weather' \
  -H 'X-Request-ID: demo-validation-arguments' \
  --data-binary @requests/invalid-arguments.json
```

Expect HTTP `200` because this is a Tool execution result for a valid call envelope. The JSON-RPC Result should contain `isError: true` and explain that `location` is missing.

## Step 2: Hostile Origin

```bash
curl -sS -w '\nHTTP %{http_code}\n' "$GATEWAY_URL" \
  -H "Host: $MCP_HOST" \
  -H 'Origin: https://hostile.invalid' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: tools/list' \
  -H 'X-Request-ID: demo-validation-origin' \
  --data-binary @requests/list-tools.json
```

Expect HTTP `403` and `untrusted Origin`.

## Step 3: Header/body mismatch

The body calls `tools/list`, while the header declares `tools/call`:

```bash
curl -sS -w '\nHTTP %{http_code}\n' "$GATEWAY_URL" \
  -H "Host: $MCP_HOST" \
  -H "Origin: http://$MCP_HOST" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: tools/call' \
  -H 'X-Request-ID: demo-validation-mismatch' \
  --data-binary @requests/list-tools.json
```

Expect HTTP `400`, JSON-RPC error code `-32020`, and a message containing `header does not match request body`.

## Step 4: Prove the backend call count is zero

```bash
curl -sS "$BACKEND_URL/__state" | jq
curl -sS "$BACKEND_URL/__state" | jq -e '.events | length == 0'
```

The first case is a Tool Execution Error and the other two are transport-boundary errors, but all three must end before any backend call.

## Clean up

```bash
kubectl delete -f resources.yaml
```
