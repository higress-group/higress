# agentic-router

A learning-based model-tier routing plugin: routes requests across fast/mid/strong model tiers by task type and complexity, and learns online from a quality reward (status / latency / usage cost) so routing improves with usage.

Division of labor vs. static `modelMapping` / model-routing plugins: this plugin makes the **tier decision** (and learns it); actual forwarding is handled by existing plugins such as ai-proxy.

## How it works

```
request body → task classification (<1ms) → 4 experts (rule / UCB / LinUCB / k-NN)
             → EXP3 meta-selector sampling → tier resolution → [active] rewrite body.model
response     → status + latency + usage cost → quality reward → feed experts + EXP3 (flywheel)
```

- **Cold start**: rule-table fallback (complexity thresholds); experts take over as they "graduate" (LinUCB at 50 samples / k-NN at 30 experiences)
- **Shadow mode** (default): log decisions without rewriting the model; switch `shadow: false` once enough data is collected
- **Explainable**: every decision is logged with its reason; `x-agentic-router-tier` / `x-agentic-router-expert` response headers are injected
- **Robustness** (validated by simulated replay): EXP3 with γ-mixing and weight explosion guard, plus tier-level ε exploration, avoiding meta-selector lock-in and exploration starvation

## Configuration

| Field | Type | Default | Description |
|---|---|---|---|
| `shadow` | bool | `true` | Log only, no rewriting |
| `crossProvider` | bool | `true` | Fall back to other providers when a tier has no candidate |
| `tierModels` | map | required | tier → model name, e.g. `{fast: "qwen-flash", mid: "qwen-plus", strong: "qwen-max"}` |
| `prices` | map | built-in generic | CNY per million tokens: `{fast: {in: 0.5, out: 2}}` |

Example:

```yaml
shadow: true
tierModels:
  fast: "qwen-flash"
  mid: "qwen-plus"
  strong: "qwen-max"
```

## Build

```bash
cd plugins/wasm-go/extensions/agentic-router
go test ./core/...                       # golden parity tests for the core
tinygo build -o plugin.wasm -target=wasi ./
```

## Known limitations

- Streaming (SSE) responses without a final aggregated usage frame: cost is recorded as 0 (latency/status signals still apply)
- shared data across workers is last-writer-wins; externalized state is planned for multi-replica deployments
- Tier-level ε exploration (default 0.08) routes to non-first-choice tiers with small probability — the necessary price of online learning

The cross-language algorithm spec and simulated-replay convergence reports live in [agentic-router-gateway](https://github.com/lhwwxy/agentic-router-gateway).
