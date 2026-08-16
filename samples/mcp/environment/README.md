# Higress MCP 公共实验环境

[English](./README_EN.md) | [返回 MCP Demo 首页](../README.md)

本目录负责启动所有 MCP Demo 共用的、与协议版本无关的实验环境。环境使用 Kind 和 Helm 安装完整的 Higress Controller、Gateway、CRD 和 Console，并部署一个可观察的普通 HTTP 天气服务。

公共环境将宿主机的 `.runtime/plugins` 挂载到 Gateway 的 `/opt/plugins`。启动环境前，先通过对应版本目录下的 `plugin/build.sh` 构建协议插件。

## 前置条件

- Docker 或 Podman；
- Kind；
- kubectl；
- Helm；
- curl；
- jq（用于执行各 Demo 的响应断言）；
- 至少约 4 GiB 可用内存；
- 能访问 Higress Helm 仓库和所需镜像仓库。

## 启动

从 Higress 仓库根目录执行：

```bash
cd samples/mcp
./protocol/2026-07-28/plugin/build.sh
./environment/scripts/up.sh
```

脚本会：

1. 创建名为 `higress-mcp-demo` 的 Kind 集群；
2. 将 `.runtime/plugins` 挂载到 Kind 节点的 `/opt/plugins`；
3. 使用固定的 Helm Chart `2.2.3` 安装 Higress；
4. 构建并部署协议无关的 `observable-weather` HTTP 服务；
5. 建立 Gateway、Console 和后端服务的本地端口转发。

脚本会在 `.runtime` 中记录创建集群时使用的容器引擎、集群名称和实例 ID，并在集群内写入匹配的所有权标记。若同名集群不是由本 Demo 创建，启动和清理命令都会拒绝操作；可以通过 `MCP_DEMO_CLUSTER` 指定其他名称。

环境地址：

| 地址 | 用途 |
| --- | --- |
| `http://127.0.0.1:18080` | Higress Gateway |
| `http://127.0.0.1:18081` | Higress Console |
| `http://127.0.0.1:18082` | 可观察 HTTP 后端 |
| `http://127.0.0.1:18082/__state` | 查询后端调用记录 |
| `POST http://127.0.0.1:18082/__reset` | 清空后端调用记录 |

## 检查状态

```bash
./environment/scripts/status.sh
```

该命令显示 `higress-system`、`mcp-demo` Pod 状态以及三个端口转发是否可用。

## 清理

```bash
./environment/scripts/down.sh
```

该命令停止端口转发并删除由本 Demo 创建的 Kind 集群。构建出的插件保留在 `.runtime/plugins`，便于下次启动复用；该目录不会提交到 Git。

## 公共后端边界

`observable-weather` 只实现普通 HTTP：

- `GET /weather?location=<city>`；
- `GET /healthz`；
- `GET /__state`；
- `POST /__reset`。

它不理解 MCP 版本、JSON-RPC 方法或 Session。特定 MCP 版本的后端 fixture 由对应 Demo 提供。

## 可覆盖参数

```bash
MCP_DEMO_CLUSTER=my-mcp-demo \
MCP_DEMO_GATEWAY_PORT=28080 \
MCP_DEMO_CONSOLE_PORT=28081 \
MCP_DEMO_BACKEND_PORT=28082 \
./environment/scripts/up.sh
```

其他可覆盖参数包括 `MCP_DEMO_KIND_NODE_IMAGE` 和 `MCP_DEMO_HIGRESS_CHART_VERSION`。
