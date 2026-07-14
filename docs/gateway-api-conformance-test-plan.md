# Higress Gateway API v1.4.0 Conformance 落地与官方申报方案

> 调研与验证日期：2026-07-13 至 2026-07-14<br>
> 当前代码基线：`sigs.k8s.io/gateway-api v1.4.0`<br>
> 首期目标：把 Gateway API v1.4.0 官方 Conformance Suite 作为 Higress 默认 PR 集成测试，跑通 `GATEWAY-HTTP` Core，并复用同一流程生成官方报告。

## 1. 结论

Higress 可以并且应该先以当前真实支持的 `v1.4.0` 为目标，不需要为了接入 Conformance 先升级 Gateway API。

正确的交付链是：

```text
固定 v1.4.0 Go module 和 CRD
  -> 每个 PR 在现有 Build and Test workflow 中部署当前提交
  -> 与现有 Higress e2e、Wasm e2e 并列运行官方 Suite
  -> 修复 GATEWAY-HTTP Core 失败项
  -> 从 GatewayClass.status.supportedFeatures 声明真实能力
  -> 每次 PR 生成报告 artifact 并阻断 Gateway API 回归
  -> release tag 复用同一 Make target 生成申报报告
  -> 提交 Gateway API 官方 report PR
  -> report PR 合并后再提交 implementations 页面 PR
```

这不是一套独立的“报告环境”。它是 Higress 默认 CI/CD 流水线中的常规集成测试 job，命名为 `higress-gateway-api-test`，与现有 `higress-conformance-test` 和 `higress-wasmplugin-test` 同级。每个 job 按现有模式创建自己的临时 kind，测试结束即清理，不引入长期集群或人工执行环境。

首期不复制、改写或重新实现官方测试用例。Higress 只提供并列的测试入口、临时环境准备、网络接入和 CI；测试定义及 manifests 全部直接使用 `sigs.k8s.io/gateway-api v1.4.0` 模块中嵌入的官方内容。

需要同时明确两个官方结果：

- v1.4.0 报告被接受后，Higress 可以进入官方 **v1.4 实现能力表**；该表只展示 Core 通过且无 skip 的报告。
- 当前最新 release 已高于 v1.4。按官方“最近两个 release 才是 Conformant、最近三个 release 可为 Partially Conformant”的规则，v1.4 报告不能长期代表最新 Conformant 身份。它仍是进入官方报告体系和申请实现列表的有效第一步，后续需要再提交 v1.5 或 v1.6 报告。

## 2. 官方规则与首期范围

### 2.1 官方规则

官方当前要求：

1. 至少选择一个 Conformance Profile 和该 Profile 中一种 Route。
2. 该组合的全部 Core tests 必须通过。
3. 所有声明支持的 Extended Features 对应测试也必须通过。
4. 正式成功报告不能包含失败或 skip；开发阶段可以使用 `--run-test` 定位，但不能用于最终报告。
5. 报告必须由 Suite 原样生成，不允许手工编辑。
6. `implementation.version` 必须指向可复现的 tag、release 或 commit，不能写分支名。
7. 官方 report PR 合并后，才具备申请加入 implementations 页面对应分类的前提。

依据：

