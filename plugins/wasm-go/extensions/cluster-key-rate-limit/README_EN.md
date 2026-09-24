---
title: Cluster Rate Limiting Based on Key  
keywords: [higress, rate-limit]  
description: Configuration reference for the Key-based cluster rate limiting plugin

---

> ⚠️ **Behavior Change Notice (no version bump)**
>
> As of this update, `rule_items` matching semantics changed from **first-match-wins** (returns on first hit) to **all-match OR overlay** (all matched rules are evaluated, any trigger rejects). The mutual exclusion between `global_threshold` and `rule_items` is also removed to support hybrid configuration.
>
> - Existing config (single `rule_items` or only `global_threshold`): behavior unchanged
> - Existing config (multiple `rule_items` expecting short-circuit match): **behavior will change** — all matched rules are evaluated
> - Redis key format adds `{rule_name}` hash tag for Redis Cluster compatibility; old counter data is incompatible
>
> See "Function Description" and "Configuration Examples" below for details.

## Function Description

The `cluster-key-rate-limit` plugin implements **cluster-level rate limiting** based on Redis, suitable for scenarios
requiring **globally consistent rate limiting across multiple Higress Gateway instances**.

It supports three rate limiting modes:

- **Rule-Level Global Rate Limiting**: Applies a unified rate limit threshold to custom rule groups based on identical `rule_name` and `global_threshold` configurations.
- **Key-Level Dynamic Rate Limiting**: Groups and limits requests by dynamic keys extracted from requests, such as URL parameters, request headers, client IPs, consumer names, or cookie fields.
- **Hybrid rate limiting**: Configure `global_threshold` (global fallback) and `rule_items` (per-dimension) simultaneously. All matched rules take effect together; any trigger rejects the request.

## Operational Attributes

- **Plugin execution phase**: `Default phase`
- **Plugin execution priority**: `20`

## Configuration Instructions

| Configuration Item       | Type          | Required                                  | Default Value       | Description                                                                |  
|--------------------------|---------------|-------------------------------------------|---------------------|----------------------------------------------------------------------------|  
| rule_name                | string        | Yes                                       | -                   | Name of the rate limiting rule. Used to construct the Redis key in the format: `rule_name:rate_limit_type:key_name:key_value`. |  
| global_threshold         | Object        | No, at least one of them is required; can be configured simultaneously | -                 | Apply rate limiting to the entire custom rule group.|  
| rule_items               | array of object | No, at least one of them is required; can be configured simultaneously | -               | Rate limiting rule items. Supports up to **10** rules. All matched `rule_item`s are evaluated and combined with an OR relationship; any trigger rejects the request. The execution order of rules does not affect the final result. See the expanded `rule_items` notes below for details. |  
| show_limit_quota_header  | bool          | No                                        | false             | Whether to display `X-RateLimit-Limit` (total allowed requests) and `X-RateLimit-Remaining` (remaining allowed requests) in the response header. |  
| rejected_code            | int           | No                                        | 429               | HTTP status code returned when a request is rate-limited.                  |  
| rejected_msg             | string        | No                                        | Too many requests | Response body returned when a request is rate-limited.                      |  
| redis                    | object        | Yes                                       | -                   | Configuration for Redis.                                                   |  

### Configuration Fields for `global_threshold`

| Configuration Item       | Type | Required                                 | Default Value | Description                          |  
|--------------------------|------|------------------------------------------|---------------|--------------------------------------|  
| query_per_second         | int  | No (choose one of `query_per_second`, `query_per_minute`, `query_per_hour`, `query_per_day`) | -           | Allowed requests per second.         |  
| query_per_minute         | int  | No (choose one of `query_per_second`, `query_per_minute`, `query_per_hour`, `query_per_day`) | -           | Allowed requests per minute.         |  
| query_per_hour           | int  | No (choose one of `query_per_second`, `query_per_minute`, `query_per_hour`, `query_per_day`) | -           | Allowed requests per hour.           |  
| query_per_day            | int  | No (choose one of `query_per_second`, `query_per_minute`, `query_per_hour`, `query_per_day`) | -           | Allowed requests per day.            |  

