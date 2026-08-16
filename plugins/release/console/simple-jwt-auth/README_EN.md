# Simple JWT Authentication

This plugin reads a JWT from a configurable request header and validates it
with a shared secret. Invalid or missing tokens receive HTTP 401.

## Configuration

- `token_secret_key`: shared key used to verify the JWT signature.
- `token_headers`: request header that carries the token.

```yaml
token_secret_key: change-me
token_headers: authorization
```

Use a secret-management mechanism for production values and rotate the shared
key regularly. This plugin is intentionally small; use a richer authentication
plugin when you need JWKS, issuer, audience, or claim validation.
