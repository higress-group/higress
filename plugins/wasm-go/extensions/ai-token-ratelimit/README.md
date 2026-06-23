---
title: AI Token限流
keywords: [ AI网关, AI token限流 ]
description: AI Token限流插件配置参考
---

> ⚠️ **行为变更提示（无版本号变化）**
>
> 自本次更新起，`rule_items` 的匹配语义从 **first-match-wins**（命中第一条即返回）改为 **all-match OR 叠加**（所有命中规则都评估，任一触发即拒绝）。同时解除 `global_threshold` 与 `rule_items` 的互斥约束，支持混合配置。
>
> - 老配置（单条 `rule_items` 或仅 `global_threshold`）：行为不变
> - 老配置（多条 `rule_items` 期望短路匹配）：**行为会变** —— 所有命中规则都会评估
> - Redis key 格式新增 `{rule_name}` hash tag 以兼容 Redis Cluster，老计数器数据不兼容
>
> 详见下方"功能说明"与"配置示例"。

## 功能说明

`ai-token-ratelimit`插件基于 Redis 实现了 AI Token 限流功能，支持以下三种限流模式：

- **规则级全局限流**：依据相同的`rule_name`与`global_threshold`配置，为自定义规则组设置全局 token 限流阈值
- **Key 级动态限流**：根据请求中的动态 Key（包括 URL 参数、请求头、客户端 IP、Consumer 名称或 Cookie 字段等）进行分组 token 限流
- **混合限流**：同时配置 `global_threshold`（全局兜底）和 `rule_items`（按维度细分），所有命中的规则**叠加生效**，任一触发即拒绝。

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`600`

## 配置说明

| 配置项                  | 类型   | 必填 | 默认值 | 说明                                                                        |
| ----------------------- | ------ | ---- | ------ |---------------------------------------------------------------------------|
| rule_name               | string | 是 | - | 限流规则名称，根据限流规则名称+限流类型+限流key名称+限流key对应的实际值来拼装redis key                      |
| global_threshold | Object | 否，至少一项；可同时配置 | - | 对整个自定义规则组进行限流 |
| rule_items | array of object | 否，至少一项；可同时配置 | -                | 限流规则项，最多支持 **10 条**。所有满足匹配条件的 `rule_item` 都会参与限流，规则之间是"或"关系，任一触发即拒绝；规则的执行顺序不影响最终结果。详见下方"配置说明"展开。 |
| rejected_code           | int | 否 | 429 | 请求被限流时，返回的HTTP状态码                                                         |
| rejected_msg            | string | 否 | Too many requests | 请求被限流时，返回的响应体                                                             |
| redis                   | object          | 是                                                           | -                | redis相关配置                                                                 |

`global_threshold` 中每一项的配置字段说明。

| 配置项           | 类型 | 必填                                                         | 默认值 | 说明                  |
| ---------------- | ---- | ------------------------------------------------------------ | ------ | --------------------- |
| token_per_second | int  | 否，`token_per_second`,`token_per_minute`,`token_per_hour`,`token_per_day` 中选填一项 | -      | 允许每秒请求token数   |
| token_per_minute | int  | 否，`token_per_second`,`token_per_minute`,`token_per_hour`,`token_per_day` 中选填一项 | -      | 允许每分钟请求token数 |
| token_per_hour   | int  | 否，`token_per_second`,`token_per_minute`,`token_per_hour`,`token_per_day` 中选填一项 | -      | 允许每小时请求token数 |
| token_per_day    | int  | 否，`token_per_second`,`token_per_minute`,`token_per_hour`,`token_per_day` 中选填一项 | -      | 允许每天请求token数   |

`rule_items`中每一项的配置字段说明。

