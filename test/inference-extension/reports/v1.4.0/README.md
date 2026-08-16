# Higress v2.2.4 Inference Extension conformance

This directory records the Gateway API Inference Extension v1.4.0 Gateway
profile conformance result for the Higress v2.2.4 release.

## Result

- Profile: `Gateway`
- Core tests: 12 passed, 0 failed, 0 skipped
- Report SHA-256:
  `79406981458f4989c8270126bd9b2a526b8e9d63e0c086ea2f5e6be9086216ca`

## Tested artifacts

- Higress source: tag `v2.2.4`, commit
  `58666ac985cee19a0a9a353421c63cead6d0cb47`
- Higress controller, Pilot, and Gateway: the same v2.2.4 images and digests
  recorded in `test/gateway/reports/v1.6.0/README.md`
- Kubernetes: kind v1.34.0
- Gateway API standard CRDs: v1.5.0
- Gateway API Inference Extension CRD and conformance suite: v1.4.0
- MetalLB: v0.13.7

## Reproduce

Create a fresh kind cluster and install:

1. MetalLB v0.13.7.
2. Gateway API v1.5.0 standard CRDs.
3. The InferencePool CRD from Gateway API Inference Extension v1.4.0.
4. The Istio CRDs pinned by the Higress v2.2.4 source tree.
5. `helm/core` from the v2.2.4 source tag with these values:

```text
controller.tag=v2.2.4
pilot.tag=v2.2.4
gateway.tag=v2.2.4
gateway.replicas=1
gateway.service.type=LoadBalancer
global.local=true
global.enableInferenceExtension=true
```

Apply `test/inference-extension/manifests/epp-tls.yaml`, then run:

```shell
INFERENCE_EXTENSION_VERSION=v1.4.0 \
INFERENCE_EXTENSION_SOURCE_DIR="$PWD/out/gateway-api-inference-extension-source/v1.4.0" \
INFERENCE_EXTENSION_REPORT="$PWD/out/gateway-api-inference-extension-v1.4.0-report.yaml" \
INFERENCE_EXTENSION_EXPECTED_PASSED=12 \
INFERENCE_EXTENSION_CONTACT=@higress-group/maintainers \
HIGRESS_CONFORMANCE_VERSION=v2.2.4 \
tools/hack/run-inference-extension-conformance.sh
```

The restricted validation host could not reach `registry.k8s.io`. The official
v1.4.0 EPP image was therefore preloaded into kind, and the three EPP
Deployments in the extracted upstream `conformance/resources/base.yaml` used
`imagePullPolicy: IfNotPresent` instead of `Always`. Image names, image versions,
functional resources, conformance test code, and assertions were unchanged.
