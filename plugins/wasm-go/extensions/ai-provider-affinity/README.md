---
title: AI Provider Affinity
keywords: [higress, ai, provider, affinity, 负载均衡, 会话保持]
description: 基于 consumer 标识对多个 AI provider 做一致性 hash 路由，同一 consumer 始终路由到同一 provider
---

# 简介

本插件基于 `x-mse-consumer` 头部，使用 FNV-1a 一致性 hash 算法将 AI 请求路由到固定的上游集群，确保同一 consumer 的请求始终落到同一个 provider，同时支持按百分比权重控制各 provider 的流量分配。

需要配合 EnvoyFilter 的 `cluster_header` 机制一起使用。

## 配置说明

| 名称 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `providers` | []Provider | 是 | - | provider 列表，所有 `weight` 之和必须为 100 |
| `consumer_header` | string | 否 | `x-mse-consumer` | 读取 consumer 标识的请求头名称 |
| `cluster_header` | string | 否 | `x-higress-target-cluster` | 写入目标 cluster 的请求头名称 |

### Provider 字段

| 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `cluster` | string | 是 | 上游集群名称，格式与 Ingress destination 一致，如 `outbound|443||llm-xxx.internal.static` |
| `weight` | int | 是 | 百分比权重，所有 provider 的 weight 之和必须为 100 |

## 配置示例

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

## 前置条件

需要为每条 AI 路由创建 EnvoyFilter，开启 `cluster_header` 机制：

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

## 优先级调整

`ai-provider-affinity` 需要在 key-auth 之后、ai-proxy 之前执行。

建议的 UNSPECIFIED_PHASE 优先级分配：

| 插件 | Phase | Priority | 说明 |
|------|-------|----------|------|
| key-auth | AUTHN | - | 认证请求，设置 `x-mse-consumer` |
| ai-provider-affinity | UNSPECIFIED_PHASE | 900 | hash 选 provider，设置目标 cluster |
| model-mapper | UNSPECIFIED_PHASE | 800 | 应用模型映射 |
| ai-proxy | UNSPECIFIED_PHASE | 100 | 协议转换 |

> Priority 越大越先执行。

## 工作流程

1. `key-auth` 认证请求，设置 `x-mse-consumer` 头部
2. `ai-provider-affinity` 读取 `x-mse-consumer`，用 FNV-1a hash 对 100 个权重槽取模，选出目标 cluster，写入 `x-higress-target-cluster`
3. `model-mapper` / `ai-proxy` 根据 `x-higress-target-cluster` 完成后续处理

若请求缺少 consumer 头部，插件直接返回 **403**。

## 路由一致性说明

插件在 `parseConfig` 阶段将 provider 列表按权重展开为 100 个槽（如 weight=69 的 provider 占 69 个槽），运行时对 consumer 字符串做 FNV-1a hash 后对 100 取模，确定目标槽位。

- **同一 consumer** 的 hash 值固定，始终路由到同一 provider
- **整体流量分布** 符合配置的百分比权重
- **无状态**，不需要维护会话状态，重启后路由结果不变
