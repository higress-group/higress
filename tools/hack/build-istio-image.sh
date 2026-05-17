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

source "$(dirname -- "$0")/setup-istio-env.sh"

cd ${ROOT}/external/istio
rm -rf out/linux_${TARGET_ARCH}; 

CONDITIONAL_HOST_MOUNTS+="--mount type=bind,source=${ROOT}/external/package,destination=/home/package "

DOCKER_RUN_OPTIONS+="-e HTTP_PROXY -e HTTPS_PROXY"

BUILD_TOOLS_IMG=${BUILD_TOOLS_IMG:-"higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/build-tools:release-1.19-ef344298e65eeb2d9e2d07b87eb4e715c2def613"}

ORIGINAL_HUB=${HUB}

echo "IMG_URL=$IMG_URL"

if [ -n "$IMG_URL" ]; then
  TAG=${IMG_URL#*:}
  HUB=${IMG_URL%:*}
  HUB=${HUB%/*}
  if [ "$TAG" == "${IMG_URL}" ]; then
    TAG=latest
  fi
fi

echo "HUB=$HUB"
echo "TAG=$TAG"

# Set DOCKER_ARCHITECTURES based on TARGET_ARCH
if [ "${TARGET_ARCH}" = "arm64" ]; then
    export DOCKER_ARCHITECTURES="linux/arm64"
else
    export DOCKER_ARCHITECTURES="linux/amd64"
fi

echo "DOCKER_ARCHITECTURES=$DOCKER_ARCHITECTURES"

GOOS_LOCAL=linux TARGET_OS=linux TARGET_ARCH=${TARGET_ARCH} \
    ISTIO_ENVOY_LINUX_RELEASE_URL=${ISTIO_ENVOY_LINUX_RELEASE_URL} \
    BUILD_WITH_CONTAINER=1 \
    USE_REAL_USER=${USE_REAL_USER:-0} \
    CONDITIONAL_HOST_MOUNTS=${CONDITIONAL_HOST_MOUNTS} \
    DOCKER_BUILD_VARIANTS=default DOCKER_TARGETS="${DOCKER_TARGETS}" \
    DOCKER_ARCHITECTURES="${DOCKER_ARCHITECTURES}" \
    ISTIO_BASE_REGISTRY="${ORIGINAL_HUB}" \
    BASE_VERSION="${HIGRESS_BASE_VERSION}" \
    DOCKER_RUN_OPTIONS=${DOCKER_RUN_OPTIONS} \
    HUB="${HUB}" \
    TAG="${TAG}" \
    IMG=${BUILD_TOOLS_IMG} \
    make "$@"
