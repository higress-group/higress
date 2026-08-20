---
title: AI Load Balance
keywords: [higress, llm, load balance]
description: LLM-oriented load balance policies
---

# Introduction

**Attention**: 
- Version of Higress should >= v2.1.5

This plug-in provides the llm-oriented load balancing capability in a hot-swappable manner. If the plugin is closed, the load balancing strategy will degenerate into the load balancing strategy of the service itself (round robin, local minimum request number, random, consistent hash, etc.).

The configuration is:

| Name                | Type         | Required          | default       | description                                 |
|--------------------|-----------------|------------------|-------------|-------------------------------------|
| `lb_type`        | string          | optional              | endpoint    | load balance policy type, `endpoint` or `cluster` |
| `lb_policy`      | string          | required              |             | load balance policy type    |
| `lb_config`      | object          | required              |             | configuration for the current load balance type    |

When `lb_type = endpoint`, current supported load balance policies are:

- `global_least_request`: global least request based on redis
- `prefix_cache`: Select the backend node based on the prompt prefix match. If the node cannot be matched by prefix matching, the service node is selected based on the global minimum number of requests.
- `endpoint_metrics`: Load balancing based on metrics exposed by the llm service

When `lb_type = cluster`, current supported load balance policies are:
- `cluster_metrics`: Load balancing based on metrics of clusters
- `cluster_hash`: Consistent hash routing based on a request header value, always routing the same hash key to the same cluster, with weighted traffic distribution

