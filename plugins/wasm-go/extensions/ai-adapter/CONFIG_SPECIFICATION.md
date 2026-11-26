# AI Adapter 插件配置规范

## 概述

AI Adapter 插件用于解决不同 AI 服务渠道之间的协议差异。通过配置文件，可以为一个模型配置多个不同渠道的转换规则。

## 渠道识别方式

插件通过以下方式识别目标渠道：

1. **请求头**: `X-Provider: <渠道名称>`
2. **Envoy 属性**: `wasm.ai_provider` (由前置插件设置)

## 配置结构

### 根级别配置

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `modelProviderMappings` | 数组 | 否 | 模型到渠道的映射配置 |
| `requestBodyRules` | 数组 | 否 | 按 URI、方法和 JSON 字段匹配的请求体修改规则 |
| `enableRequestTransform` | 布尔 | 否 | 是否启用请求转换 |
| `enableResponseTransform` | 布尔 | 否 | 是否启用响应转换 |

### RequestBodyRule 配置

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `uri` | 字符串 | 是 | 精确路径，不含查询参数；匹配时忽略请求查询参数 |
| `method` | 字符串 | 否 | 大写 HTTP 方法，默认 `POST` |
| `conditions` | 数组 | 是 | 非空，每项包含 `path` 和 `equals`，所有条件同时匹配 |
| `set` | 数组 | 是 | 非空，每项包含 `path` 和 `value`，只修改已存在的字段 |

`path` 使用非空字符串数组表示对象字段路径，最多 32 层；支持穿过 JSON 字符串，但不支持数组下标。
`equals` 和 `value` 可为任意 JSON 值（含 `null`），不能省略。比较保留类型及数字表示，不会将 `"2511"` 或 `2511.0` 当作 `2511`。
配置顺序决定优先级，只执行首条命中的规则；所有写入成功后才更新请求体。
需要开启 `enableRequestTransform`，并将插件配置在业务接口经过的路由上。
与原有转换的执行顺序、Content-Type 限制及示例见 README 的“按 URI 和请求字段替换模型”部分和 `config-seedream-model-rewrite.json`。

### ModelProviderMapping 配置

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `model` | 字符串 | 是 | 模型名称，支持通配符（如 `gpt-*`） |
| `provider` | 字符串 | 是 | 渠道名称 |
| `requestTransform` | 对象 | 否 | 请求转换配置 |
| `responseTransform` | 对象 | 否 | 响应转换配置 |

### TransformConfig 配置

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | 字符串 | 是 | 转换类型 |
| `urlRewrite` | 对象 | 否 | URL 重写配置 |
| `formatTransform` | 对象 | 否 | 格式转换配置 |
| `paramTransform` | 对象 | 否 | 参数转换配置 |
| `headerTransform` | 对象 | 否 | 头转换配置 |
| `bodyTransform` | 对象 | 否 | Body 转换配置 |

### 转换类型

| 类型 | 说明 | 支持的配置字段 |
|------|------|----------------|
| `none` | 不进行转换 | - |
| `url_rewrite` | URL 路径重写 | `urlRewrite` |
| `format_transform` | 格式转换（JSON ↔ multipart） | `formatTransform` |
| `param_transform` | JSON 参数转换 | `paramTransform` |
| `header_transform` | HTTP 头转换 | `headerTransform` |
| `body_transform` | JSON Body 路径操作 | `bodyTransform` |

## 配置示例

### Azure 渠道 - Multipart 转换

```yaml
- model: gpt-image-2
  provider: Azure
  requestTransform:
    type: format_transform
    formatTransform:
      targetFormat: multipart
      multipartConfig:
        fieldMapping:
          image: image  # 将 image 字段转换为文件流
        # addFields: 可选，添加额外的表单字段
```

**说明**：
- `fieldMapping`: 指定哪些 JSON 字段需要转换为 multipart 文件流
  - 格式：`JSON字段名: multipart字段名`
  - 在 `fieldMapping` 中的字段会被转换为文件流
  - **不在** `fieldMapping` 中的字段会作为普通 form 字段保留
- `addFields`: 可选，添加额外的表单字段
- **只支持 image 数组处理**：`image` 字段必须是数组格式
- 数组中每个元素支持：
  - Base64 编码的图片：`data:image/...;base64,...` 格式，自动解码为二进制文件流
  - URL 格式的图片：在 WASM 环境中受限，记录警告后作为文本处理
- 数组元素会被转换为独立的 multipart 文件字段：`image[0]`, `image[1]`, ...

### RightCode 渠道 - URL 重写

```yaml
- model: gpt-image-2
  provider: RightCode
  requestTransform:
    type: url_rewrite
    urlRewrite:
      fromPattern: /v1/images/edits
      toPattern: /v1/images/generations
```

