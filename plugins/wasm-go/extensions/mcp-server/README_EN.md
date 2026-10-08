# mcp-server

[中文](./README.md)

## Overview

`mcp-server` provides MCP tool services in the Higress gateway and includes the following servers:

| Server name | Capabilities |
| --- | --- |
| `quark-search` | Quark web search through the `web_search` tool |
| `amap-tools` | Amap tools for geocoding, place search, route planning, weather queries, and more |

The plugin also supports converting REST APIs into MCP tools and proxying existing MCP servers. For the full capabilities, supported protocols, and configuration reference, see [MCP Server Plugin Configuration](https://higress.cn/en/docs/ai/mcp-server/).

## Using the built-in tools

MCP server plugins require **Higress 2.1.0 or later**.

Create a route for the tool server you want to use, configure its backend, and enable the `mcp-server` plugin on that route. The examples below are plugin configurations. Built-in tools are predefined, so you do not need to configure `tools`.

### Quark Search

Obtain a Quark Search API key, configure the route backend as `cloud-iqs.aliyuncs.com`, and use:

```yaml
server:
  name: quark-search
  config:
    apiKey: "<Quark Search API key>"
```

Once the configuration takes effect, use an MCP client to call `web_search`. Chinese search queries are recommended.

### Amap

Obtain an Amap API key, configure the route backend as `restapi.amap.com`, and use:

```yaml
server:
  name: amap-tools
  config:
    apiKey: "<Amap API key>"
```

Once the configuration takes effect, use an MCP client to call the map tools, such as `maps_weather` for weather queries.

### Connecting an MCP client

Configure your MCP client with the MCP endpoint exposed by the route. You can then view the server's tools and their parameters and invoke the tools.

To provide both Quark Search and Amap services, create a separate route for each service and apply the corresponding plugin configuration to each route.

For detailed configuration of routing, tool access control, authentication, REST-to-MCP, and MCP proxying, see the [official configuration guide](https://higress.cn/en/docs/ai/mcp-server/).

## Related documentation

- [MCP quick start](https://higress.cn/en/docs/ai/mcp-quick-start/)
- [MCP server development guide](../../mcp-servers/README.md)
- [Wasm plugin marketplace](https://higress.cn/en/plugins)
