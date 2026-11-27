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
ARTIFACT_DIR="${ARTIFACT_DIR:?artifact directory is required}"
NATIVE_RESULTS="${NATIVE_RESULTS:?exact-run native outcome index is required}"
EVIDENCE_DIR="${EVIDENCE_DIR:?evidence directory is required}"
KIND_NODE_TAG='v1.34.0@sha256:7416a61b42b1662ca6ca89f02028ac133a309a2a30ba309614e8ec94d976dc5a'
KIND_BIN="$ROOT/tools/bin/kind-gateway-api"
SOURCES=(basic_auth key_auth request_block)
TESTS=(CPPWasmPluginsBasicAuth CPPWasmPluginsKeyAuth CPPWasmPluginsRequestBlock)

mkdir -p "$EVIDENCE_DIR"
cleanup() {
  status=$?
  trap - EXIT
  set +e
  cleanup_ok=true
  kubectl get pods -A -o wide > "$EVIDENCE_DIR/cleanup-pods.txt" 2>&1
  kubectl get events -A --sort-by=.lastTimestamp > "$EVIDENCE_DIR/cleanup-events.txt" 2>&1
  kubectl logs -n higress-system deployment/higress-controller --all-containers --prefix > "$EVIDENCE_DIR/controller.log" 2>&1
  kubectl logs -n higress-system deployment/higress-gateway --all-containers --prefix > "$EVIDENCE_DIR/gateway.log" 2>&1
  if [[ -x "$KIND_BIN" ]]; then
    "$KIND_BIN" export logs "$EVIDENCE_DIR/kind" --name higress
    if ! "$KIND_BIN" delete cluster --name higress; then
      cleanup_ok=false
    fi
    if "$KIND_BIN" get clusters | grep -qx higress; then
      cleanup_ok=false
    fi
    printf 'kind_cluster_deleted=%s\n' "$cleanup_ok" > "$EVIDENCE_DIR/cleanup.txt"
  fi
  if [[ "$cleanup_ok" != true && "$status" -eq 0 ]]; then
    status=1
  fi
  exit "$status"
}
trap cleanup EXIT

test "$(git rev-parse HEAD)" = "$EXPECTED_HEAD"
module_specs=(
  'envoy/go-control-plane:go-control-plane:4f11f65d9bc54bb8b18f1cc7c25460d3f8afef75'
  'istio/api:api:efc0fe428cccecc0285dbf51e2023f8e0422597c'
  'istio/client-go:client-go:09ed8dc4e7484d303c931fa42df7f7c728296923'
  'istio/istio:istio:383f79d0ccdba5d856a7f765b97cacba7c209dca'
  'istio/pkg:pkg:15c662994e6515201dc106a2ec3de85ed50e8078'
)
mkdir -p external
: > "$EVIDENCE_DIR/e2e-module-revisions.txt"
for module_spec in "${module_specs[@]}"; do
  IFS=: read -r module_path external_name expected_revision <<< "$module_spec"
  git submodule update --init --depth 1 -- "$module_path"
  actual_revision="$(git -C "$module_path" rev-parse HEAD)"
  test "$actual_revision" = "$expected_revision"
  test -f "$module_path/go.mod"
  ln -s "../$module_path" "external/$external_name"
  printf '%s\t%s\t%s\texternal/%s\n' "$module_path" "$expected_revision" "$actual_revision" "$external_name" >> "$EVIDENCE_DIR/e2e-module-revisions.txt"
done
test "$(wc -l < "$EVIDENCE_DIR/e2e-module-revisions.txt")" -eq 5
sed -i 's|image: bitinit/eureka|image: docker.io/bitinit/eureka:cpp201-pinned|' test/e2e/conformance/base/eureka.yaml
sed -i 's|llm-mock-server:latest|llm-mock-server:cpp201-pinned|' test/e2e/conformance/base/llm-mock.yaml
sed -i 's|curlimages/curl:latest|docker.io/curlimages/curl:cpp201-pinned|' test/e2e/conformance/base/opa.yaml
git diff -- test/e2e/conformance/base > "$EVIDENCE_DIR/fixture-overrides.diff"
grep -F 'docker.io/bitinit/eureka:cpp201-pinned' "$EVIDENCE_DIR/fixture-overrides.diff"
grep -F 'llm-mock-server:cpp201-pinned' "$EVIDENCE_DIR/fixture-overrides.diff"
grep -F 'docker.io/curlimages/curl:cpp201-pinned' "$EVIDENCE_DIR/fixture-overrides.diff"
! grep -R -n -E 'image:.*(:latest|bitinit/eureka["[:space:]]*$)' test/e2e/conformance/base --include='*.yaml'
printf '%s\n' "$EXPECTED_HEAD" > "$EVIDENCE_DIR/source-head.txt"
make create-gateway-api-cluster GATEWAY_API_KIND_NODE_TAG="$KIND_NODE_TAG" 2>&1 | tee "$EVIDENCE_DIR/create-cluster.log"
"$KIND_BIN" version | tee "$EVIDENCE_DIR/kind-version.txt"
kubectl version --client=true -o yaml > "$EVIDENCE_DIR/kubectl-version.yaml"
helm version > "$EVIDENCE_DIR/helm-version.txt"
go version > "$EVIDENCE_DIR/go-version.txt"
KIND="$KIND_BIN" tools/hack/cpp201-load-runtime-images.sh "$EVIDENCE_DIR" 2>&1 | tee "$EVIDENCE_DIR/load-images.log"
make pre-install
helm install higress helm/core -n higress-system --create-namespace \
  --set 'controller.tag=2.2.6' \
  --set 'pilot.tag=2.2.6' \
  --set 'gateway.tag=2.2.6' \
  --set 'gateway.replicas=1' \
  --set 'global.local=true' \
  --set 'global.imagePullPolicy=IfNotPresent' \
  --set 'global.volumeWasmPlugins=true' \
  --set 'global.onlyPushRouteCluster=false'
