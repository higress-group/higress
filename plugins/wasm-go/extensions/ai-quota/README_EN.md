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

### Refresh Quota
If the suffix of the current request URL matches the admin_path, for example, if the plugin is effective on the route example.com/v1/chat/completions, then the quota can be updated via:
curl https://example.com/v1/chat/completions/quota/refresh -H "Authorization: Bearer credential3" -d "consumer=consumer1&quota=10000"
The value of the key `chat_quota:consumer1` in Redis will be refreshed to 10000.

### Query Quota
To query the quota of a specific user, you can use: 
curl https://example.com/v1/chat/completions/quota?consumer=consumer1 -H "Authorization: Bearer credential3"
The response will return: {"quota": 10000, "consumer": "consumer1"}

### Increase or Decrease Quota
To increase or decrease the quota of a specific user, you can use:
curl https://example.com/v1/chat/completions/quota/delta -d "consumer=consumer1&value=100" -H "Authorization: Bearer credential3"
This will increase the value of the key `chat_quota:consumer1` in Redis by 100, and negative values can also be supported, thus subtracting the corresponding value.
