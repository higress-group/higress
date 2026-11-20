# MCP Router Plugin

## Feature Description
The `mcp-router` plugin provides a routing capability for MCP (Model Context Protocol) `tools/call` requests. It inspects the tool name in the request payload, and if the name is prefixed with a server identifier (e.g., `server-name/tool-name`), it dynamically reroutes the request to the appropriate backend MCP server.

This enables the creation of a unified MCP endpoint that can aggregate tools from multiple, distinct MCP servers. A client can make a `tools/call` request to a single endpoint, and the `mcp-router` will ensure it reaches the correct underlying server where the tool is actually hosted.

## Configuration Fields

| Name      | Data Type     | Required | Default Value | Description                                                                                             |
|-----------|---------------|----------|---------------|---------------------------------------------------------------------------------------------------------|
| `servers` | array of objects | Yes      | -             | A list of routing configurations for each backend MCP server.                                           |
| `servers[].name` | string | Yes | - | The unique identifier for the MCP server. This must match the prefix used in the `tools/call` request's tool name. |
| `servers[].domain` | string | No | - | The domain (authority) of the backend MCP server. If omitted, the original request's domain will be kept. |
| `servers[].path` | string | Yes | - | The path of the backend MCP server to which the request will be routed. |

## How It Works

When a `tools/call` request is processed by a route with the `mcp-router` plugin enabled, the following occurs:

1.  **Tool Name Parsing**: The plugin inspects the `name` parameter within the `params` object of the JSON-RPC request.
2.  **Prefix Matching**: It checks if the tool name follows the `server-name/tool-name` format.
    - If it does not match this format, the plugin takes no action, and the request proceeds normally.
    - If it matches, the plugin extracts the `server-name` and the actual `tool-name`.
3.  **Route Lookup**: The extracted `server-name` is used to look up the corresponding routing configuration (domain and path) from the `servers` list in the plugin's configuration.
4.  **Header Modification**:
    - The `:authority` request header is replaced with the `domain` from the matched server configuration.
    - The `:path` request header is replaced with the `path` from the matched server configuration.
5.  **Request Body Modification**: The `name` parameter in the JSON-RPC request body is updated to be just the `tool-name` (the `server-name/` prefix is removed).
6.  **Rerouting**: After the headers are modified, the gateway's routing engine processes the request again with the new destination information, sending it to the correct backend MCP server.

### Example Configuration

Here is an example of how to configure the `mcp-router` plugin in a `higress-plugins.yaml` file:

```yaml
servers:
- name: random-user-server
  domain: mcp.example.com
  path: /mcp-servers/mcp-random-user-server
- name: rest-amap-server
  domain: mcp.example.com
  path: /mcp-servers/mcp-rest-amap-server
```

### Example Usage

Consider a `tools/call` request sent to an endpoint where the `mcp-router` is active:

**Original Request:**
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

**Plugin Actions:**

1.  The plugin identifies the tool name as `rest-amap-server/get-weather`.
2.  It extracts `server-name` as `rest-amap-server` and `tool-name` as `get-weather`.
3.  It finds the matching configuration: `domain: mcp.example.com`, `path: /mcp-servers/mcp-rest-amap-server`.
4.  It modifies the request headers to:
    - `:authority`: `mcp.example.com`
    - `:path`: `/mcp-servers/mcp-rest-amap-server`
5.  It modifies the request body to:

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

The request is then rerouted to the `rest-amap-server`.
