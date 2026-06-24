---
title: AI Quota Management
keywords: [ AI Gateway, AI Quota ]
description: AI quota management plugin configuration reference
---
## Function Description
The `ai-quota` plugin implements quota rate limiting based on fixed quotas allocated to specific consumers. It also supports quota management capabilities, including querying quotas, refreshing quotas, and increasing or decreasing quotas. The `ai-quota` plugin needs to work with authentication plugins such as `key-auth`, `jwt-auth`, etc., to obtain the consumer name associated with the authenticated identity, and it needs to work with the `ai-statistics` plugin to obtain AI Token statistical information.

## Runtime Properties
Plugin execution phase: `default phase`
Plugin execution priority: `750`

## Configuration Description
| Name                 | Data Type        | Required Conditions                         | Default Value | Description                                       |
|---------------------|------------------|--------------------------------------------|---------------|---------------------------------------------------|
| `redis_key_prefix`  | string           | Optional                                   |   chat_quota: | Quota redis key prefix                            |
| `admin_consumer`    | string           | Required                                   |               | Consumer name for managing quota management identity |
| `admin_path`        | string           | Optional                                   |   /quota      | Prefix for the path to manage quota requests      |
| `enable_path_suffixes` | []string      | Optional                                   | ["/v1/chat/completions", "/v1/messages"] | Enabled path suffixes for completion quota checks only; does not affect admin API path |
| `redis`             | object           | Yes                                        |               | Redis related configuration                        |
Explanation of each configuration field in `redis`
| Configuration Item | Type   | Required | Default Value                                           | Explanation                                                                                             |
|--------------------|--------|----------|---------------------------------------------------------|---------------------------------------------------------------------------------------------------------|
| service_name       | string | Required | -                                                       | Redis service name, full FQDN name with service type, e.g., my-redis.dns, redis.my-ns.svc.cluster.local |
| service_port       | int    | No       | Default value for static service is 80; others are 6379 | Service port for the redis service                                                                      |
| username           | string | No       | -                                                       | Redis username                                                                                          |
| password           | string | No       | -                                                       | Redis password                                                                                          |
| timeout            | int    | No       | 1000                                                    | Redis connection timeout in milliseconds                                                                |
| database           | int    | No       | 0                                                       | The database ID used, for example, configured as 1, corresponds to `SELECT 1`.                          |

## Configuration Example
### Identify request parameter apikey and apply rate limiting accordingly
```yaml
redis_key_prefix: "chat_quota:"
admin_consumer: consumer3
admin_path: /quota
redis:
  service_name: redis-service.default.svc.cluster.local
  service_port: 6379
  timeout: 2000
```

### Group Shared Pool

ai-quota supports per-consumer-group shared quota pools: any consumer in the group decrements the shared pool, while the consumer's private pool is still charged per-individual. A request is allowed only when both pools are > 0 (strict mode).

> **Where `group` comes from**: `group` is not a configuration field of ai-quota itself — it is injected by an upstream authentication plugin (e.g., `key-auth`) via the `X-Mse-Consumer-Group` HTTP header at authentication time, and ai-quota only reads it at request phase. To make a consumer a member of a group, add the `group: <name>` field to the consumer definition in `key-auth`. When `X-Mse-Consumer-Group` is absent, ai-quota falls back to the legacy single-pool path and only charges the consumer's private pool.

> **Redis Cluster compatibility**: when `group` is set, Phase 1/2 use Lua scripts that operate on both the group and consumer keys atomically. Redis Cluster requires all keys in a multi-key operation to land on the same slot, otherwise it returns `CROSSSLOT`. ai-quota uses the key format `{chat_quota}:<subject>`, where `{}` acts as a hash tag ensuring all quota keys land on the same slot. **Trade-off**: all ai-quota traffic concentrates on a single slot, losing Cluster sharding.
>
> **Upgrade migration**: in versions ≤ v1.0.x, keys had the form `chat_quota:consumer1`; in the new version they become `{chat_quota}:consumer1`. After upgrade, either re-run `refresh` for every consumer/group via the admin API, or flush the old-prefixed keys in Redis.

