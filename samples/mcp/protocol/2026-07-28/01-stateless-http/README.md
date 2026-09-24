# Demo 01：无状态 HTTP Tools

[English](./README_EN.md) | [返回版本目录](../README.md)

本 Demo 验证 MCP `2026-07-28` 客户端可以通过独立 HTTP 请求完成 `server/discover`、`tools/list` 和 `tools/call`，不需要 `initialize`，也不会获得协议 Session。

## 前置条件

从 Higress 仓库根目录构建插件并启动环境：

```bash
cd samples/mcp
./protocol/2026-07-28/plugin/build.sh
./environment/scripts/up.sh
```

然后进入本目录：

```bash
cd protocol/2026-07-28/01-stateless-http
export GATEWAY_URL=http://127.0.0.1:18080/mcp
export MCP_HOST=stateless.mcp.demo
```

## Step 1：部署 Demo

```bash
kubectl apply -f resources.yaml
kubectl get ingress -n mcp-demo mcp-2026-stateless-http
kubectl get wasmplugin -n higress-system mcp-2026-stateless-http
```

等待几秒让 Higress Controller 将配置下发到 Gateway。

## Step 2：发现 Server

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

预期结果包含：

```text
result.resultType = complete
result.capabilities = {"tools": {}}
result.ttlMs = 0
result.cacheScope = private
result._meta.io.modelcontextprotocol/serverInfo.name = stateless-demo
```

## Step 3：列出工具

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

工具列表中应只有 `say_hello`，并继续携带 `resultType: complete`、`ttlMs: 0` 和 `cacheScope: private`。

## Step 4：调用工具并检查 Session

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

预期文本为 `hello Higress`。响应头中不应包含协议 Session：

```bash
if grep -qi '^Mcp-Session-Id:' /tmp/mcp-stateless-headers; then
  echo 'unexpected MCP session'
else
  echo 'no MCP session returned'
fi
```

三次请求之间没有 initialize 或 Session 依赖，每一个请求都可以独立执行。

## Step 5：查看网关证据

```bash
kubectl logs -n higress-system deployment/higress-gateway --tail=200 |
  grep 'demo-stateless' || true
```

## 清理

```bash
kubectl delete -f resources.yaml
```
