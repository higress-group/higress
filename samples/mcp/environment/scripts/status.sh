#!/usr/bin/env bash
set -euo pipefail
source "$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh"

for command_name in kind kubectl curl; do
  require_command "$command_name"
done
select_container_engine

if ! cluster_exists; then
  echo "cluster $MCP_DEMO_CLUSTER does not exist" >&2
  exit 1
fi
if ! cluster_is_owned; then
  echo "cluster $MCP_DEMO_CLUSTER is not owned by this demo environment" >&2
  exit 1
fi

kubectl config use-context "kind-$MCP_DEMO_CLUSTER" >/dev/null
kubectl get pods -n higress-system
kubectl get pods -n mcp-demo

if curl -sS "http://127.0.0.1:$MCP_DEMO_BACKEND_PORT/healthz" >/dev/null; then
  echo "backend port-forward: ready"
else
  echo "backend port-forward: unavailable"
fi

if curl -sS -o /dev/null "http://127.0.0.1:$MCP_DEMO_GATEWAY_PORT/"; then
  echo "gateway port-forward: ready"
else
  echo "gateway port-forward: unavailable"
fi

if curl -sS -o /dev/null "http://127.0.0.1:$MCP_DEMO_CONSOLE_PORT/"; then
  echo "console port-forward: ready"
else
  echo "console port-forward: unavailable"
fi
