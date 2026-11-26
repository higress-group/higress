# 验证记录

初轮日期：2026-11-04（Asia/Shanghai）；2026-11-05 浏览器补验见文末。OPE-11003 / 父任务 OPE-9626。工作目录 `/private/tmp/ope-9626-higress`，分支 `codex/ope-9626-product-analytics`，基线 `986ee824`。所有修改留在该 worktree；完成验证后按授权本地提交，实际 commit 见任务记录。未推送、部署或发布产物。

追加授权：补同仓库 ai-token-report 最小来源生产端，并允许完成验证后本地提交。容器运行测试按用户最新要求仅在 Mac 打包机、共享时段串行执行，不在本机测试容器。

方案比较：新插件可以隔离新旧语义，但增加部署与顺序维护；按已批准方案扩展 ai-route-monitor，保留旧路径，新增默认关闭的终态指标。未引入签名、票据或应用身份证明。

## 已通过

| 检查 | 结果与边界 |
|---|---|
| `go test ./... -race -count=1` | 通过。来源与未知、模型归一化、非法配置、route rule、普通成功、429/500、模拟本地拒绝、HTTP 200 JSON/SSE error、逐字节 chunk、所有切分点、CRLF/多行、正常/缺失结束、timeout/reset、重复完成回调、旧开关与旧错误体指标、异步提交隔离 |
| `go vet ./...` | 通过 |
| Go `wasip1/wasm -buildmode=c-shared` | 构建通过，产物 `/private/tmp/ope-9626-ai-route-monitor.wasm`，SHA256 `0fc0ae0be4d117a79b766d03f1872aafa5d4f6952edf3db1528c30417995b1f7` |
| `AI_ROUTE_MONITOR_WASM=… go test … -run TestCompiledWasm -v` | 真正编译 Wasm 在 SDK/Wazero **模拟宿主**执行 lifecycle/rules/legacy/submission，通过；不是运行 Higress，日志 `/private/tmp/ope-9626-wasm-tests.log` |
| Helm 3.16.4 lint / template | 指标 overlay 渲染与 lint 通过；默认渲染与 HEAD 基线逐字节相同；开启 SkyWalking 时 sink 与新标签同时存在，bootstrap JSON 可解析 |
| monitoring YAML / Python | 修改和新增 YAML 可解析；验证脚本语法检查通过 |
| `verify-bootstrap.py` | 对渲染 bootstrap 静态测试 source/model/outcome capture/removal；新 regex 不匹配旧指标名。另有真实 Higress tag extraction 和 Prometheus relabel 验证通过（见下） |
| Prometheus 2.55.1 `promtool test rules` | `queries.test.yaml` 全部通过：多实例含 reset 的 increase 再 sum、unknown 进入分母、全部失败为 0%、零/缺失流量为无数据 |
| 看板实际 7 条 expr | 从 dashboard.json 提取并替换 source/model 变量，通过 promtool 解析及空输入无数据断言；未连接实际 Grafana |
| `git diff --check` | 通过 |
| ai-token-report 原生 + 编译 Wasm | `WASM_FILE_PATH=/private/tmp/ope-9626-ai-token-report.wasm go test ./... -race -count=1` 通过；`go vet ./...` 通过。来源默认/命中/未知/自定义 Header/fallback，JSON 和 SSE，在后续 Header/host/path 改写后仍持有原快照；检查旧 token、model/userKey/requestId/taskId 和单次结束时机不变。Wasm SHA256 `59d36efaa6d54821910ac5f57b3b1b146752cd8b99eb58b20b9cb53f86c918da` |

原生 benchmark（Apple M2，darwin/arm64，每次请求解析 100 个 SSE 事件）：53,025 ns/op，19,392 B/op，307 allocs/op。响应内容不按总长度积累，当前行和事件各限制 16 KiB 数据。数字包含 Go 临时分配，**不代表 Wasm/网关 RSS、CPU、p95/p99 或首 token 延迟**，没有实测网关性能达标结论。

## 远程真实运行验证

仅在 `wujieai-mac-pkg-34`，Docker context `colima-ope10117` 串行运行。Higress `gateway:2.1.9` arm64，镜像 digest `sha256:d80b4842303ecd082716fbe598ab7744f495a0b8445fa51cab2d6703d32101d3`。五个真实 Wasm：本次 monitor/report，加未修改的 ai-proxy/model-mapper/request-block。合成提供方与报告接收端，无真实凭据/用户数据。

