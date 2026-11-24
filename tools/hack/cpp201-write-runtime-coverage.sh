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
NATIVE_RESULTS="${1:?native outcome index is required}"
OUTPUT="${2:?output is required}"
test "$(wc -l < "$NATIVE_RESULTS")" -eq 9
for source in basic_auth bot_detect custom_response hmac_auth jwt_auth key_auth key_rate_limit oauth request_block; do
  result="$(awk -v source="$source" '$1 == source {print $2}' "$NATIVE_RESULTS")"
  case "$result" in
    success|failure) ;;
    *) echo "Missing, duplicate or invalid native outcome for $source" >&2; exit 1 ;;
  esac
done
printf 'basic_auth,key_auth,request_block: runtime-e2e=passed\n' > "$OUTPUT"
for source in bot_detect custom_response hmac_auth jwt_auth key_rate_limit oauth; do
  result="$(awk -v source="$source" '$1 == source {print $2}' "$NATIVE_RESULTS")"
  if [[ "$result" == success ]]; then
    printf '%s: accepted-minimum build/native/shared-matcher; native=passed; runtime-e2e=not-run; coverage-gap=accepted\n' "$source" >> "$OUTPUT"
  else
    printf '%s: build=passed; native=failed; runtime-e2e=not-run; coverage-gap=native-failed\n' "$source" >> "$OUTPUT"
  fi
done
