# AI Model Filter Plugin

## 概述

AI Model Filter 是一个用于 Higress 网关的 WASM 插件，用于过滤和控制 AI 模型的访问。该插件可以根据配置的允许模型列表来拒绝或允许特定的 AI 模型请求，帮助企业实现 AI 模型的访问控制和合规管理。

## 功能特性

- **模型白名单控制**：支持配置允许的 AI 模型列表
- **通配符匹配**：支持使用 `*` 通配符进行模型名称匹配
- **多 AI 提供商支持**：支持 OpenAI、Anthropic、Google Gemini 等主流 AI API
- **灵活的拒绝策略**：可自定义拒绝消息和 HTTP 状态码
- **严格/宽松模式**：支持严格模式和宽松模式两种工作方式
- **详细日志记录**：提供详细的请求处理日志

## 支持的 AI API 格式

### 1. OpenAI API
- 从请求体的 `model` 字段提取模型名称
- 示例：`{"model": "gpt-4", "messages": [...]}`

### 2. Google Gemini API
- 从 URL 路径提取模型名称
- 支持 `generateContent` 和 `streamGenerateContent` 端点
- 示例：`/v1/models/gemini-pro:generateContent`

## 配置说明

### 配置参数

| 参数名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|---------|
| `allowed_models` | array | 否 | `[]` | 允许的模型列表，支持通配符 |
| `strict_mode` | boolean | 否 | `true` | 是否启用严格模式 |
| `reject_message` | string | 否 | `"Model not allowed"` | 自定义拒绝消息 |
| `reject_status_code` | integer | 否 | `403` | 自定义拒绝状态码 |

### 配置示例

#### 基本配置
```yaml
allowed_models:
  - "gpt-3.5-turbo"
  - "gpt-4"
strict_mode: true
reject_message: "The requested model is not allowed by policy"
reject_status_code: 403
```

#### 通配符配置
```yaml
allowed_models:
  - "gpt-*"          # 允许所有 gpt 开头的模型
  - "claude-*"       # 允许所有 claude 开头的模型
  - "gemini-pro"     # 精确匹配 gemini-pro
strict_mode: true
```

#### 宽松模式配置
```yaml
allowed_models:
  - "gpt-4"
  - "claude-3-sonnet"
strict_mode: false  # 即使模型不在列表中也允许通过
```

#### 允许所有模型
```yaml
allowed_models: []  # 空列表表示允许所有模型
strict_mode: false
```

## 工作模式

### 严格模式 (strict_mode: true)
- 只允许在 `allowed_models` 列表中的模型
- 如果无法提取模型名称，拒绝请求
- 如果模型不在允许列表中，拒绝请求

### 宽松模式 (strict_mode: false)
- 优先允许在 `allowed_models` 列表中的模型
- 如果无法提取模型名称，允许请求通过
- 如果模型不在允许列表中但 `allowed_models` 不为空，仍然拒绝请求
- 如果 `allowed_models` 为空，允许所有请求

## 错误响应格式

当请求被拒绝时，插件会返回 JSON 格式的错误响应：

```json
{
  "error": {
    "message": "The requested model is not allowed by policy",
    "type": "model_not_allowed",
    "code": "model_filter_rejected",
    "details": "Model not allowed: gpt-4-unauthorized"
  }
}
```

## 使用场景

### 1. 企业合规管理
- 限制员工只能使用经过审批的 AI 模型
- 防止使用未经授权或高成本的模型

### 2. 成本控制
- 限制使用高成本的 AI 模型
- 根据不同环境配置不同的模型策略

### 3. 安全控制
- 防止使用可能存在安全风险的模型
- 实现分级的模型访问控制

### 4. 开发测试
- 在开发环境允许所有模型
- 在生产环境严格控制模型使用

## 部署配置

### Higress Console 配置

1. 在 Higress Console 中创建新的 WASM 插件
2. 上传编译后的 WASM 文件
3. 配置插件参数
4. 应用到相应的路由或域名

### Kubernetes 配置示例

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: ai-model-filter
  namespace: higress-system
