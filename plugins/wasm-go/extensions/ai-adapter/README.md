# AI Adapter Plugin for Higress

## 概述

AI Adapter 插件用于解决不同 AI 服务渠道之间的协议差异问题。当一个模型需要接入多个渠道时，每个渠道的请求 URL、请求参数结构、响应结构可能不一致。本插件通过配置化的方式，在请求和响应阶段对这些差异进行自动适配。

## 功能特性

- **模型与渠道映射**：支持将特定模型路由到不同渠道
- **请求转换**：支持请求 URL 重写、参数转换、格式转换、头转换
- **响应转换**：支持响应头转换、响应 body 转换
- **灵活配置**：支持 JSON 格式配置，易于扩展
- **内存安全**：及时释放内存，防止内存泄漏

## 使用场景

### 场景 1：Azure 渠道适配

对于 `gpt-image-2` 模型，当路由到 Azure 渠道时，需要将请求 body 中的 `image` 参数转换为 multipart file 格式。

**配置示例：**
```yaml
modelProviderMappings:
  - model: "gpt-image-2"
    provider: "Azure"
    requestTransform:
      type: "format_transform"
      formatTransform:
        targetFormat: "multipart"
        multipartConfig:
          fieldMapping:
            image: "file"  # 将 image 字段映射为 file 字段
          removeFields:
            - "prompt"    # 移除不需要的字段
```

### 场景 2：RightCode 渠道适配

对于 `gpt-image-2` 模型，当路由到 RightCode 渠道时，无论是文生图 `/v1/images/generations` 还是图生图 `/v1/images/edits`，都要重写为 `/v1/images/generations`。

**配置示例：**
```yaml
modelProviderMappings:
  - model: "gpt-image-2"
    provider: "RightCode"
    requestTransform:
      type: "url_rewrite"
      urlRewrite:
        fromPattern: "/v1/images/edits"
        toPattern: "/v1/images/generations"
```

### 场景 3：Qiniu 渠道适配

对于 `gpt-image-2` 模型，当路由到 Qiniu 渠道时，不需要进行任何转换。

**配置示例：**
```yaml
modelProviderMappings:
  - model: "gpt-image-2"
    provider: "Qiniu"
    requestTransform:
      type: "none"
```

## 配置说明

### 按 URI 和请求字段替换模型

新增 `requestBodyRules` 可处理业务任务接口，不依赖 `wasm.requestModel` 或模型渠道映射。
完整示例见 `config-seedream-model-rewrite.json`，可以把其中的 `requestBodyRules` 合并进已有配置，并启用 `enableRequestTransform`。

```json
{
  "enableRequestTransform": true,
  "requestBodyRules": [{
    "uri": "/wj-open/v2/open-platform/task/task_create_sync",
    "method": "POST",
    "conditions": [
      {"path": ["taskCode"], "equals": 2511},
      {"path": ["param", "model"], "equals": "doubao-seedream-5-0-lite-260128"}
    ],
    "set": [
      {"path": ["param", "model"], "value": "doubao-seedream-5-0-pro-260628"}
    ]
  }]
}
```

- `uri` 精确匹配插件收到的请求路径，忽略查询参数，不支持通配符。`method` 默认 `POST`。
- `conditions` 全部满足才执行 `set`；规则按配置顺序处理，只执行第一条命中的规则。
- `path` 是逐层对象字段名数组，最多 32 层。支持穿过 JSON 字符串，写回后仍保留字符串类型；不支持数组下标。字段名中的点按字面处理。
- 值按 JSON 类型比较：数字 `2511` 不等于字符串 `"2511"`；数字使用原始表示，`2511.0` 不等于 `2511`。大整数不会转成浮点数。
- 此前任务请求的 `taskCode` 在顶层，因此示例为 `["taskCode"]`；如果实际请求位于 `param` 中，改为 `["param", "taskCode"]` 即可。`param` 是对象或 JSON 字符串均支持。
- 只处理未带 `Content-Encoding` 的 `application/json` 或 `application/*+json` 请求。无效 JSON、条件字段缺失或不匹配时不修改 body；写入字段必须已存在，任一写入失败则整条规则不写回。
- 未配置规则时沿用原有流程。URI 和方法命中候选规则的 JSON 请求会缓存 body（沿用 100 MB 上限），并移除 `Content-Length` 由代理处理消息边界，即使最终 body 条件未命中也是如此。
- 原有头转换仍执行；新规则在原有 body 转换之前执行。若两者修改同一字段，后执行的原有 body 转换会覆盖新规则。新规则不重算已确定的模型路由或 `wasm.requestModel`。

