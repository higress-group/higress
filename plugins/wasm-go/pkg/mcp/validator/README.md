# MCP Configuration Validator

This package provides a configuration validation library for MCP (Model Context Protocol) server configurations. It allows you to validate MCP configurations without requiring the full runtime environment, making it perfect for use in management platforms and frontend applications.

## Features

- **Lightweight Validation**: Validates configuration structure and syntax without requiring actual server instances
- **REST Tool Support**: Full validation for REST-based MCP tools including request/response templates
- **ToolSet Support**: Validates composed server configurations (toolSets)
- **Pre-registered Server Handling**: Gracefully handles pre-registered Go-based servers by skipping their validation
- **Minimal Dependencies**: Reuses the core parsing logic from the main MCP server implementation

## Usage

### Basic Validation

```go
import "github.com/higress-group/wasm-go/pkg/mcp/validator"

// Validate a configuration YAML string
yamlConfig := `
server:
  name: my-server
  config:
    apiKey: secret
tools:
  - name: my-tool
    description: A sample tool
    args:
      - name: input
        type: string
        required: true
    requestTemplate:
      url: https://api.example.com/endpoint
      method: POST
    responseTemplate:
      body: "{{.}}"
`
result, err := validator.ValidateConfigYAML(yamlConfig)
if err != nil {
    // Handle error
    return
}

if result.IsValid {
    fmt.Printf("Configuration is valid for server: %s\n", result.ServerName)
    if result.IsComposed {
        fmt.Println("This is a composed server (toolSet)")
    } else {
        fmt.Println("This is a single server")
    }
} else {
    fmt.Printf("Configuration is invalid: %v\n", result.Error)
}
```

## Supported Configuration Types

### 1. REST Server Configuration

Validates REST-based MCP servers with tools, security schemes, and templates:

```yaml
server:
  name: weather-api
  config:
    apiKey: your-api-key
  securitySchemes:
    - id: bearer-auth
      type: http
      scheme: bearer
tools:
  - name: get_weather
    description: Get current weather
    args:
      - name: city
        type: string
        required: true
    requestTemplate:
      url: "https://api.weather.com/v1/current?city={{.args.city}}"
      method: GET
    responseTemplate:
      body: "Weather: {{.temperature}}°C"
```

### 2. ToolSet Configuration (Composed Server)

Validates composed servers that aggregate tools from multiple servers:

```yaml
toolSet:
  name: ai-assistant-tools
  serverTools:
    - serverName: weather-api
      tools: ["get_weather", "get_forecast"]
    - serverName: search-api
      tools: ["web_search"]
```

### 3. Pre-registered Go-based Server

For pre-registered Go-based servers, validation focuses on basic structure and skips server instance validation:

```yaml
server:
  name: custom-go-server
  config:
    database_url: "postgres://localhost:5432/mydb"
allowTools: ["query_database"]
```

## Validation Result

The `ValidationResult` struct provides detailed information about the validation:

```go
type ValidationResult struct {
    IsValid    bool   `json:"isValid"`     // Whether the configuration is valid
    Error      error  `json:"error"`       // Validation error if any
    ServerName string `json:"serverName"`  // Parsed server name
    IsComposed bool   `json:"isComposed"`  // Whether it's a composed server
}
```

## Architecture

The validator reuses the core parsing logic from the main MCP server implementation through dependency injection:

- **parseConfigCore**: Core parsing logic with configurable dependencies
- **ConfigOptions**: Dependency config options
- **SkipPreRegisteredServers**: Flag to skip validation of pre-registered Go servers

This approach ensures:
- **Consistency**: Same validation logic as runtime
- **Maintainability**: Single source of truth for parsing logic
- **Minimal Code Duplication**: Reuses existing implementation

## Testing

Run the tests to verify the validator works correctly:

```bash
cd pkg/mcp/validator
go test -v
```

The test suite covers:
- REST server configuration validation
- ToolSet configuration validation  
- Pre-registered server handling
- Invalid configuration detection
- Error cases

## Error Handling

The validator provides detailed error messages for common configuration issues:

- Missing required fields (e.g., `server.name`)
- Invalid JSON structure
- Malformed tool definitions
- Invalid template syntax
- Missing server or toolSet configuration

These errors help developers quickly identify and fix configuration problems before deployment.
