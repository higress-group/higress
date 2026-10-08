# Official SDK interoperability

Run `bash testdata/interop/run.sh` from the `mcp-server` extension directory.
The script runs the fixture's Go tests, builds and starts the native test host,
exercises the pinned Go and TypeScript clients, and checks the final fixture
verdict. The trap stops and reaps the directly launched host binary; failed runs
retain its diagnostics.

Both SDKs require the auto probe failure to expose JSON-RPC `-32020`; an
arbitrary transport error is a failure. `check_probe.py` separately requires
HTTP 400, the original request ID, and that same error code. It also checks the
host's sticky failure count, so a failed sequence assertion cannot disappear
when the next request succeeds or an SDK catches the resulting HTTP 500.

`host/main_test.go` tests this checker through real local HTTP requests. Its
negative fixture advertises successful discovery on the expected-error path,
causing the unchanged plugin to issue a real `tools/call` hostcall. The fixture
rejects that extra callout and the checker must exit 42 even after a later
request returns the expected protocol error. This is a test of the oracle, not
an injected production-code change or a synthetic callout ledger entry.

The host executes production plugin callbacks in the proxy-Wasm test emulator.
Real Envoy/Wasm and kind coverage remain separate verification layers.
