---
title: AI 代理
keywords: [AI网关, AI代理]
description: AI 代理插件配置参考
---

## 功能说明

`AI 代理`插件实现了基于 OpenAI API 契约的 AI 代理功能。目前支持 OpenAI、Azure OpenAI、月之暗面（Moonshot）和通义千问等 AI
服务提供商。

**🚀 自动协议兼容 (Auto Protocol Compatibility)**

插件现在支持**自动协议检测**，无需配置即可同时兼容 OpenAI 和 Claude 两种协议格式：

- **OpenAI 协议**: 请求路径 `/v1/chat/completions`，使用标准的 OpenAI Messages API 格式
- **Claude 协议**: 请求路径 `/v1/messages`，使用 Anthropic Claude Messages API 格式  
- **智能转换**: 自动检测请求协议，如果目标供应商不原生支持该协议，则自动进行协议转换
- **零配置**: 用户无需设置 `protocol` 字段，插件自动处理

> **协议支持说明：**

> 请求路径后缀匹配 `/v1/chat/completions` 时，对应文生文场景，会用 OpenAI 的文生文协议解析请求 Body，再转换为对应 LLM 厂商的文生文协议

> 请求路径后缀匹配 `/v1/messages` 时，对应 Claude 文生文场景，会自动检测供应商能力：如果支持原生 Claude 协议则直接转发，否则先转换为 OpenAI 协议再转发给供应商

> 请求路径后缀匹配 `/v1/embeddings` 时，对应文本向量场景，会用 OpenAI 的文本向量协议解析请求 Body，再转换为对应 LLM 厂商的文本向量协议

> 请求路径后缀匹配 `/v1/images/generations` 时，对应文生图场景，会用 OpenAI 的图片生成协议解析请求 Body，再转换为对应 LLM 厂商的图片生成协议

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`100`

## 配置字段

### 基本配置

| 名称       | 数据类型 | 填写要求 | 默认值 | 描述                         |
| ---------- | -------- | -------- | ------ | ---------------------------- |
| `provider` | object   | 必填     | -      | 配置目标 AI 服务提供商的信息 |

`provider`的配置字段说明如下：

| 名称                   | 数据类型               | 填写要求 | 默认值 | 描述                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| ---------------------- | ---------------------- | -------- | ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------   |
| `type`                 | string                 | 必填     | -      | AI 服务提供商名称                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `apiTokens`            | array of string        | 非必填   | -      | 用于在访问 AI 服务时进行认证的令牌。如果配置了多个 token，插件会在请求时随机进行选择。部分服务提供商只支持配置一个 token。                                                                                                                                                                                                                                                                                                                 |
| `timeout`              | number                 | 非必填   | -      | 访问 AI 服务的超时时间。单位为毫秒。默认值为 120000，即 2 分钟。此项配置目前仅用于获取上下文信息，并不影响实际转发大模型请求。                                                                                                                                                                                                                                                                                                             |
| `modelMapping`         | map of string          | 非必填   | -      | AI 模型映射表，用于将请求中的模型名称映射为服务提供商支持模型名称。<br/>1. 支持前缀匹配。例如用 "gpt-3-\*" 匹配所有名称以“gpt-3-”开头的模型；<br/>2. 支持使用 "\*" 为键来配置通用兜底映射关系；<br/>3. 如果映射的目标名称为空字符串 ""，则表示保留原模型名称。<br/>4. 支持以 `~` 前缀使用正则匹配。例如用 "~gpt(.\*)" 匹配所有以 "gpt" 开头的模型并支持在目标模型中使用 capture group 引用匹配到的内容。示例: "~gpt(.\*): openai/gpt\$1" |
| `protocol`             | string                 | 非必填   | -      | 插件对外提供的 API 接口契约。目前支持以下取值：openai（默认值，使用 OpenAI 的接口契约）、original（使用目标服务提供商的原始接口契约）                                                                                                                                                                                                                                                                                                      |
| `context`              | object                 | 非必填   | -      | 配置 AI 对话上下文信息                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `customSettings`       | array of customSetting | 非必填   | -      | 为 AI 请求指定覆盖或者填充参数                                                                                                                                                                                                                                                                                                                                                                                                             |
| `failover`             | object                 | 非必填   | -      | 配置 apiToken 的 failover 策略，当 apiToken 不可用时，将其移出 apiToken 列表，待健康检测通过后重新添加回 apiToken 列表                                                                                                                                                                                                                                                                                                                     |
| `retryOnFailure`       | object                 | 非必填   | -      | 当请求失败时立即进行重试                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `reasoningContentMode` | string                 | 非必填   | -      | 如何处理大模型服务返回的推理内容。目前支持以下取值：passthrough（正常输出推理内容）、ignore（不输出推理内容）、concat（将推理内容拼接在常规输出内容之前）。默认为 passthrough。仅支持通义千问服务。                                                                                                                                                                                                                                        |
| `capabilities`         | map of string          | 非必填   | -      | 部分 provider 的部分 ai 能力原生兼容 openai/v1 格式，不需要重写，可以直接转发，通过此配置项指定来开启转发, key 表示的是采用的厂商协议能力，values 表示的真实的厂商该能力的 api path, 厂商协议能力当前支持: openai/v1/chatcompletions, openai/v1/embeddings, openai/v1/imagegeneration, openai/v1/audiospeech, cohere/v1/rerank                                                                                                             |
| `subPath`              | string                 | 非必填   | -      | 如果配置了subPath，将会先移除请求path中该前缀，再进行后续处理                                                                                                                                                                                                                                                                                                                                                                              |
| `contextCleanupCommands` | array of string      | 非必填   | -      | 上下文清理命令列表。当请求的 messages 中存在完全匹配任意一个命令的 user 消息时，将该消息及之前所有非 system 消息清理掉，只保留 system 消息和该命令之后的消息。可用于主动清理对话上下文。                                                                                                                                                                                                                                                    |

`context`的配置字段说明如下：

| 名称          | 数据类型 | 填写要求 | 默认值 | 描述                                                     |
| ------------- | -------- | -------- | ------ | -------------------------------------------------------- |
| `fileUrl`     | string   | 必填     | -      | 保存 AI 对话上下文的文件 URL。仅支持纯文本类型的文件内容 |
| `serviceName` | string   | 必填     | -      | URL 所对应的 Higress 后端服务完整名称                    |
| `servicePort` | number   | 必填     | -      | URL 所对应的 Higress 后端服务访问端口                    |

`customSettings`的配置字段说明如下：

| 名称        | 数据类型              | 填写要求 | 默认值 | 描述                                                                                                                         |
| ----------- | --------------------- | -------- | ------ | ---------------------------------------------------------------------------------------------------------------------------- |
| `name`      | string                | 必填     | -      | 想要设置的参数的名称，例如`max_tokens`                                                                                       |
| `value`     | string/int/float/bool | 必填     | -      | 想要设置的参数的值，例如 0                                                                                                   |
| `mode`      | string                | 非必填   | "auto" | 参数设置的模式，可以设置为"auto"或者"raw"，如果为"auto"则会自动根据协议对参数名做改写，如果为"raw"则不会有任何改写和限制检查 |
| `overwrite` | bool                  | 非必填   | true   | 如果为 false 则只在用户没有设置这个参数时填充参数，否则会直接覆盖用户原有的参数设置                                          |

custom-setting 会遵循如下表格，根据`name`和协议来替换对应的字段，用户需要填写表格中`settingName`列中存在的值。例如用户将`name`设置为`max_tokens`，在 openai 协议中会替换`max_tokens`，在 gemini 中会替换`maxOutputTokens`。
`none`表示该协议不支持此参数。如果`name`不在此表格中或者对应协议不支持此参数，同时没有设置 raw 模式，则配置不会生效。

| settingName | openai      | baidu             | spark       | qwen        | gemini          | hunyuan     | claude      | minimax            |
| ----------- | ----------- | ----------------- | ----------- | ----------- | --------------- | ----------- | ----------- | ------------------ |
| max_tokens  | max_tokens  | max_output_tokens | max_tokens  | max_tokens  | maxOutputTokens | none        | max_tokens  | tokens_to_generate |
| temperature | temperature | temperature       | temperature | temperature | temperature     | Temperature | temperature | temperature        |
| top_p       | top_p       | top_p             | none        | top_p       | topP            | TopP        | top_p       | top_p              |
| top_k       | none        | none              | top_k       | none        | topK            | none        | top_k       | none               |
| seed        | seed        | none              | none        | seed        | none            | none        | none        | none               |

如果启用了 raw 模式，custom-setting 会直接用输入的`name`和`value`去更改请求中的 json 内容，而不对参数名称做任何限制和修改。
对于大多数协议，custom-setting 都会在 json 内容的根路径修改或者填充参数。对于`qwen`协议，ai-proxy 会在 json 的`parameters`子路径下做配置。对于`gemini`协议，则会在`generation_config`子路径下做配置。

`failover` 的配置字段说明如下：

| 名称                | 数据类型        | 填写要求             | 默认值         | 描述                                                     |
| ------------------- | --------------- | -------------------- | -------------- | -------------------------------------------------------- |
| enabled             | bool            | 非必填               | false          | 是否启用 apiToken 的 failover 机制                       |
| failureThreshold    | int             | 非必填               | 3              | 触发 failover 连续请求失败的阈值（次数）                 |
| successThreshold    | int             | 非必填               | 1              | 健康检测的成功阈值（次数）                               |
| healthCheckInterval | int             | 非必填               | 5000           | 健康检测的间隔时间，单位毫秒                             |
| healthCheckTimeout  | int             | 非必填               | 5000           | 健康检测的超时时间，单位毫秒                             |
| healthCheckModel    | string          | 启用 failover 时必填 |                | 健康检测使用的模型                                       |
| failoverOnStatus    | array of string | 非必填               | ["4.*", "5.*"] | 需要进行 failover 的原始请求的状态码，支持正则表达式匹配 |

