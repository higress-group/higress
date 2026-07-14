#!/usr/bin/env bash

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

# The golang-filter shared object is built against the Go bindings published by
# the envoy/envoy submodule, while the Envoy binary that loads it is built from
# that same submodule. The two must be produced from the same commit, otherwise
# the cgo boundary between the Go filter and the host Envoy can diverge.
#
# This script asserts that the github.com/higress-group/envoy commit pinned by
# plugins/golang-filter/go.mod matches the commit checked out for the
# envoy/envoy git submodule. Run it from the repository root.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${REPO_ROOT}"

GO_MOD="plugins/golang-filter/go.mod"
SUBMODULE_PATH="envoy/envoy"

# Commit recorded for the envoy/envoy submodule in the current tree. Reading it
# from the tree (rather than the checked-out submodule) means the check works
# even when submodules have not been initialized.
submodule_commit="$(git rev-parse "HEAD:${SUBMODULE_PATH}")"

# The replace directive pins github.com/higress-group/envoy to a pseudo-version
# of the form v0.0.0-<UTC timestamp>-<12-char commit prefix>.
pin_line="$(grep -E 'replace[[:space:]]+github.com/envoyproxy/envoy[[:space:]]+=>[[:space:]]+github.com/higress-group/envoy' "${GO_MOD}" || true)"
if [[ -z "${pin_line}" ]]; then
  echo "ERROR: could not find the github.com/higress-group/envoy replace directive in ${GO_MOD}" >&2
  exit 1
fi

pin_prefix="$(echo "${pin_line}" | grep -oE '[0-9a-f]{12}$' || true)"
if [[ -z "${pin_prefix}" ]]; then
  echo "ERROR: could not parse the commit prefix from the envoy pseudo-version in ${GO_MOD}" >&2
  echo "       line: ${pin_line}" >&2
  exit 1
fi

if [[ "${submodule_commit:0:12}" != "${pin_prefix}" ]]; then
  cat >&2 <<EOF
ERROR: golang-filter Envoy dependency is out of sync with the envoy/envoy submodule.

  envoy/envoy submodule commit : ${submodule_commit}
  ${GO_MOD} pin prefix         : ${pin_prefix}

The Go bindings used to build golang-filter must come from the same Envoy commit
as the host Envoy binary. When you bump the envoy/envoy submodule, update the
github.com/higress-group/envoy replace directive in ${GO_MOD} to the matching
pseudo-version and run 'go mod tidy'. You can obtain the pseudo-version with:

  GOPROXY=direct go list -m -json github.com/higress-group/envoy@${submodule_commit}
EOF
  exit 1
fi

echo "OK: ${GO_MOD} envoy pin (${pin_prefix}) matches envoy/envoy submodule (${submodule_commit})."
