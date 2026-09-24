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

set -euo pipefail


TYPE=${PLUGIN_TYPE-""}
INNER_PLUGIN_NAME=${PLUGIN_NAME-""}
INNER_PLUGIN_ROOT=${PLUGIN_ROOT-""}
REPO_ROOT=$(cd "$(dirname "$0")/../.." && pwd)
RELEASE_CATALOG=${PLUGIN_RELEASE_CATALOG:-"$REPO_ROOT/plugins/release/catalog.json"}
GO_BATCH_SCOPE=${PLUGIN_GO_BATCH_SCOPE:-"release"}

if [ "$TYPE" == "CPP" ]
then
    cd ./plugins/wasm-cpp/
    if [ ! -n "$INNER_PLUGIN_NAME" ]; then
        echo "You must specify which cpp plugin you want to compile"
    else
        echo "🚀 Build CPP WasmPlugin: $INNER_PLUGIN_NAME"
        PLUGIN_NAME=${INNER_PLUGIN_NAME} make build
    fi
elif [ "$TYPE" == "RUST" ]
then
    cd ./plugins/wasm-rust/
    make lint-base
    make test-base
    if [ ! -n "$INNER_PLUGIN_NAME" ]; then
        EXTENSIONS_DIR=$(pwd)"/extensions/"
        echo "🚀 Build all Rust WasmPlugins under folder of $EXTENSIONS_DIR"
        for file in `ls $EXTENSIONS_DIR`                                   
            do
                if [ -d $EXTENSIONS_DIR$file ]; then 
                    name=${file##*/}
                    echo "🚀 Build Rust WasmPlugin: $name"
                    PLUGIN_NAME=${name} make lint 
                    PLUGIN_NAME=${name} make test 
                    PLUGIN_NAME=${name} make build
                fi
            done
    else
        echo "🚀 Build Rust WasmPlugin: $INNER_PLUGIN_NAME"
        PLUGIN_ROOT=${INNER_PLUGIN_ROOT:-extensions} PLUGIN_NAME=${INNER_PLUGIN_NAME} make lint
        PLUGIN_ROOT=${INNER_PLUGIN_ROOT:-extensions} PLUGIN_NAME=${INNER_PLUGIN_NAME} make build
    fi
elif [ "$TYPE" == "GO" ] || [ -z "$TYPE" ]
then
    echo "Build wasm-go plugins (default when PLUGIN_TYPE is unset)"
    cd "$REPO_ROOT/plugins/wasm-go/"
    if [ ! -n "$INNER_PLUGIN_NAME" ]; then
        # The release catalog is the sole production batch-build authority.
        # E2E additionally builds official Go extensions referenced by the
        # checked-in conformance manifests, without changing the production
        # release set or relying on VERSION naming conventions.
        mapfile -t release_source_dirs < <(jq -er '.plugins[] | select(.implementation == "go" and .releaseEligible == true) | .sourceDir' "$RELEASE_CATALOG" | sort)
        case "$GO_BATCH_SCOPE" in
        release)
            source_dirs=("${release_source_dirs[@]}")
            ;;
        e2e)
            mapfile -t conformance_source_dirs < <(
                find "$REPO_ROOT/test/e2e/conformance/tests" -type f -name '*.yaml' \
                    -exec grep -hEo 'file:///opt/plugins/wasm-go/extensions/[a-z0-9][a-z0-9-]*/plugin\.wasm' {} + |
                    sed -e 's#^file:///opt/plugins/#plugins/#' -e 's#/plugin\.wasm$##' |
                    sort -u
            )
            for source_dir in "${conformance_source_dirs[@]}"; do
                if ! jq -e --arg source_dir "$source_dir" \
                    'any(.plugins[]; .implementation == "go" and .sourceDir == $source_dir)' \
                    "$RELEASE_CATALOG" >/dev/null; then
                    echo "Conformance manifest references unmanaged Go extension: $source_dir" >&2
                    exit 1
                fi
            done
            mapfile -t source_dirs < <(
                printf '%s\n' "${release_source_dirs[@]}" "${conformance_source_dirs[@]}" | sort -u
            )
            ;;
        *)
            echo "Unsupported PLUGIN_GO_BATCH_SCOPE=$GO_BATCH_SCOPE (expected release or e2e)" >&2
            exit 1
            ;;
        esac
        test "${#source_dirs[@]}" -gt 0
        if [ "${PLUGIN_BATCH_LIST_ONLY:-false}" = true ]; then
            printf '%s\n' "${source_dirs[@]}"
            exit 0
        fi
        echo "🚀 Build Go WasmPlugin batch scope $GO_BATCH_SCOPE from $RELEASE_CATALOG"
        for source_dir in "${source_dirs[@]}"; do
            extension_dir="$REPO_ROOT/$source_dir"
            name=${source_dir##*/}
            test -d "$extension_dir"
            test -f "$extension_dir/VERSION"
            version=$(tr -d '[:space:]' < "$extension_dir/VERSION")
            echo "🚀 Build Go WasmPlugin: $name (version $version)"
            buildrc_file="$extension_dir/.buildrc"
            if [ -f "$buildrc_file" ]; then
                echo "Found .buildrc file, sourcing it..."
                . "$buildrc_file"
            fi
            echo "EXTRA_TAGS=${EXTRA_TAGS:-}"
            PLUGIN_NAME="$name" EXTRA_TAGS="${EXTRA_TAGS:-}" make build
            unset EXTRA_TAGS || true
        done
    else
        echo "🚀 Build Go WasmPlugin: $INNER_PLUGIN_NAME"
        PLUGIN_ROOT=${INNER_PLUGIN_ROOT:-extensions} PLUGIN_NAME=${INNER_PLUGIN_NAME} make build
    fi
else
    echo "Unsupported PLUGIN_TYPE=$TYPE (expected GO, RUST, or CPP)" >&2
    exit 1
fi
