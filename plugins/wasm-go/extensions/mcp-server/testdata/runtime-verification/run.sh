#!/usr/bin/env bash
set -u
set -o pipefail
export PYTHONDONTWRITEBYTECODE=1

HARNESS_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(git -C "$HARNESS_DIR" rev-parse --show-toplevel)
EXTENSION_DIR="$REPO_ROOT/plugins/wasm-go/extensions/mcp-server"
SOURCE_SHA=$(git -C "$REPO_ROOT" rev-parse HEAD)
if test -z "$(git -C "$REPO_ROOT" status --porcelain)"; then
  SOURCE_TREE_CLEAN=true
else
  SOURCE_TREE_CLEAN=false
fi
if test "$SOURCE_TREE_CLEAN" != true && test "${RUNTIME_ALLOW_DIRTY:-0}" != 1; then
  echo "refusing final evidence from a dirty tree; commit the harness or set RUNTIME_ALLOW_DIRTY=1 for development" >&2
  exit 2
fi

if test -n "${RUNTIME_EVIDENCE:-}"; then
  mkdir -p "$RUNTIME_EVIDENCE"
else
  # Podman machine bind mounts on macOS can access /Users, but not an
  # arbitrary host /tmp path. Keep evidence outside the worktree.
  RUNTIME_EVIDENCE=$(mktemp -d "${REPO_ROOT%/*}/higress-mcp-runtime.XXXXXX")
fi
export RUNTIME_EVIDENCE SOURCE_SHA SOURCE_TREE_CLEAN
COMPOSE_PROJECT_NAME="mcp-runtime-$$"
export COMPOSE_PROJECT_NAME
BASELINE_REVISION=c55d9825c90868f50edbff9764a6b3cf2eb13162
BASELINE_SHA=$(git -C "$REPO_ROOT" rev-parse "$BASELINE_REVISION^{commit}")
BASELINE_SOURCE_DIR=""
ORACLE_REVISION=39ec41aab6eb1d40499bed2847085696de0ebb96
ORACLE_SHA=$(git -C "$REPO_ROOT" rev-parse "$ORACLE_REVISION^{commit}")
ORACLE_SOURCE_DIR=""
CANDIDATE_CORPUS_SOURCE_DIR=""
export BASELINE_SHA ORACLE_SHA

cleanup_descriptor_inputs() {
  python3 "$HARNESS_DIR/descriptor_self_test.py" cleanup "$RUNTIME_EVIDENCE" >/dev/null 2>&1 || true
}

cleanup() {
  cleanup_descriptor_inputs
  if test -n "${RUNTIME_DESCRIPTOR_TRAP_SELF_TEST:-}"; then
    return
  fi
  podman compose -f "$HARNESS_DIR/compose.yaml" --profile verify down --volumes --remove-orphans >/dev/null 2>&1 || true
  if test -n "$BASELINE_SOURCE_DIR" && test -d "$BASELINE_SOURCE_DIR"; then
    rm -rf -- "$BASELINE_SOURCE_DIR"
  fi
  if test -n "$ORACLE_SOURCE_DIR" && test -d "$ORACLE_SOURCE_DIR"; then
    rm -rf -- "$ORACLE_SOURCE_DIR"
  fi
  if test -n "$CANDIDATE_CORPUS_SOURCE_DIR" && test -d "$CANDIDATE_CORPUS_SOURCE_DIR"; then
    rm -rf -- "$CANDIDATE_CORPUS_SOURCE_DIR"
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Static fault-injection hook: exercise the real EXIT/signal cleanup path
# without compiling Wasm or starting Podman.
if test -n "${RUNTIME_DESCRIPTOR_TRAP_SELF_TEST:-}"; then
  RUNTIME_OUT="$RUNTIME_EVIDENCE" python3 "$HARNESS_DIR/generate_envoy.py" || exit 3
  python3 "$HARNESS_DIR/descriptor_self_test.py" prepare "$RUNTIME_EVIDENCE" || exit 3
  case "$RUNTIME_DESCRIPTOR_TRAP_SELF_TEST" in
    failure) exit 7 ;;
    wait)
      while :; do
        sleep 1
      done
      ;;
    *) echo "unknown RUNTIME_DESCRIPTOR_TRAP_SELF_TEST mode" >&2; exit 3 ;;
  esac