插件必须挂在该任务创建接口实际经过的网关路由上，并在读取请求体的业务处理前执行。如果入口已重写 URI，配置需使用插件看到的路径。修改本地代码和配置不会自动改变已部署插件，需构建并发布新版本后生效。

### 完整配置结构

```yaml
{
  "modelProviderMappings": [
    {
      "model": "模型名称（支持通配符 *）",
      "provider": "渠道名称",
      "requestTransform": {
        "type": "转换类型",
        "urlRewrite": {
          "path": "新路径",
          "fromPattern": "匹配模式",
          "toPattern": "替换模式"
        },
        "paramTransform": {
          "renameMap": {
            "旧参数名": "新参数名"
          },
          "removeParams": ["要删除的参数"],
          "addParams": {
            "新参数名": "参数值"
          }
        },
        "formatTransform": {
          "targetFormat": "目标格式",
          "multipartConfig": {
            "fieldMapping": {
              "旧字段名": "新字段名"
            },
            "addFields": {
              "新字段名": "字段值"
            },
            "removeFields": ["要删除的字段"]
          }
        },
        "headerTransform": {
          "addHeaders": {
            "新头名": "头值"
          },
          "removeHeaders": ["要删除的头"],
          "renameHeaders": {
            "旧头名": "新头名"
          }
        },
        "bodyTransform": {
          "setValues": {
            "JSON路径": "值"
          },
          "removePaths": ["要删除的JSON路径"]
        }
      },
      "responseTransform": {
        // 结构与 requestTransform 相同
      }
    }
  ],
  "defaultProvider": "默认渠道名称",
  "enableRequestTransform": true,
  "enableResponseTransform": true
}
```

### 配置字段说明

#### modelProviderMappings

模型到渠道的映射配置数组。

- **model**: 模型名称，支持通配符 `*`（如 `gpt-*` 匹配所有以 gpt- 开头的模型）
- **provider**: 渠道名称
- **requestTransform**: 请求转换配置（可选）
- **responseTransform**: 响应转换配置（可选）

#### requestTransform/responseTransform

转换配置对象。

- **type**: 转换类型
  - `url_rewrite`: URL 重写
  - `param_transform`: 参数转换
  - `format_transform`: 格式转换
  - `header_transform`: 头转换
  - `body_transform`: Body 转换
  - `none`: 不进行转换

##### urlRewrite

URL 重写配置。

- **path**: 直接设置新路径
- **fromPattern**: 要匹配的路径模式
- **toPattern**: 替换为的目标路径

##### paramTransform

参数转换配置。

- **renameMap**: 参数重命名映射
- **removeParams**: 要删除的参数列表
- **addParams**: 要添加的参数

##### formatTransform

格式转换配置。

- **targetFormat**: 目标格式（`multipart`, `json`, `form`）
- **multipartConfig**: multipart 格式配置
  - **fieldMapping**: 字段名映射
  - **addFields**: 要添加的字段
  - **removeFields**: 要删除的字段

##### headerTransform

头转换配置。

- **addHeaders**: 要添加的头
- **removeHeaders**: 要删除的头
- **renameHeaders**: 要重命名的头

##### bodyTransform

Body 转换配置（JSON 路径操作）。

- **setValues**: 要设置的 JSON 路径和值
- **removePaths**: 要删除的 JSON 路径

#### defaultProvider

默认渠道名称，当没有匹配到模型时使用。

#### enableRequestTransform/enableResponseTransform

