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
EVIDENCE_DIR="${EVIDENCE_DIR:?evidence directory is required}"
BASELINE='da836d066d6c69c2faee8a55b926795308fecae0'
WORKTREE="$ROOT/out/cpp201-basic-baseline-source"

mkdir -p "$EVIDENCE_DIR"
cleanup() {
  git worktree remove --force "$WORKTREE" 2>/dev/null || true
}
trap cleanup EXIT

test "$(git rev-parse HEAD)" = "$EXPECTED_HEAD"
test "$(git rev-parse HEAD)" = "$(git rev-parse "$EXPECTED_HEAD")"
git worktree add --detach "$WORKTREE" "$BASELINE"
git -C "$WORKTREE" checkout "$EXPECTED_HEAD" -- \
  plugins/wasm-cpp/Dockerfile \
  plugins/wasm-cpp/WORKSPACE \
  plugins/wasm-cpp/bazel \
  plugins/wasm-cpp/common/BUILD \
  plugins/wasm-cpp/common/random.c \
  plugins/wasm-cpp/extensions/jwt_auth/BUILD

test "$(git -C "$WORKTREE" rev-parse HEAD)" = "$BASELINE"
git -C "$WORKTREE" diff --exit-code HEAD -- \
  plugins/wasm-cpp/common/route_rule_matcher.h \
  plugins/wasm-cpp/extensions/basic_auth
git -C "$WORKTREE" diff HEAD -- > "$EVIDENCE_DIR/build-only.diff"
git -C "$WORKTREE" diff --name-only HEAD | sort > "$EVIDENCE_DIR/build-only-files.txt"
cat > "$EVIDENCE_DIR/expected-build-only-files.txt" <<'EOF'
plugins/wasm-cpp/Dockerfile
plugins/wasm-cpp/WORKSPACE
plugins/wasm-cpp/bazel/re2.patch
plugins/wasm-cpp/bazel/wasm.bzl
plugins/wasm-cpp/common/BUILD
plugins/wasm-cpp/common/random.c
plugins/wasm-cpp/extensions/jwt_auth/BUILD
EOF
diff -u "$EVIDENCE_DIR/expected-build-only-files.txt" "$EVIDENCE_DIR/build-only-files.txt"
printf '%s\n' "$BASELINE" > "$EVIDENCE_DIR/source-head.txt"
printf '%s\n' "$EXPECTED_HEAD" > "$EVIDENCE_DIR/fixed-head.txt"
printf '%s\n' '//extensions/basic_auth:basic_auth.wasm' > "$EVIDENCE_DIR/targets.txt"

BUILDER="$(sed -n 's/^ARG BUILDER=//p' "$WORKTREE/plugins/wasm-cpp/Dockerfile")"
case "$BUILDER" in
  *@sha256:*) ;;
  *) echo 'Builder must be pinned by digest' >&2; exit 1 ;;
esac
docker pull "$BUILDER"
docker image inspect --format '{{json .RepoDigests}}' "$BUILDER" > "$EVIDENCE_DIR/builder-digests.json"
printf 'builder=%s\n' "$BUILDER" > "$EVIDENCE_DIR/toolchain.txt"
docker run --rm \
  -v "$WORKTREE/plugins/wasm-cpp:/workspace" \
  -v "$EVIDENCE_DIR:/evidence" \
  -w /workspace \
  "$BUILDER" \
  bash -euo pipefail -c '
    bazel --version | tee /evidence/bazel-version.txt
    bazel build //extensions/basic_auth:basic_auth.wasm 2>&1 | tee /evidence/build.log
    cp bazel-bin/extensions/basic_auth/basic_auth.wasm /evidence/plugin.wasm
  '
test -s "$EVIDENCE_DIR/plugin.wasm"
python3 tools/hack/cpp201_wasm_imports.py "$EVIDENCE_DIR/plugin.wasm" --output "$EVIDENCE_DIR/imports.json"
(cd "$EVIDENCE_DIR" && sha256sum plugin.wasm source-head.txt fixed-head.txt targets.txt toolchain.txt build.log bazel-version.txt build-only.diff imports.json) > "$EVIDENCE_DIR/SHA256SUMS"