fi

echo "building exact-head mcp-server WASM from $SOURCE_SHA"
(cd "$EXTENSION_DIR" && GOOS=wasip1 GOARCH=wasm go build -trimpath -buildmode=c-shared -o "$RUNTIME_EVIDENCE/plugin.wasm" .) || exit 3

file_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

PLUGIN_SHA256=$(file_sha256 "$RUNTIME_EVIDENCE/plugin.wasm")
echo "building exact-head registered-schema corpus WASM from $SOURCE_SHA"
CANDIDATE_CORPUS_SOURCE_DIR=$(mktemp -d "$RUNTIME_EVIDENCE/candidate-corpus-source.XXXXXX") || exit 3
git -C "$REPO_ROOT" archive "$SOURCE_SHA" | tar -x -C "$CANDIDATE_CORPUS_SOURCE_DIR" || exit 3
cp "$HARNESS_DIR/registered_fixture.go.txt" "$CANDIDATE_CORPUS_SOURCE_DIR/plugins/wasm-go/extensions/mcp-server/000_runtime_fixture.go" || exit 3
(cd "$CANDIDATE_CORPUS_SOURCE_DIR/plugins/wasm-go/extensions/mcp-server" && GOOS=wasip1 GOARCH=wasm go build -trimpath -buildmode=c-shared -o "$RUNTIME_EVIDENCE/corpus-plugin-candidate.wasm" .) || exit 3
CORPUS_CANDIDATE_PLUGIN_SHA256=$(file_sha256 "$RUNTIME_EVIDENCE/corpus-plugin-candidate.wasm")
rm -rf -- "$CANDIDATE_CORPUS_SOURCE_DIR"
CANDIDATE_CORPUS_SOURCE_DIR=""
echo "building pinned baseline mcp-server WASM from $BASELINE_SHA"
BASELINE_SOURCE_DIR=$(mktemp -d "$RUNTIME_EVIDENCE/baseline-source.XXXXXX") || exit 3
git -C "$REPO_ROOT" archive "$BASELINE_SHA" | tar -x -C "$BASELINE_SOURCE_DIR" || exit 3
(cd "$BASELINE_SOURCE_DIR/plugins/wasm-go/extensions/mcp-server" && GOOS=wasip1 GOARCH=wasm go build -trimpath -buildmode=c-shared -o "$RUNTIME_EVIDENCE/baseline-plugin.wasm" .) || exit 3
BASELINE_PLUGIN_SHA256=$(file_sha256 "$RUNTIME_EVIDENCE/baseline-plugin.wasm")
cp "$HARNESS_DIR/registered_fixture.go.txt" "$BASELINE_SOURCE_DIR/plugins/wasm-go/extensions/mcp-server/000_runtime_fixture.go" || exit 3
(cd "$BASELINE_SOURCE_DIR/plugins/wasm-go/extensions/mcp-server" && GOOS=wasip1 GOARCH=wasm go build -trimpath -buildmode=c-shared -o "$RUNTIME_EVIDENCE/corpus-plugin-affected.wasm" .) || exit 3
CORPUS_AFFECTED_PLUGIN_SHA256=$(file_sha256 "$RUNTIME_EVIDENCE/corpus-plugin-affected.wasm")
rm -rf -- "$BASELINE_SOURCE_DIR"
BASELINE_SOURCE_DIR=""
echo "building v2.0.0 compatibility oracle WASM from $ORACLE_SHA"
ORACLE_SOURCE_DIR=$(mktemp -d "$RUNTIME_EVIDENCE/oracle-source.XXXXXX") || exit 3
git -C "$REPO_ROOT" archive "$ORACLE_SHA" | tar -x -C "$ORACLE_SOURCE_DIR" || exit 3
(cd "$ORACLE_SOURCE_DIR/plugins/wasm-go/extensions/mcp-server" && GOOS=wasip1 GOARCH=wasm go build -trimpath -buildmode=c-shared -o "$RUNTIME_EVIDENCE/oracle-plugin.wasm" .) || exit 3
ORACLE_PLUGIN_SHA256=$(file_sha256 "$RUNTIME_EVIDENCE/oracle-plugin.wasm")
cp "$HARNESS_DIR/registered_fixture.go.txt" "$ORACLE_SOURCE_DIR/plugins/wasm-go/extensions/mcp-server/000_runtime_fixture.go" || exit 3
(cd "$ORACLE_SOURCE_DIR/plugins/wasm-go/extensions/mcp-server" && GOOS=wasip1 GOARCH=wasm go build -trimpath -buildmode=c-shared -o "$RUNTIME_EVIDENCE/corpus-plugin-oracle.wasm" .) || exit 3
CORPUS_ORACLE_PLUGIN_SHA256=$(file_sha256 "$RUNTIME_EVIDENCE/corpus-plugin-oracle.wasm")
rm -rf -- "$ORACLE_SOURCE_DIR"
ORACLE_SOURCE_DIR=""
CORPUS_FIXTURE_SHA256=$(file_sha256 "$HARNESS_DIR/registered_fixture.go.txt")
export PLUGIN_SHA256 BASELINE_PLUGIN_SHA256 ORACLE_PLUGIN_SHA256 CORPUS_CANDIDATE_PLUGIN_SHA256 \
  CORPUS_AFFECTED_PLUGIN_SHA256 CORPUS_ORACLE_PLUGIN_SHA256 CORPUS_FIXTURE_SHA256

