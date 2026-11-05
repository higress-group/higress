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

# Release workflows source this file so every `oras` invocation gets bounded
# retries against transient transport failures (GitHub runners intermittently
# get connections reset by the ACR auth endpoint). The wrapper shadows the
# oras binary with a function and keeps every workflow command line
# byte-identical.
#
# Only transport-level signatures are retried. Registry responses such as
# 404 absence evidence or authorization refusals are semantic inputs to the
# workflows' fail-closed classifiers, so they fail fast and surface
# unchanged. On the final failure the captured stderr is replayed to stderr,
# which also preserves caller-level `2>file` captures.
#
# Success-path stderr (progress noise) is swallowed; the JSON descriptors and
# digests the workflows consume travel over stdout and are untouched.
oras() {
    local attempt status err_file
    err_file=$(mktemp)
    for attempt in 1 2 3 4; do
        status=0
        command oras "$@" 2>"$err_file" || status=$?
        if [ "$status" -eq 0 ]; then
            rm -f "$err_file"
            return 0
        fi
        if ! grep -Eiq 'connection reset by peer|connection refused|context deadline exceeded|i/o timeout|Client\.Timeout exceeded|broken pipe|unexpected EOF|no route to host|network is unreachable|TLS handshake timeout|proxyconnect' "$err_file"; then
            break
        fi
        sleep "$((attempt * 2))"
    done
    cat "$err_file" >&2
    rm -f "$err_file"
    return "$status"
}
