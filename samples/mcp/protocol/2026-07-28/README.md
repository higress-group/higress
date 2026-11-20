# MCP 2026-07-28 Demo

[English](./README_EN.md) | [返回 MCP Demo 首页](../../README.md)

本目录维护 MCP `2026-07-28` 版本已经由 Higress 实现并可运行验证的特性。每个特性由一个独立 README 提供逐步操作指引。

## 准备环境和插件

Demo 01–05 使用公共 Kind 环境和固定源码的 `mcp-server`。从 Higress 仓库根目录执行以下命令。

```bash
cd samples/mcp
./protocol/2026-07-28/plugin/build.sh
./environment/scripts/up.sh
```

插件默认从以下源码构建：

```text
https://github.com/higress-group/higress.git
1268f41d5de0ab07ecd9759c8217b7815dfc655e
```

构建结果写入：

```text
.runtime/plugins/mcp-server/2026-07-28/plugin.wasm
```

Kind 节点和 Higress Gateway 会通过公共环境提供的 hostPath 看到该文件。Demo 01–05 的 `WasmPlugin` 引用以下路径。

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
| [05 Instructions](./05-instructions-passthrough/README.md) | 原生 Go SDK 直连、开启/关闭透传及上游指引变更验证 |
| [06 Virtual Tools with Inspector](./06-virtual-tools-inspector/README.md) | Inspector 发现组合入口、列举空目录，并调用 Legacy 和 Modern 来源 |

各 Demo 可以独立执行。进入任一目录，按照 README 部署资源、发送请求、观察证据并清理即可。

Demo 06 在独立 Docker 网关中运行，从当前检出构建 `mcp-router` 和 `mcp-server`。

## 使用其他源码版本

编辑 `plugin/source.env`，或者在执行前覆盖其中的变量：

```bash
MCP_DEMO_HIGRESS_REPOSITORY=https://github.com/<owner>/higress.git \
MCP_DEMO_HIGRESS_REF=<commit-sha> \
./protocol/2026-07-28/plugin/build.sh
```
