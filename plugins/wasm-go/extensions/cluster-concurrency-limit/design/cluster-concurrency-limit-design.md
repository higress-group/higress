# cluster-concurrency-limit 插件设计（定稿）

> 状态：**定稿（2026-09-30）**，开发的唯一权威依据
> 本文档整合并替代：
> - [2026-09-23 设计稿](./2026-09-23-cluster-concurrency-limit-design.md)（原始方案）
> - [2026-09-30 v2 修订稿](./2026-09-30-cluster-concurrency-limit-design-v2.md)（评审修订）
>
> 决策过程见 [2026-09-29 评审文档](./2026-09-29-cluster-concurrency-limit-design-review.md)；
> 上述三份仅作历史参考，开发以本文档为准。
>
> 范围：Higress 仓库（`plugins/wasm-go/extensions/`）新增 wasm 插件；
> higress-console 联动为二期，不在本设计内。

## 1. 背景与目标

Higress 现有限流插件全部是**时间窗口计数模型**（QPS/QPM/token 速率），
没有 "in-flight 请求数"（Gauge）语义的并发限制。数据面唯一的并发控制点是
Envoy 原生断路器，但其计数器是 cluster 粒度，无法按租户 / consumer / apikey
区分额度。（已经 Higress 2.2.2 及最新版 b7fb876 源码核实，官方无现成替代插件，
自研必要。）

本设计新增插件 **cluster-concurrency-limit**，基于 Redis 实现集群级
（跨网关实例共享）的并发数限制，支持**租户 / 用户 / apikey 三级层级限额**：

- **租户级**（`limit_by_consumer_group`）：读 `X-Mse-Consumer-Group` 头（由前置插件注入）
- **用户级**（`limit_by_consumer`）：读认证插件写入的 `X-Mse-Consumer` 头，同一 consumer 的所有凭证共享额度
- **apikey/任意凭证级**：`limit_by_header` / `limit_by_param` / `limit_by_cookie`
- **global 级**：规则组总并发兜底
- 多维度 **all-match OR 叠加**：一次请求在所有命中维度同时占一个槽位，任一维度超限即拒绝（429，不排队）

层级语义为**共享池 + 逐级封顶（cap, not reservation）**：每层配置的是上限而非
独占预留，子级实际可用 = min（自身剩余， 父级池剩余），先到先得。该语义与
Envoy RLS descriptor tree / LiteLLM `max_parallel_requests` / OpenAI 组织级
共享池一致，为业界标准做法。

对 LLM 场景（长流式请求、并发昂贵），Gauge 模型比 QPS 更符合直觉：
流式响应从发出到流结束持续占用一个并发槽位。

## 2. 技术选型与关键决策（已确认）

| # | 决策点 | 结论 | 理由 |
|---|---|---|---|
| 1 | 计数模型 | **ZSET 租约成员模型**（§5） | 释放天然幂等、无负数、崩溃 member 自动过期；最坏故障退化为"暂时轻微超额放行且自愈"，无永久损坏。性能与 INCR/DECR 等价（均为每请求 1 次 Lua 往返），差异在正确性与运维负担 |
| 2 | 本期范围 | **三级层级限额全量实现** | 评审验证 all-match 模型原生精确实现"共享池+逐级封顶"语义 |
| 3 | 租户身份来源 | 前置插件注入 `X-Mse-Consumer-Group` 头；插件侧用独立配置项 `limit_by_consumer_group`（头名内置常量） | 语义显式、头名单点管理、消除"写错头名静默失效"故障类、为二期 console 建模铺路；`limit_by_header` 保留作通用逃生门 |
| 4 | 无身份头流量 | **放行不计数**，仅 global 兜底可拦截（§7.3 显式声明） | 继承蓝本行为，已与需求方确认；防未认证流量靠认证插件 |
| 5 | 执行顺序 | priority **610**，默认阶段内先于 ai-token-ratelimit（600）——"先占并发槽、后扣 token" | 被拒请求不消耗 token 额度；反向无泄漏（槽位随 stream done 释放） |
| 6 | priority 注册方式 | WasmPlugin CR / higress-console 插件元数据下发，**不在 Go 代码中设置** | priority 是 Istio 语义字段，数值越大越先执行 |
| 7 | 放行配额头 | `X-Concurrency-Remaining` = **所有命中维度的最小剩余** | "报最紧的桶"业界惯例；Lua 已算出各维度当前值，取 min 零成本 |
| 8 | 测试设施 | Lua 边界用例引入 **miniredis** | 纯内存、CI 友好，不依赖真实 Redis |

