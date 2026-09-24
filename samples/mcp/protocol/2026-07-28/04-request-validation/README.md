# Demo 04：请求校验与前置阻断

[English](./README_EN.md) | [返回版本目录](../README.md)

本 Demo 验证三类错误在触达 REST 后端前被 Higress 拒绝：Tool 参数缺失、恶意跨域 Origin、`Mcp-Method` 与 JSON-RPC Body 不一致。

## 前置条件

从 Higress 仓库根目录执行：

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

## Step 1：缺少必填 Tool 参数

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

预期 HTTP 为 `200`，因为这是一个合法 Tool 调用的执行结果；JSON-RPC Result 中应有 `isError: true` 和缺少 `location` 的说明。

## Step 2：恶意 Origin

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

预期 HTTP `403`，错误消息为 `untrusted Origin`。

## Step 3：Header 与 Body 不一致

下面的 Body 是 `tools/list`，但 Header 声明 `tools/call`：

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

预期 HTTP `400`、JSON-RPC error code `-32020`，错误消息包含 `header does not match request body`。

## Step 4：证明后端调用数为零

```bash
curl -sS "$BACKEND_URL/__state" | jq
curl -sS "$BACKEND_URL/__state" | jq -e '.events | length == 0'
```

这一区分了 Tool Execution Error 与传输边界错误，但三种错误都必须在后端调用前结束。

## 清理

```bash
kubectl delete -f resources.yaml
```