Both policies only write the selected cluster to an internal request header. The user must also [configure an EnvoyFilter explicitly](#configure-envoyfilter-explicitly-for-cluster-mode) to make the target route use `route.cluster_header`; the Higress control plane does not create, update, or delete this EnvoyFilter automatically.


# Global Least Request
## Introduction

```mermaid
sequenceDiagram
	participant C as Client
	participant H as Higress
	participant R as Redis
	participant H1 as Host1
	participant H2 as Host2

	C ->> H: Send request
	H ->> R: Get host ongoing request number
	R ->> H: Return result
	H ->> R: According to the result, select the host with the smallest number of current requests, host rq count +1.
	R ->> H: Return result
	H ->> H1: Bypass the service's original load balancing strategy and forward the request to the corresponding host
	H1 ->> H: Return result
	H ->> R: host rq count -1
	H ->> C: Receive response
```

## Configuration

| Name                | Type         | required          | default       | description                                 |
|--------------------|-----------------|------------------|-------------|-------------------------------------|
| `serviceFQDN`      | string          | required              |             | redis FQDN, e.g.  `redis.dns`    |
| `servicePort`      | int             | required              |             | redis port                      |
| `username`         | string          | required              |             | redis username                         |
| `password`         | string          | optional              | ``          | redis password                           |
| `timeout`          | int             | optional              | 3000ms      | redis request timeout                    |
| `database`         | int             | optional              | 0           | redis database number                      |

## Configuration Example

```yaml
lb_type: endpoint
lb_policy: global_least_request
lb_config:
  serviceFQDN: redis.static
  servicePort: 6379
  username: default
  password: '123456'
```

# Prefix Cache
## Introduction
Select pods based on the prompt prefix match to reuse KV Cache. If no node can be matched by prefix match, select the service node based on the global minimum number of requests.

For example, the following request is routed to pod 1:

```json
{
  "model": "qwen-turbo",
  "messages": [
    {
      "role": "user",
      "content": "hi"
    }
  ]
}
```

Then subsequent requests with the same prefix will also be routed to pod 1:

```json
{
  "model": "qwen-turbo",
  "messages": [
    {
      "role": "user",
      "content": "hi"
    },
    {
      "role": "assistant",
      "content": "Hi! How can I assist you today? 😊"
    },
    {
      "role": "user",
      "content": "write a short story aboud 100 words"
    }
  ]
}
```

## Configuration

| Name               | Type            | required              | default     | description                     |
|--------------------|-----------------|-----------------------|-------------|---------------------------------|
| `serviceFQDN`      | string          | required              |             | redis FQDN, e.g.  `redis.dns`   |
| `servicePort`      | int             | required              |             | redis port                      |
| `username`         | string          | required              |             | redis username                  |
| `password`         | string          | optional              | ``          | redis password                  |
| `timeout`          | int             | optional              | 3000ms      | redis request timeout           |
| `database`         | int             | optional              | 0           | redis database number           |
| `redisKeyTTL`      | int             | optional              | 1800s      | prompt prefix key's ttl         |

## Configuration Example

```yaml
lb_type: endpoint
lb_policy: prefix_cache
lb_config:
  serviceFQDN: redis.static
  servicePort: 6379
  username: default
  password: '123456'
```

# Least Busy
## Introduction

wasm implementation for [gateway-api-inference-extension](https://github.com/kubernetes-sigs/gateway-api-inference-extension/blob/main/README.md)

```mermaid
sequenceDiagram
	participant C as Client
	participant H as Higress
	participant H1 as Host1
	participant H2 as Host2

	loop fetch metrics periodically
		H ->> H1: /metrics
		H1 ->> H: vllm metrics
		H ->> H2: /metrics
		H2 ->> H: vllm metrics
	end

	C ->> H: request
	H ->> H1: select pod according to vllm metrics, bypassing original service load balance policy
	H1 ->> H: response
	H ->> C: response
```

<!-- flowchart for pod selection:

![](https://github.com/kubernetes-sigs/gateway-api-inference-extension/blob/main/docs/scheduler-flowchart.png) -->

## Configuration

| Name                | Type         | Required          | default       | description                                 |
|--------------------|-----------------|------------------|-------------|-------------------------------------|
| `metric_policy`      | string | required | | How to use the metrics exposed by LLM for load balancing, currently supporting `[default, least, most]` |
| `target_metric`      | string | optional | | The metric name to use. This is valid only when `metric_policy` is `least` or `most` |
| `rate_limit`      | string | optional | 1 | The maximum percentage of requests a single node can receive, 0~1 |

## Configuration Example

Use the algorithm of [gateway-api-inference-extension](https://github.com/kubernetes-sigs/gateway-api-inference-extension/blob/main/README.md):

```yaml
lb_type: endpoint
lb_policy: metrics_based
lb_config:
  metric_policy: default
  rate_limit: 0.6
```

Load balancing based on the current number of queued requests: 

```yaml
lb_type: endpoint
lb_policy: metrics_based
lb_config:
  metric_policy: least
  target_metric: vllm:num_requests_waiting
  rate_limit: 0.6
```

Load balancing based on the number of requests currently being processed by the GPU:

```yaml
lb_type: endpoint
lb_policy: metrics_based
lb_config:
  metric_policy: least
  target_metric: vllm:num_requests_running
  rate_limit: 0.6
```

# Cross-service load balancing

## Configuration

| 名称                | 数据类型         | 填写要求          | 默认值       | 描述                                 |
|--------------------|-----------------|------------------|-------------|-------------------------------------|
| `mode`      | string | required | | how to use cluster metrics, value of `[LeastBusy, LeastTotalLatency, LeastFirstTokenLatency, AdaptiveScore]` |
| `service_list`      | []string | required | | service list of current route |
| `rate_limit`      | string | optional | 1 | The maximum percentage of requests a single node can receive, value of 0~1 |
| `cluster_header` | string | optional | `x-higress-target-cluster` | By retrieving the value of this header, we can determine which backend service to route to |
| `queue_size`      | int | optional | 100 | The metrics is calculated based on the number of most recent requests. |
| `ewma_beta` | float | optional | 0.5 | Historical EWMA weight used by `AdaptiveScore`, value of 0~1 |
| `p2c_choices` | int | optional | 2 | Number of sampled services compared by `AdaptiveScore`; compares all services when the value is not less than the service count |
| `ttft_weight` | float | optional | 0.7 | First-token latency weight used by `AdaptiveScore` |
| `total_latency_weight` | float | optional | 0.3 | Total response latency weight used by `AdaptiveScore` |
| `error_penalty` | float | optional | 3.0 | Cumulative error-rate penalty since the plugin instance started, used by `AdaptiveScore` |
| `failure_cooldown_ms` | int | optional | 30000 | Cooldown duration after a failed request in `AdaptiveScore`; omitted or 0 uses the default |
| `metrics_missing_policy` | string | optional | `least_busy` | Fallback policy when `AdaptiveScore` has no latency samples, defaulting to current in-flight requests |
| `global_inflight_enabled` | bool | optional | false | Whether `AdaptiveScore` uses Redis to track global in-flight requests across gateway instances |
| `serviceFQDN` | string | required when `global_inflight_enabled=true` | | Redis service FQDN |
| `servicePort` | int | required when `global_inflight_enabled=true` | | Redis service port |
| `username` | string | optional | | Redis username |
| `password` | string | optional | empty | Redis password |
| `database` | int | optional | 0 | Redis database number |
| `global_inflight_key_prefix` | string | optional | `higress:adaptive_score_inflight` | Redis key prefix for global in-flight counters. The actual key also includes route and mode |
| `global_inflight_timeout` | int | optional | 3000 | Redis request timeout in milliseconds; omitted or 0 uses the default |
| `global_inflight_key_ttl` | int | optional | 1800 | TTL for global in-flight Redis keys in seconds; omitted or 0 uses the default |

The meanings of the values ​​for `mode` are as follows:

- `LeastBusy`: Routes to the service with the fewest concurrent requests.
- `LeastTotalLatency`: Routes to the service with the lowest response time (RT).
- `LeastFirstTokenLatency`: Routes to the service with the lowest RT for the first packet.
- `AdaptiveScore`: Combines EWMA first-token latency, EWMA total latency, current in-flight requests, and cumulative error rate into a score, then routes to the lowest-score service. It is designed for LLM backends whose latency and load fluctuate continuously.

`AdaptiveScore` computes its error rate from cumulative success and failure counts over the lifetime of the current plugin instance; it does not currently use a time window or decay.

When `global_inflight_enabled` is enabled, `AdaptiveScore` uses Redis Lua to atomically select the service with the lowest score after applying global in-flight pressure, then increments the selected service counter by 1. The counter is decremented when the request stream completes. If Redis initialization, dispatch, or response handling fails, the plugin falls back to local `AdaptiveScore` without blocking the request.

## Configuration Example

```yaml
lb_type: cluster
lb_policy: cluster_metrics
lb_config:
  mode: AdaptiveScore
  cluster_header: x-higress-target-cluster
  rate_limit: 0.6
  ewma_beta: 0.5
  p2c_choices: 2
  ttft_weight: 0.7
  total_latency_weight: 0.3
  error_penalty: 3.0
  failure_cooldown_ms: 30000
  global_inflight_enabled: true
  serviceFQDN: redis.static
  servicePort: 6379
  username: default
  password: ""
  database: 0
  global_inflight_key_prefix: higress:adaptive_score_inflight
  global_inflight_timeout: 3000
  global_inflight_key_ttl: 1800
  service_list:
  - outbound|80||test-1.dns
  - outbound|80||test-2.static
```

# Cluster Hash

## Introduction

Reads a specified request header value and uses FNV-1a consistent hashing to route requests to a fixed upstream cluster. The same hash key always maps to the same cluster, while weighted distribution controls traffic allocation across clusters.

Requires the [explicit EnvoyFilter configuration](#configure-envoyfilter-explicitly-for-cluster-mode) below.

## Configuration

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `clusters` | []ClusterEntry | required | - | Cluster list. Sum of all `weight` values must be 100 |
| `hash_header` | string | optional | `x-mse-consumer` | Request header name to read the hash key from |
| `cluster_header` | string | optional | `x-higress-target-cluster` | Request header name to write the selected cluster into |

### ClusterEntry Fields

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `cluster` | string | yes | Upstream cluster name, e.g. `outbound\|443\|\|llm-xxx.internal.static` |
| `weight` | int | yes | Percentage weight. Sum of all cluster weights must be 100 |

## Configuration Example

```yaml
lb_type: cluster
lb_policy: cluster_hash
lb_config:
  clusters:
    - cluster: "outbound|80||llm-test1.internal.static"
      weight: 69
    - cluster: "outbound|443||llm-test2.internal.dns"
      weight: 30
    - cluster: "outbound|443||llm-test3.internal.dns"
      weight: 1
  hash_header: x-mse-consumer
  cluster_header: x-higress-target-cluster
```

If the request is missing the hash header, the plugin returns **403** directly.

# Configure EnvoyFilter explicitly for cluster mode

With `lb_type: cluster`, ai-load-balancer only writes the selected cluster name to the request header specified by `lb_config.cluster_header`. The user must create and maintain an EnvoyFilter that makes the corresponding Envoy HTTP route read its target cluster from that header. The following example assumes:

- Higress is installed in the `higress-system` namespace and the gateway Pods have the label `app: higress-gateway`;
- the target HTTPRoute or Ingress is named `llm-route` in the `default` namespace, and its generated xDS route name is `default/llm-route`;
- the target host is `llm.example.com`, the gateway listens on port `80`, and the corresponding xDS route configuration and virtual host names are `higress-rds-80.llm.example.com` and `llm.example.com:80`;
- the backend Kubernetes Services are `llm-a` and `llm-b`, both on port `80`.

First, create an ai-load-balancer WasmPlugin for the target route:

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: ai-load-balancer-cluster
  namespace: higress-system
spec:
  pluginName: ai-load-balancer
  url: oci://higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/ai-load-balancer:2.0.1
  matchRules:
    - ingress:
        - default/llm-route
      config:
        lb_type: cluster
        lb_policy: cluster_hash
        lb_config:
          clusters:
            - cluster: "outbound|80||llm-a.default.svc.cluster.local"
              weight: 50
            - cluster: "outbound|80||llm-b.default.svc.cluster.local"
              weight: 50
          hash_header: x-ai-session
          cluster_header: x-higress-target-cluster
```

Then create an EnvoyFilter explicitly for the same route:

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: ai-load-balancer-cluster-header
  namespace: higress-system
spec:
  workloadSelector:
    labels:
      app: higress-gateway
  configPatches:
    - applyTo: HTTP_ROUTE
      match:
        context: GATEWAY
        routeConfiguration:
          name: higress-rds-80.llm.example.com
          vhost:
            name: llm.example.com:80
            route:
              name: default/llm-route
              action: ROUTE
      patch:
        operation: MERGE
        value:
          route:
            cluster_header: x-higress-target-cluster
```

`patch.value.route.cluster_header` must exactly match ai-load-balancer's `lb_config.cluster_header`. `matchRules[].ingress` and `vhost.route.name` must also identify the same actual route. The original HTTPRoute or Ingress must still reference every backend service that the plugin may select so that the corresponding clusters exist in Envoy. This EnvoyFilter changes how the route selects a cluster; it does not create clusters.

The EnvoyFilter's `metadata.namespace` must be the namespace of the target gateway workload, and its `workloadSelector` must match the actual gateway Pod labels. `context: GATEWAY` limits the patch to gateway routes; `routeConfiguration.name`, `vhost.name`, and `vhost.route.name` should further limit it to only the route that uses cluster mode. Higress host-based RDS names normally have the form `higress-rds-<port>.<host>`, while virtual host names have the form `<host>:<port>`.

If the actual xDS configuration uses a standard Istio `http.*` or `https.*` route configuration instead of `higress-rds-*`, use `portNumber`, `portName`, and the internal Istio Gateway name in `gateway` to scope a Gateway API listener. For example, the `https` listener of Kubernetes Gateway `default/llm-gateway` usually maps to:

```yaml
routeConfiguration:
  portNumber: 443
  portName: https
  gateway: default/llm-gateway-istio-autogenerated-k8s-gateway-https
```

Do not combine those `gateway`/`portName`/`portNumber` fields with a `higress-rds-*` route configuration. Do not assume that Kubernetes resource names are the xDS virtual host or route names, and do not put the original Kubernetes Gateway name directly in `routeConfiguration.gateway`. Inspect the gateway's xDS configuration first:

```bash
kubectl -n higress-system exec deploy/higress-gateway -- \
  pilot-agent request GET config_dump > /tmp/higress-config-dump.json

jq -r '
  .configs[]
  | select(."@type" == "type.googleapis.com/envoy.admin.v3.RoutesConfigDump")
  | .dynamic_route_configs[].route_config
  | .name as $route_config
  | .virtual_hosts[]
  | . as $vhost
  | .routes[]
  | [$route_config, $vhost.name, ($vhost.domains | join(",")), .name,
     (.route.cluster_header // .route.clusterHeader // "-")]
  | @tsv
' /tmp/higress-config-dump.json
```

After applying the EnvoyFilter, run the command again. The last column of the target row must be `x-higress-target-cluster`. You can also confirm the Kubernetes object and gateway labels with:

```bash
kubectl -n higress-system get envoyfilter ai-load-balancer-cluster-header -o yaml
kubectl -n higress-system get pods -l app=higress-gateway --show-labels
```

Higress does not create or repair this EnvoyFilter automatically. If the EnvoyFilter is missing, its namespace or workload selector does not match, or its virtual host or route match is wrong, the original route will not read the header written by the plugin. If the route has switched to `cluster_header` but the plugin does not write the same header on that route, requests fail with `404` because no target cluster can be resolved.

`x-higress-target-cluster` is an internal routing control header, not a client API. Do not trust a value supplied by an external client. Reject or remove the header at a trusted boundary before traffic reaches Higress, and ensure that only ai-load-balancer writes it in the target route's processing chain. Otherwise, a client may bypass the intended load-balancing decision.
