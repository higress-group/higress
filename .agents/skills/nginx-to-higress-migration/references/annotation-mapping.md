# Nginx to Higress Annotation Compatibility

## ⚠️ Important: Do NOT Modify Your Ingress Resources!

**Higress natively supports `nginx.ingress.kubernetes.io/*` annotations** - no conversion or modification needed!

The Higress controller uses `ParseStringASAP()` which first tries `nginx.ingress.kubernetes.io/*` prefix, then falls back to `higress.io/*`. Your existing Ingress resources work as-is with Higress.

## Fully Compatible Annotations (Work As-Is)

These nginx annotations work directly with Higress without any changes:

| nginx annotation (keep as-is) | Higress also accepts | Notes |
|-------------------------------|---------------------|-------|
| `nginx.ingress.kubernetes.io/rewrite-target` | `higress.io/rewrite-target` | Supports capture groups |
| `nginx.ingress.kubernetes.io/use-regex` | `higress.io/use-regex` | Enable regex path matching |
| `nginx.ingress.kubernetes.io/ssl-redirect` | `higress.io/ssl-redirect` | Force HTTPS |
| `nginx.ingress.kubernetes.io/force-ssl-redirect` | `higress.io/force-ssl-redirect` | Same behavior |
| `nginx.ingress.kubernetes.io/backend-protocol` | `higress.io/backend-protocol` | HTTP/HTTPS/GRPC |
| `nginx.ingress.kubernetes.io/proxy-body-size` | `higress.io/proxy-body-size` | Max body size |

### CORS

| nginx annotation | Higress annotation |
|------------------|-------------------|
| `nginx.ingress.kubernetes.io/enable-cors` | `higress.io/enable-cors` |
| `nginx.ingress.kubernetes.io/cors-allow-origin` | `higress.io/cors-allow-origin` |
| `nginx.ingress.kubernetes.io/cors-allow-methods` | `higress.io/cors-allow-methods` |
| `nginx.ingress.kubernetes.io/cors-allow-headers` | `higress.io/cors-allow-headers` |
| `nginx.ingress.kubernetes.io/cors-expose-headers` | `higress.io/cors-expose-headers` |
| `nginx.ingress.kubernetes.io/cors-allow-credentials` | `higress.io/cors-allow-credentials` |
| `nginx.ingress.kubernetes.io/cors-max-age` | `higress.io/cors-max-age` |

### Timeout & Retry

| nginx annotation | Higress annotation |
|------------------|-------------------|
| `nginx.ingress.kubernetes.io/proxy-connect-timeout` | `higress.io/proxy-connect-timeout` |
| `nginx.ingress.kubernetes.io/proxy-send-timeout` | `higress.io/proxy-send-timeout` |
| `nginx.ingress.kubernetes.io/proxy-read-timeout` | `higress.io/proxy-read-timeout` |
| `nginx.ingress.kubernetes.io/proxy-next-upstream-tries` | `higress.io/proxy-next-upstream-tries` |

### Canary (Grayscale)

| nginx annotation | Higress annotation |
|------------------|-------------------|
| `nginx.ingress.kubernetes.io/canary` | `higress.io/canary` |
| `nginx.ingress.kubernetes.io/canary-weight` | `higress.io/canary-weight` |
| `nginx.ingress.kubernetes.io/canary-header` | `higress.io/canary-header` |
| `nginx.ingress.kubernetes.io/canary-header-value` | `higress.io/canary-header-value` |
| `nginx.ingress.kubernetes.io/canary-header-pattern` | `higress.io/canary-header-pattern` |
| `nginx.ingress.kubernetes.io/canary-by-cookie` | `higress.io/canary-by-cookie` |

### Authentication

| nginx annotation | Higress annotation |
|------------------|-------------------|
| `nginx.ingress.kubernetes.io/auth-type` | `higress.io/auth-type` |
| `nginx.ingress.kubernetes.io/auth-secret` | `higress.io/auth-secret` |
| `nginx.ingress.kubernetes.io/auth-realm` | `higress.io/auth-realm` |

### Load Balancing

| nginx annotation | Higress annotation |
|------------------|-------------------|
| `nginx.ingress.kubernetes.io/load-balance` | `higress.io/load-balance` |
| `nginx.ingress.kubernetes.io/upstream-hash-by` | `higress.io/upstream-hash-by` |

### IP Access Control

| nginx annotation | Higress annotation |
|------------------|-------------------|
| `nginx.ingress.kubernetes.io/whitelist-source-range` | `higress.io/whitelist-source-range` |
| `nginx.ingress.kubernetes.io/denylist-source-range` | `higress.io/denylist-source-range` |

