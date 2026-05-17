# Request ID Generator Plugin - Design Document

## Overview

### Plugin Purpose
The Request ID Generator plugin automatically generates and injects unique request IDs for every HTTP request passing through the Higress gateway. This enables request tracing, debugging, and correlation across distributed microservices.

### Problem It Solves
- **Distributed Tracing**: Without unique request IDs, it's difficult to trace a request's journey across multiple services
- **Debugging**: Hard to correlate logs from different services for the same request
- **Monitoring**: Difficult to track request flow and identify issues in complex service architectures
- **Client Tracking**: Clients cannot easily reference specific requests when reporting issues

### Target Users
- DevOps engineers implementing distributed tracing
- SRE teams monitoring microservice architectures
- Developers debugging distributed systems
- Support teams tracking customer requests

## Functional Design

### Core Feature 1: UUID Generation

**Feature Description:**
Generate a unique UUID v4 for each incoming HTTP request.

**Implementation Approach:**
- Use UUID v4 (random-based) format: `xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx`
- Generate using cryptographically secure random bytes
- Ensure uniqueness with extremely low collision probability (< 1 in 10^36)

**Key Code Logic:**
```go
func generateUUID() string {
    // Generate 16 random bytes
    // Set version (4) and variant bits
    // Format as standard UUID string
    return uuid
}
```

### Core Feature 2: Request Header Injection

**Feature Description:**
Inject the generated request ID into request headers before forwarding to upstream services.

**Implementation Approach:**
- Check if request ID already exists in headers
- If exists and override_existing=false, preserve original ID
- If not exists or override_existing=true, inject new ID
- Use configurable header name (default: X-Request-Id)

**Key Code Logic:**
```go
func injectRequestID(requestID string, config Config) {
    existingID := proxywasm.GetHttpRequestHeader(config.RequestHeader)
    if existingID == "" || config.OverrideExisting {
        proxywasm.AddHttpRequestHeader(config.RequestHeader, requestID)
    }
}
```

### Core Feature 3: Response Header Injection (Optional)

**Feature Description:**
Optionally include the request ID in response headers for client-side tracking.

**Implementation Approach:**
- If response_header is configured, add request ID to response
- Allows clients to reference the request ID when reporting issues
- Useful for customer support and debugging

**Key Code Logic:**
```go
func addResponseHeader(requestID string, headerName string) {
    if headerName != "" {
        proxywasm.AddHttpResponseHeader(headerName, requestID)
    }
}
```

## Configuration Parameters

| Parameter | Type | Required | Description | Default | Validation |
|-----------|------|----------|-------------|---------|------------|
| `enable` | boolean | No | Enable/disable the plugin | `true` | - |
| `request_header` | string | No | Header name for upstream requests | `"X-Request-Id"` | Non-empty string |
| `response_header` | string | No | Header name for client responses | `""` (disabled) | Any string |
| `override_existing` | boolean | No | Override existing request ID | `false` | - |

### Configuration Example

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: request-id-generator
  namespace: higress-system
spec:
  defaultConfig:
    enable: true
    request_header: "X-Request-Id"
    response_header: "X-Request-Id"
    override_existing: false
```

## Technical Implementation

### Technology Selection

**Language**: Go with TinyGo compiler
- Reason: Native Higress wasm-go SDK support
- Excellent performance in WASM environment
- Rich standard library for UUID generation

**Framework**: Higress wasm-go SDK
- Reason: Official SDK with comprehensive API
- Proven reliability and performance
- Active community support

**Dependencies**:
- `github.com/alibaba/higress/plugins/wasm-go` - Higress SDK
- No external dependencies for UUID generation (custom implementation)

### Architecture

```
┌─────────────────────────────────────────────────┐
│              Incoming Request                    │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│         Request ID Generator Plugin              │
│                                                   │
│  1. Parse Configuration                          │
│  2. Check for Existing Request ID                │
│  3. Generate New UUID (if needed)                │
│  4. Inject Request Header                        │
│  5. Store ID for Response Phase                  │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│            Forward to Upstream                   │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│            Upstream Response                     │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│   Inject Response Header (if configured)        │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│            Return to Client                      │
└─────────────────────────────────────────────────┘
```

### Performance Considerations

1. **UUID Generation**: O(1) complexity, < 0.1ms per request
2. **Header Operations**: Minimal overhead using native SDK functions
3. **Memory Usage**: ~100 bytes per request for UUID storage
4. **No Blocking Operations**: All operations are synchronous and fast

### Security Considerations

1. **UUID Unpredictability**: Using cryptographically secure random generation
2. **No Sensitive Data Exposure**: UUIDs don't contain system information
3. **Header Injection Safety**: Proper escaping handled by SDK
4. **Configuration Validation**: Validate all config parameters on load

## Test Plan

### Unit Tests

1. **UUID Generation Test**
   - Verify UUID format correctness
   - Ensure uniqueness across multiple generations
   - Validate UUID v4 version and variant bits

2. **Configuration Parsing Test**
   - Test valid configurations
   - Test invalid configurations (expect errors)
   - Test default values

3. **Header Injection Test**
   - Test request header injection
   - Test response header injection
   - Test override_existing behavior

### Integration Tests

1. **End-to-End Flow Test**
   - Send request without request ID → verify ID is generated
   - Send request with existing ID → verify ID is preserved (default)
   - Send request with override=true → verify new ID is generated

2. **Multi-Service Tracing Test**
   - Verify request ID propagates through multiple services
   - Verify logging correlation works correctly

3. **Performance Test**
   - Measure latency overhead (should be < 1ms)
   - Load test with 1000+ req/s

### Boundary Tests

1. **Edge Cases**
   - Empty request headers
   - Malformed existing request IDs
   - Very long header names
   - Concurrent request handling

2. **Error Scenarios**
   - Invalid configuration
   - Header size limits
   - Plugin initialization failures

## Limitations and Notes

### Known Limitations

1. **UUID Collision**: While extremely unlikely (< 1 in 10^36), UUID collisions are theoretically possible
2. **No Persistent Storage**: Request IDs are not stored; they exist only for the request lifecycle
3. **No Format Customization**: Currently only supports UUID v4 format
4. **WASM Environment Constraints**: Limited access to system entropy sources

### Usage Recommendations

1. **Use Consistent Header Names**: Standardize on one header name across your organization
2. **Enable Response Headers for Public APIs**: Helps users report issues with specific request IDs
3. **Don't Override Existing IDs**: Unless you're the first gateway in the request path
4. **Combine with Logging**: Ensure all services log the request ID for correlation

### Future Enhancements

1. **Custom ID Formats**: Support for other ID formats (ULID, Snowflake, etc.)
2. **ID Persistence**: Optional storage in Redis/database for long-term tracking
3. **Metrics Integration**: Export request ID correlation metrics
4. **Rate Limiting Integration**: Use request IDs for more sophisticated rate limiting
5. **Distributed Tracing Integration**: Direct integration with OpenTelemetry/Jaeger

## Compatibility

- **Higress Version**: >= 1.0.0
- **Go Version**: Go 1.19+ (for development)
- **TinyGo Version**: 0.28.0+ (for compilation)
- **WASM Spec**: wasm32-wasi

## References

- [RFC 4122](https://tools.ietf.org/html/rfc4122) - UUID Specification
- [Higress Plugin Development Guide](https://higress.io/docs/plugins/plugin-dev)
- [Distributed Tracing Best Practices](https://opentelemetry.io/docs/concepts/signals/traces/)

