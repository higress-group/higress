# Lava (熔岩) MCP Server

News-flow heat monitoring for stocks and concept boards. Structured read-only subscription: concepts, symbols, summary points, and anonymous institution / major-account IDs. It does not include directional views or original text.

- Landing page: [https://lava.ai-dolphin.com/subscribe](https://lava.ai-dolphin.com/subscribe)
- Docs: [https://lava.ai-dolphin.com/subscribe/docs](https://lava.ai-dolphin.com/subscribe/docs)
- GitHub: [https://github.com/dustinliang666-blip/lava-mcp](https://github.com/dustinliang666-blip/lava-mcp)
- OpenAPI: [https://lava.ai-dolphin.com/subscribe/v1/openapi.json](https://lava.ai-dolphin.com/subscribe/v1/openapi.json)

## Configuration

Set `apiKey` in the Higress MCP marketplace (the API key issued by the Lava console). Requests send it in the `X-API-Key` header.

New accounts receive a 100 CNY voucher; billing follows the official site. This service is a read-only data subscription. It does not provide investment advice, directional views, or original text.

## Usage

Log in to [mcp.higress.ai](https://mcp.higress.ai), generate a URL, and add it to the MCP client. The URL below is Streamable HTTP. Upstream authentication uses the `X-API-Key` header.

```json
"mcpServers": {
  "lava": {
    "url": "https://mcp.higress.ai/mcp-lava/{generate_key}"
  }
}
```

SSE uses the same path with an `/sse` suffix: `https://mcp.higress.ai/mcp-lava/{generate_key}/sse`.
