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

if [[ ! "${VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "Invalid Gateway API Inference Extension version: ${VERSION}" >&2
  exit 1
fi

if [[ -f "${SOURCE_DIR}/.higress-conformance-version" ]] &&
  [[ "$(<"${SOURCE_DIR}/.higress-conformance-version")" == "${VERSION}" ]]; then
  exit 0
fi

readonly WORK_DIR="$(mktemp -d)"
trap 'rm -rf "${WORK_DIR}"' EXIT

curl -fLSs --retry 5 --retry-delay 2 --retry-connrefused \
  "https://github.com/kubernetes-sigs/gateway-api-inference-extension/archive/refs/tags/${VERSION}.tar.gz" \
  -o "${WORK_DIR}/source.tar.gz"
mkdir -p "${WORK_DIR}/source"
tar -xzf "${WORK_DIR}/source.tar.gz" --strip-components=1 -C "${WORK_DIR}/source"

for required_file in go.work conformance/go.mod conformance/conformance_test.go \
  config/crd/bases/inference.networking.k8s.io_inferencepools.yaml; do
  if [[ ! -f "${WORK_DIR}/source/${required_file}" ]]; then
    echo "The ${VERSION} release does not contain ${required_file}" >&2
    exit 1
  fi
done

if ! grep -Eq "BundleVersion[[:space:]]*=[[:space:]]*\"${VERSION}\"" \
  "${WORK_DIR}/source/version/version.go"; then
  echo "Downloaded source does not identify itself as ${VERSION}" >&2
  exit 1
fi

mkdir -p "$(dirname "${SOURCE_DIR}")"
rm -rf "${SOURCE_DIR}"
mv "${WORK_DIR}/source" "${SOURCE_DIR}"
printf '%s\n' "${VERSION}" >"${SOURCE_DIR}/.higress-conformance-version"
