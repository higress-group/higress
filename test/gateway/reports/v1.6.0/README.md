# Higress v2.2.4 Gateway API conformance

This directory records the Gateway API v1.6.0 standard-channel conformance
result for the Higress v2.2.4 release.

## Result

- Profile: `GATEWAY-HTTP`
- Core tests: 37 passed, 0 failed, 0 skipped
- Report SHA-256:
  `6d342dd57ed470ea6780643cbe01c94a2e685f5cfb63199504dd50d7f4a11755`

## Tested artifacts

- Higress source: tag `v2.2.4`, commit
  `58666ac985cee19a0a9a353421c63cead6d0cb47`
- Controller image:
  `higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/higress:v2.2.4`
  (`sha256:0a5b7809a107cbec150f41352a156e31160b8a92df787798e2a9a06cdd5587da`)
- Pilot image:
  `higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/pilot:v2.2.4`
  (`sha256:f742ed20f938c5c1eaf6f8c36c6481a87052d06e903ab6cb0c079165ac0c8284`)
- Gateway image:
  `higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:v2.2.4`
  (`sha256:3dbd609df5db3fca61653eafe0e2310705e485190c4f8cd02d9aab8f07dcf329`)
- Kubernetes: kind v1.34.0
- Gateway API CRDs and conformance suite: v1.6.0

## Reproduce

Create a fresh kind cluster, install the Gateway API v1.6.0 standard CRDs, and
install `helm/core` from the v2.2.4 source tag with these values:

```text
controller.tag=v2.2.4
pilot.tag=v2.2.4
gateway.tag=v2.2.4
gateway.replicas=1
gateway.service.type=ClusterIP
global.local=true
global.enableGatewayAPIDeploymentController=true
```

Ensure the three release images above are available to the kind node, then run:

```shell
GATEWAY_CLASS=higress \
GATEWAY_API_VERSION=v1.6.0 \
GATEWAY_CONFORMANCE_TEST_DIR=test/gateway/v1.6 \
GATEWAY_CONFORMANCE_SUPPORTED_FEATURES=Gateway,HTTPRoute,ReferenceGrant \
GATEWAY_CONFORMANCE_PROFILE=GATEWAY-HTTP \
GATEWAY_CONFORMANCE_REPORT=out/gateway-api-v1.6.0-report.yaml \
GATEWAY_CONFORMANCE_CONTACT=https://github.com/higress-group/higress/issues \
HIGRESS_CONFORMANCE_VERSION=v2.2.4 \
GATEWAY_CONFORMANCE_ALLOW_CRDS_MISMATCH=false \
GATEWAY_CONFORMANCE_SUPPORTS_TEST_CLEANUP=true \
GATEWAY_CONFORMANCE_CLEANUP_TEST_RESOURCES=true \
GATEWAY_CONFORMANCE_PARALLEL=1 \
KIND_CLUSTER_NAME=higress \
tools/hack/run-gateway-api-conformance.sh
```

On the restricted validation host, the official conformance backend images
were preloaded into kind because the host could not reach `registry.k8s.io`.
No Gateway API manifests, test code, or assertions were changed.
