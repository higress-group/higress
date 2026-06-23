---
title: 基于 Key 集群限流
keywords: [higress, rate-limit]
description: 基于 Key 集群限流插件配置参考
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

`cluster-key-rate-limit` 插件基于 Redis 实现**集群级限流**，适用于需要跨多个 Higress Gateway 实例进行**全局一致速率限制**的场景。

支持三种限流模式：

- **规则级全局限流**：基于相同的 `rule_name` 和 `global_threshold` 配置，对自定义规则组设置全局限流阈值
- **Key 级动态限流**：根据请求中的动态 Key（如 URL 参数、请求头、客户端 IP、Consumer 名称或 Cookie 字段）进行分组限流
- **混合限流**：同时配置 `global_threshold`（全局兜底）和 `rule_items`（按维度细分），所有命中的规则**叠加生效**，任一触发即拒绝。

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`20`

## 配置说明

| 配置项                  | 类型   | 必填 | 默认值 | 说明                                                                          |
| ----------------------- | ------ | ---- | ------ |-----------------------------------------------------------------------------|
| rule_name               | string | 是 | - | 限流规则名称，根据限流规则名称 + 限流类型 + 限流 key 名称 + 限流 key 对应的实际值来拼装 redis key             |
| global_threshold | Object | 否，至少一项；可同时配置 | - | 对整个自定义规则组进行限流 |
| rule_items | array of object | 否，至少一项；可同时配置 | -                 | 限流规则项，最多支持 **10 条**。所有满足匹配条件的 `rule_item` 都会参与限流，规则之间是"或"关系，任一触发即拒绝；规则的执行顺序不影响最终结果。详见下方"配置说明"展开。 |
| show_limit_quota_header | bool | 否 | false | 响应头中是否显示 `X-RateLimit-Limit`（限制的总请求数）和 `X-RateLimit-Remaining`（剩余还可以发送的请求数） |
| rejected_code           | int | 否 | 429 | 请求被限流时，返回的 HTTP 状态码                                                         |
| rejected_msg            | string | 否 | Too many requests | 请求被限流时，返回的响应体                                                               |
| redis                   | object          | 是                                                           | -                 | redis 相关配置                                                                  |

`global_threshold` 中每一项的配置字段说明。

| 配置项           | 类型 | 必填                                                         | 默认值 | 说明               |
| ---------------- | ---- | ------------------------------------------------------------ | ------ | ------------------ |
| query_per_second | int  | 否，`query_per_second`,`query_per_minute`,`query_per_hour`,`query_per_day` 中选填一项 | -      | 允许每秒请求次数   |
| query_per_minute | int  | 否，`query_per_second`,`query_per_minute`,`query_per_hour`,`query_per_day` 中选填一项 | -      | 允许每分钟请求次数 |
| query_per_hour   | int  | 否，`query_per_second`,`query_per_minute`,`query_per_hour`,`query_per_day` 中选填一项 | -      | 允许每小时请求次数 |
| query_per_day    | int  | 否，`query_per_second`,`query_per_minute`,`query_per_hour`,`query_per_day` 中选填一项 | -      | 允许每天请求次数   |

`rule_items` 中每一项的配置字段说明。

| 配置项                | 类型            | 必填                   | 默认值 | 说明                                                                                                                                                       |
| --------------------- | --------------- |----------------------| ------ |----------------------------------------------------------------------------------------------------------------------------------------------------------|
| limit_by_header       | string          | 否，`limit_by_*` 中选填一项 | -      | 配置获取限流键值的来源 HTTP 请求头名称                                                                                                                                   |
| limit_by_param        | string          | 否，`limit_by_*` 中选填一项 | -      | 配置获取限流键值的来源 URL 参数名称                                                                                                                                     |
| limit_by_consumer     | string          | 否，`limit_by_*` 中选填一项 | -      | 根据 consumer 名称进行限流，无需添加实际值                                                                                                                               |
| limit_by_cookie       | string          | 否，`limit_by_*` 中选填一项 | -      | 配置获取限流键值的来源 Cookie中 key 名称                                                                                                                               |
| limit_by_per_header   | string          | 否，`limit_by_*` 中选填一项 | -      | 按规则匹配特定 HTTP 请求头，并对每个请求头分别计算限流，配置获取限流键值的来源 HTTP 请求头名称，配置 `limit_keys` 时支持正则表达式或 `*`                                                                      |
| limit_by_per_param    | string          | 否，`limit_by_*` 中选填一项 | -      | 按规则匹配特定 URL 参数，并对每个参数分别计算限流，配置获取限流键值的来源 URL 参数名称，配置 `limit_keys` 时支持正则表达式或 `*`                                                                           |
| limit_by_per_consumer | string          | 否，`limit_by_*` 中选填一项 | -      | 按规则匹配特定 consumer，并对每个 consumer 分别计算限流，根据 consumer 名称进行限流，无需添加实际值，配置 `limit_keys` 时支持正则表达式或 `*`                                                           |
| limit_by_per_cookie   | string          | 否，`limit_by_*` 中选填一项 | -      | 按规则匹配特定 Cookie，并对每个 Cookie 分别计算限流，配置获取限流键值的来源 Cookie中 key 名称，配置 `limit_keys` 时支持正则表达式或 `*`                                                               |
| limit_by_per_ip       | string          | 否，`limit_by_*` 中选填一项 | -      | 按规则匹配特定 IP，并对每个 IP 分别计算限流，配置获取限流键值的来源 IP 参数名称，从请求头获取，以 `from-header-对应的header名`，示例：`from-header-x-forwarded-for`，直接获取对端 socket ip，配置为 `from-remote-addr` |
| limit_keys            | array of object | 是                    | -      | 配置匹配键值后的限流次数                                                                                                                                             |

