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

`protocolStrategy` 只描述上游 profile，本期是显式配置：

| Downstream | Upstream | 当前状态 |
| --- | --- | --- |
| modern | registered / REST / composed | 支持 |
| modern | `protocolStrategy: modern` | 支持，每请求无状态转发 |
| modern | `protocolStrategy: legacy` | 支持，在单次下游交换内执行隔离的 legacy handshake |
| legacy | legacy upstream | 保留现有行为 |
| legacy | modern-only upstream | 不支持，已暂缓 |

Outbound headers 按每个 RPC 重建。`Authorization` 只会根据显式 proxy auth policy 生成或转发。`Cookie`、下游 session、`Last-Event-ID`、内部路由头和无关凭据默认不转发。未识别但格式合法的 `Mcp-Param-*` 仅在 modern→modern 的当前 Tool RPC 中透传，不得进入 discover、initialize 或 legacy RPC。

### 迁移与默认行为

现有未配置 `protocolStrategy` 的 `mcp-proxy` 继续默认使用 `legacy`，legacy downstream→legacy upstream 的 initialize/session/transport 路径不变。只有已确认上游支持 `2026-07-28` 时才应显式切换为 `modern`。本期不会自动探测、回退或重试其他版本，也不应将运行期 session ID 写入配置或测试凭据。

### 明确暂缓范围

| 层级 | 不属于本期的能力 |
| --- | --- |
| Deferred P1 | `protocolStrategy: auto`、`2025-11-25` profile、完整 JSON Schema 2020-12 与 output validation、大规模 tools pagination/cursor、legacy downstream→modern-only bridge |
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
