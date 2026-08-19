# ai-endpoint-picker

`ai-endpoint-picker` selects one healthy LLM endpoint for an OpenAI-compatible request within a single upstream cluster. It uses the upstream host metrics and override-host capabilities exposed by Higress. It does not deploy a separate Endpoint Picker service and does not implement cross-cluster routing, flow control, or admission control.

## Scheduling pipeline

The plugin runs `Filter → Normalize → Score → Pick → Feedback`:

- Filter removes only unhealthy endpoints. A missing optional metric never hard-filters an otherwise healthy endpoint.
- Queue depth and gateway-local inflight are min-max normalized across healthy candidates, with lower values scoring higher.
- KV cache and failure use `1-utilization` and `1-EWMA`, respectively.
- Approximate prefix cache estimates the longest consecutive semantic prefix observed by this WASM instance. It aggregates consecutive matched pseudo-tokens across prompts and scores `0.75 × matched/total + 0.25 × min(matched/8192,1)²`; a cold endpoint scores 0.
- When LoRA metrics are available, an endpoint scores 1 if the request `model` is already active and 0 otherwise.
- Every scorer produces a value in `[0,1]`. The final score always uses the sum of configured weights as its denominator, so a missing signal contributes zero and receives no advantage.
- Tied maximum scores are picked randomly. A malformed address, metadata object, health status, or Prometheus snapshot skips only that host. The plugin fails open to Envoy's default load balancer only when every candidate is unusable or a hostcall/override fails.

The KV cache signal prefers `vllm:kv_cache_usage_perc` and supports the legacy `vllm:gpu_cache_usage_perc`. The `vllm:lora_requests_info` family is optional.

Prefix locality supports text inputs for OpenAI Chat Completions and Completions while excluding output parameters such as temperature and max tokens. Canonical tools and each canonical Chat message—including role, content, name, tool calls, and other complete prompt-relevant fields—form ordered semantic segments. Completions text and flat token-ID prompts form independent chains. A valid batched token-ID prompt such as `[[1,2],[3,4]]` currently makes only the prefix scorer unavailable, so queue/KV scheduling continues; mixed or invalid prompt shapes remain invalid input. Canonical JSON nesting is capped at 64, and exceeding that depth or the node budget also disables only the prefix scorer. Every four UTF-8 bytes estimate one pseudo-token; segments longer than 1024 pseudo-tokens are split into bounded slices, and each prompt processes at most 131072 pseudo-tokens. Hashes include `model`, `cache_salt`, segment kind and length, content hash, and the preceding hash, so a changed middle segment prevents later segments from matching. Non-text multimodal input makes only the prefix scorer unavailable, so queue and KV scorers continue to work.

Each endpoint has a thread-safe weighted LRU whose capacity is measured in approximate backend KV blocks. The default is 31250 and a valid `vllm:cache_config_info{num_gpu_blocks=...}` overrides it. Each semantic entry costs `ceil(segmentTokens/actualBlockSize)` incrementally; the selected endpoint's valid `block_size` is used, with a fallback of 16. Scoring does not refresh the LRU.

This approximate index is local to the current WASM runtime/config instance and does not represent the backend's real KV cache. It records a request for the selected endpoint only after override-host succeeds. Prefix-chain insertion preserves the chain head and evicts suffix entries first under capacity pressure. It removes endpoints that disappear from the current host snapshot or become unhealthy; a healthy endpoint without `num_gpu_blocks` retains the default capacity.

## Upstream metrics contract

The queue, KV, and LoRA scorers require a Prometheus snapshot from each actual inference endpoint. Configure an explicit HTTP health check for every upstream host (normally the vLLM `/metrics` endpoint) and enable `store_metrics: true`, so the health-check response body is returned as that host's `metrics` by `GetUpstreamHosts()`. Do not substitute metrics aggregated behind a shared load-balancer address for per-host snapshots. The body must be valid Prometheus exposition; malformed data isolates only that host. If this contract is not met, external signals are unavailable and the plugin uses other available signals or fails open.

## Configuration

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

The plugin supports the `default` profile, the equivalent `balanced` alias, and the `max-score` picker. The zero-configuration weights above match the llm-d router defaults, with flow control disabled. LoRA, inflight, and failure remain explicitly configurable but default to weight 0. Weights must be finite non-negative numbers with at least one value greater than zero. `ewmaAlpha` must be in `(0,1]`, and `sampleRate` must be in `[0,1]`.

## Feedback and observability

After an override succeeds, the plugin maintains gateway-local inflight state for the selected endpoint. At stream completion it records TTFT, total latency, and failure EWMA. Each request owns an isolated lease, so repeated completion callbacks cannot decrement inflight or update EWMA twice. State for endpoints absent from the current upstream host set is removed once its inflight count reaches zero.

The plugin exposes fixed-name metrics without endpoint labels:

- `ai_endpoint_picker_decisions_total`
- `ai_endpoint_picker_fallback_total`
- `ai_endpoint_picker_missing_signal_total`
- `ai_endpoint_picker_feedback_total`
- `ai_endpoint_picker_inflight`

Sampled debug logs contain only a fixed decision reason (`max_score`/`random_tie`), a fixed signal-availability bitmask, candidate count, selected score, missing-signal count, and a fixed skip-reason bitmask. They do not contain prompts, tokens, bodies, endpoint details, or dynamic error text. Signal-mask bits 0..5 are fixed to queue, KV, prefix, LoRA, inflight, and failure; skip-mask bits 0..3 are fixed to malformed address, metadata, health, and Prometheus data.

## GIE boundary

Gateway API Inference Extension v1.4 ExternalEPP support was merged in [#4318](https://github.com/higress-group/higress/pull/4318). When `endpointPickerRef` names an external EPP, that path continues to use ext_proc and is not replaced by this plugin.

This plugin PR provides only the data-plane endpoint picker; it does not add GIE control-plane generation or automatic `WasmPlugin` binding. Interpreting an omitted `endpointPickerRef` as a BuiltIn picker requires a later control-plane integration on the optional `endpointPickerRef` API in GIE v1.5+. Until that integration lands, users must deploy and bind this plugin explicitly.
