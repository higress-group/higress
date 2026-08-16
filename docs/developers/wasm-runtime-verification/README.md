# Local Wasm runtime-verification harness

This directory provides a reusable local proxy-Wasm data path for baseline and
fixed-version verification. It runs Envoy from a contributor-selected Higress
gateway release image, loads one repository-relative Wasm module, sends traffic
to an isolated httpbin service, and emits access logs to container stdout.

The harness is a starting point, not evidence by itself. Copy or parameterize
the request and plugin configuration for the bug being tested, then run the
same inputs against the baseline and fixed modules. The detailed evidence
requirements are in the
[agent-assisted contribution policy](../agent-assisted-contributions.md#wasm-plugin-fixes).

## Prerequisites and pinned inputs

- Docker with the Compose plugin;
- a built baseline or fixed Wasm module inside this repository; and
- exact image references for the affected/relevant Higress gateway release and
  the httpbin upstream.

Run all commands below from the harness directory so Compose finds
`compose.yaml` and resolves `WASM_PATH` relative to the intended location. From
the repository root:

```bash
cd docs/developers/wasm-runtime-verification
```

Set every image explicitly. A digest is preferred; a release tag is acceptable
when the evidence also records the resolved digest. `v2.1.5` below is only an
illustrative historical gateway tag, not the required or current version.

```bash
export HIGRESS_GATEWAY_IMAGE='higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:v2.1.5'
export HTTPBIN_IMAGE='mccutchen/go-httpbin:v2.18.0'
export WASM_PATH='../../../plugins/wasm-go/extensions/PLUGIN_NAME/plugin.wasm'
export ENVOY_PORT='10000'
export ENVOY_ADMIN_PORT='9901'
export COMPOSE_PROJECT_NAME='higress-wasm-verify-CASE_NAME'

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1"
  else
    shasum -a 256 "$1"
  fi
}
```

Replace `PLUGIN_NAME` and `CASE_NAME`; do not copy those placeholders into
evidence. `WASM_PATH` must remain relative to this Compose directory and point
inside the repository. Do not use a machine-specific absolute path. Confirm
the selected upstream image listens on port `8080`, as expected by
[`envoy.yaml`](./envoy.yaml), or update and record both sides together.

Record the resolved images before testing:

```bash
docker pull "$HIGRESS_GATEWAY_IMAGE"
docker pull "$HTTPBIN_IMAGE"
docker image inspect --format '{{json .RepoDigests}}' \
  "$HIGRESS_GATEWAY_IMAGE" "$HTTPBIN_IMAGE"
git rev-parse HEAD
sha256_file "$WASM_PATH"
docker compose --project-name "$COMPOSE_PROJECT_NAME" config
```

## Run one variant

Edit the Wasm `configuration.value` in [`envoy.yaml`](./envoy.yaml) if the
plugin needs non-empty configuration. Keep a copy or patch of that exact config
with the evidence.

```bash
VERIFY_OUTPUT="$(mktemp -d)"
docker compose --project-name "$COMPOSE_PROJECT_NAME" up --detach
docker compose --project-name "$COMPOSE_PROJECT_NAME" ps

READY=0
ATTEMPT=1
while [ "$ATTEMPT" -le 30 ]; do
  if curl --fail --silent --max-time 2 \
    "http://127.0.0.1:${ENVOY_ADMIN_PORT}/ready" >/dev/null; then
    READY=1
    break
  fi
  ATTEMPT=$((ATTEMPT + 1))
  sleep 1
done

if [ "$READY" -ne 1 ]; then
  docker compose --project-name "$COMPOSE_PROJECT_NAME" ps
  docker compose --project-name "$COMPOSE_PROJECT_NAME" logs --no-color \
    envoy httpbin | tee "$VERIFY_OUTPUT/readiness-failure.log"
  docker compose --project-name "$COMPOSE_PROJECT_NAME" down \
    --volumes --remove-orphans
  exit 1
fi

for iteration in 1 2 3; do
  STATUS_FILE="$VERIFY_OUTPUT/status-${iteration}.txt"
  CURL_STDERR="$VERIFY_OUTPUT/curl-${iteration}.stderr"
  if ! curl --silent --show-error \
    --header "X-Verification-Iteration: ${iteration}" \
    --dump-header "$VERIFY_OUTPUT/headers-${iteration}.txt" \
    --output "$VERIFY_OUTPUT/body-${iteration}.bin" \
    --write-out '%{http_code}\n' \
    "http://127.0.0.1:${ENVOY_PORT}/anything/runtime-verification?iteration=${iteration}" \
    >"$STATUS_FILE" 2>"$CURL_STDERR"; then
    cat "$CURL_STDERR" >&2
    docker compose --project-name "$COMPOSE_PROJECT_NAME" ps
    docker compose --project-name "$COMPOSE_PROJECT_NAME" logs --no-color \
      envoy httpbin | tee "$VERIFY_OUTPUT/transport-failure-${iteration}.log"
    docker compose --project-name "$COMPOSE_PROJECT_NAME" down \
      --volumes --remove-orphans
    exit 1
  fi
  sha256_file "$VERIFY_OUTPUT/body-${iteration}.bin"
done

docker compose --project-name "$COMPOSE_PROJECT_NAME" logs --no-color envoy \
  >"$VERIFY_OUTPUT/envoy.log"
curl --fail --silent --show-error \
  "http://127.0.0.1:${ENVOY_ADMIN_PORT}/stats?usedonly" \
  >"$VERIFY_OUTPUT/envoy-stats.txt"
```

Add machine-checkable assertions for the claimed behavior. Depending on the
bug, assertions may compare status/headers/body hashes, count an exact access
log or metric signal, or prove deterministic behavior over repeated requests.
Keep the baseline and fixed output sets separate and link them from the
verification TASK with artifact hashes.

For a red/green comparison, stop and clean the baseline variant, point
`WASM_PATH` and `HIGRESS_GATEWAY_IMAGE` at the fixed inputs as applicable, and
repeat the same commands, configuration, and requests.

## Cleanup

Always tear down the Compose project, including volumes and orphans:

```bash
docker compose --project-name "$COMPOSE_PROJECT_NAME" down \
  --volumes --remove-orphans
docker compose --project-name "$COMPOSE_PROJECT_NAME" ps --all
! curl --silent --max-time 1 "http://127.0.0.1:${ENVOY_PORT}/"
! curl --silent --max-time 1 "http://127.0.0.1:${ENVOY_ADMIN_PORT}/ready"
```

Also verify that the selected listener port is no longer open. Remove the
temporary output directory after its results have been uploaded and hashed.
Do not commit generated Wasm modules, response bodies, logs, credentials, or
other runtime artifacts.

For concrete configurations, see the existing
[Wasm Go custom-response example](../../../plugins/wasm-go/examples/custom-response/docker-compose.yaml)
and
[Wasm Rust SSE timing example](../../../plugins/wasm-rust/example/sse-timing/docker-compose.yaml).
