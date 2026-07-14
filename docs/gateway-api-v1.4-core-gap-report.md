# Gateway API v1.4 Core 差距与最小修复报告

## 1. 结论

`origin/main` 和 Higress v2.2.3 在 Gateway API v1.4 `GATEWAY-HTTP` Core 测试中的结果一致：33 个必选测试中 28 个通过、5 个失败、0 个跳过。

5 个失败测试只对应 3 个 Higress core 逻辑缺口：

1. GatewayClass 被错误限制为只能使用固定名称 `higress`；
2. HTTPRoute hostname 没有收窄为 Listener 与 Route hostname 的有效交集；
3. Service backend 是否存在只依赖 Higress `LookupHostname`，缺少 Kubernetes Service 对象存在性兜底。

只修改这 3 处 Higress core 逻辑，并保持 v2.2.3 的 pilot 和 gateway 镜像不变，官方 33 个 Core 测试全部通过。因此不需要修改 Istio submodule、pilot 或 Envoy/gateway 镜像。

## 2. 验证范围

- Gateway API SDK/Conformance Suite：v1.4.0；
- Profile：`GATEWAY-HTTP`；
- 声明能力：`Gateway`、`HTTPRoute`、`ReferenceGrant`；
- 仅执行必须通过的 Core，用例范围内 `Skipped: 0`；
- Higress、Gateway、Secret 和官方测试资源部署在 `gateway-conformance-infra`；
- 测试直接访问 ACK LoadBalancer，不修改官方测试用例，不给失败用例增加豁免。

## 3. 基线和修复后结果

| 场景 | Higress core | pilot | gateway | 结果 |
| --- | --- | --- | --- | --- |
| `origin/main` 基线 | `154782660cabb5ae8d313e6348efe747bb5e9d16` | `de2c9628294f51b13c4a70b3a862b4372890797a` | `481184afc44176eb23d64e0011dc3ea1ae6a410c` | 28 通过、5 失败 |
| v2.2.3 完整基线 | `v2.2.3`，Git tag commit `39ec41aab6eb1d40499bed2847085696de0ebb96` | `de2c9628294f51b13c4a70b3a862b4372890797a` | `fcc50202f47e27f6b8391a4bd9bbc0a9127d89d7` | 28 通过、5 失败 |
| 修复 core + v2.2.3 数据面 | `7d3e4fb8ae946c7bcdf687d51405231de4b4e697` | v2.2.3 原始 tag | v2.2.3 原始 tag | 33 通过、0 失败 |
| 修复后的当前完整组合 | `7d3e4fb8ae946c7bcdf687d51405231de4b4e697` | 当前仓库统一 tag | 当前仓库统一 tag | 33 通过、0 失败 |

`v2.2.3` 与 `origin/main` 的三个相关生产代码文件没有差异，两个基线也复现了完全相同的失败集合。v2.2.3 的较早 gateway 镜像没有增加额外失败。

原始报告及 SHA-256：

| 报告 | SHA-256 |
| --- | --- |
| `out/gateway-api-conformance/ack-same-namespace-origin-main-report.yaml` | `40d643857073e64fc51d769fe30efd8c4dfacc9745c0b3f0b5a3481632aa4908` |
| `out/gateway-api-conformance/ack-v2.2.3-baseline-report.yaml` | `c3f61a97037eb87431afec96bcc233016f979fee9f0808eec6a1a4e6e7fb62d9` |
| `out/gateway-api-conformance/ack-fixed-core-v2.2.3-data-plane-report.yaml` | `2607809c8f058e51932ec74a0b5560df08cea2c2d5d8c5178a0ebf97ccac3137` |
| `out/gateway-api-conformance/ack-same-namespace-final-head-report.yaml` | `ab59aa6f15590cf0784b66a7ccce4cfad541d84468f499cdee5e1c15b811623b` |

## 4. 五个失败用例及根因

| 失败用例 | 官方预期 | main/v2.2.3 实际行为 | 根因 |
| --- | --- | --- | --- |
| `GatewayClassObservedGenerationBump` | 名称任意但 `spec.controllerName` 属于 Higress 的 GatewayClass 应被处理，状态中的 `observedGeneration` 应随 generation 更新 | 用例创建的非 `higress` 名称 GatewayClass 一直保留旧 generation | controller 同时校验名称必须为 `higress` 和 controllerName，错误忽略了同 controllerName 的其他 Class |
| `HTTPRouteHTTPSListener` | 已配置证书但 hostname 不匹配的 HTTPS 请求返回 404 | `unknown-example.org` 完成 TLS 后被错误路由到 backend，返回 200 | VirtualService 使用原始 Route hostname，没有应用 Listener hostname 约束 |
| `HTTPRouteHostnameIntersection` | 只为 Listener hostname 与 Route hostname 的交集生成路由；不相交请求返回 404 | 多个不相交 hostname 仍返回 backend 200 | 同上，生成 VirtualService 时没有计算 hostname 交集 |
| `HTTPRouteListenerHostnameMatching` | 精确和 wildcard hostname 应进入对应 backend，无匹配 hostname 返回 404 | 部分请求进入错误 backend，`foo.com`、`no.matching.host` 等无匹配请求返回 200 | 过宽的 VirtualService hosts 造成多个 Listener/Route 之间发生错误覆盖或竞争 |
| `HTTPRouteServiceTypes` | 已存在的普通、Headless、手工 EndpointSlice Service 都应得到 `ResolvedRefs=True`，并可在端点存在时访问 | 手工 EndpointSlice 场景被标记 `BackendNotFound` | `LookupHostname` 依赖 KRT 之外的 `GatewayContext`；它返回 nil 不能可靠等价于 Kubernetes Service 对象不存在，且缺少 informer 兜底 |

