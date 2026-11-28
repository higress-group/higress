<!--
Copyright (c) 2026 Alibaba Group Holding Ltd.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

# Streaming denial verification for #5220

`verify_runtime.py` compares the same request-denial configuration and requests
through the real Higress Envoy proxy-Wasm data path. It reuses the pinned images,
fixture, logging and cleanup in `../ai-data-masking-tool-arguments/runtime_lab.py`.
No model service or API key is used.

## Build and rerun

The recorded Linux/amd64 run uses Rust 1.85.1, the wasm32-wasip1 target and
WASI SDK 25.0's clang/llvm-ar. Set `CC_wasm32_wasip1` and `AR_wasm32_wasip1` to
those binaries. Docker and Python 3.12+ are needed for the runtime comparison.

From the repository root, build each revision in a separate exported source
directory and copy its result outside the checkout:

```sh
cargo +1.85.1 build --locked --release --target wasm32-wasip1 \
  --manifest-path /path/to/export/plugins/wasm-rust/extensions/ai-data-masking/Cargo.toml
# Preserve each target/wasm32-wasip1/release/ai_data_masking.wasm as baseline.wasm or candidate.wasm.

python3 verification/ai-data-masking-stream-denial/verify_runtime.py \
  --baseline-wasm /absolute/path/baseline.wasm \
  --candidate-wasm /absolute/path/candidate.wasm \
  --baseline-source 57e2786849516cde6c71206e303f03aab364093e \
  --candidate-source 8bdd6852af0663d48744eb582d2ce08a494d1153 \
  --output /absolute/path/new-evidence-directory \
  --resource-prefix pr5220-owned-run
```

The exact export/build commands are retained in the fork's
[verification workflow](https://github.com/lljjcc426/higress/blob/c40274f470578c9b2fe4c1e6c7bd73a9541d0313/.github/workflows/pr5220-verification.yaml).
That workflow is on an independent fork branch and is excluded from this PR.

## Recorded results

Verified on 2026-11-16: [successful run, attempt 2](https://github.com/lljjcc426/higress/actions/runs/43881309562/attempts/2),
[downloadable evidence](https://github.com/lljjcc426/higress/actions/runs/43881309562/artifacts/14232922268) (90-day retention).
The first attempt of this run failed to pull the gateway after an Aliyun
connection reset; the successful retry used the same source, images, inputs
and assertions.

| Variant | Streaming requests | JSON control | Plain-text control |
| --- | --- | --- | --- |
| Baseline | 3/3 reproduce zero DONE markers | passed | passed |
| Fixed | 3/3 end with exactly one DONE marker | passed | passed |

All ten requests returned HTTP 403 and did not reach the synthetic upstream.
The JSON/plain-text response hashes match between the two versions.
`cleanup.json` confirms all three containers and the Docker network were removed
and both published ports were closed.

```text
baseline source: 57e2786849516cde6c71206e303f03aab364093e
fixed source:    8bdd6852af0663d48744eb582d2ce08a494d1153
baseline Wasm SHA-256: f6afef48531d0668a3aabe74df1844e48dc4423461e58715aa6d8328af14cb4e
fixed Wasm SHA-256:    ea7dccd6b8b49df26f5e841e0901a7ef36c94b30b5587b2e1b4d073f5df905a9
uploaded evidence ZIP SHA-256: 3a11c437abc43feaef131e9fa66f0966e1e7e8d47444b6cff3cd6dca337ddd5a
```


The gateway image is pinned to:

```text
higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway@sha256:a82de493902012ce6b555ecb2244156f3c73824474d67549144a7efa334d0299
```

The shared synthetic-upstream fixture image is pinned to:

```text
higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/wasm-rust-builder@sha256:31758209b0ab11bb81db4c07789d1b93072c46f63ecb10f1ee88863baf659d90
```

Plugin configuration, identical for both versions:

```json
{"deny_openai":true,"deny_raw":true,"deny_words":["BLOCKED"],"deny_code":403,"deny_message":"blocked","deny_raw_message":"raw-denial","deny_content_type":"text/plain"}
```

Each version receives three identical streaming requests and two controls:

- JSON body `{"stream":true,"messages":[{"role":"user","content":"BLOCKED"}]}`.
  Expect HTTP 403, SSE content type and text `blocked`. The baseline reproduces
  zero DONE markers; the candidate must end with exactly one `data: [DONE]\n\n`.
- The same JSON body with stream=false. Expect HTTP 403, application/json,
  message content `blocked` and no DONE marker.
- Plain-text body `BLOCKED`. Expect HTTP 403, text/plain, body `raw-denial`.

Envoy can emit a usage event before the text event while processing its own
local reply. The assertions parse all SSE events and concatenate the content;
they do not assume that the first event carries text. A missing end marker is
recorded as a failure of the fixed streaming contract on the baseline, rather
than treating transport closure as protocol completion.

Evidence includes exact source and Wasm identities, Envoy configuration,
request/response bytes and headers, response hashes, access/plugin logs,
stats/config dump, results.json, SHA256SUMS and cleanup.json. The harness removes
its containers/network and asserts that its published ports have closed.
This covers request-denial behavior with a synthetic upstream, not Kubernetes
installation, response-side denial, live-model compatibility or capacity.

## Other checks and changed files

On macOS 26.6/arm64 with Rust 1.100.0, the resolved source passed
`cargo test --locked --lib`: **95 passed**. The current upstream's native
hostcall test support removes the earlier unresolved-symbol blocker.
`cargo fmt --check` also passed. `cargo clippy --locked --all-targets` completes
with existing upstream warnings; treating every warning as an error fails on
unchanged upstream code. This change does not modify those unrelated sites.

The PR changes `ai_data_masking.rs` to append the streaming terminator and adapt
the two existing PR regression tests to the current constructor. All upstream
tool-argument and content-parsing changes/tests are retained. This directory
adds the runtime verifier and these rerun/results notes. No existing repository
files were deleted; an uncommitted Wasmtime-only draft was replaced by this
Envoy-based verifier.