- **21 项断言全部通过**：普通 JSON、有限/缺失/未知来源、429/5xx、正常/显式错误/缺失结束 SSE、超时、上游 reset、下游断连、重试两次上游但只计一次请求；monitor 后的本地拒绝为 http_error，monitor 前拒绝没有指标；异步提交独立计数；JSON/SSE 原计费字段与来源快照保持，后续改写 Header 不影响报告。
- 当前 deepseek → Responses 转换正常结束为 success；chunked 和 content-length 两种缺失上游 `[DONE]` 均为 unknown。没有修改转换器。该结果不能推广到全部提供方/转换链路。
- 实际 `/stats/prometheus` 导出 `wujie_model_requests_total` / `wujie_model_submissions_total`，临时标签为 `wujie_source/wujie_model/wujie_outcome`，本镜像无额外名称前缀。`verify-export.py` 对实际文本通过。
- **真实 Prometheus 2.55.1** 抓取 Envoy admin 9901；PodMonitor 同一组 metric relabel 规则映射为 `source/model/outcome`，保留 instance/pod/namespace/job。target 为 up，`verify-export.py --canonical` 验证实际 federation 通过；7 条看板实际表达式替换来源/模型变量后全部返回数据。1s 抓取仅用于 fixture，生产示例为 30s。
- 小样本串行对比：每类 3 次预热、20 次测量，terminalMetrics 开/关。JSON p50 52.70/52.55ms、p95 59.82/59.04ms；SSE p50 62.51/62.57ms、p95 65.62/65.73ms。gateway 结束时内存采样 492.9/494.8MiB。开/关配额相同（gateway 1.75CPU/1792MiB + mock 0.25CPU/128MiB）；两轮负载量不同，资源采样不是峰值，不能据此量化内存差异。未观察到明显小样本延迟退化，**没有容量、启动、p99 或首 token 性能达标结论**。
- Prometheus 轮总配额仍为 2CPU/1920MiB（gateway 1.5CPU/1536MiB、mock 0.25CPU/128MiB、Prometheus 0.25CPU/256MiB）。所有端口仅 loopback 动态绑定。测试结束已删除自身三个容器和私有网络，确认仅原有 `ope10762-real-squad-db` 留存；未操作该容器。共享时段已释放。

可提交结果摘要：[runtime-evidence.json](runtime-evidence.json)。完整本地捕获在 `/private/tmp/ope-9626-higress-results-{second,prometheus,baseline}/runtime`；远程归档为 `/Users/wujieai/ope9626-validation/higress/20261104-ope11003/results-{second,prometheus,baseline}.tar.gz`。

兄弟 appserver 的 `appserver-contract.json.integrationBoundaryValidation` 另报告：真实 Go 生产端生成 JSON 经 MockMvc、usage service、序列化 MQ、account consumer 与扣费服务的 8 种边界场景通过，14 个聚焦 Java 测试类共 80 测试无失败/跳过；显式 JSON 来源优先、缺字段 Header fallback、未知与重复 requestId 不重复扣费。该证据属于 appserver，持久化/Redis/Kafka 为测试替身，不能称为部署联调。

## 未执行 / 发布现场验收

- 本机没有创建或启动测试容器；此前中止的本机镜像拉取可能留有缓存层，见共享 `higress-local-runtime.json`。
- 未检查实际部署 filter chain；已证明仅请求头回调执行后的拒绝可观察。未验证其他提供方或转换器组合。
- 未验证 pilot-agent 15020 合并、Kubernetes PodMonitor Operator 发现、真实多 Pod 重启、正式 Grafana 环境的接入。隔离 Grafana 导入/显示已在 2026-11-05 补验通过。fixture 通过不代表这些现场步骤完成。
- 未进行容量、启动、p99/首 token 延迟测试；未与实际客户端、任务服务及账务接收端/Kafka/数据库运行联调。最终异步结果来自 appserver 持久化小时快照 API，没有 `wujie_model_tasks_total`。

## 行为与发布影响

- `terminalMetrics=false` / `legacyMetrics=true` 为默认值。启用后只新增监控，不拒绝来源未知的业务请求。
- 新配置有限 allowlist：source 最多 16、model 最多 128，加各自 fallback；标签归一化碰撞合并。未完成、超限/不可检查的成功正文进入 unknown，并计入同步分母；不会按 HTTP 200 一概计为流式成功。
- opt-in bootstrap tags 需要受控网关重启；现有自定义 override 必须合并审查。样例 PodMonitor 移除固定 instance/Service 地址替换和仅内存过滤，保留旧指标及各 Pod 身份；避免与 chart Monitor 重复采集。
- 待审核插件镜像、真实 ingress/模型/异步提交路径、采集配置和 Grafana datasource/管理端 URL。未发布 OCI、Maven 私服包，未执行线上配置或 DDL，未合并。
- ai-token-report 新增 camelCase `productSource`，旧配置仍有效并输出 unknown；启用配置后输出有限归一化来源。旧计费字段、金额、上报次数时机和超时重试不改。接收端字段与账务来源迁移需先按其正常流程发布；两个插件的 source 配置需一致。

OPE-11003 保持 `in_progress`，本地开发及上述隔离运行验证完成；未将发布现场验收标为完成。完整灰度、采集检查和回滚步骤见 [README](README.md)。

## 2026-11-05 Grafana 真实浏览器补验

远程同一打包机完成真实 Grafana11.3 导入、数据源映射和 Native Chrome156 登录/查询/截图。50 项浏览器断言、8 次实时网关探测、7 条 Prometheus 查询、3 次定向断连均通过；完整画面已人工视觉核对。发现并修复默认范围包容左端导致25个桶的问题，最终恰好24个已完成小时，未漏掉最新完成小时。监控插件生产代码不变。初轮失败、截图捕获修正、历史fixture与实时链路的边界和资源清理详见 [补验报告](grafana-validation/README.md)。
