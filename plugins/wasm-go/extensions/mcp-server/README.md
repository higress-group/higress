# 功能说明

`mcp-server` 是内置 MCP Server 示例插件，在网关侧托管多个 MCP 工具服务。当前版本内置：

- **quark-search**：夸克搜索相关工具
- **amap-tools**：高德地图相关工具

客户端可通过 MCP 协议（如 `tools/list`、`tools/call`）经 Higress 统一入口调用上述工具，并复用网关的认证、限流与可观测能力。

> 使用 MCP Server 类插件需要 **Higress 2.1.0** 及以上版本。

# 配置说明

本插件通过编译期注册 MCP Server，**WasmPlugin 的 `defaultConfig` 通常无需额外字段**。具体工具列表、参数与鉴权由各子 Server 实现决定，开发新 MCP Server 请参考 [MCP Server 实现指南](../../mcp-servers/README.md)。

# 配置示例

```yaml
# 多数场景下使用空配置或仅配置路由匹配即可
{}
```

# 引用插件

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

# 相关文档

- [MCP 快速开始](https://higress.cn/ai/mcp-quick-start/)
- [MCP Server 开发指南](../../mcp-servers/README.md)
- [Wasm 插件市场](https://higress.cn/plugin/)
