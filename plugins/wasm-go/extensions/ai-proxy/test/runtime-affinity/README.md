# Persistent API Key affinity: real Envoy regression

This isolated harness uses the **real Higress 2.2.6 Envoy**, a mock OpenAI-compatible upstream and an independent Redis. It never reads deployment credentials or modifies a running gateway. All consumers, Keys and responses are synthetic. Docker containers/networks are uniquely named; only objects created by this invocation are removed. Test ports are published on loopback only.

## Regression and assertions

Before the fix, `LoadApiKeyAffinity` returned `HeaderStopIteration` (numeric 1). Request body callbacks could continue while Redis was pending, allowing a provisional Key to be sent. The fix returns `HeaderStopAllIterationAndWatermark` (numeric 4) and resumes only after the lookup callback selects and applies the Key.

With a **100 ms delay** on gateway-to-Redis writes, the same harness/input asserts:

1. First success establishes one Redis binding; five following requests use the same Key.
2. The mock upstream then returns 403 for that Key; a single client request results in two upstream attempts with the same request ID and identical non-empty body SHA-256.
3. The second attempt succeeds with a different Key; Redis eventually stores that replacement (bounded 5-second wait for the asynchronous write), and the next request uses it.
4. SSE contains `data:` and `[DONE]` and uses the replacement Key.
5. Stopping the isolated Redis does not block a successful upstream request.
6. Created containers and networks are removed, including on assertion failure.

This is not a production-provider failure test. The real Envoy retry case uses **403**; other eligible status codes and retry budgets remain covered by Go tests. The current payload is about 12 KiB; the 1 MiB replay limit is not claimed as runtime boundary-tested here. Cleanup assertions do not delete the output evidence directory.

## Requirements

Python 3.10+ (standard library), Linux Docker CLI/daemon access, and built baseline/fixed Wasm modules. On Windows use a Linux shell with reachable loopback-published Docker ports. There are no bind mounts or production ports. Each container is memory/CPU constrained.

```bash
export GATEWAY='higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway@sha256:5d0618399fa98b9f4ad3c34b58f084b51624e3cc4eccdde72554b636d16b5da4'
docker pull "$GATEWAY"
docker pull python:3.12-slim
docker pull redis:7-alpine
docker image inspect --format '{{json .RepoDigests}}' "$GATEWAY" python:3.12-slim redis:7-alpine

# Build from the plugin root; keep generated modules/evidence out of Git.
OUT=$(mktemp -d)
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o "$OUT/fixed.wasm" .
sha256sum "$OUT/fixed.wasm"

python3 test/runtime-affinity/verify_affinity_runtime.py \
  --wasm "$OUT/fixed.wasm" --gateway-image "$GATEWAY" \
  --redis-image redis:7-alpine --redis-delay-ms 100 --expect-affinity \
  --output "$OUT/fixed-1"
# Repeat with new output paths fixed-2 and fixed-3; each output path must not exist.
```

To reconstruct the pre-fix baseline from this PR's tested tree, copy the plugin and its local `pkg/jsonbody`/`pkg/pathutil` dependencies into a separate temporary build directory. Change **only** the successful asynchronous Redis-dispatch return in `onHttpRequestHeader` from `HeaderStopAllIterationAndWatermark` back to `ActionPause`; leave all other code/configuration unchanged, then build `baseline.wasm`. Run the identical command with `--wasm "$OUT/baseline.wasm" --output "$OUT/baseline"`. The expected result is exit 1 with `consumer changed API Key`, not a setup/startup error. Never perform that mutation in a deployment source tree.

For exact historical inputs in the checked-in evidence, the pre-fix merged source tree is `8236fc12603ea668d3c99dc24b9f6a3d3b59cb92` (PR head `cbb777a5ab5dd1785a6b96ee34fdcce36579476b` merged with upstream `0316bc904789ab25a9cd42db58eafca11f6a2205`). It differs from the fixed product source only in the Redis-wait action. The baseline was built from an archive with `-buildvcs=false`; Wasm hashes can differ across paths/VCS metadata even for equivalent source.

## Evidence

Sanitized machine-readable results are in `evidence/`. They retain exact Wasm/image IDs, delay, assertion outcomes and synthetic upstream body hashes/request IDs; no production requests or addresses are included. Output `envoy.json` captures the full plugin/cluster/retry configuration, and `envoy.log` captures runtime diagnostics for local reruns. Raw logs from an installation must not be uploaded without checking for credentials.

Recorded local Go tests and builds use Go 1.26.4 windows/amd64; the runtime comparison uses Linux amd64 and the gateway digest above. The source tree and commands are recorded in the evidence manifest. Host tests alone are not treated as runtime proof.
