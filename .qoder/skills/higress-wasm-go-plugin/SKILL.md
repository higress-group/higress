---
name: higress-wasm-go-plugin
description: Develop Higress WASM plugins using Go 1.24+. Use when creating, modifying, or debugging Higress gateway plugins for HTTP request/response processing, external service calls, Redis integration, or custom gateway logic.
---

# Higress WASM Go Plugin Development

Develop Higress gateway WASM plugins using Go language with the `wasm-go` SDK.

## Quick Start

### Project Setup

```bash
# Create project directory
mkdir my-plugin && cd my-plugin

# Initialize Go module
go mod init my-plugin

# Set proxy (China)
go env -w GOPROXY=https://proxy.golang.com.cn,direct

# Download dependencies
go get github.com/higress-group/proxy-wasm-go-sdk@go-1.24
go get github.com/higress-group/wasm-go@main
go get github.com/tidwall/gjson
```

### Minimal Plugin Template

```go
package main

import (
    "github.com/higress-group/wasm-go/pkg/wrapper"
    "github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
    "github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
    "github.com/tidwall/gjson"
)

func main() {}

func init() {
    wrapper.SetCtx(
        "my-plugin",
        wrapper.ParseConfig(parseConfig),
        wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
    )
}

type MyConfig struct {
    Enabled bool
}

func parseConfig(json gjson.Result, config *MyConfig) error {
    config.Enabled = json.Get("enabled").Bool()
    return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    if config.Enabled {
        proxywasm.AddHttpRequestHeader("x-my-header", "hello")
    }
    return types.HeaderContinue
}
```

### Compile

```bash
go mod tidy
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o main.wasm ./
```

## Core Concepts

### Plugin Lifecycle

1. **init()** - Register plugin with `wrapper.SetCtx()`
2. **parseConfig** - Parse YAML config (auto-converted to JSON)
3. **HTTP processing phases** - Handle requests/responses

### HTTP Processing Phases

| Phase | Trigger | Handler |
|-------|---------|---------|
| Request Headers | Gateway receives client request headers | `ProcessRequestHeaders` |
| Request Body | Gateway receives client request body | `ProcessRequestBody` |
| Response Headers | Gateway receives backend response headers | `ProcessResponseHeaders` |
| Response Body | Gateway receives backend response body | `ProcessResponseBody` |
| Stream Done | HTTP stream completes | `ProcessStreamDone` |

### Action Return Values

| Action | Behavior |
|--------|----------|
| `types.HeaderContinue` | Continue to next filter |
| `types.HeaderStopIteration` | Stop header processing, wait for body |
| `types.HeaderStopAllIterationAndWatermark` | Stop all processing, buffer data, call `proxywasm.ResumeHttpRequest/Response()` to resume |

## API Reference

### HttpContext Methods

```go
// Request info (cached, safe to call in any phase)
ctx.Scheme()   // :scheme
ctx.Host()     // :authority
ctx.Path()     // :path
ctx.Method()   // :method

// Body handling
ctx.HasRequestBody()        // Check if request has body
ctx.HasResponseBody()       // Check if response has body
ctx.DontReadRequestBody()   // Skip reading request body
ctx.DontReadResponseBody()  // Skip reading response body
ctx.BufferRequestBody()     // Buffer instead of stream
ctx.BufferResponseBody()    // Buffer instead of stream

// Content detection
ctx.IsWebsocket()           // Check WebSocket upgrade
ctx.IsBinaryRequestBody()   // Check binary content
ctx.IsBinaryResponseBody()  // Check binary content

// Context storage
ctx.SetContext(key, value)
ctx.GetContext(key)
ctx.GetStringContext(key, defaultValue)
ctx.GetBoolContext(key, defaultValue)

// Custom logging
ctx.SetUserAttribute(key, value)
ctx.WriteUserAttributeToLog()
```

### Header/Body Operations (proxywasm)

