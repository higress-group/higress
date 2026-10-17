# mcp-server

[English](./README_EN.md)

## 功能说明

`mcp-server` 是内置 MCP Server 示例插件，在网关侧托管多个 MCP 工具服务。当前版本内置：

- **quark-search**：夸克搜索相关工具
- **amap-tools**：高德地图相关工具

客户端可通过 MCP 协议（如 `tools/list`、`tools/call`）经 Higress 统一入口调用上述工具，并复用网关的认证、限流与可观测能力。

> 使用 MCP Server 类插件需要 **Higress 2.1.0** 及以上版本。

## MCP 2026 Tools 基线

插件同时保留 legacy profile，并提供 MCP `2026-07-28` 的无状态 HTTP Tools 基线：

| Profile | 精确支持版本 | 生命周期与传输 |
| --- | --- | --- |
| legacy | `2024-11-05`、`2025-03-26`、`2025-06-18` | 保留 `initialize` / `notifications/initialized`、现有 session 与 HTTP/SSE 兼容行为 |
| modern | `2026-07-28` | 每请求 `_meta`、无 initialize、无协议 session；不提供 GET/DELETE/Last-Event-ID 恢复 |

Modern profile 本期只实现 `server/discover`、`tools/list` 和 `tools/call`。它会校验 Content-Type、Accept、同源 Origin、JSON-RPC 单消息边界、身份镜像头与资源上限；批量、response envelope 和 trailing JSON 会被拒绝。

现代请求必须同时发送传输头和 `_meta`，例如：

```http
MCP-Protocol-Version: 2026-07-28
Mcp-Method: tools/call
Mcp-Name: get_weather
Content-Type: application/json
Accept: application/json, text/event-stream
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "_meta": {
      "io.modelcontextprotocol/protocolVersion": "2026-07-28",
      "io.modelcontextprotocol/clientInfo": {"name": "example", "version": "1.0.0"},
      "io.modelcontextprotocol/clientCapabilities": {}
    },
    "name": "get_weather",
    "arguments": {"location": "Hangzhou"}
  }
}
```

### 能力与结果合同

- `server/discover` 只声明当前真正可用的 `tools: {}`；不声明 `tools.listChanged`、MRTR、subscriptions、resources、prompts 或其他未实现能力。
- 所有 modern 成功结果使用 `resultType: complete`，并在 `_meta.io.modelcontextprotocol/serverInfo` 携带服务端身份。
- `server/discover` 和 `tools/list` 额外返回 `ttlMs: 0` 与 `cacheScope: private`。这只是 wire contract；本期没有 response/descriptor cache 引擎、共享缓存或主动失效。

### Proxy profile 矩阵

`protocolStrategy` 只描述上游策略；未配置、空值和显式 `legacy` 都保持原行为：

| Downstream | Upstream | 当前状态 |
| --- | --- | --- |
| modern | registered / REST / composed | 支持 |
| modern | `protocolStrategy: modern` | 支持，每请求无状态转发 |
| modern | `protocolStrategy: legacy` | 支持，在单次下游交换内执行隔离的 legacy handshake |
| modern | `auto` + `http` | 每次转发型 Tool 请求先 discover，再选择 modern 或完成一次 legacy 握手 |
| legacy | `auto` + `http` | 原 legacy 路径，不探测 modern |
| 任意已支持下游 | `auto` + `sse` | 原 legacy SSE 路径，不猜测传输或 URL |
| legacy | legacy upstream | 保留现有行为 |
| legacy | modern-only upstream | 不支持，已暂缓 |

出站头基于现有认证处理后的请求继承：`Cookie`、租户头、自定义业务头和前置插件添加的普通头默认保留，无需新增配置。已被认证逻辑移除的凭据不会恢复；`Authorization` 继续遵循 `downstreamSecurity`、`passthroughAuthHeader` 和显式上游凭据规则。下游 session、`Last-Event-ID` 和内部控制头按操作清理；MCP 协议头重新生成。格式合法的 `Mcp-Param-*` 仅透传到当前 modern tools/call，不进入 discover、initialize 或 legacy RPC。

### 迁移与默认行为