### Refresh Quota
If the suffix of the current request URL matches the admin_path, for example, if the plugin is effective on the route example.com/v1/chat/completions, then the quota can be updated via:
curl https://example.com/v1/chat/completions/quota/refresh -H "Authorization: Bearer credential3" -d "consumer=consumer1&quota=10000"
The value of the key `chat_quota:consumer1` in Redis will be refreshed to 10000.

Refresh a group shared pool:
```bash
curl https://example.com/v1/chat/completions/quota/refresh -H "Authorization: Bearer credential3" -d "group=team-a&quota=10000"
```
The Redis key `chat_quota:team-a` is set to 10000. Exactly one of `consumer` or `group` must be set; setting both or neither returns `400 ai-quota.invalid_param`.

### Query Quota
To query the quota of a specific user, you can use:
curl https://example.com/v1/chat/completions/quota?consumer=consumer1 -H "Authorization: Bearer credential3"
The response will return: `{"name": "consumer1", "quota": 10000}`

Query a group shared pool:
```bash
curl https://example.com/v1/chat/completions/quota?group=team-a -H "Authorization: Bearer credential3"
```
Returns: `{"name":"team-a","quota":10000}`

### Increase or Decrease Quota
To increase or decrease the quota of a specific user, you can use:
curl https://example.com/v1/chat/completions/quota/delta -d "consumer=consumer1&value=100" -H "Authorization: Bearer credential3"
This will increase the value of the key `chat_quota:consumer1` in Redis by 100, and negative values can also be supported, thus subtracting the corresponding value.

Increase or decrease a group shared pool:
```bash
curl https://example.com/v1/chat/completions/quota/delta -d "group=team-a&value=500" -H "Authorization: Bearer credential3"
```
Adjusts the Redis key `chat_quota:team-a` by 500 (negative values supported).

## Error Codes

| HTTP | Code | Trigger |
|------|------|---------|
| 200 | `ai-quota.refreshquota` | admin `/refresh` succeeded |
| 200 | `ai-quota.queryquota` | admin `/quota` query succeeded |
| 200 | `ai-quota.deltaquota` | admin `/delta` succeeded |
| 200 | - | normal chat completion allowed |
| 429 | `ai-quota.group_exhausted` | group shared pool ≤ 0 (only when `group != ""`) |
| 429 | `ai-quota.consumer_exhausted` | consumer private pool ≤ 0 |
| 429 | `ai-quota.both_exhausted` | both pools ≤ 0 (only when `group != ""`) |
| 400 | `ai-quota.invalid_param` | admin API parameter error (consumer/group mutual-exclusion violation, or non-integer quota/value) |
| 503 | `ai-quota.error` | Redis call failed |
| 401 | `ai-quota.no_key` | missing `X-Mse-Consumer` header |
| 403 | `ai-quota.unauthorized` | consumer not configured, or non-admin consumer calling admin API |

> **Breaking changes (≤ v1.0.x → current)**:
>
> 1. **HTTP status**: chat-completion quota denial changed from `403 ai-quota.noquota` to `429 ai-quota.consumer_exhausted`. Clients that match on the 403/noquota string must be updated.
> 2. **New admin `group` parameter**: `refresh`/`query`/`delta` now accept an optional `group` parameter, **mutually exclusive** with `consumer` (setting both or neither returns `400 ai-quota.invalid_param`).
> 3. **New quota-denial codes**: `ai-quota.group_exhausted` (group pool exhausted) and `ai-quota.both_exhausted` (both pools exhausted). Clients that match only `consumer_exhausted` will not be able to distinguish the rejection reason.
> 4. **Admin parameter-error migration**: in the old version, missing consumer / non-integer quota returned `403 ai-quota.unauthorized`; the new version returns `400 ai-quota.invalid_param`, separating parameter errors from authentication failures.
> 5. **Redis key format**: `{redis_key_prefix}<subject>` → `{<prefix>}:<subject>` (e.g., `chat_quota:consumer1` → `{chat_quota}:consumer1`) for Redis Cluster hash tags. After upgrade, re-run `refresh` once or flush the old-prefixed keys.
> 6. **admin `queryQuota` JSON field**: `"consumer"` renamed to `"name"`, so the same field carries either consumer or group names.
