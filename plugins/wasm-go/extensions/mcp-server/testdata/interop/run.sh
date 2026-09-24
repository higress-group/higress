#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
plugin_dir=$(cd -- "$script_dir/../.." && pwd)
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/higress-mcp-interop.XXXXXX")
ready_file="$temp_dir/endpoint"
host_log="$temp_dir/host.log"

cleanup() {
  status=$?
  if [ -n "${host_pid:-}" ]; then
    kill "$host_pid" 2>/dev/null || true
    wait "$host_pid" 2>/dev/null || true
  fi
  if [ "$status" -ne 0 ]; then
    cat "$host_log" 2>/dev/null || true
    echo "interop diagnostics retained in $temp_dir" >&2
    return "$status"
  fi
  rm -f -- "$ready_file" "$host_log"
  rmdir "$temp_dir"
}
trap cleanup EXIT

(cd "$plugin_dir" && go run ./testdata/interop/host -ready-file "$ready_file") >"$host_log" 2>&1 &
host_pid=$!

for _ in $(seq 1 80); do
  if [ -s "$ready_file" ]; then
    break
  fi
  if ! kill -0 "$host_pid" 2>/dev/null; then
    cat "$host_log"
    exit 1
  fi
  sleep 0.25
done
if [ ! -s "$ready_file" ]; then
  cat "$host_log"
  echo "interop host did not become ready" >&2
  exit 1
fi
export MCP_INTEROP_ROOT
MCP_INTEROP_ROOT=$(cat "$ready_file")

(cd "$script_dir/go-client" && go run .)
(cd "$script_dir/typescript" && npm ci --ignore-scripts --registry=https://registry.npmjs.org && node client.mjs)