| 配置项                | 类型            | 必填                       | 默认值 | 说明                                                         |
| --------------------- | --------------- | -------------------------- | ------ | ------------------------------------------------------------ |
| limit_by_header       | string          | 否，`limit_by_*`中选填一项 | -      | 配置获取限流键值的来源 HTTP 请求头名称                       |
| limit_by_param        | string          | 否，`limit_by_*`中选填一项 | -      | 配置获取限流键值的来源 URL 参数名称                          |
| limit_by_consumer     | string          | 否，`limit_by_*`中选填一项 | -      | 根据 consumer 名称进行限流，无需添加实际值                   |
| limit_by_cookie       | string          | 否，`limit_by_*`中选填一项 | -      | 配置获取限流键值的来源 Cookie中 key 名称                     |
| limit_by_per_header   | string          | 否，`limit_by_*`中选填一项 | -      | 按规则匹配特定 HTTP 请求头，并对每个请求头分别计算限流，配置获取限流键值的来源 HTTP 请求头名称，配置`limit_keys`时支持正则表达式或`*` |
| limit_by_per_param    | string          | 否，`limit_by_*`中选填一项 | -      | 按规则匹配特定 URL 参数，并对每个参数分别计算限流，配置获取限流键值的来源 URL 参数名称，配置`limit_keys`时支持正则表达式或`*` |
| limit_by_per_consumer | string          | 否，`limit_by_*`中选填一项 | -      | 按规则匹配特定 consumer，并对每个 consumer 分别计算限流，根据 consumer 名称进行限流，无需添加实际值，配置`limit_keys`时支持正则表达式或`*` |
| limit_by_per_cookie   | string          | 否，`limit_by_*`中选填一项 | -      | 按规则匹配特定 Cookie，并对每个 Cookie 分别计算限流，配置获取限流键值的来源 Cookie中 key 名称，配置`limit_keys`时支持正则表达式或`*` |
| limit_by_per_ip       | string          | 否，`limit_by_*`中选填一项 | -      | 按规则匹配特定 IP，并对每个 IP 分别计算限流，配置获取限流键值的来源 IP 参数名称，从请求头获取，以`from-header-对应的header名`，示例：`from-header-x-forwarded-for`，直接获取对端socket ip，配置为`from-remote-addr` |
| limit_keys            | array of object | 是                         | -      | 配置匹配键值后的限流次数                                     |

#### `rule_items` 多规则匹配语义

`rule_items` 是一个数组，**所有满足匹配条件**的 `rule_item` 都会被评估，规则之间是"或"关系，**任一触发即拒绝**。规则的执行顺序不影响最终结果。

> `rule_items` 数组最多支持 **10 条**规则。每条 `rule_item` 会按命中的 `limit_keys` 产生独立的 Redis 计数器。

##### 同维度多值匹配（合并到一条 rule_item）

同一 `limit_by_*` + key 下的多个匹配值（如多个 apikey）应收纳到**一条** `rule_item` 的 `limit_keys` 数组中，不要拆成多条 `rule_item`：

❌ 错误（3 条 rule_item，浪费配额）：
```yaml
rule_items:
  - { limit_by_param: apikey, limit_keys: [{key: k1, token_per_minute: 100}] }
  - { limit_by_param: apikey, limit_keys: [{key: k2, token_per_minute: 200}] }
  - { limit_by_param: apikey, limit_keys: [{key: k3, token_per_minute: 300}] }
```

✅ 正确（1 条 rule_item）：
```yaml
rule_items:
  - limit_by_param: apikey
    limit_keys:
      - { key: k1, token_per_minute: 100 }
      - { key: k2, token_per_minute: 200 }
      - { key: k3, token_per_minute: 300 }
```

##### 多时间窗口限流（合法的"重复"）

同一 `limit_by_*` + key 但**不同时间窗**的两条 `rule_items` 是合法配置，会产生两个独立 Redis 计数器，实现"每分钟 + 每小时"双层限流（解析时会输出信息性 warn，可忽略）：

```yaml
rule_items:
  - limit_by_param: apikey
    limit_keys: [{key: k1, token_per_minute: 100}]
  - limit_by_param: apikey
    limit_keys: [{key: k1, token_per_hour: 1000}]   # 同 type+key，不同时间窗
```

##### 完全相同的重复（应避免）