## 3. 定位与代码蓝本

| 维度 | 决策 |
|---|---|
| 插件名 | `cluster-concurrency-limit`（`cluster-` 前缀 = Redis 集群共享计数） |
| 目录 | `plugins/wasm-go/extensions/cluster-concurrency-limit/`（config/ + util/ + main.go，同蓝本布局）；按 CONTRIBUTING 要求附 `design/` 目录放设计文档 |
| 代码蓝本 | **ai-token-ratelimit**：`checkRequestAgainstLimitRule`（ai-token-ratelimit/main.go:220）、`hitRateRuleItem`、`findMatchingItem`、`util.GetConsumer` 移植后改造。**注意蓝本是 first-match + global/rule_items 互斥**（其 config/config.go:185 直接报错），本设计为 all-match + 并存，规则收集与校验逻辑是**改造**不是照抄 |
| Redis 客户端 | `wrapper.RedisClient`（higress-group/wasm-go SDK），redis 配置结构与蓝本一致（service_name/port/username/password/timeout/database） |
| 执行阶段 | 默认阶段（UNSPECIFIED_PHASE）。认证插件（key-auth/basic-auth 等）在 AUTHN 阶段，phase 顺序天然保证认证在前；priority 610 只做默认阶段内排序 |
| 身份来源 | 不自己做鉴权，只读 `X-Mse-Consumer` / `X-Mse-Consumer-Group`；读不到 → 该维度不计数、放行 |

**前置顺序约束（部署硬性要求）**：注入 `X-Mse-Consumer-Group` 的前置插件
必须先于本插件执行（priority > 610 或更早阶段），否则租户维度静默失效（§7.2）。

## 4. 配置模型

骨架对齐 cluster-key-rate-limit / ai-token-ratelimit（`rule_name` +
`global_threshold` + `rule_items`），三处语义差异：

1. `global_threshold` 与 `rule_items` **可并存**（蓝本互斥），global 作兜底维度参与 all-match；
2. 多规则 **all-match OR 叠加**（蓝本 first-match）；
3. 阈值字段为 **`concurrency`（int）**——时间窗口语义在 Gauge 模型下不存在。

