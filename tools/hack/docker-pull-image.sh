# Copyright (c) 2022 Alibaba Group Holding Ltd.

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

#      http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Docker variables
readonly IMAGE="$1"
readonly TAG="$2"

docker::image::pull() {
    docker pull "$@"
}

# Pull the docker image to the local.
echo "Pulling image ${IMAGE}:${TAG} to local ..."
docker::image::pull "${IMAGE}:${TAG}"