同 type+key+时间窗的完全重复会产生同一 Redis key 的多次评估，属冗余配置，应合并为一条。

`limit_keys`中每一项的配置字段说明。

| 配置项           | 类型   | 必填                                                         | 默认值 | 说明                                                         |
| ---------------- | ------ | ------------------------------------------------------------ | ------ | ------------------------------------------------------------ |
| key              | string | 是                                                           | -      | 匹配的键值，`limit_by_per_header`,`limit_by_per_param`,`limit_by_per_consumer`,`limit_by_per_cookie` 类型支持配置正则表达式（以regexp:开头后面跟正则表达式）或者*（代表所有），正则表达式示例：`regexp:^d.*`（以d开头的所有字符串）；`limit_by_per_ip`支持配置 IP 地址或 IP 段 |
| token_per_second | int    | 否，`token_per_second`,`token_per_minute`,`token_per_hour`,`token_per_day` 中选填一项 | -      | 允许每秒请求token数                                             |
| token_per_minute | int    | 否，`token_per_second`,`token_per_minute`,`token_per_hour`,`token_per_day` 中选填一项 | -      | 允许每分钟请求token数                                           |
| token_per_hour   | int    | 否，`token_per_second`,`token_per_minute`,`token_per_hour`,`token_per_day` 中选填一项 | -      | 允许每小时请求token数                                           |
| token_per_day    | int    | 否，`token_per_second`,`token_per_minute`,`token_per_hour`,`token_per_day` 中选填一项 | -      | 允许每天请求token数                                             |

`redis`中每一项的配置字段说明。

| 配置项       | 类型   | 必填 | 默认值                                                     | 说明                                                                                         |
| ------------ | ------ | ---- | ---------------------------------------------------------- | ---------------------------                                                                  |
| service_name | string | 必填 | -                                                          | redis 服务名称，带服务类型的完整 FQDN 名称，例如 my-redis.dns、redis.my-ns.svc.cluster.local |
| service_port | int    | 否   | 服务类型为固定地址（static service）默认值为80，其他为6379 | 输入redis服务的服务端口                                                                      |
| username     | string | 否   | -                                                          | redis用户名                                                                                  |
| password     | string | 否   | -                                                          | redis密码                                                                                    |
| timeout      | int    | 否   | 1000                                                       | redis连接超时时间，单位毫秒                                                                  |
| database     | int    | 否   | 0                                                          | 使用的数据库id，例如配置为1，对应`SELECT 1`                                       |


## 配置示例

### 自定义规则组全局限流

```yaml
rule_name: routeA-global-limit-rule
global_threshold:
  token_per_minute: 1000 # 自定义规则组每分钟1000个token
redis:
  service_name: redis.static
```

### 识别请求参数 apikey，进行区别限流

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
      # 正则表达式，匹配以a开头的所有字符串，每个apikey对应的请求10qds
      - key: "regexp:^a.*"
       	token_per_second: 10
      # 正则表达式，匹配以b开头的所有字符串，每个apikey对应的请求100qd
      - key: "regexp:^b.*"
        token_per_minute: 100
      # 兜底用，匹配所有请求，每个apikey对应的请求1000qdh
      - key: "*"
        token_per_hour: 1000
redis:
  service_name: redis.static
```

### 识别请求头 x-ca-key，进行区别限流

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
      # 正则表达式，匹配以a开头的所有字符串，每个apikey对应的请求10qds
      - key: "regexp:^a.*"
        token_per_second: 10
      # 正则表达式，匹配以b开头的所有字符串，每个apikey对应的请求100qd
      - key: "regexp:^b.*"
        token_per_minute: 100
      # 兜底用，匹配所有请求，每个apikey对应的请求1000qdh
      - key: "*"
        token_per_hour: 1000
redis:
  service_name: redis.static
```

### 根据请求头 x-forwarded-for 获取对端IP，进行区别限流

