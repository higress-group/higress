# Demo 05：原生 SDK 读取上游 Instructions

[English](./README_EN.md) | [返回版本目录](../README.md)

官方 Go SDK `v1.7.0` 同时提供原生 MCP 上游和自动客户端。上游通过 `ServerOptions.Instructions` 提供中文多行指引及 `get_weather` 工具；客户端使用 SDK 的 Connect/ListTools/CallTool，从 SDK 对象读取并逐字比较指引。MCP 报文由 SDK 生成，无需手写 JSON-RPC。

## 前置条件

需要 Go >=1.25、Python 3、curl、kubectl、Kind、Helm，以及正在运行的 Docker 或 Podman。可选 Inspector 需要 Node >=22.19.0。本 Demo 不使用真实凭据或外部业务服务。

先从仓库根目录构建固定插件并启动公共环境：

```bash
cd samples/mcp
./protocol/2026-07-28/plugin/build.sh
./environment/scripts/up.sh
cd protocol/2026-07-28/05-instructions-passthrough
export MCP_DEMO_EVIDENCE="$HOME/higress-instructions-evidence/$(date +%Y%m%d-%H%M%S)"
```

插件默认从 canonical Higress commit `1268f41d5de0ab07ecd9759c8217b7815dfc655e` 构建。共享插件 builder 保持 Go 1.24.4；原生 fixture 使用 `golang:1.25.0-alpine` 和 `alpine:3.20`。编译器不同可能产生不同 Wasm hash，查看本次构建记录，不套用旧 hash。升级源码 pin 后应同时回归 Demo 01–04。

## Step 1：执行四阶段 SDK 验证

```bash
./verify.sh
```

脚本会构建并载入 `localhost/mcp-demo/instructions-sdk:1.0.0`，部署本例的 ConfigMap、Deployment、Service、Ingress、WasmPlugin，建立自己的上游端口转发，再启动四个独立 SDK client。它复用公共环境的引擎和所有权检查，不创建第二套网关环境。

上游只有一个，指引 S 来自 [fixture/instructions.txt](./fixture/instructions.txt)，包含末尾换行。两条路径只在 `server.passthroughInstructions` 配置上有差别，不需要改 Host 或 hosts：

| 阶段 | SDK endpoint | SDK instructions | 上游实际 method 序列 |
| --- | --- | --- | --- |
| direct | `http://127.0.0.1:18083/mcp` | 完整 S | discover、list、call |
| on | `http://127.0.0.1:18080/instructions/on` | 完整 S | discover、list、call |
| off | `http://127.0.0.1:18080/instructions/off` | 空值，SDK JSON 中省略该字段 | list、call；零上游 discover |
| changed | 启用路径，全新 SDK client | 完整 S' | discover、list、call |

表中的 method 分别是 `server/discover`、`tools/list`、`tools/call`。所有阶段都必须协商 `2026-07-28`，不允许实际 `initialize`。Go SDK 将 modern discover 放在名为 `InitializeResult` 的对象中，名称不代表使用了旧握手。客户端禁用了 standalone SSE，不进行订阅。

`changed` 阶段把 ConfigMap 改为 [instructions-changed.txt](./fixture/instructions-changed.txt)，滚动重启原上游后再新建客户端，避免复用 SDK discovery 状态。脚本结束时恢复运行前本例已有的配置；首次运行则恢复 S，并停止自己的端口转发。资源保留，便于手工重复。恢复结果和进程清理写入 `completion.txt`、`restore.log`。

## Step 2：检查 SDK 与真实代理证据

```bash
python3 -m json.tool "$MCP_DEMO_EVIDENCE/on.json"
python3 -m json.tool "$MCP_DEMO_EVIDENCE/off.json"
python3 -m json.tool "$MCP_DEMO_EVIDENCE/changed.json"
cat "$MCP_DEMO_EVIDENCE/completion.txt"
```

每个 JSON 包含 `sdkResult`、`listResult`、`callResult` 和 SDK middleware 记录的 `upstream.events`。验证入口会强制检查：

- `sdkResult.instructions` 与相应 UTF-8 文件完整相等，含空白和末尾换行；off 阶段为空。
- SDK 协商版本为 `2026-07-28`；直连 `serverInfo` 为 `native-instructions-upstream`，经过 Higress 时仍为 `instructions-gateway`、版本 `1.0.0`。
- 列表中恰有 `get_weather`，调用返回 `杭州：晴，25°C`，无工具错误。
- 每阶段上游序列和现代 method/协议头严格匹配，不出现 legacy initialize；off 不触发上游 discover。
- `gateway.log` 中可关联 on/off/changed 的 `demo05-<phase>-<n>` 请求；Ingress/WasmPlugin 快照记录实际路由。

