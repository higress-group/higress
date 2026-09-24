# Higress 限流插件 FAQ

> 中文版本是内容基准；[英文版本](rate-limit-plugin-faq-en.md)为同步翻译。本文同时适用于 `ai-token-ratelimit` 和 `cluster-key-rate-limit`，字段差异会显式标注。

## 目录

1. [限流不生效](#rate-limit-not-taking-effect)
2. [配置被拒：`limit_by_per_*` 写了字面名](#literal-name-in-per-rule)
3. [配置被拒：缺少 `limit_keys`](#missing-limit-keys)
4. [Redis 连接异常](#redis-connection-issues)
5. [诊断命令速查](#diagnostic-command-cheat-sheet)
6. [二次开发说明](#developer-notes)

<a id="rate-limit-not-taking-effect"></a>
## 1. 限流不生效

### 1.1 请求始终 200，从不返回 429

**现象：** 请求持续成功，配置的阈值似乎没有生效。

**根因：** 常见原因依次是：路由/匹配条件未命中；配置解析失败，插件保留旧配置或未加载新规则；计数维度与实际请求值不一致；Redis 调用没有发生。

**诊断步骤：**

1. 在网关日志中搜索插件名、`limit_by_per_`、`missing limit_keys` 和 `must start with`。
2. 查看 `/stats` 中 Wasm 配置拒绝类计数是否增长。
3. 查看 `/clusters` 中目标 Redis cluster 是否存在，再检查其连接计数。
4. 最后用 Redis `SCAN` 检查限流 key；没有 key 不等于 Redis 故障。

**修复：** 先修复最早出现的配置解析或匹配错误，再处理 Redis 连接。不要在尚未确认规则加载时直接归因于 Redis。

**参考：** [#4067](https://github.com/higress-group/higress/issues/4067)、[#2646](https://github.com/higress-group/higress/issues/2646)。

### 1.2 Redis 中始终没有限流 key

**现象：** `SCAN` 找不到 Higress 限流 key。

**根因：** 规则可能未命中、配置可能在数据面被拒绝，或者尚未发生需要计数的请求。只有规则执行到 Redis 调用后才会产生 key。

**诊断步骤：** 先确认插件日志无解析错误，再发起确定能命中的请求，同时观察 `/stats`、`/clusters` 和 Redis `MONITOR`（仅限短时测试环境）。

**修复：** 让请求值与 `limit_by_*`/`limit_keys` 语义一致；若连接计数增长但调用失败，再检查端口、认证和网络。

**参考：** [第 2 节](#literal-name-in-per-rule)、[第 3 节](#missing-limit-keys)。

### 1.3 `cx_total=0` 且 `cx_connect_fail=0`

**现象：** Redis cluster 没有连接成功或失败计数。

**根因：** 通常表示数据面没有尝试连接：规则没有加载/命中，或查看的 cluster 名不正确。这不同于连接失败。

**诊断步骤：** 从 `service_name` 和 `service_port` 推导 `outbound|<port>||<service_name>`，在 `/clusters` 中确认对应条目；再查看 Wasm 配置拒绝日志。

**修复：** 修复加载或匹配问题；不要仅通过修改 Redis 密码来处理零连接计数。

**参考：** [#4067](https://github.com/higress-group/higress/issues/4067)。

### 1.4 `update_rejected` / `config_fail` 持续上涨

**现象：** 每次控制面推送后拒绝计数增加。

**根因：** 插件解析器返回了错误，例如 `limit_by_per_*` 使用字面 key、缺少 `limit_keys`，或 IP 来源格式不合法。

**诊断步骤：** 按时间关联拒绝计数与网关日志中的完整错误；不要只看控制面对象状态。

**修复：** 按错误中的字段名、错误值和迁移建议修正配置，然后确认计数停止增长。

**参考：** [第 2 节](#literal-name-in-per-rule)、[第 3 节](#missing-limit-keys)。

### 1.5 `config_dump` 不等于数据面已加载

**现象：** `config_dump` 能看到配置，但限流仍不生效。

**根因：** `config_dump` 证明配置已下发，不证明 Wasm 插件接受并激活了它。

**诊断步骤：** 同时检查插件日志、Wasm 拒绝计数、目标 cluster 和 Redis 调用。

**修复：** 以数据面加载结果为准，修复解析错误后重新观察 exact-current 配置。

**参考：** [#2464](https://github.com/higress-group/higress/issues/2464)。

<a id="literal-name-in-per-rule"></a>
## 2. 配置被拒：`limit_by_per_*` 写了字面名

### 2.1 错误信息长什么样

```text
the "limit_by_per_consumer" restriction must start with 'regexp:' or be exactly '*' (got "alice"); to match an exact name, use the non-per variant "limit_by_consumer" instead (limit_keys stay the same)
```

`per_` 表示“按每一个实际值分别建桶”，因此其 `limit_keys` 是选择器，只接受 `*` 或 `regexp:...`。精确字面值属于非 `per_` 变体。

### 2.2 `per_*` 与非 `per_*` 选择表

| 需求 | 使用字段 | `limit_keys[].key` |
| --- | --- | --- |
| 只限制精确请求头值 | `limit_by_header` | 字面值 |
| 每个请求头值分别限流 | `limit_by_per_header` | `*` 或 `regexp:...` |
| 只限制精确参数值 | `limit_by_param` | 字面值 |
| 每个参数值分别限流 | `limit_by_per_param` | `*` 或 `regexp:...` |
| 只限制精确 consumer | `limit_by_consumer` | consumer 字面名 |
| 每个 consumer 分别限流 | `limit_by_per_consumer` | `*` 或 `regexp:...` |
| 只限制精确 Cookie 值 | `limit_by_cookie` | 字面值 |
| 每个 Cookie 值分别限流 | `limit_by_per_cookie` | `*` 或 `regexp:...` |

### 2.3 迁移对照

将 `limit_by_per_header/param/consumer/cookie` 改为对应的 `limit_by_header/param/consumer/cookie`，保留原 `limit_keys` 和阈值字段。`limit_by_per_ip` 是例外：它选择 IP 来源，CIDR 仍写在 `limit_keys`。

<a id="missing-limit-keys"></a>
## 3. 配置被拒：缺少 `limit_keys`

### 3.1 错误信息长什么样

```text
missing limit_keys in config for limit_by_per_ip; add at least one entry, e.g. key: "0.0.0.0/0"
```

空数组会得到相同的类型和示例提示，但错误前缀是 `config limit_keys cannot be empty`。

### 3.2 各类型最小合法 key

| 类型 | 最小 `limit_keys` 示例 |
| --- | --- |
| `limit_by_header/param/cookie` | `- key: "<exact-value>"` |
| `limit_by_consumer` | `- key: "<consumer-name>"` |
| `limit_by_per_header/param/consumer/cookie` | `- key: "*"` |
| `limit_by_per_ip` | `- key: "0.0.0.0/0"` |

每项还必须包含插件对应的正数阈值：AI Token 插件使用 `token_per_*`，集群 Key 限流使用 `query_per_*`。

<a id="redis-connection-issues"></a>
## 4. Redis 连接异常

### 4.1 `cx_connect_fail > 0`

**现象：** 目标 Redis cluster 的连接失败计数增长。

**根因：** 常见于端口错误、认证失败、DNS/路由不可达或网络策略拒绝。

**诊断步骤：** 在 `/clusters` 中确认准确 cluster 名和端口；从网关容器验证 DNS/TCP；检查 Redis 认证日志。

**修复：** 修正 `service_name`、`service_port`、认证或网络策略。连接失败与 `cx_total=0` 是两类问题。

**参考：** [#2000](https://github.com/higress-group/higress/issues/2000)。

### 4.2 `service_port` 与 cluster 端口

两个插件都通过 `FQDNCluster` 生成 `outbound|<service_port>||<service_name>`。省略端口时，`.static` 服务默认使用 80，其他服务默认使用 6379。控制台生成的固定地址若实际 Redis 监听 6379，应显式填写 `service_port: 6379`。

### 4.3 关于 `.static` 服务

`.static` 是固定地址服务命名约定，不代表 Redis 必然监听 80。cluster 名使用逻辑端口；它必须与控制面生成的 cluster 一致。修改 McpBridge 端口后应重新检查 cluster、认证和插件状态。

### 4.4 历史问题：修改 McpBridge 端口后认证丢失

[#2000](https://github.com/higress-group/higress/issues/2000) 记录过修改端口后 Redis 命令不再带 AUTH、重启插件后恢复的现象。当前 Redis wrapper 会在未 ready 时重新调用 `RedisInit`，但排查升级/兼容问题时仍应核对实际版本、cluster 变化和认证日志。

<a id="diagnostic-command-cheat-sheet"></a>
## 5. 诊断命令速查

```bash
# Wasm 配置与 Redis 连接相关指标
kubectl -n higress-system exec <gateway-pod> -- \
  curl -s '127.0.0.1:15000/stats?filter=wasm|redis|update_rejected|config_fail|cx_'

# 查找目标 Redis cluster
kubectl -n higress-system exec <gateway-pod> -- \
  curl -s '127.0.0.1:15000/clusters' | grep -A8 'outbound|<port>||<service_name>'

# 配置解析日志
kubectl -n higress-system logs <gateway-pod> --tail=2000 \
  | grep -Ei 'ai-token-ratelimit|cluster-key-rate-limit|limit_by_per_|missing limit_keys|must start with|redis'

# Redis：生产环境使用 SCAN，不要使用阻塞式 KEYS
redis-cli -h <host> -p <port> --scan --pattern '*ratelimit*'
```

<a id="developer-notes"></a>
## 6. 二次开发说明

### 6.1 `per_*` 与非 `per_*` 的语义契约

非 `per_` 类型把 `limit_keys` 当精确值；非 IP 的 `per_*` 类型把它当 `*`/regexp 选择器；`limit_by_per_ip` 把字段值当 IP 来源、把 `limit_keys` 当 IP/CIDR。新增类型时必须同时更新解析、错误示例、测试和双语文档。

### 6.2 `FQDNCluster` 到 Envoy cluster 名

当前依赖中的 `wrapper.FQDNCluster.ClusterName()` 固定生成 `outbound|<port>||<fqdn>`。插件用 `service_name` 作为 FQDN、`service_port` 作为端口；任何控制面端口变化都会改变查找目标。

### 6.3 `RedisClient.Init` / `Ready` 生命周期

`Init` 设置重试闭包并调用 `RedisInit`。首次初始化失败时 client 保持 `ready=false`、记录告警，并允许后续 `Command`/`Eval` 再认证；`Ready()` 只是当前状态快照，不是永久健康承诺。不要把一次 `Init` 返回当成持续连接证明。

### 6.4 如何新增 `limit_by_*` 类型

1. 新增 `LimitRuleItemType` 常量和字段解析。
2. 定义它使用精确、`*`/regexp 还是 IP/CIDR key。
3. 更新 `exampleLimitKeyForType` 和可操作错误。
4. 为合法输入、非法输入和迁移建议补单元测试。
5. 同步两个插件时分别使用 `token_per_*` / `query_per_*`，并更新四份 README 与本 FAQ。

### 6.5 不要做什么

- 不要为共享错误 helper 引入跨插件依赖；两个插件是独立 Go module。
- 不要接受字面值作为非 IP `per_*` 的隐式精确匹配，这会改变既有配置语义。
- 不要把 CIDR 放进 `limit_by_per_ip` 字段。
- 不要用 `config_dump`、单次 `Ready()` 或“Redis 中没有 key”单独证明根因。
- 不要在生产 Redis 上用 `KEYS` 做常规诊断。
