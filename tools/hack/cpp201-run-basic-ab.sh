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
FIXED_ARTIFACT="${FIXED_ARTIFACT:?fixed artifact directory is required}"
BASELINE_ARTIFACT="${BASELINE_ARTIFACT:?baseline artifact directory is required}"
EVIDENCE_DIR="${EVIDENCE_DIR:?evidence directory is required}"
BASELINE='da836d066d6c69c2faee8a55b926795308fecae0'
PINS="$ROOT/tools/hack/cpp201-runtime-images.pins"
WORK="$ROOT/out/cpp201-basic-ab"
HARNESS="$WORK/harness"
CURRENT_PROJECT=''
CURRENT_OUTPUT=''

mkdir -p "$EVIDENCE_DIR" "$HARNESS"
cp docs/developers/wasm-runtime-verification/compose.yaml "$HARNESS/compose.yaml"
cp docs/developers/wasm-runtime-verification/basic-auth-envoy.yaml "$HARNESS/envoy.yaml"
sed -i 's/internal: true/internal: false/' "$HARNESS/compose.yaml"
GATEWAY_IMAGE="$(awk -F'|' '$1 ~ /\/gateway:2\.2\.6$/ {print $2}' "$PINS")"
HTTPBIN_IMAGE="$(awk -F'|' '$1 ~ /go-httpbin:v2\.17\.0$/ {print $2}' "$PINS")"
test -n "$GATEWAY_IMAGE"
test -n "$HTTPBIN_IMAGE"
export HIGRESS_GATEWAY_IMAGE="$GATEWAY_IMAGE"
export HTTPBIN_IMAGE="$HTTPBIN_IMAGE"

cleanup() {
  status=$?
  trap - EXIT
  set +e
  if [[ -n "$CURRENT_PROJECT" ]]; then
    docker compose --project-name "$CURRENT_PROJECT" --file "$HARNESS/compose.yaml" logs --no-color envoy httpbin > "$CURRENT_OUTPUT/cleanup-logs.txt" 2>&1
    if ! docker compose --project-name "$CURRENT_PROJECT" --file "$HARNESS/compose.yaml" down --volumes --remove-orphans > "$CURRENT_OUTPUT/cleanup.txt" 2>&1; then
      status=1
    fi
  fi
  exit "$status"
}
trap cleanup EXIT

test "$(git rev-parse HEAD)" = "$EXPECTED_HEAD"
test "$(cat "$FIXED_ARTIFACT/source-head.txt")" = "$EXPECTED_HEAD"
test "$(cat "$BASELINE_ARTIFACT/source-head.txt")" = "$BASELINE"
test "$(cat "$BASELINE_ARTIFACT/fixed-head.txt")" = "$EXPECTED_HEAD"
(cd "$FIXED_ARTIFACT" && sha256sum -c SHA256SUMS)
(cd "$BASELINE_ARTIFACT" && sha256sum -c SHA256SUMS)
for variant in baseline fixed; do
  mkdir -p "$WORK/$variant"
  if [[ "$variant" == baseline ]]; then
    cp "$BASELINE_ARTIFACT/plugin.wasm" "$WORK/$variant/plugin.wasm"
    cp "$BASELINE_ARTIFACT/source-head.txt" "$WORK/$variant/source-head.txt"
  else
    cp "$FIXED_ARTIFACT/plugin.wasm" "$WORK/$variant/plugin.wasm"
    cp "$FIXED_ARTIFACT/source-head.txt" "$WORK/$variant/source-head.txt"
  fi
done

docker pull "$GATEWAY_IMAGE"
docker pull "$HTTPBIN_IMAGE"
docker image inspect --format '{{json .RepoDigests}}' "$GATEWAY_IMAGE" "$HTTPBIN_IMAGE" > "$EVIDENCE_DIR/image-digests.json"
printf '%s\n%s\n' "$GATEWAY_IMAGE" "$HTTPBIN_IMAGE" > "$EVIDENCE_DIR/images.txt"
sha256sum "$HARNESS/compose.yaml" "$HARNESS/envoy.yaml" > "$EVIDENCE_DIR/harness-SHA256SUMS"
printf 'case\tvariant\trepeat\texpected\tactual\n' > "$EVIDENCE_DIR/summary.tsv"