**说明**：
- 将 `/v1/images/edits` 重写为 `/v1/images/generations`
- 保留原始请求的查询参数
- 支持简单的字符串替换

### Qiniu 渠道 - 不转换

```yaml
- model: gpt-image-2
  provider: Qiniu
  requestTransform:
    type: none
```

**说明**：
- 不进行任何转换，直接透传请求

### 通配符匹配

```yaml
- model: gpt-*
  provider: OpenAI
  requestTransform:
    type: header_transform
    headerTransform:
      addHeaders:
        X-Custom-Header: "value"
      removeHeaders:
        - X-Internal-Header
```

**说明**：
- `model: "gpt-*"` 匹配所有以 `gpt-` 开头的模型
- `model: "*"` 匹配所有模型（通常作为最后兜底）

## 请求示例

### 发送到 Azure 渠道

```bash
curl -X POST http://gateway/v1/images/generations \n  -H "X-Provider: Azure" \n  -H "Content-Type: application/json" \n  -d '{
    "model": "gpt-image-2",
    "image": "data:image/png;base64,iVBORw0KGgo...",
    "prompt": "a beautiful landscape"
  }'
```

**插件处理**：
- 检测到 `X-Provider: Azure`
- 匹配到 `gpt-image-2` 模型
- 将 JSON 转换为 multipart/form-data
- `image` 字段变成 `file` 字段，值为解码后的二进制数据

### 发送到 RightCode 渠道

```bash
curl -X POST http://gateway/v1/images/edits \n  -H "X-Provider: RightCode" \n  -H "Content-Type: application/json" \n  -d '{
    "model": "gpt-image-2",
    "image": "data:image/png;base64,iVBORw0KGgo...",
    "prompt": "a beautiful landscape"
  }'
```

**插件处理**：
- 检测到 `X-Provider: RightCode`
- 匹配到 `gpt-image-2` 模型
- 将 URL 从 `/v1/images/edits` 重写为 `/v1/images/generations`

### 发送到 Qiniu 渠道

```bash
curl -X POST http://gateway/v1/images/generations \n  -H "X-Provider: Qiniu" \n  -H "Content-Type: application/json" \n  -d '{
    "model": "gpt-image-2",
    "image": "data:image/png;base64,iVBORw0KGgo...",
    "prompt": "a beautiful landscape"
  }'
```

**插件处理**：
- 检测到 `X-Provider: Qiniu`
- 匹配到 `gpt-image-2` 模型
- 不进行任何转换，直接透传

## Base64 图片处理

插件自动识别和处理 Base64 编码的图片：

1. **识别格式**: `data:image/<type>;base64,<data>`
2. **自动解码**: 解码 Base64 数据为二进制
3. **文件上传**: 作为 multipart file 字段上传
4. **文件命名**: 默认使用字段名 + `.png` 扩展名

**示例**：
```json
{
  "image": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII="
}
```

转换后：
```
Content-Disposition: form-data; name="file"; filename="file.png"
Content-Type: image/png

<binary data>
```

## 查询参数保留

URL 重写时会保留原始请求的查询参数：

```
原始请求: /v1/images/edits?param=value
重写后:   /v1/images/generations?param=value
```

## 高级配置

### 参数转换

```yaml
requestTransform:
  type: param_transform
  paramTransform:
    renameMap:
      old_param: new_param
    removeParams:
      - unused_param
    addParams:
      extra_param: "value"
```

### Body 转换

```yaml
requestTransform:
  type: body_transform
  bodyTransform:
    setValues:
      new_field: "value"
    removePaths:
      - "internal.field"
```

### 头转换

```yaml
requestTransform:
  type: header_transform
  headerTransform:
    addHeaders:
      X-Custom-Header: "value"
    removeHeaders:
      - X-Internal-Header
    renameHeaders:
      X-Old-Header: X-New-Header
```

### 响应转换

```yaml
responseTransform:
  type: body_transform
  bodyTransform:
    setValues:
      provider: "OpenAI"
    removePaths:
      - "internal_field"
```

## 注意事项

1. **配置顺序**: `modelProviderMappings` 按顺序匹配，通配符规则应该放在最后
2. **渠道识别**: 必须通过 `X-Provider` 头或 `wasm.ai_provider` 属性指定渠道
3. **类型检查**: 插件会检查配置类型的合法性，配置错误时会记录日志
4. **错误处理**: 转换失败时不会阻塞请求，只会记录错误日志
5. **性能考虑**: multipart 转换会有一定的性能开销，建议合理使用

## 配置验证

部署前建议验证配置：

```bash
# 1. 检查 JSON 格式
cat config.json | jq .

# 2. 测试配置加载
# (通过 Higress 控制台或 API)

# 3. 观察日志
kubectl logs -n higress-system <gateway-pod> | grep ai-adapter
```
