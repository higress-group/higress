# Common Nginx Snippet to WASM Plugin Patterns

## Header Manipulation

### Add Response Header

**Nginx snippet:**
```nginx
more_set_headers "X-Custom-Header: custom-value";
more_set_headers "X-Request-ID: $request_id";
```

**WASM plugin:**
```go
func onHttpResponseHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    proxywasm.AddHttpResponseHeader("X-Custom-Header", "custom-value")
    
    // For request ID, get from request context
    if reqId, err := proxywasm.GetHttpRequestHeader("x-request-id"); err == nil {
        proxywasm.AddHttpResponseHeader("X-Request-ID", reqId)
    }
    return types.HeaderContinue
}
```

### Remove Headers

**Nginx snippet:**
```nginx
more_clear_headers "Server";
more_clear_headers "X-Powered-By";
```

**WASM plugin:**
```go
func onHttpResponseHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    proxywasm.RemoveHttpResponseHeader("Server")
    proxywasm.RemoveHttpResponseHeader("X-Powered-By")
    return types.HeaderContinue
}
```

### Conditional Header

**Nginx snippet:**
```nginx
if ($http_x_custom_flag = "enabled") {
    more_set_headers "X-Feature: active";
}
```

**WASM plugin:**
```go
func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    flag, _ := proxywasm.GetHttpRequestHeader("x-custom-flag")
    if flag == "enabled" {
        proxywasm.AddHttpRequestHeader("X-Feature", "active")
    }
    return types.HeaderContinue
}
```

## Request Validation

### Block by Path Pattern

**Nginx snippet:**
```nginx
if ($request_uri ~* "(\.php|\.asp|\.aspx)$") {
    return 403;
}
```

**WASM plugin:**
```go
import "regexp"

type MyConfig struct {
    BlockPattern *regexp.Regexp
}

func parseConfig(json gjson.Result, config *MyConfig) error {
    pattern := json.Get("blockPattern").String()
    if pattern == "" {
        pattern = `\.(php|asp|aspx)$`
    }
    config.BlockPattern = regexp.MustCompile(pattern)
    return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    path := ctx.Path()
    if config.BlockPattern.MatchString(path) {
        proxywasm.SendHttpResponse(403, nil, []byte("Forbidden"), -1)
        return types.HeaderStopAllIterationAndWatermark
    }
    return types.HeaderContinue
}
```

### Block by User Agent

**Nginx snippet:**
```nginx
if ($http_user_agent ~* "(bot|crawler|spider)") {
    return 403;
}
```

**WASM plugin:**
```go
func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    ua, _ := proxywasm.GetHttpRequestHeader("user-agent")
    ua = strings.ToLower(ua)
    
    blockedPatterns := []string{"bot", "crawler", "spider"}
    for _, pattern := range blockedPatterns {
        if strings.Contains(ua, pattern) {
            proxywasm.SendHttpResponse(403, nil, []byte("Blocked"), -1)
            return types.HeaderStopAllIterationAndWatermark
        }
    }
    return types.HeaderContinue
}
```

### Request Size Validation

**Nginx snippet:**
```nginx
if ($content_length > 10485760) {
    return 413;
}
```

**WASM plugin:**
```go
func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    clStr, _ := proxywasm.GetHttpRequestHeader("content-length")
    if cl, err := strconv.ParseInt(clStr, 10, 64); err == nil {
        if cl > 10*1024*1024 { // 10MB
            proxywasm.SendHttpResponse(413, nil, []byte("Request too large"), -1)
            return types.HeaderStopAllIterationAndWatermark
        }
    }
    return types.HeaderContinue
}
```

## Request Modification

### URL Rewrite with Logic

**Nginx snippet:**
```nginx
set $backend "default";
if ($http_x_version = "v2") {
    set $backend "v2";
}
rewrite ^/api/(.*)$ /api/$backend/$1 break;
```

**WASM plugin:**
```go
func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    version, _ := proxywasm.GetHttpRequestHeader("x-version")
    backend := "default"
    if version == "v2" {
        backend = "v2"
    }
    
    path := ctx.Path()
    if strings.HasPrefix(path, "/api/") {
        newPath := "/api/" + backend + path[4:]
        proxywasm.ReplaceHttpRequestHeader(":path", newPath)
    }
    return types.HeaderContinue
}
```

