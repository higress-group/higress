---
title: Cluster Concurrency Limit
keywords: [ AI Gateway, concurrency limit, rate limit ]
description: Cluster concurrency limit plugin configuration reference
---

## Description

The `cluster-concurrency-limit` plugin provides **cluster-wide (shared across gateway instances)
in-flight concurrency limiting** backed by Redis. Unlike window-based rate limiting plugins
(QPS/QPM/token rate), this plugin implements Gauge semantics: a request occupies one concurrency
slot from arrival until the response completes (including the whole streaming lifetime), and the
slot is released as soon as the stream ends. For LLM long-streaming workloads, concurrency is a
more intuitive capacity model than QPS.

It supports **tenant / consumer / credential (apikey) hierarchical quotas plus a global fallback**:

- **Tenant** (`limit_by_consumer_group`): reads the `X-Mse-Consumer-Group` header (injected by a preceding plugin)
- **Consumer** (`limit_by_consumer`): reads the `X-Mse-Consumer` header written by authentication plugins; all credentials of the same consumer share the quota
- **Credential / arbitrary key**: `limit_by_header` / `limit_by_param` / `limit_by_cookie`
- **Global** (`global_threshold`): total concurrency fallback of the rule group

Multiple dimensions are matched in an **all-match** manner: a request occupies one slot in every
matched dimension, and is rejected (default 429, no queuing) as soon as any dimension is exhausted.
The hierarchy semantics is **shared pool + per-level cap (cap, not reservation)**: each level
configures an upper bound rather than an exclusive reservation; the effective availability of a
child level = min(own remaining, parent pool remaining), first-come-first-served.

## Runtime Properties

