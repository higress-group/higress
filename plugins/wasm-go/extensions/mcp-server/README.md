# mcp-server

[English](./README_EN.md)

## 功能说明

`mcp-server` 在 Higress 网关中提供 MCP 工具服务，当前内置以下工具服务：

| 服务名称 | 功能 |
| --- | --- |
| `quark-search` | 夸克网页搜索，提供 `web_search` 工具 |
| `amap-tools` | 高德地图工具，包括地理编码、地点搜索、路线规划和天气查询等 |

插件也支持将 REST API 转换为 MCP 工具，以及代理已有 MCP Server。完整能力、协议支持和配置说明请参阅 [MCP Server 插件配置](https://higress.cn/docs/ai/mcp-server/)。

## 使用内置工具

使用 MCP Server 类插件需要 **Higress 2.1.0 及以上版本**。

为需要使用的工具服务创建路由，配置对应后端，并在该路由上启用 `mcp-server` 插件。以下示例为插件配置内容；内置工具已预先定义，无需填写 `tools`。

### 夸克搜索

准备夸克搜索 API Key，将路由后端配置为 `cloud-iqs.aliyuncs.com`，并填写：

```yaml
server:
  name: quark-search
  config:
    apiKey: "<夸克搜索 API Key>"
```

配置生效后，可通过 MCP 客户端调用 `web_search` 工具。搜索关键词建议使用中文。

### 高德地图

准备高德地图 API Key，将路由后端配置为 `restapi.amap.com`，并填写：

```yaml
server:
  name: amap-tools
  config:
    apiKey: "<高德地图 API Key>"
```

配置生效后，可通过 MCP 客户端调用地图工具，例如使用 `maps_weather` 查询天气。

### 连接 MCP 客户端

将对应路由对外提供的 MCP 访问地址配置到客户端，即可查看该服务的工具列表、参数说明并调用工具。

如需同时提供夸克搜索和高德地图服务，可分别创建路由，并为各自的路由填写对应插件配置。

路由接入、工具访问控制、认证、REST-to-MCP 和 MCP 代理等详细配置，请参阅 [官网配置文档](https://higress.cn/docs/ai/mcp-server/)。

## 相关文档

- [MCP 快速开始](https://higress.cn/docs/ai/mcp-quick-start/)
- [MCP Server 开发指南](../../mcp-servers/README.md)
- [Wasm 插件市场](https://higress.cn/plugins)
