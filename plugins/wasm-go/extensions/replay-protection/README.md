---
title: 防重放攻击
keywords: [higress,replay-protection]
description: 防重放攻击插件配置参考
---

## 功能说明

防重放插件通过验证请求中的一次性随机数来防止请求重放攻击。每个请求都需要携带一个唯一的 nonce 值，服务器会记录并校验这个值的唯一性，从而防止请求被恶意重放

具体包含一下功能：

- **强制或可选的 nonce 校验**：可根据配置决定是否强制要求请求携带 nonce 值。
- **基于 Redis 的 nonce 唯一性验证**：通过 Redis 存储和校验 nonce 值，确保其唯一性。
- **可配置的 nonce 有效期**：支持设置 nonce 的有效期，过期后自动失效。
- **nonce 格式和长度校验**：支持对 nonce 值的格式（Base64）和长度进行验证。
- **自定义错误响应**：支持配置拒绝请求时的状态码和错误信息。
- **可自定义 nonce 请求头**：可以自定义携带 nonce 的请求头名称。

## 运行属性

插件执行阶段：`认证阶段`
插件执行优先级：`800`

## 配置字段

| 名称                | 数据类型 | 必填 | 默认值          | 描述                              |
|----------------------|--------|------|-----------------|---------------------------------|
| `force_nonce`        | bool   | 否   | true      | 是否强制要求请求携带 nonce 值       |
| `nonce_header`       | string | 否   | `X-Higress-Nonce`   | 指定携带 nonce 值的请求头名称       |
| `nonce_ttl`          | int    | 否   | 900        | nonce 的有效期，单位秒    |
| `nonce_min_length`   | int    | 否   | 8            | nonce 值的最小长度               |
| `nonce_max_length`   | int    | 否   | 128        | nonce 值的最大长度               |
| `reject_code`        | int    | 否   | 429        | 拒绝请求时返回的状态码             |
| `reject_msg`         | string | 否   | `Replay Attack Detected` | 拒绝请求时返回的错误信息           |
| `validate_base64`    | bool    | 否   | false | 是否校验 nonce 的 base64 编码格式 |
| `failure_mode` | string | 否 | `fail_closed` | Redis 调用失败、超时或响应异常时的策略：`fail_closed` 返回固定 503；`fail_open` 放行请求。可用性优先且接受暂时跳过 nonce 去重时可选择 `fail_open`。 |
| `redis` | Object | 是   | -              | redis 相关配置 |

`redis` 中每一项的配置字段说明

| 名称           | 数据类型 | 必填 | 默认值                 | 描述|
| -------------- | -------- | ---- |---------------------| --------------------------------------- |
| `service_name` | string   | 是   | -                   | redis 服务名称，带服务类型的完整 FQDN 名称，例如 my-redis.dns、redis.my-ns.svc.cluster.local |
| `service_port` | int      | 否   | 6379                | redis 服务端口|
| `username`     | string | 否   | -                   | redis 用户名|
| `password`     | string | 否   | -                   | redis 密码|
| `timeout`      | int      | 否   | 1000                | redis 连接超时时间，单位毫秒 |
| `database`     | int    | 否   | 0                   | 使用的数据库id，例如配置为1，对应`SELECT 1`|
| `key_prefix`   | string   | 否   | `replay-protection` | redis 键前缀，用于区分不同的 nonce 键 |

## 配置示例

以下是一个防重放攻击插件的完整配置示例：

```yaml
force_nonce: true
nonce_header: "X-Higress-Nonce"    # 指定 nonce 请求头名称
nonce_ttl: 900                    # nonce 有效期，设置为 900 秒
nonce_min_length: 8               # nonce 的最小长度
nonce_max_length: 128             # nonce 的最大长度
validate_base64: true             # 是否开启 base64 格式校验
reject_code: 429                  # 当拒绝请求时返回的 HTTP 状态码
reject_msg: "Replay Attack Detected"  # 拒绝请求时返回的错误信息内容
redis:
  service_name: redis.static       # Redis 服务的名称
  service_port: 80                # Redis 服务所使用的端口
  timeout: 1000                   # Redis 操作的超时时间（单位：毫秒）
  key_prefix: "replay-protection" # Redis 中键的前缀
```

## 使用说明

### 请求头要求

| 请求头名称       | 是否必须         | 说明                                       |
|-----------------|----------------|------------------------------------------|
| `X-Higress-Nonce`  | 根据 `force_nonce` 配置决定 | 请求中携带的随机生成的 nonce 值，需符合 Base64 格式。 |

> **注意**：可以通过 `nonce_header` 配置自定义请求头名称，默认值为 `X-Higress-Nonce`。

### 使用示例

```bash
# Generate nonce
nonce=$(openssl rand -base64 32)

# Send request
curl -X POST 'https://api.example.com/path' \
  -H "X-Higress-Nonce: $nonce" \
  -d '{"key": "value"}'
```

## 返回结果

```json
{
  "code": 429,
  "message": "Replay Attack Detected"
}
```

## 错误响应示例

| 错误场景                 | 状态码 | 错误信息               |
|------------------------|-------|--------------------|
| 缺少 nonce 请求头         | 400 | `Missing Required Header` |
| nonce 长度不符合要求      | 400 | `Invalid Nonce` |
| nonce 格式不符合 Base64 | 400 | `Invalid Nonce` |
| nonce 已被使用（重放攻击） | 429 | `Replay Attack Detected` |