### Redirect

| nginx annotation | Higress annotation |
|------------------|-------------------|
| `nginx.ingress.kubernetes.io/permanent-redirect` | `higress.io/permanent-redirect` |
| `nginx.ingress.kubernetes.io/temporal-redirect` | `higress.io/temporal-redirect` |
| `nginx.ingress.kubernetes.io/permanent-redirect-code` | `higress.io/permanent-redirect-code` |

### Header Control

| nginx annotation | Higress annotation |
|------------------|-------------------|
| `nginx.ingress.kubernetes.io/proxy-set-headers` | `higress.io/proxy-set-headers` |
| `nginx.ingress.kubernetes.io/proxy-hide-headers` | `higress.io/proxy-hide-headers` |
| `nginx.ingress.kubernetes.io/proxy-pass-headers` | `higress.io/proxy-pass-headers` |

### Upstream TLS

| nginx annotation | Higress annotation |
|------------------|-------------------|
| `nginx.ingress.kubernetes.io/proxy-ssl-secret` | `higress.io/proxy-ssl-secret` |
| `nginx.ingress.kubernetes.io/proxy-ssl-verify` | `higress.io/proxy-ssl-verify` |

### TLS Protocol & Cipher Control

Higress provides fine-grained TLS control via dedicated annotations:

| nginx annotation | Higress annotation | Notes |
|------------------|-------------------|-------|
| `nginx.ingress.kubernetes.io/ssl-protocols` | (see below) | Use Higress-specific annotations |

**Higress TLS annotations (no nginx equivalent - use these directly):**

| Higress annotation | Description | Example value |
|-------------------|-------------|---------------|
| `higress.io/tls-min-protocol-version` | Minimum TLS version | `TLSv1.2` |
| `higress.io/tls-max-protocol-version` | Maximum TLS version | `TLSv1.3` |
| `higress.io/ssl-cipher` | Allowed cipher suites | `ECDHE-RSA-AES128-GCM-SHA256` |

**Example: Restrict to TLS 1.2+**
```yaml
# nginx (using ssl-protocols)
annotations:
  nginx.ingress.kubernetes.io/ssl-protocols: "TLSv1.2 TLSv1.3"

# Higress (use dedicated annotations)
annotations:
  higress.io/tls-min-protocol-version: "TLSv1.2"
  higress.io/tls-max-protocol-version: "TLSv1.3"
```

**Example: Custom cipher suites**
```yaml
annotations:
  higress.io/ssl-cipher: "ECDHE-RSA-AES128-GCM-SHA256:ECDHE-RSA-AES256-GCM-SHA384"
```

## Unsupported Annotations (Require WASM Plugin)

These annotations have no direct Higress equivalent and require custom WASM plugins:

### Configuration Snippets
```yaml
# NOT supported - requires WASM plugin
nginx.ingress.kubernetes.io/server-snippet: |
  location /custom { ... }
nginx.ingress.kubernetes.io/configuration-snippet: |
  more_set_headers "X-Custom: value";
nginx.ingress.kubernetes.io/stream-snippet: |
  # TCP/UDP snippets
```

### Lua Scripting
```yaml
# NOT supported - convert to WASM plugin
nginx.ingress.kubernetes.io/lua-resty-waf: "active"
nginx.ingress.kubernetes.io/lua-resty-waf-score-threshold: "10"
```

### ModSecurity
```yaml
# NOT supported - use Higress WAF plugin or custom WASM
nginx.ingress.kubernetes.io/enable-modsecurity: "true"
nginx.ingress.kubernetes.io/modsecurity-snippet: |
  SecRule ...
```

### Rate Limiting (Complex)
```yaml
# Basic rate limiting supported via plugin
# Complex Lua-based rate limiting requires WASM
nginx.ingress.kubernetes.io/limit-rps: "10"
nginx.ingress.kubernetes.io/limit-connections: "5"
```

### Other Unsupported
```yaml
# NOT directly supported
nginx.ingress.kubernetes.io/client-body-buffer-size
nginx.ingress.kubernetes.io/proxy-buffering
nginx.ingress.kubernetes.io/proxy-buffers-number
nginx.ingress.kubernetes.io/proxy-buffer-size
nginx.ingress.kubernetes.io/mirror-uri
nginx.ingress.kubernetes.io/mirror-request-body
nginx.ingress.kubernetes.io/grpc-backend
nginx.ingress.kubernetes.io/custom-http-errors
nginx.ingress.kubernetes.io/default-backend
```

## Migration Script

Use this script to analyze Ingress annotations:

```bash
# scripts/analyze-ingress.sh in this skill
./scripts/analyze-ingress.sh <namespace>
```
