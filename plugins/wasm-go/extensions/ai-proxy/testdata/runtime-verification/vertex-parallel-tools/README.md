# Vertex parallel tool-call runtime regression

`verify.py` loads an already-built `ai-proxy` Wasm module into an actual Envoy
process and exercises the OpenAI-to-Vertex Express conversion against a local
strict HTTP fixture. It requires Python 3 and an Envoy binary with the V8
proxy-Wasm runtime, such as the Higress gateway binary. It uses no Docker,
external service, Google account, or real credential. Every address is loopback;
the static upstream cluster cannot connect to the rewritten Google hostname.

```sh
python3 verify.py --envoy /path/to/envoy --wasm /path/to/baseline.wasm \
  --output /path/to/new-baseline-evidence --expect baseline
python3 verify.py --envoy /path/to/envoy --wasm /path/to/fixed.wasm \
  --output /path/to/new-fixed-evidence --expect fixed
```

Use Linux (including WSL) when the Envoy binary is a Linux executable. The script
uses only Python's standard library and dynamically allocates loopback ports.
Run directories must not already exist, so failed runs remain available.

Each of 13 scenarios runs three times by default (`--repeat` overrides this):

- Non-streaming parallel calls, text mixed with calls, and same-name calls with
  different arguments.
- Streaming calls in one SSE event, across events, and across deliberately small
  flushed HTTP chunks (including within UTF-8 and JSON). A separate terminal event
  checks that `finish_reason: tool_calls` survives beyond the call event.
- The client assembles the received assistant message, executes deterministic
  fixture tools, and sends results in reverse order using the received call IDs.
  The upstream checks every call and argument, signature ownership, assistant
  text, one user turn for results, and result ordering by original call ID.
- Two independent request cases use a canonical assistant message so a response
  conversion failure cannot hide request conversion defects. These exercise the
  Google `extra_content` thought signature form.
- Single-call and text-only controls in both streaming and non-streaming modes.

The default remains these original 13 scenarios. Use the repeatable `--scenario`
option to run only selected scenarios. The additional `sse-two-tool-rounds`
scenario is opt-in:

```sh
python3 verify.py --envoy /path/to/envoy --wasm /path/to/fixed.wasm \
  --output /path/to/new-two-round-evidence --expect fixed \
  --scenario sse-two-tool-rounds
```

This scenario uses candidate 0 throughout and makes three real HTTP requests per
repetition. The first stream calls `weather(Hangzhou)` once. After the client
returns that result using the received ID, the second stream calls
`weather(Hangzhou)` again and `weather(Shanghai)`. Its two calls arrive in one
event followed by a separate terminal event. Both streams must restart their
tool indexes at zero, preserve their own signature on the first call, issue
distinct call IDs, and terminate with `tool_calls` and one `[DONE]`.

The client replays the received messages and sends the second round's results in
reverse order. Result payloads contain the execution round so identical function
names and arguments cannot hide replacement of earlier results. The final
upstream request must have five turns (`user`, `model`, `user`, `model`, `user`),
preserve the first round's call, signature and result, and associate the second
round's results with its own calls. Each round's assembled message and all three
HTTP exchanges are recorded; cleanup uses the same lifecycle as the default set.
No missing call is synthesized for the baseline: its supported first single call
allows it to reach the second round, where the strict fixture can reject the
incomplete replay. Run the same selection with `--expect baseline` to reproduce it.
For this scenario, baseline acceptance additionally requires failures in both
second-round client call completeness and the final upstream history's
second-round call completeness. A missing finish reason alone is insufficient.

`--expect fixed` requires every assertion to pass. `--expect baseline` succeeds
only when every regression scenario reproduces a behavioral failure, while the
transport, parsing, and stable control checks pass. Baseline single-call responses
also record the existing `STOP` (non-streaming) or missing (streaming) finish
reason versus `tool_calls` mismatch as a behavioral failure. The text-only stream
control permits an absent finish reason, which is outside the tool-call fix.
A success exit code in baseline mode therefore means the defect was
reproduced, not that the behavior was correct. Infrastructure exceptions always
fail either mode.

Evidence includes complete client/upstream requests and response bytes, assembled
assistant messages, source HTTP chunk schedules, client read timing, Envoy config,
logs and stats, binary version/path, and `summary.json` with actual/expected values.
`cleanup.json` records shutdown of the owned Envoy process and fixture server.
The dummy API key is intentionally visible. No content digests are generated.

This verifies real Envoy/proxy-Wasm conversion for complete native Vertex
`functionCall` objects. It does not call the live Vertex service, exercise OAuth,
the Kubernetes control plane, or incremental native function-argument streaming.
HTTP chunk boundaries may be coalesced by the network stack; the fixture records
its exact writes rather than claiming a one-to-one proxy callback boundary.