### Configuration Fields for `rule_items`

| Configuration Item            | Type          | Required                          | Default Value | Description                                                                 |  
|-------------------------------|---------------|-----------------------------------|---------------|-----------------------------------------------------------------------------|  
| limit_by_header               | string        | No (choose one of `limit_by_*` fields) | -           | Configures the HTTP request header name to extract the rate limiting key.   |  
| limit_by_param                | string        | No (choose one of `limit_by_*` fields) | -           | Configures the URL parameter name to extract the rate limiting key.        |  
| limit_by_consumer             | string        | No (choose one of `limit_by_*` fields) | -           | Rate limits based on the consumer name (no need to add a specific value).   |  
| limit_by_cookie               | string        | No (choose one of `limit_by_*` fields) | -           | Configures the Cookie key name to extract the rate limiting key.           |  
| limit_by_per_header           | string        | No (choose one of `limit_by_*` fields) | -           | Calculates a separate limit per header value. `limit_keys` **MUST NOT be a literal name; only `*` or `regexp:...` is accepted**. For an exact match, use `limit_by_header` (drop `per_`). |
| limit_by_per_param            | string        | No (choose one of `limit_by_*` fields) | -           | Calculates a separate limit per parameter value. `limit_keys` **MUST NOT be a literal name; only `*` or `regexp:...` is accepted**. For an exact match, use `limit_by_param` (drop `per_`). |
| limit_by_per_consumer         | string        | No (choose one of `limit_by_*` fields) | -           | Calculates a separate limit per consumer. `limit_keys` **MUST NOT be a literal name; only `*` or `regexp:...` is accepted**. For an exact match, use `limit_by_consumer` (drop `per_`). |
| limit_by_per_cookie           | string        | No (choose one of `limit_by_*` fields) | -           | Calculates a separate limit per Cookie value. `limit_keys` **MUST NOT be a literal name; only `*` or `regexp:...` is accepted**. For an exact match, use `limit_by_cookie` (drop `per_`). |
| limit_by_per_ip               | string        | No (choose one of `limit_by_*` fields) | -           | Selects the client-IP source: `from-header-<header_name>` or `from-remote-addr`. Put IP/CIDR values in `limit_keys[].key`. |
| limit_keys                    | array of object | Yes                               | -           | Configures the rate limits for matched key values.                          |  

#### `rule_items` Multi-Rule Matching Semantics

`rule_items` is an array. **All** `rule_item`s whose match conditions are satisfied are evaluated, and the rules are combined with an OR relationship: **any trigger rejects the request**. The execution order of rules does not affect the final result.

> The `rule_items` array supports up to **10** rules. Each `rule_item` produces independent Redis counters for each matched `limit_keys`.

##### X-RateLimit-* headers in multi-rule scenarios

When multiple rules are matched and none trigger (with `show_limit_quota_header: true`):
- `X-RateLimit-Limit` / `X-RateLimit-Remaining`: from the matched rule with the smallest remaining ratio (tightest constraint)
- `X-RateLimit-Reset` (returned when triggered): from the first triggered rule (in `rule_items` array order, global first)  

### Configuration Fields for `limit_keys`

| Configuration Item       | Type   | Required                                 | Default Value | Description                                                                 |  
|--------------------------|--------|------------------------------------------|---------------|-----------------------------------------------------------------------------|  
| key                      | string | Yes                                      | -             | Non-`per_` types accept exact literals. `limit_by_per_header`, `limit_by_per_param`, `limit_by_per_consumer`, and `limit_by_per_cookie` accept only `regexp:...` or `*`. `limit_by_per_ip` accepts IP addresses or CIDR blocks. |
| query_per_second         | int    | No (choose one of `query_per_second`, `query_per_minute`, `query_per_hour`, `query_per_day`) | -           | Allowed requests per second.                                                |  
| query_per_minute         | int    | No (choose one of `query_per_second`, `query_per_minute`, `query_per_hour`, `query_per_day`) | -           | Allowed requests per minute.                                                |  
| query_per_hour           | int    | No (choose one of `query_per_second`, `query_per_minute`, `query_per_hour`, `query_per_day`) | -           | Allowed requests per hour.                                                  |  
| query_per_day            | int    | No (choose one of `query_per_second`, `query_per_minute`, `query_per_hour`, `query_per_day`) | -           | Allowed requests per day.                                                   |  

