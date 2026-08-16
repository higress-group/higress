#!/usr/bin/env bash
set -u
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

cleanup() {
  podman compose -f "$HARNESS_DIR/compose.yaml" --profile verify down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

echo "building exact-head mcp-server WASM from $SOURCE_SHA"
(cd "$EXTENSION_DIR" && GOOS=wasip1 GOARCH=wasm go build -trimpath -buildmode=c-shared -o "$RUNTIME_EVIDENCE/plugin.wasm" .) || exit 3
if command -v sha256sum >/dev/null 2>&1; then
  PLUGIN_SHA256=$(sha256sum "$RUNTIME_EVIDENCE/plugin.wasm" | awk '{print $1}')
else
  PLUGIN_SHA256=$(shasum -a 256 "$RUNTIME_EVIDENCE/plugin.wasm" | awk '{print $1}')
fi
export PLUGIN_SHA256

RUNTIME_OUT="$RUNTIME_EVIDENCE" python3 "$HARNESS_DIR/generate_envoy.py" || exit 3
podman pull --quiet higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:v2.2.3 >/dev/null || exit 3
podman pull --quiet docker.io/library/python:3.12-alpine >/dev/null || exit 3
podman version --format '{{.Client.Version}}' >"$RUNTIME_EVIDENCE/podman-version.txt"
podman compose version >"$RUNTIME_EVIDENCE/compose-version.txt" 2>&1
podman image inspect higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:v2.2.3 --format '{{range .RepoDigests}}{{println .}}{{end}}' >"$RUNTIME_EVIDENCE/gateway-image-digests.txt"
podman image inspect docker.io/library/python:3.12-alpine --format '{{range .RepoDigests}}{{println .}}{{end}}' >"$RUNTIME_EVIDENCE/backend-image-digests.txt"
podman compose -f "$HARNESS_DIR/compose.yaml" --profile verify config \
  | sed -e 's/runtime-upstream-token/<redacted>/g' -e 's/runtime-key/<redacted>/g' >"$RUNTIME_EVIDENCE/compose-config.yaml"

podman compose -f "$HARNESS_DIR/compose.yaml" up -d backend-primary backend-secondary gateway-auto || exit 4
sleep 2
podman compose -f "$HARNESS_DIR/compose.yaml" exec -T backend-primary wget -q -O - http://127.0.0.1:8080/__state >"$RUNTIME_EVIDENCE/backend-auto-state.json" || exit 4
podman compose -f "$HARNESS_DIR/compose.yaml" up -d gateway || exit 4
set +e
podman compose -f "$HARNESS_DIR/compose.yaml" --profile verify run --rm verifier
VERIFY_STATUS=$?
set -e
podman compose -f "$HARNESS_DIR/compose.yaml" stop gateway gateway-auto || exit 4
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
if test "$VERIFY_STATUS" -ne 0 || test "$FINALIZE_STATUS" -ne 0; then
  exit 1
fi
