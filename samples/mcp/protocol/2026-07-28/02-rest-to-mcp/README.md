# Demo 02：REST-to-MCP

[English](./README_EN.md) | [返回版本目录](../README.md)

本 Demo 验证 Higress 将一个 MCP `tools/call` 转换为一次普通 REST 请求，并通过协议无关后端的事件账本证明没有重复调用。

## 前置条件

从 Higress 仓库根目录执行：

```bash
cd samples/mcp
./protocol/2026-07-28/plugin/build.sh
./environment/scripts/up.sh
cd protocol/2026-07-28/02-rest-to-mcp
export GATEWAY_URL=http://127.0.0.1:18080/mcp
export BACKEND_URL=http://127.0.0.1:18082
export MCP_HOST=rest-to-mcp.mcp.demo
```

## Step 1：部署并重置后端

```bash
kubectl apply -f resources.yaml
curl -sS -X POST "$BACKEND_URL/__reset" | jq
```

## Step 2：查看发布的 REST Tool

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

应看到必填参数为 `location` 的 `get_weather`。

## Step 3：调用 Tool

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

响应应包含 `weather for Hangzhou` 和 `resultType: complete`。

## Step 4：证明只有一次 REST 调用

```bash
curl -sS "$BACKEND_URL/__state" | jq
```

事件账本应只有一条：

```json
{
  "httpMethod": "GET",
  "path": "/weather",
  "query": {"location": "Hangzhou"}
}
```

可以直接检查事件数量：

```bash
curl -sS "$BACKEND_URL/__state" | jq -e '.events | length == 1'
```

## 清理

```bash
kubectl delete -f resources.yaml
```
