# MCP Router 插件

## 功能说明
`mcp-router` 插件为 MCP (Model Context Protocol) 的 `tools/call` 请求提供了路由能力。它会检查请求负载中的工具名称，如果名称带有服务器标识符前缀（例如 `server-name/tool-name`），它会动态地将请求重新路由到相应的后端 MCP 服务器。

这使得创建一个统一的 MCP 端点成为可能，该端点可以聚合来自多个不同 MCP 服务器的工具。客户端可以向单个端点发出 `tools/call` 请求，`mcp-router` 将确保请求到达托管该工具的正确底层服务器。

## 配置字段

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|---|---|---|---|---|
| `servers` | 对象数组 | 是 | - | 每个后端 MCP 服务器的路由配置列表。 |
| `servers[].name` | 字符串 | 是 | - | MCP 服务器的唯一标识符。这必须与 `tools/call` 请求的工具名称中使用的前缀相匹配。 |
| `servers[].domain` | 字符串 | 否 | - | 后端 MCP 服务器的域名 (authority)。如果省略，将保留原始请求的域名。 |
| `servers[].path` | 字符串 | 是 | - | 请求将被路由到的后端 MCP 服务器的路径。 |

## 工作原理

当一个启用了 `mcp-router` 插件的路由处理 `tools/call` 请求时，会发生以下情况：

1.  **工具名称解析**：插件检查 JSON-RPC 请求中 `params` 对象的 `name` 参数。
2.  **前缀匹配**：它检查工具名称是否遵循 `server-name/tool-name` 格式。
    - 如果不匹配此格式，插件不执行任何操作，请求将正常继续。
    - 如果匹配，插件将提取 `server-name` 和实际的 `tool-name`。
3.  **路由查找**：提取的 `server-name` 用于从插件配置的 `servers` 列表中查找相应的路由配置（domain 和 path）。
4.  **Header 修改**：
    - `:authority` 请求头被替换为匹配的服务器配置中的 `domain`。
    - `:path` 请求头被替换为匹配的服务器配置中的 `path`。
5.  **请求体修改**：JSON-RPC 请求体中的 `name` 参数被更新为仅包含 `tool-name`（移除了 `server-name/` 前缀）。
6.  **重新路由**：在 Header 修改后，网关的路由引擎会使用新的目标信息再次处理请求，将其发送到正确的后端 MCP 服务器。

### 配置示例

以下是在 `higress-plugins.yaml` 文件中配置 `mcp-router` 插件的示例：

```yaml
servers:
- name: random-user-server
  domain: mcp.example.com
  path: /mcp-servers/mcp-random-user-server
- name: rest-amap-server
  domain: mcp.example.com
  path: /mcp-servers/mcp-rest-amap-server
```

### 使用示例

假设一个 `tools/call` 请求被发送到激活了 `mcp-router` 的端点：

**原始请求:**
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "rest-amap-server/get-weather",
    "arguments": {
      "location": "New York"
    }
  }
}
```

**插件行为:**

1.  插件识别出工具名称为 `rest-amap-server/get-weather`。
2.  它提取出 `server-name` 为 `rest-amap-server`，`tool-name` 为 `get-weather`。
3.  它找到匹配的配置：`domain: mcp.example.com`, `path: /mcp-servers/mcp-rest-amap-server`。
4.  它将请求头修改为：
    - `:authority`: `mcp.example.com`
    - `:path`: `/mcp-servers/mcp-rest-amap-server`
5.  它将请求体修改为：

    ```json
    {
      "jsonrpc": "2.0",
      "id": 2,
      "method": "tools/call",
      "params": {
        "name": "get-weather",
        "arguments": {
          "location": "New York"
        }
      }
    }
    ```

请求随后被重新路由到 `rest-amap-server`。
