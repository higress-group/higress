# Demo 03：新版客户端访问 Legacy MCP Server

[English](./README_EN.md) | [返回版本目录](../README.md)

本 Demo 验证新版无状态客户端通过 Higress 访问 MCP `2025-03-26` Server。Higress 在每个下游请求内部执行隔离的 legacy handshake，同时适配新版结果合同并阻止敏感下游 Header 穿越协议边界。

## 前置条件

从 Higress 仓库根目录执行：

```bash
cd samples/mcp
./protocol/2026-07-28/plugin/build.sh
./environment/scripts/up.sh
cd protocol/2026-07-28/03-modern-to-legacy
export GATEWAY_URL=http://127.0.0.1:18080/mcp
export MCP_HOST=legacy-bridge.mcp.demo
```

## Step 1：部署 Legacy MCP fixture

fixture 使用 `python:3.12-alpine`，Python 源码通过 ConfigMap 挂载：

```bash
kubectl -n mcp-demo create configmap legacy-mcp-fixture \
  --from-file=server.py=fixture/legacy_server.py \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f fixture/deployment.yaml
kubectl -n mcp-demo rollout status deployment/legacy-mcp --timeout=180s
```

部署 Higress 路由和 MCP Proxy：

```bash
kubectl apply -f resources.yaml
```

清空 fixture 事件：

```bash
kubectl -n mcp-demo exec deployment/legacy-mcp -- python -c \
  'import urllib.request; urllib.request.urlopen(urllib.request.Request("http://127.0.0.1:8080/__reset", data=b"", method="POST"))'
```

## Step 2：用新版请求列出 Legacy Tool

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

客户端应看到新版合同：`proxy_echo`、`resultType: complete`、`ttlMs: 0` 和 `cacheScope: private`。响应头不应暴露上游 Session：

```bash
grep -i '^Mcp-Session-Id:' /tmp/mcp-bridge-list-headers || echo 'upstream session is hidden'
```

## Step 3：调用 Legacy Tool

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

预期结果为 `echo:bridge`、`isError: false` 和 `resultType: complete`。

## Step 4：检查精确握手序列

```bash
kubectl -n mcp-demo exec deployment/legacy-mcp -- python -c \
  'import urllib.request; print(urllib.request.urlopen("http://127.0.0.1:8080/__state").read().decode())' |
  jq
```

两次下游请求应产生严格的六个上游事件：

```text
initialize
notifications/initialized
tools/list
initialize
notifications/initialized
tools/call
```

可以直接检查：

```bash
kubectl -n mcp-demo exec deployment/legacy-mcp -- python -c \
  'import urllib.request; print(urllib.request.urlopen("http://127.0.0.1:8080/__state").read().decode())' |
  jq -e '[.events[].rpcMethod] == ["initialize","notifications/initialized","tools/list","initialize","notifications/initialized","tools/call"]'
```

事件中的 `cookiePresent`、`lastEventIDPresent`、`authorizationPresent`、`unrelatedCredentialPresent` 应全部为 `false`，`futureParam` 应为 `null`。Legacy handshake 自己产生的 `Mcp-Session-Id` 只在上游交换中使用。

## 清理

```bash
kubectl delete -f resources.yaml
kubectl delete -f fixture/deployment.yaml
kubectl -n mcp-demo delete configmap legacy-mcp-fixture
```