现有 `mcp-proxy` 无需迁移，默认仍为 `legacy`。确定上游支持 `2026-07-28` 的路由可显式选择 `modern`，避免探测开销。需要请求级识别时显式启用 `auto`；它不改变下游协议。回滚不识别 auto 的旧插件前，先将配置切回 `legacy` 并确认生效，再回滚镜像。不要将运行期 session ID 写入配置或测试凭据。

### 可选 auto 协议识别

```yaml
server:
  name: upstream-tools
  type: mcp-proxy
  transport: http
  protocolStrategy: auto
  mcpServerURL: https://mcp.example.com/mcp
  timeout: 5000
  autoDetection:
    probeTimeoutMs: 1000
```

`autoDetection.probeTimeoutMs` 仅在 `auto + http` 下解析：缺省 1000 ms，必须是可表示为 uint32 的正整数，实际探测超时取它与有效 `timeout` 的较小值。单独配置 autoDetection 不启用 auto；插件 `timeout` 约束探测/握手等独立子请求（缺省 5000ms），最终 HTTP 业务请求由原路由的 timeout 控制，整条链路总耗时可能更长。

每次真正转发的 `tools/list` 或 `tools/call` 都先完成现有权限检查，再使用一次解析的有效上游凭证发送 `server/discover`。modern 成功序列为 discover → Tool RPC（2 次）；legacy 成功序列为 discover → initialize → initialized → Tool RPC（4 次）。没有缓存、跨请求会话复用或合并；调用工具无需先 list。下游 discover 只查询网关自身能力，不访问上游。

| 探测结果 | 处理 |
| --- | --- |
| 合法 discover，支持 2026-07-28 且声明 Tools | modern |
| -32022 明确支持 2025-06-18 / 2025-03-26 | 优先 2025-06-18，完成一次兼容 HTTP legacy 握手 |
| HTTP 200、ID 匹配的 -32601 且无 modern 证据；普通 400/404/405 且无 modern 错误 | 最多尝试一次 2025-03-26 初始化 |
| modern 错误、HTTP 404 的合法 -32601、401/403/429、5xx、网络错误或超时 | 终止，不降级；保留适用的认证挑战和 Retry-After |
| 错误 ID、损坏 JSON/SSE、batch、trailing JSON、探测响应超过 1 MiB | 终止，不通过解析失败猜测 legacy |

legacy 握手严格检查返回版本、Tools 能力、serverInfo 和 initialized 成功；会话仅在本次请求内携带。业务发出后发生错误、断连或版本变化都不重放。现代 requestState/inputResponses 无法转到 legacy 时在业务前拒绝；cursor 错误不自动重开列表。

有效凭证必须有 discover 权限。固定、工具级和透传凭证都应用于本次探测与业务；显式 Cookie 可使用 `apiKey` 的 `in: header, name: Cookie` 生成，原有 `in: cookie` 不受支持。探测不携带 Mcp-Name/Mcp-Param；auto 仅在实际 modern tools/call 转发适用参数头。

同一上游池实例必须提供一致协议能力；探测与业务仍可能落到不同实例。HTTP JSON/SSE 在完整业务响应后解析，不提供实时进度或订阅。1 MiB 限制在复制探测 body 到 Wasm 前检查，不限制 Envoy 已接收的全部缓冲，也不限制 Tool 结果大小。取消阻止后续阶段，不宣称能撤回已提交的工具副作用。取消后的有效连接明确终止响应，断连后的回调不再访问流。

独立示例见 [auto.yaml](../../../../samples/mcp/protocol/2026-07-28/auto.yaml)。

### 主请求转发兼容性

HTTP `legacy`、`modern`、`auto` 的最终 `tools/list` / `tools/call` 使用当前 SDK 的 `RouteCall` 继续原请求，后置请求插件和原路由的 header/path rewrite、timeout、retry、hash policy 因此生效。继续保留 `DisableReroute`：本插件修改 host/path 不主动重新匹配路由。插件只提交一次业务请求；Envoy 路由重试可能产生多次后端尝试，不承诺工具恰好执行一次。

initialize、initialized、auto discover 仍是独立子请求，不经过后置插件；后置插件添加的凭据不能反向作用于已完成的握手。旧 SSE 仍使用主请求 GET 建立通道和独立 POST RPC，同时恢复普通头继承。上游协议识别要求后续插件不把最终业务改到另一个逻辑服务。

