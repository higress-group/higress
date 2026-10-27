#!/usr/bin/env bash
set -euo pipefail
unset GH_TOKEN GITHUB_TOKEN
CASE_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source "$CASE_DIR/../../../environment/scripts/common.sh"
source "$CASE_DIR/../plugin/source.env"
for name in kubectl go curl python3 cmp; do require_command "$name"; done
select_container_engine
if ! cluster_is_owned || [[ "$(kubectl config current-context)" != "kind-$MCP_DEMO_CLUSTER" ]]; then
  echo "run the public up.sh for this owned MCP_DEMO_CLUSTER and KUBECONFIG first" >&2
  exit 2
fi
: "${MCP_DEMO_EVIDENCE:?set an external evidence directory}"
mkdir -p "$MCP_DEMO_EVIDENCE"
MCP_DEMO_EVIDENCE=$(CDPATH= cd -- "$MCP_DEMO_EVIDENCE" && pwd)
export MCP_DEMO_EVIDENCE
repo=$(git -C "$CASE_DIR" rev-parse --show-toplevel)
case "$MCP_DEMO_EVIDENCE/" in "$repo/"*) echo "evidence must be outside the checkout" >&2; exit 2 ;; esac
exec > >(tee "$MCP_DEMO_EVIDENCE/verify.log") 2>&1
upstream_port=${MCP_DEMO_INSTRUCTIONS_PORT:-18083}
admin="http://127.0.0.1:$upstream_port"
gateway="http://127.0.0.1:$MCP_DEMO_GATEWAY_PORT"
restore_file=""
restore_needed=0
run_id=$(python3 -c 'import uuid; print(uuid.uuid4().hex)')
printf '%s\n' "$run_id" >"$MCP_DEMO_EVIDENCE/run-id.txt"

config_manifest() {
  kubectl -n mcp-demo create configmap mcp-instructions-config \
    --from-file="instructions.txt=$1" --dry-run=client -o json |
    python3 -c 'import json,sys; v=json.load(sys.stdin); v["metadata"]["labels"]={"mcp.higress.io/demo":"instructions"}; print(json.dumps(v,ensure_ascii=False,indent=2))' >"$2"
}

forward_upstream() {
  start_port_forward instructions-upstream mcp-demo service/instructions-mcp "$upstream_port:8080"
  wait_http "$admin/healthz"
}


wait_route() {
  local path=$1
  for _ in $(seq 1 60); do
    # GET is an admission probe only: the loaded MCP filter returns405 before
    # any SDK method. The unrewritten upstream path would instead return404.
    if [[ "$(curl -sS -o /dev/null -w '%{http_code}' "$gateway$path" || true)" == 405 ]]; then return; fi
    sleep 1
  done
  echo "MCP route did not become ready: $path" >&2
  return 1
}


wait_gateway_access() {
  : >"$MCP_DEMO_EVIDENCE/gateway-poll.log"
  for attempt in $(seq 1 60); do
    printf 'attempt=%s\n' "$attempt" >>"$MCP_DEMO_EVIDENCE/gateway-poll.log"
    if kubectl --request-timeout=5s -n higress-system logs deployment/higress-gateway --since=15m >"$MCP_DEMO_EVIDENCE/gateway.log" 2>>"$MCP_DEMO_EVIDENCE/gateway-poll.log"; then
      code=0
      python3 "$CASE_DIR/verify_gateway.py" --evidence "$MCP_DEMO_EVIDENCE" --run-id "$run_id" >>"$MCP_DEMO_EVIDENCE/gateway-poll.log" 2>&1 || code=$?
      if [[ "$code" == 0 ]]; then return 0; fi
      if [[ "$code" != 1 ]]; then return 1; fi
    fi
    sleep 1
  done
  echo "current-run gateway access records did not flush within 60 attempts" >&2
  return 1
}