helm get manifest higress -n higress-system > "$EVIDENCE_DIR/helm-manifest.yaml"
helm get values higress -n higress-system -o yaml > "$EVIDENCE_DIR/helm-values.yaml"
kubectl wait --timeout=10m -n higress-system deployment/higress-controller --for=condition=Available
kubectl wait --timeout=10m -n higress-system deployment/higress-gateway --for=condition=Available

for source in "${SOURCES[@]}"; do
  artifact="$ARTIFACT_DIR/cpp201-$source-$EXPECTED_HEAD"
  test -s "$artifact/plugin.wasm"
  test "$(cat "$artifact/source-head.txt")" = "$EXPECTED_HEAD"
  (cd "$artifact" && sha256sum -c SHA256SUMS)
  cp "$artifact/plugin.wasm" "plugins/wasm-cpp/extensions/$source/plugin.wasm"
  test "$(sha256sum "$artifact/plugin.wasm" | awk '{print $1}')" = "$(sha256sum "plugins/wasm-cpp/extensions/$source/plugin.wasm" | awk '{print $1}')"
done
kubectl rollout restart -n higress-system deployment/higress-gateway
kubectl rollout status --timeout=10m -n higress-system deployment/higress-gateway
kubectl get pods -A -o json > "$EVIDENCE_DIR/pods.json"
kubectl get pods -A -o jsonpath='{range .items[*]}{.metadata.namespace}{"/"}{.metadata.name}{"\n"}{range .status.containerStatuses[*]}{.name}{"\t"}{.image}{"\t"}{.imageID}{"\n"}{end}{end}' > "$EVIDENCE_DIR/pod-image-identities.txt"

assert_wasm_hashes() {
  local pod source artifact_hash host_hash gateway_hash
  pod="$(kubectl get pod -n higress-system -l app=higress-gateway -o jsonpath='{.items[0].metadata.name}')"
  test -n "$pod"
  : > "$EVIDENCE_DIR/gateway-wasm-hashes.txt"
  for source in "${SOURCES[@]}"; do
    artifact_hash="$(sha256sum "$ARTIFACT_DIR/cpp201-$source-$EXPECTED_HEAD/plugin.wasm" | awk '{print $1}')"
    host_hash="$(sha256sum "plugins/wasm-cpp/extensions/$source/plugin.wasm" | awk '{print $1}')"
    gateway_hash="$(kubectl exec -n higress-system "$pod" -- sha256sum "/opt/plugins/wasm-cpp/extensions/$source/plugin.wasm" | awk '{print $1}')"
    test "$artifact_hash" = "$host_hash"
    test "$artifact_hash" = "$gateway_hash"
    printf '%s\t%s\t%s\t%s\n' "$source" "$artifact_hash" "$host_hash" "$gateway_hash" >> "$EVIDENCE_DIR/gateway-wasm-hashes.txt"
  done
}

assert_wasm_hashes
for test_name in "${TESTS[@]}"; do
  set +e
  make run-higress-e2e-test-wasmplugin PLUGIN_TYPE=CPP TEST_SHORTNAME="$test_name" 2>&1 | tee "$EVIDENCE_DIR/$test_name.log"
  test_status=${PIPESTATUS[0]}
  set -e
  printf '%d\n' "$test_status" > "$EVIDENCE_DIR/$test_name.exit"
  test "$test_status" -eq 0
  pass_count="$(grep -c -- "--- PASS: TestHigressConformanceTests/$test_name " "$EVIDENCE_DIR/$test_name.log" || true)"
  run_count="$(grep -c -- "Running Conformance Test: $test_name" "$EVIDENCE_DIR/$test_name.log" || true)"
  test "$pass_count" -eq 1
  test "$run_count" -eq 1
  ! grep -E -- "--- (SKIP|FAIL): TestHigressConformanceTests/$test_name([ /]|$)" "$EVIDENCE_DIR/$test_name.log"
  assert_wasm_hashes
  conformance_namespaces="$(kubectl get namespaces -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | grep '^higress-conformance' || true)"
  printf '%s\n' "$conformance_namespaces" > "$EVIDENCE_DIR/$test_name.namespace-cleanup.txt"
  for namespace in $conformance_namespaces; do
    kubectl wait --for=delete "namespace/$namespace" --timeout=180s 2>&1 | tee -a "$EVIDENCE_DIR/$test_name.namespace-cleanup.txt"
  done
  printf 'namespace_cleanup=passed\n' >> "$EVIDENCE_DIR/$test_name.namespace-cleanup.txt"
done

cat "$EVIDENCE_DIR"/CPPWasmPlugins*.log > "$EVIDENCE_DIR/all-e2e.log"
test "$(grep -c -- 'Running Conformance Test: CPPWasmPlugins' "$EVIDENCE_DIR/all-e2e.log")" -eq 3
bash tools/hack/cpp201-write-runtime-coverage.sh "$NATIVE_RESULTS" "$EVIDENCE_DIR/runtime-coverage.txt"
cp "$NATIVE_RESULTS" "$EVIDENCE_DIR/native-results.txt"