`retryOnFailure` 的配置字段说明如下：

目前仅支持对非流式请求进行重试。

| 名称          | 数据类型        | 填写要求 | 默认值         | 描述                                               |
| ------------- | --------------- | -------- | -------------- | -------------------------------------------------- |
| enabled       | bool            | 非必填   | false          | 是否启用失败请求重试                               |
| maxRetries    | int             | 非必填   | 1              | 最大重试次数                                       |
| retryTimeout  | int             | 非必填   | 30000          | 重试超时时间，单位毫秒                             |
| retryOnStatus | array of string | 非必填   | ["4.*", "5.*"] | 需要进行重试的原始请求的状态码，支持正则表达式匹配 |

### 提供商特有配置

#### OpenAI

OpenAI 所对应的 `type` 为 `openai`。它特有的配置字段如下:

| 名称                 | 数据类型 | 填写要求 | 默认值 | 描述                                                                               |
| -------------------- | -------- | -------- | ------ | ---------------------------------------------------------------------------------- |
| `openaiCustomUrl`    | string   | 非必填   | -      | 基于 OpenAI 协议的自定义后端 URL，例如: <www.example.com/myai/v1/chat/completions> |
| `responseJsonSchema` | object   | 非必填   | -      | 预先定义 OpenAI 响应需满足的 Json Schema, 注意目前仅特定的几种模型支持该用法       |

#### Azure OpenAI

Azure OpenAI 所对应的 `type` 为 `azure`。它特有的配置字段如下：

| 名称              | 数据类型 | 填写要求 | 默认值 | 描述                                                                                                   |
| ----------------- | -------- | -------- | ------ | ------------------------------------------------------------------------------------------------------ |
| `azureServiceUrl` | string   | 必填     | -      | Azure OpenAI 服务的 URL。`/openai/v1` 新版路径无需日期型 `api-version`；legacy 路径或仅资源名称模式仍须包含。 |

**注意：**
1. Azure OpenAI 只支持配置一个 API Token。
2. `azureServiceUrl` 支持新版 `/openai/v1` 和 legacy 配置格式：
   1. 新版 v1 格式，例如：`https://YOUR_RESOURCE_NAME.openai.azure.com/openai/v1`
      - 插件会直接使用该 v1 base URL，且不会自动追加日期型 `api-version`。
   2. Legacy 完整路径格式，例如：`https://YOUR_RESOURCE_NAME.openai.azure.com/openai/deployments/YOUR_DEPLOYMENT_NAME/chat/completions?api-version=2024-02-15-preview`
      - 插件会直接将请求转发至该 URL，不会参考实际的请求路径。
   3. Legacy 部署名称格式，例如：`https://YOUR_RESOURCE_NAME.openai.azure.com/openai/deployments/YOUR_DEPLOYMENT_NAME?api-version=2024-02-15-preview`
      - 插件会根据实际的请求路径拼接后续路径。路径中的部署名称会保留不变，不会按照模型映射规则进行修改。同时支持 URL 中不包含部署名称的接口。
   4. Legacy 资源名称格式，例如：`https://YOUR_RESOURCE_NAME.openai.azure.com?api-version=2024-02-15-preview`
      - 插件会根据实际的请求路径拼接后续路径。路径中的部署名称会根据请求中的模型名称结合模型映射规则进行填入。同时支持 URL 中不包含部署名称的接口。

#### 月之暗面（Moonshot）

月之暗面所对应的 `type` 为 `moonshot`。它特有的配置字段如下：

| 名称             | 数据类型 | 填写要求 | 默认值 | 描述                                                                                                 |
| ---------------- | -------- | -------- | ------ | ---------------------------------------------------------------------------------------------------- |
| `moonshotFileId` | string   | 非必填   | -      | 通过文件接口上传至月之暗面的文件 ID，其内容将被用做 AI 对话的上下文。不可与 `context` 字段同时配置。 |

#### 通义千问（Qwen）

通义千问所对应的 `type` 为 `qwen`。它特有的配置字段如下：

| 名称                   | 数据类型        | 填写要求 | 默认值 | 描述                                                                                                    |
| ---------------------- | --------------- | -------- | ------ | ------------------------------------------------------------------------------------------------------- |
| `qwenEnableSearch`     | boolean         | 非必填   | -      | 是否启用通义千问内置的互联网搜索功能。                                                                  |
| `qwenFileIds`          | array of string | 非必填   | -      | 通过文件接口上传至 Dashscope 的文件 ID，其内容将被用做 AI 对话的上下文。不可与 `context` 字段同时配置。 |
| `qwenEnableCompatible` | boolean         | 非必填   | false  | 开启通义千问兼容模式。启用通义千问兼容模式后，将调用千问的兼容模式接口，同时对请求/响应不做修改。       |

#### 百川智能 (Baichuan AI)

百川智能所对应的 `type` 为 `baichuan` 。它并无特有的配置字段。

#### 零一万物（Yi）

零一万物所对应的 `type` 为 `yi`。它并无特有的配置字段。

#### 智谱 AI（Zhipu AI）

智谱 AI 所对应的 `type` 为 `zhipuai`。它并无特有的配置字段。

#### DeepSeek（DeepSeek）

DeepSeek 所对应的 `type` 为 `deepseek`。它并无特有的配置字段。

#### Groq

Groq 所对应的 `type` 为 `groq`。它并无特有的配置字段。

#### Grok

Grok 所对应的 `type` 为 `grok`。它并无特有的配置字段。

#### OpenRouter

OpenRouter 所对应的 `type` 为 `openrouter`。它并无特有的配置字段。

#### Fireworks AI

Fireworks AI 所对应的 `type` 为 `fireworks`。它并无特有的配置字段。

#### 文心一言（Baidu）

文心一言所对应的 `type` 为 `baidu`。它并无特有的配置字段。

#### 360 智脑

360 智脑所对应的 `type` 为 `ai360`。它并无特有的配置字段。

#### GitHub 模型

GitHub 模型所对应的 `type` 为 `github`。它并无特有的配置字段。

#### Mistral

Mistral 所对应的 `type` 为 `mistral`。它并无特有的配置字段。

#### MiniMax

MiniMax 所对应的 `type` 为 `minimax`。它特有的配置字段如下：

| 名称             | 数据类型 | 填写要求                       | 默认值 | 描述                                                                    |
| ---------------- | -------- | ------------------------------ | ------ | ----------------------------------------------------------------------- |
| `minimaxApiType` | string   | v2 和 pro 中选填一项           | v2     | v2 代表 ChatCompletion v2 API，pro 代表 ChatCompletion Pro API          |
| `minimaxGroupId` | string   | `minimaxApiType` 为 pro 时必填 | -      | `minimaxApiType` 为 pro 时使用 ChatCompletion Pro API，需要设置 groupID |

#### Anthropic Claude

Anthropic Claude 所对应的 `type` 为 `claude`。它特有的配置字段如下：

| 名称            | 数据类型 | 填写要求 | 默认值 | 描述                                      |
| --------------- | -------- | -------- | ------ | ----------------------------------------- |
| `claudeVersion` | string   | 可选     | -      | Claude 服务的 API 版本，默认为 2023-06-01 |
| `claudeCodeMode` | boolean | 可选     | false  | 启用 Claude Code 模式，用于支持 Claude Code OAuth 令牌认证。启用后将伪装成 Claude Code 客户端发起请求 |

**Claude Code 模式说明**

启用 `claudeCodeMode: true` 时，插件将：
- 使用 Bearer Token 认证替代 x-api-key（适配 Claude Code OAuth 令牌）
- 设置 Claude Code 特定的请求头（user-agent、x-app、anthropic-beta）
- 为请求 URL 添加 `?beta=true` 查询参数
- 自动注入 Claude Code 的系统提示词（如未提供）

这允许在 Higress 中直接使用 Claude Code 的 OAuth Token 进行身份验证。

#### Ollama

Ollama 所对应的 `type` 为 `ollama`。它特有的配置字段如下：

| 名称               | 数据类型 | 填写要求 | 默认值 | 描述                                |
| ------------------ | -------- | -------- | ------ | ----------------------------------- |
| `ollamaServerHost` | string   | 必填     | -      | Ollama 服务器的主机地址             |
| `ollamaServerPort` | number   | 必填     | -      | Ollama 服务器的端口号，默认为 11434 |

#### 通用代理（Generic）

当只需要借助 AI Proxy 的鉴权、basePath 处理或首包超时能力，且不希望插件改写路径时，可将 `provider.type` 设置为 `generic`。该 Provider 不绑定任何模型厂商，也不会做能力映射。

| 名称          | 数据类型 | 填写要求 | 默认值 | 描述                                                                 |
| ------------- | -------- | -------- | ------ | -------------------------------------------------------------------- |
| `genericHost` | string   | 非必填   | -      | 指定要转发到的目标 Host；未配置时沿用客户端请求的 Host。 |

- 配置了 `apiTokens` 时，会自动写入 `Authorization: Bearer <token>` 请求头，复用全局的 Token 轮询能力。
- 当配置了 `firstByteTimeout` 时，会自动注入 `x-envoy-upstream-rq-first-byte-timeout-ms`。
- `basePath` 与 `basePathHandling` 同样适用，可在通用转发中快捷地移除或添加统一前缀。