- [Gateway API v1.4.0 release](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.4.0)
- [Conformance 运行说明](https://gateway-api.sigs.k8s.io/docs/concepts/conformance/)
- [Implementer's Guide](https://gateway-api.sigs.k8s.io/guides/implementers-guide/)
- [Conformance Report 提交规则](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/reports/README.md)
- [v1.4 实现能力表](https://gateway-api.sigs.k8s.io/docs/implementations/versions/v1.4/)
- [Implementations 列表与新增规则](https://gateway-api.sigs.k8s.io/docs/implementations/list/)

### 2.2 首期范围

| 项目 | 决策 |
| --- | --- |
| Gateway API | `v1.4.0`，不升级 `go.mod` |
| CRD channel | `standard` |
| Profile | `GATEWAY-HTTP` |
| Core features | `Gateway`、`HTTPRoute`、`ReferenceGrant` |
| Extended features | 首份报告不主动声明；通过后逐项加入 |
| GatewayClass | `higress` |
| Controller name | `higress.io/gateway-controller` |
| Mode | `default` |
| PR 执行 | `.github/workflows/build-and-test.yaml` 每次 `pull_request` 默认执行 |
| 测试位置 | `test/gateway/`，与现有 `test/e2e/` 并列 |
| 官方目录 | `conformance/reports/v1.4.0/alibaba-higress/` |

选择 `GATEWAY-HTTP` 是满足官方报告要求的最小完整范围，也与 Higress 当前北南向 HTTP API Gateway 定位一致。不要因为代码继承自 Istio 就同时声明 `MESH-HTTP`、`GATEWAY-GRPC` 或 `GATEWAY-TLS`。

Gateway API v1.4.0 已接受的 Istio 报告中，`GATEWAY-HTTP` Core 为 33 个测试全部通过，可将 `Passed: 33 / Failed: 0 / Skipped: 0` 作为本版本的结果核对基准，但最终以 Higress 实际生成的报告为准。

## 3. 当前仓库现状

### 3.1 已具备

- `go.mod` 已固定 `sigs.k8s.io/gateway-api v1.4.0`；官方 runner 直接复用该模块，`go mod tidy` 只补齐其日志适配器的间接依赖。
- Helm 默认配置：`global.enableGatewayAPI=true`、`global.gatewayClass=higress`。
- Controller name 已固定为 `higress.io/gateway-controller`。
- 控制面已有 GatewayClass、Gateway、HTTPRoute、ReferenceGrant、status 和数据面转换代码。
- `test/e2e/` 已有 kind 创建、controller 镜像构建/加载、Helm 安装和清理流程。
- kind 配置已把节点的 80/443 端口映射到宿主机，`global.local=true` 时 Higress gateway Pod 也使用 80/443 `hostPort`。
- `.github/workflows/build-and-test.yaml` 原先预留了空的 Gateway API job，现已重命名并实现为 `higress-gateway-api-test`。
- 该 workflow 已对所有目标分支的 `pull_request` 触发，因此补全 job 后无需新增 workflow 或额外触发器。

### 3.2 实施前缺口

1. `Makefile.core.mk` 原有的空 `gateway-conformance-test` target 已替换为 `higress-gateway-api-test`。
2. CI 的同名 job 只有 checkout，是“空成功”。
3. `test/gateway/e2e.go` 和 `test/gateway/e2e_test.go` 没有测试逻辑。
4. 测试环境没有显式安装官方 v1.4.0 Standard CRD。
5. `GatewayClass.status.supportedFeatures` 为空；v1.4 Suite 在未传手工 features 时会自动从该字段推断，并因空集合失败。
6. `pkg/ingress/kube/gateway/istio/supported_features.go` 为 `features.AllFeatures`，但既没有反映 Higress 实测能力，也没有写入 GatewayClass status。
7. 通用 kind 环境固定 Kubernetes v1.25.3，已超出 Gateway API v1.4.0 的支持窗口。
8. kind 中 LoadBalancer Service 没有真正的云负载均衡地址，Higress 会保持地址 pending 且不写入 `Gateway.status.addresses`；测试安装必须改用 ClusterIP，随后由网络适配把 status 中的内部 hostname 映射到宿主机入口。
9. CI 没有保存标准报告或失败时的 Gateway/Route status、事件和组件日志。

## 4. 关键技术决策

### 4.1 默认 PR 门禁，而不是独立报告任务

`higress-gateway-api-test` 必须放在现有 `.github/workflows/build-and-test.yaml` 中，不新建只供人工运行的申报 workflow。它与 `higress-conformance-test` 一样依赖 `build`，在每个 PR 中默认执行：

```text
lint + coverage
        |
       build
        |-------------------------------|
        |                               |
higress-conformance-test      higress-gateway-api-test
                                        |
                              v1.4.0 GATEWAY-HTTP Core
```

Wasm e2e 当前位于插件测试 workflow，可继续保持其现有触发范围；从仓库组织和 Make target 角度，三者是并列测试族：

- `test/e2e/` + `make higress-conformance-test`；
- `test/e2e/` + `make higress-wasmplugin-test`；
- `test/gateway/` + `make higress-gateway-api-test`。

默认 PR job 的成功条件就是完整 GATEWAY-HTTP Core 通过。它不能以“只生成报告”“允许失败”或“仅定时运行”替代 PR 门禁。仓库启用 required checks 后，应将 `higress-gateway-api-test` 列为保护分支必需检查。

### 4.2 使用官方入口，不维护自定义测试清单

测试入口使用官方公开 API：

```go
func TestGatewayAPIConformance(t *testing.T) {
    opts := conformance.DefaultOptions(t)
    // 注入 Higress kind 环境所需的 dialer，见下一节。
    conformance.RunConformanceWithOptions(t, opts)
}
```

`DefaultOptions` 会完成以下工作：

- 从 kubeconfig 创建 Kubernetes clients；
- 读取官方 flags；
- 加载 v1.4.0 官方嵌入式 manifests 和测试清单；
- 检查已安装 CRD 的 `bundle-version` 和 `channel`；
- 执行 Suite Setup/Run；
- 生成标准 `ConformanceReport`。

Higress 不应在 `test/e2e/conformance/tests/` 中复制官方 `<test>.go + <test>.yaml`。该目录继续服务 Higress Ingress、插件和扩展行为测试。

### 4.3 kind 网络使用官方 RoundTripper 扩展点

官方 Suite 从 Gateway status 读取地址并构造请求。Higress kind 环境的实际可达入口是宿主机 `127.0.0.1:80/443`，原因是：

- kind 将宿主机 80/443 映射到 node container；
- `global.local=true` 时 Higress gateway Pod 使用相同端口的 `hostPort`；
- LoadBalancer Service 在 kind 中通常没有可从宿主机访问的外部地址。

因此 kind CI 中的测试入口应调用 `conformance.DefaultOptions(t)` 后，为官方 `roundtripper.DefaultRoundTripper` 设置 `CustomDialContext`：保留 Suite 构造的 URL、Host、TLS SNI 和目标端口，只把实际 TCP 连接地址改为 `127.0.0.1:<原端口>`。该适配由 `HIGRESS_GATEWAY_API_TEST_DIAL_LOCALHOST=true` 显式启用；在可直接访问 Gateway status 地址的云上 Kubernetes 中关闭，不改变测试 runner。

这不是修改测试期望，而是使用官方明确提供的 RoundTripper 注入接口解决测试执行环境的网络边界。正式复现文档必须说明这一点。

示意代码：

```go
opts := conformance.DefaultOptions(t)
if os.Getenv("HIGRESS_GATEWAY_API_TEST_DIAL_LOCALHOST") == "true" {
    opts.RoundTripper = &roundtripper.DefaultRoundTripper{
        Debug:         opts.Debug,
        TimeoutConfig: opts.TimeoutConfig,
        CustomDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
            _, port, err := net.SplitHostPort(address)
            if err != nil {
                return nil, err
            }
            return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
        },
    }
}
conformance.RunConformanceWithOptions(t, opts)
```

首期只声明 GATEWAY-HTTP Core，不包含使用 H2C prior knowledge 的 Extended 测试；后续扩展 H2C 时需要同时确认该协议路径是否也使用自定义 dialer。

### 4.4 能力声明分两步

首次摸底和最终报告不能混为一谈。

开发调试阶段：

- 临时传入 `--supported-features=Gateway,HTTPRoute,ReferenceGrant`；
- 目的是绕过当前空的 `GatewayClass.status.supportedFeatures`，尽快得到 33 个 Core 测试的真实失败清单；
- 允许用 `--run-test=<ShortName>` 单独复测失败项；
- 这些参数只能由开发者本地显式传入，不能成为 PR job 默认参数。

默认 PR 与正式报告阶段：

- 修复 `GatewayClass.status.supportedFeatures`，至少准确写入 `Gateway`、`HTTPRoute`、`ReferenceGrant`；
- 删除 `--supported-features`，让 Suite 从 GatewayClass status 自动推断；
- 不使用 `--all-features`、`--skip-tests`、`--exempt-features`、`--allow-crds-mismatch`；
- 只有已经通过官方测试的 Extended Feature 才能加入 status。

这样 GatewayClass 对用户的能力声明、Suite 的测试选择和报告内容来自同一事实源。

### 4.5 一套测试入口，两种报告元数据

PR 回归和官方申报必须调用同一个 `make higress-gateway-api-test`，不能维护两套 runner 或两套环境。差异只在 `implementation.version`：

- PR：使用 `${GITHUB_SHA}`，它是不可变 commit；报告作为 CI artifact 保存，不提交上游。
- 正式申报：checkout Higress release tag，并把同一个变量设为该语义化版本；生成的报告原样提交上游。

这样每个 PR 验证的就是将来生成官方报告的实际代码路径，避免“日常 CI 一套、申报时临时搭另一套”。

### 4.6 版本必须三处一致

以下三处必须全部为 `v1.4.0`：

1. `go.mod` 中 `sigs.k8s.io/gateway-api`；
2. 集群安装的 CRD bundle；
3. Suite 生成报告中的 `gatewayAPIVersion`。

正式流程禁止 `--allow-crds-mismatch`。如果三者不一致，应直接失败，而不是生成 `UNDEFINED` 报告。

## 5. 文件级实施方案

### 5.1 `test/gateway/e2e_test.go`

保留现有文件，新增：

- `TestGatewayAPIConformance`；
- 调用 `conformance.DefaultOptions`；
- kind CI 中按环境变量注入 localhost dialer，云上集群使用官方默认网络路径；
- 调用 `conformance.RunConformanceWithOptions`。

`test/gateway/e2e.go` 如果没有共享逻辑可以删除；不要为了两个空文件保留无意义抽象。

### 5.2 `pkg/ingress/kube/gateway/istio/supported_features.go`

把 `features.AllFeatures` 替换为 Higress 经过验证的显式集合。首期只以 GATEWAY-HTTP Core 为承诺基线；Extended 集合在测试通过后逐项增加。

### 5.3 `pkg/ingress/kube/gateway/istio/gatewayclass.go`

`GetClassStatus` 除 Accepted condition 外，还需要：

- 将显式能力集合转换为 `[]gatewayv1.SupportedFeature`；
- 按名称稳定排序，避免 status 每次 reconcile 抖动；
- 写入 `existing.SupportedFeatures`；
- 增加单元测试验证默认类、定制类和稳定排序。

### 5.4 测试部署 namespace

官方基础 Gateway 与同 namespace TLS Secret 固定创建在 `gateway-conformance-infra`。测试专用 Helm 安装也使用这个 namespace，使 Higress controller、pilot、gateway Pod、gateway Service、Gateway 与基础 Secret 位于同一信任边界。这样可以直接沿用 Higress 现有的同 namespace Service 发现和 Istio SDS 约束，不需要为了测试放宽跨 namespace 工作负载选择或证书访问逻辑。

官方仍有少量 ReferenceGrant 用例刻意把 Secret 或 backend Service 放在 `gateway-conformance-web-backend`；这些资源不能搬迁，否则就不再是官方原始用例。它们的授权与状态仍由 `higress-core` 按 ReferenceGrant 处理。

### 5.5 `Makefile.core.mk`

增加变量：

```make
GATEWAY_API_VERSION ?= v1.4.0
GATEWAY_CONFORMANCE_PROFILE ?= GATEWAY-HTTP
GATEWAY_CONFORMANCE_REPORT ?= out/gateway-api-conformance/report.yaml
HIGRESS_CONFORMANCE_VERSION ?= $(shell git rev-parse HEAD)
GATEWAY_CONFORMANCE_CONTACT ?= https://github.com/alibaba/higress/issues
GATEWAY_API_TEST_NAMESPACE ?= gateway-conformance-infra
GATEWAY_API_GATEWAY_SERVICE_TYPE ?= ClusterIP
GATEWAY_API_KIND_NODE_TAG ?= v1.34.0@sha256:7416a61b42b1662ca6ca89f02028ac133a309a2a30ba309614e8ec94d976dc5a
```

默认值让普通 PR 无需额外参数即可执行并生成合法元数据。正式申报时必须显式将 `HIGRESS_CONFORMANCE_VERSION` 覆盖为被测试的 Higress 语义化 release tag。`GATEWAY_CONFORMANCE_CONTACT` 使用稳定的公开 issue 地址，维护者也可以在申报前覆盖为确定的 GitHub 团队。

增加 targets：

- `create-gateway-api-cluster` / `delete-gateway-api-cluster`：使用独立的 kind v0.30.0 和固定 digest 的 Kubernetes v1.34.0 node；不改变现有 Ingress/Wasm 流程使用的 kind v0.17.0 / Kubernetes v1.25.3。
- `install-gateway-api-crds`：安装并等待 v1.4.0 Standard CRD Established。
- `install-dev-gateway-api`：沿用现有开发镜像安装参数，将整套 Higress 安装到 `gateway-conformance-infra`；kind 默认把 gateway Service 设为 ClusterIP，ACK 验证可覆盖为 LoadBalancer。controller 使用当前提交构建的 `TAG`，pilot 和 gateway 分别使用仓库统一的 `ISTIO_LATEST_IMAGE_TAG`、`ENVOY_LATEST_IMAGE_TAG`。
- `higress-gateway-api-test-prepare`：delete/create kind、安装 CRD、构建/加载当前提交的 Higress controller、加载仓库统一的 pilot/gateway 镜像、Helm install、等待 workloads 和 GatewayClass Accepted。Gateway API job 不维护自己的数据面 tag；若改动涉及 pilot，先构建并推送对应 submodule SHA 的 pilot 镜像，再统一更新 `ISTIO_LATEST_IMAGE_TAG`，让现有 e2e 与 Gateway API e2e 使用同一产物。
- `run-higress-gateway-api-test`：执行完整官方 GATEWAY-HTTP Core 并生成报告；这是 PR 默认路径。
- `higress-gateway-api-test-clean`：删除 kind。
- `higress-gateway-api-test`：串联 prepare/run；失败时由 CI `always()` 收集诊断后清理。

开发者需要单项调试时，通过可选 `GATEWAY_CONFORMANCE_RUN_TEST` 传给官方 `--run-test`，而不是维护第二个测试 target。该变量在 CI 中默认必须为空。

CRD 安装命令固定为：

```bash
kubectl apply --server-side=true \
  -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.0/standard-install.yaml
```

安装后至少等待：

```bash
kubectl wait --for=condition=Established crd/gatewayclasses.gateway.networking.k8s.io --timeout=120s
kubectl wait --for=condition=Established crd/gateways.gateway.networking.k8s.io --timeout=120s
kubectl wait --for=condition=Established crd/httproutes.gateway.networking.k8s.io --timeout=120s
kubectl wait --for=condition=Established crd/referencegrants.gateway.networking.k8s.io --timeout=120s
```

### 5.6 PR 与正式报告共用的运行命令

测试代码只保留 runner 和网络适配，所有申报元数据通过官方 flags 传入：

```bash
mkdir -p out/gateway-api-conformance
go test -v ./test/gateway \
  -run '^TestGatewayAPIConformance$' \
  -args \
  --gateway-class=higress \
  --conformance-profiles=GATEWAY-HTTP \
  --organization=alibaba \
  --project=higress \
  --url=https://github.com/alibaba/higress \
  --version="${HIGRESS_CONFORMANCE_VERSION}" \
  --contact="${GATEWAY_CONFORMANCE_CONTACT}" \
  --mode=default \
  --cleanup-base-resources=false \
  --report-output=out/gateway-api-conformance/report.yaml
```

必须关闭 Suite 自己的基础资源清理：Higress 与基础 Gateway 共用 `gateway-conformance-infra`，如果让 Suite 删除基础 namespace，会同时删除正在被测的 Higress。CI 在上传报告和诊断后直接删除整个临时 kind 集群，不会遗留资源。

PR 默认命令和正式申报命令中都故意没有：

- `--supported-features`：从 GatewayClass status 推断；
- `--skip-tests` 或 `--run-test`：避免 skip；
- `--all-features`：避免虚假声明全部 Extended；
- `--allow-crds-mismatch`：保证报告版本可信。

### 5.7 `.github/workflows/build-and-test.yaml`

把空 job 改为真实门禁：

1. 继续使用现有 `pull_request` 触发器，不添加 paths filter、定时限制或手工触发限制；
2. checkout 当前 PR commit，包括构建所需 submodules；
3. 配置与现有 e2e 一致的 Go、Docker 和磁盘空间；
4. 执行 `make higress-gateway-api-test HIGRESS_CONFORMANCE_VERSION=${{ github.event.pull_request.head.sha || github.sha }}`；
5. 测试失败则 job 失败，直接阻断 PR；
6. `if: always()` 收集：
   - `out/gateway-api-conformance/report.yaml`；
   - `kubectl get gatewayclass,gateway,httproute,referencegrant -A -o yaml`；
   - `kubectl get events -A --sort-by=.lastTimestamp`；
   - Higress controller、pilot、gateway logs；
   - kind export logs；
7. 上传 artifacts；
8. 收集完成后清理 kind。

PR 和 main CI 都使用当前 commit SHA 作为内部报告版本并保存 artifact。正式申报不创建新 workflow 或新环境：checkout Higress release tag 后，执行完全相同的 Make target，并将版本参数覆盖为 release tag。

### 5.8 测试文档

更新 `test/README.md` 和 `test/README_CN.md`，明确区分：

- `higress-conformance-test`：Higress 自有 Ingress/插件 e2e；
- `higress-gateway-api-test`：基于 Gateway API 官方 Suite 的 Higress Gateway API 集成测试；
- 默认模式：每个 PR 跑完整 Core、零 skip、生成 commit 报告 artifact；
- 调试模式：开发者显式指定一个官方 ShortName，仅用于本地定位；
- 申报模式：同一个 target 在 release tag 上执行，生成可提交报告。

## 6. 执行阶段与验收

### 阶段 A：接入默认 PR 流水线并跑出真实失败清单

1. 实现官方 runner 和仅供 kind CI 启用的 localhost dialer。
2. 实现 CRD/prepare/run/clean targets。
3. 将现有空 job 重命名并补全为 `higress-gateway-api-test`，使所有 PR 默认运行完整 `GATEWAY-HTTP`。
4. 第一次实现期间可以在开发分支手工传 Core features获得失败清单，但合入前要完成 GatewayClass status 推断。
5. 保存失败测试名、Gateway/Route status、events 和日志。

验收：`higress-gateway-api-test` 不再空成功，所有 PR 默认执行，失败会使该 workflow 失败，并且能稳定得到 33 个 Core 测试的结果。

### 阶段 B：按规范修复 Core

按以下类别归档失败，不为测试名称写特判：

1. GatewayClass Accepted、observedGeneration 和 supportedFeatures。
2. Gateway Accepted/Programmed、listeners、attachedRoutes 和 addresses。
3. HTTPRoute parentRef、sectionName、hostname 交集和 listener 绑定。
4. backendRef、ReferenceGrant、跨 namespace 和无效引用。
5. path/header/query/method 基础匹配、权重与优先级。
6. 无匹配、无后端及无效后端要求的响应码。
7. 资源更新后的状态与数据面最终一致性。

单项调试：

```bash
go test -v ./test/gateway \
  -run '^TestGatewayAPIConformance$' \
  -args --gateway-class=higress \
  --conformance-profiles=GATEWAY-HTTP \
  --supported-features=Gateway,HTTPRoute,ReferenceGrant \
  --run-test=HTTPRouteSimpleSameNamespace
```

单项通过后必须回归完整 Profile。

阶段验收：完整 Core `Failed: 0`、`Skipped: 0`，并由默认 PR job 在 amd64 runner 上复现通过。

### 阶段 C：稳定默认 CI，并从同一流程生成正式报告

1. 根据实测结果完善 `GatewayClass.status.supportedFeatures`。
2. 移除运行命令中的手工 supported features。
3. 确认普通 PR 不需要额外参数即可运行完整 Profile。
4. checkout 一个不可变 Higress release tag。
5. 执行与 PR 完全相同的 `make higress-gateway-api-test`，只覆盖 implementation version。
6. 不修改生成的 YAML。

报告必须确认：

- `gatewayAPIVersion: v1.4.0`；
- `gatewayAPIChannel: standard`；
- `implementation.organization: alibaba`；
- `implementation.project: higress`；
- `implementation.version` 与被测试的 Higress release 完全一致；
- `mode: default`；
- 只有 `GATEWAY-HTTP` profile；
- Core `result: success`；
- Core `Failed: 0`、`Skipped: 0`；
- 没有未经测试就声明的 Extended Features。

### 阶段 D：提交官方 report PR

在 Gateway API fork 中增加：

```text
conformance/reports/v1.4.0/alibaba-higress/
  README.md
  standard-<higress-version>-default-report.yaml
```

报告文件必须是 Suite 输出原件，只允许重命名，不修改内容。

README 模板：

```markdown
# Higress

Higress is a cloud-native API gateway based on Istio and Envoy.

## Table of Contents

| API channel | Implementation version | Mode | Report |
| --- | --- | --- | --- |
| standard | [<version>](https://github.com/alibaba/higress/releases/tag/<version>) | default | [<version> report](./standard-<version>-default-report.yaml) |

## Reproduce

git clone https://github.com/alibaba/higress.git
cd higress
git checkout <version>
make higress-gateway-api-test \
  HIGRESS_CONFORMANCE_VERSION=<version>
```

report PR 的验收不是“PR 已创建”，而是：

- Gateway API CI 接受报告格式和结果；
- 维护者可以根据 README 复现；
- PR 已合并；
- Higress 出现在 v1.4 实现能力表的 Gateway / HTTPRoute 行。

### 阶段 E：申请 implementations 页面条目

官方规则要求 conformance report submission PR 已合并后，才能满足新增实现条目的 Conformance 前提。因此顺序必须是两个 PR：

1. 先合并 report PR；
2. 再修改官方 implementations 页面，增加 Higress 简介、GitHub、安装/快速开始文档和 v1.4 报告说明。

按当前版本时效，Higress 的 v1.4 报告可支撑版本能力展示和至少 Partially Conformant 的申请，但不能声称最新 Conformant。页面文字必须如实标注 `Gateway API v1.4.0 / GATEWAY-HTTP Core`。

## 7. 2026-07-14 云集群实测结果

第一轮在用户提供的 ACK Kubernetes v1.36.1 集群上使用了 `higress-system` 与 `gateway-conformance-infra` 分离的部署。该轮用于定位问题，但不是最终 CI 拓扑。最终验证将 Higress Helm release 整体安装到 `gateway-conformance-infra`，使 controller、pilot、gateway Pod、gateway Service 与官方基础 Gateway、同 namespace TLS Secret 处于同一个 namespace。

ACK 中 Gateway API CRD 的 `gateway.networking.k8s.io/bundle-version` 实际为 `v1.4.0`、channel 为 `standard`。因此最终运行保持 `--allow-crds-mismatch=false`，与正式 kind CI 完全一致，没有通过跳过 CRD 版本校验来放宽结果。

最终直接通过 ACK LoadBalancer `47.238.232.74` 运行，未使用 `kubectl port-forward`。原始报告位于 `out/gateway-api-conformance/ack-same-namespace-direct-report.yaml`，SHA-256 为 `925e89ce5dece347ba24ba25ae0eeada677bca256d416a7ef3d90219b567f6a8`，结果为：

```yaml
gatewayAPIVersion: v1.4.0
gatewayAPIChannel: standard
implementation:
  version: 8b5cdbc91a7b71b0bc60e4f15015ffa12ceffe1f
profiles:
- name: GATEWAY-HTTP
  core:
    result: success
    statistics:
      Failed: 0
      Passed: 33
      Skipped: 0
```

执行期间发现并修复的强制 Core 问题如下；没有为测试名称添加特判：

| 官方用例/阶段 | 原因 | 修复 | 验证 |
| --- | --- | --- | --- |
| Suite 基础 Gateway 地址 | 第一轮把 gateway Service 放在 `higress-system`，而官方 Gateway 固定在 `gateway-conformance-infra`，触发了 Higress 现有同 namespace 假设 | 测试专用 Helm release 改到 `gateway-conformance-infra`；回滚跨 namespace FQDN、selector 优先级和初始化时序修改 | 所有基础 Gateway 均得到 `47.238.232.74`，全量通过 |
| `GatewayClassObservedGenerationBump` | controller 只处理名为 `higress` 的 GatewayClass，忽略 controllerName 相同的其他 Class | 以 `spec.controllerName=higress.io/gateway-controller` 作为归属条件 | 单项与最终全量通过 |
| `HTTPRouteCrossNamespace` | 第一轮共享 gateway workload 与 Gateway 不同 namespace，工作负载关联失败 | 让 Higress gateway Service/Pod 与官方 Gateway 同 namespace；不修改生产 selector 行为 | 全量通过 |
| `HTTPRouteHTTPSListener` | 第一轮 Gateway/Secret 虽同 namespace，但 gateway Pod 在 `higress-system`，被 Istio 现有 SDS 信任边界拒绝 | gateway Pod 与 Gateway/Secret 同 namespace；Istio submodule 和统一 pilot tag 均回滚到原值 | HTTPS 请求直接通过 LoadBalancer 验证通过 |
| `HTTPRouteHostnameIntersection` | VirtualService 使用原始 HTTPRoute hostname，没有收窄到 listener/route hostname 的有效交集 | 按 Gateway API hostname 子集规则生成交集并去重 | 全部 wildcard、精确与不相交子场景通过 |
| `HTTPRouteServiceTypes` | backend 存在性依赖 Istio 服务目录；无 Endpoint 的 headless Service 未进入目录，被误报 `BackendNotFound` | 用 Kubernetes Service informer 校验 Gateway API Service 引用是否存在 | ClusterIP、headless、手工 EndpointSlice 三类请求全部通过 |

最终使用的镜像如下。只有 `higress-core` 使用当前提交重新构建的镜像；pilot 和 gateway 均保持与仓库其他集成测试一致的统一 tag，没有为 Gateway API 用例制作专用数据面镜像：

| 组件 | tag | 运行结果 |
| --- | --- | --- |
| higress-core | `8b5cdbc91a7b71b0bc60e4f15015ffa12ceffe1f`，digest `sha256:fb6aad3971b191d4467a917cc5f50d0e581351174fc4024a6a7713218c90a5a4` | Ready，0 次重启 |
| pilot | `de2c9628294f51b13c4a70b3a862b4372890797a` | Ready，0 次重启 |
| gateway | `481184afc44176eb23d64e0011dc3ea1ae6a410c` | Ready，0 次重启 |

两次同 namespace 全量执行均得到 `Passed: 33 / Failed: 0 / Skipped: 0`。第一轮在 ACK LoadBalancer 尚未就绪时通过临时 port-forward 验证；第二轮直接访问 LoadBalancer，作为最终验收结果。`go test` 输出中其他 profile 的 `SKIP` 是未选择的 Extended、Mesh、GRPC、TLSRoute 等测试，不计入 `GATEWAY-HTTP Core`；官方报告中的 Core `Skipped` 为 0。

Higress 部署中只有 `higress-core` 监听 Gateway API；discovery 容器显式设置 `PILOT_ENABLE_GATEWAY_API=false`。因此本轮不修改 Istio submodule。v1.4 Core 的跨 namespace Secret + ReferenceGrant 用例只验证 Gateway listener status，不覆盖跨 namespace TLS 数据面握手；不能因为状态用例通过就宣称该数据面能力已经完整支持。

最终分支的 Istio submodule commit 与 `origin/main` 完全一致，均为 `de2c9628294f51b13c4a70b3a862b4372890797a`。第一轮为绕过 namespace 分离而引入的跨 namespace Service FQDN、selector 优先级、selector 初始化时序和 Istio SDS 放宽均已回滚；它们不属于 v1.4 `GATEWAY-HTTP Core` 的必要产品修复。保留的生产逻辑改动仅对应三个真实 Core 缺口：按 controllerName 认领 GatewayClass、计算 listener/route hostname 交集、用 Kubernetes Service informer 判断 Service backend 是否存在；另补充了准确的 Core supportedFeatures 状态。

本地 Apple Silicon 验证还发现统一的 gateway tag 当前是 `linux/amd64` 单架构镜像，在 arm64 kind 节点的模拟执行中 Envoy 会段错误。默认 GitHub Actions 和 ACK 验证环境均为 amd64；为保持与其他 e2e 完全相同的数据面产物，本方案不为 Gateway API job 单独替换 tag。arm64 本地执行需要等统一 gateway tag 提供 arm64 产物后再作为支持路径。

## 8. Done 定义

只有以下项目全部有证据时，首期目标才算完成：

- `make higress-gateway-api-test` 真正执行 v1.4.0 官方 Suite。
- `.github/workflows/build-and-test.yaml` 的所有 PR 默认执行该 target，没有允许失败或路径过滤。
- Gateway API job 与现有 Higress e2e job 并列，失败会使 workflow 和依赖它的 `publish` job 失败；是否加入分支保护 required checks 由仓库管理员配置。
- Go module、CRD bundle 和 report 都是 v1.4.0。
- `GatewayClass.status.supportedFeatures` 是实测能力，不是 `AllFeatures`。
- `GATEWAY-HTTP` Core 失败为 0、skip 为 0。
- CI 对回归有阻断作用，并保存报告和诊断材料。
- PR 报告来自当前 commit 并作为 artifact 保存；正式报告来自不可变 Higress release。
- 两类报告由同一个测试入口、Make target 和临时 kind 流程生成。
- report YAML 未被手工修改。
- Gateway API 官方 report PR 已合并。
- Higress 已出现在官方 v1.4 实现能力表。
- report PR 合并后，implementations 页面 PR 已提交并按真实时效分类。

## 9. 后续路线

完成 v1.4 首期后：

1. 将已通过的 GATEWAY-HTTP Extended Features 逐项加入 status 和报告。
2. 评估 `GATEWAY-GRPC`，再评估 `GATEWAY-TLS`。
3. 升级到 v1.5 或 v1.6，提交最近两个 Gateway API release 的成功报告，获得当前 Conformant 身份。
4. 每次 Gateway API minor release 后安排一次依赖、CRD、Suite 和报告更新，避免条目因报告过期被降级或移除。

## 10. 风险与边界

- 不能用 Istio 已通过的报告代表 Higress；controller name、安装方式、数据面镜像和能力集合都不同。
- 不能用 Higress 自有 e2e 替代官方 Suite。
- 不能用 `AllFeatures` 换取表面覆盖率；声明即承诺，Suite 会打开对应 Extended/联合特性测试。
- 不能用 skip 生成正式成功报告。
- 不能手改报告中的版本、结果或 feature 列表。
- 不能用 `main` 作为正式 implementation version。
- localhost dialer 只在 kind CI 中启用，只改变测试发起端的连接路径，不得改变请求的 Host、SNI、协议、端口或预期响应；云上集群应关闭它。
- v1.4 报告完成的是“当前支持版本的官方证明”；最新 Conformant 身份需要后续版本报告。
