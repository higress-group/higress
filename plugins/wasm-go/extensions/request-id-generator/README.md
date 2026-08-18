# Request ID Generator Plugin

## 功能说明

Request ID Generator 插件为每个通过 Higress 网关的 HTTP 请求自动生成并注入唯一的请求 ID。这对于分布式追踪、调试和请求关联非常有用。

## 使用场景

- **分布式追踪**：在微服务架构中追踪请求的完整生命周期
- **日志关联**：将不同服务的日志通过请求 ID 关联起来
- **调试和排错**：快速定位和追踪特定请求的问题
- **客户支持**：客户可以引用请求 ID 报告问题

## 配置说明

| 配置项 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| `enable` | boolean | 否 | `true` | 是否启用插件 |
| `request_header` | string | 否 | `"X-Request-Id"` | 上游请求头中的请求 ID 字段名 |
| `response_header` | string | 否 | `""` | 响应头中的请求 ID 字段名（空字符串表示不添加） |
| `override_existing` | boolean | 否 | `false` | 是否覆盖已存在的请求 ID |

## 配置示例

### 基础配置

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: request-id-generator
  namespace: higress-system
spec:
  defaultConfig:
    enable: true
    request_header: "X-Request-Id"
```

### 完整配置（包含响应头）

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: request-id-generator
  namespace: higress-system
spec:
  defaultConfig:
    enable: true
    request_header: "X-Request-Id"
    response_header: "X-Request-Id"
    override_existing: false
```

### 自定义请求头名称

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: request-id-generator
  namespace: higress-system
spec:
  defaultConfig:
    enable: true
    request_header: "X-Trace-Id"
    response_header: "X-Trace-Id"
```

## 行为说明

### 请求 ID 生成规则

1. **无现有请求 ID**：插件生成新的 UUID v4 格式的请求 ID
2. **存在请求 ID 且 override_existing=false**：保留原有请求 ID
3. **存在请求 ID 且 override_existing=true**：生成新的请求 ID 并覆盖

### UUID 格式

插件生成标准的 UUID v4 格式：`xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx`

示例：`550e8400-e29b-41d4-a716-446655440000`

### 响应头行为

- 如果配置了 `response_header`，插件会将请求 ID 添加到响应头中
- 这使得客户端可以获取请求 ID 用于问题报告或日志关联
- 如果不需要向客户端暴露请求 ID，可以不配置或设置为空字符串

## 使用示例

### 场景 1：微服务链路追踪

```yaml
# Gateway 配置
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: request-id-generator
spec:
  defaultConfig:
    request_header: "X-Request-Id"
    override_existing: false  # 保留上游传来的 ID
```

所有经过网关的请求都会携带统一的请求 ID，微服务可以在日志中记录这个 ID：

```go
// 微服务代码示例
requestID := r.Header.Get("X-Request-Id")
log.Printf("[%s] Processing user request", requestID)
```

### 场景 2：客户端请求追踪

```yaml
# 配置响应头，让客户端可以获取请求 ID
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: request-id-generator
spec:
  defaultConfig:
    request_header: "X-Request-Id"
    response_header: "X-Request-Id"  # 在响应中返回
```

客户端可以从响应头获取请求 ID：

```javascript
// 前端代码示例
fetch('/api/users')
  .then(response => {
    const requestId = response.headers.get('X-Request-Id');
    console.log('Request ID:', requestId);
  });
```

### 场景 3：多层网关场景

在多层网关架构中，通常只在最外层网关生成请求 ID：

```yaml
# 外层网关：生成请求 ID
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: request-id-generator
  namespace: external-gateway
spec:
  defaultConfig:
    request_header: "X-Request-Id"
    override_existing: false
```

```yaml
# 内层网关：保留现有请求 ID
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: request-id-generator
  namespace: internal-gateway
spec:
  defaultConfig:
    request_header: "X-Request-Id"
    override_existing: false  # 不覆盖外层网关生成的 ID
```

## 性能说明

- UUID 生成时间：< 0.1ms
- 内存占用：每个请求约 100 字节
- 对请求延迟的影响：< 1ms

## 注意事项

1. **请求头名称一致性**：建议在整个组织内使用统一的请求头名称（如 `X-Request-Id`）
2. **UUID 唯一性**：虽然 UUID v4 冲突概率极低（< 1 in 10^36），但理论上存在冲突可能
3. **不覆盖现有 ID**：默认情况下不覆盖已有的请求 ID，这样可以保持请求 ID 在整个调用链中的一致性
4. **日志集成**：确保所有微服务都在日志中记录请求 ID，以便进行日志关联

## 与其他插件的集成

### 与日志插件集成

```yaml
# 先生成请求 ID
- request-id-generator
# 然后在日志中记录
- log-request-response
```

### 与监控插件集成

请求 ID 可以用于关联监控指标和追踪数据，与 OpenTelemetry、Jaeger 等追踪系统集成。

## 故障排查

### 问题：请求 ID 没有被注入

**可能原因**：
- 插件未启用（`enable: false`）
- 配置错误
- 插件加载失败

**解决方法**：
1. 检查插件配置是否正确
2. 查看 Higress 日志确认插件是否成功加载
3. 确认 WasmPlugin 资源已正确应用

### 问题：请求 ID 被意外覆盖

**可能原因**：
- `override_existing` 设置为 `true`
- 多个插件实例配置冲突

**解决方法**：
1. 检查 `override_existing` 配置
2. 确保只在需要的地方生成新的请求 ID

## 版本历史

- v1.0.0：初始版本，支持 UUID v4 生成和请求/响应头注入