health、`/__state` 和 `POST /__reset` 仅作辅助观察和分阶段清空 ledger，不实现 MCP 协议。生成的客户端二进制、SDK 输出、源码/插件/镜像身份、ConfigMap 快照与校验和均写入外部 `MCP_DEMO_EVIDENCE`，不写入 Git。`verify.sh` 不运行 Inspector。

## Step 3：手工重复 SDK 调用

`verify.sh` 已将上游配置恢复。重新建立本例端口转发：

```bash
source ../../../environment/scripts/common.sh
start_port_forward instructions-upstream mcp-demo service/instructions-mcp 18083:8080
wait_http http://127.0.0.1:18083/healthz
"$MCP_DEMO_EVIDENCE/client" \
  --phase manual-on \
  --endpoint http://127.0.0.1:18080/instructions/on \
  --admin-url http://127.0.0.1:18083 \
  --instructions-file fixture/instructions.txt
```

使用 off 路径时同时加 `--instructions-absent --expected-methods tools/list,tools/call`；直连时使用 `/mcp` 并加 `--server-name native-instructions-upstream`。每次命令都会创建新 SDK client 并重置上游 ledger，请勿与其他客户端并发执行计数验证。

也可以单独部署本例：运行 `fixture/build.sh`，用 `kubectl -n mcp-demo create configmap mcp-instructions-config --from-file=instructions.txt=fixture/instructions.txt --dry-run=client -o yaml | kubectl apply -f -` 创建配置，再执行 `kubectl label -n mcp-demo configmap mcp-instructions-config mcp.higress.io/demo=instructions --overwrite`、`kubectl apply -f fixture/deployment.yaml` 和 `kubectl apply -f resources.yaml`。等待 Deployment rollout 完成后按上面的 SDK 命令验证。

## Step 4：可选 Inspector 2.12.0 手动复现

安装/执行固定版本，显式指定 Modern 和 HTTP。路径名不是 `/mcp`，因此不能依赖 transport 自动判断。把可写 catalog 放在证据目录，避免修改默认个人 catalog：

```bash
node --version  # >=22.19.0
npx --yes @modelcontextprotocol/inspector@2.12.0 --cli \
  http://127.0.0.1:18080/instructions/on \
  --transport http --protocol-era modern \
  --catalog "$MCP_DEMO_EVIDENCE/inspector-catalog.json" \
  --method initialize --format json
npx --yes @modelcontextprotocol/inspector@2.12.0 --cli \
  http://127.0.0.1:18080/instructions/on \
  --transport http --protocol-era modern \
  --catalog "$MCP_DEMO_EVIDENCE/inspector-catalog.json" \
  --method tools/list --format json
npx --yes @modelcontextprotocol/inspector@2.12.0 --cli \
  http://127.0.0.1:18080/instructions/on \
  --transport http --protocol-era modern \
  --catalog "$MCP_DEMO_EVIDENCE/inspector-catalog.json" \
  --method tools/call --tool-name get_weather --tool-arg city=杭州
curl -fsS http://127.0.0.1:18083/__state | python3 -m json.tool
```

CLI 的 `initialize` 名称表示 connect-only probe。在显式 modern 模式检查真正的 discover instructions 与工具结果，并查看上游 ledger 确认没有 legacy `initialize`。这些是可选手动步骤，不能仅凭本说明认为已执行；自动验收使用上面的原生 Go SDK。

## 边界与清理

这里只演示单个 explicit-modern HTTP 代理的正常成功响应。网关只合并非空且不超过 32768 decoded UTF-8 字节的 instructions，仍拥有自身身份、能力和缓存提示；字段不会缓存。上游指引是文本，不作为工具授权，也不在本例中执行。错误、SSE、认证和非法字段矩阵由功能测试覆盖，本例不新增产品行为。

```bash
source ../../../environment/scripts/common.sh
stop_port_forward instructions-upstream
kubectl delete -f resources.yaml
kubectl delete -f fixture/deployment.yaml
kubectl -n mcp-demo delete configmap mcp-instructions-config
# 全部实验完成后，从 samples/mcp 执行 ./environment/scripts/down.sh
```
