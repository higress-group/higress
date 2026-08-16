#!/usr/bin/env bash
set -euo pipefail

PLUGIN_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
VERSION_DIR=$(CDPATH= cd -- "$PLUGIN_DIR/.." && pwd)
MCP_DEMO_ROOT=$(CDPATH= cd -- "$VERSION_DIR/../.." && pwd)
source "$PLUGIN_DIR/source.env"

if [[ -f "$MCP_DEMO_ROOT/.runtime/container-engine" ]]; then
  echo "stop the MCP demo environment before rebuilding the plugin" >&2
  exit 1
fi

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  container_engine=docker
elif command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1; then
  container_engine=podman
else
  echo "Docker or Podman with a running engine is required" >&2
  exit 2
fi

output_dir="$MCP_DEMO_ROOT/.runtime/plugins/mcp-server/2026-07-28"
mkdir -p "$output_dir"

"$container_engine" build \
  --build-arg "HIGRESS_REPOSITORY=$MCP_DEMO_HIGRESS_REPOSITORY" \
  --build-arg "HIGRESS_REF=$MCP_DEMO_HIGRESS_REF" \
  -t "$MCP_DEMO_PLUGIN_BUILD_IMAGE" \
  -f "$PLUGIN_DIR/Dockerfile" \
  "$PLUGIN_DIR"

output_mount="$output_dir:/output:Z"
"$container_engine" run --rm \
  -v "$output_mount" \
  "$MCP_DEMO_PLUGIN_BUILD_IMAGE"

if command -v sha256sum >/dev/null 2>&1; then
  plugin_sha=$(sha256sum "$output_dir/plugin.wasm" | awk '{print $1}')
else
  plugin_sha=$(shasum -a 256 "$output_dir/plugin.wasm" | awk '{print $1}')
fi

printf '%s\n' "$MCP_DEMO_HIGRESS_REF" >"$output_dir/source-commit.txt"
printf '%s  plugin.wasm\n' "$plugin_sha" >"$output_dir/SHA256SUMS"

printf 'Built MCP 2026-07-28 plugin:\n'
printf '  source: %s@%s\n' "$MCP_DEMO_HIGRESS_REPOSITORY" "$MCP_DEMO_HIGRESS_REF"
printf '  output: %s\n' "$output_dir/plugin.wasm"
printf '  sha256: %s\n' "$plugin_sha"