#### `rule_items` 多规则匹配语义

`rule_items` 是一个数组，**所有满足匹配条件**的 `rule_item` 都会被评估，规则之间是"或"关系，**任一触发即拒绝**。规则的执行顺序不影响最终结果。

> `rule_items` 数组最多支持 **10 条**规则。每条 `rule_item` 会按命中的 `limit_keys` 产生独立的 Redis 计数器。

##### 同维度多值匹配（合并到一条 rule_item）

同一 `limit_by_*` + key 下的多个匹配值（如多个 apikey）应收纳到**一条** `rule_item` 的 `limit_keys` 数组中，不要拆成多条 `rule_item`：

❌ 错误（3 条 rule_item，浪费配额）：
```yaml
rule_items:
  - { limit_by_param: apikey, limit_keys: [{key: k1, query_per_minute: 100}] }
  - { limit_by_param: apikey, limit_keys: [{key: k2, query_per_minute: 200}] }
  - { limit_by_param: apikey, limit_keys: [{key: k3, query_per_minute: 300}] }
```

✅ 正确（1 条 rule_item）：
```yaml
rule_items:
  - limit_by_param: apikey
    limit_keys:
      - { key: k1, query_per_minute: 100 }
      - { key: k2, query_per_minute: 200 }
      - { key: k3, query_per_minute: 300 }
```

##### 多时间窗口限流（合法的"重复"）

同一 `limit_by_*` + key 但**不同时间窗**的两条 `rule_items` 是合法配置，会产生两个独立 Redis 计数器，实现"每分钟 + 每小时"双层限流（解析时会输出信息性 warn，可忽略）：

```yaml
rule_items:
  - limit_by_param: apikey
    limit_keys: [{key: k1, query_per_minute: 100}]
  - limit_by_param: apikey
    limit_keys: [{key: k1, query_per_hour: 1000}]   # 同 type+key，不同时间窗
```

##### 完全相同的重复（应避免）

同 type+key+时间窗的完全重复会产生同一 Redis key 的多次评估，属冗余配置，应合并为一条。

##### 多规则场景下的 X-RateLimit-* 头

当多条规则同时未触发、配置 `show_limit_quota_header: true` 时：
- `X-RateLimit-Limit` / `X-RateLimit-Remaining`：取剩余比例最小（最紧约束）的命中规则
- `X-RateLimit-Reset`（触发限流时返回）：取第一条触发的规则（按 `rule_items` 数组顺序，全局优先）

`limit_keys` 中每一项的配置字段说明。

| 配置项           | 类型   | 必填                                                         | 默认值 | 说明                                                         |
| ---------------- | ------ | ------------------------------------------------------------ | ------ | ------------------------------------------------------------ |
| key              | string | 是                                                           | -      | 匹配的键值，`limit_by_per_header`,`limit_by_per_param`,`limit_by_per_consumer`,`limit_by_per_cookie` 类型支持配置正则表达式（以regexp:开头后面跟正则表达式）或者*（代表所有），正则表达式示例：`regexp:^d.*`（以d开头的所有字符串）；`limit_by_per_ip`支持配置 IP 地址或 IP 段 |
| query_per_second | int    | 否，`query_per_second`,`query_per_minute`,`query_per_hour`,`query_per_day` 中选填一项 | -      | 允许每秒请求次数                                             |
| query_per_minute | int    | 否，`query_per_second`,`query_per_minute`,`query_per_hour`,`query_per_day` 中选填一项 | -      | 允许每分钟请求次数                                           |
| query_per_hour   | int    | 否，`query_per_second`,`query_per_minute`,`query_per_hour`,`query_per_day` 中选填一项 | -      | 允许每小时请求次数                                           |
| query_per_day    | int    | 否，`query_per_second`,`query_per_minute`,`query_per_hour`,`query_per_day` 中选填一项 | -      | 允许每天请求次数                                             |

