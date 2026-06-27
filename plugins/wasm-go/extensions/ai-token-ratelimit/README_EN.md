---
title: AI Token Rate Limiting
keywords: [ AI Gateway, AI Token Rate Limiting ]
description: AI Token Rate Limiting Plugin Configuration Reference
---

> ⚠️ **Behavior Change Notice (no version bump)**
>
> As of this update, `rule_items` matching semantics changed from **first-match-wins** (returns on first hit) to **all-match OR overlay** (all matched rules are evaluated, any trigger rejects). The mutual exclusion between `global_threshold` and `rule_items` is also removed to support hybrid configuration.
>
> - Existing config (single `rule_items` or only `global_threshold`): behavior unchanged
> - Existing config (multiple `rule_items` expecting short-circuit match): **behavior will change** — all matched rules are evaluated
> - Redis key format adds `{rule_name}` hash tag for Redis Cluster compatibility; old counter data is incompatible
>
> See "Features" and "Configuration Examples" below for details.

## Function Description

The `ai-token-ratelimit` plugin implements AI Token rate limiting based on Redis, supporting the following three rate limiting modes:

- **Rule-level Global Rate Limiting**: Sets a global token rate limit threshold for custom rule groups based on the same `rule_name` and `global_threshold` configurations.
- **Key-level Dynamic Rate Limiting**: Performs grouped token rate limiting based on dynamic keys in requests (including URL parameters, request headers, client IP, Consumer name, or Cookie fields, etc.).
- **Hybrid rate limiting**: Configure `global_threshold` (global fallback) and `rule_items` (per-dimension) simultaneously. All matched rules take effect together; any trigger rejects the request.


## Runtime Properties

Plugin execution phase: `Default Phase`
Plugin execution priority: `600`


## Configuration Description

| Configuration Item       | Type           | Required | Default Value | Description                                                                                     |
|--------------------------|----------------|----------|---------------|-------------------------------------------------------------------------------------------------|
| rule_name                | string         | Yes      | -             | Name of the rate limiting rule. The Redis key is assembled based on the rate limiting rule name + rate limiting type + rate limiting key name + actual value corresponding to the rate limiting key. |
| global_threshold         | Object         | No, at least one of them is required; can be configured simultaneously | - | Rate limits the entire custom rule group |
| rule_items               | array of object| No, at least one of them is required; can be configured simultaneously | - | Rate limiting rule items. Supports up to **10** rules. All matched `rule_item`s are evaluated and combined with an OR relationship; any trigger rejects the request. The execution order of rules does not affect the final result. See the expanded `rule_items` notes below for details. |
| rejected_code            | int            | No       | 429           | HTTP status code returned when a request is rate-limited                                         |
| rejected_msg             | string         | No       | Too many requests | Response body returned when a request is rate-limited                                            |
| redis                    | object         | Yes      | -             | Redis-related configurations                                                                   |


### Description of Configuration Fields in `global_threshold`

| Configuration Item    | Type | Required | Default Value | Description                                   |
|-----------------------|------|----------|---------------|-----------------------------------------------|
| token_per_second      | int  | No, one of `token_per_second`, `token_per_minute`, `token_per_hour`, `token_per_day` is required | - | Allowed number of request tokens per second   |
| token_per_minute      | int  | No, one of `token_per_second`, `token_per_minute`, `token_per_hour`, `token_per_day` is required | - | Allowed number of request tokens per minute   |
| token_per_hour        | int  | No, one of `token_per_second`, `token_per_minute`, `token_per_hour`, `token_per_day` is required | - | Allowed number of request tokens per hour     |
| token_per_day         | int  | No, one of `token_per_second`, `token_per_minute`, `token_per_hour`, `token_per_day` is required | - | Allowed number of request tokens per day      |


### Description of Configuration Fields in `rule_items`