```yaml
rule_name: default_rule
rule_items:
  - limit_by_per_ip: from-header-x-forwarded-for
    limit_keys:
      # 精确ip
      - key: 1.1.1.1
        token_per_day: 10
      # ip段，符合这个ip段的ip，每个ip 100qpd
      - key: 1.1.1.0/24
        token_per_day: 100
      # 兜底用，即默认每个ip 1000qpd
      - key: 0.0.0.0/0
        token_per_day: 1000
redis:
  service_name: redis.static
```

### 识别consumer，进行区别限流

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
      # 正则表达式，匹配以a开头的所有字符串，每个consumer对应的请求10qds
      - key: "regexp:^a.*"
        token_per_second: 10
      # 正则表达式，匹配以b开头的所有字符串，每个consumer对应的请求100qd
      - key: "regexp:^b.*"
        token_per_minute: 100
      # 兜底用，匹配所有请求，每个consumer对应的请求1000qdh
      - key: "*"
        token_per_hour: 1000
redis:
  service_name: redis.static
```

### 识别cookie中的键值对，进行区别限流

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
      # 正则表达式，匹配以a开头的所有字符串，每个cookie中的value对应的请求10qds
      - key: "regexp:^a.*"
        token_per_second: 10
      # 正则表达式，匹配以b开头的所有字符串，每个cookie中的value对应的请求100qd
      - key: "regexp:^b.*"
        token_per_minute: 100
      # 兜底用，匹配所有请求，每个cookie中的value对应的请求1000qdh
      - key: "*"
        token_per_hour: 1000
rejected_code: 200
rejected_msg: '{"code":-1,"msg":"Too many requests"}'
redis:
  service_name: redis.static
```

## 完整示例

AI Token 限流插件依赖 Redis 记录剩余可用的 token 数，因此首先需要部署 Redis 服务。

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

在本例中，使用通义千问作为 AI 服务提供商。另外还需要设置 AI 统计插件，因为 AI Token 限流插件依赖 AI 统计插件计算每次请求消耗的 token 数，以下配置限制每分钟的 input 和 output token 总数为 200 个。

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
      # 默认情况下，为了减轻数据面的压力，Higress 的 global.onlyPushRouteCluster 配置参数被设置为 true，意味着不会自动发现 Kubernetes Service
      # 如果需要使用 Kubernetes Service 作为服务发现，可以将 global.onlyPushRouteCluster 参数设置为 false，
      # 这样就可以直接将 service_name 设置为 Kubernetes Service, 而无须为 Redis 创建 McpBridge 以及 Ingress 路由
      # service_name: redis.default.svc.cluster.local
      service_name: redis.dns
      service_port: 6379
  url: oci://higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/ai-token-ratelimit:1.0.0
  phase: UNSPECIFIED_PHASE
  priority: 600
```

注意，AI Token 限流插件中的 Redis 配置项 `service_name` 来自 McpBridge 中配置的服务来源，另外我们还需要在 McpBridge 中配置通义千问服务的访问地址。

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

分别创建两条路由规则。

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

转发 higress-gateway 的流量到本地，方便进行测试。

```bash
kubectl port-forward svc/higress-gateway -n higress-system 18000:80
```

触发限流效果如下：

```bash
curl "http://localhost:18000/v1/chat/completions?apikey=123456" \
-H "Host: qwen-test.com" \
-H "Content-Type: application/json"  \
-d '{
  "model": "gpt-3",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "stream": false
}'
{"id":"88cfa80f-545d-93b4-8ff3-3f5245ca33ba","choices":[{"index":0,"message":{"role":"assistant","content":"我是通义千问，由阿里云开发的AI助手。我可以回答各种问题、提供信息和与用户进行对话。有什么我可以帮助你的吗？"},"finish_reason":"stop"}],"created":1719909825,"model":"qwen-turbo","object":"chat.completion","usage":{"prompt_tokens":13,"completion_tokens":33,"total_tokens":46}}
curl "http://qwen-test.com:18000/v1/chat/completions?apikey=123456" -H "Content-Type: application/json"  -d '{
  "model": "gpt-3",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "stream": false
}'
Too many requests  # 限流成功
```
