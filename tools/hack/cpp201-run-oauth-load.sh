#!/usr/bin/env bash
# Copyright (c) 2026 Alibaba Group Holding Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
EXPECTED_HEAD="${EXPECTED_HEAD:?exact PR head is required}"
OAUTH_ARTIFACT="${OAUTH_ARTIFACT:?oauth artifact directory is required}"
EVIDENCE_DIR="${EVIDENCE_DIR:?evidence directory is required}"
PINS="$ROOT/tools/hack/cpp201-runtime-images.pins"
HARNESS="$ROOT/out/cpp201-oauth-load/harness"
PROJECT='cpp201-oauth-load'

mkdir -p "$EVIDENCE_DIR" "$HARNESS"
cp docs/developers/wasm-runtime-verification/compose.yaml "$HARNESS/compose.yaml"
sed -i 's/internal: true/internal: false/' "$HARNESS/compose.yaml"
OAUTH_CONFIG='{"consumers":[{"name":"consumer1","client_id":"client","client_secret":"secret"}]}'
sed "s|value: \"{}\"|value: '$OAUTH_CONFIG'|" docs/developers/wasm-runtime-verification/envoy.yaml > "$HARNESS/envoy.yaml"
grep -F 'client_secret' "$HARNESS/envoy.yaml"
cp "$OAUTH_ARTIFACT/plugin.wasm" "$HARNESS/plugin.wasm"
GATEWAY_IMAGE="$(awk -F'|' '$1 ~ /\/gateway:2\.2\.6$/ {print $2}' "$PINS")"
HTTPBIN_IMAGE="$(awk -F'|' '$1 ~ /go-httpbin:v2\.17\.0$/ {print $2}' "$PINS")"
export HIGRESS_GATEWAY_IMAGE="$GATEWAY_IMAGE"
export HTTPBIN_IMAGE="$HTTPBIN_IMAGE"
export COMPOSE_PROJECT_NAME="$PROJECT"
export WASM_PATH='./plugin.wasm'
export ENVOY_PORT=10000
export ENVOY_ADMIN_PORT=9901

cleanup() {
  status=$?
  trap - EXIT
  set +e
  if [[ -n "${CURRENT_OUTPUT:-}" ]]; then
    (
      cd "$HARNESS"
      docker compose --project-name "$PROJECT" logs --no-color envoy httpbin
    ) > "$CURRENT_OUTPUT/cleanup-logs.txt" 2>&1
    if ! (
      cd "$HARNESS"
      docker compose --project-name "$PROJECT" down --volumes --remove-orphans
      docker compose --project-name "$PROJECT" ps --all
    ) > "$CURRENT_OUTPUT/cleanup.txt" 2>&1; then
      status=1
    fi
  fi
  exit "$status"
}
trap cleanup EXIT

test "$(git rev-parse HEAD)" = "$EXPECTED_HEAD"
test "$(cat "$OAUTH_ARTIFACT/source-head.txt")" = "$EXPECTED_HEAD"
(cd "$OAUTH_ARTIFACT" && sha256sum -c SHA256SUMS)
test "$(sha256sum "$OAUTH_ARTIFACT/plugin.wasm" | awk '{print $1}')" = "$(sha256sum "$HARNESS/plugin.wasm" | awk '{print $1}')"
cp "$OAUTH_ARTIFACT/imports.json" "$EVIDENCE_DIR/imports.json"
docker pull "$GATEWAY_IMAGE"
docker pull "$HTTPBIN_IMAGE"
docker image inspect --format '{{json .RepoDigests}}' "$GATEWAY_IMAGE" "$HTTPBIN_IMAGE" > "$EVIDENCE_DIR/image-digests.json"
CURRENT_OUTPUT="$EVIDENCE_DIR"
(
  cd "$HARNESS"
  docker compose --project-name "$PROJECT" config
) > "$EVIDENCE_DIR/compose-config.yaml"
(
  cd "$HARNESS"
  docker compose --project-name "$PROJECT" up --detach
)
ready=0
for attempt in $(seq 1 45); do
  if curl --fail --silent --max-time 2 "http://127.0.0.1:$ENVOY_ADMIN_PORT/ready" > /dev/null; then
    ready=1
    break
  fi
  sleep 1
done
test "$ready" -eq 1
status="$(curl --silent --show-error --max-time 10 \
  --dump-header "$EVIDENCE_DIR/unauthorized.headers" \
  --output "$EVIDENCE_DIR/unauthorized.body" \
  --write-out '%{http_code}' \
  "http://127.0.0.1:$ENVOY_PORT/anything/oauth-load")"
printf '%s\n' "$status" > "$EVIDENCE_DIR/unauthorized.status"
test "$status" = '401'
(
  cd "$HARNESS"
  docker compose --project-name "$PROJECT" logs --no-color envoy httpbin
) > "$EVIDENCE_DIR/envoy-httpbin.log"
curl --fail --silent --show-error "http://127.0.0.1:$ENVOY_ADMIN_PORT/stats?usedonly" > "$EVIDENCE_DIR/envoy-stats.txt"
(
  cd "$HARNESS"
  docker compose --project-name "$PROJECT" down --volumes --remove-orphans
  docker compose --project-name "$PROJECT" ps --all
) > "$EVIDENCE_DIR/cleanup.txt"
! curl --silent --max-time 1 "http://127.0.0.1:$ENVOY_PORT/" > /dev/null 2>&1
! curl --silent --max-time 1 "http://127.0.0.1:$ENVOY_ADMIN_PORT/ready" > /dev/null 2>&1
printf 'proxy-wasm-load-smoke=passed\nruntime-e2e=not-run\ncoverage-gap=accepted\ncleanup=passed\n' >> "$EVIDENCE_DIR/coverage.txt"
CURRENT_OUTPUT=''
find "$HARNESS" "$EVIDENCE_DIR" -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > "$EVIDENCE_DIR/SHA256SUMS"