| Configuration Item          | Type            | Required | Default Value | Description                                                                                                                                                                                                 |
|-----------------------------|-----------------|----------|---------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| limit_by_header             | string          | No, one of `limit_by_*` is required | - | Configures the source of the rate limiting key value as the HTTP request header name                                                                                                                         |
| limit_by_param              | string          | No, one of `limit_by_*` is required | - | Configures the source of the rate limiting key value as the URL parameter name                                                                                                                              |
| limit_by_consumer           | string          | No, one of `limit_by_*` is required | - | Performs rate limiting based on the consumer name; no actual value needs to be added                                                                                                                         |
| limit_by_cookie             | string          | No, one of `limit_by_*` is required | - | Configures the source of the rate limiting key value as the key name in the Cookie                                                                                                                           |
| limit_by_per_header         | string          | No, one of `limit_by_*` is required | - | Matches specific HTTP request headers by rule and calculates rate limits for each header separately. Configures the source of the rate limiting key value as the HTTP request header name. Regular expressions or `*` are supported when configuring `limit_keys`. |
| limit_by_per_param          | string          | No, one of `limit_by_*` is required | - | Matches specific URL parameters by rule and calculates rate limits for each parameter separately. Configures the source of the rate limiting key value as the URL parameter name. Regular expressions or `*` are supported when configuring `limit_keys`.       |
| limit_by_per_consumer       | string          | No, one of `limit_by_*` is required | - | Matches specific consumers by rule and calculates rate limits for each consumer separately. Performs rate limiting based on the consumer name; no actual value needs to be added. Regular expressions or `*` are supported when configuring `limit_keys`.      |
| limit_by_per_cookie         | string          | No, one of `limit_by_*` is required | - | Matches specific Cookies by rule and calculates rate limits for each Cookie separately. Configures the source of the rate limiting key value as the key name in the Cookie. Regular expressions or `*` are supported when configuring `limit_keys`.             |
| limit_by_per_ip             | string          | No, one of `limit_by_*` is required | - | Matches specific IPs by rule and calculates rate limits for each IP separately. Configures the source of the rate limiting key value as the IP parameter name, obtained from the request header in the format `from-header-corresponding_header_name` (e.g., `from-header-x-forwarded-for`), or directly obtains the peer socket IP by configuring `from-remote-addr`. |
| limit_keys                  | array of object | Yes      | -             | Configures the rate limiting count after matching the key value                                                                                                                                             |

#### `rule_items` Multi-Rule Matching Semantics

`rule_items` is an array. **All** `rule_item`s whose match conditions are satisfied are evaluated, and the rules are combined with an OR relationship: **any trigger rejects the request**. The execution order of rules does not affect the final result.

> The `rule_items` array supports up to **10** rules. Each `rule_item` produces independent Redis counters for each matched `limit_keys`.

##### Duplicate rule_items (not allowed)

A `rule_item` with the same `limit_by_*` + key combination may **only appear once**, including multi-window scenarios where the same key has different time windows (e.g., per-minute and per-hour limits for the same apikey). Duplicate declarations will emit a warn log during config parsing:

```yaml
rule_items:
  - limit_by_param: apikey
    limit_keys: [{key: k1, token_per_minute: 100}]
  - limit_by_param: apikey
    limit_keys: [{key: k1, token_per_hour: 1000}]   # duplicate: same type+key, triggers warn
```


### Description of Configuration Fields in `limit_keys`

| Configuration Item    | Type   | Required | Default Value | Description                                                                                                                                                                                                 |
|-----------------------|--------|----------|---------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| key                   | string | Yes      | -             | The matched key value. For types `limit_by_per_header`, `limit_by_per_param`, `limit_by_per_consumer`, and `limit_by_per_cookie`, regular expressions (starting with `regexp:` followed by the regular expression, e.g., `regexp:^d.*` for all strings starting with "d") or `*` (representing all) are supported. For `limit_by_per_ip`, IP addresses or IP segments are supported. |
| token_per_second      | int    | No, one of `token_per_second`, `token_per_minute`, `token_per_hour`, `token_per_day` is required | - | Allowed number of request tokens per second   |
| token_per_minute      | int    | No, one of `token_per_second`, `token_per_minute`, `token_per_hour`, `token_per_day` is required | - | Allowed number of request tokens per minute   |
| token_per_hour        | int    | No, one of `token_per_second`, `token_per_minute`, `token_per_hour`, `token_per_day` is required | - | Allowed number of request tokens per hour     |
| token_per_day         | int    | No, one of `token_per_second`, `token_per_minute`, `token_per_hour`, `token_per_day` is required | - | Allowed number of request tokens per day      |


### Description of Configuration Fields in `redis`

| Configuration Item | Type   | Required | Default Value | Description                                                                                     |
|--------------------|--------|----------|---------------|-------------------------------------------------------------------------------------------------|
| service_name       | string | Yes      | -             | Redis service name, a complete FQDN with service type, e.g., my-redis.dns, redis.my-ns.svc.cluster.local |
| service_port       | int    | No       | 80 for static services, 6379 for others | Enter the service port of the Redis service                                                     |
| username           | string | No       | -             | Redis username                                                                                  |
| password           | string | No       | -             | Redis password                                                                                  |
| timeout            | int    | No       | 1000          | Redis connection timeout in milliseconds                                                       |
| database           | int    | No       | 0             | The database ID to use, e.g., configuring 1 corresponds to `SELECT 1`                            |


