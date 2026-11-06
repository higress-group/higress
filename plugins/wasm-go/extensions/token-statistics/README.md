# Token Statistics Plugin

该目录实现了 Higress 的 `token-statistics` wasm 插件，用于从各类 AI 服务响应中提取 token 使用量并导出到日志/指标，以便进行成本统计与监控。

## 主要功能

- 从多厂商响应（OpenAI, Azure OpenAI, Anthropic/Claude, Google Gemini, Qwen, Baichuan, Cohere, DeepL 等）中解析 token 使用信息（input/output/total tokens）。
- 支持多种响应/流式格式的兼容解析（实现了多厂商的 ExtractTokenUsage）。
- 支持路径/内容类型过滤，避免对不关心的请求进行统计。
- 将统计结果导出为日志和 Prometheus 风格指标（由 `PrometheusExporter` 与 `LogExporter` 负责）。

## 主要文件

- `main.go` - 插件主逻辑：配置解析、路径过滤、请求/响应钩子、token 使用记录与指标导出。
- `statistics.go` - 各厂商 token 提取器实现（ExtractTokenUsage）。
- `main_test.go` - 单元测试（标准库-only），覆盖路径过滤与多厂商 token 提取场景。

## 配置示例

插件使用 JSON 配置。关键字段：

- `enable_path_suffixes`：仅针对以这些后缀为结尾的路径启用统计，如果为空则对所有路径生效。
- `enable_content_types`：仅针对匹配内容类型的响应/请求进行统计。
- `exporters`：设置导出器（例如 `log` / `prometheus`）。

示例配置：

```json
{
  "enable_path_suffixes": ["/chat/completions", "/v1/chat/completions"],
  "enable_content_types": ["application/json"],
  "exporters": [
    {"type":"log"},
    {"type":"metric", "config": {"namespace":"higress", "subsystem":"token_statistics"}}
  ]
}
```

## 本地开发与测试

注意：插件在真实运行时依赖 proxy-wasm hostcalls（例如定义/记录指标、获取 HTTP header 等）。直接在本地通过 `go run` 或 `go test` 运行会遇到 hostcall 不可用的问题（会触发 panic）。为方便本地开发，本仓库做了以下改进：

- 在 `main.go` 中，将指标抽象为 `metricCounter` 并提供 `noopCounter`，在非 Wasm 环境下跳过调用真实 hostcalls，避免 panic。
- 单元测试已重写为仅使用标准库（`testing`），并验证 `isPathEnabled` 与 `extractStreamingTokenUsage` 在多厂商与边界场景下的行为。

在本地运行：

```bash
cd plugins/wasm-go/extensions/token-statistics
go test -v
go run ./
```

跟着官网https://higress.cn/zh-cn/blog/e2e-debug/测试插件
```bash
PLUGIN_NAME=token-statistics make higress-wasmplugin-test 
```
## 在 Wasm 运行时部署

当插件被真正部署到 Higress 的 wasm 运行时（或任何提供 proxy-wasm hostcalls 的宿主）时，`init()` 中将会调用 `proxywasm.DefineCounterMetric` 来创建真实指标，导出到宿主的指标系统（如 Envoy/Prometheus）中。

部署前请确保：

- wasm 运行环境提供必要的 proxy-wasm hostcalls。
- 如果你定制了 exporter（例如 PrometheusExporter），请确保配置与宿主指标系统兼容。

## 常见问题（FAQ）

Q: 为什么本地运行出现 `failed to define metrics` 或 panic？

A: 因为 proxy-wasm 的 hostcall 只有在 wasm 宿主中可用；本地执行没有宿主，调用这些 hostcall 会导致 nil-pointer panic。为了本地开发，我们在非 wasm 环境下会跳过 metrics 初始化并使用 no-op 实现。

Q: 如何为新的供应商添加 token 解析器？

A: 在 `statistics.go` 中添加新的类型并实现 `ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage`。然后在 `extractStreamingTokenUsage` 中添加对应的 case，映射适当的厂商名/别名。

Q: 我想在测试中断言 exporter 行为，应该怎么做？

A: 当前 exporter 是在包内实现的。你可以将 exporter 抽象为接口并注入 mock 实现，或在测试环境中临时替换 exporter 的实例（需稍作代码改造）。我可以帮你把 exporter 重构为可注入接口并编写对应的单元测试。

## 贡献

欢迎提交 PR：

- 添加更多厂商的解析兼容性。
- 增强 exporter（例如将 Prometheus 导出改为支持标签维度、模型维度等）。
- 改进单元测试覆盖更多钩子逻辑（例如 `onHttpRequestBody`、`onHttpResponseHeaders` 行为模拟）。

## 联系

如有问题，请在仓库中创建 issue，或直接在 PR 中指明你遇到的问题与预期行为。

创建WasmPlugin资源配置：
确保插件执行在 AI-Cache 插件之后（通过priority配置，数值越大执行越晚）；

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: token-statistics
  namespace: higress-system
spec:
  url: file:///opt/plugins/token-statistics/main.wasm
  phase: UNSPECIFIED_PHASE
  priority: 200
  defaultConfig:
    dimensions:
      - name: "model"
        value_source: "response_body"
        value: "model"
        apply_to_metric: true
        apply_to_log: true
      - name: "consumer"
        value_source: "request_header"
        value: "x-mse-consumer"
        apply_to_metric: true
        apply_to_log: true
    exporters:
      - type: "prometheus"
        config:
          namespace: "ai"
          subsystem: "token"
      - type: "log"
        config:
          level: "info"
    enable_path_suffixes:
      - "/v1/chat/completions"
      - "/v1/completions"
    enable_content_types:
      - "application/json"
      - "text/event-stream"
  defaultConfigDisable: false
```

### 4.6.3 功能验证

#### 验证指标输出

部署后可以通过Prometheus查询相关指标：

higress_token_statistics_<model>_input_tokens_total
higress_token_statistics_<model>_output_tokens_total
higress_token_statistics_<model>_total_tokens_total
举例（如果 model = "gpt4" 或已被替换为合法字符 gpt_4）：

higress_token_statistics_gpt4_input_tokens_total
higress_token_statistics_gpt4_output_tokens_total
higress_token_statistics_gpt4_total_tokens_total
PromQL 示例

最近 1 小时内某模型的总 token 增量：
increase(higress_token_statistics_gpt4_total_tokens_total[1h])

匹配所有模型的 input tokens（用 name 正则）：
sum({name=~"higress_token_statistics_.*_input_tokens_total"})

#### 验证日志输出

检查Higress网关日志中是否包含Token统计信息：

```json
{
  "timestamp": "2025-12-17T10:00:00Z",
  "level": "INFO",
  "message": "Token usage statistics",
  "model": "gpt-3.5-turbo",
  "input_tokens": 15,
  "output_tokens": 42,
  "total_tokens": 57,
  "consumer": "test-user"
}
```

#### 验证HTTP导出

如果配置了HTTP导出器，可以检查目标接收端是否收到统计数据。

<!-- todo -->
grafana dashboard json file

