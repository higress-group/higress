#!/usr/bin/env bash

# Copyright (c) 2023 Alibaba Group Holding Ltd.

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

#      http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname -- "$0")" &> /dev/null && pwd)"
source "${SCRIPT_DIR}/setup-istio-env.sh"

if ! command -v patch >/dev/null 2>&1; then
    echo "ERROR: 'patch' command not found. Please install it first (e.g. apt install patch / yum install patch / brew install gpatch)." >&2
    exit 1
fi

cd ${ROOT}/external/proxy

if patch_output=$(patch -d . -s --dry-run -p1 < ${SCRIPT_DIR}/build-envoy.patch 2>&1); then
    patch -d . -s -p1 < ${SCRIPT_DIR}/build-envoy.patch
elif reverse_output=$(patch -d . -s -R --dry-run -p1 < ${SCRIPT_DIR}/build-envoy.patch 2>&1); then
    echo "build-envoy.patch was already patched"
else
    echo "ERROR: failed to apply build-envoy.patch" >&2
    echo "Patch output:" >&2
    echo "${patch_output}" >&2
    echo "Reverse patch output:" >&2
    echo "${reverse_output}" >&2
    exit 1
fi

CONDITIONAL_HOST_MOUNTS+="--mount type=bind,source=${ROOT}/external/package,destination=/home/package "
CONDITIONAL_HOST_MOUNTS+="--mount type=bind,source=${ROOT}/external/envoy,destination=/home/envoy "

BUILD_TOOLS_IMG=${BUILD_TOOLS_IMG:-"higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/build-tools-proxy:master-eebcdda8856e2d4f528991d27d4808880cce4c52"}

BUILD_WITH_CONTAINER=1 \
    CONDITIONAL_HOST_MOUNTS=${CONDITIONAL_HOST_MOUNTS} \
    BUILD_ENVOY_BINARY_ONLY=1 \
    DOCKER_RUN_OPTIONS="--user root -e HTTP_PROXY -e HTTPS_PROXY" \
    IMG=${BUILD_TOOLS_IMG} \
    make test_release