RUNTIME_OUT="$RUNTIME_EVIDENCE" python3 "$HARNESS_DIR/generate_envoy.py" || exit 3
python3 "$HARNESS_DIR/orchestration_self_test.py" || exit 3
bash "$HARNESS_DIR/lifecycle_self_test.sh" || exit 3
python3 "$HARNESS_DIR/descriptor_self_test.py" prepare "$RUNTIME_EVIDENCE" || exit 3
bash "$HARNESS_DIR/descriptor_gate.sh" "$RUNTIME_EVIDENCE" || exit 3
cleanup_descriptor_inputs
if test "${RUNTIME_SKIP_PULL:-0}" = 1; then
  if test "${RUNTIME_ALLOW_DIRTY:-0}" != 1; then
    echo "RUNTIME_SKIP_PULL=1 is allowed only for dirty development runs" >&2
    exit 2
  fi
else
  podman pull --quiet higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:v2.2.3 >/dev/null || exit 3
  podman pull --quiet docker.io/library/python:3.12-alpine >/dev/null || exit 3
fi
podman version --format '{{.Client.Version}}' >"$RUNTIME_EVIDENCE/podman-version.txt"
podman compose version >"$RUNTIME_EVIDENCE/compose-version.txt" 2>&1
podman image inspect higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:v2.2.3 --format '{{range .RepoDigests}}{{println .}}{{end}}' >"$RUNTIME_EVIDENCE/gateway-image-digests.txt"
podman image inspect docker.io/library/python:3.12-alpine --format '{{range .RepoDigests}}{{println .}}{{end}}' >"$RUNTIME_EVIDENCE/backend-image-digests.txt"
podman compose -f "$HARNESS_DIR/compose.yaml" --profile verify config \
  | sed -e 's/runtime-upstream-token/<redacted>/g' -e 's/runtime-key/<redacted>/g' >"$RUNTIME_EVIDENCE/compose-config.yaml"
podman compose -f "$HARNESS_DIR/compose.yaml" --profile verify config --format json \
  | sed -e 's/runtime-upstream-token/<redacted>/g' -e 's/runtime-key/<redacted>/g' >"$RUNTIME_EVIDENCE/compose-config.json"
python3 "$HARNESS_DIR/compose_config_self_test.py" "$RUNTIME_EVIDENCE/compose-config.json" || exit 3

compose_runtime() {
  podman compose -f "$HARNESS_DIR/compose.yaml" "$@"
}

RUNTIME_LIFECYCLE_DIAGNOSTICS="$RUNTIME_EVIDENCE/lifecycle-diagnostics.log"
export RUNTIME_LIFECYCLE_DIAGNOSTICS
: >"$RUNTIME_LIFECYCLE_DIAGNOSTICS"
. "$HARNESS_DIR/lifecycle.sh"

capture_gateway_log() {
  service=$1
  output=$2
  compose_runtime logs --no-color "$service" \
    | sed -e 's/runtime-upstream-token/<redacted>/g' -e 's/runtime-key/<redacted>/g' >"$output"
}

