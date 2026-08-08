# ai-endpoint-picker

`ai-endpoint-picker` 在单个 upstream cluster 内为 OpenAI 兼容请求选择一个健康的 LLM endpoint。插件直接使用 Higress 提供的 upstream host metrics 和 override host 能力，不部署额外的 Endpoint Picker 服务，也不负责 cluster 间路由、流控或 admission control。

## 调度流程

插件按 `Filter → Normalize → Score → Pick → Feedback` 执行：

- Filter 只排除非健康 endpoint，缺少可选指标不会触发硬过滤。
- queue 和本地 inflight 在健康候选集内做 min-max 归一化，值越小分越高。
- KV cache 和 failure 分别使用 `1-utilization` 与 `1-EWMA`。
- LoRA 指标存在时，当前请求的 `model` 已加载得 1 分，否则得 0 分。
- 每项 scorer 输出 `[0,1]`。总分始终除以固定配置权重之和，因此缺失信号贡献 0，不会获得额外优势。
- 最高分相同时随机选择。没有健康候选、metrics 损坏或 hostcall/override 失败时 fail-open，由 Envoy 默认负载均衡继续处理。

KV cache 优先读取 `vllm:kv_cache_usage_perc`，并兼容旧名称 `vllm:gpu_cache_usage_perc`。LoRA 指标 `vllm:lora_requests_info` 可以缺失。

## 配置

```yaml
profile: balanced
weights:
  queue: 2
  kvCache: 2
  loraAffinity: 1
  inflight: 1
  failure: 1
feedback:
  ewmaAlpha: 0.2
picker:
  mode: max-score
debug:
  sampleRate: 0
```

当前仅支持 `balanced` profile 和 `max-score` picker。权重必须是非负有限数且至少一项大于 0；`ewmaAlpha` 范围为 `(0,1]`；`sampleRate` 范围为 `[0,1]`。省略字段时使用上面的默认值。

## Feedback 与可观测性

override 成功后，插件按 endpoint 维护 gateway-local inflight；stream 完成时记录 TTFT、总时延与 failure EWMA。每个请求使用独立 lease，重复的完成回调不会重复扣减 inflight 或更新 EWMA。已不在当前 upstream host 集合且 inflight 为 0 的状态会被清理。

插件提供以下固定名称、无 endpoint label 的指标：

- `ai_endpoint_picker_decisions_total`
- `ai_endpoint_picker_fallback_total`
- `ai_endpoint_picker_missing_signal_total`
- `ai_endpoint_picker_feedback_total`
- `ai_endpoint_picker_inflight`

采样 debug 日志只包含候选数量、选中分数与缺失信号数量，不记录 prompt、token、完整 endpoint 或动态错误文本。
