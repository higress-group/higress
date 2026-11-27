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
OAUTH_PROBE_VERSION="${OAUTH_PROBE_VERSION:-v0}"
case "$OAUTH_PROBE_VERSION" in
  v0) variants=(default_disabled candidate_eh) ;;
  v1) variants=(candidate_eh candidate_eh_nodiag) ;;
  production) variants=(oauth) ;;
  *) exit 1 ;;
esac
OUT="$ROOT/out/oauth-$OAUTH_PROBE_VERSION"
mkdir -p "$OUT"
test "$(git rev-parse HEAD)" = "$EXPECTED_HEAD"
if [[ "$OAUTH_PROBE_VERSION" != production ]]; then
  git diff --exit-code c87eb4ccf4a3dd429a2895815cc6956698e0cc17 -- plugins/wasm-cpp/extensions/oauth
fi
git diff --exit-code b2319bd51d52253783ddb88b5049ed70b7611aa0 -- plugins/wasm-cpp/WORKSPACE plugins/wasm-cpp/bazel plugins/wasm-cpp/common plugins/wasm-cpp/Dockerfile plugins/wasm-cpp/.bazelrc plugins/wasm-cpp/.bazelversion
if [[ "$OAUTH_PROBE_VERSION" = v1 ]]; then
  git diff --exit-code 311fa2f3bf8017a61b283562d5748eade44b3ec1 -- \
    plugins/wasm-cpp/extensions plugins/wasm-cpp/WORKSPACE plugins/wasm-cpp/bazel \
    plugins/wasm-cpp/common plugins/wasm-cpp/Dockerfile plugins/wasm-cpp/.bazelrc \
    plugins/wasm-cpp/.bazelversion plugins/wasm-cpp/tests/oauth_parse_probe/parse.h \
    plugins/wasm-cpp/tests/oauth_parse_probe/probe.cc plugins/wasm-cpp/tests/oauth_parse_probe/native.cc \
    tools/hack/oauth-v0/fixtures.py docs/developers/wasm-runtime-verification
  git diff --binary 311fa2f3bf8017a61b283562d5748eade44b3ec1 HEAD > "$OUT/v1-only.diff"
fi
printf '%s\n' "$EXPECTED_HEAD" > "$OUT/source-head.txt"
git diff --binary c87eb4ccf4a3dd429a2895815cc6956698e0cc17...HEAD > "$OUT/test-only-and-approved-small-rework.diff"
git ls-tree HEAD plugins/wasm-cpp/extensions/oauth/ > "$OUT/production-oauth-blobs.txt"
git ls-tree HEAD plugins/wasm-cpp/WORKSPACE plugins/wasm-cpp/Dockerfile plugins/wasm-cpp/.bazelversion > "$OUT/toolchain-blobs.txt"
sha256sum plugins/wasm-cpp/WORKSPACE plugins/wasm-cpp/bazel/wasm.bzl plugins/wasm-cpp/Dockerfile plugins/wasm-cpp/tests/oauth_parse_probe/* tools/hack/oauth-v0/* > "$OUT/input-SHA256SUMS"
sha256sum .github/workflows/oauth-v0-feasibility.yaml docs/developers/wasm-runtime-verification/compose.yaml docs/developers/wasm-runtime-verification/envoy.yaml >> "$OUT/input-SHA256SUMS"
sha256sum plugins/wasm-cpp/extensions/oauth/BUILD plugins/wasm-cpp/extensions/oauth/plugin.cc plugins/wasm-cpp/extensions/oauth/plugin_test.cc >> "$OUT/input-SHA256SUMS"
python3 tools/hack/oauth-v0/fixtures.py "$OUT/fixtures"
if [[ "$OAUTH_PROBE_VERSION" = v1 ]]; then
  printf '%s  %s\n' 28fba56d19d6faa7130daad97b45424e29390ed96bb22f773dfc2fbf5bd74a22 "$OUT/fixtures/cases.json" | sha256sum --check
fi
builder="$(sed -n 's/^ARG BUILDER=//p' plugins/wasm-cpp/Dockerfile)"
printf '%s\n' "$builder" > "$OUT/builder.txt"
docker pull "$builder"
docker image inspect "$builder" > "$OUT/builder-inspect.json"
build_status=0
docker run --rm -e OAUTH_PROBE_VERSION="$OAUTH_PROBE_VERSION" -v "$ROOT:/repo" -w /repo/plugins/wasm-cpp "$builder" bash /repo/tools/hack/oauth-v0/inside-build.sh || build_status=$?
sudo chown -R "$(id -u):$(id -g)" "$OUT"
printf '%s\n' "$build_status" > "$OUT/builder.exit"
for variant in "${variants[@]}"; do
  if [[ -s "$OUT/$variant/plugin.wasm" ]]; then
    python3 tools/hack/oauth-v0/module_info.py "$OUT/$variant/plugin.wasm" "$OUT/$variant/module-info.json"
  fi
done
if [[ "$OAUTH_PROBE_VERSION" = production ]]; then
  python3 tools/hack/oauth-v0/check_actions.py "$OUT/oauth"
else
  python3 tools/hack/oauth-v0/check_actions.py "$OUT/candidate_eh"
fi
if [[ "$OAUTH_PROBE_VERSION" = v1 ]]; then
  python3 tools/hack/oauth-v0/check_actions.py "$OUT/candidate_eh_nodiag"
fi
test "$build_status" -eq 0
