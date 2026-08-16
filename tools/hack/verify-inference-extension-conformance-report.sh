#!/usr/bin/env bash

# Copyright (c) 2026 Alibaba Group Holding Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

readonly VERSION="${INFERENCE_EXTENSION_VERSION:?INFERENCE_EXTENSION_VERSION must be set}"
readonly EXPECTED_PASSED="${INFERENCE_EXTENSION_EXPECTED_PASSED:?INFERENCE_EXTENSION_EXPECTED_PASSED must be set}"
readonly REPORT="${INFERENCE_EXTENSION_REPORT:?INFERENCE_EXTENSION_REPORT must be set}"

if [[ ! -s "${REPORT}" ]]; then
  echo "Conformance report was not generated: ${REPORT}" >&2
  exit 1
fi

report_value() {
  local key="$1"
  awk -v key="${key}" '$1 == key ":" {print $2; exit}' "${REPORT}"
}

actual_version="$(report_value GatewayAPIInferenceExtensionVersion)"
result="$(report_value result)"
passed="$(report_value Passed)"
failed="$(report_value Failed)"
skipped="$(report_value Skipped)"

if [[ "${actual_version}" != "${VERSION}" ]]; then
  echo "Report version ${actual_version:-<missing>} does not match ${VERSION}" >&2
  exit 1
fi
if [[ "${result}" != "success" || "${passed}" != "${EXPECTED_PASSED}" || "${failed}" != "0" || "${skipped}" != "0" ]]; then
  echo "Unexpected conformance result: result=${result:-<missing>} passed=${passed:-<missing>} failed=${failed:-<missing>} skipped=${skipped:-<missing>}" >&2
  exit 1
fi

echo "Gateway API Inference Extension ${VERSION}: ${passed} passed, ${failed} failed, ${skipped} skipped"
