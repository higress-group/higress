# 模型请求小时监控（OPE-11003）

数据链路为 `ai-route-monitor → Envoy 宿主 Counter → 每 Pod /stats/prometheus → Prometheus → Grafana`。不逐请求 HTTP 上报数据库或 Grafana。已在远程 Mac 的真实 Higress 2.1.9 容器验证终态、来源报告、实际导出和 Prometheus 查询，证据见 [VALIDATION](VALIDATION.md)。2026-11-05 已补充隔离 Grafana 导入、真实登录和浏览器渲染验证，见 [报告](grafana-validation/README.md)。尚未部署到实际集群。

## 指标契约

插件定义原始宿主名：

```
wujie_model_requests_total.wujie_source=wujieai.wujie_model=gpt-4.wujie_outcome=success
```

真实 Higress 2.1.9 arm64 导出为 `wujie_model_requests_total{wujie_source,wujie_model,wujie_outcome}`，本镜像没有额外前缀；其他宿主版本仍需验证。启用 `gateway.modelRequestMetrics` 后，bootstrap 的三个 scoped stats tags 抽出 `wujie_source/wujie_model/wujie_outcome` 并移除名称片段。PodMonitor 将这些临时标签映射为 `source/model/outcome`，删除临时标签，并把剩余的带前缀名称规范为：

```
wujie_model_requests_total{source="wujieai",model="gpt-4",outcome="success"}
wujie_model_submissions_total{source="wujieai",model="gpt-4",outcome="success"}
```

`instance/pod/namespace/job` 是采集目标维度，必须保留。请求只在 SDK `proxy_on_log → OnHttpStreamDone` 时递增一次；内部重试不增加请求分母。无 token usage/扣费依赖。

| outcome | 判定，按优先级 |
|---|---|
| timeout | Envoy 明确超时 flags（UT/SI/UMSDR/DT）或 HTTP 408/504 |
| interrupted | 上下游连接/流 reset、协议错误等明确 flags；不推断为用户主动取消 |
| http_error | HTTP ≥400，含可观察的本地拒绝 |
| stream_error | HTTP 200 正文显式 error、SSE error、response.failed/incomplete、content_filter 等 |
| unknown | 非 2xx、缺少结束、无成功终止事件、超限/坏内容、不可检查的成功正文或未识别的异常 flags |
| success | 同步：2xx、观察到完整结束、无上述错误，SSE 还必须观察到支持的终止事件；提交：独立提交请求已正常接受，非最终生成成功 |

旧指标保持响应头结算语义。新旧成功率口径不同，不应合并。source/model 的配置边界见 [插件说明](../../plugins/wasm-go/extensions/ai-route-monitor/README.md)。

## 配置与部署顺序

1. 审查并构建插件；记录 Wasm/镜像 digest。编辑 [wasmplugin.yaml](wasmplugin.yaml) 中 namespace、真实 generation ingress、模型 allowlist、实际异步提交路径和 OCI 版本。来源由每次请求普通 Header 决定，不把共享路由统一记成 wujieai。查询/轮询接口必须不匹配终态采集规则。
2. 对照真实网关镜像、已有 bootstrap override 和插件链路，审查 [helm-values.yaml](helm-values.yaml)。`gateway.modelRequestMetrics` 默认 false；true 会挂载 `higress-custom-bootstrap`，通过已有 `ISTIO_BOOTSTRAP_OVERRIDE` 机制增加独立标签。保留 SkyWalking sink。bootstrap 变更需要受控滚动重启；已有自定义 override 的部署先合并配置，不能覆盖其职责。all-in-one 不使用本 Helm chart 时，应把渲染出的 `stats_config.stats_tags` 合入其实际 bootstrap，不能只套 chart 值。
3. 保留现有 stats inclusion 配置；本 chart 默认 `global.proxy.proxyStatsMatcher.inclusionRegexps=[".*"]`。若现场限制 stats，按原规则**追加** `.*wujie_model_(requests|submissions)_total.*`，不要替换其他指标。实际宿主是否创建/暴露新指标须验证。
4. 确认网关 Pod 的 15020 端口名（chart 为 `istio-prom`）和 Prometheus Operator selector。使用 chart 自带 PodMonitor + overlay，或独立 [网关 PodMonitor](../higress-gateway-podmonitor.yaml)，**不要同时部署两个采集同一 Pod 的 Monitor**。all-in-one 示例使用每 Pod 的 targetPort 15020（需已声明 containerPort），不再把目标替换为负载均衡 Service，也不覆盖 instance。三个旧 all-in-one 文件是同一资源的替代示例，只选一个。
5. 先部署 bootstrap/采集配置并检查启动与旧指标，再灰度启用插件 `terminalMetrics`。无来源 Header 的请求仍进入 unknown，不被拒绝。请求在更早的过滤器被拒绝、未执行 monitor decode headers 时不在覆盖范围；`AUTHN/priority=100` 沿用原插件，无法单凭元数据保证它早于所有鉴权/限流插件。必须查看实际 filter chain 和拒绝测试。
6. 把 [dashboard.json](dashboard.json) 导入 Grafana，选择 `DS_PROMETHEUS` 数据源。UID `wujie-model-requests`，默认 `source=wujieai`。管理端可配置 URL：`https://GRAFANA_HOST/d/wujie-model-requests?var-source=wujieai&from=now-23h%2Fh&to=now%2Fh-1h%2B1s&timezone=Asia%2FShanghai`。仅配置已有授权可访问的 URL，不提供公开匿名访问或嵌入绕过登录。