```yaml
rule_name: ai-route-concurrency
global_threshold:
  concurrency: 200                    # 规则组总并发兜底
rule_items:
  # 租户级：X-Mse-Consumer-Group（前置插件注入）
  - limit_by_consumer_group: ""
    limit_keys:
      - key: "tenant-a"
        concurrency: 10
      - key: "*"
        concurrency: 5
  # 用户级：X-Mse-Consumer，同一 consumer 所有 key 共享额度
  - limit_by_consumer: ""
    limit_keys:
      - key: "alice"
        concurrency: 7                # 子级 cap < 父级池，防饿死（见 §4.2）
      - key: "regexp:.*"
        concurrency: 3
  # apikey 级：key 在任何头/参数/cookie 里都能限
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

### 4.1 配置项总表

| 配置项 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| rule_name | string | 是 | - | 规则名，参与 Redis key 拼装；key 含 `{rule_name}` **hash tag**（§5.1） |
| global_threshold | object | 否 | - | 规则组总并发：`concurrency: int`；与 rule_items 可并存（至少一项） |
| rule_items | array | 否（最多 10 条） | - | 维度规则项，all-match OR 叠加 |
| rule_items[].limit_by_consumer_group | string | 选一 | - | **租户维度**，读 `X-Mse-Consumer-Group`，无需值（头名为插件内置常量） |
| rule_items[].limit_by_consumer | string | 选一 | - | 用户维度，读 `X-Mse-Consumer`，无需值 |
| rule_items[].limit_by_header | string | 选一 | - | header 维度，值为 header 名（通用逃生门） |
| rule_items[].limit_by_param | string | 选一 | - | URL 参数维度 |
| rule_items[].limit_by_cookie | string | 选一 | - | Cookie 维度 |
| rule_items[].limit_by_per_* | string | 选一 | - | 按值分别计数，limit_keys 仅接受 `*` / `regexp:...`（同蓝本） |
| rule_items[].limit_by_per_ip | string | 选一 | - | `from-remote-addr` / `from-header-<名>`，CIDR 写 limit_keys（同蓝本） |
| rule_items[].limit_keys[].key | string | 是 | - | 非 per_ 类型为精确字面值；per_ 类型仅 `*` / `regexp:...` |
| rule_items[].limit_keys[].concurrency | int | 是 | - | 该 key 的并发上限（Gauge），正整数 |
| rejected_code | int | 否 | 429 | 超限状态码 |
| rejected_msg | string | 否 | "concurrency limit exceeded" | 超限响应体 |
| show_limit_quota_header | bool | 否 | false | 返回 `X-Concurrency-Limit` / `X-Concurrency-Remaining`（语义见 §6） |
| lease_ttl_seconds | int | 否 | 600 | member 租约时长（秒），语义见 §5.3 |
| redis | object | 是 | - | 同蓝本 redis 配置 |

校验规则：`concurrency` 必填正整数；per_* 的 limit_keys 不允许字面名；
rule_items 上限 10 条；global_threshold 与 rule_items 至少一项（**不**校验
互斥，与蓝本相反）；"常见错误"文档章节按本语义编写。

### 4.2 三级配置策略：封顶不预留，防饿死靠 cap 梯度

- 每层是上限不是预留，子级可用 = min（自身剩余， 父级池剩余），先到先得；
- **子级 cap = 父级池时防不住饿死**（例：租户池 10、用户各 10，用户 a 占满
  10 后用户 b 为 0——需求方已确认接受该语义）；
- 需缓解时，标准做法是**子级 cap < 父级池**（如用户各 7、租户 10，单用户
  最多给他人留 3 个）。两种策略本模型均可表达，README 给出上述示例。

## 5. 计数生命周期：ZSET 租约成员模型

### 5.1 模型

每个限流维度对应一个 ZSET key；每个在途请求是其中一个 member：

- **member**：请求唯一 ID（优先 `x-request-id`，缺失则插件生成 UUID）；
- **score**：过期时间戳（毫秒）= 获取时刻 + `lease_ttl`；
- **计数** = 清理过期 member 后的 `ZCARD`；
- **释放** = `ZREM` 自己的 member（天然幂等：重复删/删不存在均 no-op）。

Redis key 格式：`{rule_name}:concurrency:<dim>:<value>`。`{rule_name}` 为
hash tag，保证一次请求涉及的所有维度 key 落在 Redis Cluster 同一 slot，单条
Lua 才能原子操作（蓝本均为单 key Eval 无此需求，这是本设计的必要增强）。

### 5.2 状态机

```
on_http_request_headers
  → 规则收集（global + rule_items 全量收集，all-match）
  → 无命中：直接放行（不发起 Redis 调用）
  → 有命中：一条 Lua 脚本原子处理（先检后占，无需回滚）：
      ARGV: now_ms, lease_ttl_ms, member, limit_1..limit_N
      第一轮 i=1..N：ZREMRANGEBYSCORE key_i 0 now_ms          # 惰性清理过期 member
                     cur_i = ZCARD key_i
                     cur_i >= limit_i → 返回 {reject=i, cur_i}  # 第 i 个维度超限
      第二轮 i=1..N：ZADD key_i (now_ms+lease_ttl) member       # 全部通过才占槽
                     PEXPIRE key_i lease_ttl_ms                 # 整 key 垃圾回收
      返回 {ok, cur_1..cur_N}
  → 拒绝：按 global → rule_items 顺序第一个超限维度报告 429
  → 放行：(key, member) 列表与各维度 cur 存入流上下文

OnHttpStreamDone（stream 关闭，唯一释放入口）
  → 对上下文记录的 (key, member) 逐个 ZREM（fire-and-forget，无需脚本）
