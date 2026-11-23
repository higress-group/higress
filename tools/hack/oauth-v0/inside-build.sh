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

set -uo pipefail
OAUTH_PROBE_VERSION="${OAUTH_PROBE_VERSION:-v0}"
case "$OAUTH_PROBE_VERSION" in
  v0) variants=(default_disabled candidate_eh) ;;
  v1) variants=(candidate_eh candidate_eh_nodiag) ;;
  production) variants=(oauth) ;;
  *) exit 1 ;;
esac
OUT="/repo/out/oauth-$OAUTH_PROBE_VERSION"
mkdir -p "$OUT/native" "$OUT/hmac"
bazel --version > "$OUT/bazel-version.txt"
package=tests/oauth_parse_probe
if [[ "$OAUTH_PROBE_VERSION" = production ]]; then
  package=extensions/oauth
  bazel test --runs_per_test=2 --cache_test_results=no --test_output=all //extensions/oauth:oauth_test 2>&1 | tee "$OUT/native/native.log"
  printf '%s\n' "${PIPESTATUS[0]}" > "$OUT/native/run.exit"
  cp -rL bazel-testlogs/extensions/oauth/oauth_test "$OUT/native/testlogs"
else
bazel build --subcommands --verbose_failures //tests/oauth_parse_probe:native_probe 2>&1 | tee "$OUT/native/build.log"
printf '%s\n' "${PIPESTATUS[0]}" > "$OUT/native/build.exit"
if [[ "$(cat "$OUT/native/build.exit")" -eq 0 ]]; then
  bazel-bin/tests/oauth_parse_probe/native_probe "$OUT/fixtures/cases.tsv" 2>&1 | tee "$OUT/native/results.tsv"
  printf '%s\n' "${PIPESTATUS[0]}" > "$OUT/native/run.exit"
fi
fi
if [[ "$OAUTH_PROBE_VERSION" = v0 ]]; then
  bazel test --test_output=all //extensions/hmac_auth:hmac_auth_test 2>&1 | tee "$OUT/hmac/native.log"
  printf '%s\n' "${PIPESTATUS[0]}" > "$OUT/hmac/native.exit"
fi
for variant in "${variants[@]}"; do
  mkdir -p "$OUT/$variant"
  bazel build --subcommands --verbose_failures --materialize_param_files \
    --execution_log_json_file="$OUT/$variant/execution.json" \
    "//$package:$variant.wasm" 2>&1 | tee "$OUT/$variant/build.log"
  build_status=${PIPESTATUS[0]}
  printf '%s\n' "$build_status" > "$OUT/$variant/build.exit"
  if [[ "$build_status" -eq 0 ]]; then
    cp "bazel-bin/$package/$variant.wasm" "$OUT/$variant/plugin.wasm"
  fi
  mkdir -p "$OUT/params"
  find -L bazel-out -name '*.params' -path "*$package*" -type f -exec cp --parents '{}' "$OUT/params/" ';'
  bazel aquery --output=jsonproto --include_commandline --include_param_files \
    "mnemonic('CppCompile|CppLink', deps(//$package:$variant.wasm))" \
    > "$OUT/$variant/actions.json" 2> "$OUT/$variant/aquery.log"
  printf '%s\n' "$?" > "$OUT/$variant/aquery.exit"
done
base="$(bazel info output_base)"
mkdir -p "$OUT/actual-sources"
cp "$base/external/proxy_wasm_cpp_sdk/bazel/defs.bzl" "$OUT/actual-sources/sdk-defs.bzl"
cp "$base/external/proxy_wasm_cpp_sdk/bazel/repositories.bzl" "$OUT/actual-sources/sdk-repositories.bzl"
cp "$base/external/com_github_thalhammer_jwt_cpp/include/jwt-cpp/jwt.h" "$OUT/actual-sources/jwt.h"
cp "$base/external/com_github_thalhammer_jwt_cpp/include/jwt-cpp/base.h" "$OUT/actual-sources/base.h"
find -L "$base/external" -path '*/emscripten/emcc.py' -type f -print > "$OUT/emcc-paths.txt"
while IFS= read -r compiler; do
  python3 "$compiler" --version >> "$OUT/emscripten-version.txt" 2>&1
  printf '%s\n' "$?" >> "$OUT/emscripten-version-command.exit"
  emscripten_root="$(dirname "$compiler")"
  cp "$emscripten_root/emscripten-version.txt" "$OUT/actual-sources/emscripten-version.txt"
  cp "$emscripten_root/src/settings.js" "$OUT/actual-sources/emscripten-settings.js"
  cp "$emscripten_root/tools/link.py" "$OUT/actual-sources/emscripten-link.py"
  cp "$emscripten_root/tools/system_libs.py" "$OUT/actual-sources/emscripten-system-libs.py"
done < "$OUT/emcc-paths.txt"
exit 0