#### 混元

混元所对应的 `type` 为 `hunyuan`。它特有的配置字段如下：

| 名称             | 数据类型 | 填写要求 | 默认值 | 描述                       |
| ---------------- | -------- | -------- | ------ | -------------------------- |
| `hunyuanAuthId`  | string   | 必填     | -      | 混元用于 v3 版本认证的 id  |
| `hunyuanAuthKey` | string   | 必填     | -      | 混元用于 v3 版本认证的 key |

#### 阶跃星辰 (Stepfun)

阶跃星辰所对应的 `type` 为 `stepfun`。它并无特有的配置字段。

#### Cloudflare Workers AI

Cloudflare Workers AI 所对应的 `type` 为 `cloudflare`。它特有的配置字段如下：

| 名称                  | 数据类型 | 填写要求 | 默认值 | 描述                                                                                                                       |
| --------------------- | -------- | -------- | ------ | -------------------------------------------------------------------------------------------------------------------------- |
| `cloudflareAccountId` | string   | 必填     | -      | [Cloudflare Account ID](https://developers.cloudflare.com/workers-ai/get-started/rest-api/#1-get-api-token-and-account-id) |

#### 星火 (Spark)

星火所对应的 `type` 为 `spark`。它并无特有的配置字段。

讯飞星火认知大模型的`apiTokens`字段值为`APIKey:APISecret`。即填入自己的 APIKey 与 APISecret，并以`:`分隔。

#### Gemini

Gemini 所对应的 `type` 为 `gemini`。它特有的配置字段如下：

| 名称                   | 数据类型      | 填写要求 | 默认值   | 描述                                                         |
| ---------------------- | ------------- | -------- | -------- | ------------------------------------------------------------ |
| `geminiSafetySetting`  | map of string | 非必填   | -        | Gemini AI 内容过滤和安全级别设定。参考[Safety settings](https://ai.google.dev/gemini-api/docs/safety-settings) |
| `apiVersion`           | string        | 非必填   | `v1beta` | 用于指定 API 的版本, 可选择 `v1` 或 `v1beta` 。 版本差异请参考[API versions explained](https://ai.google.dev/gemini-api/docs/api-versions)。 |
| `geminiThinkingBudget` | number        | 非必填   | -        | gemini2.5系列的参数，0是不开启思考，-1动态调整，具体参数指可参考官网 |

#### DeepL

DeepL 所对应的 `type` 为 `deepl`。它特有的配置字段如下：

| 名称         | 数据类型 | 填写要求 | 默认值 | 描述                         |
| ------------ | -------- | -------- | ------ | ---------------------------- |
| `targetLang` | string   | 必填     | -      | DeepL 翻译服务需要的目标语种 |

#### Cohere

Cohere 所对应的 `type` 为 `cohere`。它并无特有的配置字段。

#### Together-AI

Together-AI 所对应的 `type` 为 `together-ai`。它并无特有的配置字段。

#### Dify

Dify 所对应的 `type` 为 `dify`。它特有的配置字段如下:

| 名称             | 数据类型 | 填写要求 | 默认值 | 描述                                                                             |
| ---------------- | -------- | -------- | ------ | -------------------------------------------------------------------------------- |
| `difyApiUrl`     | string   | 非必填   | -      | dify 私有化部署的 url                                                            |
| `botType`        | string   | 非必填   | -      | dify 的应用类型，Chat/Completion/Agent/Workflow                                  |
| `inputVariable`  | string   | 非必填   | -      | dify 中应用类型为 workflow 时需要设置输入变量，当 botType 为 workflow 时一起使用 |
| `outputVariable` | string   | 非必填   | -      | dify 中应用类型为 workflow 时需要设置输出变量，当 botType 为 workflow 时一起使用 |

#### Google Vertex AI

Google Vertex AI 所对应的 type 为 vertex。支持两种认证模式：

**标准模式**（使用 Service Account）：

| 名称                         | 数据类型       | 填写要求   | 默认值    | 描述                                                                            |
|-----------------------------|---------------|--------|--------|-------------------------------------------------------------------------------|
| `vertexAuthKey`             | string        | 必填     | -      | 用于认证的 Google Service Account JSON Key，格式为 PEM 编码的 PKCS#8 私钥和 client_email 等信息 |
| `vertexRegion`              | string        | 必填     | -      | Google Cloud 区域（如 us-central1, europe-west4 等），用于构建 Vertex API 地址             |
| `vertexProjectId`           | string        | 必填     | -      | Google Cloud 项目 ID，用于标识目标 GCP 项目                                              |
| `vertexAuthServiceName`     | string        | 必填     | -      | 用于 OAuth2 认证的服务名称，该服务为了访问oauth2.googleapis.com                                |
| `geminiSafetySetting`       | map of string | 非必填   | -      | Gemini AI 内容过滤和安全级别设定。参考[Safety settings](https://ai.google.dev/gemini-api/docs/safety-settings)                             |
| `vertexTokenRefreshAhead`   | number        | 非必填   | -      | Vertex access token刷新提前时间(单位秒)                                                |

**Express Mode**（使用 API Key，简化配置）：

Express Mode 是 Vertex AI 推出的简化访问模式，只需 API Key 即可快速开始使用，无需配置 Service Account。详见 [Vertex AI Express Mode 文档](https://cloud.google.com/vertex-ai/generative-ai/docs/start/express-mode/overview)。

| 名称                         | 数据类型       | 填写要求   | 默认值    | 描述                                                                            |
|-----------------------------|---------------|--------|--------|-------------------------------------------------------------------------------|
| `apiTokens`                 | array of string | 必填   | -      | Express Mode 使用的 API Key，从 Google Cloud Console 的 API & Services > Credentials 获取 |
| `geminiSafetySetting`       | map of string | 非必填   | -      | Gemini AI 内容过滤和安全级别设定。参考[Safety settings](https://ai.google.dev/gemini-api/docs/safety-settings)                             |

**OpenAI 兼容模式**（使用 Vertex AI Chat Completions API）：

Vertex AI 提供了 OpenAI 兼容的 Chat Completions API 端点，可以直接使用 OpenAI 格式的请求和响应，无需进行协议转换。详见 [Vertex AI OpenAI 兼容性文档](https://cloud.google.com/vertex-ai/generative-ai/docs/migrate/openai/overview)。

| 名称                         | 数据类型       | 填写要求   | 默认值    | 描述                                                                            |
|-----------------------------|---------------|--------|--------|-------------------------------------------------------------------------------|
| `vertexOpenAICompatible`    | boolean       | 非必填   | false  | 启用 OpenAI 兼容模式。启用后将使用 Vertex AI 的 OpenAI-compatible Chat Completions API |
| `vertexAuthKey`             | string        | 必填     | -      | 用于认证的 Google Service Account JSON Key |
| `vertexRegion`              | string        | 必填     | -      | Google Cloud 区域（如 us-central1, europe-west4 等） |
| `vertexProjectId`           | string        | 必填     | -      | Google Cloud 项目 ID |
| `vertexAuthServiceName`     | string        | 必填     | -      | 用于 OAuth2 认证的服务名称 |

**注意**：OpenAI 兼容模式与 Express Mode 互斥，不能同时配置 `apiTokens` 和 `vertexOpenAICompatible`。

#### AWS Bedrock

AWS Bedrock 所对应的 type 为 bedrock。它支持两种认证方式：

1. **AWS Signature V4 认证**：使用 `awsAccessKey` 和 `awsSecretKey` 进行 AWS 标准签名认证
2. **Bearer Token 认证**：使用 `apiTokens` 配置 AWS Bearer Token（适用于 IAM Identity Center 等场景）

**注意**：两种认证方式二选一，如果同时配置了 `apiTokens`，将优先使用 Bearer Token 认证方式。

它特有的配置字段如下：

| 名称                        | 数据类型        | 填写要求            | 默认值 | 描述                                                |
|---------------------------|---------------|-------------------|-------|---------------------------------------------------|
| `apiTokens`               | array of string | 与 ak/sk 二选一   | -     | AWS Bearer Token，用于 Bearer Token 认证方式          |
| `awsAccessKey`            | string        | 与 apiTokens 二选一 | -     | AWS Access Key，用于 AWS Signature V4 认证            |
| `awsSecretKey`            | string        | 与 apiTokens 二选一 | -     | AWS Secret Access Key，用于 AWS Signature V4 认证     |
| `awsRegion`               | string        | 必填              | -     | AWS 区域，例如：us-east-1                              |
| `bedrockAdditionalFields` | map           | 非必填            | -     | Bedrock 额外模型请求参数                               |

#### NVIDIA Triton Interference Server

NVIDIA Triton Interference Server 所对应的 type 为 triton。它特有的配置字段如下：

| 名称                   | 数据类型 | 填写要求 | 默认值 | 描述                                      |
|----------------------|--------|--------|-------|------------------------------------------|
| `tritonModelVersion` | string | 非必填   | -     | 用于指定 Triton Server 中 model version     |
| `tritonDomain`       | string | 非必填   | -     | Triton Server 部署的指定请求 Domain          |

## 用法示例

### 使用 OpenAI 协议代理 Azure OpenAI 服务

使用最基本的 Azure OpenAI 服务，不配置任何上下文。

**配置信息**

```yaml
provider:
  type: azure
  apiTokens:
    - "YOUR_AZURE_OPENAI_API_TOKEN"
  azureServiceUrl: "https://YOUR_RESOURCE_NAME.openai.azure.com/openai/v1",
```

**请求示例**

```json
{
  "model": "gpt-3",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "temperature": 0.3
}
```

**响应示例**

```json
{
  "choices": [
    {
      "content_filter_results": {
        "hate": {
          "filtered": false,
          "severity": "safe"
        },
        "self_harm": {
          "filtered": false,
          "severity": "safe"
        },
        "sexual": {
          "filtered": false,
          "severity": "safe"
        },
        "violence": {
          "filtered": false,
          "severity": "safe"
        }
      },
      "finish_reason": "stop",
      "index": 0,
      "logprobs": null,
      "message": {
        "content": "你好！我是一个AI助手，可以回答你的问题和提供帮助。有什么我可以帮到你的吗？",
        "role": "assistant"
      }
    }
  ],
  "created": 1714807624,
  "id": "chatcmpl-abcdefg1234567890",
  "model": "gpt-35-turbo-16k",
  "object": "chat.completion",
  "prompt_filter_results": [
    {
      "prompt_index": 0,
      "content_filter_results": {
        "hate": {
          "filtered": false,
          "severity": "safe"
        },
        "self_harm": {
          "filtered": false,
          "severity": "safe"
        },
        "sexual": {
          "filtered": false,
          "severity": "safe"
        },
        "violence": {
          "filtered": false,
          "severity": "safe"
        }
      }
    }
  ],
  "system_fingerprint": null,
  "usage": {
    "completion_tokens": 40,
    "prompt_tokens": 15,
    "total_tokens": 55
  }
}
```

### 使用 OpenAI 协议代理通义千问服务

使用通义千问服务，并配置从 OpenAI 大模型到通义千问的模型映射关系。

**配置信息**

```yaml
provider:
  type: qwen
  apiTokens:
    - 'YOUR_QWEN_API_TOKEN'
  modelMapping:
    'gpt-3': 'qwen-turbo'
    'gpt-35-turbo': 'qwen-plus'
    'gpt-4-turbo': 'qwen-max'
    'gpt-4-*': 'qwen-max'
    'gpt-4o': 'qwen-vl-plus'
    'text-embedding-v1': 'text-embedding-v1'
    '*': 'qwen-turbo'
```

**AI 对话请求示例**

URL: <http://your-domain/v1/chat/completions>

请求示例：

```json
{
  "model": "gpt-3",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "temperature": 0.3
}
```

响应示例：

```json
{
  "id": "c2518bd3-0f46-97d1-be34-bb5777cb3108",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "我是通义千问，由阿里云开发的AI助手。我可以回答各种问题、提供信息和与用户进行对话。有什么我可以帮助你的吗？"
      },
      "finish_reason": "stop"
    }
  ],
  "created": 1715175072,
  "model": "qwen-turbo",
  "object": "chat.completion",
  "usage": {
    "prompt_tokens": 24,
    "completion_tokens": 33,
    "total_tokens": 57
  }
}
```

**多模态模型 API 请求示例（适用于 `qwen-vl-plus` 和 `qwen-vl-max` 模型）**

URL: <http://your-domain/v1/chat/completions>

请求示例：

```json
{
  "model": "gpt-4o",
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "image_url",
          "image_url": {
            "url": "https://dashscope.oss-cn-beijing.aliyuncs.com/images/dog_and_girl.jpeg"
          }
        },
        {
          "type": "text",
          "text": "这个图片是哪里？"
        }
      ]
    }
  ],
  "temperature": 0.3
}
```

响应示例：

```json
{
  "id": "17c5955d-af9c-9f28-bbde-293a9c9a3515",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": [
          {
            "text": "这张照片显示的是一位女士和一只狗在海滩上。由于我无法获取具体的地理位置信息，所以不能确定这是哪个地方的海滩。但是从视觉内容来看，它可能是一个位于沿海地区的沙滩海岸线，并且有海浪拍打着岸边。这样的场景在全球许多美丽的海滨地区都可以找到。如果您需要更精确的信息，请提供更多的背景或细节描述。"
          }
        ]
      },
      "finish_reason": "stop"
    }
  ],
  "created": 1723949230,
  "model": "qwen-vl-plus",
  "object": "chat.completion",
  "usage": {
    "prompt_tokens": 1279,
    "completion_tokens": 78
  }
}
```

**文本向量请求示例**

URL: <http://your-domain/v1/embeddings>

请求示例：

```json
{
  "model": "text-embedding-v1",
  "input": "Hello"
}
```

响应示例：

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "index": 0,
      "embedding": [
        -1.0437825918197632,
        5.208984375,
        3.0483806133270264,
        -1.7897135019302368,
        -2.0107421875,
        ...,
        0.8125,
        -1.1759847402572632,
        0.8174641728401184,
        1.0432943105697632,
        -0.5885213017463684
      ]
    }
  ],
  "model": "text-embedding-v1",
  "usage": {
    "prompt_tokens": 1,
    "total_tokens": 1
  }
}
```

### 使用通义千问配合纯文本上下文信息

使用通义千问服务，同时配置纯文本上下文信息。

**配置信息**

```yaml
provider:
  type: qwen
  apiTokens:
    - "YOUR_QWEN_API_TOKEN"
  modelMapping:
    "*": "qwen-turbo"
  context:
    - fileUrl: "http://file.default.svc.cluster.local/ai/context.txt",
      serviceName: "file.dns",
      servicePort: 80
```

**请求示例**

```json
{
  "model": "gpt-3",
  "messages": [
    {
      "role": "user",
      "content": "请概述文案内容"
    }
  ],
  "temperature": 0.3
}
```

**响应示例**

```json
{
  "id": "cmpl-77861a17681f4987ab8270dbf8001936",
  "object": "chat.completion",
  "created": 9756990,
  "model": "moonshot-v1-128k",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "这份文案是一份关于..."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 20181,
    "completion_tokens": 439,
    "total_tokens": 20620
  }
}
```

### 使用通义千问配合其原生的文件上下文

提前上传文件至通义千问，以文件内容作为上下文使用其 AI 服务。

**配置信息**

```yaml
provider:
  type: qwen
  apiTokens:
    - 'YOUR_QWEN_API_TOKEN'
  modelMapping:
    '*': 'qwen-long' # 通义千问的文件上下文只能在 qwen-long 模型下使用
  qwenFileIds:
    - 'file-fe-xxx'
    - 'file-fe-yyy'
```

**请求示例**

```json
{
  "model": "gpt-4-turbo",
  "messages": [
    {
      "role": "user",
      "content": "请概述文案内容"
    }
  ],
  "temperature": 0.3
}
```

**响应示例**

```json
{
  "output": {
    "choices": [
      {
        "finish_reason": "stop",
        "message": {
          "role": "assistant",
          "content": "您上传了两个文件，`context.txt` 和 `context_2.txt`，它们似乎都包含了关于xxxx"
        }
      }
    ]
  },
  "usage": {
    "total_tokens": 2023,
    "output_tokens": 530,
    "input_tokens": 1493
  },
  "request_id": "187e99ba-5b64-9ffe-8f69-01dafbaf6ed7"
}
```

### 使用 original 协议代理百炼智能体应用

**配置信息**

```yaml
provider:
  type: qwen
  apiTokens:
    - 'YOUR_DASHSCOPE_API_TOKEN'
  protocol: original
```

**请求实例**

```json
{
  "input": {
    "prompt": "介绍一下Dubbo"
  },
  "parameters": {},
  "debug": {}
}
```

**响应实例**

```json
{
  "output": {
    "finish_reason": "stop",
    "session_id": "677e7e8fbb874e1b84792b65042e1599",
    "text": "Apache Dubbo 是一个..."
  },
  "usage": {
    "models": [
      {
        "output_tokens": 449,
        "model_id": "qwen-max",
        "input_tokens": 282
      }
    ]
  },
  "request_id": "b59e45e3-5af4-91df-b7c6-9d746fd3297c"
}
```

### 使用 OpenAI 协议代理豆包大模型服务

**配置信息**

```yaml
provider:
  type: doubao
  apiTokens:
    - YOUR_DOUBAO_API_KEY
  modelMapping:
    '*': YOUR_DOUBAO_ENDPOINT
  timeout: 1200000
```

### 使用 original 协议代理 Coze 应用

**配置信息**

```yaml
provider:
  type: coze
  apiTokens:
    - YOUR_COZE_API_KEY
  protocol: original
```

### 使用月之暗面配合其原生的文件上下文

提前上传文件至月之暗面，以文件内容作为上下文使用其 AI 服务。

**配置信息**

```yaml
provider:
  type: moonshot
  apiTokens:
    - "YOUR_MOONSHOT_API_TOKEN"
  moonshotFileId: "YOUR_MOONSHOT_FILE_ID",
  modelMapping:
    '*': "moonshot-v1-32k"
```

**请求示例**

```json
{
  "model": "gpt-4-turbo",
  "messages": [
    {
      "role": "user",
      "content": "请概述文案内容"
    }
  ],
  "temperature": 0.3
}
```

**响应示例**

```json
{
  "id": "cmpl-e5ca873642ca4f5d8b178c1742f9a8e8",
  "object": "chat.completion",
  "created": 1872961,
  "model": "moonshot-v1-128k",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "文案内容是关于一个名为“xxxx”的支付平台..."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 11,
    "completion_tokens": 498,
    "total_tokens": 509
  }
}
```

### 使用 OpenAI 协议代理 Groq 服务

**配置信息**

```yaml
provider:
  type: groq
  apiTokens:
    - 'YOUR_GROQ_API_TOKEN'
```

**请求示例**

```json
{
  "model": "llama3-8b-8192",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ]
}
```

**响应示例**

```json
{
  "id": "chatcmpl-26733989-6c52-4056-b7a9-5da791bd7102",
  "object": "chat.completion",
  "created": 1715917967,
  "model": "llama3-8b-8192",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "😊 Ni Hao! (That's \"hello\" in Chinese!)\n\nI am LLaMA, an AI assistant developed by Meta AI that can understand and respond to human input in a conversational manner. I'm not a human, but a computer program designed to simulate conversations and answer questions to the best of my ability. I'm happy to chat with you in Chinese or help with any questions or topics you'd like to discuss! 😊"
      },
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 16,
    "prompt_time": 0.005,
    "completion_tokens": 89,
    "completion_time": 0.104,
    "total_tokens": 105,
    "total_time": 0.109
  },
  "system_fingerprint": "fp_dadc9d6142",
  "x_groq": {
    "id": "req_01hy2awmcxfpwbq56qh6svm7qz"
  }
}
```

### 使用 OpenAI 协议代理 Grok 服务

**配置信息**

```yaml
provider:
  type: grok
  apiTokens:
    - 'YOUR_GROK_API_TOKEN'
