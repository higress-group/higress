---
title: AI Provider Affinity
keywords: [higress, ai, provider, affinity, load balancing, session affinity]
description: Consistent hash routing across multiple AI providers based on consumer identity, ensuring the same consumer always routes to the same provider
---

# Introduction

This plugin routes AI requests to a fixed upstream cluster using FNV-1a consistent hashing on the `x-mse-consumer` header. The same consumer always lands on the same provider, while overall traffic distribution respects the configured percentage weights.

Requires the EnvoyFilter `cluster_header` mechanism to be enabled on the target route.

## Configuration

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `providers` | []Provider | Yes | - | Provider list. All `weight` values must sum to 100 |
| `consumer_header` | string | No | `x-mse-consumer` | Request header used to read the consumer identity |
| `cluster_header` | string | No | `x-higress-target-cluster` | Request header to write the selected cluster name into |

### Provider Fields

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `cluster` | string | Yes | Upstream cluster name, same format as Ingress destination, e.g. `outbound|443||llm-xxx.internal.static` |
| `weight` | int | Yes | Percentage weight. All provider weights must sum to 100 |

## Example Configuration

```yaml
providers:
  - cluster: "outbound|80||llm-test1.internal.static"
    weight: 69
  - cluster: "outbound|443||llm-test2..internal.dns"
    weight: 30
  - cluster: "outbound|443||llm-test3.internal.dns"
    weight: 1
consumer_header: x-mse-consumer
cluster_header: x-higress-target-cluster
```

## Prerequisites

An EnvoyFilter must be created for each AI route to enable the `cluster_header` mechanism:

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: ai-cluster-header
  namespace: higress-system
spec:
  configPatches:
    - applyTo: HTTP_ROUTE
      match:
        context: GATEWAY
        routeConfiguration:
          name: "higress-rds-80.*"
          vhost:
            name: "*:80"
            route:
              name: "your-ai-route-name"
      patch:
        operation: MERGE
        value:
          route:
            cluster_header: "x-higress-target-cluster"
```

## Plugin Priority

`ai-provider-affinity` must run after key-auth and before ai-proxy.

Recommended UNSPECIFIED_PHASE priority allocation:

| Plugin | Phase | Priority | Description |
|--------|-------|----------|-------------|
| key-auth | AUTHN | - | Authenticates request, sets `x-mse-consumer` |
| ai-provider-affinity | UNSPECIFIED_PHASE | 900 | Selects provider via hash, sets target cluster |
| model-mapper | UNSPECIFIED_PHASE | 800 | Applies model mapping |
| ai-proxy | UNSPECIFIED_PHASE | 100 | Protocol conversion |

> Higher priority values execute first.

## How It Works

1. `key-auth` authenticates the request and sets the `x-mse-consumer` header
2. `ai-provider-affinity` reads `x-mse-consumer`, computes FNV-1a hash mod 100 over the weight-expanded slot table, and writes the selected cluster to `x-higress-target-cluster`
3. `model-mapper` / `ai-proxy` use `x-higress-target-cluster` for downstream processing

If the consumer header is missing, the plugin returns **403** immediately.

## Routing Consistency

At parse time, the plugin expands the provider list into 100 slots proportional to each provider's weight (e.g. a provider with weight=69 occupies 69 slots). At request time, the consumer string is hashed with FNV-1a and the result is taken mod 100 to select a slot.

- **Same consumer** always maps to the same slot and therefore the same provider
- **Overall traffic distribution** matches the configured percentage weights
- **Stateless** — no session state is maintained; routing results are stable across restarts