## Configuration Example

### Custom Rule Group Global Rate Limiting

```yaml
rule_name: routeA-global-limit-rule
global_threshold:
  token_per_minute: 1000 # 1000 tokens per minute for the custom rule group
redis:
  service_name: redis.static
```

### Identify request parameter apikey for differentiated rate limiting
```yaml
rule_name: default_rule
rule_items:
  - limit_by_param: apikey
    limit_keys:
      - key: 9a342114-ba8a-11ec-b1bf-00163e1250b5
        token_per_minute: 10
      - key: a6a6d7f2-ba8a-11ec-bec2-00163e1250b5
        token_per_hour: 100
  - limit_by_per_param: apikey
    limit_keys:
      # Regular expression, matches all strings starting with a, each apikey corresponds to 10 qds
      - key: "regexp:^a.*"
        token_per_second: 10
      # Regular expression, matches all strings starting with b, each apikey corresponds to 100 qd
      - key: "regexp:^b.*"
        token_per_minute: 100
      # Fallback, matches all requests, each apikey corresponds to 1000 qdh
      - key: "*"
        token_per_hour: 1000
redis:
  service_name: redis.static
```
### Identify request header x-ca-key for differentiated rate limiting
```yaml
rule_name: default_rule
rule_items:
  - limit_by_header: x-ca-key
    limit_keys:
      - key: 102234
        token_per_minute: 10
      - key: 308239
        token_per_hour: 10
  - limit_by_per_header: x-ca-key
    limit_keys:
      # Regular expression, matches all strings starting with a, each apikey corresponds to 10 qds
      - key: "regexp:^a.*"
        token_per_second: 10
      # Regular expression, matches all strings starting with b, each apikey corresponds to 100 qd
      - key: "regexp:^b.*"
        token_per_minute: 100
      # Fallback, matches all requests, each apikey corresponds to 1000 qdh
      - key: "*"
        token_per_hour: 1000
redis:
  service_name: redis.static
```
### Get the peer IP using the request header x-forwarded-for for differentiated rate limiting
```yaml
rule_name: default_rule
rule_items:
  - limit_by_per_ip: from-header-x-forwarded-for
    limit_keys:
      # Exact IP
      - key: 1.1.1.1
        token_per_day: 10
      # IP segment, matching IPs in this segment, each IP 100 qpd
      - key: 1.1.1.0/24
        token_per_day: 100
      # Fallback, i.e., default each IP 1000 qpd
      - key: 0.0.0.0/0
        token_per_day: 1000
redis:
  service_name: redis.static
```
### Identify consumer for differentiated rate limiting
```yaml
rule_name: default_rule
rule_items:
  - limit_by_consumer: ''
    limit_keys:
      - key: consumer1
        token_per_second: 10
      - key: consumer2
        token_per_hour: 100
  - limit_by_per_consumer: ''
    limit_keys:
      # Regular expression, matches all strings starting with a, each consumer corresponds to 10 qds
      - key: "regexp:^a.*"
        token_per_second: 10
      # Regular expression, matches all strings starting with b, each consumer corresponds to 100 qd
      - key: "regexp:^b.*"
        token_per_minute: 100
      # Fallback, matches all requests, each consumer corresponds to 1000 qdh
      - key: "*"
        token_per_hour: 1000
redis:
  service_name: redis.static
```
### Identify key-value pairs in cookies for differentiated rate limiting
```yaml
rule_name: default_rule
rule_items:
  - limit_by_cookie: key1
    limit_keys:
      - key: value1
        token_per_minute: 10
      - key: value2
        token_per_hour: 100
  - limit_by_per_cookie: key1
    limit_keys:
      # Regular expression, matches all strings starting with a, each value in cookie corresponds to 10 qds
      - key: "regexp:^a.*"
        token_per_second: 10
      # Regular expression, matches all strings starting with b, each value in cookie corresponds to 100 qd
      - key: "regexp:^b.*"
        token_per_minute: 100
      # Fallback, matches all requests, each value in cookie corresponds to 1000 qdh
      - key: "*"
        token_per_hour: 1000
rejected_code: 200
rejected_msg: '{"code":-1,"msg":"Too many requests"}'
redis:
  service_name: redis.static
```

## Example

The AI Token Rate Limiting Plugin relies on Redis to track the remaining available tokens, so the Redis service must be deployed first.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  labels:
    app: redis
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis
        ports:
        - containerPort: 6379
---
apiVersion: v1
kind: Service
metadata:
  name: redis
  labels:
    app: redis
spec:
  ports:
  - port: 6379
    targetPort: 6379
  selector:
    app: redis
