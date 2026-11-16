---
title: 集群并发限制
keywords: [ AI网关, 并发限制, 限流 ]
description: 集群并发限制插件配置参考
---

## 功能说明

`cluster-concurrency-limit` 插件基于 Redis 实现**集群级(跨网关实例共享)的并发数(in-flight 请求数)限制**。
与现有基于时间窗口计数(QPS/QPM/token 速率)的限流插件不同,本插件是 Gauge 语义:
一个请求从进入到响应结束(含流式响应全程)持续占用一个并发槽位,请求结束立即释放。
对 LLM 长流式请求场景,并发模型比 QPS 更贴合容量直觉。

支持**租户 / 用户 / apikey(任意凭证)三级层级限额 + global 兜底**:

- **租户级**(`limit_by_consumer_group`):读 `X-Mse-Consumer-Group` 头(由前置插件注入)
- **用户级**(`limit_by_consumer`):读认证插件写入的 `X-Mse-Consumer` 头,同一 consumer 的所有凭证共享额度
- **apikey/任意凭证级**:`limit_by_header` / `limit_by_param` / `limit_by_cookie`
- **global 级**(`global_threshold`):规则组总并发兜底

多维度 **all-match 叠加**:一次请求在所有命中维度同时占一个槽位,任一维度超限即拒绝(默认 429,不排队)。
层级语义为**共享池 + 逐级封顶(cap, not reservation)**:每层配置的是上限而非独占预留,
子级实际可用 = min(自身剩余, 父级池剩余),先到先得。

**分级语义(配了才限)**:每级独立判定——某级 `limit_keys` 未命中(含身份头缺失)→ 该级跳过,
不限、不占槽、不拒绝;已配置的祖先级照常生效。例:只配租户级,其下所有用户/apikey 共享租户池;
租户没配但用户配了,用户级限额仍生效。访问准入由认证插件负责,本插件只管配额。
只配 `global_threshold` 时不做任何身份校验,所有请求共用总并发池。

## 运行属性

插件执行阶段:`默认阶段`
插件执行优先级:`610`(先于 ai-token-ratelimit 的 600 执行——先占并发槽、后扣 token)

> priority 通过 WasmPlugin CR / higress-console 插件元数据下发,不在插件代码中设置。

## 配置说明

| 配置项 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| rule_name | string | 是 | - | 规则名,参与 Redis key 拼装;key 含 `{rule_name}` hash tag,保证一次请求的所有维度 key 落在 Redis Cluster 同一 slot |
| global_threshold | object | 否 | - | 规则组总并发:`concurrency: int`;与 rule_items **可并存**(至少一项),global 作兜底维度参与 all-match |
| rule_items | array | 否(最多 10 条) | - | 维度规则项,all-match 叠加(命中所有规则,而非第一个) |
| rejected_code | int | 否 | 429 | 超限状态码 |
| rejected_msg | string | 否 | concurrency limit exceeded | 超限响应体 |
| show_limit_quota_header | bool | 否 | false | 返回 `X-Concurrency-Limit` / `X-Concurrency-Remaining` 响应头(取所有命中维度的最小剩余,"报最紧的桶") |
| lease_ttl_seconds | int | 否 | 600 | member 租约时长(秒),仅用于防御进程崩溃导致的槽位泄漏;建议大于业务典型流时长 |
| redis | object | 是 | - | redis 相关配置(同 cluster-key-rate-limit / ai-token-ratelimit) |

`global_threshold` 中每一项的配置字段说明。

| 配置项 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| concurrency | int | 是 | - | 规则组总并发上限,正整数 |

`rule_items` 中每一项的配置字段说明。

| 配置项 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| limit_by_consumer_group | string | 否,`limit_by_*` 中选填一项 | - | **租户维度**,读 `X-Mse-Consumer-Group` 头(前置插件注入),无需值 |
| limit_by_consumer | string | 否,`limit_by_*` 中选填一项 | - | 用户维度,读 `X-Mse-Consumer` 头,无需值 |
| limit_by_header | string | 否,`limit_by_*` 中选填一项 | - | header 维度,值为 header 名 |
| limit_by_param | string | 否,`limit_by_*` 中选填一项 | - | URL 参数维度 |
| limit_by_cookie | string | 否,`limit_by_*` 中选填一项 | - | Cookie 维度 |
| limit_by_per_header / limit_by_per_param / limit_by_per_consumer / limit_by_per_cookie | string | 否,`limit_by_*` 中选填一项 | - | 按值分别计数,limit_keys 仅接受 `*` / `regexp:...` |
| limit_by_per_ip | string | 否,`limit_by_*` 中选填一项 | - | `from-remote-addr` / `from-header-<名>`,CIDR 写 limit_keys |
| limit_keys | array of object | 是 | - | 键值与并发上限列表 |

`limit_keys` 中每一项的配置字段说明。

