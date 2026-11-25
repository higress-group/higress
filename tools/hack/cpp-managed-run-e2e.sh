#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
EXPECTED_HEAD="${EXPECTED_HEAD:?artifact source commit is required}"
VERIFICATION_HEAD="${VERIFICATION_HEAD:?verification tooling head is required}"
ARTIFACT_DIR="${ARTIFACT_DIR:?exact-run artifacts are required}"
NATIVE_RESULTS="${NATIVE_RESULTS:?exact-run native index is required}"
EVIDENCE_DIR="${EVIDENCE_DIR:?evidence directory is required}"
GITHUB_RUN_ID="${GITHUB_RUN_ID:?exact workflow run is required}"
GITHUB_RUN_ATTEMPT="${GITHUB_RUN_ATTEMPT:?exact workflow attempt is required}"
KIND_NODE_TAG='v1.34.0@sha256:7416a61b42b1662ca6ca89f02028ac133a309a2a30ba309614e8ec94d976dc5a'
KIND_BIN="$ROOT/tools/bin/kind-gateway-api"
SOURCES=(basic_auth bot_detect custom_response hmac_auth jwt_auth key_auth key_rate_limit oauth request_block)
TESTS=(CPPWasmPluginsBasicAuth CPPWasmPluginsKeyAuth CPPWasmPluginsRequestBlock)

test "$(git rev-parse HEAD)" = "$VERIFICATION_HEAD"
mkdir -p "$EVIDENCE_DIR"
python3 tools/hack/cpp-managed-evidence.py --artifact-dir "$ARTIFACT_DIR" --head "$EXPECTED_HEAD" --verification-head "$VERIFICATION_HEAD" --run-id "$GITHUB_RUN_ID" --run-attempt "$GITHUB_RUN_ATTEMPT" --output-dir "$EVIDENCE_DIR/build-index"
cmp "$NATIVE_RESULTS" "$EVIDENCE_DIR/build-index/native-results.txt"
cp "$NATIVE_RESULTS" "$EVIDENCE_DIR/native-results.txt"
jq -n --arg artifactSourceCommit "$EXPECTED_HEAD" --arg verificationHead "$VERIFICATION_HEAD" --arg runId "$GITHUB_RUN_ID" --arg runAttempt "$GITHUB_RUN_ATTEMPT" '{artifactSourceCommit:$artifactSourceCommit,verificationHead:$verificationHead,runId:$runId,runAttempt:$runAttempt}' > "$EVIDENCE_DIR/identity.json"
cluster_started=false
cleanup() {
  status=$?
  trap - EXIT
  set +e
  cleanup_ok=true
  if [[ "$cluster_started" == true ]]; then
    kubectl get pods -A -o wide > "$EVIDENCE_DIR/cleanup-pods.txt" 2>&1
    kubectl get events -A --sort-by=.lastTimestamp > "$EVIDENCE_DIR/cleanup-events.txt" 2>&1
    kubectl logs -n higress-system deployment/higress-controller --all-containers --prefix > "$EVIDENCE_DIR/controller.log" 2>&1
    kubectl logs -n higress-system deployment/higress-gateway --all-containers --prefix > "$EVIDENCE_DIR/gateway.log" 2>&1
    "$KIND_BIN" export logs "$EVIDENCE_DIR/kind" --name higress
    "$KIND_BIN" delete cluster --name higress || cleanup_ok=false
    if "$KIND_BIN" get clusters | grep -qx higress; then cleanup_ok=false; fi
  fi
  for source in "${SOURCES[@]}"; do rm -f "plugins/wasm-cpp/extensions/$source/plugin.wasm"; done
  printf 'kind_cluster_deleted=%s\n' "$cleanup_ok" > "$EVIDENCE_DIR/cleanup.txt"
  if [[ "$cleanup_ok" != true && "$status" -eq 0 ]]; then status=1; fi
  exit "$status"
}
trap cleanup EXIT

for source in "${SOURCES[@]}"; do
  artifact="$ARTIFACT_DIR/cpp-managed-$source-$EXPECTED_HEAD"
  test -s "$artifact/plugin.wasm"
  cp "$artifact/plugin.wasm" "plugins/wasm-cpp/extensions/$source/plugin.wasm"
  cmp "$artifact/plugin.wasm" "plugins/wasm-cpp/extensions/$source/plugin.wasm"
done

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
  if test -L "external/$external_name"; then
    test "$(readlink "external/$external_name")" = "../$module_path"
  else
    ln -s "../$module_path" "external/$external_name"
  fi
  printf '%s\t%s\t%s\texternal/%s\n' "$module_path" "$expected_revision" "$actual_revision" "$external_name" >> "$EVIDENCE_DIR/e2e-module-revisions.txt"