spec:
  defaultConfig:
    allowed_models:
      - "gpt-3.5-turbo"
      - "gpt-4"
      - "claude-*"
    strict_mode: true
    reject_message: "Model not allowed by company policy"
    reject_status_code: 403
  url: oci://your-registry/ai-model-filter:latest
```

### 使用 matchRules 针对特定路由配置

如果需要针对不同的Ingress路由应用不同的模型过滤策略，可以使用`matchRules`：

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: ai-model-filter
  namespace: higress-system
spec:
  defaultConfigDisable: true
  failStrategy: FAIL_OPEN
  imagePullPolicy: UNSPECIFIED_POLICY
  matchRules:
  - config:
      allowed_models:
        - "gpt-3.5-turbo"
        - "gpt-4"
        - "claude-*"
      strict_mode: true
      reject_message: "Model not allowed by company policy"
      reject_status_code: 403
    configDisable: false
    ingress:
    - ai-route-test-deepseek.internal
  - config:
      allowed_models:
        - "gpt-4"
        - "claude-3-*"
      strict_mode: false
      reject_message: "Premium models only"
      reject_status_code: 402
    configDisable: false
    ingress:
    - premium-ai-route.internal
  phase: UNSPECIFIED_PHASE
  priority: 600
  url: oci://your-registry/ai-model-filter:latest
```

#### matchRules 配置说明

- **defaultConfigDisable**: 设置为 `true` 时禁用默认配置，只使用 matchRules 中的配置
- **failStrategy**: 插件失败时的策略
  - `FAIL_OPEN`: 插件失败时允许请求通过
  - `FAIL_CLOSE`: 插件失败时拒绝请求
- **matchRules**: 匹配规则列表，按顺序匹配
  - **config**: 针对匹配路由的具体配置
  - **configDisable**: 是否禁用此规则的配置
  - **ingress**: 匹配的 Ingress 名称列表
- **phase**: 插件执行阶段（通常使用 UNSPECIFIED_PHASE）
- **priority**: 插件执行优先级，数值越大优先级越高

#### 使用场景

1. **多环境部署**: 为开发、测试、生产环境配置不同的模型策略
2. **多租户支持**: 为不同客户或部门配置不同的模型访问权限
3. **成本控制**: 为不同服务配置不同成本级别的模型
4. **A/B测试**: 为不同用户群体配置不同的模型策略

## 日志示例

```
[INFO] AI Model Filter Config: allowed_models=[gpt-4 claude-3-sonnet], strict_mode=true, reject_message=Model not allowed, reject_status_code=403
[INFO] Extracted model name: gpt-4
[INFO] Model 'gpt-4' is allowed, continuing request
[WARN] Model 'gpt-4-unauthorized' is not in the allowed list: [gpt-4 claude-3-sonnet]
[WARN] Rejecting request: Model not allowed: gpt-4-unauthorized
```

## 编译和构建

```bash
# 进入插件目录
cd plugins/wasm-go/extensions/ai-model-filter

# 编译 WASM 文件
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o ./main.wasm .

# 或使用 Higress 提供的构建脚本
make build
```

## 注意事项

1. **性能影响**：插件会解析请求体来提取模型名称，对于大型请求可能有轻微性能影响
2. **模型名称提取**：目前支持主流 AI API 格式，如需支持其他格式可能需要扩展代码
3. **配置更新**：配置更新后需要重新加载插件才能生效
4. **日志级别**：建议在生产环境中适当调整日志级别以避免过多日志输出

## 故障排除

### 常见问题

1. **无法提取模型名称**
   - 检查请求格式是否符合支持的 API 格式
   - 查看日志确认请求体内容

2. **配置不生效**
   - 确认插件配置格式正确
   - 检查插件是否正确应用到路由

3. **意外拒绝请求**
   - 检查 `allowed_models` 配置
   - 确认 `strict_mode` 设置
   - 查看详细日志了解拒绝原因

### 调试建议

1. 启用详细日志记录
2. 使用测试请求验证配置
3. 检查 Higress 网关日志
4. 确认 WASM 插件加载状态