是否启用请求/响应转换的全局开关。

## 执行顺序

本插件应该在以下插件之后执行：

1. **key-auth**：鉴权插件
2. **model-router**：模型路由插件
3. **ai-proxy**：Body 重写插件

执行流程：

```
请求 → 鉴权 → 模型路由 → Body 重写 → AI Adapter (请求转换) → 调用上游
                                                ↓
响应 ← AI Adapter (响应转换) ← 上游响应
```

## 内存管理

本插件采用了以下措施确保内存安全：

1. **配置复用**：配置在插件启动时加载并复用，避免重复解析
2. **及时清理**：请求处理完成后，上下文中的临时数据自动清理
3. **大小限制**：请求和响应 body 大小限制为 100MB
4. **内存重建**：每处理 1000 个请求或内存超过 200MB 后重建配置

## 依赖

本插件依赖以下 Higress WASM SDK：

- `github.com/higress-group/proxy-wasm-go-sdk`
- `github.com/higress-group/wasm-go`
- `github.com/tidwall/gjson` (JSON 解析)
- `github.com/tidwall/sjson` (JSON 修改)

## 构建

```bash
# 使用 Go 1.24 原生编译
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o main.wasm

# 或使用 TinyGo 编译
./build.sh
```

## 部署

### Docker 镜像

```bash
# 构建镜像
docker build -t ai-adapter:1.0.0 .

# 推送到镜像仓库
docker push your-registry/ai-adapter:1.0.0
```

### Higress 配置

在 Higress 中配置插件：

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: ai-adapter
  namespace: higress-system
spec:
  image: your-registry/ai-adapter:1.0.0
  priority: 500  # 确保在 ai-proxy 之后执行
  phase: AUTHN
  matchRules:
    - config:
        modelProviderMappings:
          - model: "gpt-image-2"
            provider: "Azure"
            requestTransform:
              type: "format_transform"
              formatTransform:
                targetFormat: "multipart"
                multipartConfig:
                  fieldMapping:
                    image: "file"
        enableRequestTransform: true
        enableResponseTransform: false
```

## 示例

### 完整示例配置

```yaml
{
  "modelProviderMappings": [
    {
      "model": "gpt-image-2",
      "provider": "Azure",
      "requestTransform": {
        "type": "format_transform",
        "formatTransform": {
          "targetFormat": "multipart",
          "multipartConfig": {
            "fieldMapping": {
              "image": "file"
            },
            "removeFields": ["model"]
          }
        }
      }
    },
    {
      "model": "gpt-image-2",
      "provider": "RightCode",
      "requestTransform": {
        "type": "url_rewrite",
        "urlRewrite": {
          "fromPattern": "/v1/images/edits",
          "toPattern": "/v1/images/generations"
        }
      }
    },
    {
      "model": "gpt-image-2",
      "provider": "Qiniu",
      "requestTransform": {
        "type": "none"
      }
    },
    {
      "model": "*",
      "provider": "OpenAI",
      "requestTransform": {
        "type": "header_transform",
        "headerTransform": {
          "addHeaders": {
            "X-Custom-Header": "value"
          }
        }
      }
    }
  ],
  "defaultProvider": "OpenAI",
  "enableRequestTransform": true,
  "enableResponseTransform": false
}
```

## 注意事项

1. **Provider 识别**：插件通过请求头 `X-Provider` 或 Envoy 属性 `wasm.ai_provider` 来识别渠道
2. **Base64 图片处理**：支持将 Base64 编码的图片 data URI 自动转换为 multipart file 格式
3. **查询参数保留**：URL 重写时会保留原始请求的查询参数
4. **错误处理**：转换失败时会记录错误日志，但不会阻塞请求继续处理
5. **性能考虑**：复杂的格式转换（如 multipart）会有一定的性能开销

## 版本历史

### v1.0.0
- 初始版本
- 支持模型与渠道映射
- 支持多种请求/响应转换类型
- 支持 multipart 格式转换
- 内存安全保证

## 许可证

Apache License 2.0
