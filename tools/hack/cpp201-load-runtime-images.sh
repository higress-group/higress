#!/usr/bin/env bash
# Copyright (c) 2026 Alibaba Group Holding Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
KIND="${KIND:-$ROOT/tools/bin/kind}"
EVIDENCE_DIR="${1:?evidence directory is required}"
PINS="$ROOT/tools/hack/cpp201-runtime-images.pins"

mkdir -p "$EVIDENCE_DIR"
: > "$EVIDENCE_DIR/images.tsv"
while IFS='|' read -r local_ref pinned_ref load_kind; do
  case "$local_ref" in
    ''|'#'*) continue ;;
  esac
  case "$pinned_ref" in
    *@sha256:*) ;;
    *) echo "Image is not digest pinned: $pinned_ref" >&2; exit 1 ;;
  esac
  if [[ "$local_ref" == *latest* || "$pinned_ref" == *latest* ]]; then
    echo "Mutable latest reference is forbidden: $local_ref $pinned_ref" >&2
    exit 1
  fi
  if [[ "$load_kind" != true ]]; then
    continue
  fi
  docker pull "$pinned_ref"
  docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$pinned_ref" > "$EVIDENCE_DIR/resolved-image.txt"
  grep -F "@${pinned_ref##*@}" "$EVIDENCE_DIR/resolved-image.txt"
  docker tag "$pinned_ref" "$local_ref"
  "$KIND" load docker-image --name higress "$local_ref"
  printf '%s\t%s\t%s\n' "$local_ref" "$pinned_ref" "$(tr '\n' ',' < "$EVIDENCE_DIR/resolved-image.txt")" >> "$EVIDENCE_DIR/images.tsv"
done < "$PINS"

test "$(wc -l < "$EVIDENCE_DIR/images.tsv")" -eq 15