| 配置项 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| key | string | 是 | - | 非 per_ 类型支持精确字面值 / `*` / `regexp:...`;per_ 类型仅接受 `*` / `regexp:...`;`limit_by_per_ip` 支持 IP 地址或 IP 段(CIDR) |
| concurrency | int | 是 | - | 该 key 的并发上限(Gauge),正整数 |

`redis` 中每一项的配置字段说明。

| 配置项 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| service_name | string | 是 | - | redis 服务名称,带服务类型的完整 FQDN 名称,例如 `my-redis.dns`、`redis.static` |
| service_port | int | 否 | 静态服务(static)为 80,其他为 6379 | redis 服务端口 |
| username | string | 否 | - | redis 用户名 |
| password | string | 否 | - | redis 密码 |
| timeout | int | 否 | 1000 | redis 连接超时时间,单位毫秒 |
| database | int | 否 | 0 | redis 数据库 id |

## 配置示例

```yaml
rule_name: ai-route-concurrency
global_threshold:
  concurrency: 200                    # 规则组总并发兜底
rule_items:
  # 租户级:X-Mse-Consumer-Group(前置插件注入)
  - limit_by_consumer_group: ""
    limit_keys:
      - key: "tenant-a"
        concurrency: 10
      - key: "*"
        concurrency: 5
  # 用户级:X-Mse-Consumer,同一 consumer 所有 key 共享额度
  - limit_by_consumer: ""
    limit_keys:
      - key: "alice"
        concurrency: 7                # 子级 cap < 父级池,防饿死
      - key: "regexp:.*"
        concurrency: 3
  # apikey 级:key 在任何头/参数/cookie 里都能限
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

### 层级配置策略:封顶不预留,防饿死靠 cap 梯度

- 每层是上限不是预留,子级可用 = min(自身剩余, 父级池剩余),先到先得;
- **子级 cap = 父级池时防不住饿死**(例:租户池 10、用户各 10,用户 a 占满 10 后用户 b 为 0);
- 需缓解时,标准做法是**子级 cap < 父级池**(如用户各 7、租户 10,单用户最多给他人留 3 个)。

## 计数模型说明(ZSET 租约成员模型)

每个限流维度对应一个 ZSET key(`{rule_name}:concurrency:<dim>:<value>`),每个在途请求是其中一个
member(取 `x-request-id`,缺失则插件生成 UUID),score 为租约过期时间戳:

- **占槽**:一条 Lua 脚本原子完成"惰性清理过期 member → 逐维度检查 → 全部通过后 ZADD 占槽",
  先检后占、无需回滚;
- **释放**:HTTP stream 关闭(正常结束/上游 reset/客户端断开)时 ZREM 自己的 member,天然幂等;
- **崩溃兜底**:进程崩溃导致 member 未释放时,score 到期后被惰性清理,无永久泄漏;
- **流存活超过租约**:member 提前过期 → 槽位被提前释放 → 暂时性轻微超额放行,随流结束自愈。
  `lease_ttl_seconds` 建议大于业务典型流时长。

## 部署与排障

### 部署清单

1. WasmPlugin 启用并配置 priority **610**;
2. Redis 服务(McpBridge 注册,`service_name` 与配置一致);hash tag 兼容 Redis Cluster;
3. 若启用租户维度:部署注入 `X-Mse-Consumer-Group` 的前置插件;
4. 认证插件(key-auth/jwt-auth/basic-auth)按需配置,保证 `X-Mse-Consumer` 写入。

### ⚠️ 前置顺序约束

> **注入 `X-Mse-Consumer-Group` 的插件必须先于本插件执行**(priority > 610 或更早阶段)。
> 否则租户维度读到空头,该级跳过不计数,**租户限额静默失效,无任何报错**。

### 无身份头行为声明

读不到 `X-Mse-Consumer` / `X-Mse-Consumer-Group`(或其他维度取值)的请求,对应维度**跳过、放行且不计数**;
只有 global 兜底(若配置)能拦截。防未认证流量请在路由上配置认证插件拒绝无凭证请求。

### Redis 故障

**fail-open 放行**,记 error 日志 + metric(`concurrency_limit_redis_error`)。

### Metrics

- `concurrency_limit_rejected` — 拒绝数(可接告警)
- `concurrency_limit_redis_error` — fail-open 计数

### 排障指引

| 现象 | 排查 |
| --- | --- |
| 租户维度不生效 | 检查前置注入插件 priority 是否 > 610;`X-Mse-Consumer-Group` 头是否到达本插件 |
| 崩溃残留 | member 随 score 过期自动清理;`redis-cli ZRANGEBYSCORE key -inf +inf WITHSCORES` 看 member 过期时间 |
| 实时在途请求 | `redis-cli ZRANGE "{rule}:concurrency:..." 0 -1` |