以上均为操作说明；本任务未执行 helm upgrade、kubectl apply、重启或上线。

## 查询和时间

整体小时调用量（**每实例先 increase 再 sum**，处理进程重启）：

```promql
sum(increase(wujie_model_requests_total{source="wujieai"}[1h]))
```

整体成功率：成功序列尚不存在但有失败流量时补 0；总量为 0 或无数据时不补成 100%。

```promql
(sum(increase(wujie_model_requests_total{source="wujieai",outcome="success"}[1h]))
 or (0 * sum(increase(wujie_model_requests_total{source="wujieai"}[1h]))))
/ (sum(increase(wujie_model_requests_total{source="wujieai"}[1h])) > 0)
```

看板还包含分模型成功率/调用量、失败分类、unknown 占比及独立异步提交。默认范围 `now-23h/h → now/h-1h+1s`，timezone `Asia/Shanghai`，查询步长 1h；点 T 表示刚结束的 `(T-1h,T]` 小时。手动选择时间时保留整点；Grafana 在超长范围可能自动扩大步长，建议小时看板限制在 30 天内。

Grafana 11.3 对结束参数 `/h` 按小时末包容解析。旧范围 `now-24h/h → now/h` 实测产生 **25** 个小时桶，最后点为当前小时起点 H（并未纳入当前未完成小时），多的是左端前一小时窗口。新范围配合固定 1h step，返回恰好 H-23h 至 H 的 **24** 个端点，覆盖 `(H-24h,H]`；`to` 的实际原始边界为 H+999ms，查询最后点仍是 H。`+1s` 防止结束边界退到 H-1ms 后漏掉最近完成小时。管理端 URL 中 `+` 必须编码为 `%2B`，或省略 from/to 使用看板默认值。

Prometheus `increase` 是采样外推，可能产生小数；新序列首次采集之前的增量、短命 Pod 未被采集的请求无法恢复。它不是逐笔账本。上线先确保抓取正常并预热观察至少两次采集，不能以这些图对账扣费或承诺精确逐次总数。

## 异步最终结果与兄弟契约

`asyncSubmissionPaths` 只把提交请求归入 `wujie_model_submissions_total`，提交成功不进入同步请求成功率。任务轮询不生成最终结果计数。

appserver OPE-11002 已选择持久化小时快照 API：`TaskProductAnalyticsApi.daily(token,date)`，按唯一 `base_info` task id 和当前终态汇总，重复采集/轮询不累计。**没有 `wujie_model_tasks_total` 导出，不得查询虚构的指标。** 网关提交请求数与任务行数单位不同，不应相加。真实任务状态覆盖、重试修正和历史可用性由 `appserver-contract.json` 的 asyncModels 约定，管理端展示业务快照。未到达网关的客户端网络失败不属于本指标。

本次追加授权已在同仓库 `ai-token-report` 为原有 usage-report 添加 `productSource`，配置与 monitor 的 source 语义一致：请求头阶段快照、响应结束时序列化到原报告，异步回调不重读 Header。它仍沿用已有计费报告，不增加请求数、不改 token/金额/ID/时机/重试。旧配置保持有效并上报 unknown。接收端先发布支持该字段和账务来源迁移，再审核启用 source 配置；详见 [report 说明](../../plugins/wasm-go/extensions/ai-token-report/README.md)。monitor 仍只发宿主计数器，不按报告是否存在判断请求成功。

## 验证

本地命令（先进入仓库根目录，插件构建命令注明目录）：

```sh
# plugins/wasm-go/extensions/ai-route-monitor
go test ./... -race -count=1
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o /tmp/ai-route-monitor.wasm ./
AI_ROUTE_MONITOR_WASM=/tmp/ai-route-monitor.wasm go test ./... -run TestCompiledWasm -v -count=1
go test ./... -run '^$' -bench BenchmarkSSE -benchmem

# 仓库根目录（仅渲染，不部署）
helm lint helm/core -f monitoring/model-requests/helm-values.yaml
helm template higress helm/core -n higress-system -f monitoring/model-requests/helm-values.yaml
promtool test rules monitoring/model-requests/queries.test.yaml
```

`TestCompiledWasm` 在 SDK/Wazero **宿主模拟器**运行真实编译 Wasm，仍不是 Higress 集成。测试覆盖来源/模型/非法配置、global 和 route rule、200/429/5xx/模拟本地拒绝、跨 chunk SSE、缺失结束、超时/reset flags、只计一次、旧指标开关、异步提交隔离。原始/转换后真实协议与 filter 顺序未在该模拟器复现。

实际集群灰度验收（隔离容器验证已通过，以下发布现场检查仍待执行）：