### Add Query Parameter

**Nginx snippet:**
```nginx
if ($args !~ "source=") {
    set $args "${args}&source=gateway";
}
```

**WASM plugin:**
```go
func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    path := ctx.Path()
    if !strings.Contains(path, "source=") {
        separator := "?"
        if strings.Contains(path, "?") {
            separator = "&"
        }
        newPath := path + separator + "source=gateway"
        proxywasm.ReplaceHttpRequestHeader(":path", newPath)
    }
    return types.HeaderContinue
}
```

## Lua Script Conversion

### Simple Lua Access Check

**Nginx Lua:**
```lua
access_by_lua_block {
    local token = ngx.var.http_authorization
    if not token or token == "" then
        ngx.exit(401)
    end
}
```

**WASM plugin:**
```go
func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    token, _ := proxywasm.GetHttpRequestHeader("authorization")
    if token == "" {
        proxywasm.SendHttpResponse(401, [][2]string{
            {"WWW-Authenticate", "Bearer"},
        }, []byte("Unauthorized"), -1)
        return types.HeaderStopAllIterationAndWatermark
    }
    return types.HeaderContinue
}
```

### Lua with Redis

**Nginx Lua:**
```lua
access_by_lua_block {
    local redis = require "resty.redis"
    local red = redis:new()
    red:connect("127.0.0.1", 6379)
    
    local ip = ngx.var.remote_addr
    local count = red:incr("rate:" .. ip)
    if count > 100 then
        ngx.exit(429)
    end
    red:expire("rate:" .. ip, 60)
}
```

**WASM plugin:**
```go
// See references/redis-client.md in higress-wasm-go-plugin skill
func parseConfig(json gjson.Result, config *MyConfig) error {
    config.redis = wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
        FQDN: json.Get("redisService").String(),
        Port: json.Get("redisPort").Int(),
    })
    return config.redis.Init("", json.Get("redisPassword").String(), 1000)
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    ip, _ := proxywasm.GetHttpRequestHeader("x-real-ip")
    if ip == "" {
        ip, _ = proxywasm.GetHttpRequestHeader("x-forwarded-for")
    }
    
    key := "rate:" + ip
    err := config.redis.Incr(key, func(val int) {
        if val > 100 {
            proxywasm.SendHttpResponse(429, nil, []byte("Rate limited"), -1)
            return
        }
        config.redis.Expire(key, 60, nil)
        proxywasm.ResumeHttpRequest()
    })
    
    if err != nil {
        return types.HeaderContinue // Fallback on Redis error
    }
    return types.HeaderStopAllIterationAndWatermark
}
```

## Response Modification

### Inject Script/Content

**Nginx snippet:**
```nginx
sub_filter '</head>' '<script src="/tracking.js"></script></head>';
sub_filter_once on;
```

**WASM plugin:**
```go
func init() {
    wrapper.SetCtx(
        "inject-script",
        wrapper.ParseConfig(parseConfig),
        wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
        wrapper.ProcessResponseBody(onHttpResponseBody),
    )
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
    if strings.Contains(contentType, "text/html") {
        ctx.BufferResponseBody()
        proxywasm.RemoveHttpResponseHeader("content-length")
    }
    return types.HeaderContinue
}

func onHttpResponseBody(ctx wrapper.HttpContext, config MyConfig, body []byte) types.Action {
    bodyStr := string(body)
    injection := `<script src="/tracking.js"></script></head>`
    newBody := strings.Replace(bodyStr, "</head>", injection, 1)
    proxywasm.ReplaceHttpResponseBody([]byte(newBody))
    return types.BodyContinue
}
```

## Best Practices

1. **Error Handling**: Always handle external call failures gracefully
2. **Performance**: Cache regex patterns in config, avoid recompiling
3. **Timeout**: Set appropriate timeouts for external calls (default 500ms)
4. **Logging**: Use `proxywasm.LogInfo/Warn/Error` for debugging
5. **Testing**: Test locally with Docker Compose before deploying
