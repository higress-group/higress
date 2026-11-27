<!--
 Copyright (c) 2022 Alibaba Group Holding Ltd.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
-->

# Higress E2E Tests

This directory contains the ingress conformance suite (`conformance`) and the
scale regression guard (`scale`).

## Ingress conformance

The conformance suite is wired into per-PR CI and is described by the test
definitions under `conformance/tests`. Run it with:

```shell
make higress-conformance-test
```

## Scale regression test

`test/e2e/scale` is a dedicated performance regression guard for incremental
Ingress-to-route propagation. It creates a backend and then builds two cluster
sizes in the same namespace:

- Phase A: `E2E_SCALE_BASE` Ingresses (default `200`) with hosts
  `scale-a-<i>.example.com`.
- Phase B: another `E2E_SCALE_BASE` Ingresses (default `400` total) with hosts
  `scale-b-<i>.example.com`.

Each phase first creates a warm-up Ingress and waits for it to return HTTP 200.
It then creates three measured Ingresses one at a time. For every measured
Ingress, the test records the time from the Kubernetes create call until an HTTP
request with that Ingress host returns 200 through the gateway.

### What it guards

Adding one Ingress should have a roughly constant incremental propagation cost.
If per-event work grows with the total number of host routes and merged
gateways, doubling the Ingress count can make one event roughly four times
slower. That quadratic-style behavior is what this guard is intended to catch;
it protects the host-route dependency and cache-index scope, not just raw
throughput.

### Thresholds

The test fails when either condition is violated:

- `median(phase B) / median(phase A) > E2E_SCALE_RATIO_MAX` (default `3.0`).
  A roughly 4x ratio is called out as a likely quadratic dependency regression.
- Any measured event exceeds `E2E_SCALE_EVENT_MAX_SECONDS` (default `60`).
  An absolute timeout is reported as the controller or gateway not keeping up.

On failure the test prints every raw sample, the configured Ingress counts, the
medians and ratio, and a short diagnosis hint. A ratio near 4x points at
per-event work growing with the total route/gateway set; an absolute timeout
points at controller or config-push capacity.

### Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `E2E_SCALE_ADDRESS` | required | Gateway address, for example `172.18.0.2:80` |
| `E2E_SCALE_INGRESS_CLASS` | `higress` | Ingress class used by generated Ingresses |
| `E2E_SCALE_BASE` | `200` | Ingresses per phase; phase B runs with twice this total |
| `E2E_SCALE_RATIO_MAX` | `3.0` | Maximum allowed median B/A ratio |
| `E2E_SCALE_EVENT_MAX_SECONDS` | `60` | Maximum allowed latency for one measured event |
| `E2E_SCALE_NAMESPACE` | `scale-test` | Namespace created and deleted by the test |

### Running in CI

The dedicated `E2E Scale Regression` workflow runs weekly and can be started
manually from the Actions tab or with:

```shell
gh workflow run e2e-scale.yaml --repo higress-group/higress
```

It is intentionally not part of per-PR CI. The workflow frees runner disk space,
runs `make prebuild`, and then runs:

```shell
make higress-scale-test
```

That target creates a kind cluster, builds and loads the images, installs
Higress, waits for the controller and gateway, resolves the gateway address
(preferring the LoadBalancer service status and falling back to the kind node
address used by the gateway host ports), runs the tagged test with a 45 minute
timeout, and deletes the cluster on success. Controller and gateway logs are
uploaded when the workflow fails.

To run the tagged test directly against an existing cluster:

```shell
E2E_SCALE_ADDRESS=<gateway-address> \
  go test -v -tags scale ./test/e2e/scale -timeout 45m
```

### Resource warning

Do not run this test on a small development machine. It needs Docker, a kind
cluster, a Higress control plane and gateway, and several hundred Ingresses. Use
the CI workflow or a dedicated machine with CI-sized CPU, memory, and disk.
