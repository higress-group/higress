# 功能说明

`sni-misdirect` 用于缓解 HTTPS 场景下 **TLS SNI** 与 HTTP **`:authority`（Host）** 不一致带来的错误路由或错误证书问题。当两者不匹配且不符合通配 SNI 规则时，网关返回 **421 Misdirected Request**。

# 匹配规则

插件在以下条件**全部满足**时进行校验：

- 协议为 HTTP/2 或 HTTP/3（跳过 HTTP/1.x）
- 请求为 HTTPS
- `Content-Type` 不是 `application/grpc`（gRPC 跳过）

校验逻辑：

1. 若 SNI 与 `:authority`（去掉端口后）**完全相同** → 放行
2. 若 SNI **不以** `*.` 开头且与 Host 不一致 → 返回 421
3. 若 SNI 为 `*.example.com` 形式，则 Host 须包含 `example.com` 后缀，否则返回 421

# 配置字段

本插件**无配置项**。

# 配置示例

```yaml
# 无额外配置字段
```

# 说明

- 适用于多证书、多租户或通配证书场景，避免客户端 SNI 与 HTTP Host 不一致时误打到错误虚拟主机。
- 仅作请求头/连接属性检查，不修改上游路由目标。
