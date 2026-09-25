# Higress Built-in Plugins

Before writing custom WASM plugins, check if Higress has a built-in plugin that meets your needs.

**Plugin docs and images**: https://github.com/higress-group/higress-console/tree/main/backend/sdk/src/main/resources/plugins

## Authentication & Authorization

| Plugin | Description | Replaces nginx feature |
|--------|-------------|----------------------|
| `basic-auth` | HTTP Basic Authentication | `auth_basic` directive |
| `jwt-auth` | JWT token validation | JWT Lua scripts |
| `key-auth` | API Key authentication | Custom auth headers |
| `hmac-auth` | HMAC signature authentication | Signature validation |
| `oauth` | OAuth 2.0 authentication | OAuth Lua scripts |
| `oidc` | OpenID Connect | OIDC integration |
| `ext-auth` | External authorization service | `auth_request` directive |
| `opa` | Open Policy Agent integration | Complex auth logic |

## Traffic Control

| Plugin | Description | Replaces nginx feature |
|--------|-------------|----------------------|
| `key-rate-limit` | Rate limiting by key | `limit_req` directive |
| `cluster-key-rate-limit` | Distributed rate limiting | `limit_req` with shared state |
| `ip-restriction` | IP whitelist/blacklist | `allow`/`deny` directives |
| `request-block` | Block requests by pattern | `if` + `return 403` |
| `traffic-tag` | Traffic tagging | Custom headers for routing |
| `bot-detect` | Bot detection & blocking | Bot detection Lua scripts |

## Request/Response Modification

| Plugin | Description | Replaces nginx feature |
|--------|-------------|----------------------|
| `transformer` | Transform request/response | `proxy_set_header`, `more_set_headers` |
| `cors` | CORS headers | `add_header` CORS headers |
| `custom-response` | Custom static response | `return` directive |
| `request-validation` | Request parameter validation | Validation Lua scripts |
| `de-graphql` | GraphQL to REST conversion | GraphQL handling |

## Security

| Plugin | Description | Replaces nginx feature |
|--------|-------------|----------------------|
| `waf` | Web Application Firewall | ModSecurity module |
| `geo-ip` | GeoIP-based access control | `geoip` module |

## Caching & Performance

| Plugin | Description | Replaces nginx feature |
|--------|-------------|----------------------|
| `cache-control` | Cache control headers | `expires`, `add_header Cache-Control` |

## AI Features (Higress-specific)

| Plugin | Description |
|--------|-------------|
| `ai-proxy` | AI model proxy |
| `ai-cache` | AI response caching |
| `ai-quota` | AI token quota |
| `ai-token-ratelimit` | AI token rate limiting |
| `ai-transformer` | AI request/response transform |
| `ai-security-guard` | AI content security |
| `ai-statistics` | AI usage statistics |
| `mcp-server` | Model Context Protocol server |

## Using Built-in Plugins

### Via WasmPlugin CRD

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: basic-auth-plugin
  namespace: higress-system
spec:
  # Use built-in plugin image
  url: oci://higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/basic-auth:1.0.0
  phase: AUTHN
  priority: 320
  defaultConfig:
    consumers:
    - name: user1
      credential: "admin:123456"
```

### Via Higress Console

1. Navigate to **Plugins** → **Plugin Market**
2. Find the desired plugin
3. Click **Enable** and configure

## Image Registry Locations

Select the nearest registry based on your location:

| Region | Registry |
|--------|----------|
| China/Default | `higress-registry.cn-hangzhou.cr.aliyuncs.com` |
| North America | `higress-registry.us-west-1.cr.aliyuncs.com` |
| Southeast Asia | `higress-registry.ap-southeast-7.cr.aliyuncs.com` |

Example with regional registry:
```yaml
spec:
  url: oci://higress-registry.us-west-1.cr.aliyuncs.com/plugins/basic-auth:1.0.0
```

## Plugin Configuration Reference

Each plugin has its own configuration schema. View the spec.yaml in the plugin directory:
https://github.com/higress-group/higress-console/tree/main/backend/sdk/src/main/resources/plugins/<plugin-name>/spec.yaml

Or check the README files for detailed documentation.
