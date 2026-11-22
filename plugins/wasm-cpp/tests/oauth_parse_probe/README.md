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
