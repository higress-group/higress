# 功能说明

`http-call` 插件在请求头阶段异步调用外部 HTTP 服务，将其响应体及指定响应头写入当前请求头，再继续转发。适用于转发前向认证、元数据等辅助服务拉取信息的场景。

# 配置字段

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
| -------- | -------- | -------- | -------- | -------- |
| bodyHeader | string | 必填 | - | 将外部服务响应体写入的请求头名称 |
| tokenHeader | string | 必填 | - | 从外部服务响应头读取并写入请求头的名称 |
| requestPath | string | 必填 | - | 调用外部服务时的请求路径 |
| serviceSource | string | 必填 | - | 服务发现类型：`k8s`、`nacos`、`ip`、`dns` |
| serviceName | string | 必填 | - | 服务名称 |
| servicePort | number | 必填 | - | 服务端口 |
| namespace | string | 选填 | - | `k8s` / `nacos` 时的命名空间 |
| domain | string | 选填 | - | `dns` 类型时的域名 |

# 配置示例

## 调用 K8s 服务

```yaml
bodyHeader: x-auth-body
tokenHeader: x-auth-token
requestPath: /validate
serviceSource: k8s
serviceName: auth-service
servicePort: 8080
namespace: default
```

## 调用 DNS 服务

```yaml
bodyHeader: x-auth-body
tokenHeader: authorization
requestPath: /api/token
serviceSource: dns
serviceName: auth.dns
servicePort: 443
domain: auth.example.com
```

# 说明

- 外部服务返回非 `200` 时，插件记录错误日志，请求仍会继续转发（不会自动拒绝）。
- 响应体中的换行符会被替换为 `#`，避免协议错误。
- 插件在异步回调完成前会暂停请求头处理（`HeaderStopAllIterationAndWatermark`）。
