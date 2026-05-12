# Advanced Patterns

## Streaming Body Processing

Process body chunks as they arrive without buffering:

```go
func init() {
    wrapper.SetCtx(
        "streaming-plugin",
        wrapper.ParseConfig(parseConfig),
        wrapper.ProcessStreamingRequestBody(onStreamingRequestBody),
        wrapper.ProcessStreamingResponseBody(onStreamingResponseBody),
    )
}

func onStreamingRequestBody(ctx wrapper.HttpContext, config MyConfig, chunk []byte, isLastChunk bool) []byte {
    // Modify chunk and return
    modified := bytes.ReplaceAll(chunk, []byte("old"), []byte("new"))
    return modified
}

func onStreamingResponseBody(ctx wrapper.HttpContext, config MyConfig, chunk []byte, isLastChunk bool) []byte {
    // Can call external services with NeedPauseStreamingResponse()
    return chunk
}
```

## Buffered Body Processing

Buffer entire body before processing:

```go
func init() {
    wrapper.SetCtx(
        "buffered-plugin",
        wrapper.ParseConfig(parseConfig),
        wrapper.ProcessRequestBody(onRequestBody),
        wrapper.ProcessResponseBody(onResponseBody),
    )
}

func onRequestBody(ctx wrapper.HttpContext, config MyConfig, body []byte) types.Action {
    // Full request body available
    var data map[string]interface{}
    json.Unmarshal(body, &data)
    
    // Modify and replace
    data["injected"] = "value"
    newBody, _ := json.Marshal(data)
    proxywasm.ReplaceHttpRequestBody(newBody)
    
    return types.ActionContinue
}
```

## Route Call Pattern

Call the current route's upstream with modified request:

```go
func onRequestBody(ctx wrapper.HttpContext, config MyConfig, body []byte) types.Action {
    err := ctx.RouteCall("POST", "/modified-path", [][2]string{
        {"Content-Type", "application/json"},
        {"X-Custom", "header"},
    }, body, func(statusCode int, headers [][2]string, body []byte) {
        // Handle response from upstream
        proxywasm.SendHttpResponse(statusCode, headers, body, -1)
    })
    
    if err != nil {
        proxywasm.SendHttpResponse(500, nil, []byte("Route call failed"), -1)
    }
    return types.ActionContinue
}
```

## Tick Functions (Periodic Tasks)

Register periodic background tasks:

```go
func parseConfig(json gjson.Result, config *MyConfig) error {
    // Register tick functions during config parsing
    wrapper.RegisterTickFunc(1000, func() {
        // Executes every 1 second
        log.Info("1s tick")
    })
    
    wrapper.RegisterTickFunc(5000, func() {
        // Executes every 5 seconds
        log.Info("5s tick")
    })
    
    return nil
}
```

## Leader Election

For tasks that should run on only one VM instance:

```go
func init() {
    wrapper.SetCtx(
        "leader-plugin",
        wrapper.PrePluginStartOrReload(onPluginStart),
        wrapper.ParseConfig(parseConfig),
    )
}

func onPluginStart(ctx wrapper.PluginContext) error {
    ctx.DoLeaderElection()
    return nil
}

func parseConfig(json gjson.Result, config *MyConfig) error {
    wrapper.RegisterTickFunc(10000, func() {
        if ctx.IsLeader() {
            // Only leader executes this
            log.Info("Leader task")
        }
    })
    return nil
}
```

## Plugin Context Storage

Store data across requests at plugin level:

```go
type MyConfig struct {
    // Config fields
}

func init() {
    wrapper.SetCtx(
        "context-plugin",
        wrapper.ParseConfigWithContext(parseConfigWithContext),
        wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
    )
}

func parseConfigWithContext(ctx wrapper.PluginContext, json gjson.Result, config *MyConfig) error {
    // Store in plugin context (survives across requests)
    ctx.SetContext("initTime", time.Now().Unix())
    return nil
}
```

## Rule-Level Config Isolation

Enable graceful degradation when rule config parsing fails:

```go
func init() {
    wrapper.SetCtx(
        "isolated-plugin",
        wrapper.PrePluginStartOrReload(func(ctx wrapper.PluginContext) error {
            ctx.EnableRuleLevelConfigIsolation()
            return nil
        }),
        wrapper.ParseOverrideConfig(parseGlobal, parseRule),
    )
}

func parseGlobal(json gjson.Result, config *MyConfig) error {
    // Parse global config
    return nil
}

func parseRule(json gjson.Result, global MyConfig, config *MyConfig) error {
    // Parse per-rule config, inheriting from global
    *config = global // Copy global defaults
    // Override with rule-specific values
    return nil
}
```

## Memory Management

Configure automatic VM rebuild to prevent memory leaks:

```go
func init() {
    wrapper.SetCtxWithOptions(
        "memory-managed-plugin",
        wrapper.ParseConfig(parseConfig),
        wrapper.WithRebuildAfterRequests(10000),           // Rebuild after 10k requests
        wrapper.WithRebuildMaxMemBytes(100*1024*1024),     // Rebuild at 100MB
        wrapper.WithMaxRequestsPerIoCycle(20),             // Limit concurrent requests
    )
}
```

## Custom Logging

Add structured fields to access logs:

```go
func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    // Set custom attributes
    ctx.SetUserAttribute("user_id", "12345")
    ctx.SetUserAttribute("request_type", "api")
    
    return types.HeaderContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    // Write to access log
    ctx.WriteUserAttributeToLog()
    
    // Or write to trace spans
    ctx.WriteUserAttributeToTrace()
    
    return types.HeaderContinue
}
```

## Disable Re-routing

Prevent Envoy from recalculating routes after header modification:

```go
func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    // Call BEFORE modifying headers
    ctx.DisableReroute()
    
    // Now safe to modify headers without triggering re-route
    proxywasm.ReplaceHttpRequestHeader(":path", "/new-path")
    
    return types.HeaderContinue
}
```

## Buffer Limits

Set per-request buffer limits to control memory usage:

```go
func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    // Allow larger request bodies for this request
    ctx.SetRequestBodyBufferLimit(10 * 1024 * 1024) // 10MB
    return types.HeaderContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    // Allow larger response bodies
    ctx.SetResponseBodyBufferLimit(50 * 1024 * 1024) // 50MB
    return types.HeaderContinue
}
```
