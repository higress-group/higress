# MCP Server Implementation Guide

> **🌟 Try it now!** Experience Higress-hosted Remote MCP Servers at [https://mcp.higress.ai/](https://mcp.higress.ai/). This platform allows you to see firsthand how Higress can host and manage Remote MCP Servers.
> 
> ![Higress MCP Server Platform](https://img.alicdn.com/imgextra/i2/O1CN01nmVa0a1aChgpyyWOX_!!6000000003294-0-tps-3430-1742.jpg)

## Background

  Higress, as an Envoy-based API gateway, supports hosting MCP Servers through its plugin mechanism. MCP (Model Context Protocol) is essentially an AI-friendly API that enables AI Agents to more easily call various tools and services. Higress provides unified capabilities for authentication, authorization, rate limiting, and observability for tool calls, simplifying the development and deployment of AI applications.

  ![](https://img.alicdn.com/imgextra/i1/O1CN01wv8H4g1mS4MUzC1QC_!!6000000004952-2-tps-1764-597.png)

  By hosting MCP Servers with Higress, you can achieve:
  - Unified authentication and authorization mechanisms, ensuring the security of AI tool calls
  - Fine-grained rate limiting to prevent abuse and resource exhaustion
  - Comprehensive audit logs recording all tool call behaviors
  - Rich observability for monitoring the performance and health of tool calls
  - Simplified deployment and management through Higress's plugin mechanism for quickly adding new MCP Servers

This guide explains how to implement a Model Context Protocol (MCP) server using the Higress WASM Go SDK. MCP servers provide tools and resources that extend the capabilities of AI assistants.

 [**MCP Server QuickStart**](https://higress.cn/en/ai/mcp-quick-start/)

 [**Wasm Plugin Hub**](https://higress.cn/en/plugin/)

## Overview

An MCP server is a standalone application that communicates with AI assistants through the Model Context Protocol. It can provide:

- **Tools**: Functions that can be called by the AI to perform specific tasks
- **Resources**: Data that can be accessed by the AI

> **Note**: MCP server plugins require Higress version 2.1.0 or higher to be used.

## Project Structure

A typical MCP server project has the following structure:

```
my-mcp-server/
├── go.mod                 # Go module definition
├── go.sum                 # Go module checksums
├── main.go                # Entry point that registers tools and resources
└── tools/
    └── my_tool.go         # Tool implementation
```

## Server Configuration

Define a configuration structure for your MCP server to store settings like API keys:

```go
// config/config.go
package config

type MyServerConfig struct {
    ApiKey string `json:"apiKey"`
}
```

## Tool Implementation

Each tool should be implemented as a struct with the following methods:

1. `Description()`: Returns a description of the tool
2. `InputSchema()`: Returns the JSON schema for the tool's input parameters
3. `Create()`: Creates a new instance of the tool with the provided parameters
4. `Call()`: Executes the tool's functionality

Example:

```go
// tools/my_tool.go
package tools

import (
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    
    "my-mcp-server/config"
    "github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/server"
    "github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
)

// Define your tool structure with input parameters
type MyTool struct {
    Param1 string `json:"param1" jsonschema_description:"Description of param1" jsonschema:"example=example value"`
    Param2 int    `json:"param2,omitempty" jsonschema_description:"Description of param2" jsonschema:"default=5"`
}

// Description returns the description field for the MCP tool definition.
// This corresponds to the "description" field in the MCP tool JSON response,
// which provides a human-readable explanation of the tool's purpose and usage.
func (t MyTool) Description() string {
    return `Detailed description of what this tool does and when to use it.`
}

// InputSchema returns the inputSchema field for the MCP tool definition.
// This corresponds to the "inputSchema" field in the MCP tool JSON response,
// which defines the JSON Schema for the tool's input parameters, including
// property types, descriptions, and required fields.
func (t MyTool) InputSchema() map[string]any {
    return server.ToInputSchema(&MyTool{})
}

// Create instantiates a new tool instance based on the input parameters
// from an MCP tool call. It deserializes the JSON parameters into a struct,
// applying default values for optional fields, and returns the configured tool instance.
func (t MyTool) Create(params []byte) server.Tool {
    myTool := &MyTool{
        Param2: 5, // Default value
    }
    json.Unmarshal(params, &myTool)
    return myTool
}

// Call implements the core logic for handling an MCP tool call. This method is executed
// when the tool is invoked through the MCP framework. It processes the configured parameters,
// makes any necessary API requests, and formats the results to be returned to the caller.
func (t MyTool) Call(ctx server.HttpContext, s server.Server) error {
    // Get server configuration
    serverConfig := &config.MyServerConfig{}
    s.GetConfig(serverConfig)
    if serverConfig.ApiKey == "" {
        return errors.New("missing api key in server configuration")
    }
    
    // Implement your tool's logic here
    // ...
    
    // Return results
    utils.SendMCPToolTextResult(ctx, fmt.Sprintf("Result: %s, %d", t.Param1, t.Param2))
    return nil
}
```

## Tool Loading

For better organization, you can create a separate file to load all your tools:

```go
// tools/load_tools.go
package tools

import (
    "github.com/alibaba/higress/plugins/wasm-go/pkg/mcp"
    "github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/server"
)

func LoadTools(server *mcp.MCPServer) server.Server {
    return server.AddMCPTool("my_tool", &MyTool{}).
        AddMCPTool("another_tool", &AnotherTool{})
        // Add more tools as needed
}
```

This approach to organizing code facilitates integration with the all-in-one MCP server plugin. The all-in-one plugin combines multiple MCP servers into a single plugin, reducing the overhead of deploying multiple plugins on the gateway.

### All-in-One Integration

The all-in-one plugin packages multiple MCP servers into a single WASM binary. Each MCP server maintains its own identity and configuration, but they share the same plugin instance. Here's an example of how multiple MCP servers are integrated in the all-in-one plugin:

```go
// all-in-one/main.go
package main

import (
    amap "amap-tools/tools"
    quark "quark-search/tools"
    
    "github.com/alibaba/higress/plugins/wasm-go/pkg/mcp"
)

func main() {}

func init() {
    mcp.LoadMCPServer(mcp.AddMCPServer("quark-search",
        quark.LoadTools(&mcp.MCPServer{})))
    mcp.LoadMCPServer(mcp.AddMCPServer("amap-tools",
        amap.LoadTools(&mcp.MCPServer{})))
    mcp.InitMCPServer()
}
```

The configuration for the all-in-one plugin follows the same pattern as individual MCP server plugins. The `name` field in the server configuration is used to identify and route requests to the appropriate MCP server within the all-in-one plugin.

## REST-to-MCP Configuration

Higress supports a special REST-to-MCP configuration that allows you to convert REST APIs to MCP tools without writing any code. This is useful for quickly integrating existing REST APIs with AI assistants. This capability is built into all MCP servers and can be used with the all-in-one plugin. The implementation is available at [rest_server.go](https://github.com/alibaba/higress/blob/wasm-go-1.24/plugins/wasm-go/pkg/mcp/server/rest_server.go).

### Configuration Format

To use the REST-to-MCP feature, you need to define your tools in the plugin configuration:

```yaml
server:
  name: rest-amap-server
  config:
    apiKey: your-api-key-here
tools:
- name: maps-geo
  description: "Convert structured address information to latitude and longitude coordinates. Supports parsing landmarks, scenic spots, and building names into coordinates."
  args:
  - name: address
    description: "The structured address to parse"
    required: true
  - name: city
    description: "The city to search in"
    required: false
  requestTemplate:
    url: "https://restapi.amap.com/v3/geocode/geo?key={{.config.apiKey}}&address={{.args.address}}&city={{.args.city}}&source=ts_mcp"
    method: GET
    headers:
    - key: x-api-key
      value: "{{.config.apiKey}}"
    - key: Content-Type
      value: application/json
  responseTemplate:
    body: |
      # Geocoding Information
      {{- range $index, $geo := .Geocodes }}
      ## Location {{add $index 1}}

      - **Country**: {{ $geo.Country }}
      - **Province**: {{ $geo.Province }}
      - **City**: {{ $geo.City }}
      - **City Code**: {{ $geo.Citycode }}
      - **District**: {{ $geo.District }}
      - **Street**: {{ $geo.Street }}
      - **Number**: {{ $geo.Number }}
      - **Administrative Code**: {{ $geo.Adcode }}
      - **Coordinates**: {{ $geo.Location }}
      - **Level**: {{ $geo.Level }}
      {{- end }}
```

### Template Syntax

The REST-to-MCP feature uses the [GJSON Template](https://github.com/higress-group/gjson_template) library for template rendering, which combines Go's template syntax with GJSON's powerful path syntax:

- **Request Templates**: Used to construct the HTTP request URL, headers, and body
  - Access configuration values with `.config.fieldName`
  - Access tool arguments with `.args.argName`

- **Response Templates**: Used to transform the HTTP response into a format suitable for AI consumption
  - Access JSON response fields using GJSON path syntax
  - Use template functions like `add`, `upper`, `lower`, etc.
  - Use control structures like `if`, `range`, etc.

GJSON Template includes all of [Sprig](https://github.com/Masterminds/sprig)'s functions, providing a rich set of over 70 template functions for string manipulation, math operations, date formatting, list processing, and more. This makes GJSON Template functionally equivalent to Helm's template capabilities.

Some commonly used Sprig functions include:

- **String manipulation**: `trim`, `upper`, `lower`, `replace`, `plural`, `nospace`
- **Math operations**: `add`, `sub`, `mul`, `div`, `max`, `min`
- **Date formatting**: `now`, `date`, `dateInZone`, `dateModify`
- **List operations**: `list`, `first`, `last`, `uniq`, `sortAlpha`
- **Dictionary operations**: `dict`, `get`, `set`, `hasKey`, `pluck`
- **Flow control**: `ternary`, `default`, `empty`, `coalesce`
- **Type conversion**: `toString`, `toJson`, `toPrettyJson`, `toRawJson`
- **Encoding/decoding**: `b64enc`, `b64dec`, `urlquery`, `urlqueryescape`
- **UUID generation**: `uuidv4`

For a complete reference of all available functions, see the [Helm documentation on functions](https://helm.sh/docs/chart_template_guide/function_list/), as GJSON Template includes the same function set.

### GJSON Path Syntax

GJSON Template supports the full GJSON path syntax, which provides powerful JSON querying capabilities:

- **Dot notation**: `address.city`
- **Array indexing**: `users.0.name`
- **Array iteration**: `users.#.name`
- **Wildcards**: `users.*.name`
- **Array filtering**: `users.#(age>=30)#.name`
- **Modifiers**: `users.@reverse.#.name`
- **Multipath**: `{name:users.0.name,count:users.#}`
- **Escape characters**: `path.with\.dot`

For more complex queries, you can use the `gjson` function directly in your templates:

```
<!-- Using the gjson function for complex queries -->
Active users: {{gjson "users.#(active==true)#.name"}}

<!-- Array filtering with multiple conditions -->
Active developers over 30: {{gjson "users.#(active==true && age>30)#.name"}}

<!-- Using modifiers -->
User names (reversed): {{gjson "users.@reverse.#.name"}}

<!-- Iterating over filtered results -->
Admins:
{{range $user := gjson "users.#(roles.#(==admin)>0)#"}}
  - {{$user.name}} ({{$user.age}})
{{end}}
```

For a complete reference of GJSON path syntax, see the [GJSON documentation](https://github.com/tidwall/gjson#path-syntax).

### AI Prompt for Template Generation

When working with AI assistants to generate templates for REST-to-MCP configuration, you can use the following prompt:

```
Please help me create a REST-to-MCP configuration for Higress that converts a REST API to an MCP tool. The configuration should follow this format:

```yaml
server:
  name: rest-api-server
  config:
    apiKey: your-api-key-here
tools:
- name: tool-name
  description: "Detailed description of what this tool does"
  args:
  - name: arg1
    description: "Description of argument 1"
    required: true
  - name: arg2
    description: "Description of argument 2"
    required: false
    default: "default value"
  requestTemplate:
    url: "https://api.example.com/endpoint?key={{.config.apiKey}}&param={{.args.arg1}}"
    method: GET
    headers:
    - key: x-api-key
      value: "{{.config.apiKey}}"
    - key: Content-Type
      value: application/json
    body: |
      {
        "param1": "{{.args.arg1}}",
        "param2": "{{.args.arg2}}"
      }
  responseTemplate:
    body: |
      # Result
      {{- range $index, $item := .items }}
      ## Item {{add $index 1}}
      - **Name**: {{ $item.name }}
      - **Value**: {{ $item.value }}
      {{- end }}
```

The REST API I want to convert is [describe your API here, including endpoints, parameters, and response format].

Please generate a complete configuration that:
1. Has a descriptive name and appropriate server configuration
2. Defines all necessary arguments with clear descriptions and appropriate required/default values
3. Creates a requestTemplate that correctly formats the API request, including headers with template values
4. Creates a responseTemplate that transforms the API response into a readable format for AI consumption

The templates use GJSON Template syntax (https://github.com/higress-group/gjson_template), which combines Go templates with GJSON path syntax for JSON processing. The template engine supports:

1. Basic dot notation for accessing fields: {{.fieldName}}
2. The gjson function for complex queries: {{gjson "users.#(active==true)#.name"}}
3. All Sprig template functions (like Helm): {{add}}, {{upper}}, {{lower}}, {{date}}, etc.
4. Control structures: {{if}}, {{range}}, {{with}}, etc.
5. Variable assignment: {{$var := .value}}

For complex JSON responses, consider using GJSON's powerful filtering and querying capabilities to extract and format the most relevant information.
```

## Main Entry Point

The main.go file is the entry point for your MCP server. It registers your tools and resources:

```go
// main.go
package main

import (
    "my-mcp-server/tools"
    
    "github.com/alibaba/higress/plugins/wasm-go/pkg/mcp"
)

func main() {}

func init() {
    mcp.LoadMCPServer(mcp.AddMCPServer("my-mcp-server",
        tools.LoadTools(&mcp.MCPServer{})))
    mcp.InitMCPServer()
}
```

## Plugin Configuration

When deploying your MCP server as a Higress plugin, you need to configure it in the Higress configuration. Here's an example configuration:

```yaml
server:
  # MCP server name - MUST match the name used in mcp.AddMCPServer() in your code
  name: my-mcp-server
  # MCP server configuration
  config:
    apiKey: your-api-key-here
  # Optional: If configured, acts as a whitelist - only tools listed here can be called
allowTools:
- my_tool
- another_tool
```

> **Important**: The `name` field in the server configuration must exactly match the server name used in the `mcp.AddMCPServer()` call in your code. This is how the system identifies which MCP server should handle the request.

## Dependencies

Your MCP server must use a specific version of the wasm-go SDK that supports Go 1.24's WebAssembly compilation features:

```bash
# Add the required dependency
go get github.com/alibaba/higress/plugins/wasm-go
```

Make sure your go.mod file specifies Go 1.24:

```
module my-mcp-server

go 1.24

require (
    github.com/alibaba/higress/plugins/wasm-go v1.4.4-0.20250324133957-dab499f6ade6
    // other dependencies
)
```

## Building the WASM Binary

To compile your Go code into a WebAssembly (WASM) file, use the following command:

```bash
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o main.wasm main.go
```

This command sets the target operating system to `wasip1` (WebAssembly System Interface) and architecture to `wasm` (WebAssembly), then builds your code as a C-shared library and outputs it as `main.wasm`.

## Using the Makefile

A Makefile is provided to simplify the build process. It includes the following targets:

- `make build`: Builds the WASM binary for your MCP server
- `make build-image`: Builds a Docker image containing your MCP server
- `make build-push`: Builds and pushes the Docker image to a registry
- `make clean`: Removes build artifacts
- `make help`: Shows available targets and variables

You can customize the build by setting the following variables:

```bash
# Build with a custom server name
make SERVER_NAME=my-mcp-server build

# Build with a custom registry
make REGISTRY=my-registry.example.com/ build-image

# Build with a specific version tag
make SERVER_VERSION=1.0.0 build-image
```

## Testing

You can create unit tests for your tools to verify their functionality:

```go
// tools/my_tool_test.go
package tools

import (
    "encoding/json"
    "fmt"
    "testing"
)

// TestMyToolInputSchema tests the InputSchema method of MyTool
// to verify that the JSON schema configuration is correct.
func TestMyToolInputSchema(t *testing.T) {
    myTool := MyTool{}
    schema := myTool.InputSchema()
    
    schemaJSON, err := json.MarshalIndent(schema, "", "  ")
    if err != nil {
        t.Fatalf("Failed to marshal schema to JSON: %v", err)
    }
    
    fmt.Printf("MyTool InputSchema:\n%s\n", string(schemaJSON))
    
    if len(schema) == 0 {
        t.Error("InputSchema returned an empty schema")
    }
}
