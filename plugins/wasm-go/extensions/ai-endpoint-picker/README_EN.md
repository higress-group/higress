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

Prefix locality supports text inputs for OpenAI Chat Completions and Completions while excluding output parameters such as temperature and max tokens. `prefix.toolMode` controls tool-prefix precision; every canonical Chat message, including role, content, name, tool calls, and other complete prompt-relevant fields, still forms an ordered semantic segment. Completions text and flat token-ID prompts form independent chains. A valid batched token-ID prompt such as `[[1,2],[3,4]]` currently makes only the prefix scorer unavailable, so queue/KV scheduling continues; mixed or invalid prompt shapes remain invalid input. Canonical JSON nesting is capped at 64, and exceeding that depth or the node budget also disables only the prefix scorer. Every four UTF-8 bytes estimate one pseudo-token. Segments are split according to `prefix.blockSizeTokens` (1024 by default) under the existing 131072-pseudo-token hard cap. `prefix.maxBlocks` additionally caps blocks across the entire request, including tools, messages, Completion string arrays, and flat token IDs. Once exhausted, extraction preserves the approximate prefix already emitted and stops later semantic work. Hashes include `model`, `cache_salt`, segment kind and length, content hash, and the preceding hash, so a changed middle segment prevents later segments from matching. Non-text multimodal input makes only the prefix scorer unavailable, so queue and KV scorers continue to work.

Each endpoint has a thread-safe weighted LRU whose capacity is measured in approximate backend KV blocks. The default is 31250 and a valid `vllm:cache_config_info{num_gpu_blocks=...}` overrides it. Each semantic entry costs `ceil(segmentTokens/actualBlockSize)` incrementally; the selected endpoint's valid `block_size` is used, with a fallback of 16. Scoring does not refresh the LRU.

This approximate index is local to the current WASM runtime/config instance and does not represent the backend's real KV cache. It records a request for the selected endpoint only after override-host succeeds. Prefix-chain insertion preserves the chain head and evicts suffix entries first under capacity pressure. It removes endpoints that disappear from the current host snapshot or become unhealthy; a healthy endpoint without `num_gpu_blocks` retains the default capacity.

## Upstream metrics contract

The queue, KV, and LoRA scorers require a Prometheus snapshot from each actual inference endpoint. Configure an explicit HTTP health check for every upstream host (normally the vLLM `/metrics` endpoint) and enable `store_metrics: true`, so the health-check response body is returned as that host's `metrics` by `GetUpstreamHosts()`. Do not substitute metrics aggregated behind a shared load-balancer address for per-host snapshots. The plugin caches compact host snapshots for 250ms, avoiding repeated host reads within that window and allowing signals to be up to 250ms stale. A failed refresh never serves an expired snapshot and instead fails open. On refresh, it first filters the five queue, current/legacy KV, LoRA, and cache-config families and fingerprints only that relevant subset. Unrelated metric churn therefore does not trigger another Prometheus parse. The relevant subset is capped at 64KiB; malformed or over-limit relevant data isolates only that host, and unrelated families never build DTOs.

The plugin cache retains neither raw metadata nor the metrics response body. However, `store_metrics: true` still makes Envoy retain and expose the complete raw health-check response; a plugin-only change cannot remove that upstream storage and hostcall-copy cost.

## Configuration

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

The plugin supports the `default` profile, the equivalent `balanced` alias, and the `max-score` picker. Queue, KV-cache, and prefix-cache weights retain the llm-d router defaults. Higress additionally enables gateway-local inflight with weight 1 by default, steering requests away from endpoints that this gateway has already assigned more unfinished work between `/metrics` updates. Its lower weight makes it a real-time correction rather than a replacement for queue, KV, or prefix signals; set `inflight: 0` explicitly to disable it. LoRA and failure default to weight 0, and flow control remains disabled. Weights must be finite non-negative numbers with at least one value greater than zero. `ewmaAlpha` must be in `(0,1]`, and `sampleRate` must be in `[0,1]`.

