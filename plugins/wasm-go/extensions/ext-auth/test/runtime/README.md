# Internal-redirect identity regression

This standalone Linux/Docker harness uses a real Higress Envoy + proxy-Wasm
data path, a loopback-only mock authorization/upstream server, and synthetic
credentials. It does not require a cluster, provider account, or API key.
No deployment configuration or credentials should be added here.

## Verification plan

Build the pre-fix and fixed ext-auth sources with the same Go 1.24.4 toolchain
and unchanged module dependencies. Use the same pinned gateway image, plugin
configuration, and request bytes for both runs.

The mock authorizer accepts only `fixture-client`. The first upstream receives
rewritten provider credentials and returns 503. The custom-response filter
internally redirects to a backup route, where ext-auth must authenticate the
original caller again while the upstream still receives backup credentials.

Three repeated runs of each Authorization/x-api-key-only combination check:

- primary 200: one auth call, no redirect, exact binary response body;
- primary 503: baseline returns 401 and never reaches backup; fixed returns
  200 after two auth calls and reaches backup with correct upstream credentials;
- invalid request after a valid request on the same TCP connection: 401,
  including when a forged original-auth header is supplied, with no upstream
  call. This catches accidental connection-lifespan identity retention.

The redirected requests intentionally have empty bodies. This focused harness
does not exercise AI-provider body reconstruction, which needs its own filters
and configuration; it must not be cited as evidence for that separate feature.
The non-redirected binary requests separately verify body transparency.

Record Wasm hashes, Envoy version, image digest, full generated bootstrap,
response/body hashes, auth/upstream event assertions, Envoy logs/metrics and
cleanup proof. Native helper/parser tests alone are not this runtime test.

## Run

Build the two modules outside the tracked source tree. From each source
revision's `plugins/wasm-go/extensions/ext-auth` directory:

```sh
go test ./...
go vet ./...
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o /tmp/ext-auth-VARIANT.wasm .
```

On Linux, with Python 3 and Docker, run the following from the repository root.
Set the gateway image explicitly to a relevant Higress release's resolved
digest, not a mutable tag. Output directories must not already exist.

```sh
export GATEWAY_IMAGE='higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway@sha256:REPLACE_WITH_DIGEST'
python3 plugins/wasm-go/extensions/ext-auth/test/runtime/verify.py \
  --gateway-image "$GATEWAY_IMAGE" --wasm /tmp/ext-auth-baseline.wasm \
  --expect legacy --output /tmp/ext-auth-baseline-evidence
python3 plugins/wasm-go/extensions/ext-auth/test/runtime/verify.py \
  --gateway-image "$GATEWAY_IMAGE" --wasm /tmp/ext-auth-fixed.wasm \
  --expect fixed --output /tmp/ext-auth-fixed-evidence
```

Both commands exit successfully only if their expected assertions hold. The
baseline explicitly expects the reproduced 401 failure; the fixed variant
expects 200. Only the Wasm module and expected result differ. Generated port
numbers are ephemeral; the plugin/request policy is otherwise identical.
`--docker 'sudo -n docker'` is available where Docker requires sudo.

The harness always removes its uniquely named container and stops its mock
thread, then checks that all three loopback ports are closed. Evidence remains
in `--output`; inspect and hash it for the PR, but do not commit generated
Wasm, logs or deployment-specific output into source control.