---
```

In this example, qwen is used as the AI service provider. Additionally, the AI Statistics Plugin must be configured, as the AI Token Rate Limiting Plugin depends on it to calculate the number of tokens consumed per request. The following configuration limits the total number of input and output tokens to 200 per minute.

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: ai-proxy
  namespace: higress-system
spec:
  matchRules:
  - config:
      provider:
        type: qwen
        apiTokens:
        - "<YOUR_API_TOKEN>"
        modelMapping:
          'gpt-3': "qwen-turbo"
          'gpt-35-turbo': "qwen-plus"
          'gpt-4-turbo': "qwen-max"
          '*': "qwen-turbo"
    ingress:
    - qwen
  url: oci://higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/ai-proxy:1.0.0
  phase: UNSPECIFIED_PHASE
  priority: 100
---
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: ai-token-ratelimit
  namespace: higress-system
spec:
  defaultConfig:
    rule_name: default_limit_by_param_apikey
    rule_items:
    - limit_by_param: apikey
      limit_keys:
      - key: 123456
        token_per_minute: 200
    redis:
      # By default, to reduce data plane pressure, the `global.onlyPushRouteCluster` parameter in Higress is set to true, meaning that Kubernetes Services are not automatically discovered.
      # If you need to use Kubernetes Service for service discovery, set `global.onlyPushRouteCluster` to false,
      # allowing you to directly set `service_name` to the Kubernetes Service without needing to create an McpBridge and an Ingress route for Redis.
      # service_name: redis.default.svc.cluster.local
      service_name: redis.dns
      service_port: 6379
  url: oci://higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/ai-token-ratelimit:1.0.0
  phase: UNSPECIFIED_PHASE
  priority: 600
```

Note that the `service_name` in the Redis configuration of the AI Token Rate Limiting Plugin is derived from the service source configured in McpBridge. Additionally, we need to configure the access address of the qnwen service in McpBridge.

```yaml
apiVersion: networking.higress.io/v1
kind: McpBridge
metadata:
  name: default
  namespace: higress-system
spec:
  registries:
  - domain: dashscope.aliyuncs.com
    name: qwen
    port: 443
    type: dns
  - domain: redis.default.svc.cluster.local # Kubernetes Service
    name: redis
    type: dns
    port: 6379
```

Create two routing rules separately.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    higress.io/backend-protocol: HTTPS
    higress.io/destination: qwen.dns
    higress.io/proxy-ssl-name: dashscope.aliyuncs.com
    higress.io/proxy-ssl-server-name: "on"
  labels:
    higress.io/resource-definer: higress
  name: qwen
  namespace: higress-system
spec:
  ingressClassName: higress
  rules:
  - host: qwen-test.com
    http:
      paths:
      - backend:
          resource:
            apiGroup: networking.higress.io
            kind: McpBridge
            name: default
        path: /
        pathType: Prefix
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    higress.io/destination: redis.dns
    higress.io/ignore-path-case: "false"
  labels:
    higress.io/resource-definer: higress
  name: redis
spec:
  ingressClassName: higress
  rules:
  - http:
      paths:
      - backend:
          resource:
            apiGroup: networking.higress.io
            kind: McpBridge
            name: default
        path: /
        pathType: Prefix
```

Forward the traffic of higress-gateway to the local, making it convenient for testing.

```bash
kubectl port-forward svc/higress-gateway -n higress-system 18000:80
```

The rate limiting effect is triggered as follows:

```bash
curl "http://localhost:18000/v1/chat/completions?apikey=123456" \
-H "Host: qwen-test.com" \
-H "Content-Type: application/json" \
-d '{
  "model": "gpt-3",
  "messages": [
    {
      "role": "user",
      "content": "Hello, who are you?"
    }
  ],
  "stream": false
}'
{"id":"88cfa80f-545d-93b4-8ff3-3f5245ca33ba","choices":[{"index":0,"message":{"role":"assistant","content":"I am Tongyi Qianwen, an AI assistant developed by Alibaba Cloud. I can answer various questions, provide information, and have conversations with users. How can I assist you?"},"finish_reason":"stop"}],"created":1719909825,"model":"qwen-turbo","object":"chat.completion","usage":{"prompt_tokens":13,"completion_tokens":33,"total_tokens":46}}
curl "http://qwen-test.com:18000/v1/chat/completions?apikey=123456" -H "Content-Type: application/json"  -d '{
  "model": "gpt-3",
  "messages": [
    {
      "role": "user",
      "content": "Hello, who are you?"
    }
  ],
  "stream": false
}'
Too many requests  # Rate limiting successful
```
