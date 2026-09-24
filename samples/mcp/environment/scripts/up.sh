#!/usr/bin/env bash
set -euo pipefail
source "$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh"

for command_name in kind kubectl helm curl sed; do
  require_command "$command_name"
done
select_container_engine

mkdir -p "$MCP_DEMO_RUNTIME/plugins" "$MCP_DEMO_RUNTIME"

if cluster_exists; then
  if ! cluster_is_owned; then
    echo "refusing to reuse unowned Kind cluster $MCP_DEMO_CLUSTER" >&2
    echo "choose another MCP_DEMO_CLUSTER or remove the existing cluster yourself" >&2
    exit 1
  fi
else
  if cluster_exists_anywhere; then
    echo "refusing to create $MCP_DEMO_CLUSTER: an unowned cluster with that name exists" >&2
    exit 1
  fi
  if [[ -f "$MCP_DEMO_PROVIDER_FILE" || -f "$MCP_DEMO_CLUSTER_FILE" || -f "$MCP_DEMO_INSTANCE_FILE" ]]; then
    if local_ownership_matches; then
      clear_cluster_ownership
    else
      echo "runtime ownership state does not match cluster $MCP_DEMO_CLUSTER" >&2
      exit 1
    fi
  fi
  plugin_path=${MCP_DEMO_RUNTIME//&/\\&}/plugins
  sed "s|__PLUGIN_HOST_PATH__|$plugin_path|g" \
    "$MCP_DEMO_ROOT/environment/kind/cluster.yaml.tpl" \
    >"$MCP_DEMO_RUNTIME/kind-cluster.yaml"
  kind create cluster \
    --name "$MCP_DEMO_CLUSTER" \
    --image "$MCP_DEMO_KIND_NODE_IMAGE" \
    --config "$MCP_DEMO_RUNTIME/kind-cluster.yaml"
  if ! record_cluster_ownership; then
    kind delete cluster --name "$MCP_DEMO_CLUSTER" || true
    clear_cluster_ownership
    echo "failed to record MCP demo cluster ownership" >&2
    exit 1
  fi
fi

kubectl config use-context "kind-$MCP_DEMO_CLUSTER" >/dev/null

helm repo add higress.io https://higress.io/helm-charts --force-update >/dev/null
helm repo update higress.io >/dev/null
helm upgrade --install higress higress.io/higress \
  --version "$MCP_DEMO_HIGRESS_CHART_VERSION" \
  --namespace higress-system \
  --create-namespace \
  --values "$MCP_DEMO_ROOT/environment/higress/values.yaml" \
  --wait \
  --timeout 10m

backend_image=localhost/mcp-demo/observable-weather:1.0.0
"$MCP_DEMO_CONTAINER_ENGINE" build \
  -t "$backend_image" \
  "$MCP_DEMO_ROOT/environment/apps/observable-weather"
if [[ "$MCP_DEMO_CONTAINER_ENGINE" == podman ]]; then
  backend_archive="$MCP_DEMO_RUNTIME/observable-weather.tar"
  podman save --format docker-archive -o "$backend_archive" "$backend_image"
  kind load image-archive "$backend_archive" \
    --name "$MCP_DEMO_CLUSTER" \
    --nodes "${MCP_DEMO_CLUSTER}-control-plane"
  unlink "$backend_archive"
else
  kind load docker-image "$backend_image" --name "$MCP_DEMO_CLUSTER"
fi
kubectl apply -f "$MCP_DEMO_ROOT/environment/apps/observable-weather/deployment.yaml"
kubectl -n mcp-demo rollout status deployment/observable-weather --timeout=180s
kubectl -n higress-system rollout status deployment/higress-gateway --timeout=300s
kubectl -n higress-system rollout status deployment/higress-controller --timeout=300s

start_port_forward gateway higress-system service/higress-gateway "$MCP_DEMO_GATEWAY_PORT:80"
start_port_forward console higress-system service/higress-console "$MCP_DEMO_CONSOLE_PORT:8080"
start_port_forward backend mcp-demo service/observable-weather "$MCP_DEMO_BACKEND_PORT:8080"
wait_http "http://127.0.0.1:$MCP_DEMO_BACKEND_PORT/healthz"
wait_http "http://127.0.0.1:$MCP_DEMO_CONSOLE_PORT/"
wait_http "http://127.0.0.1:$MCP_DEMO_GATEWAY_PORT/"

printf '\nHigress MCP demo environment is ready:\n'
printf '  Kubernetes context: kind-%s\n' "$MCP_DEMO_CLUSTER"
printf '  Higress gateway:    http://127.0.0.1:%s\n' "$MCP_DEMO_GATEWAY_PORT"
printf '  Higress console:    http://127.0.0.1:%s\n' "$MCP_DEMO_CONSOLE_PORT"
printf '  Observable backend: http://127.0.0.1:%s\n' "$MCP_DEMO_BACKEND_PORT"
printf '\nNext: follow a demo README under the selected protocol version.\n'
