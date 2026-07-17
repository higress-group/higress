#!/usr/bin/env bash

set -euo pipefail

GATEWAY_CLASS="${GATEWAY_CLASS:-higress}"
SUPPORTED_FEATURES="${GATEWAY_CONFORMANCE_SUPPORTED_FEATURES:-Gateway,HTTPRoute,TLSRoute,GRPCRoute,TCPRoute,ReferenceGrant}"
CONFORMANCE_PROFILES="${GATEWAY_CONFORMANCE_PROFILE:-GATEWAY-HTTP,GATEWAY-TLS,GATEWAY-GRPC,GATEWAY-TCP}"
REPORT="${GATEWAY_CONFORMANCE_REPORT:-out/gateway-api-conformance/report.yaml}"
CONTACT="${GATEWAY_CONFORMANCE_CONTACT:-https://github.com/alibaba/higress/issues}"
VERSION="${HIGRESS_CONFORMANCE_VERSION:-$(git rev-parse HEAD)}"
ALLOW_CRDS_MISMATCH="${GATEWAY_CONFORMANCE_ALLOW_CRDS_MISMATCH:-false}"
CLEANUP_TEST_RESOURCES="${GATEWAY_CONFORMANCE_CLEANUP_TEST_RESOURCES:-true}"
RUN_TEST="${GATEWAY_CONFORMANCE_RUN_TEST:-}"
TEST_PARALLEL="${GATEWAY_CONFORMANCE_PARALLEL:-1}"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-higress}"
CONFORMANCE_IMAGE="${HIGRESS_CONFORMANCE_TEST_IMAGE:-higress-gateway-conformance:v1.6-local}"

: "${KUBECONFIG:?KUBECONFIG must point to the target Kubernetes cluster}"

mkdir -p "$(dirname "${REPORT}")"
REPORT_DIR="$(cd "$(dirname "${REPORT}")" && pwd)"
REPORT_NAME="$(basename "${REPORT}")"
KIND_NODE="${KIND_CLUSTER_NAME}-control-plane"
NODE_ARCH="$(docker exec "${KIND_NODE}" uname -m)"
case "${NODE_ARCH}" in
  aarch64|arm64)
    GOARCH=arm64
    ;;
  x86_64|amd64)
    GOARCH=amd64
    ;;
  *)
    echo "unsupported kind node architecture: ${NODE_ARCH}" >&2
    exit 1
    ;;
esac

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "${WORK_DIR}"' EXIT

CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH}" go -C test/gateway test -c -o "${WORK_DIR}/gateway-conformance.test" .
KIND_NODE_IMAGE="$(docker inspect "${KIND_NODE}" --format '{{.Config.Image}}')"
docker build -q \
  --build-arg "KIND_NODE_IMAGE=${KIND_NODE_IMAGE}" \
  -f test/gateway/Dockerfile.conformance \
  -t "${CONFORMANCE_IMAGE}" \
  "${WORK_DIR}" >/dev/null

cp "${KUBECONFIG}" "${WORK_DIR}/kubeconfig"
KUBE_CLUSTER="$(KUBECONFIG="${WORK_DIR}/kubeconfig" kubectl config view --minify -o jsonpath='{.contexts[0].context.cluster}')"
KUBECONFIG="${WORK_DIR}/kubeconfig" kubectl config set-cluster "${KUBE_CLUSTER}" --server=https://127.0.0.1:6443 >/dev/null
CLUSTER_DNS="$(kubectl --kubeconfig "${KUBECONFIG}" -n kube-system get service kube-dns -o jsonpath='{.spec.clusterIP}')"
printf 'nameserver %s\nsearch svc.cluster.local cluster.local\noptions ndots:5\n' "${CLUSTER_DNS}" >"${WORK_DIR}/resolv.conf"

ARGS=(
  -test.v
  -test.run '^TestGatewayAPIConformance$'
  "-test.parallel=${TEST_PARALLEL}"
  "--gateway-class=${GATEWAY_CLASS}"
  "--supported-features=${SUPPORTED_FEATURES}"
  "--conformance-profiles=${CONFORMANCE_PROFILES}"
  --organization=alibaba
  --project=higress
  --url=https://github.com/alibaba/higress
  "--version=${VERSION}"
  "--contact=${CONTACT}"
  --mode=default
  --cleanup-base-resources=false
  "--cleanup-test-resources=${CLEANUP_TEST_RESOURCES}"
  "--allow-crds-mismatch=${ALLOW_CRDS_MISMATCH}"
  "--report-output=/report/${REPORT_NAME}"
)
if [[ -n "${RUN_TEST}" ]]; then
  ARGS+=("--run-test=${RUN_TEST}")
fi

docker run --rm \
  --network "container:${KIND_NODE}" \
  -e KUBECONFIG=/kubeconfig \
  -e HIGRESS_GATEWAY_API_TEST_DIAL_LOCALHOST=true \
  -v "${WORK_DIR}/kubeconfig:/kubeconfig:ro" \
  -v "${WORK_DIR}/resolv.conf:/etc/resolv.conf:ro" \
  -v "${REPORT_DIR}:/report" \
  "${CONFORMANCE_IMAGE}" "${ARGS[@]}"
