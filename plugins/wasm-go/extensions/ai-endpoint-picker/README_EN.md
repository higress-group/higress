# ai-endpoint-picker

`ai-endpoint-picker` selects one healthy LLM endpoint for an OpenAI-compatible request within a single upstream cluster. It uses the upstream host metrics and override-host capabilities exposed by Higress. It does not deploy a separate Endpoint Picker service and does not implement cross-cluster routing, flow control, or admission control.

## Scheduling pipeline

The plugin runs `Filter → Normalize → Score → Pick → Feedback`:

- Filter removes only unhealthy endpoints. A missing optional metric never hard-filters an otherwise healthy endpoint.
- Queue depth and gateway-local inflight are min-max normalized across healthy candidates, with lower values scoring higher.
- KV cache and failure use `1-utilization` and `1-EWMA`, respectively.
- Approximate prefix cache scores the longest consecutive prefix observed by this WASM instance as `matched blocks / total blocks`; a cold endpoint scores 0.
- When LoRA metrics are available, an endpoint scores 1 if the request `model` is already active and 0 otherwise.
- Every scorer produces a value in `[0,1]`. The final score always uses the sum of configured weights as its denominator, so a missing signal contributes zero and receives no advantage.
- Tied maximum scores are picked randomly. When there is no healthy candidate, metrics are malformed, or a hostcall/override fails, the plugin fails open and leaves selection to Envoy's default load balancer.

The KV cache signal prefers `vllm:kv_cache_usage_perc` and supports the legacy `vllm:gpu_cache_usage_perc`. The `vllm:lora_requests_info` family is optional.

Prefix locality supports text inputs for OpenAI Chat Completions and Completions, including tools, roles, text prompts, and token-ID prompts. Output parameters such as temperature and max tokens are excluded. The estimate tokenizer packs every four UTF-8 bytes into one pseudo-token. Block hashes are namespaced by `model` and `cache_salt` and chained to the preceding block. The effective block size defaults to 64 tokens and can be raised by a valid `block_size` label on the first healthy candidate's `vllm:cache_config_info` metric; at most 131072 tokens are indexed. Each endpoint has a thread-safe 31250-block LRU by default, and a valid `num_gpu_blocks` label overrides that capacity. Non-text multimodal input makes only the prefix scorer unavailable, so queue and KV scorers continue to work.

This approximate index is local to the current WASM runtime/config instance and does not represent the backend's real KV cache. It records a request for the selected endpoint only after override-host succeeds and removes endpoints absent from the current host snapshot.

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

Sampled debug logs contain only candidate count, selected score, and missing-signal count. They do not contain prompts, tokens, full endpoint addresses, or dynamic error text.