### Configuration Fields for `redis`

| Configuration Item   | Type   | Required | Default Value                                                     | Description                                                                 |  
|----------------------|--------|----------|-------------------------------------------------------------------|-----------------------------------------------------------------------------|  
| service_name         | string | Yes      | -                                                                 | The fully qualified domain name (FQDN) of the Redis service, including the service type (e.g., `my-redis.dns`, `redis.my-ns.svc.cluster.local`). |  
| service_port         | int    | No       | 80 (for static services), 6379 for other services                  | The port of the Redis service.                                              |  
| username             | string | No       | -                                                                 | Redis username for authentication.                                          |  
| password             | string | No       | -                                                                 | Redis password for authentication.                                          |  
| timeout              | int    | No       | 1000 (milliseconds)                                               | Redis connection timeout in milliseconds.                                  |  
| database             | int    | No       | 0                                                                 | The ID of the Redis database to use (e.g., configuring `1` corresponds to `SELECT 1`). |  

## Configuration Examples

### Global Rate Limiting for Custom Rule Group

```yaml  
rule_name: routeA-global-limit-rule
global_threshold:
  query_per_minute: 1000 # Maximum 1000 requests per minute for this rule group
redis:
  service_name: redis.static
show_limit_quota_header: true
```

### Rate Limiting by Request Parameter `apikey`

```yaml  
rule_name: routeA-request-param-limit-rule
rule_items:
  - limit_by_param: apikey
    limit_keys:
      - key: 9a342114-ba8a-11ec-b1bf-00163e1250b5
        query_per_minute: 10
      - key: a6a6d7f2-ba8a-11ec-bec2-00163e1250b5
        query_per_hour: 100
  - limit_by_per_param: apikey
    limit_keys:
      # Regular expression to match all strings starting with "a"; 10 requests per second for each apikey  
      - key: "regexp:^a.*"
        query_per_second: 10
      # Regular expression to match all strings starting with "b"; 100 requests per minute for each apikey  
      - key: "regexp:^b.*"
        query_per_minute: 100
      # Fallback rule to match all requests; 1000 requests per hour for each apikey  
      - key: "*"
        query_per_hour: 1000
redis:
  service_name: redis.static
show_limit_quota_header: true
```

## Common Pitfalls

### Literal consumer name under `limit_by_per_consumer`

```yaml
# ❌ Wrong: per_consumer limit_keys accepts only * or regexp:...
rule_items:
  - limit_by_per_consumer: ""
    limit_keys:
      - key: "alice"
        query_per_day: 100

# ✅ Right: drop per_ for an exact-name match
rule_items:
  - limit_by_consumer: ""
    limit_keys:
      - key: "alice"
        query_per_day: 100
```

### CIDR placed in `limit_by_per_ip`

```yaml
# ❌ Wrong: limit_by_per_ip selects the IP source; it is not a CIDR field
rule_items:
  - limit_by_per_ip: "0.0.0.0/0"

# ✅ Right: put the CIDR in limit_keys
rule_items:
  - limit_by_per_ip: "from-remote-addr"
    limit_keys:
      - key: "0.0.0.0/0"
        query_per_day: 1000
```

### Missing `limit_keys`

```yaml
# ❌ Wrong: the entire rule_item is rejected
rule_items:
  - limit_by_per_ip: "from-remote-addr"

# ✅ Right: provide at least one key and request threshold
rule_items:
  - limit_by_per_ip: "from-remote-addr"
    limit_keys:
      - key: "0.0.0.0/0"
        query_per_day: 1000
```

## Diagnosing

