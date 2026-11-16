# E2BDev MCP Server

An implementation of the Model Context Protocol (MCP) server that integrates the E2B API for sandbox lifecycle management. The server provides only the `create_sandbox` and `kill_sandbox` tools; it does not execute code.


## Usage Guide

### Get API-KEY
1. Register for an E2B account [Resigter Entry](https://e2b.dev/auth/sign-up). Each new account will receive 100 credits for free.
2. Generate API Key in Dashboard [Manage API-KEY](https://e2b.dev/dashboard?tab=keys)

### Configure MCP Client

On the user's MCP Client interface, add E2BDev MCP Server configuration.

```json
"mcpServers": {
    "e2bdev": {
      "url": "https://mcp.higress.ai/mcp-e2bdev/{generate_key}",
    }
}
```

### Tools

The server provides only the following sandbox lifecycle tools:

- **create_sandbox**: Create E2B sandbox environment
  - Parameters:
    - timeout: Sandbox timeout in seconds, sandbox will be terminated after timeout
  - Returns: Sandbox ID

- **kill_sandbox**: Terminate sandbox environment
  - Parameters:
    - sandbox_id: Sandbox ID to terminate
  - Returns: Termination result
