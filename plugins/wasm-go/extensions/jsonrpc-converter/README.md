# 功能说明

`jsonrpc-converter` 是 MCP（Model Context Protocol）相关过滤器，将 JSON-RPC / MCP 请求与响应中的关键字段提取到 Envoy 可路由、可观测的 HTTP 头中，便于在网关侧做日志、路由或策略匹配。

支持的方法默认包括 `tools/list`、`tools/call`，可通过 `allowed_methods` 扩展。

# 配置字段

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
| -------- | -------- | -------- | -------- | -------- |
| stage | string | 必填 | - | 处理阶段：`request`（请求前写入头）或 `response`（响应前写入头） |
| max_header_length | number | 选填 | 4000 | 写入头的字符串最大长度，超出则截断 |
| allowed_methods | array of string | 选填 | `tools/list`, `tools/call` | 允许处理的 JSON-RPC 方法列表 |

# 写入的请求头（stage=request）

| 请求头 | 说明 |
| -------- | -------- |
| x-envoy-jsonrpc-id | JSON-RPC id |
| x-envoy-jsonrpc-method | 方法名 |
| x-envoy-jsonrpc-params | 参数（`tools/call` 除外，见下） |
| x-envoy-mcp-tool-name | `tools/call` 的工具名 |
| x-envoy-mcp-tool-arguments | `tools/call` 的工具参数 |

# 写入的响应头（stage=response）

| 响应头 | 说明 |
| -------- | -------- |
| x-envoy-jsonrpc-id | JSON-RPC id |
| x-envoy-jsonrpc-method | 方法名 |
| x-envoy-jsonrpc-result | 结果（部分方法由专用逻辑写入） |
| x-envoy-jsonrpc-error | 错误信息 |
| x-envoy-mcp-tool-response | `tools/call` 的响应内容 |
| x-envoy-mcp-tool-error | `tools/call` 是否出错 |

# 配置示例

```yaml
stage: request
max_header_length: 4000
allowed_methods:
  - tools/list
  - tools/call
```

# 说明

- 通常在同一路由上配置两个 WasmPlugin 实例，分别设置 `stage: request` 与 `stage: response`，以在请求链路与响应链路上分别注入头信息。
- 与 `stage` 相对的另一侧会自动清理已写入的头，避免头信息残留。
- 需配合 Higress MCP 过滤器链使用，详见 [MCP Server 开发指南](../../mcp-servers/README.md)。