同步 modern 通过 Continue 放行，异步握手成功才 Resume 一次。响应转换和取消清理在 RouteCall 回调及 stream-done 中完成，覆盖无 body 的响应头阶段和完整 body 的响应体阶段。本次不修改或升级 SDK；保留 RouteCall 已有 INFO 完整请求/响应日志以及内部改写操作未逐项检查返回错误的行为，不要求调整日志级别。

旧配置无需迁移。回滚时将 WasmPlugin 的 `url` 恢复为之前记录的插件镜像 digest（本地挂载环境恢复原 Wasm 文件并重新加载），保留原路由配置；回滚后最终业务的 sidecall 差异与普通头限制也会恢复。真实数据面三版本验证入口见 [runtime-verification](testdata/runtime-verification/README.md#routing-compatibility-4597)。

### 明确暂缓范围

| 层级 | 不属于本期的能力 |
| --- | --- |
| Deferred P1 | `2025-11-25` profile、完整 JSON Schema 2020-12 与 output validation、大规模 tools pagination/cursor、legacy downstream→modern-only bridge |
| Deferred P2 | MRTR / `input_required` 生成、subscriptions/listen 与 `tools.listChanged`、通用 state/requestState 的 TTL、持久化与恢复 |
| Separate Proposal | 完整 OAuth resource server/client、Tasks、MCP Apps、Resources、Prompts、Completion，以及独立 native `plugins/golang-filter/mcp-server` 的协议同步 |

## 配置说明

本插件通过编译期注册 MCP Server，**WasmPlugin 的 `defaultConfig` 通常无需额外字段**。如需在 MCP `initialize` 响应中展示自定义服务版本，可在 `server.version` 中配置；未配置时默认返回 `1.0.0`。具体工具列表、参数与鉴权由各子 Server 实现决定，开发新 MCP Server 请参考 [MCP Server 实现指南](../../mcp-servers/README.md)。

### REST Server 示例

```yaml
server:
  name: weather
  type: rest
tools:
  - name: get_weather
    description: Query weather
    args:
      - name: location
        type: string
        required: true
    responseTemplate:
      body: "weather for {{.args.location}}"
```

### 自定义 Server 版本示例

```yaml
# 自定义 initialize 响应中的 serverInfo.version
server:
  name: quark-search
  version: 2.5.0
```

### MCP Proxy 示例

```yaml
server:
  name: upstream-tools
  type: mcp-proxy
  transport: http
  protocolStrategy: modern # 或 legacy
  mcpServerURL: https://mcp.example.com/mcp
```

## 引用插件

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: mcp-server
  namespace: higress-system
spec:
  selector:
    matchLabels:
      higress: higress-system-higress-gateway
  url: oci://higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/mcp-server:<version>
```

## 可复现验证

官方规范示例固定到 `modelcontextprotocol/modelcontextprotocol@f817239f4d6b1efff2c4dfc2f7af85c985d73076`，SDK 固定为 Go `v1.7.0` 和 TypeScript `2.0.0`。测试不会获取移动的 `latest` 版本。

```bash
# 单元、官方示例和兼容回归
go test -count=1 ./...

# Go 1.25+、Node.js 20+；会对 direct/modern-proxy/legacy-proxy 执行 discover/list/call
./testdata/interop/run.sh

# 独立构建 e2e 必需的 WASM（不依赖 VERSION/-alpha 扫描）
make -C ../../../.. build-mcp-server-wasmplugin
```

完整 kind/Envoy e2e 需要 Docker、kind、kubectl 和 Helm：

```bash
PLUGIN_TYPE=GO PLUGIN_NAME=mcp-server TEST_SHORTNAME=WasmPluginsMCP20260728 make higress-wasmplugin-test
```

仓库本地验证不能替代该真实数据面步骤；CI 会执行它。`plugin.wasm` 是构建产物，不应提交。

## 相关文档

- [MCP 快速开始](https://higress.cn/ai/mcp-quick-start/)
- [MCP Server 开发指南](../../mcp-servers/README.md)
- [Wasm 插件市场](https://higress.cn/plugin/)