run_repeat() {
  local variant="$1"
  local repeat="$2"
  local output="$EVIDENCE_DIR/$variant-repeat-$repeat"
  local project="cpp201-basic-ab-$variant-$repeat"
  local ready=0
  mkdir -p "$output"
  CURRENT_PROJECT="$project"
  CURRENT_OUTPUT="$output"
  export COMPOSE_PROJECT_NAME="$project"
  export WASM_PATH="../$variant/plugin.wasm"
  export ENVOY_PORT=10000
  export ENVOY_ADMIN_PORT=9901
  (
    cd "$HARNESS"
    docker compose --project-name "$project" config
  ) > "$output/compose-config.yaml"
  (
    cd "$HARNESS"
    docker compose --project-name "$project" up --detach
  )
  for attempt in $(seq 1 45); do
    if curl --fail --silent --max-time 2 "http://127.0.0.1:$ENVOY_ADMIN_PORT/ready" > /dev/null; then
      ready=1
      break
    fi
    sleep 1
  done
  if [[ "$ready" -ne 1 ]]; then
    (
      cd "$HARNESS"
      docker compose --project-name "$project" ps --all
      docker compose --project-name "$project" logs --no-color envoy httpbin
    ) > "$output/readiness-failure.log" 2>&1
    return 1
  fi

  while IFS='|' read -r case_name host authorization baseline_expected fixed_expected; do
    [[ -n "$case_name" ]] || continue
    local expected actual
    if [[ "$variant" == baseline ]]; then
      expected="$baseline_expected"
    else
      expected="$fixed_expected"
    fi
    actual="$(curl --silent --show-error --max-time 10 \
      --header "Host: $host" \
      --header "Authorization: $authorization" \
      --header "X-Request-ID: $variant-$repeat-$case_name" \
      --header "X-Verification-Iteration: $repeat" \
      --dump-header "$output/$case_name.headers" \
      --output "$output/$case_name.body" \
      --write-out '%{http_code}' \
      "http://127.0.0.1:$ENVOY_PORT/anything/runtime-verification?iteration=$repeat")"
    printf '%s\n' "$actual" > "$output/$case_name.status"
    printf '%s\t%s\t%s\t%s\t%s\n' "$case_name" "$variant" "$repeat" "$expected" "$actual" >> "$EVIDENCE_DIR/summary.tsv"
    if [[ "$actual" != "$expected" ]]; then
      echo "$variant $case_name expected $expected got $actual" >&2
      return 1
    fi
  done <<'CASES'
mixed-exact-invalid|exact.example|Basic dXNlcjppbnZhbGlk|200|401
mixed-suffix-invalid|www.suffix.EXAMPLE|Basic dXNlcjppbnZhbGlk|200|401
mixed-prefix-invalid|prefix.COM|Basic dXNlcjppbnZhbGlk|200|401
lowercase-invalid|lower.example|Basic dXNlcjppbnZhbGlk|401|401
mixed-exact-valid|exact.example|Basic YWRtaW46MTIzNDU2|200|200
lowercase-valid|lower.example|Basic YWRtaW46MTIzNDU2|200|200
unrelated-invalid|unrelated.example|Basic dXNlcjppbnZhbGlk|200|200
CASES

  (
    cd "$HARNESS"
    docker compose --project-name "$project" logs --no-color envoy httpbin
  ) > "$output/envoy-httpbin.log"
  curl --fail --silent --show-error "http://127.0.0.1:$ENVOY_ADMIN_PORT/stats?usedonly" > "$output/envoy-stats.txt"
  (
    cd "$HARNESS"
    docker compose --project-name "$project" down --volumes --remove-orphans
    docker compose --project-name "$project" ps --all
  ) > "$output/cleanup.txt"
  ! curl --silent --max-time 1 "http://127.0.0.1:$ENVOY_PORT/" > /dev/null 2>&1
  ! curl --silent --max-time 1 "http://127.0.0.1:$ENVOY_ADMIN_PORT/ready" > /dev/null 2>&1
  printf 'variant=%s\nrepeat=%s\ncleanup=passed\n' "$variant" "$repeat" >> "$output/cleanup.txt"
  CURRENT_PROJECT=''
  CURRENT_OUTPUT=''
}

for variant in baseline fixed; do
  for repeat in 1 2; do
    run_repeat "$variant" "$repeat"
  done
done

find "$WORK/baseline" "$WORK/fixed" -type f -print0 | sort -z | xargs -0 sha256sum > "$EVIDENCE_DIR/wasm-SHA256SUMS"
printf 'baseline_source=%s\nfixed_source=%s\nrepeats_per_variant=2\n' "$BASELINE" "$EXPECTED_HEAD" > "$EVIDENCE_DIR/identity.txt"
