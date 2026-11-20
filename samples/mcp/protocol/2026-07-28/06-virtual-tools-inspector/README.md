# Demo 06：用 MCP Inspector 验证虚拟工具集

[English](./README_EN.md) | [返回版本目录](../README.md)

通过一个虚拟 MCP 地址调用两个来源各自的 `echo`。Inspector 以 `2026-07-28` 无状态 HTTP 接入。Higress 先运行 `mcp-router`，再运行 `mcp-server`，按组合名分派到 Legacy `2025-03-26` 或 Modern `2026-07-28` 上游。

## 一键验证

需要支持 host networking 的 Docker、Go 1.24.4、Python 3.8+、Node.js 22.19.0+ 和 npm。脚本从当前检出构建两份 Wasm，启动固定镜像的 Higress Envoy 和本地 echo fixture。该实验自带网关配置。

从仓库根目录执行以下命令。

```bash
cd samples/mcp/protocol/2026-07-28/06-virtual-tools-inspector
python3 verify.py
```

脚本通过 **MCP Inspector CLI 2.12.0** 向真实网关发送八次操作。每次都显式设置 `--transport http --protocol-era modern`。

| 路径 | Inspector 操作 | 验收结果 |
| --- | --- | --- |
| `/virtual` | 发现、列举、调用两个工具 | `capabilities.tools: {}`，列表含 `legacy___echo` 和 `modern___echo`，调用分别返回 `legacy: hello` 和 `modern: hello` |
| `/empty` | 发现、列举 | `capabilities.tools: {}`，列举返回 `tools: []` |
| `/disabled` | 发现 | 分派规则禁用，`capabilities` 省略 `tools` |
| `/unmatched` | 发现 | 当前路由无匹配的分派规则，`capabilities` 省略 `tools` |

Inspector 的 CLI 方法名 `initialize` 表示连接探测。现代模式实际发送 `server/discover`。脚本核对 Inspector 协商的协议版本，还用 Envoy access log 确认四个入口实际执行了 `server/discover`。上游记录显示 Legacy 来源收到 `initialize`、`notifications/initialized`、`tools/call`，Modern 来源只收到 `tools/call`，其 `Mcp-Name` 与 `params.name` 均为 `echo`。

[plugins.json](./plugins.json) 展示路由器的 `servers` 和路由规则，以及服务端的 `toolSet.serverTools`。两个来源的工具原名都是 `echo`，入口用前缀区分它们。

证据目录会打印在终端。可用 `--evidence` 指定一个新目录。目录里有各 Inspector 操作的 JSON、`gateway.log`、`upstream.json`，以及包含源码提交、Wasm 摘要和验收结果的 `summary.json`。验证结束后脚本清理本次网关和本地 fixture。

## 在 Inspector 界面尝试

在一个终端运行网关，保留进程。复制它打印的 Inspector URL。

```bash
python3 verify.py --serve
```

在另一个终端启动 Web Inspector。把 `MCP_URL` 换成实际打印的地址。

```bash
MCP_URL='http://127.0.0.1:<port>/virtual'
npx --yes @modelcontextprotocol/inspector@2.12.0 --web \
  --server-url "$MCP_URL" --transport http --protocol-era modern
```

打开 Inspector 启动信息给出的页面，连接后进入 **Tools**。列举并任选一个 `___echo` 工具，以 `hello` 填写 `message` 后调用。要观察空目录，将 URL 的 `/virtual` 换成 `/empty`，重新连接并列举。用 Ctrl+C 停止网关和 Inspector。