capture_empty_primary_backend() {
  output=$1
  compose_runtime exec -T backend-primary wget -q -O - \
    http://127.0.0.1:8080/__state >"$output" || return 1
  python3 - "$output" <<'PY'
import json
import sys

path = sys.argv[1]
events = json.load(open(path, encoding="utf-8")).get("events")
if events != []:
    raise SystemExit(f"expected zero backend events in {path}, got {events!r}")
PY
}

wait_for_rejection_markers() {
  service=$1
  first=$2
  second=$3
  timeout=${RUNTIME_REJECTION_TIMEOUT:-120}
  deadline=$(( $(date +%s) + timeout ))
  while test "$(date +%s)" -lt "$deadline"; do
    service_log=$(compose_runtime logs --no-color "$service" 2>&1 || true)
    first_seen=false
    second_seen=false
    case "$service_log" in *"$first"*) first_seen=true ;; esac
    case "$service_log" in *"$second"*) second_seen=true ;; esac
    test "$first_seen" = true && test "$second_seen" = true && return 0
    container_id=$(compose_runtime ps -a -q "$service" 2>/dev/null || true)
    if test -n "$container_id"; then
      running=$(podman inspect "$container_id" --format '{{.State.Running}}' 2>/dev/null || true)
      if test "$running" = false; then
        # Compose may observe exit before its log reader sees the last buffered
        # Wasm lines. Give the completed process one bounded flush interval.
        sleep 1
        service_log=$(compose_runtime logs --no-color "$service" 2>&1 || true)
        case "$service_log" in *"$first"*"$second"*|*"$second"*"$first"*) return 0 ;; esac
        echo "$service exited before both rejection markers were recorded" >&2
        return 1
      fi
    fi
    sleep 1
  done
  echo "timed out waiting for rejection markers from $service after ${timeout}s" >&2
  return 1
}

compose_runtime up -d backend-primary backend-secondary || exit 4
wait_primary_backend_ready || exit 4
run_static_rejection_phase gateway-auto "$RUNTIME_EVIDENCE/gateway-auto.log" "$RUNTIME_EVIDENCE/backend-auto-state.json" \
  "invalid protocolStrategy value: auto" "plugin start failed" || exit 4
run_static_rejection_phase gateway-baseline "$RUNTIME_EVIDENCE/gateway-baseline.log" "$RUNTIME_EVIDENCE/backend-baseline-state.json" \
  "requires a primitive type" "plugin start failed" || exit 4
for variant in candidate affected oracle; do
  run_static_rejection_phase "gateway-control-$variant" "$RUNTIME_EVIDENCE/gateway-control-$variant.log" \
    "$RUNTIME_EVIDENCE/backend-control-$variant-state.json" "error parsing URL template" "plugin start failed" || exit 4
done
# Keep the aggregate compatibility filename while retaining per-revision phase snapshots.
cp "$RUNTIME_EVIDENCE/backend-control-oracle-state.json" "$RUNTIME_EVIDENCE/backend-control-state.json" || exit 4

VERIFY_STATUS=0
podman compose -f "$HARNESS_DIR/compose.yaml" up -d gateway-oracle || exit 4
set +e
podman compose -f "$HARNESS_DIR/compose.yaml" --profile verify run --rm --no-deps \
  -e RUNTIME_GATEWAY_HOST=gateway-oracle -e RUNTIME_ORACLE=1 verifier
ORACLE_VERIFY_STATUS=$?
set -e
if test "$ORACLE_VERIFY_STATUS" -ne 0; then
  VERIFY_STATUS=1
fi
stop_runtime_service gateway-oracle || exit 4
podman compose -f "$HARNESS_DIR/compose.yaml" logs --no-color gateway-oracle \
  | sed -e 's/runtime-upstream-token/<redacted>/g' -e 's/runtime-key/<redacted>/g' >"$RUNTIME_EVIDENCE/gateway-oracle.log"

run_corpus_verifier() {
  revision=$1
  set +e
  compose_runtime --profile verify run --rm --no-deps \
    -e "RUNTIME_GATEWAY_HOST=gateway-corpus-$revision" -e "RUNTIME_CORPUS_REVISION=$revision" verifier
  verifier_status=$?
  set -e
  return "$verifier_status"
}

