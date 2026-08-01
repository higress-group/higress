# AI Request ID Header Plugin

## 概述

AI Request ID Header 插件用于在网关侧传播同一个 request ID：

- request headers 阶段写入 upstream `request-id`，便于 open-platform 等上游服务读取；
- response headers 阶段写入客户端响应头 `request-id`，便于客户端本地 session 记录；
- 与 `ai-token-report` 使用同一 request ID 来源，保证 token usage 上报与 header 链路一致。

## 核心行为

- 读取 request ID 来源：

```go
proxywasm.GetHttpRequestHeader("request-id")
proxywasm.GetProperty([]string{"wasm.requestId"})
proxywasm.GetProperty([]string{"x_request_id"})
proxywasm.GetHttpRequestHeader("x-request-id")
```

- 当 request ID 非空且不为 `-`，会先写入 `wasm.requestId`，便于后续插件（如 `ai-token-report`）在 request header 被移除后仍可读取同一个业务 ID。
- 当请求不是转发给 AI 提供商时，在 request headers 阶段写入 upstream header：

```text
request-id: <request id>
```

- 当请求转发给 AI 提供商时，在 request headers 阶段删除 upstream `request-id`，不论该 header 是客户端原本携带还是本插件准备注入。
- 同时在 response headers 阶段写入客户端响应头（无论是否转发给 AI 提供商，响应头都返回给客户端）：

```text
request-id: <request id>
```

- 当 request ID 为空、`-` 或读取失败时，不写 header，不影响主请求链路。

## AI 提供商路由识别

为避免向 AI 提供商（如 rightcode）携带 `request-id` 导致上游返回 404，插件在 request headers 阶段先判断当前请求是否转发给 AI 提供商，是则删除 upstream `request-id`。判定顺序：

1. **主信号**：读取 `wasm.providerType` 属性。该属性由 `ai-proxy` 在请求阶段写入（见 `ai-proxy/main.go` 的 `onHttpRequestHeader`），与 `ai-token-report` 读取该属性的方式一致。属性非空即视为 AI 路由。
   - 前提：本插件的执行优先级必须低于 `ai-proxy`，否则属性尚未写入会导致漏判。
2. **兜底信号**：读取 `cluster_name` 属性，命中配置的 `skipClusterNamePatterns` 中任一子串即视为 AI 路由。覆盖绕开 `ai-proxy` 直连 AI 厂商的路由（如 `right-codes.dns`）。

response headers 阶段不进行该判断——响应头只返回给客户端，不会到达 AI 提供商。

## 配置说明

```json
{
  "skipClusterNamePatterns": ["llm-", "right-codes.dns", "right-codes-v2.dns"]
}
```

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `skipClusterNamePatterns` | `[]string` | `["llm-"]` | cluster_name 子串匹配列表，命中任一则删除 upstream `request-id`。传空数组 `[]` 可关闭 cluster_name 兜底（仅依赖 `wasm.providerType`）。 |

默认 `["llm-"]` 对应集群命名约定（`llm-RightCodes.internal.dns` 等 AI 提供商 cluster 均带此前缀）。若存在不带 `llm-` 前缀的直连 AI 路由（如 `right-codes.dns`），需在配置中追加对应 cluster 名。

## 构建

```bash
go test ./...
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o main.wasm ./
```

## Request Header Mutation 与路由缓存约束

插件在 request headers 阶段对 `request-id` 发生任何 mutation（write 或 remove）**前**，必须先调用 `ctx.DisableReroute()`（等价于设置 Envoy 属性 `clear_route_cache=off`）。

**根因**：Envoy/Higress 默认在 Wasm 插件修改请求头后重新计算路由（清理 route cache）。`ai-proxy` 会将入口路径（如 `/v1/images/edits`）改写为上游内部路径（如 `/openai/v1/images/edits`），并调用 `DisableReroute()` 防止路由重算。若本插件在此之后再次修改请求头且未关闭 route cache 清理，Envoy 会以已改写的路径重新匹配入口路由，导致匹配失败（`404 NR route_not_found`）。

**执行规则**：

- skip 分支（provider/cluster 判断为 AI 路由）：先读取 `request-id` 请求头，仅在值非空时才调用 `DisableReroute()` 然后执行 remove；不存在时直接跳过，避免无意义的 mutation。
- 非 skip 分支：仅在实际写入/替换非空 `request-id` 时调用 `DisableReroute()`；requestID 为空时提前返回，不产生 mutation。
- 若未来新增其它 request header mutation，必须遵循同样的约束：mutation 前先调用 `DisableReroute()`。

## 说明

该插件只处理 request/response headers，不读取请求体或响应体。`ai-token-report` 使用同一个 request ID 作为上报体中的 `requestId`；当 AI provider 路由删除 upstream `request-id` 后，`ai-token-report` 可通过 `wasm.requestId` 继续读取该业务 ID。

## 迁移说明

该插件由旧名 `ai-request-id-response-header` 改名为 `ai-request-id-header`。如果已有部署使用旧 WasmPlugin 名称或镜像名，需要同步迁移到新名称。