```

**请求示例**

```json
{
  "messages": [
    {
      "role": "system",
      "content": "You are a helpful assistant that can answer questions and help with tasks."
    },
    {
      "role": "user",
      "content": "What is 101*3?"
    }
  ],
  "model": "grok-4"
}
```

**响应示例**

```json
{
  "id": "a3d1008e-4544-40d4-d075-11527e794e4a",
  "object": "chat.completion",
  "created": 1752854522,
  "model": "grok-4",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "101 multiplied by 3 is 303.",
        "refusal": null
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 32,
    "completion_tokens": 9,
    "total_tokens": 135,
    "prompt_tokens_details": {
      "text_tokens": 32,
      "audio_tokens": 0,
      "image_tokens": 0,
      "cached_tokens": 6
    },
    "completion_tokens_details": {
      "reasoning_tokens": 94,
      "audio_tokens": 0,
      "accepted_prediction_tokens": 0,
      "rejected_prediction_tokens": 0
    },
    "num_sources_used": 0
  },
  "system_fingerprint": "fp_3a7881249c"
}
```

### 使用 OpenAI 协议代理 OpenRouter 服务

**配置信息**

```yaml
provider:
  type: openrouter
  apiTokens:
    - 'YOUR_OPENROUTER_API_TOKEN'
  modelMapping:
    'gpt-4': 'openai/gpt-4-turbo-preview'
    'gpt-3.5-turbo': 'openai/gpt-3.5-turbo'
    'claude-3': 'anthropic/claude-3-opus'
    '*': 'openai/gpt-3.5-turbo'
