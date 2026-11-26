#!/usr/bin/env bash
# Run only after the shared remote-container-slot is released and claimed.
set -euo pipefail
cd "$(dirname "$0")"
if [[ "$PWD" != /Users/wujieai/ope9626-validation/higress/* ]] || [[ "${OPE9626_RUNTIME_SLOT:-}" != OPE-11003 ]]; then
  echo "Run only in the assigned remote packaging-host directory with OPE9626_RUNTIME_SLOT=OPE-11003" >&2
  exit 2
fi
docker_cli=(/opt/homebrew/bin/docker --context colima-ope10117)
gateway_image="${OPE9626_GATEWAY_IMAGE:-higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:2.1.9}"
mock_image="${OPE9626_MOCK_IMAGE:-python:3.12-alpine}"
gateway=ope9626-higress-gateway
mock=ope9626-higress-mock
network=ope9626-higress-net
prometheus=ope9626-higress-prometheus
with_prometheus="${OPE9626_PROMETHEUS:-0}"
gateway_cpu=1.75
gateway_memory=1792m
if [[ "$with_prometheus" == 1 ]]; then gateway_cpu=1.5; gateway_memory=1536m; fi
# Refuse name collisions; cleanup applies only to resources created by this run.
for name in "$gateway" "$mock" "$prometheus"; do
  if "${docker_cli[@]}" container inspect "$name" >/dev/null 2>&1; then
    echo "Existing $name must be reviewed by its owner before running" >&2
    exit 2
  fi
done
"${docker_cli[@]}" network create "$network" > network-id.txt
cleanup() {
  "${docker_cli[@]}" stats --no-stream --format '{{json .}}' "$gateway" "$mock" "$prometheus" > resource-sample.jsonl 2>/dev/null || true
  "${docker_cli[@]}" logs "$gateway" > gateway.log 2>&1 || true
  "${docker_cli[@]}" logs "$mock" > mock.log 2>&1 || true
  "${docker_cli[@]}" logs "$prometheus" > prometheus.log 2>&1 || true
  "${docker_cli[@]}" inspect "$gateway" "$mock" "$prometheus" > containers.json 2>/dev/null || true
  "${docker_cli[@]}" rm -f "$gateway" "$mock" "$prometheus" >/dev/null 2>&1 || true
  "${docker_cli[@]}" network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT
python3 configure.py
"${docker_cli[@]}" run -d --name "$mock" --network "$network" --cpus=0.25 --memory=128m \
  -v "$PWD:/validation:ro" "$mock_image" python /validation/mock.py > mock-id.txt
"${docker_cli[@]}" run -d --name "$gateway" --network "$network" --cpus="$gateway_cpu" --memory="$gateway_memory" \
  -p 127.0.0.1::8080 -p 127.0.0.1::9901 -v "$PWD:/validation:ro" \
  --entrypoint /usr/local/bin/envoy "$gateway_image" -c /validation/envoy.json --concurrency 1 --log-level warning > gateway-id.txt
"${docker_cli[@]}" inspect "$gateway" > gateway-inspect.json
if [[ "$with_prometheus" == 1 ]]; then
  "${docker_cli[@]}" run -d --name "$prometheus" --network "$network" --cpus=0.25 --memory=256m \
    -p 127.0.0.1::9090 -v "$PWD:/validation:ro" \
    "${OPE9626_PROMETHEUS_IMAGE:-prom/prometheus:v2.55.1}" --config.file=/validation/prometheus.json \
    --storage.tsdb.retention.time=1h --storage.tsdb.path=/tmp/prometheus > prometheus-id.txt
  "${docker_cli[@]}" inspect "$prometheus" > prometheus-inspect.json
fi
python3 exercise.py gateway-inspect.json
if [[ "$with_prometheus" == 1 ]]; then python3 verify-prometheus.py; fi
