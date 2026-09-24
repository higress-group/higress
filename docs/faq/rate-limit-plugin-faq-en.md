# Higress Rate-limit Plugin FAQ

> Chinese is the canonical source; this document is its synchronized English translation. It applies to both `ai-token-ratelimit` and `cluster-key-rate-limit`; plugin-specific fields are called out explicitly.

## Contents

1. [Rate limit not taking effect](#rate-limit-not-taking-effect)
2. [Configuration rejected: literal name under `limit_by_per_*`](#literal-name-in-per-rule)
3. [Configuration rejected: missing `limit_keys`](#missing-limit-keys)
4. [Redis connection issues](#redis-connection-issues)
5. [Diagnostic command cheat sheet](#diagnostic-command-cheat-sheet)
6. [Developer notes](#developer-notes)

<a id="rate-limit-not-taking-effect"></a>
## 1. Rate limit not taking effect

### 1.1 Requests always return 200, never 429

**Symptom:** Requests keep succeeding and the configured threshold appears ineffective.

**Root cause:** In order of investigation: route or match conditions did not match; parsing failed and the new rule was not loaded; the configured dimension differs from the request value; or no Redis call occurred.

**Diagnostic steps:**

1. Search gateway logs for the plugin name, `limit_by_per_`, `missing limit_keys`, and `must start with`.
2. Check `/stats` for increasing Wasm configuration-rejection counters.
3. Confirm the target Redis cluster in `/clusters`, then inspect its connection counters.
4. Finally use Redis `SCAN` for rate-limit keys. No key alone does not prove a Redis failure.

**Fix:** Correct the earliest parsing or matching error before debugging connectivity. Do not blame Redis until the rule is known to be loaded.

**See also:** [#4067](https://github.com/higress-group/higress/issues/4067), [#2646](https://github.com/higress-group/higress/issues/2646).

### 1.2 Redis always has no rate-limit keys

**Symptom:** `SCAN` finds no Higress rate-limit key.

**Root cause:** The rule may not match, data-plane parsing may reject it, or no request requiring a counter has occurred. A key appears only after rule execution reaches Redis.

**Diagnostic steps:** Confirm that plugin logs contain no parse error, send a request known to match, and correlate `/stats`, `/clusters`, and short-lived Redis `MONITOR` output in a test environment.

**Fix:** Align request values with `limit_by_*` and `limit_keys`. If connection attempts occur and fail, then inspect port, credentials, and network.

**See also:** [Section 2](#literal-name-in-per-rule), [Section 3](#missing-limit-keys).

### 1.3 `cx_total=0` and `cx_connect_fail=0`

**Symptom:** The Redis cluster reports neither successful nor failed connection attempts.

**Root cause:** Usually the data plane never tried to connect: the rule was not loaded or matched, or the wrong cluster name is being inspected. This is different from a connection failure.

**Diagnostic steps:** Derive `outbound|<port>||<service_name>` from `service_name` and `service_port`, locate that exact entry in `/clusters`, and inspect Wasm rejection logs.

**Fix:** Correct loading or matching first; changing Redis credentials cannot fix zero connection attempts.

**See also:** [#4067](https://github.com/higress-group/higress/issues/4067).

### 1.4 `update_rejected` / `config_fail` keeps increasing

**Symptom:** Rejection counters increase after each control-plane push.

**Root cause:** The plugin parser returned an error, commonly a literal key under `limit_by_per_*`, missing `limit_keys`, or an invalid IP-source format.

**Diagnostic steps:** Correlate the counter timestamp with the complete gateway log error; do not rely only on control-plane object state.

**Fix:** Correct the field, rejected value, or migration target named by the error, then verify that the counter stops increasing.

**See also:** [Section 2](#literal-name-in-per-rule), [Section 3](#missing-limit-keys).

### 1.5 `config_dump` does not mean loaded by the data plane

**Symptom:** Configuration appears in `config_dump`, but rate limiting is inactive.

**Root cause:** `config_dump` proves delivery, not that the Wasm plugin accepted and activated the configuration.

**Diagnostic steps:** Correlate plugin logs, Wasm rejection counters, the target cluster, and Redis calls.

**Fix:** Use data-plane load results as authority and re-observe the current configuration after fixing parse errors.

**See also:** [#2464](https://github.com/higress-group/higress/issues/2464).

<a id="literal-name-in-per-rule"></a>
## 2. Configuration rejected: literal name under `limit_by_per_*`

### 2.1 What the error looks like

```text
the "limit_by_per_consumer" restriction must start with 'regexp:' or be exactly '*' (got "alice"); to match an exact name, use the non-per variant "limit_by_consumer" instead (limit_keys stay the same)
```

`per_` means “create a separate bucket for each actual value,” so its `limit_keys` are selectors and accept only `*` or `regexp:...`. Exact literals belong to the non-`per_` variant.

### 2.2 Choosing `per_*` or non-`per_*`

| Goal | Field | `limit_keys[].key` |
| --- | --- | --- |
| Limit one exact header value | `limit_by_header` | Literal |
| Limit each header value separately | `limit_by_per_header` | `*` or `regexp:...` |
| Limit one exact parameter value | `limit_by_param` | Literal |
| Limit each parameter value separately | `limit_by_per_param` | `*` or `regexp:...` |
| Limit one exact consumer | `limit_by_consumer` | Consumer literal |
| Limit each consumer separately | `limit_by_per_consumer` | `*` or `regexp:...` |
| Limit one exact Cookie value | `limit_by_cookie` | Literal |
| Limit each Cookie value separately | `limit_by_per_cookie` | `*` or `regexp:...` |

### 2.3 Migration map

Change `limit_by_per_header/param/consumer/cookie` to the corresponding `limit_by_header/param/consumer/cookie`, keeping `limit_keys` and threshold fields. `limit_by_per_ip` is different: it selects the IP source, while CIDRs remain in `limit_keys`.

<a id="missing-limit-keys"></a>
## 3. Configuration rejected: missing `limit_keys`

### 3.1 What the error looks like

```text
missing limit_keys in config for limit_by_per_ip; add at least one entry, e.g. key: "0.0.0.0/0"
```

An empty array carries the same type and example, with the prefix `config limit_keys cannot be empty`.

### 3.2 Minimal valid key by type

| Type | Minimal `limit_keys` entry |
| --- | --- |
| `limit_by_header/param/cookie` | `- key: "<exact-value>"` |
| `limit_by_consumer` | `- key: "<consumer-name>"` |
| `limit_by_per_header/param/consumer/cookie` | `- key: "*"` |
| `limit_by_per_ip` | `- key: "0.0.0.0/0"` |

Each entry also needs a positive plugin-specific threshold: `token_per_*` for AI Token rate limiting, or `query_per_*` for cluster key rate limiting.

<a id="redis-connection-issues"></a>
## 4. Redis connection issues

### 4.1 `cx_connect_fail > 0`

**Symptom:** Connection failures increase for the target Redis cluster.

**Root cause:** Common causes are a wrong port, authentication failure, unreachable DNS/route, or network policy.

**Diagnostic steps:** Confirm the exact cluster name and port in `/clusters`; verify DNS/TCP from the gateway container; inspect Redis authentication logs.

**Fix:** Correct `service_name`, `service_port`, credentials, or network policy. A connection failure is distinct from `cx_total=0`.

**See also:** [#2000](https://github.com/higress-group/higress/issues/2000).

### 4.2 `service_port` and the cluster port

Both plugins use `FQDNCluster` to generate `outbound|<service_port>||<service_name>`. If the port is omitted, `.static` services default to 80 and others default to 6379. If a console-generated static service points to Redis on 6379, set `service_port: 6379` explicitly.

### 4.3 About `.static` services

`.static` is a fixed-address naming convention; it does not mean Redis listens on port 80. The cluster name uses the logical port and must match the control-plane-generated cluster. After changing an McpBridge port, re-check the cluster, credentials, and plugin state.

### 4.4 Historical authentication loss after an McpBridge port change

[#2000](https://github.com/higress-group/higress/issues/2000) reported Redis commands losing AUTH after a port edit and recovering after plugin restart. The current Redis wrapper retries `RedisInit` while not ready, but upgrade investigations should still verify the actual version, cluster transition, and authentication logs.

<a id="diagnostic-command-cheat-sheet"></a>
## 5. Diagnostic command cheat sheet

```bash
# Wasm configuration and Redis connection metrics
kubectl -n higress-system exec <gateway-pod> -- \
  curl -s '127.0.0.1:15000/stats?filter=wasm|redis|update_rejected|config_fail|cx_'

# Locate the target Redis cluster
kubectl -n higress-system exec <gateway-pod> -- \
  curl -s '127.0.0.1:15000/clusters' | grep -A8 'outbound|<port>||<service_name>'

# Configuration parsing logs
kubectl -n higress-system logs <gateway-pod> --tail=2000 \
  | grep -Ei 'ai-token-ratelimit|cluster-key-rate-limit|limit_by_per_|missing limit_keys|must start with|redis'

# Redis: use SCAN in production, not blocking KEYS
redis-cli -h <host> -p <port> --scan --pattern '*ratelimit*'
```

<a id="developer-notes"></a>
## 6. Developer notes

### 6.1 Semantic contract for `per_*` and non-`per_*`

Non-`per_` types treat `limit_keys` as exact values; non-IP `per_*` types treat them as `*`/regexp selectors; `limit_by_per_ip` treats the field as the IP source and `limit_keys` as IP/CIDR values. A new type must update parsing, error examples, tests, and bilingual docs together.

### 6.2 `FQDNCluster` to Envoy cluster name

The current dependency's `wrapper.FQDNCluster.ClusterName()` generates `outbound|<port>||<fqdn>`. The plugins use `service_name` as the FQDN and `service_port` as the port; a control-plane port change therefore changes the lookup target.

### 6.3 `RedisClient.Init` / `Ready` lifecycle

`Init` installs a retry closure and calls `RedisInit`. On initial failure, the client remains `ready=false`, logs a warning, and lets a later `Command`/`Eval` retry authentication. `Ready()` is a current-state snapshot, not a permanent health guarantee. Do not treat one `Init` return as proof of sustained connectivity.

### 6.4 Adding a `limit_by_*` type

1. Add the `LimitRuleItemType` constant and field parsing.
2. Define whether keys are exact, `*`/regexp, or IP/CIDR.
3. Update `exampleLimitKeyForType` and actionable errors.
4. Add unit tests for valid input, invalid input, and migration guidance.
5. Keep `token_per_*` / `query_per_*` distinct and update all four READMEs plus this FAQ.

### 6.5 What not to do

- Do not add a cross-plugin dependency for a small shared error helper; the plugins are separate Go modules.
- Do not silently accept literals for non-IP `per_*`; that changes the established configuration contract.
- Do not put CIDRs in the `limit_by_per_ip` field.
- Do not use `config_dump`, one `Ready()` result, or “no Redis key” alone to prove root cause.
- Do not use Redis `KEYS` for routine production diagnosis.