```

**请求示例**

```json
{
  "model": "gpt-4",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "temperature": 0.7
}
```

**响应示例**

```json
{
  "id": "gen-1234567890abcdef",
  "object": "chat.completion",
  "created": 1699123456,
  "model": "openai/gpt-4-turbo-preview",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "你好！我是一个AI助手，通过OpenRouter平台提供服务。我可以帮助回答问题、协助创作、进行对话等。有什么我可以帮助你的吗？"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 12,
    "completion_tokens": 46,
    "total_tokens": 58
  }
}
```

### 使用 OpenAI 协议代理 Fireworks AI 服务

**配置信息**

```yaml
provider:
  type: fireworks
  apiTokens:
    - "YOUR_FIREWORKS_API_TOKEN"
  modelMapping:
    "gpt-4": "accounts/fireworks/models/llama-v3p1-70b-instruct"
    "gpt-3.5-turbo": "accounts/fireworks/models/llama-v3p1-8b-instruct"
    "*": "accounts/fireworks/models/llama-v3p1-8b-instruct"
```

**请求示例**

```json
{
  "model": "gpt-4",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "temperature": 0.7,
  "max_tokens": 100
}
```

**响应示例**

```json
{
  "id": "fw-123456789",
  "object": "chat.completion",
  "created": 1699123456,
  "model": "accounts/fireworks/models/llama-v3p1-70b-instruct",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "你好！我是一个由 Fireworks AI 提供的人工智能助手，基于 Llama 3.1 模型。我可以帮助回答问题、进行对话和提供各种信息。有什么我可以帮助你的吗？"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 15,
    "completion_tokens": 45,
    "total_tokens": 60
  }
}
```

### 使用自动协议兼容功能

插件现在支持自动协议检测，可以同时处理 OpenAI 和 Claude 两种协议格式的请求。

**配置信息**

```yaml
provider:
  type: claude  # 原生支持 Claude 协议的供应商
  apiTokens:
    - 'YOUR_CLAUDE_API_TOKEN'
  version: '2023-06-01'
```

**OpenAI 协议请求示例**

URL: `http://your-domain/v1/chat/completions`

```json
{
  "model": "claude-3-opus-20240229",
  "max_tokens": 1024,
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ]
}
```

**Claude 协议请求示例**

URL: `http://your-domain/v1/messages`

```json
{
  "model": "claude-3-opus-20240229",
  "max_tokens": 1024,
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ]
}
```

**响应示例**

两种协议格式的请求都会返回相应格式的响应：

```json
{
  "id": "msg_01Jt3GzyjuzymnxmZERJguLK",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "您好,我是一个由人工智能公司Anthropic开发的聊天助手。我的名字叫Claude,是一个聪明友善、知识渊博的对话系统。很高兴认识您!我可以就各种话题与您聊天,回答问题,提供建议和帮助。我会尽最大努力给您有帮助的回复。希望我们能有个愉快的交流!"
      },
      "finish_reason": "stop"
    }
  ],
  "created": 1717385918,
  "model": "claude-3-opus-20240229",
  "object": "chat.completion",
  "usage": {
    "prompt_tokens": 16,
    "completion_tokens": 126,
    "total_tokens": 142
  }
}
```

### 使用 Claude Code 模式

Claude Code 是 Anthropic 提供的官方 CLI 工具。通过启用 `claudeCodeMode`，可以使用 Claude Code 的 OAuth Token 进行身份验证：

**配置信息**

```yaml
provider:
  type: claude
  apiTokens:
    - 'sk-ant-oat01-xxxxx'  # Claude Code OAuth Token
  claudeCodeMode: true  # 启用 Claude Code 模式
```

启用此模式后，插件将自动：
- 使用 Bearer Token 认证（而非 x-api-key）
- 设置 Claude Code 特定的请求头和查询参数
- 注入 Claude Code 的系统提示词（如未提供）

**请求示例**

```json
{
  "model": "claude-sonnet-4-5-20250929",
  "max_tokens": 8192,
  "messages": [
    {
      "role": "user",
      "content": "List files in current directory"
    }
  ]
}
```

插件将自动转换为适合 Claude Code 的请求格式，包括：
- 添加系统提示词：`"You are Claude Code, Anthropic's official CLI for Claude."`
- 设置适当的认证和请求头

### 使用智能协议转换

当目标供应商不原生支持 Claude 协议时，插件会自动进行协议转换：

**配置信息**

```yaml
provider:
  type: qwen  # 不原生支持 Claude 协议，会自动转换
  apiTokens:
    - 'YOUR_QWEN_API_TOKEN'
  modelMapping:
    'claude-3-opus-20240229': 'qwen-max'
    '*': 'qwen-turbo'
```

**Claude 协议请求**

URL: `http://your-domain/v1/messages` (自动转换为 OpenAI 协议调用供应商)

```json
{
  "model": "claude-3-opus-20240229",
  "max_tokens": 1024,
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ]
}
```

### 使用 OpenAI 协议代理混元服务

**配置信息**

```yaml
provider:
  type: 'hunyuan'
  hunyuanAuthKey: '<YOUR AUTH KEY>'
  apiTokens:
    - ''
  hunyuanAuthId: '<YOUR AUTH ID>'
  timeout: 1200000
  modelMapping:
    '*': 'hunyuan-lite'
```

**请求示例**

请求脚本：

```shell
curl --location 'http://<your higress domain>/v1/chat/completions' \
--header 'Content-Type:  application/json' \
--data '{
  "model": "gpt-3",
  "messages": [
    {
      "role": "system",
      "content": "你是一个名专业的开发人员！"
    },
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "temperature": 0.3,
  "stream": false
}'
```

**响应示例**

```json
{
  "id": "fd140c3e-0b69-4b19-849b-d354d32a6162",
  "choices": [
    {
      "index": 0,
      "delta": {
        "role": "assistant",
        "content": "你好！我是一名专业的开发人员。"
      },
      "finish_reason": "stop"
    }
  ],
  "created": 1717493117,
  "model": "hunyuan-lite",
  "object": "chat.completion",
  "usage": {
    "prompt_tokens": 15,
    "completion_tokens": 9,
    "total_tokens": 24
  }
}
```

### 使用 OpenAI 协议代理百度文心一言服务

**配置信息**

```yaml
provider:
  type: baidu
  apiTokens:
    - 'YOUR_BAIDU_API_TOKEN'
  modelMapping:
    'gpt-3': 'ERNIE-4.0'
    '*': 'ERNIE-4.0'
```

**请求示例**

```json
{
  "model": "gpt-4-turbo",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "stream": false
}
```

**响应示例**

