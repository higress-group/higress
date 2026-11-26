# AI route monitor

旧指标继续在上游响应头阶段记录，200–399 记成功，旧命名和错误分类保持兼容。OPE-11003 增加独立的请求终态指标，默认关闭，见 [部署与验收说明](../../../../monitoring/model-requests/README.md)。

```json
{
  "legacyMetrics": true,
  "terminalMetrics": true,
  "source": {
    "headerName": "x-wujie-source",
    "allowedValues": ["wujieai"],
    "fallbackValue": "unknown"
  },
  "models": {
    "allowedValues": ["gpt-4", "claude-sonnet-4.5"],
    "fallbackValue": "unknown"
  },
  "asyncSubmissionPaths": []
}
```

配置可放在 `WasmPlugin.spec.defaultConfig` 或 `matchRules[].config`。Higress controller 将它们转换为插件配置根对象和 `_rules_`；wrapper 对每条规则独立解析，不合并 defaultConfig，因此规则应填写所需的完整配置。

| 配置 | 默认值 | 行为 |
|---|---|---|
| legacyMetrics | true | 旧指标和旧错误体缓冲路径；独立开关 |
| terminalMetrics | false | 在 `proxy_on_log` 对应的 `OnHttpStreamDone` 累加一次新指标 |
| source.headerName | x-wujie-source | 合法 HTTP header token，大小写归一化 |
| source.allowedValues | [] | 每次请求读取 Header，最多 16 个精确匹配值；未配置不识别任何来源 |
| source.fallbackValue | unknown | 未知或缺失来源统一归入此值；普通归因标记，不做身份认证 |
| models.allowedValues | [] | 最多 128 个、每个 1–64 字节的**原始**模型名；结束时读取 `wasm.requestModel`，未匹配归入 fallback |
| models.fallbackValue | unknown | 有限模型兜底值 |
| asyncSubmissionPaths | [] | 最多 32 个精确路径，不含查询串；命中后使用独立提交计数器；轮询路径由路由匹配排除 |

来源值和两个 fallback 均限 `[A-Za-z0-9_-]{1,64}`。已配置模型名中的其他字符转为 `_`，例如 `claude-sonnet-4.5` → `claude-sonnet-4_5`；归一化碰撞会合并，发布前检查模型列表。任意请求来源/模型不能增加未配置的维度。非法配置返回配置解析错误，沿用 wrapper 的配置装载行为，不拦截业务请求。

新指标仅包含有限 `source/model/outcome` 业务维度；不包含 consumer、UID、请求 ID、URI 或错误原文。旧指标仍保留原有维度，需要按已有监控策略管理基数。

流式路径只保留当前 SSE 行和事件，各自最多 16 KiB 数据，不积累整个回答。识别 OpenAI `[DONE]`、Anthropic `message_stop`、Responses `response.completed`，以及显式错误/不完整事件。支持跨 chunk、CRLF、多行 data；超限、坏 JSON、未完整结束事件归 `unknown`。非流式 JSON 最多检查 16 KiB，过大或无法解析归 unknown；其他普通响应需要 clean end-of-stream。压缩/二进制响应不检查正文，成功不可确认时归 unknown。旧 HTTP 错误体仍沿用原有 10 KiB 缓冲限制。

限制：观察到的是监控所在过滤器位置的响应。当前 `ai-proxy/provider/responses_to_openai.go` 会在缺少上游 `[DONE]` 的 EOF 合成 `response.completed`；这种转换可隐藏上游未完整结束，监控无法从转换后的内容恢复证据。上线前必须检查实际过滤器顺序和转换路径并进行截断流测试，不能把此类路由宣称为已验证的上游完整成功率。远程隔离链路已验证当前 deepseek → Responses 转换的正常结束及 chunked/content-length 缺失 `[DONE]`：分别为 success/unknown/unknown。该证据只覆盖所测转换路径，本任务未改变转换器行为，其他部署链路仍需验证。

构建：`./build.sh` 生成 `main.wasm`，现有 scratch Dockerfile 将它打入 `plugin.wasm`。构建和发布 OCI 镜像走正常审核流程，本任务不发布镜像或 Maven 包。