done
test "$(wc -l < "$EVIDENCE_DIR/e2e-module-revisions.txt")" -eq 5
sed -i 's|image: bitinit/eureka|image: docker.io/bitinit/eureka:cpp201-pinned|' test/e2e/conformance/base/eureka.yaml
sed -i 's|llm-mock-server:latest|llm-mock-server:cpp201-pinned|' test/e2e/conformance/base/llm-mock.yaml
sed -i 's|curlimages/curl:latest|docker.io/curlimages/curl:cpp201-pinned|' test/e2e/conformance/base/opa.yaml
git diff -- test/e2e/conformance/base > "$EVIDENCE_DIR/fixture-overrides.diff"
! grep -R -n -E 'image:.*(:latest|bitinit/eureka["[:space:]]*$)' test/e2e/conformance/base --include='*.yaml'
cluster_started=true
make create-gateway-api-cluster GATEWAY_API_KIND_NODE_TAG="$KIND_NODE_TAG" 2>&1 | tee "$EVIDENCE_DIR/create-cluster.log"
"$KIND_BIN" version > "$EVIDENCE_DIR/kind-version.txt"
kubectl version --client=true -o yaml > "$EVIDENCE_DIR/kubectl-version.yaml"
helm version > "$EVIDENCE_DIR/helm-version.txt"
go version > "$EVIDENCE_DIR/go-version.txt"
KIND="$KIND_BIN" bash tools/hack/cpp201-load-runtime-images.sh "$EVIDENCE_DIR" 2>&1 | tee "$EVIDENCE_DIR/load-images.log"
make pre-install
helm install higress helm/core -n higress-system --create-namespace \
  --set 'controller.tag=2.2.6' --set 'pilot.tag=2.2.6' --set 'gateway.tag=2.2.6' \
  --set 'gateway.replicas=1' --set 'global.local=true' --set 'global.imagePullPolicy=IfNotPresent' \
  --set 'global.volumeWasmPlugins=true' --set 'global.onlyPushRouteCluster=false'
helm get manifest higress -n higress-system > "$EVIDENCE_DIR/helm-manifest.yaml"
helm get values higress -n higress-system -o yaml > "$EVIDENCE_DIR/helm-values.yaml"
kubectl wait --timeout=10m -n higress-system deployment/higress-controller --for=condition=Available
kubectl wait --timeout=10m -n higress-system deployment/higress-gateway --for=condition=Available
kubectl rollout restart -n higress-system deployment/higress-gateway
kubectl rollout status --timeout=10m -n higress-system deployment/higress-gateway
kubectl get pods -A -o json > "$EVIDENCE_DIR/pods.json"
kubectl get pods -A -o jsonpath='{range .items[*]}{.metadata.namespace}{"/"}{.metadata.name}{"\n"}{range .status.containerStatuses[*]}{.name}{"\t"}{.image}{"\t"}{.imageID}{"\n"}{end}{end}' > "$EVIDENCE_DIR/pod-image-identities.txt"

assert_wasm_hashes() {
  local pod source artifact_hash host_hash gateway_hash
  pod="$(kubectl get pod -n higress-system -l app=higress-gateway -o json | jq -er '[.items[] | select(.metadata.deletionTimestamp == null and .status.phase == "Running") | select(any(.status.containerStatuses[]?; .name == "higress-gateway" and .ready == true))] | if length == 1 then .[0].metadata.name else error("expected one ready, non-terminating gateway pod") end')"
  test -n "$pod"
  : > "$EVIDENCE_DIR/gateway-wasm-hashes.txt"
  for source in "${SOURCES[@]}"; do
    artifact_hash="$(sha256sum "$ARTIFACT_DIR/cpp-managed-$source-$EXPECTED_HEAD/plugin.wasm" | awk '{print $1}')"
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
  test "$(grep -c -- "--- PASS: TestHigressConformanceTests/$test_name " "$EVIDENCE_DIR/$test_name.log" || true)" -eq 1
  test "$(grep -c -- "Running Conformance Test: $test_name" "$EVIDENCE_DIR/$test_name.log" || true)" -eq 1
  ! grep -E -- "--- (SKIP|FAIL): TestHigressConformanceTests/$test_name([ /]|$)" "$EVIDENCE_DIR/$test_name.log"
  assert_wasm_hashes
  conformance_namespaces="$(kubectl get namespaces -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | grep '^higress-conformance' || true)"
  printf '%s\n' "$conformance_namespaces" > "$EVIDENCE_DIR/$test_name.namespace-cleanup.txt"
  for namespace in $conformance_namespaces; do kubectl wait --for=delete "namespace/$namespace" --timeout=180s 2>&1 | tee -a "$EVIDENCE_DIR/$test_name.namespace-cleanup.txt"; done
  printf 'namespace_cleanup=passed\n' >> "$EVIDENCE_DIR/$test_name.namespace-cleanup.txt"
done
printf 'basic_auth,key_auth,request_block: runtime-e2e=passed\n' > "$EVIDENCE_DIR/runtime-coverage.txt"
for source in bot_detect custom_response hmac_auth jwt_auth key_rate_limit oauth; do
  printf '%s: native=passed; wasm=passed; runtime-e2e=not-run; coverage-gap=accepted\n' "$source" >> "$EVIDENCE_DIR/runtime-coverage.txt"
done
printf 'OCI candidate/public publication=not-run; registry writes=none\n' > "$EVIDENCE_DIR/release-boundary.txt"
