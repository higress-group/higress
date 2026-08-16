#!/usr/bin/env bash
set -euo pipefail

MCP_DEMO_ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
MCP_DEMO_RUNTIME="$MCP_DEMO_ROOT/.runtime"
MCP_DEMO_PROVIDER_FILE="$MCP_DEMO_RUNTIME/container-engine"
MCP_DEMO_CLUSTER_FILE="$MCP_DEMO_RUNTIME/cluster-name"
MCP_DEMO_INSTANCE_FILE="$MCP_DEMO_RUNTIME/instance-id"
MCP_DEMO_INSTANCE_CONFIGMAP=higress-mcp-demo-owner
MCP_DEMO_CLUSTER=${MCP_DEMO_CLUSTER:-higress-mcp-demo}
MCP_DEMO_KIND_NODE_IMAGE=${MCP_DEMO_KIND_NODE_IMAGE:-kindest/node:v1.32.2}
MCP_DEMO_HIGRESS_CHART_VERSION=${MCP_DEMO_HIGRESS_CHART_VERSION:-2.2.3}
MCP_DEMO_GATEWAY_PORT=${MCP_DEMO_GATEWAY_PORT:-18080}
MCP_DEMO_CONSOLE_PORT=${MCP_DEMO_CONSOLE_PORT:-18081}
MCP_DEMO_BACKEND_PORT=${MCP_DEMO_BACKEND_PORT:-18082}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
}

use_container_engine() {
  local engine=$1
  if ! command -v "$engine" >/dev/null 2>&1 || ! "$engine" info >/dev/null 2>&1; then
    echo "recorded container engine $engine is not available or not running" >&2
    exit 2
  fi
  MCP_DEMO_CONTAINER_ENGINE=$engine
  if [[ "$engine" == podman ]]; then
    export KIND_EXPERIMENTAL_PROVIDER=podman
  else
    unset KIND_EXPERIMENTAL_PROVIDER || true
  fi
}

select_container_engine() {
  if [[ -f "$MCP_DEMO_PROVIDER_FILE" ]]; then
    local recorded_engine
    recorded_engine=$(<"$MCP_DEMO_PROVIDER_FILE")
    case "$recorded_engine" in
      docker|podman) use_container_engine "$recorded_engine"; return ;;
      *) echo "invalid recorded container engine: $recorded_engine" >&2; exit 2 ;;
    esac
  fi
  if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    use_container_engine docker
    return
  fi
  if command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1; then
    use_container_engine podman
    return
  fi
  echo "Docker or Podman with a running engine is required" >&2
  exit 2
}

cluster_exists_with_engine() {
  local engine=$1
  local control_plane="${MCP_DEMO_CLUSTER}-control-plane"
  case "$engine" in
    podman)
      command -v podman >/dev/null 2>&1 && podman container exists "$control_plane" >/dev/null 2>&1
      ;;
    docker)
      command -v docker >/dev/null 2>&1 && docker container inspect "$control_plane" >/dev/null 2>&1
      ;;
    *) return 1 ;;
  esac
}

cluster_exists() {
  cluster_exists_with_engine "$MCP_DEMO_CONTAINER_ENGINE"
}

cluster_exists_anywhere() {
  cluster_exists_with_engine docker || cluster_exists_with_engine podman
}

local_ownership_matches() {
  [[ -f "$MCP_DEMO_PROVIDER_FILE" && -f "$MCP_DEMO_CLUSTER_FILE" && -f "$MCP_DEMO_INSTANCE_FILE" ]] || return 1
  [[ "$(<"$MCP_DEMO_PROVIDER_FILE")" == "$MCP_DEMO_CONTAINER_ENGINE" ]] || return 1
  [[ "$(<"$MCP_DEMO_CLUSTER_FILE")" == "$MCP_DEMO_CLUSTER" ]]
}

cluster_is_owned() {
  local_ownership_matches || return 1
  local expected_instance
  local actual_instance
  expected_instance=$(<"$MCP_DEMO_INSTANCE_FILE")
  [[ -n "$expected_instance" ]] || return 1
  actual_instance=$(kubectl --context "kind-$MCP_DEMO_CLUSTER" \
    -n kube-system get configmap "$MCP_DEMO_INSTANCE_CONFIGMAP" \
    -o jsonpath='{.data.instance-id}' 2>/dev/null || true)
  [[ "$actual_instance" == "$expected_instance" ]]
}

record_cluster_ownership() {
  local instance_id
  instance_id=$(uuidgen 2>/dev/null || true)
  if [[ -z "$instance_id" ]]; then
    instance_id="$(date +%s)-$$-$RANDOM"
  fi
  printf '%s\n' "$MCP_DEMO_CONTAINER_ENGINE" >"$MCP_DEMO_PROVIDER_FILE"
  printf '%s\n' "$MCP_DEMO_CLUSTER" >"$MCP_DEMO_CLUSTER_FILE"
  printf '%s\n' "$instance_id" >"$MCP_DEMO_INSTANCE_FILE"
  kubectl --context "kind-$MCP_DEMO_CLUSTER" -n kube-system create configmap "$MCP_DEMO_INSTANCE_CONFIGMAP" \
    --from-literal="instance-id=$instance_id" >/dev/null
}

clear_cluster_ownership() {
  unlink "$MCP_DEMO_PROVIDER_FILE" 2>/dev/null || true
  unlink "$MCP_DEMO_CLUSTER_FILE" 2>/dev/null || true
  unlink "$MCP_DEMO_INSTANCE_FILE" 2>/dev/null || true
}

stop_port_forward() {
  local name=$1
  local pid_file="$MCP_DEMO_RUNTIME/${name}.pid"
  local command_file="$MCP_DEMO_RUNTIME/${name}.command"
  if [[ -f "$pid_file" ]]; then
    local pid
    pid=$(<"$pid_file")
    if [[ "$pid" =~ ^[0-9]+$ && -f "$command_file" ]] && kill -0 "$pid" >/dev/null 2>&1; then
      local command_line
      local expected_command
      command_line=$(ps -p "$pid" -o command= 2>/dev/null || true)
      expected_command=$(<"$command_file")
      if [[ -n "$expected_command" && "$command_line" == *"$expected_command"* ]]; then
        kill "$pid" >/dev/null 2>&1 || true
        wait "$pid" 2>/dev/null || true
      else
        echo "not stopping stale $name PID $pid: command does not match this demo" >&2
      fi
    fi
    unlink "$pid_file" 2>/dev/null || true
  fi
  unlink "$command_file" 2>/dev/null || true
}

start_port_forward() {
  local name=$1
  local namespace=$2
  local resource=$3
  local ports=$4
  local command_signature="kubectl -n $namespace port-forward --address 127.0.0.1 $resource $ports"
  stop_port_forward "$name"
  printf '%s\n' "$command_signature" >"$MCP_DEMO_RUNTIME/${name}.command"
  nohup kubectl -n "$namespace" port-forward --address 127.0.0.1 "$resource" "$ports" \
    >"$MCP_DEMO_RUNTIME/${name}.log" 2>&1 </dev/null &
  echo "$!" >"$MCP_DEMO_RUNTIME/${name}.pid"
}

wait_http() {
  local url=$1
  local attempts=${2:-60}
  for _ in $(seq 1 "$attempts"); do
    if curl -sS "$url" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done
  echo "timed out waiting for $url" >&2
  return 1
}
