## ai-proxy: opt-in persistent API Key affinity

- Add an explicitly enabled Redis-backed consumer-to-Key binding within one provider. Bind only after a successful response; retry eligible 401/402/403/429 failures with another available Key and persist a successful replacement. No plaintext API Keys are stored in Redis.
- Keep existing behavior when disabled or without a consumer identity. Enabling affinity requires Redis and supported use with token failover; this is not an account pool or quota-recovery detector.
- Stop both headers and body while awaiting Redis, preventing a provisional Key from reaching the upstream before the bound Key is resolved. Include deterministic host tests and a real Envoy/Redis timing regression harness.
- Preserve the original request URI/body through the matching internal redirect rule. Cache benefits depend on the upstream's actual isolation domain; affinity is not enabled by default and can skew Key usage for uneven consumers.
