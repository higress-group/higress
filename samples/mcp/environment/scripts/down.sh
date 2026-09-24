#!/usr/bin/env bash
set -euo pipefail
source "$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh"

require_command kind
require_command kubectl
select_container_engine

if cluster_exists; then
  if ! cluster_is_owned; then
    echo "refusing to delete unowned Kind cluster $MCP_DEMO_CLUSTER" >&2
    exit 1
  fi
elif cluster_exists_anywhere; then
  echo "refusing to delete unowned cluster $MCP_DEMO_CLUSTER" >&2
  exit 1
elif [[ -f "$MCP_DEMO_PROVIDER_FILE" || -f "$MCP_DEMO_CLUSTER_FILE" || -f "$MCP_DEMO_INSTANCE_FILE" ]]; then
  if ! local_ownership_matches; then
    echo "runtime ownership state does not match cluster $MCP_DEMO_CLUSTER" >&2
    exit 1
  fi
fi

stop_port_forward gateway
stop_port_forward console
stop_port_forward backend

if cluster_exists; then
  kind delete cluster --name "$MCP_DEMO_CLUSTER"
  clear_cluster_ownership
else
  echo "cluster $MCP_DEMO_CLUSTER is already absent"
  clear_cluster_ownership
fi

echo "Higress MCP demo environment has been removed."
