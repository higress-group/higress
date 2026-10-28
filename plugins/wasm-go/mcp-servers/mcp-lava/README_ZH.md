# 熔岩（Lava）MCP Server

股票和概念板块的消息面热度监测。结构化只读订阅：概念/标的/摘要要点与匿名机构/大V ID，不含多空方向与原文。

- 落地页：[https://lava.ai-dolphin.com/subscribe](https://lava.ai-dolphin.com/subscribe)
- 文档：[https://lava.ai-dolphin.com/subscribe/docs](https://lava.ai-dolphin.com/subscribe/docs)
- GitHub：[https://github.com/dustinliang666-blip/lava-mcp](https://github.com/dustinliang666-blip/lava-mcp)
- OpenAPI：[https://lava.ai-dolphin.com/subscribe/v1/openapi.json](https://lava.ai-dolphin.com/subscribe/v1/openapi.json)

## 配置

在 Higress MCP 市场配置项中填写 `apiKey`（熔岩控制台发放的 API Key，请求头 `X-API-Key`）。

开通赠 100 元代金券；计费以官网为准。本服务为只读数据订阅，不提供投资建议、多空方向或原文。

## 使用

登录 [mcp.higress.ai](https://mcp.higress.ai) 生成 URL 后，在 MCP Client 中添加。下列地址为 Streamable HTTP；上游鉴权请求头为 `X-API-Key`。

```json
"mcpServers": {
  "lava": {
    "url": "https://mcp.higress.ai/mcp-lava/{generate_key}"
  }
}
```

SSE 使用同一路径加 `/sse` 后缀：`https://mcp.higress.ai/mcp-lava/{generate_key}/sse`。
