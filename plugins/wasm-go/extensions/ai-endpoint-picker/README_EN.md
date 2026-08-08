# ai-endpoint-picker

`ai-endpoint-picker` selects one healthy LLM endpoint for an OpenAI-compatible request within a single upstream cluster. It uses the upstream host metrics and override-host capabilities exposed by Higress. It does not deploy a separate Endpoint Picker service and does not implement cross-cluster routing, flow control, or admission control.

## Scheduling pipeline

The plugin runs `Filter → Normalize → Score → Pick → Feedback`:

- Filter removes only unhealthy endpoints. A missing optional metric never hard-filters an otherwise healthy endpoint.
- Queue depth and gateway-local inflight are min-max normalized across healthy candidates, with lower values scoring higher.
- KV cache and failure use `1-utilization` and `1-EWMA`, respectively.
- When LoRA metrics are available, an endpoint scores 1 if the request `model` is already active and 0 otherwise.
- Every scorer produces a value in `[0,1]`. The final score always uses the sum of configured weights as its denominator, so a missing signal contributes zero and receives no advantage.
- Tied maximum scores are picked randomly. When there is no healthy candidate, metrics are malformed, or a hostcall/override fails, the plugin fails open and leaves selection to Envoy's default load balancer.

The KV cache signal prefers `vllm:kv_cache_usage_perc` and supports the legacy `vllm:gpu_cache_usage_perc`. The `vllm:lora_requests_info` family is optional.

## Configuration

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

The first version supports only the `balanced` profile and `max-score` picker. Weights must be finite non-negative numbers with at least one value greater than zero. `ewmaAlpha` must be in `(0,1]`, and `sampleRate` must be in `[0,1]`. Omitted fields use the defaults above.

## Feedback and observability

After an override succeeds, the plugin maintains gateway-local inflight state for the selected endpoint. At stream completion it records TTFT, total latency, and failure EWMA. Each request owns an isolated lease, so repeated completion callbacks cannot decrement inflight or update EWMA twice. State for endpoints absent from the current upstream host set is removed once its inflight count reaches zero.

The plugin exposes fixed-name metrics without endpoint labels:

- `ai_endpoint_picker_decisions_total`
- `ai_endpoint_picker_fallback_total`
- `ai_endpoint_picker_missing_signal_total`
- `ai_endpoint_picker_feedback_total`
- `ai_endpoint_picker_inflight`

Sampled debug logs contain only candidate count, selected score, and missing-signal count. They do not contain prompts, tokens, full endpoint addresses, or dynamic error text.
