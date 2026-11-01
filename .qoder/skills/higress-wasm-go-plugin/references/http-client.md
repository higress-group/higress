# HTTP Client Reference

## Cluster Types

### FQDNCluster (Most Common)

For services registered in Higress with FQDN:

```go
wrapper.NewClusterClient(wrapper.FQDNCluster{
    FQDN: "my-service.dns",      // Service FQDN with suffix
    Port: 8080,
    Host: "optional-host-header", // Optional
})
```

Common FQDN suffixes:
- `.dns` - DNS service
- `.static` - Static IP service (port defaults to 80)
- `.nacos` - Nacos service

### K8sCluster

For Kubernetes services:

```go
wrapper.NewClusterClient(wrapper.K8sCluster{
    ServiceName: "my-service",
    Namespace:   "default",
    Port:        8080,
    Version:     "",    // Optional subset version
})
// Generates: outbound|8080||my-service.default.svc.cluster.local
```

### NacosCluster

For Nacos registry services:

```go
wrapper.NewClusterClient(wrapper.NacosCluster{
    ServiceName: "my-service",
    Group:       "DEFAULT-GROUP",
    NamespaceID: "public",
    Port:        8080,
    IsExtRegistry: false, // true for EDAS/SAE
})
```

### StaticIpCluster

For static IP services:

```go
wrapper.NewClusterClient(wrapper.StaticIpCluster{
    ServiceName: "my-service",
    Port:        8080,
})
// Generates: outbound|8080||my-service.static
```

### DnsCluster

For DNS-resolved services:

```go
wrapper.NewClusterClient(wrapper.DnsCluster{
    ServiceName: "my-service",
    Domain:      "api.example.com",
    Port:        443,
})
```

### RouteCluster

Use current route's upstream:

```go
wrapper.NewClusterClient(wrapper.RouteCluster{
    Host: "optional-host-override",
})
```

### TargetCluster

Direct cluster name specification:

```go
wrapper.NewClusterClient(wrapper.TargetCluster{
    Cluster: "outbound|8080||my-service.dns",
    Host:    "api.example.com",
})
```

## HTTP Methods

```go
client.Get(path, headers, callback, timeout...)
client.Post(path, headers, body, callback, timeout...)
client.Put(path, headers, body, callback, timeout...)
client.Patch(path, headers, body, callback, timeout...)
client.Delete(path, headers, body, callback, timeout...)
client.Head(path, headers, callback, timeout...)
client.Options(path, headers, callback, timeout...)
client.Call(method, path, headers, body, callback, timeout...)
```

## Callback Signature

```go
func(statusCode int, responseHeaders http.Header, responseBody []byte)
```

## Complete Example

```go
type MyConfig struct {
    client      wrapper.HttpClient
    requestPath string
    tokenHeader string
}

func parseConfig(json gjson.Result, config *MyConfig) error {
    config.tokenHeader = json.Get("tokenHeader").String()
    if config.tokenHeader == "" {
        return errors.New("missing tokenHeader")
    }
    
    config.requestPath = json.Get("requestPath").String()
    if config.requestPath == "" {
        return errors.New("missing requestPath")
    }
    
    serviceName := json.Get("serviceName").String()
    servicePort := json.Get("servicePort").Int()
    if servicePort == 0 {
        if strings.HasSuffix(serviceName, ".static") {
            servicePort = 80
        }
    }
    
    config.client = wrapper.NewClusterClient(wrapper.FQDNCluster{
        FQDN: serviceName,
        Port: servicePort,
    })
    return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    err := config.client.Get(config.requestPath, nil,
        func(statusCode int, responseHeaders http.Header, responseBody []byte) {
            if statusCode != http.StatusOK {
                log.Errorf("http call failed, status: %d", statusCode)
                proxywasm.SendHttpResponse(http.StatusInternalServerError, nil,
                    []byte("http call failed"), -1)
                return
            }
            
            token := responseHeaders.Get(config.tokenHeader)
            if token != "" {
                proxywasm.AddHttpRequestHeader(config.tokenHeader, token)
            }
            proxywasm.ResumeHttpRequest()
        })

    if err != nil {
        log.Errorf("http call dispatch failed: %v", err)
        return types.HeaderContinue
    }
    return types.HeaderStopAllIterationAndWatermark
}
```

## Important Notes

1. **Cannot use net/http** - Must use wrapper's HTTP client
2. **Default timeout is 500ms** - Pass explicit timeout for longer calls
3. **Callback is async** - Must return `HeaderStopAllIterationAndWatermark` and call `ResumeHttpRequest()` in callback
4. **Error handling** - If dispatch fails, return `HeaderContinue` to avoid blocking
