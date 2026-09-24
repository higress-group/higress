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

prefix locality 支持 OpenAI Chat Completions 和 Completions 的文本输入，不包含 temperature、max tokens 等输出参数。Chat 的工具前缀精度由 `prefix.toolMode` 控制；每条包含 role、content、name、tool calls 等完整字段的 canonical message 仍分别形成有序语义 segment。Completions 文本或平坦 token ID prompt 独立建链。合法 batched token ID prompt（如 `[[1,2],[3,4]]`）当前只把 prefix scorer 标为 unavailable，不影响 queue/KV 调度；混合或无效 prompt 仍按无效输入处理。canonical JSON 最大嵌套深度为 64，超深或超出 node budget 时同样只禁用 prefix scorer。每 4 个 UTF-8 bytes 估算一个 pseudo-token，segment 按 `prefix.blockSizeTokens` 有界切片（默认 1024），并保留 131072 pseudo-token 硬上限。`prefix.maxBlocks` 进一步限制整个请求（tools、messages、Completion 字符串数组或平坦 token IDs 合计）的 block 数；预算耗尽时保留已生成的近似前缀并停止后续语义提取。hash 包含 `model`、`cache_salt`、segment 类型/长度、内容 hash 和前一个 hash，因此中间 segment 变化后不会命中后续 segment。非文本 multimodal 输入只让 prefix scorer unavailable，queue/KV 等 scorer 继续工作。

每 endpoint 的 thread-safe weighted LRU 容量单位是近似后端 KV block，默认 31250，合法 `vllm:cache_config_info{num_gpu_blocks=...}` 可覆盖容量。每个语义 entry 的增量成本为 `ceil(segmentTokens/actualBlockSize)`；actual block size 读取所选 endpoint 的合法 `block_size`，否则使用 16。Score 不刷新 LRU。

这个 approximate 索引只存在于当前 WASM runtime/config 实例，不能代表后端真实 KV cache；只有 override host 成功后才写入所选 endpoint。写入 prefix chain 时优先保留 chain head、容量不足时先淘汰 suffix。已从当前 host snapshot 消失或变为 unhealthy 的 endpoint 会被清理；健康 endpoint 缺少 `num_gpu_blocks` 时继续使用默认容量。

## Upstream metrics 契约

queue、KV 和 LoRA scorer 依赖每个实际 inference endpoint 自己的 Prometheus snapshot。upstream cluster 必须对各 endpoint 配置显式 HTTP health check（通常访问 vLLM `/metrics`），并启用 `store_metrics: true`，使 health-check 响应体作为对应 host 的 `metrics` 由 `GetUpstreamHosts()` 返回。不要使用经共享负载均衡地址聚合的 metrics 代替逐 host snapshot。插件将 host snapshot 缓存 250ms：窗口内不重复读取 host，因而调度信号最多滞后 250ms；过期刷新失败时不使用旧 snapshot，而是 fail-open。刷新时先筛选 queue、当前/旧 KV、LoRA 和 cache config 五类指标，并只对这个相关子集生成指纹；非相关指标变化不会触发 Prometheus 重解析。相关子集上限为 64KiB，相关数据损坏或超限只隔离该 host，大量无关指标不会构建 DTO。

插件缓存不保存原始 metadata 或 metrics body，但 `store_metrics: true` 意味着 Envoy 仍会在 health-check 层保存并通过 hostcall 提供完整原始响应；仅修改 WASM 插件无法消除这部分原始存储和复制开销。

## 配置

```yaml
profile: default
prefix:
  toolMode: identity
  maxBlocks: 32
  blockSizeTokens: 1024
  maxCacheBlocksPerEndpoint: 31250
limits:
  maxRequestBodyBytes: 4194304
  vmRebuildThresholdBytes: 209715200
weights:
  queue: 2
  kvCache: 2
  prefixCache: 3
  loraAffinity: 0
  inflight: 1
  failure: 0
feedback:
  ewmaAlpha: 0.2
picker:
  mode: max-score
debug:
  sampleRate: 0
```

当前支持 `default` profile、同义的 `balanced` profile 和 `max-score` picker。queue、KV cache 与 prefix cache 权重沿用 llm-d router 默认值；Higress 额外以权重 1 默认启用 gateway-local inflight，用于在两次 `/metrics` 更新之间避开已被当前网关压入更多未完成请求的 endpoint。该权重低于 queue、KV 和 prefix，主要作为实时纠偏信号；显式配置 `inflight: 0` 可关闭。LoRA 与 failure scorer 默认权重为 0，flow control 关闭。权重必须是非负有限数且至少一项大于 0；`ewmaAlpha` 范围为 `(0,1]`；`sampleRate` 范围为 `[0,1]`。

