# AgentCert + TrustScore verification (ext-auth)

Verify a calling agent's **AgentCert + live TrustScore** on every request through
Higress using the built-in [`ext-auth`](../../plugins/wasm-go/extensions/ext-auth)
plugin — **no code in Higress**. The plugin speaks Envoy `ext_authz` (HTTP); it
calls the [AgentCert TAG](https://github.com/pdxlab/agentcert-tag) verify sidecar,
which does the crypto (signature chain + revocation + TrustScore) and returns
allow/deny plus `x-agentcert-*` verdict headers. The gateway stays decoupled from
the trust model.

`agentcert-ext-auth.yaml` in this directory is the full manifest.

## 1. Run the sidecar (shadow by default)

```bash
docker run -p 8080:8080 -e TAG_MODE=shadow \
  -e TAG_ANCHORS_PEM=/anchors/roots.pem -v $PWD/anchors:/anchors:ro \
  ghcr.io/pdxlab/agentcert-tag/sidecar:latest
# health: curl localhost:8080/healthz -> {"ok":true,"extauthz_path":"/ext_authz"}
```

Register it as a service (McpBridge / ServiceSource) so the FQDN in the manifest
(`tag-sidecar.default.svc.cluster.local`) resolves — same as any ext-auth target.

## 2. Apply the plugin

```bash
kubectl apply -f agentcert-ext-auth.yaml
```

`forward_auth` mode calls the sidecar on a fixed `/ext_authz` path regardless of
the client path. The request headers `x-agent-cert`, `x-agent-cert-proof`,
`x-agent-cert-carriage`, `x-agent-id` are forwarded to the verifier; the
`x-agentcert-*` verdict (`x-agentcert-verdict`, `x-agentcert-trustscore`,
`x-agentcert-tier`, …) is copied back onto the upstream request so your own route
policy and logs can act on it.

## 3. Roll out

1. **Shadow (default).** The sidecar decides + logs but returns `200` on
   everything — watch `x-agentcert-shadow-would` (`allow`/`deny`) to see what
   enforce *would* block. Zero request impact; `agentcert-ext-auth.yaml` unchanged.
2. **Enforce.** Set `TAG_MODE=enforce` on the sidecar (optionally `TAG_MIN_SCORE=N`
   for a TrustScore floor). `VERIFIED` allows, everything else denies.
3. Keep the sidecar's `TAG_FAIL_MODE` (`open`/`closed`) aligned with the plugin's
   `failure_mode_allow`.

## Trust model / assumptions

- **TrustScore** — `0–1000` continuous reputation score the sidecar resolves
  live (falling back to the cert-baked score); `x-agentcert-tier` buckets it
  (`Trusted` / `Caution` / `Untrusted`).
- **Revocation freshness** — checked by the sidecar per request against its
  configured anchors/CRL source; no caching in Higress.
- **Metadata only** — the sidecar reads request headers (the cert), never the body.
- **Failure behavior** — if the sidecar is unreachable or returns 5xx, Higress
  applies `failure_mode_allow` (fail-open here); the sidecar's own `TAG_FAIL_MODE`
  governs its internal fail posture. In shadow nothing is ever blocked.

Full header/verdict contract:
[`agentcert-tag/docs/ext_authz.md`](https://github.com/pdxlab/agentcert-tag/blob/main/docs/ext_authz.md).
