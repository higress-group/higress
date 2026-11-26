# Copyright 2026 Higress Authors
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

build_promotion_verify_args() {
    local release_id plan previous previous_path
    release_id=$(jq -er .gatewayVersion "$SNAPSHOT_PATH")
    if [ "${RELEASE_MODE:-standard}" = cpp-hotfix ]; then
        test "$SNAPSHOT_PATH" = plugins/release/snapshots/cpp-hotfix-2.0.1.json
        release_id=cpp-hotfix-2.0.1
    fi
    plan="plugins/release/plans/$release_id.json"
    test -f "$plan"
    args=(verify-snapshot --root ../.. --catalog ../../plugins/release/catalog.json --snapshot "../../$SNAPSHOT_PATH" --plan "../../$plan" --committed-source "$SOURCE_COMMIT" --resolve --oci-source candidate)
    if [ "${RELEASE_MODE:-standard}" = cpp-hotfix ]; then
        args+=(--mode=cpp-hotfix)
    elif [ -z "$(jq -r '.bootstrapEvidence.path // empty' "$SNAPSHOT_PATH")" ]; then
        previous=$(jq -r '.previousRelease // empty' "$SNAPSHOT_PATH")
        [[ "$previous" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
        previous_path="plugins/release/snapshots/$previous.json"
        test -f "$previous_path"
        args+=(--previous "../../$previous_path")
    elif jq -e 'any(.plugins[]; .wasmSha256 != null or .candidateImport != null or .history != null) or .historyImport != null' "$SNAPSHOT_PATH" >/dev/null; then
        local artifact_freeze marker
        artifact_freeze=$(jq -er .sourceCommit "$SNAPSHOT_PATH")
        marker=$(jq -er .bootstrapEvidence.path "$SNAPSHOT_PATH")
        previous=$(jq -er .previousRelease "$SNAPSHOT_PATH")
        (cd tools/plugin-release && go run . bootstrap-snapshot --root ../.. --catalog ../../plugins/release/catalog.json --gateway-version "$previous" --source "$artifact_freeze" --existing-evidence "../../$marker" --output /tmp/promotion-bootstrap-previous.json)
        args+=(--previous /tmp/promotion-bootstrap-previous.json)
    fi
}
