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

readonly VERSION="${METALLB_VERSION:-v0.13.7}"

kubectl apply -f "https://raw.githubusercontent.com/metallb/metallb/${VERSION}/config/manifests/metallb-native.yaml"
kubectl wait --namespace metallb-system --for=condition=Available deployment/controller --timeout=180s
kubectl wait --namespace metallb-system --for=condition=Ready pod --selector=component=speaker --timeout=180s

gateway=""
while IFS= read -r candidate; do
  if [[ "${candidate}" =~ ^([0-9]+\.){3}[0-9]+$ ]]; then
    gateway="${candidate}"
    break
  fi
done < <(docker network inspect kind --format '{{range .IPAM.Config}}{{if .Gateway}}{{println .Gateway}}{{end}}{{end}}')
if [[ -z "${gateway}" ]]; then
  echo "Unable to determine the IPv4 gateway for the kind Docker network" >&2
  exit 1
fi
prefix="${gateway%.*}"

kubectl apply -f - <<EOF
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: inference-conformance
  namespace: metallb-system
spec:
  addresses:
  - ${prefix}.200-${prefix}.250
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: inference-conformance
  namespace: metallb-system
spec:
  ipAddressPools:
  - inference-conformance
EOF