cleanup() {
  status=$?
  trap - EXIT INT TERM
  set +e
  curl -fsS "$admin/__state" >"$MCP_DEMO_EVIDENCE/final-upstream-state.json"
  kubectl -n mcp-demo logs deployment/instructions-mcp >"$MCP_DEMO_EVIDENCE/upstream.log" 2>&1
  if [[ "$status" == 0 ]]; then
    wait_gateway_access || status=1
  else
    kubectl -n higress-system logs deployment/higress-gateway --since=15m >"$MCP_DEMO_EVIDENCE/gateway.log" 2>&1
  fi
  if [[ "$restore_needed" == 1 ]]; then
    if kubectl apply -f "$restore_file" >"$MCP_DEMO_EVIDENCE/restore.log" 2>&1 &&
       kubectl -n mcp-demo rollout restart deployment/instructions-mcp >>"$MCP_DEMO_EVIDENCE/restore.log" 2>&1 &&
       kubectl -n mcp-demo rollout status deployment/instructions-mcp --timeout=180s >>"$MCP_DEMO_EVIDENCE/restore.log" 2>&1; then
      python3 - "$restore_file" <<'PY' >"$MCP_DEMO_EVIDENCE/restored-instructions.txt"
import json,sys
sys.stdout.write(json.load(open(sys.argv[1]))["data"]["instructions.txt"])
PY
      kubectl -n mcp-demo exec deployment/instructions-mcp -- cat /config/instructions.txt >"$MCP_DEMO_EVIDENCE/restored-pod-instructions.txt"
      cmp "$MCP_DEMO_EVIDENCE/restored-instructions.txt" "$MCP_DEMO_EVIDENCE/restored-pod-instructions.txt" || status=1
    else
      status=1
    fi
  fi
  stop_port_forward instructions-upstream
  printf 'exit=%s; case port-forward stopped; see restore.log; case resources retained for manual repeat\n' "$status" >"$MCP_DEMO_EVIDENCE/completion.txt"
  python3 - "$MCP_DEMO_EVIDENCE" "$status" <<'PY_SUMMARY'
import hashlib,json,sys
from pathlib import Path
root=Path(sys.argv[1])
phases={}
for phase in ("direct","on","off","changed"):
    path=root/(phase+".json")
    if path.exists() and path.stat().st_size:
        phases[phase]=json.loads(path.read_text())
(root/"summary.json").write_text(json.dumps({"exitCode":int(sys.argv[2]),"phases":phases},ensure_ascii=False,indent=2)+"\n")
(root/"SHA256SUMS").write_text("".join(hashlib.sha256(p.read_bytes()).hexdigest()+"  "+p.name+"\n" for p in sorted(root.iterdir()) if p.is_file() and p.name not in ("SHA256SUMS","verify.log")))
PY_SUMMARY
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

git -C "$repo" rev-parse HEAD >"$MCP_DEMO_EVIDENCE/source-commit.txt"
git -C "$repo" status --porcelain >"$MCP_DEMO_EVIDENCE/source-status.txt"
go version >"$MCP_DEMO_EVIDENCE/go-version.txt"
printf 'MCP_DEMO_CLUSTER=%s MCP_DEMO_EVIDENCE=%s ./verify.sh\n' "$MCP_DEMO_CLUSTER" "$MCP_DEMO_EVIDENCE" >"$MCP_DEMO_EVIDENCE/command.txt"
plugin="$MCP_DEMO_RUNTIME/plugins/mcp-server/2026-07-28"
if [[ "$(<"$plugin/source-commit.txt")" != "$MCP_DEMO_HIGRESS_REF" ]]; then
  echo "plugin source differs from plugin/source.env; rebuild it before starting the environment" >&2
  exit 2
fi
cp "$plugin/source-commit.txt" "$MCP_DEMO_EVIDENCE/plugin-source-commit.txt"
cp "$plugin/SHA256SUMS" "$MCP_DEMO_EVIDENCE/plugin-SHA256SUMS"
(cd "$CASE_DIR" && go build -trimpath -o "$MCP_DEMO_EVIDENCE/client" ./cmd/client)
go version -m "$MCP_DEMO_EVIDENCE/client" >"$MCP_DEMO_EVIDENCE/client-build-info.txt"
"$CASE_DIR/fixture/build.sh"
config_manifest "$CASE_DIR/fixture/instructions.txt" "$MCP_DEMO_EVIDENCE/config-initial.json"
restore_file="$MCP_DEMO_EVIDENCE/config-initial.json"
if kubectl -n mcp-demo get configmap mcp-instructions-config -o json >"$MCP_DEMO_EVIDENCE/config-before.json" 2>/dev/null; then
  python3 - "$MCP_DEMO_EVIDENCE/config-before.json" "$MCP_DEMO_EVIDENCE/config-restore.json" <<'PY'
import json,sys
v=json.load(open(sys.argv[1]))
if v["metadata"].get("labels",{}).get("mcp.higress.io/demo") != "instructions":
    raise SystemExit("refusing to replace a ConfigMap not owned by this case")
v["metadata"]={k:v["metadata"][k] for k in ("name","namespace","labels")}
json.dump(v,open(sys.argv[2],"w"),ensure_ascii=False,indent=2)
PY
  restore_file="$MCP_DEMO_EVIDENCE/config-restore.json"
fi
kubectl apply -f "$MCP_DEMO_EVIDENCE/config-initial.json"
restore_needed=1
kubectl apply -f "$CASE_DIR/fixture/deployment.yaml"
kubectl -n mcp-demo rollout restart deployment/instructions-mcp
kubectl -n mcp-demo rollout status deployment/instructions-mcp --timeout=180s
kubectl apply -f "$CASE_DIR/resources.yaml"
forward_upstream
wait_route /instructions/on
wait_route /instructions/off
client="$MCP_DEMO_EVIDENCE/client"
"$client" --run-id "$run_id" --phase direct --endpoint "$admin/mcp" --admin-url "$admin" \
  --instructions-file "$CASE_DIR/fixture/instructions.txt" --server-name native-instructions-upstream >"$MCP_DEMO_EVIDENCE/direct.json"
"$client" --run-id "$run_id" --phase on --endpoint "$gateway/instructions/on" --admin-url "$admin" \
  --instructions-file "$CASE_DIR/fixture/instructions.txt" >"$MCP_DEMO_EVIDENCE/on.json"
"$client" --run-id "$run_id" --phase off --endpoint "$gateway/instructions/off" --admin-url "$admin" \
  --instructions-file "$CASE_DIR/fixture/instructions.txt" --instructions-absent \
  --expected-methods tools/list,tools/call >"$MCP_DEMO_EVIDENCE/off.json"
config_manifest "$CASE_DIR/fixture/instructions-changed.txt" "$MCP_DEMO_EVIDENCE/config-changed.json"
kubectl apply -f "$MCP_DEMO_EVIDENCE/config-changed.json"
kubectl -n mcp-demo rollout restart deployment/instructions-mcp
kubectl -n mcp-demo rollout status deployment/instructions-mcp --timeout=180s
forward_upstream
"$client" --run-id "$run_id" --phase changed --endpoint "$gateway/instructions/on" --admin-url "$admin" \
  --instructions-file "$CASE_DIR/fixture/instructions-changed.txt" >"$MCP_DEMO_EVIDENCE/changed.json"
kubectl -n mcp-demo get ingress mcp-2026-instructions-on mcp-2026-instructions-off -o yaml >"$MCP_DEMO_EVIDENCE/ingress.yaml"
kubectl -n higress-system get wasmplugin mcp-2026-instructions -o yaml >"$MCP_DEMO_EVIDENCE/wasmplugin.yaml"
printf 'PASS direct S / gateway on S / gateway off absent and no upstream discover / fresh client S-prime\n'
