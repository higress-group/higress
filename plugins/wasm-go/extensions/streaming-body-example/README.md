# 功能说明

`streaming-body-example` 演示 Higress Wasm Go SDK 的**流式请求体/响应体**处理能力，包括：

- 在请求头/响应头阶段移除 `Content-Length`（便于分块传输）
- 通过 `ProcessStreamingRequestBody` / `ProcessStreamingResponseBody` 按 chunk 处理 body
- 在示例中将每个 chunk 替换为固定内容 `test\n` 并打日志

本插件**无配置项**，仅用于学习与参考实现，请勿用于生产环境。

# 处理流程

| 阶段 | 行为 |
| -------- | -------- |
| 请求头 | 删除 `content-length` |
| 请求体（流式） | 记录 chunk 日志，返回 `test\n` |
| 响应头 | 删除 `content-length` |
| 响应体（流式） | 记录 chunk 日志，返回 `test\n` |

# 配置示例

```yaml
# 无额外配置字段
```

# 构建

```bash
cd plugins/wasm-go
PLUGIN_NAME=streaming-body-example make build
```

开发自定义流式插件时，可参考本目录 `main.go` 与 [wasm-go 插件开发文档](../../README.md)。
