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
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"
EXPECTED_HEAD="${EXPECTED_HEAD:?exact head required}"
OUT="$ROOT/out/oauth-v0"
mkdir -p "$OUT"
test "$(git rev-parse HEAD)" = "$EXPECTED_HEAD"
git diff --exit-code c87eb4ccf4a3dd429a2895815cc6956698e0cc17 -- plugins/wasm-cpp/extensions/oauth plugins/wasm-cpp/WORKSPACE plugins/wasm-cpp/bazel plugins/wasm-cpp/common
printf '%s\n' "$EXPECTED_HEAD" > "$OUT/source-head.txt"
git diff --binary c87eb4ccf4a3dd429a2895815cc6956698e0cc17...HEAD > "$OUT/test-only-and-approved-small-rework.diff"
git ls-tree HEAD plugins/wasm-cpp/extensions/oauth/ > "$OUT/production-oauth-blobs.txt"
git ls-tree HEAD plugins/wasm-cpp/WORKSPACE plugins/wasm-cpp/Dockerfile plugins/wasm-cpp/.bazelversion > "$OUT/toolchain-blobs.txt"
sha256sum plugins/wasm-cpp/WORKSPACE plugins/wasm-cpp/bazel/wasm.bzl plugins/wasm-cpp/Dockerfile plugins/wasm-cpp/tests/oauth_parse_probe/* tools/hack/oauth-v0/* > "$OUT/input-SHA256SUMS"
python3 tools/hack/oauth-v0/fixtures.py "$OUT/fixtures"
builder="$(sed -n 's/^ARG BUILDER=//p' plugins/wasm-cpp/Dockerfile)"
printf '%s\n' "$builder" > "$OUT/builder.txt"
docker pull "$builder"
docker image inspect "$builder" > "$OUT/builder-inspect.json"
docker run --rm -v "$ROOT:/repo" -w /repo/plugins/wasm-cpp "$builder" bash /repo/tools/hack/oauth-v0/inside-build.sh
for variant in default_disabled candidate_eh; do
  if [[ -s "$OUT/$variant/plugin.wasm" ]]; then
    python3 tools/hack/oauth-v0/module_info.py "$OUT/$variant/plugin.wasm" "$OUT/$variant/module-info.json"
  fi
done
python3 tools/hack/oauth-v0/check_actions.py "$OUT/candidate_eh"