```json
{
  "id": "as-e90yfg1pk1",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "你好，我是文心一言，英文名是ERNIE Bot。我能够与人对话互动，回答问题，协助创作，高效便捷地帮助人们获取信息、知识和灵感。"
      },
      "finish_reason": "stop"
    }
  ],
  "created": 1717251488,
  "model": "ERNIE-4.0",
  "object": "chat.completion",
  "usage": {
    "prompt_tokens": 4,
    "completion_tokens": 33,
    "total_tokens": 37
  }
}
```

### 使用 OpenAI 协议代理 MiniMax 服务

**配置信息**

```yaml
provider:
  type: minimax
  apiTokens:
    - 'YOUR_MINIMAX_API_TOKEN'
  modelMapping:
    'gpt-3': 'abab6.5s-chat'
    'gpt-4': 'abab6.5g-chat'
    '*': 'abab6.5t-chat'
```

**请求示例**

```json
{
  "model": "gpt-3",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "stream": false
}
```

**响应示例**

```json
{
  "id": "03ac4fcfe1c6cc9c6a60f9d12046e2b4",
  "choices": [
    {
      "finish_reason": "stop",
      "index": 0,
      "message": {
        "content": "你好，我是一个由MiniMax公司研发的大型语言模型，名为MM智能助理。我可以帮助回答问题、提供信息、进行对话和执行多种语言处理任务。如果你有任何问题或需要帮助，请随时告诉我！",
        "role": "assistant",
        "name": "MM智能助理",
        "audio_content": ""
      }
    }
  ],
  "created": 1734155471,
  "model": "abab6.5s-chat",
  "object": "chat.completion",
  "usage": {
    "total_tokens": 116,
    "total_characters": 0,
    "prompt_tokens": 70,
    "completion_tokens": 46
  },
  "input_sensitive": false,
  "output_sensitive": false,
  "input_sensitive_type": 0,
  "output_sensitive_type": 0,
  "output_sensitive_int": 0,
  "base_resp": {
    "status_code": 0,
    "status_msg": ""
  }
}
```

### 使用 OpenAI 协议代理 GitHub 模型服务

**配置信息**

```yaml
provider:
  type: github
  apiTokens:
    - 'YOUR_GITHUB_ACCESS_TOKEN'
  modelMapping:
    'gpt-4o': 'gpt-4o'
    'gpt-4': 'Phi-3.5-MoE-instruct'
    'gpt-3.5': 'cohere-command-r-08-2024'
    'text-embedding-3-large': 'text-embedding-3-large'
```

**请求示例**

```json
{
  "messages": [
    {
      "role": "system",
      "content": "You are a helpful assistant."
    },
    {
      "role": "user",
      "content": "What is the capital of France?"
    }
  ],
  "stream": true,
  "temperature": 1.0,
  "top_p": 1.0,
  "max_tokens": 1000,
  "model": "gpt-4o"
}
```

**响应示例**

```json
{
  "choices": [
    {
      "finish_reason": "stop",
      "index": 0,
      "logprobs": null,
      "message": {
        "content": "The capital of France is Paris.",
        "role": "assistant"
      }
    }
  ],
  "created": 1728131051,
  "id": "chatcmpl-AEy7PU2JImdsD1W6Jw8GigZSEnM2u",
  "model": "gpt-4o-2024-08-06",
  "object": "chat.completion",
  "system_fingerprint": "fp_67802d9a6d",
  "usage": {
    "completion_tokens": 7,
    "prompt_tokens": 24,
    "total_tokens": 31
  }
}
```

**文本向量请求示例**

```json
{
  "input": ["first phrase", "second phrase", "third phrase"],
  "model": "text-embedding-3-large"
}
```

响应示例：

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "index": 0,
      "embedding": [
        -0.0012583479,
        0.0020349282,
        ...
        0.012051377,
        -0.0053306012,
        0.0060688322
      ]
    }
  ],
  "model": "text-embedding-3-large",
  "usage": {
    "prompt_tokens": 6,
    "total_tokens": 6
  }
}
```

### 使用 OpenAI 协议代理 360 智脑服务

**配置信息**

```yaml
provider:
  type: ai360
  apiTokens:
    - 'YOUR_360_API_TOKEN'
  modelMapping:
    'gpt-4o': '360gpt-turbo-responsibility-8k'
    'gpt-4': '360gpt2-pro'
    'gpt-3.5': '360gpt-turbo'
    'text-embedding-3-small': 'embedding_s1_v1.2'
    '*': '360gpt-pro'
```

**请求示例**

```json
{
  "model": "gpt-4o",
  "messages": [
    {
      "role": "system",
      "content": "你是一个专业的开发人员！"
    },
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ]
}
```

**响应示例**

```json
{
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "你好，我是360智脑，一个大型语言模型。我可以帮助回答各种问题、提供信息、进行对话等。有什么可以帮助你的吗？"
      },
      "finish_reason": "",
      "index": 0
    }
  ],
  "created": 1724257207,
  "id": "5e5c94a2-d989-40b5-9965-5b971db941fe",
  "model": "360gpt-turbo",
  "object": "",
  "usage": {
    "completion_tokens": 33,
    "prompt_tokens": 24,
    "total_tokens": 57
  },
  "messages": [
    {
      "role": "system",
      "content": "你是一个专业的开发人员！"
    },
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "context": null
}
```

**文本向量请求示例**

URL: <http://your-domain/v1/embeddings>

请求示例：

```json
{
  "input": ["你好"],
  "model": "text-embedding-3-small"
}
```

响应示例：

```json
{
  "data": [
    {
      "embedding": [
        -0.011237,
        -0.015433,
        ...,
        -0.028946,
        -0.052778,
        0.003768,
        -0.007917,
        -0.042201
      ],
      "index": 0,
      "object": ""
    }
  ],
  "model": "embedding_s1_v1.2",
  "object": "",
  "usage": {
    "prompt_tokens": 2,
    "total_tokens": 2
  }
}
```

### 使用 OpenAI 协议代理 Cloudflare Workers AI 服务

**配置信息**

```yaml
provider:
  type: cloudflare
  apiTokens:
    - 'YOUR_WORKERS_AI_API_TOKEN'
  cloudflareAccountId: 'YOUR_CLOUDFLARE_ACCOUNT_ID'
  modelMapping:
    '*': '@cf/meta/llama-3-8b-instruct'
```

**请求示例**

```json
{
  "model": "gpt-3.5",
  "max_tokens": 1024,
  "messages": [
    {
      "role": "user",
      "content": "Who are you?"
    }
  ]
}
```

**响应示例**

```json
{
  "id": "id-1720367803430",
  "object": "chat.completion",
  "created": 1720367803,
  "model": "@cf/meta/llama-3-8b-instruct",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "I am LLaMA, an AI assistant developed by Meta AI that can understand and respond to human input in a conversational manner. I'm not a human, but a computer program designed to simulate conversation and answer questions to the best of my knowledge. I can be used to generate text on a wide range of topics, from science and history to entertainment and culture.\n\nI'm a large language model, which means I've been trained on a massive dataset of text from the internet and can generate human-like responses. I can understand natural language and respond accordingly, making me suitable for tasks such as:\n\n* Answering questions on various topics\n* Generating text based on a given prompt\n* Translating text from one language to another\n* Summarizing long pieces of text\n* Creating chatbot dialogues\n\nI'm constantly learning and improving, so the more conversations I have with users like you, the better I'll become."
      },
      "logprobs": null,
      "finish_reason": "stop"
    }
  ]
}
```

### 使用 OpenAI 协议代理 Spark 服务

**配置信息**

```yaml
provider:
  type: spark
  apiTokens:
    - 'APIKey:APISecret'
  modelMapping:
    'gpt-4o': 'generalv3.5'
    'gpt-4': 'generalv3'
    '*': 'general'
```

**请求示例**

```json
{
  "model": "gpt-4o",
  "messages": [
    {
      "role": "system",
      "content": "你是一名专业的开发人员！"
    },
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "stream": false
}
```

**响应示例**

```json
{
  "id": "cha000c23c6@dx190ef0b4b96b8f2532",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "你好！我是一名专业的开发人员，擅长编程和解决技术问题。有什么我可以帮助你的吗？"
      }
    }
  ],
  "created": 1721997415,
  "model": "generalv3.5",
  "object": "chat.completion",
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 19,
    "total_tokens": 29
  }
}
```

### 使用 OpenAI 协议代理 Gemini 服务

**配置信息**

```yaml
provider:
  type: gemini
  apiTokens:
    - "YOUR_GEMINI_API_TOKEN"
  modelMapping:
    "*": "gemini-pro"
  geminiSafetySetting:
    "HARM_CATEGORY_SEXUALLY_EXPLICIT" :"BLOCK_NONE"
    "HARM_CATEGORY_HATE_SPEECH" :"BLOCK_NONE"
    "HARM_CATEGORY_HARASSMENT" :"BLOCK_NONE"
    "HARM_CATEGORY_DANGEROUS_CONTENT" :"BLOCK_NONE"
```

**请求示例**

```json
{
  "model": "gpt-3.5",
  "messages": [
    {
      "role": "user",
      "content": "Who are you?"
    }
  ],
  "stream": false
}
```

**响应示例**

```json
{
  "id": "chatcmpl-b010867c-0d3f-40ba-95fd-4e8030551aeb",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "I am a large multi-modal model, trained by Google. I am designed to provide information and answer questions to the best of my abilities."
      },
      "finish_reason": "stop"
    }
  ],
  "created": 1722756984,
  "model": "gemini-pro",
  "object": "chat.completion",
  "usage": {
    "prompt_tokens": 5,
    "completion_tokens": 29,
    "total_tokens": 34
  }
}
```

### 使用 OpenAI 协议代理 DeepL 文本翻译服务

**配置信息**

```yaml
provider:
  type: deepl
  apiTokens:
    - 'YOUR_DEEPL_API_TOKEN'
  targetLang: 'ZH'
