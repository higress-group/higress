# Higress MCP Demo

[English](./README_EN.md)

本目录集中维护可运行、可复现的 Higress MCP Demo。它既提供一套完整的本地 Higress 环境，也按 MCP 协议版本保存插件构建方式和该版本已经实现的能力验证手册。

## 设计目标

- **环境完整**：使用 Kind、Helm 拉起 Higress Controller、Gateway、CRD 和 Console。
- **公共依赖复用**：协议无关的集群、Higress 配置和模拟业务后端统一放在 `environment/`。
- **版本边界明确**：协议相关的插件源码 commit、构建方式和 Demo 都归档在对应的 `protocol/<version>/`。
- **逐步可验证**：每个 Demo 都是一份独立 README，说明验证目标、操作步骤、预期响应和网关/后端证据。
- **独立执行**：读者可以选择关心的能力，按照对应 README 完成实验。

## 快速开始

从 Higress 仓库根目录执行：

```bash
cd samples/mcp

# 1. 构建目标协议版本对应的 MCP Server 插件
./protocol/2026-07-28/plugin/build.sh

# 2. 拉起公共 Higress 环境
./environment/scripts/up.sh

# 3. 选择一个 Demo，按其 README 逐步验证
less ./protocol/2026-07-28/01-stateless-http/README.md

# 4. 全部实验完成后清理环境
./environment/scripts/down.sh
```

环境依赖、端口、启动流程和故障排查见[公共环境说明](./environment/README.md)。

## Demo 导航

| MCP 协议版本 | 插件来源 | 已提供的 Demo |
| --- | --- | --- |
| [2026-07-28](./protocol/2026-07-28/README.md) | 固定 Higress 源码 commit 本地构建 | 无状态 HTTP、REST-to-MCP、modern-to-legacy、请求前置校验 |

## 目录结构

```text
samples/mcp/
├── README.md
├── README_EN.md
├── environment/                    # 与 MCP 协议版本无关的公共环境
│   ├── kind/                       # Kind 集群定义
│   ├── higress/                    # Higress Helm values
│   ├── apps/                       # 通用模拟业务后端
│   └── scripts/                    # 环境启动、状态检查和清理
└── protocol/
    └── <protocol-version>/
        ├── README.md               # 该版本的能力和 Demo 索引
        ├── plugin/                 # 固定源码 commit 的插件构建入口
        └── <number>-<feature>/     # 一个已实现能力对应一个独立 Demo
            ├── README.md
            ├── README_EN.md
            ├── resources.yaml
            ├── requests/
            └── fixture/            # 仅在该 Demo 需要时存在
```

## 边界约定

`environment/` 保存与 MCP wire contract 无关的 Kind、Higress Helm 配置和普通 REST 业务后端。legacy MCP Server、特殊客户端行为和版本专属报文等 fixture 放在对应 Demo 中。

每个 Demo README 至少应包含：

1. 验证目标和不在范围内的能力；
2. 前置条件以及适用的协议和插件版本；
3. 可直接复制的 step-by-step 操作；
4. 每一步的预期响应和断言命令；
5. 必要的 Gateway、插件或后端侧证据；
6. 该 Demo 自己创建资源的清理命令。

## 新增协议版本或 Demo

新增协议版本时，在 `protocol/<version>/plugin/` 中记录实现该版本的 Higress commit 和构建方法。

新增 Demo 时：

- 直接创建在所属协议版本目录下，并用数字前缀保持阅读顺序；
- 一个目录只验证一个主要能力，跨能力依赖应在 README 中明确；
- 提供中英文 README、确定性请求和预期证据；
- 不依赖真实凭据或私有服务，不提交生成的 WASM、运行日志和临时证据。