1. 在实际版本测试网关取得 config dump，核对 monitor decode/encode 相对鉴权、限流和 ai-proxy 的顺序；核对 `stats_config.stats_tags` 保留旧 extractor。将 bootstrap JSON 交给 `verify-bootstrap.py` 检查新 regex；此脚本本身只做静态检查。
2. 用真实请求覆盖：有限来源和未知；普通 200；429/500；monitor 之后的本地拒绝；HTTP 200 SSE error；正常/跨 chunk/无结束；上游超时；下游断连；内部重试只产生一个终态。对 monitor 之前的拒绝明确记录不可见。对 ai-proxy 转换路由特别测试缺少上游终止事件，因为现有转换器可能合成 `response.completed/message_stop/[DONE]`，造成无法从转换后内容判断上游完整性的边界。
3. 每个 Pod 各自抓取 `/stats/prometheus`，保存真实样本（不要保存业务响应/凭据），运行 `python3 monitoring/model-requests/verify-export.py CAPTURE.txt`；记录真实导出名、有限标签和终态增量。脚本不负责替代请求结果断言。若标签未被抽取或名称仍含维度，不可验收。
4. 在 Prometheus targets 确认每 Pod 一个目标、地址/instance 独立；确认规范名和标签。可抓取 `/federate` 的文本结果后用 `verify-export.py --canonical` 检查。用 PromQL 分 Pod 查看 reset 并核对汇总；在测试实例滚动重启后验证无跨实例混算，不对生产执行破坏性测试。
5. 实际执行看板查询并导入 Grafana 检查整点、0%、无数据及分模型显示。比较开关前后真实网关 RSS、CPU、p95/p99/首 token 延迟和流输出；若出现可察觉退化则关闭新指标并回滚，不能用原生 benchmark 宣称网关性能达标。

## 回滚

先将灰度规则 `terminalMetrics=false`，保留 `legacyMetrics=true`，停止新的终态计数并恢复旧成功响应不读正文路径。需要完整版本回滚时恢复上一插件镜像 digest。旧数据在 Prometheus 中按保留期存在，关闭后不能把停止增长误读成真实无流量。

确认旧指标采集正常后再移除新看板/采集 relabel；关闭 `gateway.modelRequestMetrics` 并按正常发布流程滚动恢复 bootstrap。SkyWalking 启用时仍保留其原 override 和 sink。仅修改 ConfigMap 不会让运行中的 Envoy 重新装载 stats tags。

### 仅 Mac 打包机运行验证

按用户最新要求，不在本机起测试容器。远程测试使用 `wujieai-mac-pkg-34` 的 `/opt/homebrew/bin/docker --context colima-ope10117`，先等待共享 `remote-container-slot.json` 由前一个任务标记 completed/released、确认其测试容器停止后再认领。不得操作其他任务容器或全局 Docker 配置。

`runtime/` 提供小规模顺序测试脚本。把同一提交构建的 `ai-route-monitor.wasm`、`ai-token-report.wasm`、`ai-proxy.wasm`、`model-mapper.wasm`、`request-block.wasm`，以及 Helm 渲染的 bootstrap JSON（命名为 `bootstrap-tags.json`）放入远程任务目录 `/Users/wujieai/ope9626-validation/higress/RUN/runtime/`。目录内运行 `OPE9626_RUNTIME_SLOT=OPE-11003 bash run-remote.sh`。脚本默认创建独立命名 gateway/mock 和私有网络；`OPE9626_PROMETHEUS=1` 增加独立 Prometheus，gateway 配额同步调低；总限额 2 CPU / 1920 MiB，仅绑定 `127.0.0.1` 动态端口，退出保留 logs/config dump/stats/reports/results 并清理自身容器与网络，不清理镜像。

脚本比较真实终态增量、early/observable reject、一次重试一个分母、源 Header 后续改写和现有报告字段。特别对 `/v1/responses` 缺失上游 `[DONE]` 的转换路径要求 unknown；若失败，结果明确保留为失败，不能因为收到合成 `response.completed` 标为通过。该脚本不能替代生产集群的 PodMonitor/Grafana、容量或真实请求链路验证。


启用 Prometheus 验证时，同目录还需 `dashboard.json` 和 `verify-export.py`；`metric-relabelings.json` 已从仓库 PodMonitor 原样转换（`sourceLabels → source_labels`、`targetLabel → target_label`）。PodMonitor 规则修改时重新导出该 fixture。此隔离 fixture 抓取 Envoy admin 9901，不覆盖集群 pilot-agent 15020 合并与 Operator 发现。验证脚本只使用合成请求和报告。镜像可用 `OPE9626_GATEWAY_IMAGE`、`OPE9626_MOCK_IMAGE`、`OPE9626_PROMETHEUS_IMAGE` 指定已审核的远程版本/镜像代理；不更改 daemon 的全局 registry 设置。

`OPE9626_TERMINAL_METRICS=0 OPE9626_BENCH_ONLY=1` 可串行运行相同小样本基线，仅用于观察明显回归，不作为容量验收。退出清理自己的三种固定名称容器；运行前若任意名称已存在则拒绝启动，不删除已有容器。运行结果摘要见 [runtime-evidence.json](runtime-evidence.json)。
