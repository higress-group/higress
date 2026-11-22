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
mkdir -p "$EVIDENCE_DIR"

test "$(git rev-parse HEAD)" = "$EXPECTED_HEAD"
python3 tools/hack/cpp201_check_workspace.py "$ROOT" 2>&1 | tee "$EVIDENCE_DIR/valid.log"
git diff --exit-code 392b64e8cce43e63d3207f00ba5c8fe2ff8921d9 -- plugins/wasm-cpp/common/route_rule_matcher.h
! git diff --name-only 392b64e8cce43e63d3207f00ba5c8fe2ff8921d9...HEAD | grep -E '^(plugins/release/|tools/plugin-release/|plugin-server/|\.github/workflows/(prepare|validate|promote)-)'

FIXTURE="$(mktemp -d)"
mkdir -p "$FIXTURE/plugins" "$FIXTURE/tools/hack" "$FIXTURE/.github/workflows" "$FIXTURE/test/e2e/conformance/tests"
cp -a plugins/wasm-cpp "$FIXTURE/plugins/wasm-cpp"
cp -a test/e2e/conformance/base "$FIXTURE/test/e2e/conformance/base"
cp test/e2e/conformance/tests/cpp-wasm-basic-auth.yaml "$FIXTURE/test/e2e/conformance/tests/cpp-wasm-basic-auth.yaml"
cp tools/hack/cpp201-runtime-images.pins "$FIXTURE/tools/hack/cpp201-runtime-images.pins"
cp tools/hack/cpp201-run-e2e.sh "$FIXTURE/tools/hack/cpp201-run-e2e.sh"
cp .github/workflows/wasm-cpp-2.0.1-verification.yaml "$FIXTURE/.github/workflows/wasm-cpp-2.0.1-verification.yaml"

expect_failure() {
  local name="$1"
  local directory="$2"
  if python3 tools/hack/cpp201_check_workspace.py "$directory" > "$EVIDENCE_DIR/$name.log" 2>&1; then
    echo "negative contract unexpectedly passed: $name" >&2
    return 1
  fi
}

cp -a "$FIXTURE" "$FIXTURE-missing-hash"
sed -i '/sha256 =/d' "$FIXTURE-missing-hash/plugins/wasm-cpp/WORKSPACE"
expect_failure missing-hash "$FIXTURE-missing-hash"

cp -a "$FIXTURE" "$FIXTURE-wrong-version"
printf '2.0.0\n' > "$FIXTURE-wrong-version/plugins/wasm-cpp/extensions/basic_auth/VERSION"
expect_failure wrong-version "$FIXTURE-wrong-version"

cp -a "$FIXTURE" "$FIXTURE-extra-plugin"
sed -i '/          - request_block/a\          - sni_misdirect' "$FIXTURE-extra-plugin/.github/workflows/wasm-cpp-2.0.1-verification.yaml"
expect_failure extra-plugin "$FIXTURE-extra-plugin"

cp -a "$FIXTURE" "$FIXTURE-latest"
sed -i 's/cpp201-pinned/latest/g' "$FIXTURE-latest/tools/hack/cpp201-runtime-images.pins"
expect_failure latest "$FIXTURE-latest"

cp -a "$FIXTURE" "$FIXTURE-builder"
sed -i 's/@sha256:[0-9a-f]\{64\}//' "$FIXTURE-builder/plugins/wasm-cpp/Dockerfile"
expect_failure builder-digest "$FIXTURE-builder"

printf 'valid=passed\nmissing-hash=failed-closed\nwrong-version=failed-closed\nextra-plugin=failed-closed\nlatest=failed-closed\nbuilder-digest=failed-closed\n' > "$EVIDENCE_DIR/summary.txt"
find "$EVIDENCE_DIR" -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > "$EVIDENCE_DIR/SHA256SUMS"
