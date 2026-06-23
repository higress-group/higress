---
title: AI 配额管理
keywords: [ AI网关, AI配额 ]
description: AI 配额管理插件配置参考
---

## 功能说明

`ai-quota` 插件实现给特定 consumer 根据分配固定的 quota 进行 quota 策略限流，同时支持 quota 管理能力，包括查询 quota 、刷新 quota、增减 quota。

`ai-quota` 插件需要配合 认证插件比如 `key-auth`、`jwt-auth` 等插件获取认证身份的 consumer 名称，同时需要配合 `ai-statistics` 插件获取 AI Token 统计信息。

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`750`

## 配置说明

| 名称                 | 数据类型            | 填写要求                                 | 默认值 | 描述                                         |
|--------------------|-----------------|--------------------------------------| ---- |--------------------------------------------|
| `redis_key_prefix` | string          |  选填                                     |   chat_quota:   | qutoa redis key 前缀                         |
| `admin_consumer`   | string          | 必填                                   |      | 管理 quota 管理身份的 consumer 名称                 |
| `admin_path`       | string          | 选填                                   |   /quota   | 管理 quota 请求 path 前缀                        |
| `enable_path_suffixes` | []string     | 选填                                   |  ["/v1/chat/completions", "/v1/messages"] | 启用配额校验的请求路径后缀（仅用于 completion 请求，不影响管理接口路径） |
| `redis`            | object          | 是                                    |      | redis相关配置                                  |

`redis`中每一项的配置字段说明

| 配置项       | 类型   | 必填 | 默认值                                                     | 说明                                                                                         |
| ------------ | ------ | ---- | ---------------------------------------------------------- | ---------------------------                                                                  |
| service_name | string | 必填 | -                                                          | redis 服务名称，带服务类型的完整 FQDN 名称，例如 my-redis.dns、redis.my-ns.svc.cluster.local |
| service_port | int    | 否   | 服务类型为固定地址（static service）默认值为80，其他为6379 | 输入redis服务的服务端口                                                                      |
| username     | string | 否   | -                                                          | redis用户名                                                                                  |
| password     | string | 否   | -                                                          | redis密码                                                                                    |
| timeout      | int    | 否   | 1000                                                       | redis连接超时时间，单位毫秒                                                                  |
| database     | int    | 否   | 0                                                          | 使用的数据库id，例如配置为1，对应`SELECT 1`                                                  |


## 配置示例

### 识别请求参数 apikey，进行区别限流
```yaml
redis_key_prefix: "chat_quota:"
admin_consumer: consumer3
admin_path: /quota
redis:
  service_name: redis-service.default.svc.cluster.local
  service_port: 6379
  timeout: 2000
```


### Group 共享池

ai-quota 支持按 consumer group 共享配额池：组内任一 consumer 消耗都扣减共享池，consumer 私有池仍按人扣减。两个池都 > 0 才放行（严格模式）。

Group 由 key-auth 在认证时通过 `X-Mse-Consumer-Group` header 注入。ai-quota 兼容 group 缺失场景——按老逻辑仅扣 consumer 私有池。

###  刷新 quota

如果当前请求 url 的后缀符合 admin_path，例如插件在 example.com/v1/chat/completions 这个路由上生效，那么更新 quota 可以通过
curl https://example.com/v1/chat/completions/quota/refresh -H "Authorization: Bearer credential3" -d "consumer=consumer1&quota=10000"

Redis 中 key 为 chat_quota:consumer1 的值就会被刷新为 10000

刷新 group 共享池：
```bash
curl https://example.com/v1/chat/completions/quota/refresh -H "Authorization: Bearer credential3" -d "group=team-a&quota=10000"
```
Redis 中 key 为 `chat_quota:team-a` 的值会被刷新为 10000。`consumer` 与 `group` 必须恰好设置一个；同时设置或都不设置返回 `400 ai-quota.invalid_param`。

### 查询 quota

查询特定用户的 quota 可以通过 curl https://example.com/v1/chat/completions/quota?consumer=consumer1 -H "Authorization: Bearer credential3"
将返回： {"quota": 10000, "name": "consumer1"}

查询 group 共享池：
```bash
curl https://example.com/v1/chat/completions/quota?group=team-a -H "Authorization: Bearer credential3"
```
返回：`{"name":"team-a","quota":10000}`

### 增减 quota

增减特定用户的 quota 可以通过 curl https://example.com/v1/chat/completions/quota/delta -d "consumer=consumer1&value=100" -H "Authorization: Bearer credential3"
这样 Redis 中 Key 为 chat_quota:consumer1 的值就会增加100，可以支持负数，则减去对应值。

增减 group 共享池：
```bash
curl https://example.com/v1/chat/completions/quota/delta -d "group=team-a&value=500" -H "Authorization: Bearer credential3"
```
Redis 中 key `chat_quota:team-a` 增减 500。

## 相关错误码

| HTTP | 错误码 | 触发场景 |
|------|--------|---------|
| 200 | `ai-quota.refreshquota` | admin `/refresh` 成功 |
| 200 | `ai-quota.queryquota` | admin `/quota` 查询成功 |
| 200 | `ai-quota.deltaquota` | admin `/delta` 成功 |
| 200 | - | 普通 chat completion 通过 |
| 429 | `ai-quota.group_exhausted` | group 共享池 ≤ 0（仅 group != "" 路径） |
| 429 | `ai-quota.consumer_exhausted` | consumer 私有池 ≤ 0 |
| 429 | `ai-quota.both_exhausted` | 两池都 ≤ 0（仅 group != "" 路径） |
| 400 | `ai-quota.invalid_param` | admin API 入参错误（consumer/group 互斥违反，或 quota/value 非整数） |
| 503 | `ai-quota.error` | Redis 调用失败 |
| 401 | `ai-quota.no_key` | 缺少 `X-Mse-Consumer` header |
| 403 | `ai-quota.unauthorized` | consumer 未配置或非 admin consumer |

> **破坏性变更**：老版本（≤ v1.0.x）chat completion 路径的配额耗尽返回 `403 ai-quota.noquota`，新版本统一为 `429 ai-quota.consumer_exhausted`（spec §5.5、§8.1）。依赖 403/noquota 字符串的 client 需要同步更新。另外 admin `queryQuota` 返回的 JSON 字段 `"consumer"` 在新版本中更名为 `"name"`，以同时承载 consumer 与 group 名称（spec §5.4.2、§8.1）。
