# Demo 02: REST-to-MCP

[中文](./README.md) | [Back to version index](../README_EN.md)

This demo proves that Higress translates one MCP `tools/call` into one ordinary REST request, with a protocol-neutral backend ledger proving that no duplicate call occurred.

## Prerequisites

From the Higress repository root:

```bash
cd samples/mcp
./protocol/2026-07-28/plugin/build.sh
./environment/scripts/up.sh
cd protocol/2026-07-28/02-rest-to-mcp
export GATEWAY_URL=http://127.0.0.1:18080/mcp
export BACKEND_URL=http://127.0.0.1:18082
export MCP_HOST=rest-to-mcp.mcp.demo
```

## Step 1: Deploy and reset the backend

```bash
kubectl apply -f resources.yaml
curl -sS -X POST "$BACKEND_URL/__reset" | jq
```

## Step 2: Inspect the published REST Tool

```bash
curl -sS "$GATEWAY_URL" \
  -H "Host: $MCP_HOST" \
  -H "Origin: http://$MCP_HOST" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: tools/list' \
  -H 'X-Request-ID: demo-rest-list' \
  --data-binary @requests/list-tools.json | jq '.result.tools'
```

Expect `get_weather` with required argument `location`.

## Step 3: Call the Tool

```bash
curl -sS "$GATEWAY_URL" \
  -H "Host: $MCP_HOST" \
  -H "Origin: http://$MCP_HOST" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: tools/call' \
  -H 'Mcp-Name: get_weather' \
  -H 'X-Request-ID: demo-rest-call' \
  --data-binary @requests/call-weather.json | jq
```

The response should contain `weather for Hangzhou` and `resultType: complete`.

## Step 4: Prove there was exactly one REST call

```bash
curl -sS "$BACKEND_URL/__state" | jq
curl -sS "$BACKEND_URL/__state" | jq -e '.events | length == 1'
```

The only event should be `GET /weather?location=Hangzhou`.

## Clean up

```bash
kubectl delete -f resources.yaml
```
