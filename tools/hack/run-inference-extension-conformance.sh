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
readonly SOURCE_DIR="${INFERENCE_EXTENSION_SOURCE_DIR:?INFERENCE_EXTENSION_SOURCE_DIR must be set}"
readonly REPORT="${INFERENCE_EXTENSION_REPORT:?INFERENCE_EXTENSION_REPORT must be set}"
readonly EXPECTED_PASSED="${INFERENCE_EXTENSION_EXPECTED_PASSED:?INFERENCE_EXTENSION_EXPECTED_PASSED must be set}"
readonly CONTACT="${INFERENCE_EXTENSION_CONTACT:-@higress-group/maintainers}"
readonly IMPLEMENTATION_VERSION="${HIGRESS_CONFORMANCE_VERSION:-$(git rev-parse HEAD)}"
readonly ORGANIZATION="${HIGRESS_CONFORMANCE_ORGANIZATION:-higress-group}"
readonly PROJECT="${HIGRESS_CONFORMANCE_PROJECT:-higress}"
readonly URL="${HIGRESS_CONFORMANCE_URL:-https://github.com/higress-group/higress}"

if [[ "$(<"${SOURCE_DIR}/.higress-conformance-version")" != "${VERSION}" ]]; then
  echo "Conformance source in ${SOURCE_DIR} does not match ${VERSION}" >&2
  exit 1
fi

mkdir -p "$(dirname "${REPORT}")"
args=(
  -gateway-class=higress
  -cleanup-base-resources=false
  "-report-output=${REPORT}"
  "-organization=${ORGANIZATION}"
  "-project=${PROJECT}"
  "-url=${URL}"
  "-contact=${CONTACT}"
  "-version=${IMPLEMENTATION_VERSION}"
  -mode=default
  -allow-crds-mismatch=false
)
if [[ -n "${INFERENCE_EXTENSION_RUN_TEST:-}" ]]; then
  args+=("-run-test=${INFERENCE_EXTENSION_RUN_TEST}")
fi

(
  cd "${SOURCE_DIR}"
  GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}" \
    go test -v ./conformance -run '^TestConformance$' -args "${args[@]}"
)

INFERENCE_EXTENSION_VERSION="${VERSION}" \
INFERENCE_EXTENSION_EXPECTED_PASSED="${EXPECTED_PASSED}" \
INFERENCE_EXTENSION_REPORT="${REPORT}" \
  "$(dirname "$0")/verify-inference-extension-conformance-report.sh"
