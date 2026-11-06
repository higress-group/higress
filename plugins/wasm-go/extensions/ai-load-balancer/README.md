---
title: AI负载均衡
keywords: [higress, llm, load balance]
description: 针对LLM服务的负载均衡策略
---

# 功能说明

**注意**：
- Higress网关版本需要>=v2.1.5

对LLM服务提供热插拔的负载均衡策略，如果关闭插件，负载均衡策略会退化为服务本身的负载均衡策略（轮训、本地最小请求数、随机、一致性hash等）。

配置如下：

| 名称                | 数据类型         | 填写要求          | 默认值       | 描述                                 |
|--------------------|-----------------|------------------|-------------|-------------------------------------|
| `lb_type`        | string          | 选填              | endpoint    | 负载均衡类型，可选`endpoint`,`cluster` |
| `lb_policy`      | string          | 必填              |             | 负载均衡策略类型    |
| `lb_config`      | object          | 必填              |             | 当前负载均衡策略类型的配置    |

`lb_type` 为 `endpoint` 时支持的负载均衡策略包括：

- `global_least_request`: 基于redis实现的全局最小请求数负载均衡
- `prefix_cache`: 基于 prompt 前缀匹配选择后端节点，如果通过前缀匹配无法匹配到节点，则通过全局最小请求数进行服务节点的选择
- `endpoint_metrics`: 基于 llm 服务暴露的 metrics 进行负载均衡

`lb_type` 为 `cluster` 时支持的负载均衡策略包括：
- `cluster_metrics`: 基于网关统计的不同service的指标进行服务之间的负载均衡
- `cluster_hash`: 读取指定请求头做 FNV-1a 一致性 hash，将相同 key 的请求始终路由到同一 cluster，支持按权重分配流量

