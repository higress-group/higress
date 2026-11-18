# AI Prompts for Request ID Generator Plugin

## Initial Request to AI

```
I need to create a new Higress wasm-go plugin called "request-id-generator" with the following requirements:

**Purpose:**
Generate and inject unique request IDs for every HTTP request passing through Higress gateway.

**Core Functionality:**
1. Generate a unique request ID for each incoming request (UUID v4 format)
2. Add the request ID to request headers (configurable header name, default: X-Request-Id)
3. Optionally pass the request ID to upstream services
4. Optionally add the request ID to response headers for client tracking

**Configuration Parameters:**
- enable: Enable/disable the plugin (boolean, default: true)
- request_header: Header name for request ID in upstream requests (string, default: "X-Request-Id")
- response_header: Header name for request ID in client responses (string, optional)
- override_existing: Whether to override existing request ID if present (boolean, default: false)

**Technology Stack:**
- Language: Go (TinyGo for WASM compilation)
- Framework: Higress wasm-go SDK
- UUID Generation: Use standard UUID v4 algorithm

**Use Cases:**
1. Request tracing and correlation across microservices
2. Debugging and troubleshooting
3. Logging and monitoring
4. Client-side request tracking

**Non-Functional Requirements:**
- High performance (minimal latency overhead)
- Thread-safe UUID generation
- Consistent with Higress plugin architecture
- Compatible with existing Higress plugins

Please create:
1. Complete design document
2. Plugin implementation code
3. README with usage examples
4. Test cases
```

## Follow-up Clarifications

### Clarification 1: UUID Generation
```
Q: How should we generate UUIDs in the WASM environment?
A: Use a simple UUID v4 implementation based on random bytes. We can use the crypto/rand 
   functionality available in the wasm-go SDK for generating random values.
```

### Clarification 2: Header Handling
```
Q: What should happen if a request already has a request ID?
A: By default, preserve the existing request ID (override_existing: false). This allows 
   requests to maintain their IDs across multiple gateway hops. When override_existing is true, 
   always generate a new ID.
```

### Clarification 3: Performance Considerations
```
Q: Are there any performance concerns with UUID generation?
A: UUID generation should be fast enough for most use cases. We'll use a lightweight 
   implementation that doesn't rely on complex cryptographic operations. Each request 
   will have minimal overhead (< 1ms).
```

## Design Decisions Made with AI

1. **UUID Format**: Chose UUID v4 (random) over UUID v1 (time-based) for better privacy and simplicity
2. **Header Names**: Made header names fully configurable to support different organizational standards
3. **Optional Response Header**: Allow users to choose whether to expose request IDs to clients
4. **Preserve Existing IDs**: Default to not overriding existing IDs to support distributed tracing scenarios
5. **Simple Configuration**: Keep configuration minimal and intuitive

## Implementation Approach

The AI suggested the following implementation structure:
1. Parse configuration during plugin initialization
2. Use the Higress wasm-go SDK's request wrapper for header manipulation
3. Implement a lightweight UUID v4 generator
4. Add appropriate logging for debugging
5. Follow Higress plugin conventions for error handling