```go
// Request headers
proxywasm.GetHttpRequestHeader(name)
proxywasm.AddHttpRequestHeader(name, value)
proxywasm.ReplaceHttpRequestHeader(name, value)
proxywasm.RemoveHttpRequestHeader(name)
proxywasm.GetHttpRequestHeaders()
proxywasm.ReplaceHttpRequestHeaders(headers)

// Response headers
proxywasm.GetHttpResponseHeader(name)
proxywasm.AddHttpResponseHeader(name, value)
proxywasm.ReplaceHttpResponseHeader(name, value)
proxywasm.RemoveHttpResponseHeader(name)
proxywasm.GetHttpResponseHeaders()
proxywasm.ReplaceHttpResponseHeaders(headers)

// Request body (only in body phase)
proxywasm.GetHttpRequestBody(start, size)
proxywasm.ReplaceHttpRequestBody(body)
proxywasm.AppendHttpRequestBody(data)
proxywasm.PrependHttpRequestBody(data)

// Response body (only in body phase)
proxywasm.GetHttpResponseBody(start, size)
proxywasm.ReplaceHttpResponseBody(body)
proxywasm.AppendHttpResponseBody(data)
proxywasm.PrependHttpResponseBody(data)

// Direct response
proxywasm.SendHttpResponse(statusCode, headers, body, grpcStatus)

// Flow control
proxywasm.ResumeHttpRequest()   // Resume paused request
proxywasm.ResumeHttpResponse()  // Resume paused response
```

## Common Patterns

### External HTTP Call

See [references/http-client.md](references/http-client.md) for complete HTTP client patterns.

```go
func parseConfig(json gjson.Result, config *MyConfig) error {
    serviceName := json.Get("serviceName").String()
    servicePort := json.Get("servicePort").Int()
    config.client = wrapper.NewClusterClient(wrapper.FQDNCluster{
        FQDN: serviceName,
        Port: servicePort,
    })
    return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    err := config.client.Get("/api/check", nil, func(statusCode int, headers http.Header, body []byte) {
        if statusCode != 200 {
            proxywasm.SendHttpResponse(403, nil, []byte("Forbidden"), -1)
            return
        }
        proxywasm.ResumeHttpRequest()
    }, 3000) // timeout ms
    
    if err != nil {
        return types.HeaderContinue // fallback on error
    }
    return types.HeaderStopAllIterationAndWatermark
}
```

### Redis Integration

See [references/redis-client.md](references/redis-client.md) for complete Redis patterns.

```go
func parseConfig(json gjson.Result, config *MyConfig) error {
    config.redis = wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
        FQDN: json.Get("redisService").String(),
        Port: json.Get("redisPort").Int(),
    })
    return config.redis.Init(
        json.Get("username").String(),
        json.Get("password").String(),
        json.Get("timeout").Int(),
    )
}
```

### Multi-level Config

插件配置支持在控制台不同级别设置：全局、域名级、路由级。控制面会自动处理配置的优先级和匹配逻辑，插件代码中通过 `parseConfig` 解析到的就是当前请求匹配到的配置。

## Local Testing

See [references/local-testing.md](references/local-testing.md) for Docker Compose setup.

## Advanced Topics

See [references/advanced-patterns.md](references/advanced-patterns.md) for:
- Streaming body processing
- Route call pattern
- Tick functions (periodic tasks)
- Leader election
- Memory management
- Custom logging

## Best Practices

1. **Never call Resume after SendHttpResponse** - Response auto-resumes
2. **Check HasRequestBody() before returning HeaderStopIteration** - Avoids blocking
3. **Use cached ctx methods** - `ctx.Path()` works in any phase, `GetHttpRequestHeader(":path")` only in header phase
4. **Handle external call failures gracefully** - Return `HeaderContinue` on error to avoid blocking
5. **Set appropriate timeouts** - Default HTTP call timeout is 500ms
