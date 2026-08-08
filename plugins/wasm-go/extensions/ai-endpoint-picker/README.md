# ai-endpoint-picker

`ai-endpoint-picker` 在单个 upstream cluster 内为 OpenAI 兼容请求选择一个健康的 LLM endpoint。插件直接使用 Higress 提供的 upstream host metrics 和 override host 能力，不部署额外的 Endpoint Picker 服务，也不负责 cluster 间路由、流控或 admission control。

## 调度流程

插件按 `Filter → Normalize → Score → Pick → Feedback` 执行：

- Filter 只排除非健康 endpoint，缺少可选指标不会触发硬过滤。
- queue 和本地 inflight 在健康候选集内做 min-max 归一化，值越小分越高。
- KV cache 和 failure 分别使用 `1-utilization` 与 `1-EWMA`。
- approximate prefix cache 使用本 WASM 实例观察到的已选请求估算最长连续前缀，得分为 `matched blocks / total blocks`；冷启动得 0 分。
- LoRA 指标存在时，当前请求的 `model` 已加载得 1 分，否则得 0 分。
- 每项 scorer 输出 `[0,1]`。总分始终除以固定配置权重之和，因此缺失信号贡献 0，不会获得额外优势。
- 最高分相同时随机选择。没有健康候选、metrics 损坏或 hostcall/override 失败时 fail-open，由 Envoy 默认负载均衡继续处理。

KV cache 优先读取 `vllm:kv_cache_usage_perc`，并兼容旧名称 `vllm:gpu_cache_usage_perc`。LoRA 指标 `vllm:lora_requests_info` 可以缺失。

prefix locality 支持 OpenAI Chat Completions 和 Completions 的文本输入（包括 tools、role、文本 prompt 与 token ID prompt），不包含 temperature、max tokens 等输出参数。估算 tokenizer 每 4 个 UTF-8 bytes 打包一个 pseudo-token；block hash 以 `model` 和 `cache_salt` 隔离命名空间并链接前一 block。有效 block size 默认 64 tokens；首个健康候选的合法 `vllm:cache_config_info{block_size=...}` 可将其提高，最多索引 131072 tokens。每 endpoint 的 thread-safe LRU 默认保存 31250 个 block，合法的 `num_gpu_blocks` label 可覆盖容量。非文本 multimodal 输入只让 prefix scorer unavailable，queue/KV 等 scorer 继续工作。

这个 approximate 索引只存在于当前 WASM runtime/config 实例，不能代表后端真实 KV cache；只有 override host 成功后才写入所选 endpoint，已从当前 host snapshot 消失的 endpoint 会被清理。

## 配置

```yaml
profile: default
weights:
  queue: 2
  kvCache: 2
  prefixCache: 3
  loraAffinity: 0
  inflight: 0
  failure: 0
feedback:
  ewmaAlpha: 0.2
picker:
  mode: max-score
debug:
  sampleRate: 0
```

当前支持 `default` profile、同义的 `balanced` profile 和 `max-score` picker。上述无配置权重与 llm-d router 默认值一致，flow control 关闭。LoRA、inflight 与 failure scorer 保留显式配置能力，但默认权重为 0。权重必须是非负有限数且至少一项大于 0；`ewmaAlpha` 范围为 `(0,1]`；`sampleRate` 范围为 `[0,1]`。

## Feedback 与可观测性

override 成功后，插件按 endpoint 维护 gateway-local inflight；stream 完成时记录 TTFT、总时延与 failure EWMA。每个请求使用独立 lease，重复的完成回调不会重复扣减 inflight 或更新 EWMA。已不在当前 upstream host 集合且 inflight 为 0 的状态会被清理。

插件提供以下固定名称、无 endpoint label 的指标：

- `ai_endpoint_picker_decisions_total`
- `ai_endpoint_picker_fallback_total`
- `ai_endpoint_picker_missing_signal_total`
- `ai_endpoint_picker_feedback_total`
- `ai_endpoint_picker_inflight`

采样 debug 日志只包含候选数量、选中分数与缺失信号数量，不记录 prompt、token、完整 endpoint 或动态错误文本。
