#!/usr/bin/env bash
# Exclusive shared-slot test. Native Chrome is launched once by browser.mjs.
set -euo pipefail
cd "$(dirname "$0")"
[[ "$PWD" == /Users/wujieai/ope9626-validation/higress/*/runtime/grafana ]] && [[ "${OPE9626_RUNTIME_SLOT:-}" == OPE-11003 ]] || exit 2
python3 - <<'PY'
import shutil
assert shutil.disk_usage('.').free >= 20 * 1024**3, 'Below 20GiB free; stop adding load'
PY
docker_cli=(/opt/homebrew/bin/docker --context colima-ope10117)
mock_image=c3fbb6bad8024d7585094912dc1a4273.mirror.swr.myhuaweicloud.com/library/python:3.12-alpine
prom_image=c3fbb6bad8024d7585094912dc1a4273.mirror.swr.myhuaweicloud.com/prom/prometheus:v2.55.1
grafana_image="${OPE9626_GRAFANA_IMAGE:-c3fbb6bad8024d7585094912dc1a4273.mirror.swr.myhuaweicloud.com/grafana/grafana:11.3.0}"
names=(ope9626-higress-gateway ope9626-higress-mock ope9626-higress-prometheus ope9626-higress-grafana ope9626-higress-backfill)
network=ope9626-higress-net
for name in "${names[@]}"; do
  if "${docker_cli[@]}" container inspect "$name" >/dev/null 2>&1; then echo "Name collision: $name" >&2; exit 2; fi
done
"${docker_cli[@]}" network create "$network" > network-id.txt
cleanup() {
  "${docker_cli[@]}" stats --no-stream --format '{{json .}}' "${names[@]:0:4}" > resource-final.jsonl 2>/dev/null || true
  for name in "${names[@]}"; do "${docker_cli[@]}" logs "$name" > "$name.log" 2>&1 || true; done
  "${docker_cli[@]}" inspect "${names[@]:0:4}" > containers.json 2>/dev/null || true
  "${docker_cli[@]}" rm -fv "${names[@]}" >/dev/null 2>&1 || true
  "${docker_cli[@]}" network rm "$network" >/dev/null 2>&1 || true
  if [[ -f monitor.pid ]]; then kill "$(cat monitor.pid)" 2>/dev/null || true; fi
}
trap cleanup EXIT
trap 'exit 130' INT TERM
python3 resources.py "$$" &
echo "$!" > monitor.pid
python3 prepare.py
mkdir -p prometheus-data
chmod 777 prometheus-data
# Backfill finishes before starting the four services, within the same budget.
"${docker_cli[@]}" run --name ope9626-higress-backfill --cpus=.35 --memory=384m \
  -v "$PWD:/fixture:ro" -v "$PWD/prometheus-data:/data" --entrypoint /bin/promtool "$prom_image" \
  tsdb create-blocks-from openmetrics /fixture/history.openmetrics /data > backfill.log
python3 ../configure.py
python3 - <<'PY'
from pathlib import Path
import secrets
Path('password').write_text(secrets.token_urlsafe(18)+'\n')
Path('password').chmod(0o644)  # task-only random password readable by Grafana's container UID
PY
if [[ "${OPE9626_RENDER_ONLY:-0}" != 1 ]]; then
"${docker_cli[@]}" run -d --name ope9626-higress-mock --network "$network" --cpus=.15 --memory=128m -v "$(cd .. && pwd):/validation:ro" "$mock_image" python /validation/mock.py > mock-id.txt
"${docker_cli[@]}" run -d --name ope9626-higress-gateway --network "$network" --cpus=1 --memory=1024m -p 127.0.0.1::8080 -p 127.0.0.1::9901 -v "$(cd .. && pwd):/validation:ro" --entrypoint /usr/local/bin/envoy higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:2.1.9 -c /validation/envoy.json --concurrency 1 --log-level warning > gateway-id.txt
"${docker_cli[@]}" inspect ope9626-higress-gateway > ../gateway-inspect.json
fi
"${docker_cli[@]}" run -d --name ope9626-higress-prometheus --network "$network" --cpus=.35 --memory=384m -p 127.0.0.1::9090 -v "$(cd .. && pwd):/validation:ro" -v "$PWD/prometheus-data:/prometheus" "$prom_image" --config.file=/validation/prometheus.json --storage.tsdb.retention.time=2d --storage.tsdb.path=/prometheus > prometheus-id.txt
"${docker_cli[@]}" inspect ope9626-higress-prometheus > ../prometheus-inspect.json
"${docker_cli[@]}" run -d --name ope9626-higress-grafana --network "$network" --cpus=.5 --memory=512m -p 127.0.0.1::3000 -v "$PWD/password:/run/fixture-password:ro" -e GF_SECURITY_ADMIN_USER=fixture-admin -e GF_SECURITY_ADMIN_PASSWORD__FILE=/run/fixture-password -e GF_USERS_ALLOW_SIGN_UP=false -e GF_AUTH_ANONYMOUS_ENABLED=false -e GF_ANALYTICS_REPORTING_ENABLED=false -e GF_ANALYTICS_CHECK_FOR_UPDATES=false -e GF_PLUGINS_PREINSTALL_DISABLED=true "$grafana_image" > grafana-id.txt
"${docker_cli[@]}" inspect ope9626-higress-grafana > grafana-inspect.json
# Live Wasm/filter/export/query checks stay separate from historical series.
if [[ "${OPE9626_RENDER_ONLY:-0}" != 1 ]]; then
  python3 live.py > live-results.log
  python3 ../verify-prometheus.py > live-prometheus.log
fi
python3 import.py
/Users/wujieai/.nvm/versions/node/v22.23.1/bin/node --max-old-space-size=128 browser.mjs

if [[ "${OPE9626_RENDER_ONLY:-0}" != 1 ]]; then python3 disconnect.py; fi
