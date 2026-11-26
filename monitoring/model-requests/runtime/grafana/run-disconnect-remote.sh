#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
[[ "$PWD" == /Users/wujieai/ope9626-validation/higress/*/runtime/grafana ]] && [[ "${OPE9626_RUNTIME_SLOT:-}" == OPE-11003 ]] || exit 2
docker_cli=(/opt/homebrew/bin/docker --context colima-ope10117)
for name in ope9626-higress-gateway ope9626-higress-mock; do
  if "${docker_cli[@]}" container inspect "$name" >/dev/null 2>&1; then echo "Name collision: $name" >&2; exit 2; fi
done
"${docker_cli[@]}" network create ope9626-higress-net > disconnect-network-id.txt
cleanup() {
  "${docker_cli[@]}" stats --no-stream --format '{{json .}}' ope9626-higress-gateway ope9626-higress-mock > disconnect-resources.jsonl 2>/dev/null || true
  "${docker_cli[@]}" inspect ope9626-higress-gateway ope9626-higress-mock > disconnect-containers.json 2>/dev/null || true
  "${docker_cli[@]}" logs ope9626-higress-gateway > disconnect-gateway.log 2>&1 || true
  "${docker_cli[@]}" rm -fv ope9626-higress-gateway ope9626-higress-mock >/dev/null 2>&1 || true
  "${docker_cli[@]}" network rm ope9626-higress-net >/dev/null 2>&1 || true
}
trap cleanup EXIT
trap 'exit 130' INT TERM
python3 ../configure.py
"${docker_cli[@]}" run -d --name ope9626-higress-mock --network ope9626-higress-net --cpus=.25 --memory=128m -v "$(cd .. && pwd):/validation:ro" c3fbb6bad8024d7585094912dc1a4273.mirror.swr.myhuaweicloud.com/library/python:3.12-alpine python /validation/mock.py > disconnect-mock-id.txt
"${docker_cli[@]}" run -d --name ope9626-higress-gateway --network ope9626-higress-net --cpus=1.75 --memory=1792m -p 127.0.0.1::8080 -p 127.0.0.1::9901 -v "$(cd .. && pwd):/validation:ro" --entrypoint /usr/local/bin/envoy higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:2.1.9 -c /validation/envoy.json --concurrency 1 --log-level warning > disconnect-gateway-id.txt
"${docker_cli[@]}" inspect ope9626-higress-gateway > ../gateway-inspect.json
python3 disconnect.py