```

**请求示例**
此处 `model` 表示 DeepL 的服务类型，只能填 `Free` 或 `Pro`。`content` 中设置需要翻译的文本；在 `role: system` 的 `content` 中可以包含可能影响翻译但本身不会被翻译的上下文，例如翻译产品名称时，可以将产品描述作为上下文传递，这种额外的上下文可能会提高翻译的质量。

```json
{
  "model": "Free",
  "messages": [
    {
      "role": "system",
      "content": "money"
    },
    {
      "content": "sit by the bank"
    },
    {
      "content": "a bank in China"
    }
  ]
}
```

**响应示例**

```json
{
  "choices": [
    {
      "index": 0,
      "message": { "name": "EN", "role": "assistant", "content": "坐庄" }
    },
    {
      "index": 1,
      "message": { "name": "EN", "role": "assistant", "content": "中国银行" }
    }
  ],
  "created": 1722747752,
  "model": "Free",
  "object": "chat.completion",
  "usage": {}
}
```

### 使用 OpenAI 协议代理 Together-AI 服务

**配置信息**

```yaml
provider:
  type: together-ai
  apiTokens:
    - 'YOUR_TOGETHER_AI_API_TOKEN'
  modelMapping:
    '*': 'Qwen/Qwen2.5-72B-Instruct-Turbo'
```

**请求示例**

```json
{
  "model": "Qwen/Qwen2.5-72B-Instruct-Turbo",
  "messages": [
    {
      "role": "user",
      "content": "Who are you?"
    }
  ]
}
```

**响应示例**

```json
{
  "id": "8f5809d54b73efac",
  "object": "chat.completion",
  "created": 1734785851,
  "model": "Qwen/Qwen2.5-72B-Instruct-Turbo",
  "prompt": [],
  "choices": [
    {
      "finish_reason": "eos",
      "seed": 12830868308626506000,
      "logprobs": null,
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "I am Qwen, a large language model created by Alibaba Cloud. I am designed to assist users in generating various types of text, such as articles, stories, poems, and more, as well as answering questions and providing information on a wide range of topics. How can I assist you today?",
        "tool_calls": []
      }
    }
  ],
  "usage": {
    "prompt_tokens": 33,
    "completion_tokens": 61,
    "total_tokens": 94
  }
}
```

### 使用 OpenAI 协议代理 Dify 服务

**配置信息**

```yaml
provider:
  type: dify
  apiTokens:
    - 'YOUR_DIFY_API_TOKEN'
  modelMapping:
    '*': 'dify'
```

**请求示例**

```json
{
  "model": "gpt-4-turbo",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "stream": false
}
```

**响应示例**

```json
{
  "id": "e33fc636-f9e8-4fae-8d5e-fbd0acb09401",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "你好！我是ChatGPT，由OpenAI开发的人工智能语言模型。我可以帮助回答问题、提供建议或进行各种对话。如果你有任何需要，随时告诉我哦！"
      },
      "finish_reason": "stop"
    }
  ],
  "created": 1736657752,
  "model": "dify",
  "object": "chat.completion",
  "usage": {
    "prompt_tokens": 16,
    "completion_tokens": 243,
    "total_tokens": 259
  }
}
```

### 使用 OpenAI 协议代理 Google Vertex 服务（标准模式）

**配置信息**

```yaml
provider:
  type: vertex
  vertexAuthKey: |
    {
      "type": "service_account",
      "project_id": "your-project-id",
      "private_key_id": "your-private-key-id",
      "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
      "client_email": "your-service-account@your-project.iam.gserviceaccount.com",
      "token_uri": "https://oauth2.googleapis.com/token"
    }
  vertexRegion: us-central1
  vertexProjectId: your-project-id
  vertexAuthServiceName: your-auth-service-name
```

**请求示例**

```json
{
  "model": "gemini-2.0-flash-001",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "stream": false
}
```

**响应示例**

```json
{
  "id": "chatcmpl-0000000000000",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "你好！我是 Vertex AI 提供的 Gemini 模型，由 Google 开发的人工智能助手。我可以回答问题、提供信息和帮助完成各种任务。有什么我可以帮您的吗？"
      },
      "finish_reason": "stop"
    }
  ],
  "created": 1729986750,
  "model": "gemini-2.0-flash-001",
  "object": "chat.completion",
  "usage": {
    "prompt_tokens": 15,
    "completion_tokens": 43,
    "total_tokens": 58
  }
}
```

### 使用 OpenAI 协议代理 Google Vertex 服务（Express Mode）

Express Mode 是 Vertex AI 的简化访问模式，只需 API Key 即可快速开始使用。

**配置信息**

```yaml
provider:
  type: vertex
  apiTokens:
    - "YOUR_API_KEY"
```

**请求示例**

```json
{
  "model": "gemini-2.5-flash",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "stream": false
}
```

**响应示例**

```json
{
  "id": "chatcmpl-0000000000000",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "你好！我是 Gemini，由 Google 开发的人工智能助手。有什么我可以帮您的吗？"
      },
      "finish_reason": "stop"
    }
  ],
  "created": 1729986750,
  "model": "gemini-2.5-flash",
  "object": "chat.completion",
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 25,
    "total_tokens": 35
  }
}
```

### 使用 OpenAI 协议代理 Google Vertex 服务（OpenAI 兼容模式）

OpenAI 兼容模式使用 Vertex AI 的 OpenAI-compatible Chat Completions API，请求和响应都使用 OpenAI 格式，无需进行协议转换。

**配置信息**

```yaml
provider:
  type: vertex
  vertexOpenAICompatible: true
  vertexAuthKey: |
    {
      "type": "service_account",
      "project_id": "your-project-id",
      "private_key_id": "your-private-key-id",
      "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
      "client_email": "your-service-account@your-project.iam.gserviceaccount.com",
      "token_uri": "https://oauth2.googleapis.com/token"
    }
  vertexRegion: us-central1
  vertexProjectId: your-project-id
  vertexAuthServiceName: your-auth-service-name
  modelMapping:
    "gpt-4": "gemini-2.0-flash"
    "*": "gemini-1.5-flash"
```

**请求示例**

```json
{
  "model": "gpt-4",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "stream": false
}
```

**响应示例**

```json
{
  "id": "chatcmpl-abc123",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "你好！我是由 Google 开发的 Gemini 模型。我可以帮助回答问题、提供信息和进行对话。有什么我可以帮您的吗？"
      },
      "finish_reason": "stop"
    }
  ],
  "created": 1729986750,
  "model": "gemini-2.0-flash",
  "object": "chat.completion",
  "usage": {
    "prompt_tokens": 12,
    "completion_tokens": 35,
    "total_tokens": 47
  }
}
```

### 使用 OpenAI 协议代理 Google Vertex 图片生成服务

Vertex AI 支持使用 Gemini 模型进行图片生成。通过 ai-proxy 插件，可以使用 OpenAI 的 `/v1/images/generations` 接口协议来调用 Vertex AI 的图片生成能力。

**配置信息**

```yaml
provider:
  type: vertex
  apiTokens:
    - "YOUR_API_KEY"
  modelMapping:
    "dall-e-3": "gemini-2.0-flash-exp"
  geminiSafetySetting:
    HARM_CATEGORY_HARASSMENT: "OFF"
    HARM_CATEGORY_HATE_SPEECH: "OFF"
    HARM_CATEGORY_SEXUALLY_EXPLICIT: "OFF"
    HARM_CATEGORY_DANGEROUS_CONTENT: "OFF"
```

**使用 curl 请求**

```bash
curl -X POST "http://your-gateway-address/v1/images/generations" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.0-flash-exp",
    "prompt": "一只可爱的橘猫在阳光下打盹",
    "size": "1024x1024"
  }'
```

**使用 OpenAI Python SDK**

```python
from openai import OpenAI

client = OpenAI(
    api_key="any-value",  # 可以是任意值，认证由网关处理
    base_url="http://your-gateway-address/v1"
)

response = client.images.generate(
    model="gemini-2.0-flash-exp",
    prompt="一只可爱的橘猫在阳光下打盹",
    size="1024x1024",
    n=1
)