```

模型特性：

- **先检后占，无回滚**：检查全部通过后第二轮才 ZADD，原子性由单条 Lua
  保证，不存在"加了 A 发现 B 超限要回滚 A"的窗口；
- **无负数、无需下界保护**：释放是删自己的 member 而不是减整数；
- **释放天然幂等**：重复 done / 乱序 done / 迟到 done 均无副作用；
- 计数与释放对高并发安全：所有判断与修改在 Redis 单线程 Lua 内原子完成。

### 5.3 释放时机与租约语义

- `OnHttpStreamDone`（wasm-go `ProcessStreamDone`，SDK plugin_wrapper.go:395）
  在 stream 关闭时必触发——正常结束、上游 reset、客户端断开全覆盖；流式与
  非流式统一绑此事件，插件无需区分形态。stream done 阶段发起 fire-and-forget
  Redis 调用已有生产先例（ai-load-balancer/global_least_request lb_policy.go:284-293）。
- **租约（score 过期）只防御进程崩溃**（OOM / kill -9，done 未执行）：
  崩溃请求的 member 到期后被惰性清理。
- **流存活超过租约**：member 提前过期 → 槽位被提前释放 → 暂时性轻微
  **超额放行**，随流结束自愈，无永久损坏。`lease_ttl_seconds` 默认 600，
  建议大于业务典型流时长；**分片续租为后续优化**（§9），本期不做。
- `PEXPIRE` 使空维度 key 整体过期，不留垃圾 key。

### 5.4 与蓝本的差异清单

| 维度 | ai-token-ratelimit | 本插件 |
|---|---|---|
| 计数语义 | 窗口累加（INCRBY + TTL 过期） | Gauge（ZSET member，score = 租约过期时间） |
| 规则模型 | first-match，global/rule_items 互斥 | all-match，global + rule_items 并存 |
| Redis key | 单 key，无 hash tag | 多 key，`{rule_name}` hash tag 同 slot |
| 阈值字段 | token_per_second/minute/hour/day | concurrency（单字段） |
| 释放 | 无（窗口自然过期） | OnHttpStreamDone 时 ZREM member + score 过期兜底 |
| Lua 脚本 | 多 key INCRBY + 窗口判断 | 两阶段：清理+检查全部 → 通过后 ZADD 全部 |
| 流式 body 处理 | 累计 token usage | 不处理（占用即计数） |
| 执行优先级 | 600（默认阶段） | 610（默认阶段，先于 600 执行） |

### 5.5 两个已验证的安全性事实

- **Envoy 重试不重复计数**：wasm http context 挂在下游 stream 上，上游
  retry 不触发新的 `on_http_request_headers`，每请求只占槽一次；
- **被后续插件拒绝不泄漏**：本插件放行后若 ai-token-ratelimit 等后续插件
  拒绝，stream 仍会关闭并触发 done，槽位正常释放。

## 6. 拒绝语义与可观测性

| 行为 | 决策 |
|---|---|
| 拒绝响应 | `rejected_code`（默认 429）+ `rejected_msg` |
| 拒绝报告 | 按 global → rule_items 数组顺序取**第一个超限维度**报告 |
| 放行配额头 | `show_limit_quota_header: true` 时返回 `X-Concurrency-Limit` / `X-Concurrency-Remaining`，取所有命中维度的**最小剩余** |
| Redis 故障 | **fail-open 放行**（与蓝本一致），记 error 日志 + metric；`Eval` 异步调用 + `HeaderStopAllIterationAndWatermark` 暂停-放行模式复用蓝本 |
| 与其他限流插件共存 | Redis 键前缀独立（`{rule_name}:concurrency:`），无冲突 |

Metrics（复用 `defineCounterMetric` 模式）：

- `concurrency_limit_rejected` — 拒绝数（可接告警）
- `concurrency_limit_redis_error` — fail-open 计数

请求属性：`ctx.SetUserAttribute("concurrency_limit_status", "limited")`
写入 access log（AI 场景走 AILogKey，同蓝本）。

排查：`redis-cli ZRANGE "{rule}:concurrency:..."` 看实时在途 member；
Envoy `/stats` 过滤 `concurrency_limit`；hash tag 兼容 Redis Cluster
（README 注明该行为）。

## 7. 部署与排障

### 7.1 部署清单

1. WasmPlugin 启用并配置 priority **610**（console 元数据或手工 CR）；
2. Redis 服务（McpBridge 注册，`service_name` 与配置一致）；
3. 若启用租户维度：部署注入 `X-Mse-Consumer-Group` 的前置插件（§7.2）；
4. 认证插件（key-auth/jwt-auth/basic-auth）按需配置，保证 `X-Mse-Consumer` 写入。

### 7.2 前置顺序约束（README 显著提示）

> **注入 `X-Mse-Consumer-Group` 的插件必须先于本插件执行**
> （priority > 610 或更早阶段）。否则租户维度读到空头，按 §7.3
> 放行不计数，**租户限额静默失效，无任何报错**。

### 7.3 无身份头行为声明

读不到 `X-Mse-Consumer` / `X-Mse-Consumer-Group` 的请求**放行且不计数**。
含义：未认证流量不受用户/租户维度限制，只有 global 兜底能拦截。防未认证
流量请在路由上配置认证插件拒绝无凭证请求。

### 7.4 排障指引

| 现象 | 排查 |
|---|---|
| 租户维度不生效 | 检查前置注入插件 priority 是否 > 610；`X-Mse-Consumer-Group` 头是否到达本插件 |
| 额度异常偏移 | ZSET 模型下不应发生；检查是否有旧版本（INCR/DECR）残留 key：`redis-cli KEYS "*concurrency*"` |
| 崩溃残留 | member 随 score 过期自动清理；`ZRANGEBYSCORE key -inf +inf WITHSCORES` 看 member 过期时间 |

## 8. 测试与验证计划

**单元测试**（沿用蓝本 main_test 模式，不依赖真实 Redis）：

1. 配置解析：合法/非法（concurrency 缺失/非正整数、per_* 字面名、
   rule_items 超 10 条、global 与 rule_items 并存合法/全缺非法）；
2. 规则收集：global + 多 rule_items all-match 全量命中、无命中、
   per_ip CIDR 匹配；
3. 身份解析：`X-Mse-Consumer` / `X-Mse-Consumer-Group` 存在/缺失/空头。

**Lua 脚本边界用例**（miniredis，纯内存）：

1. 恰好到限：ZADD 后 cur == concurrency → 放行；再来一个 → 拒绝；
2. 先检后占无幻影：第 N 个维度超限拒绝后，所有维度 ZCARD 不变；
3. 释放幂等：ZREM 后 ZCARD 回基线；重复 ZREM 无副作用；
4. 租约过期：ZADD 后不 ZREM，推进时间后过期 member 被清理、ZCARD 回落；
5. 崩溃恢复：模拟进程死亡（不调用 done），租约到期后新请求正常占槽；
6. 长流超租约：member 过期后新请求可占槽（符合 §5.3 预期），迟到 ZREM 不报错。

**端到端验证**（httpbun mock，见 docs/ai-route-fallback-timeout-retry.md §14 模式）：

1. 流式请求（`"stream": true`）挂住期间，第二请求被 429；
2. 流断开后槽位立即释放（OnHttpStreamDone）；
3. 多网关副本共享额度（两 pod 各发一半请求，合计超限即拒）；
4. 三级联动：租户池 10 / 用户 a、b 各 7，a 占 7 后 b 最多 3。

## 9. 后续阶段（不在本设计内）

- **higress-console 联动**：Consumer/租户模型加 `concurrencyLimit` 字段、
  UI、保存时同步插件配置（参考 ai-quota 与 KeyAuthCredentialHandler.java:180
  的联动模式）；
- **分片续租**：响应分片时刷新 member score，消除长流超租约的超额放行窗口；
- **ai 路由作用域**：路由级默认额度，减少逐 consumer 配置成本。

## 附录：文档沿革

| 日期 | 文档 | 说明 |
|---|---|---|
| 2026-09-23 | 原始设计稿 | 首版方案（INCR/DECR + EXPIRE 租约模型） |
| 2026-09-29 | 评审文档 | 核实技术前提，提出 P1 释放缺陷、P2 表述失真等问题，追加三级层级语义 |
| 2026-09-30 | v2 修订稿 | 按 7 项确认决策修订（ZSET 模型、三级范围等） |
| 2026-09-30 | **本文档** | 整合定稿，开发唯一依据 |
