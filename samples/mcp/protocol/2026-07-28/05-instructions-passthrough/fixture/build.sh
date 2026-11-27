#!/usr/bin/env bash
set -euo pipefail
unset GH_TOKEN GITHUB_TOKEN
CASE_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
source "$CASE_DIR/../../../environment/scripts/common.sh"
for name in kind kubectl; do require_command "$name"; done
select_container_engine
if ! cluster_is_owned; then
  echo "start the owned public MCP demo environment with environment/scripts/up.sh first" >&2
  exit 2
fi
output=${MCP_DEMO_EVIDENCE:-$MCP_DEMO_RUNTIME/instructions}
mkdir -p "$output"
image=localhost/mcp-demo/instructions-sdk:1.0.0
builder_image=docker.io/library/golang:1.25.0-alpine
# BuildKit may keep FROM layers only in its own cache, outside image inspect.
if [[ "$MCP_DEMO_CONTAINER_ENGINE" == docker ]]; then
  "$MCP_DEMO_CONTAINER_ENGINE" pull "$builder_image"
fi
"$MCP_DEMO_CONTAINER_ENGINE" build -t "$image" -f "$CASE_DIR/fixture/Dockerfile" "$CASE_DIR"
"$MCP_DEMO_CONTAINER_ENGINE" image inspect "$image" --format '{{.Id}}' >"$output/fixture-image-id.txt"
"$MCP_DEMO_CONTAINER_ENGINE" image inspect "$builder_image" --format '{{.Id}} {{json .RepoDigests}}' >"$output/fixture-builder-image.txt"
if [[ "$MCP_DEMO_CONTAINER_ENGINE" == podman ]]; then
  archive="$output/instructions-sdk-image.tar"
  podman save --format docker-archive -o "$archive" "$image"
  kind load image-archive "$archive" --name "$MCP_DEMO_CLUSTER" --nodes "${MCP_DEMO_CLUSTER}-control-plane"
  unlink "$archive"
else
  kind load docker-image "$image" --name "$MCP_DEMO_CLUSTER"
fi