这两种策略只负责将选中的 cluster 写入内部请求头。用户必须另外[显式配置 EnvoyFilter](#为-cluster-模式显式配置-envoyfilter)，将目标路由改为使用 `route.cluster_header`；Higress 控制面不会自动创建、更新或删除该 EnvoyFilter。

# 全局最小请求数
## 功能说明

```mermaid
sequenceDiagram
	participant C as Client
	participant H as Higress
	participant R as Redis
	participant H1 as Host1
	participant H2 as Host2

	C ->> H: 发起请求
	H ->> R: 获取 host ongoing 请求数
	R ->> H: 返回结果
	H ->> R: 根据结果选择当前请求数最小的host，计数+1
	R ->> H: 返回结果
	H ->> H1: 绕过service原本的负载均衡策略，转发请求到对应host
	H1 ->> H: 返回响应
	H ->> R: host计数-1
	H ->> C: 返回响应
```

## 配置说明

| 名称                | 数据类型         | 填写要求          | 默认值       | 描述                                 |
|--------------------|-----------------|------------------|-------------|-------------------------------------|
| `serviceFQDN`      | string          | 必填              |             | redis 服务的FQDN，例如: `redis.dns`    |
| `servicePort`      | int             | 必填              |             | redis 服务的port                      |
| `username`         | string          | 必填              |             | redis 用户名                         |
| `password`         | string          | 选填              | 空          | redis 密码                           |
| `timeout`          | int             | 选填              | 3000ms      | redis 请求超时时间                    |
| `database`         | int             | 选填              | 0           | redis 数据库序号                      |

## 配置示例

```yaml
lb_type: endpoint
lb_policy: global_least_request
lb_config:
  serviceFQDN: redis.static
  servicePort: 6379
  username: default
  password: '123456'
```

# 前缀匹配
## 功能说明
根据 prompt 前缀匹配选择 pod，以复用 KV Cache，如果通过前缀匹配无法匹配到节点，则通过全局最小请求数进行服务节点的选择

例如以下请求被路由到了pod 1

```json
{
  "model": "qwen-turbo",
  "messages": [
    {
      "role": "user",
      "content": "hi"
    }
  ]
}
```

那么后续具有相同前缀的请求也会被路由到 pod 1
```json
{
  "model": "qwen-turbo",
  "messages": [
    {
      "role": "user",
      "content": "hi"
    },
    {
      "role": "assistant",
      "content": "Hi! How can I assist you today? 😊"
    },
    {
      "role": "user",
      "content": "write a short story aboud 100 words"
    }
  ]
}
```

## 配置说明

| 名称                | 数据类型         | 填写要求          | 默认值       | 描述                                 |
|--------------------|-----------------|------------------|-------------|-------------------------------------|
| `serviceFQDN`      | string          | 必填              |             | redis 服务的FQDN，例如: `redis.dns`    |
| `servicePort`      | int             | 必填              |             | redis 服务的port                      |
| `username`         | string          | 必填              |             | redis 用户名                         |
| `password`         | string          | 选填              | 空          | redis 密码                           |
| `timeout`          | int             | 选填              | 3000ms      | redis 请求超时时间                    |
| `database`         | int             | 选填              | 0           | redis 数据库序号                      |
| `redisKeyTTL`      | int             | 选填              | 1800s      | prompt 前缀对应的key的ttl             |

## 配置示例

```yaml
lb_type: endpoint
lb_policy: prefix_cache
lb_config:
  serviceFQDN: redis.static
  servicePort: 6379
  username: default
  password: '123456'
```

# 最小负载
## 功能说明
[gateway-api-inference-extension](https://github.com/kubernetes-sigs/gateway-api-inference-extension/blob/main/README.md) 的 wasm 实现

```mermaid
sequenceDiagram
	participant C as Client
	participant H as Higress
	participant H1 as Host1
	participant H2 as Host2

	loop 定期拉取metrics
		H ->> H1: /metrics
		H1 ->> H: vllm metrics
		H ->> H2: /metrics
		H2 ->> H: vllm metrics
	end

	C ->> H: 发起请求
	H ->> H1: 根据vllm metrics选择合适的pod，绕过服务原始的lb policy直接转发
	H1 ->> H: 返回响应
	H ->> C: 返回响应
```

<!-- pod选取流程图如下：

![](https://github.com/kubernetes-sigs/gateway-api-inference-extension/blob/main/docs/scheduler-flowchart.png) -->

## 配置说明

| 名称                | 数据类型         | 填写要求          | 默认值       | 描述                                 |
|--------------------|-----------------|------------------|-------------|-------------------------------------|
| `metric_policy`      | string | 必填 | | 如何使用llm暴露的metrics做负载均衡，当前支持`[default, least, most]` |
| `target_metric`      | string | 选填 | | 要使用的metric名称，`metric_policy` 取值为 `least` 或者 `most` 时生效 |
| `rate_limit`      | string | 选填 | 1 | 单个节点处理请求比例上限，取值范围0~1 |


## 配置示例

使用 [gateway-api-inference-extension](https://github.com/kubernetes-sigs/gateway-api-inference-extension/blob/main/README.md) 中的算法

```yaml
lb_type: endpoint
lb_policy: metrics_based
lb_config:
  metric_policy: default
  rate_limit: 0.6 # 单个节点承载的最大请求比例
```

根据当前排队请求数进行负载均衡

```yaml
lb_type: endpoint
lb_policy: metrics_based
lb_config:
  metric_policy: least
  target_metric: vllm:num_requests_waiting
  rate_limit: 0.6 # 单个节点承载的最大请求比例
```

根据当前GPU中正在处理的请求数进行负载均衡

```yaml
lb_type: endpoint
lb_policy: metrics_based
lb_config:
  metric_policy: least
  target_metric: vllm:num_requests_running
  rate_limit: 0.6 # 单个节点承载的最大请求比例
```


# 跨服务负载均衡

## 配置说明

| 名称                | 数据类型         | 填写要求          | 默认值       | 描述                                 |
|--------------------|-----------------|------------------|-------------|-------------------------------------|
| `mode`      | string | 必填 | | 如何使用服务级指标做负载均衡，当前支持`[LeastBusy, LeastTotalLatency, LeastFirstTokenLatency, AdaptiveScore]` |
| `service_list`      | []string | 必填 | | 路由后端服务列表 |
| `rate_limit`      | string | 选填 | 1 | 单个服务处理请求比例上限，取值范围0~1 |
| `cluster_header` | string | 选填 | `x-higress-target-cluster` | 通过取该header的值得知需要路由到哪个后端服务 |
| `queue_size`      | int | 选填 | 100 | 根据最近的多少个请求进行观测指标的计算 |
| `ewma_beta` | float | 选填 | 0.5 | `AdaptiveScore` 中历史 EWMA 值的权重，取值范围 0~1 |
| `p2c_choices` | int | 选填 | 2 | `AdaptiveScore` 每次随机采样并比较的候选服务数，配置值不小于服务数时比较全部服务 |
| `ttft_weight` | float | 选填 | 0.7 | `AdaptiveScore` 首包延迟权重 |
| `total_latency_weight` | float | 选填 | 0.3 | `AdaptiveScore` 总响应延迟权重 |
| `error_penalty` | float | 选填 | 3.0 | `AdaptiveScore` 对插件实例启动以来累计错误率的惩罚系数 |
| `failure_cooldown_ms` | int | 选填 | 30000 | `AdaptiveScore` 请求失败后服务进入冷却的时间；未配置或为 0 时使用默认值 |
| `metrics_missing_policy` | string | 选填 | `least_busy` | `AdaptiveScore` 缺少延迟指标时的兜底策略，默认按当前并发数选择 |
| `global_inflight_enabled` | bool | 选填 | false | `AdaptiveScore` 是否使用 Redis 记录跨网关实例的全局 in-flight |
| `serviceFQDN` | string | `global_inflight_enabled=true` 时必填 | | Redis 服务的 FQDN |
| `servicePort` | int | `global_inflight_enabled=true` 时必填 | | Redis 服务的端口 |
| `username` | string | 选填 | | Redis 用户名 |
| `password` | string | 选填 | 空 | Redis 密码 |
| `database` | int | 选填 | 0 | Redis 数据库序号 |
| `global_inflight_key_prefix` | string | 选填 | `higress:adaptive_score_inflight` | 全局 in-flight 计数 Redis key 前缀，实际 key 会追加 route 和 mode |
| `global_inflight_timeout` | int | 选填 | 3000 | Redis 请求超时时间，单位毫秒；未配置或为 0 时使用默认值 |
| `global_inflight_key_ttl` | int | 选填 | 1800 | 全局 in-flight Redis key 的 TTL，单位秒；未配置或为 0 时使用默认值 |

`mode` 各取值含义如下：
- `LeastBusy`: 路由到当前并发请求数最少的服务
- `LeastTotalLatency`: 路由到当前RT最低的服务
- `LeastFirstTokenLatency`: 路由到当前首包RT最低的服务
- `AdaptiveScore`: 综合 EWMA 首包延迟、EWMA 总延迟、当前并发数和累计失败率计算分数，选择分数最低的服务；适合 LLM 后端服务延迟和负载持续波动的场景

`AdaptiveScore` 的失败率基于当前插件实例生命周期内累计的成功和失败次数计算，目前不使用时间窗口或衰减。

`AdaptiveScore` 开启 `global_inflight_enabled` 后，会使用 Redis Lua 原子完成“按本地分数和全局 in-flight 修正后的分数选择服务，并将选中服务计数 +1”。请求结束时插件会将该服务计数 -1。Redis 初始化、请求或返回异常时会降级到本地 `AdaptiveScore`，不阻断请求。

## 配置示例

```yaml
lb_type: cluster
lb_policy: cluster_metrics
lb_config:
  mode: AdaptiveScore # 策略名称
  cluster_header: x-higress-target-cluster # 写入选中 cluster 的内部请求头
  queue_size: 100 # 统计指标时使用的最近请求数
  rate_limit: 0.6 # 单个服务承载的最大请求比例
  ewma_beta: 0.5 # 历史 EWMA 值权重
  p2c_choices: 2 # 每次比较的候选服务数
  ttft_weight: 0.7 # 首包延迟权重
  total_latency_weight: 0.3 # 总响应延迟权重
  error_penalty: 3.0 # 错误率惩罚系数
  failure_cooldown_ms: 30000 # 失败冷却时间
  global_inflight_enabled: true # 开启跨网关实例全局 in-flight
  serviceFQDN: redis.static # Redis 服务
  servicePort: 6379
  username: default
  password: ""
  database: 0
  global_inflight_key_prefix: higress:adaptive_score_inflight
  global_inflight_timeout: 3000
  global_inflight_key_ttl: 1800
  service_list:
  - outbound|80||test-1.dns
  - outbound|80||test-2.static
```

# Cluster Hash（一致性 Hash 路由）

## 功能说明

读取指定请求头的值，使用 FNV-1a 一致性 hash 算法将请求路由到固定的上游集群，确保相同 hash key 的请求始终落到同一个 cluster，同时支持按百分比权重控制各 cluster 的流量分配。

需要配合下文的[显式 EnvoyFilter 配置](#为-cluster-模式显式配置-envoyfilter)一起使用。

## 配置说明

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|------|----------|----------|--------|------|
| `clusters` | []ClusterEntry | 必填 | - | cluster 列表，所有 `weight` 之和必须为 100 |
| `hash_header` | string | 选填 | `x-mse-consumer` | 读取 hash key 的请求头名称 |
| `cluster_header` | string | 选填 | `x-higress-target-cluster` | 写入目标 cluster 的请求头名称 |

### ClusterEntry 字段

| 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `cluster` | string | 是 | 上游集群名称，如 `outbound|443||llm-xxx.internal.static` |
| `weight` | int | 是 | 百分比权重，所有 cluster 的 weight 之和必须为 100 |

## 配置示例

```yaml
lb_type: cluster
lb_policy: cluster_hash
lb_config:
  clusters:
    - cluster: "outbound|80||llm-test1.internal.static"
      weight: 69
    - cluster: "outbound|443||llm-test2.internal.dns"
      weight: 30
    - cluster: "outbound|443||llm-test3.internal.dns"
      weight: 1
  hash_header: x-mse-consumer
  cluster_header: x-higress-target-cluster
```

若请求缺少 hash header，插件直接返回 **403**。

# 为 cluster 模式显式配置 EnvoyFilter

`lb_type: cluster` 时，ai-load-balancer 只会把选中的 cluster 名称写入 `lb_config.cluster_header` 指定的请求头。用户必须自行创建并维护 EnvoyFilter，将对应的 Envoy HTTP route 改为从该请求头读取目标 cluster。下面的示例假设：

- Higress 安装在 `higress-system` namespace，网关 Pod 标签为 `app: higress-gateway`；
- 目标 HTTPRoute 或 Ingress 位于 `default` namespace，资源名为 `llm-route`，生成的 xDS route 名为 `default/llm-route`；
- 目标域名为 `llm.example.com`，网关监听端口为 `80`，对应的 xDS route configuration 和 virtual host 名分别为 `higress-rds-80.llm.example.com` 和 `llm.example.com:80`；
- 后端 Kubernetes Service 为 `llm-a` 和 `llm-b`，端口均为 `80`。

先创建下面的路由，假设 Gateway `default/llm-gateway` 已有接收 `llm.example.com` 的 `80` 端口 HTTP listener，且两个后端 Service 均已存在。启用 `onlyPushRouteCluster=true` 时，必须保留两个 backendRefs，才能下发两个 cluster。若使用 Ingress，则通过 `higress.io/destination` 注解引用两个后端。

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: llm-route
  namespace: default
spec:
  parentRefs:
    - name: llm-gateway
  hostnames:
    - llm.example.com
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: llm-a
          port: 80
          weight: 50
        - name: llm-b
          port: 80
          weight: 50
```

然后为目标 route 创建 ai-load-balancer WasmPlugin：

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: ai-load-balancer-cluster
  namespace: higress-system
spec:
  pluginName: ai-load-balancer
  url: oci://higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/ai-load-balancer:2.0.3
  matchRules:
    - ingress:
        - default/llm-route
      config:
        lb_type: cluster
        lb_policy: cluster_hash
        lb_config:
          clusters:
            - cluster: "outbound|80||llm-a.default.svc.cluster.local"
              weight: 50
            - cluster: "outbound|80||llm-b.default.svc.cluster.local"
              weight: 50
          hash_header: x-ai-session
          cluster_header: x-higress-target-cluster
```

再显式创建与同一 route 对齐的 EnvoyFilter：

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: ai-load-balancer-cluster-header
  namespace: higress-system
spec:
  workloadSelector:
    labels:
      app: higress-gateway
  configPatches:
    - applyTo: HTTP_ROUTE
      match:
        context: GATEWAY
        routeConfiguration:
          name: higress-rds-80.llm.example.com
          vhost:
            name: llm.example.com:80
            route:
              name: default/llm-route
              action: ROUTE
      patch:
        operation: MERGE
        value:
          route:
            cluster_header: x-higress-target-cluster
```

`patch.value.route.cluster_header` 必须与 ai-load-balancer 的 `lb_config.cluster_header` 完全一致。`matchRules[].ingress` 和 `vhost.route.name` 也必须指向同一条实际 route。原 HTTPRoute 或 Ingress 仍需引用所有可能被选中的后端服务，使对应 cluster 存在于 Envoy 中；该 EnvoyFilter 只改变 route 选择 cluster 的方式，不会创建 cluster。

EnvoyFilter 的 `metadata.namespace` 必须是目标网关工作负载所在的 namespace，`workloadSelector` 也必须匹配实际网关 Pod 标签。`context: GATEWAY` 将补丁限定在网关路由上；`routeConfiguration.name`、`vhost.name` 和 `vhost.route.name` 应继续把范围缩小到需要启用 cluster 模式的 route。Higress 默认的按 host RDS 名形如 `higress-rds-<port>.<host>`，vhost 名形如 `<host>:<port>`。

标准 Istio RDS 的 `http.<port>` 不包含 port name 或 gateway 部分，应使用实际观察到的 route configuration 名和 vhost/route 名限定范围。HTTPS RDS 名形如 `https.<port>.<server-port-name>.<internal-gateway-name>.<namespace>`。Higress 的 Gateway API 内部 server port name 固定为 `default`，内部 Gateway 名的生成后缀取自 listener 名，而非协议。

选择 matcher 前先检查下面的 config dump。例如，**仅当 dump 中实际显示** `https.443.default.llm-gateway-istio-autogenerated-k8s-gateway-https.default`、vhost `llm.example.com:443` 和 route `default/llm-route` 时，才用以下 `match` 替换 HTTP 示例中的 match：

```yaml
match:
  context: GATEWAY
  routeConfiguration:
    portNumber: 443
    portName: default
    gateway: default/llm-gateway-istio-autogenerated-k8s-gateway-https
    vhost:
      name: llm.example.com:443
      route:
        name: default/llm-route
        action: ROUTE
```

不要将此 matcher 与 `higress-rds-*` 名称混用。实际 RDS、内部 Gateway、vhost 和 route 名应从 dump 获取，不要根据 Kubernetes 资源名猜测：

```bash
kubectl -n higress-system exec deploy/higress-gateway -- \
  pilot-agent request GET config_dump > /tmp/higress-config-dump.json

jq -r '
  .configs[]
  | select(."@type" == "type.googleapis.com/envoy.admin.v3.RoutesConfigDump")
  | .dynamic_route_configs[].route_config
  | .name as $route_config
  | .virtual_hosts[]
  | . as $vhost
  | .routes[]
  | [$route_config, $vhost.name, ($vhost.domains | join(",")), .name,
     (.route.cluster_header // .route.clusterHeader // "-")]
  | @tsv
' /tmp/higress-config-dump.json
```

应用 EnvoyFilter 后，再运行一次上述命令。目标行最后一列必须显示 `x-higress-target-cluster`；同时可用下面的命令确认 Kubernetes 对象和网关标签：

```bash
kubectl -n higress-system get envoyfilter ai-load-balancer-cluster-header -o yaml
kubectl -n higress-system get pods -l app=higress-gateway --show-labels
```

同一 HTTPRoute 或 Ingress 的所有 rules/paths 共享相同的生成 route 名，因此该补丁作用于整个资源；所有被修改的 rule/path 都必须被会写入该 header 的 WasmPlugin matchRules 和配置覆盖。

Higress 不会自动补建或修正这个 EnvoyFilter。补丁缺失或失配时，原来的 `weighted_clusters` 路由会静默继续生效，插件写入的路由 header 被忽略。路由重命名、host/port/namespace 变化、SRDS 开关或工作负载标签变化都可能使 matcher 失效。发生这些变化后应重新检查 dump；没有控制器自动协调修复补丁。

若 route 已使用 `cluster_header`，但 header 缺失或指向不存在的 cluster，cluster 解析失败：Gateway API HTTPRoute 返回 `500`，Ingress 路由默认返回 `503`。带有 `route_not_found` 的 `404` 表示没有匹配到任何 route，应另行排查。

`x-higress-target-cluster` 是内部路由控制头。插件会在命中的路由上覆盖它。匹配覆盖范围是信任边界的一部分：若 EnvoyFilter 修改了 WasmPlugin matchRules 或配置未覆盖的路由，客户端传入的 header 就能选择 Envoy ClusterManager 中任意可用的 cluster。必须确保每条被修改的路由都执行插件并写入同名 header，同时确保后续过滤器不会用客户端可控的数据替换它。若存在可信上游边界，可在该处清除或拒绝入站同名 header 作为纵深防御；直接面向互联网的 Higress 必须自行保证匹配覆盖范围一致。