前三个 HTTPRoute hostname 测试是同一个产品缺口的三种外部表现，因此不是五项独立代码修复。

## 5. 最小必要改动

### 5.1 按 controllerName 认领 GatewayClass

文件：`pkg/ingress/kube/gateway/istio/gatewayclass_collection.go`

- 删除 `metadata.name == higress` 的额外限制；
- 保留 `spec.controllerName == higress.io/gateway-controller` 校验；
- 不新增或修改 `GatewayClass.status.supportedFeatures`。

对应测试：增加不同 GatewayClass 名称但 controllerName 相同的单元测试。

### 5.2 计算 Listener/Route hostname 交集

文件：

- `pkg/ingress/kube/gateway/istio/conversion.go`；
- `pkg/ingress/kube/gateway/istio/route_collections.go`。

新增最小交集规则：

1. Listener 未指定 hostname 时保留 Route hostname；
2. Route hostname 比 Listener 更具体时使用 Route hostname；
3. Listener hostname 比 Route 更具体时使用 Listener hostname；
4. 两者不相交时不生成该 VirtualService；
5. 对计算后的 hostname 去重。

golden 文件中 `hosts: ['*']` 收窄为 Listener 指定域名，是该修复的预期输出变化，不是为了迎合测试手工修改结果。

### 5.3 保留 Higress LookupHostname，并增加 Service informer 兜底

文件：`pkg/ingress/kube/gateway/istio/conversion.go`

- 原有 `hostname = <name>.<namespace>.svc.<domainSuffix>` 和 `LookupHostname(hostname, namespace, "Service")` 保持不变；
- `LookupHostname` 仍是第一判断，从而保留 Higress 服务命名和 WasmPlugin 服务绑定语义；
- 只有 `LookupHostname` 返回 nil 时，才使用 `namespace/name` 从 `ctx.Services` informer 检查 Kubernetes Service 是否存在；
- 两条路径都确认不存在时才设置 `BackendNotFound`；
- EndpointSlice 和端点健康状态仍由现有服务发现/数据面链路处理。

兜底使用的 Kubernetes Service informer 与当前 Istio 1.27 fork 的 Service backend 存在性判断来源一致，但不会替换 Higress 原有的 `LookupHostname` 路径。

## 6. 兼容性分析

| 改动 | 兼容性影响 | 风险判断 |
| --- | --- | --- |
| 按 controllerName 认领 GatewayClass | 以前被 Higress 忽略、但 controllerName 明确声明属于 Higress 的其他 GatewayClass 名称现在会被处理 | 行为范围扩大；符合 Gateway API 归属语义。若用户曾依赖“同 controllerName 但改名即可让 Higress 忽略”，行为会变化，但这种依赖不符合规范意图 |
| hostname 交集 | Route 声明了超出 Listener hostname 范围的流量将不再被错误接收，通常由原来的 200/错误 backend 变为 404 | 存在可观察行为收窄，但属于路由正确性和隔离修复；依赖旧错误匹配的流量会受影响，应在发布说明中明确 |
| LookupHostname 后增加 Service informer 兜底 | Higress 原服务命名、Destination Host 和 WasmPlugin 服务绑定路径不变；已存在但 LookupHostname 暂不可见的 Service 不再被标记 `BackendNotFound` | 只改变 false negative 的状态判断。无可用端点时请求仍可能返回 503，修复不会凭空生成端点，风险低 |

## 7. 已排除和回滚的无关改动

以下改动不是这 5 个 Core 失败的必要条件，最终实现中均不保留：

- Istio submodule 修改；
- 跨 namespace Service FQDN 或 workload selector 放宽；
- selector 初始化时序调整；
- Istio SDS/Secret namespace 限制放宽；
- GatewayClass `status.supportedFeatures` 写入；
- 对官方测试 YAML、断言或超时进行定制；
- 为失败用例增加 skip 或豁免。

最终生产逻辑变更只涉及上述 3 个 Higress core 文件。测试框架、Make target 和 CI job 属于持续执行官方测试所需的工程接入，不改变 Gateway API 产品行为。
