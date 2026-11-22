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
OUT=/repo/out/oauth-v0
mkdir -p "$OUT/native" "$OUT/hmac" "$OUT/default_disabled" "$OUT/candidate_eh"
bazel --version > "$OUT/bazel-version.txt"
bazel build --subcommands --verbose_failures //tests/oauth_parse_probe:native_probe 2>&1 | tee "$OUT/native/build.log"
printf '%s\n' "${PIPESTATUS[0]}" > "$OUT/native/build.exit"
if [[ "$(cat "$OUT/native/build.exit")" -eq 0 ]]; then
  bazel-bin/tests/oauth_parse_probe/native_probe "$OUT/fixtures/cases.tsv" 2>&1 | tee "$OUT/native/results.tsv"
  printf '%s\n' "${PIPESTATUS[0]}" > "$OUT/native/run.exit"
fi
bazel test --test_output=all //extensions/hmac_auth:hmac_auth_test 2>&1 | tee "$OUT/hmac/native.log"
printf '%s\n' "${PIPESTATUS[0]}" > "$OUT/hmac/native.exit"
for variant in default_disabled candidate_eh; do
  bazel build --subcommands --verbose_failures --materialize_param_files \
    --execution_log_json_file="$OUT/$variant/execution.json" \
    "//tests/oauth_parse_probe:$variant.wasm" 2>&1 | tee "$OUT/$variant/build.log"
  build_status=${PIPESTATUS[0]}
  printf '%s\n' "$build_status" > "$OUT/$variant/build.exit"
  if [[ "$build_status" -eq 0 ]]; then
    cp "bazel-bin/tests/oauth_parse_probe/$variant.wasm" "$OUT/$variant/plugin.wasm"
  fi
  bazel aquery --output=jsonproto --include_commandline --include_param_files \
    "mnemonic('CppCompile|CppLink', deps(//tests/oauth_parse_probe:$variant.wasm))" \
    > "$OUT/$variant/actions.json" 2> "$OUT/$variant/aquery.log"
  printf '%s\n' "$?" > "$OUT/$variant/aquery.exit"
done
base="$(bazel info output_base)"
mkdir -p "$OUT/actual-sources"
cp "$base/external/proxy_wasm_cpp_sdk/bazel/defs.bzl" "$OUT/actual-sources/sdk-defs.bzl"
cp "$base/external/proxy_wasm_cpp_sdk/bazel/repositories.bzl" "$OUT/actual-sources/sdk-repositories.bzl"
cp "$base/external/com_github_thalhammer_jwt_cpp/include/jwt-cpp/jwt.h" "$OUT/actual-sources/jwt.h"
cp "$base/external/com_github_thalhammer_jwt_cpp/include/jwt-cpp/base.h" "$OUT/actual-sources/base.h"
find "$base/external" -path '*/emscripten/emcc.py' -type f -print > "$OUT/emcc-paths.txt"
while IFS= read -r compiler; do
  python3 "$compiler" --version >> "$OUT/emscripten-version.txt" 2>&1
done < "$OUT/emcc-paths.txt"
find bazel-out -name '*.params' -path '*oauth_parse_probe*' -type f -exec cp --parents '{}' "$OUT/" ';'
exit 0
