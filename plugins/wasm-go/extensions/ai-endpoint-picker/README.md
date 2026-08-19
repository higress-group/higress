# ai-endpoint-picker

`ai-endpoint-picker` 在单个 upstream cluster 内为 OpenAI 兼容请求选择一个健康的 LLM endpoint。插件直接使用 Higress 提供的 upstream host metrics 和 override host 能力，不部署额外的 Endpoint Picker 服务，也不负责 cluster 间路由、流控或 admission control。

## 调度流程

插件按 `Filter → Normalize → Score → Pick → Feedback` 执行：

- Filter 只排除非健康 endpoint，缺少可选指标不会触发硬过滤。
- queue 和本地 inflight 在健康候选集内做 min-max 归一化，值越小分越高。
- KV cache 和 failure 分别使用 `1-utilization` 与 `1-EWMA`。
- approximate prefix cache 使用本 WASM 实例观察到的已选请求估算最长连续语义前缀。先汇总多 prompt 的连续命中 pseudo-token 数，再计算 `0.75 × matched/total + 0.25 × min(matched/8192,1)²`；冷启动得 0 分。
- LoRA 指标存在时，当前请求的 `model` 已加载得 1 分，否则得 0 分。
- 每项 scorer 输出 `[0,1]`。总分始终除以固定配置权重之和，因此缺失信号贡献 0，不会获得额外优势。
- 最高分相同时随机选择。单个 host 的 address、metadata、health 或 Prometheus 数据损坏时只跳过该候选；所有候选都不可用，或 hostcall/override 失败时才 fail-open，由 Envoy 默认负载均衡继续处理。

KV cache 优先读取 `vllm:kv_cache_usage_perc`，并兼容旧名称 `vllm:gpu_cache_usage_perc`。LoRA 指标 `vllm:lora_requests_info` 可以缺失。

prefix locality 支持 OpenAI Chat Completions 和 Completions 的文本输入，不包含 temperature、max tokens 等输出参数。Chat 的 canonical tools 和每条包含 role、content、name、tool calls 等完整字段的 canonical message 分别形成有序语义 segment；Completions 文本或平坦 token ID prompt 独立建链。合法 batched token ID prompt（如 `[[1,2],[3,4]]`）当前只把 prefix scorer 标为 unavailable，不影响 queue/KV 调度；混合或无效 prompt 仍按无效输入处理。canonical JSON 最大嵌套深度为 64，超深或超出 node budget 时同样只禁用 prefix scorer。每 4 个 UTF-8 bytes 估算一个 pseudo-token，超过 1024 pseudo-tokens 的 segment 有界切片，单 prompt 最多处理 131072 pseudo-tokens。hash 包含 `model`、`cache_salt`、segment 类型/长度、内容 hash 和前一个 hash，因此中间 segment 变化后不会命中后续 segment。非文本 multimodal 输入只让 prefix scorer unavailable，queue/KV 等 scorer 继续工作。

每 endpoint 的 thread-safe weighted LRU 容量单位是近似后端 KV block，默认 31250，合法 `vllm:cache_config_info{num_gpu_blocks=...}` 可覆盖容量。每个语义 entry 的增量成本为 `ceil(segmentTokens/actualBlockSize)`；actual block size 读取所选 endpoint 的合法 `block_size`，否则使用 16。Score 不刷新 LRU。

这个 approximate 索引只存在于当前 WASM runtime/config 实例，不能代表后端真实 KV cache；只有 override host 成功后才写入所选 endpoint。写入 prefix chain 时优先保留 chain head、容量不足时先淘汰 suffix。已从当前 host snapshot 消失或变为 unhealthy 的 endpoint 会被清理；健康 endpoint 缺少 `num_gpu_blocks` 时继续使用默认容量。

## Upstream metrics 契约

queue、KV 和 LoRA scorer 依赖每个实际 inference endpoint 自己的 Prometheus snapshot。upstream cluster 必须对各 endpoint 配置显式 HTTP health check（通常访问 vLLM `/metrics`），并启用 `store_metrics: true`，使 health-check 响应体作为对应 host 的 `metrics` 由 `GetUpstreamHosts()` 返回。不要使用经共享负载均衡地址聚合的 metrics 代替逐 host snapshot。响应体必须是合法 Prometheus exposition；单个 host 数据损坏只会隔离该 host。未满足该契约时，外部 signals unavailable，插件仍可使用其他可用 signal 或 fail-open。

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

采样 debug 日志只包含固定选择原因（`max_score`/`random_tie`）、固定 signal availability bitmask、候选数量、选中分数、缺失信号数量和固定 skip reason bitmask，不记录 prompt、token、body、endpoint 明细或动态错误文本。signal mask 的 bit 0..5 固定对应 queue、KV、prefix、LoRA、inflight、failure；skip mask 的 bit 0..3 固定对应 address、metadata、health、Prometheus 损坏。

## 与 GIE 的边界

Gateway API Inference Extension v1.4 ExternalEPP 支持已由 [#4318](https://github.com/higress-group/higress/pull/4318) 合并。该路径在 `endpointPickerRef` 指向外部 EPP 时继续使用 ext_proc，与本插件互不替代。

本插件 PR 只提供数据面的 endpoint picker，不包含 GIE 控制面自动生成或绑定 `WasmPlugin` 的逻辑。把“未配置 `endpointPickerRef`”解释为 BuiltIn picker 需要后续升级到 GIE v1.5+ 的 optional `endpointPickerRef` API；在该控制面集成完成前，用户必须显式部署并绑定本插件。