CORPUS_RUN_STATUS=0
run_corpus_revisions "$RUNTIME_EVIDENCE" || CORPUS_RUN_STATUS=$?
case "$CORPUS_RUN_STATUS" in
  0) ;;
  "$CORPUS_VERIFY_FAILED") VERIFY_STATUS=1 ;;
  *) exit 4 ;;
esac

capture_generation_process() {
  output=$1
  container_id=$(podman compose -f "$HARNESS_DIR/compose.yaml" ps -q gateway-generation) || return 1
  test -n "$container_id" || return 1
  podman inspect "$container_id" --format '{{.Id}}|{{.State.Pid}}|{{.State.StartedAt}}' >"$output"
}

podman compose -f "$HARNESS_DIR/compose.yaml" up -d gateway-generation || exit 4
capture_generation_process "$RUNTIME_EVIDENCE/generation-process-before.txt" || exit 4
set +e
podman compose -f "$HARNESS_DIR/compose.yaml" --profile verify run --rm --no-deps \
  -e RUNTIME_GATEWAY_HOST=gateway-generation -e RUNTIME_GENERATION_TRANSITION=1 verifier
GENERATION_VERIFY_STATUS=$?
set -e
capture_generation_process "$RUNTIME_EVIDENCE/generation-process-after.txt" || exit 4
if test "$GENERATION_VERIFY_STATUS" -ne 0; then
  VERIFY_STATUS=1
fi
stop_runtime_service gateway-generation || exit 4
podman compose -f "$HARNESS_DIR/compose.yaml" logs --no-color gateway-generation \
  | sed -e 's/runtime-upstream-token/<redacted>/g' -e 's/runtime-key/<redacted>/g' >"$RUNTIME_EVIDENCE/gateway-generation.log"

podman compose -f "$HARNESS_DIR/compose.yaml" up -d gateway || exit 4
set +e
podman compose -f "$HARNESS_DIR/compose.yaml" --profile verify run --rm verifier
MAIN_VERIFY_STATUS=$?
set -e
if test "$MAIN_VERIFY_STATUS" -ne 0; then
  VERIFY_STATUS=1
fi
stop_runtime_service gateway || exit 4
podman compose -f "$HARNESS_DIR/compose.yaml" logs --no-color gateway \
  | sed -e 's/runtime-upstream-token/<redacted>/g' -e 's/runtime-key/<redacted>/g' -e 's/downstream-[A-Za-z0-9_-]*/<redacted>/g' >"$RUNTIME_EVIDENCE/gateway.log"
podman compose -f "$HARNESS_DIR/compose.yaml" logs --no-color gateway-auto \
  | sed -e 's/runtime-upstream-token/<redacted>/g' -e 's/runtime-key/<redacted>/g' >"$RUNTIME_EVIDENCE/gateway-auto.log"
cleanup
trap - EXIT INT TERM
if test -z "$(podman ps -a --filter "label=com.docker.compose.project=$COMPOSE_PROJECT_NAME" --format '{{.ID}}')"; then
  echo "PASS no containers remain for compose project $COMPOSE_PROJECT_NAME" >"$RUNTIME_EVIDENCE/cleanup-proof.txt"
else
  echo "FAIL containers remain for compose project $COMPOSE_PROJECT_NAME" >"$RUNTIME_EVIDENCE/cleanup-proof.txt"
  VERIFY_STATUS=1
fi

set +e
python3 "$HARNESS_DIR/finalize_evidence.py"
FINALIZE_STATUS=$?
set -e
unlink "$RUNTIME_EVIDENCE/plugin.wasm"
unlink "$RUNTIME_EVIDENCE/baseline-plugin.wasm"
unlink "$RUNTIME_EVIDENCE/oracle-plugin.wasm"
unlink "$RUNTIME_EVIDENCE/corpus-plugin-candidate.wasm"
unlink "$RUNTIME_EVIDENCE/corpus-plugin-affected.wasm"
unlink "$RUNTIME_EVIDENCE/corpus-plugin-oracle.wasm"
if test "$VERIFY_STATUS" -ne 0 || test "$FINALIZE_STATUS" -ne 0; then
  exit 1
fi