`prefix.toolMode` offers three explicit trade-offs between gateway work and approximate-prefix precision. Unknown values reject the plugin configuration:

- `identity` (default) hashes only ordered `type` and `function.name` values for at most 64 tools and an 8192-byte total identity budget. It never recursively canonicalizes descriptions, parameters, or complete schemas. This is suitable for most agents with stable tool names; a schema-only change under the same name can produce an approximate scheduling false hit.
- `none` ignores top-level `tools` completely and has the lowest tool-processing cost. Use it when a route's tool set is fixed or gateway cost matters most. Requests with the same messages but different tools then share an approximate fingerprint, so dynamic-tool workloads can more often select a node without the real KV prefix.
- `full` canonically hashes complete tools JSON under the existing depth, node, and token limits. Use it when tool definitions change dynamically and closer chat-template simulation is worth additional CPU and temporary allocation.

When `identity` reaches either budget it preserves the prefix already produced and ignores remaining tools. All three modes affect only the gateway scheduling hint. The inference engine still verifies real token/KV Cache matches, so an approximate false hit cannot change model-output correctness.

`prefix.maxBlocks` defaults to 32 and accepts 1..128. Smaller values stop tools/messages or Completion prompt extraction sooner, reducing gateway CPU and temporary memory at the cost of a shorter locality hint. Larger values improve approximation for long contexts with more hashing work. This is a request-wide operating budget and does not replace the existing token, JSON-depth, or canonical-node hard limits.

`prefix.blockSizeTokens` defaults to 1024 and accepts 1..1024. It caps the pseudo-tokens in one approximate hash block for long text and token-ID prompts. Smaller values improve prefix-match granularity but shorten the maximum observable prefix when `maxBlocks` is unchanged. With 32 blocks, values of 64 and 128 cover about 2048 and 4096 pseudo-tokens, respectively. This setting does not change the actual KV `block_size` reported by vLLM `/metrics`.

`prefix.maxCacheBlocksPerEndpoint` defaults to 31250 and accepts 1..1048576. It is both the fallback capacity when `num_gpu_blocks` is absent and the upper bound applied to reported capacity, preventing abnormal or very large metrics from growing the gateway-local weighted LRU without limit. Capacity is measured in approximate backend KV blocks; total resident memory also depends on endpoint count.

`limits.maxRequestBodyBytes` defaults to 4 MiB and accepts 1 byte..100 MiB. The plugin buffers only requests with a trustworthy positive `Content-Length` at or below this value. Oversized requests and unknown-length chunked requests skip endpoint picking and continue unchanged instead of receiving a 413 response. This bounds the optional picker's body buffering without affecting model availability.

`limits.vmRebuildThresholdBytes` defaults to 200 MiB and accepts 0..4 GiB; zero disables it. At the threshold the plugin asks Higress to rebuild the current WASM VM, matching ai-proxy's 200 MiB policy. This is a soft rebuild threshold for leak/fragmentation recovery, not a hard memory quota; hard limits remain the responsibility of the gateway container and WASM runtime.

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

Control-plane integration is provided by [#4608](https://github.com/higress-group/higress/pull/4608) and [higress-group/istio#69](https://github.com/higress-group/istio/pull/69). With GIE v1.4, a normal core `Service` reference continues to use external EPP/ext_proc. Only the following Higress well-known reference generates and binds the built-in plugin for the corresponding InferencePool routes, with route rules kept isolated:

```yaml
endpointPickerRef:
  group: extensions.higress.io
  kind: WasmPlugin
  name: ai-endpoint-picker
```

The controller generates route matching and the plugin reference, so omitted tuning fields inherit the defaults above. When GIE BuiltIn mode binds the plugin, the controller also configures the final inference cluster with exactly one `/metrics` health check and `store_metrics: true`; this replaces other health checks on that cluster. Standalone WasmPlugin deployments must still configure these upstream metrics capabilities explicitly.
