# Test-only OAuth V0 feasibility probe

This directory is not an OAuth production plugin or a production mechanism
selection. It is authorized only by Design section 15 and TASK-5403003:
https://github.com/higress-group/higress/issues/5403#issuecomment-7078370725

The same parser/catch source is built into an ordinary native reference and
two proxy-Wasm modules. The default module uses SDK defaults; the candidate
passes `-fwasm-exceptions` through the SDK's target-local `copts` and `linkopts`
interfaces and enables emsdk's target-local `exceptions` feature to remove its
trailing `-fno-exceptions`. The feature emits a generic `-fexceptions` compiler
switch; native Wasm EH remains explicitly selected at both compile and link.
The default target cannot compile try/catch under `-fno-exceptions`, so only its
local catch syntax is conditional on `__cpp_exceptions`; the parser and verify
operations remain identical. The SDK macro does not propagate `testonly` to its wrapper rules,
so Wasm targets remain isolated here with a test-only tag and are never wired
into official plugin build/release discovery. No shared flags or dependencies
are changed.

Only the probe translation unit instantiates the inline pinned jwt-cpp parser
and catches its errors. This does not prove arbitrary exception unwinding
through separately compiled SDK or crypto libraries. The fixture corpus tests
the actual decoder, claim access, signature verification, non-object JSON and
numeric overflow, with valid signed controls after every invalid input.

Run the dedicated read-only GitHub Actions workflow, not local commands.
It records actual compile/link commands, parameter files, execution logs,
module features/imports, response bytes, upstream counters and project cleanup.
Unsupported results are reported, not repaired by changing the engine/parser.
The production OAuth source/target and the blocking QUESTION stay unchanged.

## V1 diagnostic-settings contrast

Design section 16 and TASK-5403004 authorize one additional test-only contrast:
https://github.com/higress-group/higress/issues/5403#issuecomment-7082036311

`candidate_eh.wasm` stays unchanged as the diagnostic-on control.
`candidate_eh_nodiag.wasm` differs only by its target name and paired local
linkopts `-sASSERTIONS=0 -sEXCEPTION_STACK_TRACES=0`. Both retain the SDK feature,
compile/link native EH flags and existing `-O0` linking. Disabling assertions
also loses other target assertion/default debug checks and exception stack
diagnostics: it is not a zero-cost removal of one import.

The dedicated workflow at `oauth-v0-feasibility.yaml` now runs the V1 pair on
one exact head/runner with the same native reference, 23 fixtures, configuration
and pinned images. It uses a separate `runtime_v1.py` proof gate, not a weakened
V0 verdict. The original V0 runtime entry point and historical evidence retain
their original meaning. The original diagnostic-on load failure remains a
failure in V1 evidence, never a successful runtime control. V1 requires actual
expanded compile/link actions, both current-run module identities, unchanged control source/target,
two complete nodiag corpus rounds with immediate valid-after controls, per-case
probe logs, no traps, final artifact/host/container byte equality and checked
cleanup command outcomes. Import disappearance or readiness alone cannot pass.

Design section 17 authorizes a minimal harness-only closure: historical control
SHA equality is retained as a non-gating diagnostic with an unknown cause for
differences; cross-run bit-identical rebuilds are not promised. This does not
claim semantic equivalence or relabel earlier failed/PARTIAL CI. Every current-run
input/action/artifact/helper/tag/runtime/recovery/cleanup proof remains a hard gate.

No production mechanism, cross-library unwinding, arbitrary runtime version,
nine-plugin acceptance or release is authorized by a successful V1 probe.
