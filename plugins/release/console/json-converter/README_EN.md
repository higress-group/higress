# JSON Converter

The `jsonrpc-converter` plugin extracts selected JSON-RPC and MCP request or
response fields into HTTP headers for gateway-side logging, routing, and policy
matching. Its logical Console name is `json-converter`, while the released
image remains `jsonrpc-converter`.

Configure `stage` as `request` or `response`. `max_header_length` limits the
generated header values, and `allowed_methods` extends the default MCP methods
handled by the plugin.
