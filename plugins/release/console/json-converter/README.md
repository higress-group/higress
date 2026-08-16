# JSON 转换器

`jsonrpc-converter` 插件从 JSON-RPC / MCP 请求与响应中提取关键字段并写入
HTTP 请求头或响应头，方便在网关侧配置日志、路由与策略匹配。控制台中的逻辑名称为
`json-converter`，对应的正式插件镜像名称为 `jsonrpc-converter`。

详细字段、生成的请求头及示例请参见
[插件文档](../../../wasm-go/extensions/jsonrpc-converter/README.md)。