`prefix.toolMode` 在网关开销与近似前缀精度之间提供三档选择，未知值会使插件配置失败：

- `identity`（默认）：按原顺序只计算最多 64 个工具的 `type` 和 `function.name`，工具身份总预算为 8192 bytes；不递归 canonicalize description、parameters 或完整 Schema。适合绝大多数工具名称稳定的 Agent 请求。同名工具只修改 Schema 时可能产生调度假命中。
- `none`：完全忽略顶层 `tools`，开销最低。适合某条路由的工具集合固定不变，或用户更重视网关开销；相同 messages 但不同工具集合会共享近似指纹，因此动态工具场景可能更频繁选到没有真实 KV 前缀的节点。
- `full`：对完整 tools JSON 做有深度、节点数和 token 上限的 canonical 计算。适合工具定义经常动态变化、需要尽量贴近 chat template 的场景，但会增加 CPU 和临时内存开销。

`identity` 达到工具数量或身份字节预算后会保留已经生成的近似前缀并停止处理后续工具。三种模式都只影响网关调度提示；推理引擎仍基于真实 token/KV Cache 判断命中，因此近似假命中不会改变模型输出正确性。

`prefix.maxBlocks` 默认为 32，合法范围为 1..128。较小值会更早停止 tools/messages 或 Completion prompt 的语义扫描，从而压低网关 CPU 和临时内存开销，但可能减少 locality 命中长度；较大值可提高长上下文的近似精度，代价是更多哈希工作。它是 request-wide 运行预算，不会改变原有 token、JSON depth 和 canonical node 硬上限。

`prefix.blockSizeTokens` 默认为 1024，合法范围为 1..1024，控制长文本和 token-ID prompt 的单个近似哈希块最多包含多少个 pseudo-token。较小值提高前缀匹配粒度，但在 `maxBlocks` 不变时会缩短可观察的最长前缀。例如保持 32 块时，64 和 128 分别最多覆盖约 2048 和 4096 个 pseudo-token。该配置不改变 vLLM `/metrics` 中的真实 KV `block_size`。

`prefix.maxCacheBlocksPerEndpoint` 默认为 31250，合法范围为 1..1048576。它同时作为缺少 `num_gpu_blocks` 时的默认容量，以及指标上报容量的上限，防止异常或超大指标无限扩大 gateway-local weighted LRU。容量按近似 backend KV block 计量，实际常驻内存还会随 endpoint 数量变化。

`limits.maxRequestBodyBytes` 默认为 4 MiB，合法范围为 1 byte..100 MiB。插件只缓冲具有可信、正数 `Content-Length` 且不超过该值的请求；超限请求和长度未知的 chunked 请求跳过 endpoint picker 并原样转发，而不是返回 413。这样可保证 picker 的可选调度收益不会以无界请求体缓冲为代价。

`limits.vmRebuildThresholdBytes` 默认为 200 MiB，合法值为 0..4 GiB；0 表示关闭。达到阈值时插件请求 Higress 重建当前 WASM VM，与 ai-proxy 的 200 MiB 策略一致。该参数是泄漏/碎片化保护用的软重建阈值，不是强制内存配额；真正的硬上限仍由网关容器和 WASM runtime 管理。

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

控制面集成由 [#4608](https://github.com/higress-group/higress/pull/4608) 和 [higress-group/istio#69](https://github.com/higress-group/istio/pull/69) 提供。GIE v1.4 中，普通 core `Service` reference 继续走外部 EPP/ext_proc；只有以下 Higress 约定 reference 才为对应 InferencePool 路由生成并绑定内置插件，各路由规则相互隔离：

```yaml
endpointPickerRef:
  group: extensions.higress.io
  kind: WasmPlugin
  name: ai-endpoint-picker
```

控制面生成路由匹配和插件引用，省略上述可选调优字段，因此自动继承本节默认值。通过 GIE BuiltIn 模式绑定插件时，控制面还会在最终 inference cluster 上配置唯一的 `/metrics` health check 和 `store_metrics: true`；该配置会替换同一 cluster 上的其他 health check。独立创建 WasmPlugin 时，仍需由部署侧显式配置这些 upstream metrics 能力。