See the [Rate-limit Plugin FAQ](../../../../docs/faq/rate-limit-plugin-faq-en.md#rate-limit-not-taking-effect) for complete diagnosis recipes.

| Symptom | Check first |
| --- | --- |
| Requests always return 200 and Redis has no rate-limit keys | No rule matched, or configuration parsing prevented the rule from loading |
| Redis cluster `cx_connect_fail` is greater than 0 | Port, credentials, or network connectivity |
| Wasm `update_rejected` / `config_fail` keeps increasing | The parser rejected the pushed configuration; inspect the concrete gateway log error |

```bash
kubectl -n higress-system logs <gateway-pod> --tail=2000 \
  | grep -Ei 'cluster-key-rate-limit|limit_by_per_|missing limit_keys|must start with'
```

Seeing configuration in `config_dump` proves only that the control plane delivered it; it does not prove that the data-plane plugin loaded it. Correlate `/stats`, `/clusters`, and gateway logs.

## Configuration Examples (continued)

### Rate Limiting by Request Header `x-ca-key`

```yaml  
rule_name: routeA-request-header-limit-rule
rule_items:
  - limit_by_header: x-ca-key
    limit_keys:
      - key: 102234
        query_per_minute: 10
      - key: 308239
        query_per_hour: 10
  - limit_by_per_header: x-ca-key
    limit_keys:
      # Regular expression to match all strings starting with "a"; 10 requests per second for each key  
      - key: "regexp:^a.*"
        query_per_second: 10
      # Regular expression to match all strings starting with "b"; 100 requests per minute for each key  
      - key: "regexp:^b.*"
        query_per_minute: 100
      # Fallback rule to match all requests; 1000 requests per hour for each key  
      - key: "*"
        query_per_hour: 1000
redis:
  service_name: redis.static
show_limit_quota_header: true
```

### Rate Limiting by Client IP Extracted from `x-forwarded-for` Header

```yaml  
rule_name: routeA-client-ip-limit-rule
rule_items:
  - limit_by_per_ip: from-header-x-forwarded-for
    limit_keys:
      # Exact IP match  
      - key: 1.1.1.1
        query_per_day: 10
      # CIDR block match; 100 requests per day for each IP in the block  
      - key: 1.1.1.0/24
        query_per_day: 100
      # Fallback rule for all IPs; 1000 requests per day for each IP  
      - key: 0.0.0.0/0
        query_per_day: 1000
redis:
  service_name: redis.static
show_limit_quota_header: true
```

### Rate Limiting by Consumer

```yaml  
rule_name: routeA-consumer-limit-rule
rule_items:
  - limit_by_consumer: ''
    limit_keys:
      - key: consumer1
        query_per_second: 10
      - key: consumer2
        query_per_hour: 100
  - limit_by_per_consumer: ''
    limit_keys:
      # Regular expression to match all consumer names starting with "a"; 10 requests per second for each consumer  
      - key: "regexp:^a.*"
        query_per_second: 10
      # Regular expression to match all consumer names starting with "b"; 100 requests per minute for each consumer  
      - key: "regexp:^b.*"
        query_per_minute: 100
      # Fallback rule to match all consumers; 1000 requests per hour for each consumer  
      - key: "*"
        query_per_hour: 1000
redis:
  service_name: redis.static
show_limit_quota_header: true
```

### Rate Limiting by Cookie Value

```yaml  
rule_name: routeA-cookie-limit-rule
rule_items:
  - limit_by_cookie: key1
    limit_keys:
      - key: value1
        query_per_minute: 10
      - key: value2
        query_per_hour: 100
  - limit_by_per_cookie: key1
    limit_keys:
      # Regular expression to match all cookie values starting with "a"; 10 requests per second for each value  
      - key: "regexp:^a.*"
        query_per_second: 10
      # Regular expression to match all cookie values starting with "b"; 100 requests per minute for each value  
      - key: "regexp:^b.*"
        query_per_minute: 100
      # Fallback rule to match all cookie values; 1000 requests per hour for each value  
      - key: "*"
        query_per_hour: 1000
rejected_code: 200
rejected_msg: '{"code":-1,"msg":"Too many requests"}'
redis:
  service_name: redis.static
show_limit_quota_header: true
```
