# MCP 2026-07-28 Demo

[English](./README_EN.md) | [返回 MCP Demo 首页](../../README.md)

本目录维护 MCP `2026-07-28` 版本已经由 Higress 实现并可运行验证的特性。版本目录负责从固定 Higress commit 构建 `mcp-server` 插件；每个特性由一个独立 README 提供逐步操作指引。

## 准备环境和插件

从 Higress 仓库根目录执行：

```bash
cd samples/mcp
./protocol/2026-07-28/plugin/build.sh
./environment/scripts/up.sh
```

插件默认从以下源码构建：

```text
https://github.com/higress-group/higress.git
14c36d9bd70b3dc38237cda6175b3f9dede0dccd
```

构建结果写入：

```text
.runtime/plugins/mcp-server/2026-07-28/plugin.wasm
```

Kind 节点和 Higress Gateway 会通过公共环境提供的 hostPath 看到该文件。所有 Demo 的 `WasmPlugin` 都引用：

```text
file:///opt/plugins/mcp-server/2026-07-28/plugin.wasm
```

## Demo 列表

| Demo | 验证特性 |
| --- | --- |
| [01 Stateless HTTP](./01-stateless-http/README.md) | 无 initialize、无协议 Session 的 discover/list/call |
| [02 REST-to-MCP](./02-rest-to-mcp/README.md) | 将 MCP Tool 调用转换成一次普通 REST 请求 |
| [03 Modern-to-Legacy](./03-modern-to-legacy/README.md) | request-scoped legacy handshake、结果适配和 Header 隔离 |
| [04 Request Validation](./04-request-validation/README.md) | 参数、Origin、Header/Body 一致性校验以及后端零调用 |

各 Demo 可以独立执行。进入任一目录，按照 README 部署资源、发送请求、观察证据并清理即可。

## 使用其他源码版本

编辑 `plugin/source.env`，或者在执行前覆盖其中的变量：

```bash
MCP_DEMO_HIGRESS_REPOSITORY=https://github.com/<owner>/higress.git \
MCP_DEMO_HIGRESS_REF=<commit-sha> \
./protocol/2026-07-28/plugin/build.sh
```