# 获取生成的图片（base64 编码）
image_data = response.data[0].b64_json
print(f"Generated image (base64): {image_data[:100]}...")
```

**响应示例**

```json
{
  "created": 1729986750,
  "data": [
    {
      "b64_json": "iVBORw0KGgoAAAANSUhEUgAABAAAAAQACAIAAADwf7zUAAAA..."
    }
  ],
  "usage": {
    "total_tokens": 1356,
    "input_tokens": 13,
    "output_tokens": 1120
  }
}
```

**支持的尺寸参数**

Vertex AI 支持的宽高比（aspectRatio）：`1:1`、`3:2`、`2:3`、`3:4`、`4:3`、`4:5`、`5:4`、`9:16`、`16:9`、`21:9`

Vertex AI 支持的分辨率（imageSize）：`1k`、`2k`、`4k`

| OpenAI size 参数 | Vertex AI aspectRatio | Vertex AI imageSize |
|------------------|----------------------|---------------------|
| 256x256          | 1:1                  | 1k                  |
| 512x512          | 1:1                  | 1k                  |
| 1024x1024        | 1:1                  | 1k                  |
| 1792x1024        | 16:9                 | 2k                  |
| 1024x1792        | 9:16                 | 2k                  |
| 2048x2048        | 1:1                  | 2k                  |
| 4096x4096        | 1:1                  | 4k                  |
| 1536x1024        | 3:2                  | 2k                  |
| 1024x1536        | 2:3                  | 2k                  |
| 1024x768         | 4:3                  | 1k                  |
| 768x1024         | 3:4                  | 1k                  |
| 1280x1024        | 5:4                  | 1k                  |
| 1024x1280        | 4:5                  | 1k                  |
| 2560x1080        | 21:9                 | 2k                  |

**注意事项**

- 图片生成使用 Gemini 模型（如 `gemini-2.0-flash-exp`、`gemini-3-pro-image-preview`），不同模型的可用性可能因区域而异
- 返回的图片数据为 base64 编码格式（`b64_json`）
- 可以通过 `geminiSafetySetting` 配置内容安全过滤级别
- 如果需要使用模型映射（如将 `dall-e-3` 映射到 Gemini 模型），可以配置 `modelMapping`

### 使用 OpenAI 协议代理 AWS Bedrock 服务

AWS Bedrock 支持两种认证方式：

#### 方式一：使用 AWS Access Key/Secret Key 认证（AWS Signature V4）

**配置信息**

```yaml
provider:
  type: bedrock
  awsAccessKey: "YOUR_AWS_ACCESS_KEY_ID"
  awsSecretKey: "YOUR_AWS_SECRET_ACCESS_KEY"
  awsRegion: "us-east-1"
  bedrockAdditionalFields:
    top_k: 200
```

#### 方式二：使用 Bearer Token 认证（适用于 IAM Identity Center 等场景）

**配置信息**

```yaml
provider:
  type: bedrock
  apiTokens:
    - "YOUR_AWS_BEARER_TOKEN"
  awsRegion: "us-east-1"
  bedrockAdditionalFields:
    top_k: 200
```

**请求示例**

```json
{
  "model": "us.anthropic.claude-3-5-haiku-20241022-v1:0",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "stream": false
}
```

**响应示例**

```json
{
  "id": "dc5812e2-6a62-49d6-829e-5c327b15e4e2",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "你好!我是Claude,一个由Anthropic开发的AI助手。很高兴认识你!我的目标是以诚实、有益且有意义的方式与人类交流。我会尽力提供准确和有帮助的信息,同时保持诚实和正直。请问我今天能为你做些什么呢?"
      },
      "finish_reason": "stop"
    }
  ],
  "created": 1749657608,
  "model": "arn:aws:bedrock:us-west-2::foundation-model/anthropic.claude-3-5-haiku-20241022-v1:0",
  "object": "chat.completion",
  "usage": {
    "prompt_tokens": 16,
    "completion_tokens": 101,
    "total_tokens": 117
  }
}
```

### 使用 OpenAI 协议代理 NVIDIA Triton Interference Server 服务

**配置信息**

```yaml
providers:
  - type: triton
    tritonDomain: <LOCAL_TRITON_DOMAIN>
    tritonModelVersion: <MODEL_VERSION>
    apiTokens:
      - "****"
    modelMapping:
      "*": gpt2
```

**请求示例**

```json
{
  "model": "gpt2",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ],
  "stream": false
}
```

**响应示例**

```json
{
    "choices": [
        {
            "index": 0,
            "message": {
                "role": "assistant",
                "content": "我是一个AI模型"
            },
            "finish_reason": "stop",
        }
    ],
    "model": "gpt2",
}
```

### 使用上下文清理命令

配置上下文清理命令后，用户可以通过发送特定消息来主动清理对话历史，实现"重新开始对话"的效果。

**配置信息**

```yaml
provider:
  type: qwen
  apiTokens:
    - "YOUR_QWEN_API_TOKEN"
  modelMapping:
    "*": "qwen-turbo"
  contextCleanupCommands:
    - "清理上下文"
    - "/clear"
    - "重新开始"
    - "新对话"
```

**请求示例**

当用户发送包含清理命令的请求时：

```json
{
  "model": "gpt-3",
  "messages": [
    {
      "role": "system",
      "content": "你是一个助手"
    },
    {
      "role": "user",
      "content": "你好"
    },
    {
      "role": "assistant",
      "content": "你好！有什么可以帮助你的？"
    },
    {
      "role": "user",
      "content": "今天天气怎么样"
    },
    {
      "role": "assistant",
      "content": "抱歉，我无法获取实时天气信息。"
    },
    {
      "role": "user",
      "content": "清理上下文"
    },
    {
      "role": "user",
      "content": "现在开始新话题，介绍一下你自己"
    }
  ]
}
```

**实际发送给 AI 服务的请求**

插件会自动清理"清理上下文"命令及之前的所有非 system 消息：

```json
{
  "model": "qwen-turbo",
  "messages": [
    {
      "role": "system",
      "content": "你是一个助手"
    },
    {
      "role": "user",
      "content": "现在开始新话题，介绍一下你自己"
    }
  ]
}
```

**说明**

- 清理命令必须完全匹配配置的字符串，部分匹配不会触发清理
- 当存在多个清理命令时，只处理最后一个匹配的命令
- 清理会保留所有 system 消息，删除命令及之前的 user、assistant、tool 消息
- 清理命令之后的所有消息都会保留

## 完整配置示例

### Kubernetes 示例

以下以使用 OpenAI 协议代理 Groq 服务为例，展示完整的插件配置示例。

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: ai-proxy-groq
  namespace: higress-system
spec:
  matchRules:
    - config:
        provider:
          type: groq
          apiTokens:
            - 'YOUR_API_TOKEN'
      ingress:
        - groq
  url: oci://higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/ai-proxy:1.0.0
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    higress.io/backend-protocol: HTTPS
    higress.io/destination: groq.dns
    higress.io/proxy-ssl-name: api.groq.com
    higress.io/proxy-ssl-server-name: 'on'
  labels:
    higress.io/resource-definer: higress
  name: groq
  namespace: higress-system
spec:
  ingressClassName: higress
  rules:
    - host: <YOUR-DOMAIN>
      http:
        paths:
          - backend:
              resource:
                apiGroup: networking.higress.io
                kind: McpBridge
                name: default
            path: /
            pathType: Prefix
---
apiVersion: networking.higress.io/v1
kind: McpBridge
metadata:
  name: default
  namespace: higress-system
spec:
  registries:
    - domain: api.groq.com
      name: groq
      port: 443
      type: dns
```

访问示例：

```bash
curl "http://<YOUR-DOMAIN>/v1/chat/completions" -H "Content-Type: application/json" -d '{
  "model": "llama3-8b-8192",
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ]
}'
```

### Docker-Compose 示例

`docker-compose.yml` 配置文件：

```yaml
version: '3.7'
services:
  envoy:
    image: higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/envoy:1.20
    entrypoint: /usr/local/bin/envoy
    # 开启了 debug 级别日志方便调试
    command: -c /etc/envoy/envoy.yaml --component-log-level wasm:debug
    networks:
      - higress-net
    ports:
      - '10000:10000'
    volumes:
      - ./envoy.yaml:/etc/envoy/envoy.yaml
      - ./plugin.wasm:/etc/envoy/plugin.wasm
networks:
  higress-net: {}
```

`envoy.yaml` 配置文件：

```yaml
admin:
  address:
    socket_address:
      protocol: TCP
      address: 0.0.0.0
      port_value: 9901
static_resources:
  listeners:
    - name: listener_0
      address:
        socket_address:
          protocol: TCP
          address: 0.0.0.0
          port_value: 10000
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                '@type': type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                scheme_header_transformation:
                  scheme_to_overwrite: https
                stat_prefix: ingress_http
                # Output envoy logs to stdout
                access_log:
                  - name: envoy.access_loggers.stdout
                    typed_config:
                      '@type': type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog
                # Modify as required
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: local_service
                      domains: ['*']
                      routes:
                        - match:
                            prefix: '/'
                          route:
                            cluster: claude
                            timeout: 300s
                http_filters:
                  - name: claude
                    typed_config:
                      '@type': type.googleapis.com/udpa.type.v1.TypedStruct
                      type_url: type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm
                      value:
                        config:
                          name: claude
                          vm_config:
                            runtime: envoy.wasm.runtime.v8
                            code:
                              local:
                                filename: /etc/envoy/plugin.wasm
                          configuration:
                            '@type': 'type.googleapis.com/google.protobuf.StringValue'
                            value: | # 插件配置
                              {
                                "provider": {
                                  "type": "claude",                                
                                  "apiTokens": [
                                    "YOUR_API_TOKEN"
                                  ]                  
                                }
                              }
                  - name: envoy.filters.http.router
  clusters:
    - name: claude
      connect_timeout: 30s
      type: LOGICAL_DNS
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: claude
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: api.anthropic.com # API 服务地址
                      port_value: 443
      transport_socket:
        name: envoy.transport_sockets.tls
        typed_config:
          '@type': type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
          'sni': 'api.anthropic.com'
```

访问示例：

```bash
curl "http://localhost:10000/v1/chat/completions"  -H "Content-Type: application/json"  -d '{
  "model": "claude-3-opus-20240229",
  "max_tokens": 1024,
  "messages": [
    {
      "role": "user",
      "content": "你好，你是谁？"
    }
  ]
}'
```