`redis` 中每一项的配置字段说明。

| 配置项       | 类型   | 必填 | 默认值                                                     | 说明                                                                                         |
| ------------ | ------ | ---- | ---------------------------------------------------------- | ---------------------------------------------------------------------------                  |
| service_name | string | 必填 | -                                                          | redis 服务名称，带服务类型的完整 FQDN 名称，例如 my-redis.dns、redis.my-ns.svc.cluster.local |
| service_port | int    | 否   | 服务类型为固定地址（static service）默认值为80，其他为6379 | 输入redis服务的服务端口                                                                      |
| username     | string | 否   | -                                                          | redis 用户名                                                                                 |
| password     | string | 否   | -                                                          | redis 密码                                                                                   |
| timeout      | int    | 否   | 1000                                                       | redis 连接超时时间，单位毫秒                                                                 |
| database     | int    | 否   | 0                                                          | 使用的数据库id，例如配置为1，对应`SELECT 1`                                                  |

## 配置示例

### 自定义规则组全局限流

```yaml
rule_name: routeA-global-limit-rule
global_threshold:
  query_per_minute: 1000 # 自定义规则组每分钟最多1000次请求
redis:
  service_name: redis.static
show_limit_quota_header: true
```

### 识别请求参数 apikey，进行区别限流

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
      # 正则表达式，匹配以 a 开头的所有字符串，每个 apikey 对应的请求 10qds
      - key: "regexp:^a.*"
        query_per_second: 10
      # 正则表达式，匹配以 b 开头的所有字符串，每个 apikey 对应的请求 100qd
      - key: "regexp:^b.*"
        query_per_minute: 100
      # 兜底用，匹配所有请求，每个 apikey 对应的请求 1000qdh
      - key: "*"
        query_per_hour: 1000
redis:
  service_name: redis.static
show_limit_quota_header: true
```

### 识别请求头 x-ca-key，进行区别限流

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
      # 正则表达式，匹配以 a 开头的所有字符串，每个 apikey 对应的请求 10qds
      - key: "regexp:^a.*"
        query_per_second: 10
      # 正则表达式，匹配以b开头的所有字符串，每个 apikey 对应的请求 100qd
      - key: "regexp:^b.*"
        query_per_minute: 100
      # 兜底用，匹配所有请求，每个 apikey 对应的请求 1000qdh
      - key: "*"
        query_per_hour: 1000
redis:
  service_name: redis.static
show_limit_quota_header: true
```

### 根据请求头 x-forwarded-for 获取对端 IP，进行区别限流

```yaml
rule_name: routeA-client-ip-limit-rule
rule_items:
  - limit_by_per_ip: from-header-x-forwarded-for
    limit_keys:
      # 精确 IP
      - key: 1.1.1.1
        query_per_day: 10
      # IP 段，符合这个 IP 段的 IP，每个 IP 100qpd
      - key: 1.1.1.0/24
        query_per_day: 100
      # 兜底用，即默认每个 IP 1000 qpd
      - key: 0.0.0.0/0
        query_per_day: 1000
redis:
  service_name: redis.static
show_limit_quota_header: true
```

### 识别 consumer，进行区别限流

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
      # 正则表达式，匹配以 a 开头的所有字符串，每个 consumer 对应的请求 10qds
      - key: "regexp:^a.*"
        query_per_second: 10
      # 正则表达式，匹配以 b 开头的所有字符串，每个 consumer 对应的请求 100qd
      - key: "regexp:^b.*"
        query_per_minute: 100
      # 兜底用，匹配所有请求，每个 consumer 对应的请求 1000qdh
      - key: "*"
        query_per_hour: 1000
redis:
  service_name: redis.static
show_limit_quota_header: true 
```

### 识别 Cookie 中的键值对，进行区别限流

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
      # 正则表达式，匹配以 a 开头的所有字符串，每个 cookie 中的 value 对应的请求 10qds
      - key: "regexp:^a.*"
        query_per_second: 10
      # 正则表达式，匹配以 b 开头的所有字符串，每个 cookie 中的 value 对应的请求 100qd
      - key: "regexp:^b.*"
        query_per_minute: 100
      # 兜底用，匹配所有请求，每个 cookie 中的 value 对应的请求 1000qdh
      - key: "*"
        query_per_hour: 1000
rejected_code: 200
rejected_msg: '{"code":-1,"msg":"Too many requests"}'
redis:
  service_name: redis.static
show_limit_quota_header: true
```