Plugin phase: `default phase`
Plugin priority: `610` (runs before ai-token-ratelimit's 600 — occupy the concurrency slot first, then deduct tokens)

> The priority is delivered via the WasmPlugin CR / higress-console plugin metadata, not set in plugin code.

## Configuration

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| rule_name | string | Yes | - | Rule name, used in Redis key assembly; the key embeds a `{rule_name}` hash tag so all dimension keys of one request land in the same Redis Cluster slot |
| global_threshold | object | No | - | Total concurrency of the rule group: `concurrency: int`; **can coexist** with rule_items (at least one of them required) and participates in all-match as a fallback dimension |
| rule_items | array | No (max 10) | - | Dimension rule items, all-match |
| rejected_code | int | No | 429 | HTTP status code when rejected |
| rejected_msg | string | No | concurrency limit exceeded | Response body when rejected |
| show_limit_quota_header | bool | No | false | Return `X-Concurrency-Limit` / `X-Concurrency-Remaining` response headers (minimum remaining across all matched dimensions) |
| lease_ttl_seconds | int | No | 600 | Member lease TTL in seconds; only guards against slot leaks from process crashes; should exceed the typical stream duration |
| redis | object | Yes | - | Redis configuration (same as cluster-key-rate-limit / ai-token-ratelimit) |

Fields in `global_threshold`:

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| concurrency | int | Yes | - | Total concurrency limit of the rule group, positive integer |

Fields in each item of `rule_items`:

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| limit_by_consumer_group | string | No, one of `limit_by_*` | - | **Tenant dimension**, reads the `X-Mse-Consumer-Group` header, no value needed |
| limit_by_consumer | string | No, one of `limit_by_*` | - | Consumer dimension, reads the `X-Mse-Consumer` header, no value needed |
| limit_by_header | string | No, one of `limit_by_*` | - | Header dimension, value is the header name |
| limit_by_param | string | No, one of `limit_by_*` | - | URL parameter dimension |
| limit_by_cookie | string | No, one of `limit_by_*` | - | Cookie dimension |
| limit_by_per_header / limit_by_per_param / limit_by_per_consumer / limit_by_per_cookie | string | No, one of `limit_by_*` | - | Count each value separately; limit_keys only accept `*` / `regexp:...` |
| limit_by_per_ip | string | No, one of `limit_by_*` | - | `from-remote-addr` / `from-header-<name>`; CIDRs go in limit_keys |
| limit_keys | array of object | Yes | - | Key and concurrency limit list |

Fields in each item of `limit_keys`:

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| key | string | Yes | - | Non-per types accept literal / `*` / `regexp:...`; per types accept only `*` / `regexp:...`; `limit_by_per_ip` accepts IPs or CIDRs |
| concurrency | int | Yes | - | Concurrency limit of this key (Gauge), positive integer |

Fields in `redis`:

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| service_name | string | Yes | - | Redis service name, full FQDN with service type, e.g. `my-redis.dns`, `redis.static` |
| service_port | int | No | 80 for static services, 6379 otherwise | Redis service port |
| username | string | No | - | Redis username |
| password | string | No | - | Redis password |
| timeout | int | No | 1000 | Redis connection timeout in milliseconds |
| database | int | No | 0 | Redis database id |

## Configuration Example

```yaml
rule_name: ai-route-concurrency
global_threshold:
  concurrency: 200                    # total concurrency fallback of the rule group
rule_items:
  # Tenant level: X-Mse-Consumer-Group (injected by a preceding plugin)
  - limit_by_consumer_group: ""
    limit_keys:
      - key: "tenant-a"
        concurrency: 10
      - key: "*"
        concurrency: 5
  # Consumer level: X-Mse-Consumer; all keys of the same consumer share the quota
  - limit_by_consumer: ""
    limit_keys:
      - key: "alice"
        concurrency: 7                # child cap < parent pool, to mitigate starvation
      - key: "regexp:.*"
        concurrency: 3
  # Credential level: limit by any header/param/cookie
  - limit_by_header: "X-API-Key"
    limit_keys:
      - key: "sk-aaa111"
        concurrency: 2
      - key: "*"
        concurrency: 1
redis:
  service_name: redis.static
show_limit_quota_header: true
rejected_code: 429
```

### Hierarchy strategy: cap, not reservation

- Each level is an upper bound, not a reservation; child availability = min(own remaining, parent pool remaining), first-come-first-served;
- **A child cap equal to the parent pool cannot prevent starvation** (e.g. tenant pool 10, consumers 10 each: consumer a occupying all 10 leaves consumer b with 0);
- The standard mitigation is **child cap < parent pool** (e.g. consumers 7 each within a tenant pool of 10, so a single consumer leaves at least 3 slots for others).

## Counting Model (ZSET lease-member model)

Each limiting dimension maps to one ZSET key (`{rule_name}:concurrency:<dim>:<value>`); each
in-flight request is a member (the `x-request-id`, or a plugin-generated UUID when absent) whose
score is the lease expiry timestamp:

- **Acquire**: one Lua script atomically performs "lazy-expire stale members → check every
  dimension → ZADD only when all pass" — check-before-occupy, no rollback needed;
- **Release**: when the HTTP stream closes (normal completion / upstream reset / client
  disconnect), the plugin ZREMs its own member — naturally idempotent;
- **Crash fallback**: members leaked by a process crash are lazily cleaned after their score expires;
- **Streams outliving the lease**: the member expires early → the slot is released early →
  temporary slight over-admission, which self-heals when the stream ends.
  `lease_ttl_seconds` should exceed the typical stream duration.

## Deployment & Troubleshooting

### Deployment checklist

1. Enable the WasmPlugin with priority **610**;
2. Redis service (registered via McpBridge, `service_name` matching the config); the hash tag is Redis Cluster compatible;
3. If the tenant dimension is used: deploy the preceding plugin that injects `X-Mse-Consumer-Group`;
4. Configure authentication plugins (key-auth/jwt-auth/basic-auth) as needed so that `X-Mse-Consumer` is written.

### ⚠️ Ordering prerequisite

> **The plugin injecting `X-Mse-Consumer-Group` must run before this plugin**
> (priority > 610 or an earlier phase). Otherwise the tenant dimension reads an empty header,
> passes without counting, and **the tenant quota silently stops working without any error**.

### Traffic without identity headers

Requests without `X-Mse-Consumer` / `X-Mse-Consumer-Group` are **passed without counting**; only
the global fallback can intercept them. To block unauthenticated traffic, configure an
authentication plugin on the route to reject credential-less requests.

### Redis failure

**Fail-open**: requests pass, with an error log and the `concurrency_limit_redis_error` metric.

### Metrics

- `concurrency_limit_rejected` — rejection count (suitable for alerting)
- `concurrency_limit_redis_error` — fail-open count

### Troubleshooting

| Symptom | What to check |
| --- | --- |
| Tenant dimension ineffective | Whether the preceding injecting plugin's priority > 610; whether `X-Mse-Consumer-Group` reaches this plugin |
| Crash leftovers | Members are cleaned automatically when their scores expire; `redis-cli ZRANGEBYSCORE key -inf +inf WITHSCORES` shows member expiry times |
| Live in-flight requests | `redis-cli ZRANGE "{rule}:concurrency:..." 0 -1` |
