---
title: AI Proxy
keywords: [AI Gateway, AI Proxy]
description: Reference for configuring the AI Proxy plugin
---

## Function Description

The `AI Proxy` plugin implements AI proxy functionality based on the OpenAI API contract. It currently supports AI service providers such as OpenAI, Azure OpenAI, Moonshot, and Qwen.

**🚀 Auto Protocol Compatibility**

The plugin now supports **automatic protocol detection**, allowing seamless compatibility with both OpenAI and Claude protocol formats without configuration:

- **OpenAI Protocol**: Request path `/v1/chat/completions`, using standard OpenAI Messages API format
- **Claude Protocol**: Request path `/v1/messages`, using Anthropic Claude Messages API format  
- **Intelligent Conversion**: Automatically detects request protocol and performs conversion if the target provider doesn't natively support it
- **Zero Configuration**: No need to set `protocol` field, the plugin handles everything automatically

> **Protocol Support:**

> When the request path suffix matches `/v1/chat/completions`, it corresponds to text-to-text scenarios. The request body will be parsed using OpenAI's text-to-text protocol and then converted to the corresponding LLM vendor's text-to-text protocol.

> When the request path suffix matches `/v1/messages`, it corresponds to Claude text-to-text scenarios. The plugin automatically detects provider capabilities: if native Claude protocol is supported, requests are forwarded directly; otherwise, they are converted to OpenAI protocol first.

> When the request path suffix matches `/v1/embeddings`, it corresponds to text vector scenarios. The request body will be parsed using OpenAI's text vector protocol and then converted to the corresponding LLM vendor's text vector protocol.

> When the request path suffix matches `/v1/images/generations`, it corresponds to text-to-image scenarios. The request body will be parsed using OpenAI's image generation protocol and then converted to the corresponding LLM vendor's image generation protocol.

## Execution Properties
Plugin execution phase: `Default Phase`
Plugin execution priority: `100`


## Configuration Fields

### Basic Configuration

| Name       | Data Type   | Requirement | Default | Description               |
|------------|--------|------|-----|------------------|
| `provider` | object | Required   | -   | Configures information for the target AI service provider |

**Details for the `provider` configuration fields:**

| Name             | Data Type              | Requirement | Default | Description                                                                                                                                                                                                                                                                                                                                                                               |
| --------------   | ---------------        | --------    | ------  | -------------------------------------------------------------------------------------------------------------------------------------------------------------                                                                                                                                                                                                                             |
| `type`           | string                 | Required    | -       | Name of the AI service provider                                                                                                                                                                                                                                                                                                                                                           |
| `apiTokens`      | array of string        | Optional    | -       | Tokens used for authentication when accessing AI services. If multiple tokens are configured, the plugin randomly selects one for each request. Some service providers only support configuring a single token.                                                                                                                                                                           |
| `timeout`        | number                 | Optional    | -       | Timeout for accessing AI services, in milliseconds. The default value is 120000, which equals 2 minutes. Only used when retrieving context data. Won't affect the request forwarded to the LLM upstream.                                                                                                                                                                                  |
| `modelMapping`   | map of string          | Optional    | -       | Mapping table for AI models, used to map model names in requests to names supported by the service provider.<br/>1. Supports prefix matching. For example, "gpt-3-\*" matches all model names starting with “gpt-3-”;<br/>2. Supports using "\*" as a key for a general fallback mapping;<br/>3. If the mapped target name is an empty string "", the original model name is preserved. |
| `protocol`       | string                 | Optional    | -       | API contract provided by the plugin. Currently supports the following values: openai (default, uses OpenAI's interface contract), original (uses the raw interface contract of the target service provider). **Note: Auto protocol detection is now supported, no need to configure this field to support both OpenAI and Claude protocols**                                                                                                                                                                               |
| `context`        | object                 | Optional    | -       | Configuration for AI conversation context information                                                                                                                                                                                                                                                                                                                                     |
| `customSettings` | array of customSetting | Optional    | -       | Specifies overrides or fills parameters for AI requests                                                                                                                                                                                                                                                                                                                                   |
| `subPath`        | string                 | Optional    | -       | If subPath is configured, the prefix will be removed from the request path before further processing.                                                                                                                                                                                                                                                                                     |
| `contextCleanupCommands` | array of string | Optional    | -       | List of context cleanup commands. When a user message in the request exactly matches any of the configured commands, that message and all non-system messages before it will be removed, keeping only system messages and messages after the command. This enables users to actively clear conversation history.                                                                           |

**Details for the `context` configuration fields:**

| Name            | Data Type   | Requirement | Default | Description                               |
|---------------|--------|------|-----|----------------------------------|
| `fileUrl`     | string | Required   | -   | File URL to save AI conversation context. Only supports file content of plain text type |
| `serviceName` | string | Required   | -   | Full name of the Higress backend service corresponding to the URL        |
| `servicePort` | number | Required   | -   | Port for accessing the Higress backend service corresponding to the URL        |

**Details for the `customSettings` configuration fields:**

| Name        | Data Type              | Requirement | Default | Description                                                                                                                         |
| ----------- | --------------------- | -------- | ------ | ---------------------------------------------------------------------------------------------------------------------------- |
| `name`      | string                | Required     | -      | Name of the parameter to set, e.g., `max_tokens`                                                                                       |
| `value`     | string/int/float/bool | Required     | -      | Value of the parameter to set, e.g., 0                                                                                                    |
| `mode`      | string                | Optional   | "auto" | Mode for setting the parameter, can be set to "auto" or "raw"; if "auto", the parameter name will be automatically rewritten based on the protocol; if "raw", no rewriting or restriction checks will be applied |
| `overwrite` | bool                  | Optional   | true   | If false, the parameter is only filled if the user has not set it; otherwise, it directly overrides the user's existing parameter settings                                            |

The `custom-setting` adheres to the following table, replacing the corresponding field based on `name` and protocol. Users need to fill in values from the `settingName` column that exists in the table. For instance, if a user sets `name` to `max_tokens`, in the openai protocol, it replaces `max_tokens`; for gemini, it replaces `maxOutputTokens`. `"none"` indicates that the protocol does not support this parameter. If `name` is not in this table or the corresponding protocol does not support the parameter, and "raw" mode is not set, the configuration will not take effect.

| settingName | openai      | baidu             | spark       | qwen        | gemini          | hunyuan     | claude      | minimax            |
| ----------- | ----------- | ----------------- | ----------- | ----------- | --------------- | ----------- | ----------- | ------------------ |
| max_tokens  | max_tokens  | max_output_tokens | max_tokens  | max_tokens  | maxOutputTokens | none        | max_tokens  | tokens_to_generate |
| temperature | temperature | temperature       | temperature | temperature | temperature     | Temperature | temperature | temperature        |
| top_p       | top_p       | top_p             | none        | top_p       | topP            | TopP        | top_p       | top_p              |
| top_k       | none        | none              | top_k       | none        | topK            | none        | top_k       | none               |
| seed        | seed        | none              | none        | seed        | none            | none        | none        | none               |

If raw mode is enabled, `custom-setting` will directly alter the JSON content using the input `name` and `value`, without any restrictions or modifications to the parameter names.
For most protocols, `custom-setting` modifies or fills parameters at the root path of the JSON content. For the `qwen` protocol, ai-proxy configures under the `parameters` subpath. For the `gemini` protocol, it configures under the `generation_config` subpath.

### Provider-Specific Configurations

#### OpenAI

For OpenAI, the corresponding `type` is `openai`. Its unique configuration fields include:

| Name              | Data Type | Requirement | Default | Description                                                                          |
|-------------------|----------|----------|--------|-------------------------------------------------------------------------------|
| `openaiCustomUrl` | string   | Optional   | -      | Custom backend URL based on the OpenAI protocol, e.g., www.example.com/myai/v1/chat/completions |
| `responseJsonSchema` | object | Optional | - | Predefined Json Schema that OpenAI responses must adhere to; note that currently only a few specific models support this usage|

#### Azure OpenAI

For Azure OpenAI, the corresponding `type` is `azure`. Its unique configuration field is:

| Name                 | Data Type   | Filling Requirements | Default Value | Description                                                                                                    |
|---------------------|-------------|----------------------|---------------|---------------------------------------------------------------------------------------------------------------|
| `azureServiceUrl`   | string      | Required             | -             | The URL of the Azure OpenAI service, must include the `api-version` query parameter.                           |

**Note:**
1. Azure OpenAI only supports configuring one API Token.
2. `azureServiceUrl` accepts three formats：
    1. Full URL. e.g. `https://YOUR_RESOURCE_NAME.openai.azure.com/openai/deployments/YOUR_DEPLOYMENT_NAME/chat/completions?api-version=2024-02-15-preview`
        - Request will be forwarded to the given URL, no matter what original path the request uses.
    2. Resource name + deployment name，e.g. `https://YOUR_RESOURCE_NAME.openai.azure.com/openai/deployments/YOUR_DEPLOYMENT_NAME?api-version=2024-02-15-preview`
        - The path will be updated based on the actual request path, leaving the deployment name unchanged. APIs with no deployment name in the path are also support.
    3. Resource name only.e.g.`https://YOUR_RESOURCE_NAME.openai.azure.com?api-version=2024-02-15-preview`
        - The path will be updated based on the actual request path. The deployment name will be filled based on the model name in the request and the configured model mapping rule. APIs with no deployment name in the path are also support.

#### Moonshot

For Moonshot, the corresponding `type` is `moonshot`. Its unique configuration field is:

| Name                | Data Type   | Filling Requirements | Default Value | Description                                                                                                      |
|-------------------|-------------|----------------------|---------------|-----------------------------------------------------------------------------------------------------------------|
| `moonshotFileId`   | string      | Optional             | -             | The file ID uploaded via the file interface to Moonshot, whose content will be used as context for AI conversations. Cannot be configured with the `context` field. |

#### Qwen (Tongyi Qwen)

For Qwen (Tongyi Qwen), the corresponding `type` is `qwen`. Its unique configuration fields are:

| Name                 | Data Type            | Filling Requirements | Default Value | Description                                                                                                            |
|--------------------|-----------------|----------------------|---------------|------------------------------------------------------------------------------------------------------------------------|
| `qwenEnableSearch`  | boolean          | Optional             | -             | Whether to enable the built-in Internet search function provided by Qwen.                                             |
| `qwenFileIds`       | array of string   | Optional             | -             | The file IDs uploaded via the Dashscope file interface, whose content will be used as context for AI conversations. Cannot be configured with the `context` field. |
| `qwenEnableCompatible` | boolean          | Optional | false         | Enable Qwen compatibility mode. When Qwen compatibility mode is enabled, the compatible mode interface of Qwen will be called, and the request/response will not be modified. |

#### Baichuan AI

For Baichuan AI, the corresponding `type` is `baichuan`. It has no unique configuration fields.

#### Yi (Zero One Universe)

For Yi (Zero One Universe), the corresponding `type` is `yi`. It has no unique configuration fields.

#### Zhipu AI

For Zhipu AI, the corresponding `type` is `zhipuai`. It has no unique configuration fields.

#### DeepSeek

For DeepSeek, the corresponding `type` is `deepseek`. It has no unique configuration fields.

#### Groq

For Groq, the corresponding `type` is `groq`. It has no unique configuration fields.

#### Grok

For Grok, the corresponding `type` is `grok`. It has no unique configuration fields.

#### OpenRouter

For OpenRouter, the corresponding `type` is `openrouter`. It has no unique configuration fields.

#### Fireworks AI

For Fireworks AI, the corresponding `type` is `fireworks`. It has no unique configuration fields.

#### ERNIE Bot

For ERNIE Bot, the corresponding `type` is `baidu`. It has no unique configuration fields.

### 360 Brain

For 360 Brain, the corresponding `type` is `ai360`. It has no unique configuration fields.

### Mistral

For Mistral, the corresponding `type` is `mistral`. It has no unique configuration fields.

#### MiniMax

For MiniMax, the corresponding `type` is `minimax`. Its unique configuration field is:

| Name             | Data Type | Filling Requirements | Default Value | Description                                                                                                 |
| ---------------- | -------- | --------------------- |---------------|------------------------------------------------------------------------------------------------------------|
| `minimaxGroupId` | string   | Required when using models `abab6.5-chat`, `abab6.5s-chat`, `abab5.5s-chat`, `abab5.5-chat` | -             | When using models `abab6.5-chat`, `abab6.5s-chat`, `abab5.5s-chat`, `abab5.5-chat`, Minimax uses ChatCompletion Pro and requires setting the groupID. |

#### Anthropic Claude

For Anthropic Claude, the corresponding `type` is `claude`. Its unique configuration fields are:

| Name        | Data Type   | Filling Requirements | Default Value | Description                                                                                                    |
|------------|-------------|----------------------|---------------|---------------------------------------------------------------------------------------------------------------|
| `claudeVersion` | string | Optional             | -             | The version of the Claude service's API, default is 2023-06-01.                                               |
| `claudeCodeMode` | boolean | Optional             | false         | Enable Claude Code mode for OAuth token authentication. When enabled, requests will be formatted as Claude Code client requests. |

**Claude Code Mode**

When `claudeCodeMode: true` is enabled, the plugin will:
- Use Bearer Token authentication instead of x-api-key (compatible with Claude Code OAuth tokens)
- Set Claude Code-specific request headers (user-agent, x-app, anthropic-beta)
- Add `?beta=true` query parameter to request URLs
- Automatically inject Claude Code system prompt if not provided

This enables direct use of Claude Code OAuth tokens for authentication in Higress.

#### Ollama

For Ollama, the corresponding `type` is `ollama`. Its unique configuration field is:

| Name                | Data Type   | Filling Requirements | Default Value | Description                                                                                              |
|-------------------|-------------|----------------------|---------------|---------------------------------------------------------------------------------------------------------|
| `ollamaServerHost` | string      | Required             | -             | The host address of the Ollama server.                                                                |
| `ollamaServerPort` | number      | Required             | -             | The port number of the Ollama server, defaults to 11434.                                              |

#### Generic

For a vendor-agnostic passthrough, set the provider `type` to `generic`. Requests are forwarded without path remapping, while still benefiting from the shared header/basePath utilities.

| Name           | Data Type | Requirement | Default | Description                                                                                              |
|----------------|-----------|-------------|---------|----------------------------------------------------------------------------------------------------------|
| `genericHost`  | string    | Optional    | -       | Overrides the upstream `Host` header. Use it to route traffic to a specific backend domain for generic proxying. |

- When `apiTokens` are configured, the Generic provider injects `Authorization: Bearer <token>` automatically.
- `firstByteTimeout` applies to any request whose body sets `stream: true`, ensuring consistent streaming behavior even without capability definitions.
- `basePath` and `basePathHandling` remain available to strip or prepend prefixes before forwarding.

#### Hunyuan

For Hunyuan, the corresponding `type` is `hunyuan`. Its unique configuration fields are:

| Name                | Data Type   | Filling Requirements | Default Value | Description                                                                                              |
|-------------------|-------------|----------------------|---------------|---------------------------------------------------------------------------------------------------------|
| `hunyuanAuthId`    | string      | Required             | -             | Hunyuan authentication ID for version 3 authentication.                                                |
| `hunyuanAuthKey`   | string      | Required             | -             | Hunyuan authentication key for version 3 authentication.                                               |

#### Stepfun

For Stepfun, the corresponding `type` is `stepfun`. It has no unique configuration fields.

#### Cloudflare Workers AI

For Cloudflare Workers AI, the corresponding `type` is `cloudflare`. Its unique configuration field is:

| Name                | Data Type   | Filling Requirements | Default Value | Description                                                                                              |
|-------------------|-------------|----------------------|---------------|---------------------------------------------------------------------------------------------------------|
| `cloudflareAccountId` | string      | Required             | -             | [Cloudflare Account ID](https://developers.cloudflare.com/workers-ai/get-started/rest-api/#1-get-api-token-and-account-id). |

#### Spark

For Spark, the corresponding `type` is `spark`. It has no unique configuration fields.

The `apiTokens` field value for Xunfei Spark (Xunfei Star) is `APIKey:APISecret`. That is, enter your own APIKey and APISecret, separated by `:`.

#### Gemini

For Gemini, the corresponding `type` is `gemini`. Its unique configuration field is:

| Name                  | Data Type | Filling Requirements | Default Value | Description                                                                                              |
|---------------------|----------|----------------------|---------------|---------------------------------------------------------------------------------------------------------|
| `geminiSafetySetting` | map of string   | Optional             | -             | Gemini AI content filtering and safety level settings. Refer to [Safety settings](https://ai.google.dev/gemini-api/docs/safety-settings). |
| `apiVersion` | string | 非必填 | `v1beta` | To specify the version of the API, you can choose either 'v1' or 'v1beta'. Version differences refer to https://ai.google.dev/gemini-api/docs/api-versions |
| `geminiThinkingBudget` | number | 非必填 | - | The parameters of the gemini2.5 series: 0 indicates no thinking mode, -1 represents dynamic adjustment. For specific parameter references, please refer to the official website |

### DeepL

For DeepL, the corresponding `type` is `deepl`. Its unique configuration field is:

| Name         | Data Type | Requirement | Default | Description                         |
| ------------ | --------- | ----------- | ------- | ------------------------------------ |
| `targetLang` | string    | Required    | -       | The target language required by the DeepL translation service |

#### Google Vertex AI
For Vertex, the corresponding `type` is `vertex`. It supports two authentication modes:

**Standard Mode** (using Service Account):

| Name                        | Data Type     | Requirement   | Default | Description                                                                                                                                                 |
|-----------------------------|---------------|---------------| ------ |-------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `vertexAuthKey`             | string        | Required      | -      | Google Service Account JSON Key used for authentication. The format should be PEM encoded PKCS#8 private key along with client_email and other information  |
| `vertexRegion`              | string        | Required      | -      | Google Cloud region (e.g., us-central1, europe-west4) used to build the Vertex API address                                                                  |
| `vertexProjectId`           | string        | Required      | -      | Google Cloud Project ID, used to identify the target GCP project                                                                                            |
| `vertexAuthServiceName`     | string        | Required      | -      | Service name for OAuth2 authentication, used to access oauth2.googleapis.com                                                                                |
| `vertexGeminiSafetySetting` | map of string | Optional      | -      | Gemini model content safety filtering settings.                                                                                                             |
| `vertexTokenRefreshAhead`   | number        | Optional      | -      | Vertex access token refresh ahead time in seconds                                                                                                           |

**Express Mode** (using API Key, simplified configuration):

Express Mode is a simplified access mode introduced by Vertex AI. You can quickly get started with just an API Key, without configuring a Service Account. See [Vertex AI Express Mode documentation](https://cloud.google.com/vertex-ai/generative-ai/docs/start/express-mode/overview).

| Name                        | Data Type        | Requirement   | Default | Description                                                                                                                                                 |
|-----------------------------|------------------|---------------| ------ |-------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `apiTokens`                 | array of string  | Required      | -      | API Key for Express Mode, obtained from Google Cloud Console under API & Services > Credentials                                                              |
| `vertexGeminiSafetySetting` | map of string    | Optional      | -      | Gemini model content safety filtering settings.                                                                                                             |

**OpenAI Compatible Mode** (using Vertex AI Chat Completions API):

Vertex AI provides an OpenAI-compatible Chat Completions API endpoint, allowing you to use OpenAI format requests and responses directly without protocol conversion. See [Vertex AI OpenAI Compatibility documentation](https://cloud.google.com/vertex-ai/generative-ai/docs/migrate/openai/overview).

| Name                        | Data Type        | Requirement   | Default | Description                                                                                                                                                 |
|-----------------------------|------------------|---------------| ------ |-------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `vertexOpenAICompatible`    | boolean          | Optional      | false  | Enable OpenAI compatible mode. When enabled, uses Vertex AI's OpenAI-compatible Chat Completions API |
| `vertexAuthKey`             | string           | Required      | -      | Google Service Account JSON Key for authentication |
| `vertexRegion`              | string           | Required      | -      | Google Cloud region (e.g., us-central1, europe-west4) |
| `vertexProjectId`           | string           | Required      | -      | Google Cloud Project ID |
| `vertexAuthServiceName`     | string           | Required      | -      | Service name for OAuth2 authentication |

**Note**: OpenAI Compatible Mode and Express Mode are mutually exclusive. You cannot configure both `apiTokens` and `vertexOpenAICompatible` at the same time.

#### AWS Bedrock

For AWS Bedrock, the corresponding `type` is `bedrock`. It supports two authentication methods:

1. **AWS Signature V4 Authentication**: Uses `awsAccessKey` and `awsSecretKey` for standard AWS signature authentication
2. **Bearer Token Authentication**: Uses `apiTokens` to configure AWS Bearer Token (suitable for IAM Identity Center and similar scenarios)

**Note**: Choose one of the two authentication methods. If `apiTokens` is configured, Bearer Token authentication will be used preferentially.

Its unique configuration fields are:

| Name                      | Data Type       | Requirement              | Default | Description                                                       |
|---------------------------|-----------------|--------------------------|---------|-------------------------------------------------------------------|
| `apiTokens`               | array of string | Either this or ak/sk     | -       | AWS Bearer Token for Bearer Token authentication                   |
| `awsAccessKey`            | string          | Either this or apiTokens | -       | AWS Access Key for AWS Signature V4 authentication                 |
| `awsSecretKey`            | string          | Either this or apiTokens | -       | AWS Secret Access Key for AWS Signature V4 authentication          |
| `awsRegion`               | string          | Required                 | -       | AWS region, e.g., us-east-1                                        |
| `bedrockAdditionalFields` | map             | Optional                 | -       | Additional inference parameters that the model supports            |

## Usage Examples

### Using OpenAI Protocol Proxy for Azure OpenAI Service

Using the basic Azure OpenAI service without configuring any context.

**Configuration Information**

```yaml
provider:
  type: azure
  apiTokens:
    - "YOUR_AZURE_OPENAI_API_TOKEN"
  azureServiceUrl: "https://YOUR_RESOURCE_NAME.openai.azure.com/openai/deployments/YOUR_DEPLOYMENT_NAME/chat/completions?api-version=2024-02-15-preview",
```

**Request Example**

```json
{
  "model": "gpt-3",
  "messages": [
    {
      "role": "user",
      "content": "Hello, who are you?"
    }
  ],
  "temperature": 0.3
}
```

**Response Example**

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
        "content": "Hello! I am an AI assistant, here to answer your questions and provide assistance. Is there anything I can help you with?",
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

### Using OpenAI Protocol Proxy for Qwen Service

Using Qwen service and configuring the mapping relationship between OpenAI large models and Qwen models.

**Configuration Information**

```yaml
provider:
  type: qwen
  apiTokens:
    - "YOUR_QWEN_API_TOKEN"
  modelMapping:
    'gpt-3': "qwen-turbo"
    'gpt-35-turbo': "qwen-plus"
    'gpt-4-turbo': "qwen-max"
    'gpt-4-*': "qwen-max"
    'gpt-4o': "qwen-vl-plus"
    'text-embedding-v1': 'text-embedding-v1'
    '*': "qwen-turbo"
```

**AI Conversation Request Example**

URL: http://your-domain/v1/chat/completions

Request Example:

```json
{
  "model": "gpt-3",
  "messages": [
    {
      "role": "user",
      "content": "Hello, who are you?"
    }
  ],
  "temperature": 0.3
}
```

Response Example:

```json
{
  "id": "c2518bd3-0f46-97d1-be34-bb5777cb3108",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "I am Qwen, an AI assistant developed by Alibaba Cloud. I can answer various questions, provide information, and engage in conversations with users. How can I assist you?"
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

**Multimodal Model API Request Example (Applicable to `qwen-vl-plus` and `qwen-vl-max` Models)**

URL: http://your-domain/v1/chat/completions

Request Example:

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
                    "text": "Where is this picture from?"
                }
            ]
        }
    ],
    "temperature": 0.3
}
```

Response Example:

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
                        "text": "This photo depicts a woman and a dog on a beach. As I cannot access specific geographical information, I cannot pinpoint the exact location of this beach. However, visually, it appears to be a sandy coastline along a coastal area with waves breaking on the shore. Such scenes can be found in many beautiful seaside locations worldwide. If you need more precise information, please provide additional context or descriptive details."
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

**Text Embedding Request Example**

URL: http://your-domain/v1/embeddings

Request Example:

```json
{
  "model": "text-embedding-v1",
  "input": "Hello"
}
```

Response Example:

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

### Using Qwen Service with Pure Text Context Information

Using Qwen service while configuring pure text context information.

**Configuration Information**

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

**Request Example**

```json
{
  "model": "gpt-3",
  "messages": [
    {
      "role": "user",
      "content": "Please summarize the content"
    }
  ],
  "temperature": 0.3
}
```

**Response Example**

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
        "content": "The content of this document is about..."
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

### Using Qwen Service with Native File Context

Uploading files to Qwen in advance to use them as context when utilizing its AI service.

**Configuration Information**

```yaml
provider:
  type: qwen
  apiTokens:
    - "YOUR_QWEN_API_TOKEN"
  modelMapping:
    "*": "qwen-long" # Qwen's file context can only be used in the qwen-long model
  qwenFileIds:
  - "file-fe-xxx"
  - "file-fe-yyy"
```

**Request Example**

```json
{
  "model": "gpt-4-turbo",
  "messages": [
    {
      "role": "user",
      "content": "Please summarize the content"
    }
  ],
  "temperature": 0.3
}
```

**Response Example**

```json
{
  "output": {
    "choices": [
      {
        "finish_reason": "stop",
        "message": {
          "role": "assistant",
          "content": "You uploaded two files, `context.txt` and `context_2.txt`, which seem to contain information about..."
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

### Forwards requests to AliCloud Bailian with the "original" protocol

**Configuration Information**

```yaml
activeProviderId: my-qwen
providers:
  - id: my-qwen
    type: qwen
    apiTokens:
      - "YOUR_DASHSCOPE_API_TOKEN"
    protocol: original
```

**Example Request**

```json
{
  "input": {
    "prompt": "What is Dubbo?"
  },
  "parameters": {},
  "debug": {}
}
```

**Example Response**

```json
{
  "output": {
    "finish_reason": "stop",
    "session_id": "677e7e8fbb874e1b84792b65042e1599",
    "text": "Apache Dubbo is a..."
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

### Using OpenAI Protocol Proxy for Doubao Service

```yaml
activeProviderId: my-doubao
providers:
- id: my-doubao
  type: doubao
  apiTokens:
    - YOUR_DOUBAO_API_KEY
  modelMapping:
    '*': YOUR_DOUBAO_ENDPOINT
  timeout: 1200000
```

### Using original Protocol Proxy for Coze applications

```yaml
provider:
  type: coze
  apiTokens:
    - YOUR_COZE_API_KEY
  protocol: original
```

### Utilizing Moonshot with its Native File Context

Upload files to Moonshot in advance and use its AI services based on file content.

**Configuration Information**

```yaml
provider:
  type: moonshot
  apiTokens:
    - "YOUR_MOONSHOT_API_TOKEN"
  moonshotFileId: "YOUR_MOONSHOT_FILE_ID",
  modelMapping:
    '*': "moonshot-v1-32k"
```

**Example Request**

```json
{
  "model": "gpt-4-turbo",
  "messages": [
    {
      "role": "user",
      "content": "Please summarize the content"
    }
  ],
  "temperature": 0.3
}
```

**Example Response**

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
        "content": "The content of the text is about a payment platform named ‘xxxx’..."
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

### Using OpenAI Protocol Proxy for Groq Service

**Configuration Information**

```yaml
provider:
  type: groq
  apiTokens:
    - "YOUR_GROQ_API_TOKEN"
```

**Example Request**

```json
{
  "model": "llama3-8b-8192",
  "messages": [
    {
      "role": "user",
      "content": "Hello, who are you?"
    }
  ]
}
```

**Example Response**

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

### Using OpenAI Protocol Proxy for Grok Service

**Configuration Information**

```yaml
provider:
  type: grok
  apiTokens:
    - "YOUR_GROK_API_TOKEN"
```

**Example Request**

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

**Example Response**

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

### Using OpenAI Protocol Proxy for OpenRouter Service

**Configuration Information**

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

**Example Request**

```json
{
  "model": "gpt-4",
  "messages": [
    {
      "role": "user",
      "content": "Hello, who are you?"
    }
  ],
  "temperature": 0.7
}
```

**Example Response**

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
        "content": "Hello! I am an AI assistant powered by OpenRouter. I can help answer questions, assist with creative tasks, engage in conversations, and more. How can I assist you today?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 12,
    "completion_tokens": 35,
    "total_tokens": 47
  }
}
```

### Using OpenAI Protocol Proxy for Fireworks AI Service

**Configuration Information**

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

**Request Example**

```json
{
  "model": "gpt-4",
  "messages": [
    {
      "role": "user",
      "content": "Hello, who are you?"
    }
  ],
  "temperature": 0.7,
  "max_tokens": 100
}
```

**Response Example**

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
        "content": "Hello! I am an AI assistant powered by Fireworks AI, based on the Llama 3.1 model. I can help answer questions, engage in conversations, and provide various information. How can I assist you today?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 15,
    "completion_tokens": 38,
    "total_tokens": 53
  }
}
```

### Using Auto Protocol Compatibility

The plugin now supports automatic protocol detection, capable of handling both OpenAI and Claude protocol format requests simultaneously.

**Configuration Information**

```yaml
provider:
  type: claude  # Provider with native Claude protocol support
  apiTokens:
    - "YOUR_CLAUDE_API_TOKEN"
  version: "2023-06-01"
```

**OpenAI Protocol Request Example**

URL: `http://your-domain/v1/chat/completions`

```json
{
  "model": "claude-3-opus-20240229",
  "max_tokens": 1024,
  "messages": [
    {
      "role": "user",
      "content": "Hello, who are you?"
    }
  ]
}
```

**Claude Protocol Request Example**

URL: `http://your-domain/v1/messages`

```json
{
  "model": "claude-3-opus-20240229",
  "max_tokens": 1024,
  "messages": [
    {
      "role": "user",
      "content": "Hello, who are you?"
    }
  ]
}
```

**Example Response**

Both protocol formats will return responses in their respective formats:

```json
{
  "id": "msg_01Jt3GzyjuzymnxmZERJguLK",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello, I am a conversation system developed by Anthropic, a company specializing in artificial intelligence. My name is Claude, a friendly and knowledgeable chatbot. Nice to meet you! I can engage in discussions on various topics, answer questions, provide suggestions, and assist you. I'll do my best to give you helpful responses. I hope we have a pleasant exchange!"
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

### Using Claude Code Mode

Claude Code is Anthropic's official CLI tool. By enabling `claudeCodeMode`, you can authenticate using Claude Code OAuth tokens:

**Configuration Information**

```yaml
provider:
  type: claude
  apiTokens:
    - "sk-ant-oat01-xxxxx"  # Claude Code OAuth Token
  claudeCodeMode: true  # Enable Claude Code mode
```

Once this mode is enabled, the plugin will automatically:
- Use Bearer Token authentication (instead of x-api-key)
- Set Claude Code-specific request headers and query parameters
- Inject Claude Code system prompt if not provided

**Request Example**

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

The plugin will automatically transform the request into Claude Code format, including:
- Adding system prompt: `"You are Claude Code, Anthropic's official CLI for Claude."`
- Setting appropriate authentication and request headers

### Using Intelligent Protocol Conversion

When the target provider doesn't natively support Claude protocol, the plugin automatically performs protocol conversion:

**Configuration Information**

```yaml
provider:
  type: qwen  # Doesn't natively support Claude protocol, auto-conversion applied
  apiTokens:
    - "YOUR_QWEN_API_TOKEN"
  modelMapping:
    'claude-3-opus-20240229': 'qwen-max'
    '*': 'qwen-turbo'
```

**Claude Protocol Request**

URL: `http://your-domain/v1/messages` (automatically converted to OpenAI protocol for provider)

```json
{
  "model": "claude-3-opus-20240229",
  "max_tokens": 1024,
  "messages": [
    {
      "role": "user",
      "content": "Hello, who are you?"
    }
  ]
}
```

### Using OpenAI Protocol Proxy for Hunyuan Service

**Configuration Information**

```yaml
provider:
  type: "hunyuan"
  hunyuanAuthKey: "<YOUR AUTH KEY>"
  apiTokens:
    - ""
  hunyuanAuthId: "<YOUR AUTH ID>"
  timeout: 1200000
  modelMapping:
    "*": "hunyuan-lite"
```

**Example Request**

Request script:

```shell
curl --location 'http://<your higress domain>/v1/chat/completions' \
--header 'Content-Type:  application/json' \
--data '{
  "model": "gpt-3",
  "messages": [
    {
      "role": "system",
      "content": "You are a professional developer!"
    },
    {
      "role": "user",
      "content": "Hello, who are you?"
    }
  ],
  "temperature": 0.3,
  "stream": false
}'
```

**Example Response**

```json
{
    "id": "fd140c3e-0b69-4b19-849b-d354d32a6162",
    "choices": [
        {
            "index": 0,
            "delta": {
                "role": "assistant",
                "content": "Hello! I am a professional developer."
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

### Using OpenAI Protocol Proxy for ERNIE Bot Service

**Configuration Information**

```yaml
provider:
  type: baidu
  apiTokens:
    - "YOUR_BAIDU_API_TOKEN"
  modelMapping:
    'gpt-3': "ERNIE-4.0"
    '*': "ERNIE-4.0"
```

**Request Example**

```json
{
    "model": "gpt-4-turbo",
    "messages": [
        {
            "role": "user",
            "content": "Hello, who are you?"
        }
    ],
    "stream": false
}
```

**Response Example**

```json
{
    "id": "as-e90yfg1pk1",
    "choices": [
        {
            "index": 0,
            "message": {
                "role": "assistant",
                "content": "Hello, I am ERNIE Bot. I can interact with people, answer questions, assist in creation, and efficiently provide information, knowledge, and inspiration."
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

### Using OpenAI Protocol Proxy for MiniMax Service

**Configuration Information**

```yaml
provider:
  type: minimax
  apiTokens:
    - "YOUR_MINIMAX_API_TOKEN"
  modelMapping:
    "gpt-3": "abab6.5g-chat"
    "gpt-4": "abab6.5-chat"
    "*": "abab6.5g-chat"
  minimaxGroupId: "YOUR_MINIMAX_GROUP_ID"
```

**Request Example**

```json
{
    "model": "gpt-4-turbo",
    "messages": [
        {
            "role": "user",
            "content": "Hello, who are you?"
        }
    ],
    "stream": false
}
```

**Response Example**

```json
{
    "id": "02b2251f8c6c09d68c1743f07c72afd7",
    "choices": [
        {
            "finish_reason": "stop",
            "index": 0,
            "message": {
                "content": "Hello! I am MM Intelligent Assistant, a large language model developed by MiniMax. I can help answer questions, provide information, and engage in conversations. How can I assist you?",
                "role": "assistant"
            }
        }
    ],
    "created": 1717760544,
    "model": "abab6.5s-chat",
    "object": "chat.completion",
    "usage": {
        "total_tokens": 106
    },
    "input_sensitive": false,
    "output_sensitive": false,
    "input_sensitive_type": 0,
    "output_sensitive_type": 0,
    "base_resp": {
        "status_code": 0,
        "status_msg": ""
    }
}
```

### Using OpenAI Protocol Proxy for 360 Brain Services

**Configuration Information**

```yaml
provider:
  type: ai360
  apiTokens:
    - "YOUR_AI360_API_TOKEN"
  modelMapping:
    "gpt-4o": "360gpt-turbo-responsibility-8k"
    "gpt-4": "360gpt2-pro"
    "gpt-3.5": "360gpt-turbo"
    "text-embedding-3-small": "embedding_s1_v1.2"
    "*": "360gpt-pro"
```

**Request Example**

```json
{
  "model": "gpt-4o",
  "messages": [
    {
      "role": "system",
      "content": "You are a professional developer!"
    },
    {
      "role": "user",
      "content": "Hello, who are you?"
    }
  ]
}
```

**Response Example**

```json
{
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "Hello, I am 360 Brain, a large language model. I can assist with answering various questions, providing information, engaging in conversations, and more. How can I assist you?"
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
      "content": "You are a professional developer!"
    },
    {
      "role": "user",
      "content": "Hello, who are you?"
    }
  ],
  "context": null
}
```

**Text Embedding Request Example**

**URL**: http://your-domain/v1/embeddings

**Request Example**

```json
{
  "input":["Hello"],
  "model":"text-embedding-3-small"
}
```

**Response Example**

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

### Using OpenAI Protocol Proxy for Cloudflare Workers AI Service

**Configuration Information**

```yaml
provider:
  type: cloudflare
  apiTokens:
    - "YOUR_WORKERS_AI_API_TOKEN"
  cloudflareAccountId: "YOUR_CLOUDFLARE_ACCOUNT_ID"
  modelMapping:
    "*": "@cf/meta/llama-3-8b-instruct"
```

**Request Example**

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

**Response Example**

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
        "content": "I am LLaMA, an AI assistant developed by Meta AI that can understand and respond to human input in a conversational manner. I'm not a human, but a computer program designed to simulate conversation and answer questions to the best of my knowledge. I can be used to generate text on a wide range of topics, from science and history to entertainment and culture."
      },
      "logprobs": null,
      "finish_reason": "stop"
    }
  ]
}
```

### Using OpenAI Protocol Proxy for Spark Service

**Configuration Information**

```yaml
provider:
  type: spark
  apiTokens:
    - "APIKey:APISecret"
  modelMapping:
    "gpt-4o": "generalv3.5"
    "gpt-4": "generalv3"
    "*": "general"
```

**Request Example**

```json
{
    "model": "gpt-4o",
    "messages": [
        {
            "role": "system",
            "content": "You are a professional developer!"
        },
        {
            "role": "user",
            "content": "Hello, who are you?"
        }
    ],
    "stream": false
}
```

**Response Example**

```json
{
    "id": "cha000c23c6@dx190ef0b4b96b8f2532",
    "choices": [
        {
            "index": 0,
            "message": {
                "role": "assistant",
                "content": "Hello! I am a professional developer skilled in programming and problem-solving. What can I assist you with?"
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

### Utilizing OpenAI Protocol Proxy for Gemini Services

**Configuration Information**

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

**Request Example**

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

**Response Example**

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

### Utilizing OpenAI Protocol Proxy for DeepL Text Translation Service

**Configuration Information**

```yaml
provider:
  type: deepl
  apiTokens:
    - "YOUR_DEEPL_API_TOKEN"
  targetLang: "ZH"
```

**Request Example**
Here, `model` denotes the service tier of DeepL and can only be either `Free` or `Pro`. The `content` field contains the text to be translated; within `role: system`, `content` may include context that influences the translation but isn't translated itself. For instance, when translating product names, including a product description as context could enhance translation quality.

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

**Response Example**

```json
{
  "choices": [
    {
      "index": 0,
      "message": { "name": "EN", "role": "assistant", "content": "operate a gambling establishment" }
    },
    {
      "index": 1,
      "message": { "name": "EN", "role": "assistant", "content": "Bank of China" }
    }
  ],
  "created": 1722747752,
  "model": "Free",
  "object": "chat.completion",
  "usage": {}
}
```

### Utilizing OpenAI Protocol Proxy for Together-AI Services

**Configuration Information**
```yaml
provider:
  type: together-ai
  apiTokens:
    - "YOUR_TOGETHER_AI_API_TOKEN"
  modelMapping:
    "*": "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo"
```

**Request Example**
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

**Response Example**
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

### Utilizing OpenAI Protocol Proxy for Google Vertex Services (Standard Mode)
**Configuration Information**
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

**Request Example**
```json
{
  "model": "gemini-2.0-flash-001",
  "messages": [
    {
      "role": "user",
      "content": "Who are you?"
    }
  ],
  "stream": false
}
```

**Response Example**
```json
{
  "id": "chatcmpl-0000000000000",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! I am the Gemini model provided by Vertex AI, developed by Google. I can answer questions, provide information, and assist in completing various tasks. How can I help you today?"
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

### Utilizing OpenAI Protocol Proxy for Google Vertex Services (Express Mode)

Express Mode is a simplified access mode for Vertex AI. You only need an API Key to get started quickly.

**Configuration Information**
```yaml
provider:
  type: vertex
  apiTokens:
    - "YOUR_API_KEY"
```

**Request Example**
```json
{
  "model": "gemini-2.5-flash",
  "messages": [
    {
      "role": "user",
      "content": "Who are you?"
    }
  ],
  "stream": false
}
```

**Response Example**
```json
{
  "id": "chatcmpl-0000000000000",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! I am Gemini, an AI assistant developed by Google. How can I help you today?"
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

### Utilizing OpenAI Protocol Proxy for Google Vertex Services (OpenAI Compatible Mode)

OpenAI Compatible Mode uses Vertex AI's OpenAI-compatible Chat Completions API. Both requests and responses use OpenAI format, requiring no protocol conversion.

**Configuration Information**
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

**Request Example**
```json
{
  "model": "gpt-4",
  "messages": [
    {
      "role": "user",
      "content": "Hello, who are you?"
    }
  ],
  "stream": false
}
```

**Response Example**
```json
{
  "id": "chatcmpl-abc123",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! I am Gemini, an AI model developed by Google. I can help answer questions, provide information, and engage in conversations. How can I assist you today?"
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

### Utilizing OpenAI Protocol Proxy for Google Vertex Image Generation

Vertex AI supports image generation using Gemini models. Through the ai-proxy plugin, you can use OpenAI's `/v1/images/generations` API to call Vertex AI's image generation capabilities.

**Configuration Information**

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

**Using curl**

```bash
curl -X POST "http://your-gateway-address/v1/images/generations" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.0-flash-exp",
    "prompt": "A cute orange cat napping in the sunshine",
    "size": "1024x1024"
  }'
```

**Using OpenAI Python SDK**

```python
from openai import OpenAI

client = OpenAI(
    api_key="any-value",  # Can be any value, authentication is handled by the gateway
    base_url="http://your-gateway-address/v1"
)

response = client.images.generate(
    model="gemini-2.0-flash-exp",
    prompt="A cute orange cat napping in the sunshine",
    size="1024x1024",
    n=1
)

# Get the generated image (base64 encoded)
image_data = response.data[0].b64_json
print(f"Generated image (base64): {image_data[:100]}...")
```

**Response Example**

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

**Supported Size Parameters**

Vertex AI supported aspect ratios: `1:1`, `3:2`, `2:3`, `3:4`, `4:3`, `4:5`, `5:4`, `9:16`, `16:9`, `21:9`

Vertex AI supported resolutions (imageSize): `1k`, `2k`, `4k`

| OpenAI size parameter | Vertex AI aspectRatio | Vertex AI imageSize |
|-----------------------|----------------------|---------------------|
| 256x256               | 1:1                  | 1k                  |
| 512x512               | 1:1                  | 1k                  |
| 1024x1024             | 1:1                  | 1k                  |
| 1792x1024             | 16:9                 | 2k                  |
| 1024x1792             | 9:16                 | 2k                  |
| 2048x2048             | 1:1                  | 2k                  |
| 4096x4096             | 1:1                  | 4k                  |
| 1536x1024             | 3:2                  | 2k                  |
| 1024x1536             | 2:3                  | 2k                  |
| 1024x768              | 4:3                  | 1k                  |
| 768x1024              | 3:4                  | 1k                  |
| 1280x1024             | 5:4                  | 1k                  |
| 1024x1280             | 4:5                  | 1k                  |
| 2560x1080             | 21:9                 | 2k                  |

**Notes**

- Image generation uses Gemini models (e.g., `gemini-2.0-flash-exp`, `gemini-3-pro-image-preview`). Model availability may vary by region
- The returned image data is in base64 encoded format (`b64_json`)
- Content safety filtering levels can be configured via `geminiSafetySetting`
- If you need model mapping (e.g., mapping `dall-e-3` to a Gemini model), configure `modelMapping`

### Utilizing OpenAI Protocol Proxy for AWS Bedrock Services

AWS Bedrock supports two authentication methods:

#### Method 1: Using AWS Access Key/Secret Key Authentication (AWS Signature V4)

**Configuration Information**
```yaml
provider:
  type: bedrock
  awsAccessKey: "YOUR_AWS_ACCESS_KEY_ID"
  awsSecretKey: "YOUR_AWS_SECRET_ACCESS_KEY"
  awsRegion: "us-east-1"
  bedrockAdditionalFields:
    top_k: 200
```

#### Method 2: Using Bearer Token Authentication (suitable for IAM Identity Center and similar scenarios)

**Configuration Information**
```yaml
provider:
  type: bedrock
  apiTokens:
    - "YOUR_AWS_BEARER_TOKEN"
  awsRegion: "us-east-1"
  bedrockAdditionalFields:
    top_k: 200
```

**Request Example**
```json
{
  "model": "us.anthropic.claude-3-5-haiku-20241022-v1:0",
  "messages": [
    {
      "role": "user",
      "content": "who are you"
    }
  ],
  "stream": false
}
```

**Response Example**
```json
{
  "id": "d52da49d-daf3-49d9-a105-0b527481fe14",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "I'm Claude, an AI created by Anthropic. I aim to be helpful, honest, and harmless. I won't pretend to be human, and I'll always try to be direct and truthful about what I am and what I can do."
      },
      "finish_reason": "stop"
    }
  ],
  "created": 1749659050,
  "model": "arn:aws:bedrock:us-west-2::foundation-model/anthropic.claude-3-5-haiku-20241022-v1:0",
  "object": "chat.completion",
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 57,
    "total_tokens": 67
  }
}
```

### Utilizing OpenAI Protocol Proxy for NVIDIA Triton Interference Server Services

**Configuration Information**

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

**Request Example**

```json
{
  "model": "gpt2",
  "messages": [
    {
      "role": "user",
      "content": "Hi, who are you？"
    }
  ],
  "stream": false
}
```
**Response Example**

```json
{
    "choices": [
        {
            "index": 0,
            "message": {
                "role": "assistant",
                "content": "I am a lagguage model."
            },
            "finish_reason": "stop",
        }
    ],
    "model": "gpt2",
}
```

### Using Context Cleanup Commands

After configuring context cleanup commands, users can actively clear conversation history by sending specific messages, achieving a "start over" effect.

**Configuration**

```yaml
provider:
  type: qwen
  apiTokens:
    - "YOUR_QWEN_API_TOKEN"
  modelMapping:
    "*": "qwen-turbo"
  contextCleanupCommands:
    - "clear context"
    - "/clear"
    - "start over"
    - "new conversation"
```

**Request Example**

When a user sends a request containing a cleanup command:

```json
{
  "model": "gpt-3",
  "messages": [
    {
      "role": "system",
      "content": "You are an assistant"
    },
    {
      "role": "user",
      "content": "Hello"
    },
    {
      "role": "assistant",
      "content": "Hello! How can I help you?"
    },
    {
      "role": "user",
      "content": "What's the weather like today"
    },
    {
      "role": "assistant",
      "content": "Sorry, I cannot get real-time weather information."
    },
    {
      "role": "user",
      "content": "clear context"
    },
    {
      "role": "user",
      "content": "Let's start a new topic, introduce yourself"
    }
  ]
}
```

**Actual Request Sent to AI Service**

The plugin automatically removes the cleanup command and all non-system messages before it:

```json
{
  "model": "qwen-turbo",
  "messages": [
    {
      "role": "system",
      "content": "You are an assistant"
    },
    {
      "role": "user",
      "content": "Let's start a new topic, introduce yourself"
    }
  ]
}
```

**Notes**

- The cleanup command must exactly match the configured string; partial matches will not trigger cleanup
- When multiple cleanup commands exist in messages, only the last matching command is processed
- Cleanup preserves all system messages and removes user, assistant, and tool messages before the command
- All messages after the cleanup command are preserved

## Full Configuration Example

### Kubernetes Example

Here's a full plugin configuration example using the OpenAI protocol proxy for Groq services.

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
          - "YOUR_API_TOKEN"
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
    higress.io/proxy-ssl-server-name: "on"
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

Access Example:

```bash
curl "http://<YOUR-DOMAIN>/v1/chat/completions" -H "Content-Type: application/json" -d '{
  "model": "llama3-8b-8192",
  "messages": [
    {
      "role": "user",
      "content": "hello, who are you?"
    }
  ]
}'
```

### Docker-Compose Example

`docker-compose.yml` configuration file:

```yaml
version: '3.7'
services:
  envoy:
    image: higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/envoy:1.20
    entrypoint: /usr/local/bin/envoy
    # Enables debug level logging for easier debugging
    command: -c /etc/envoy/envoy.yaml --component-log-level wasm:debug
    networks:
      - higress-net
    ports:
      - "10000:10000"
    volumes:
      - ./envoy.yaml:/etc/envoy/envoy.yaml
      - ./plugin.wasm:/etc/envoy/plugin.wasm
networks:
  higress-net: {}
```

`envoy.yaml` configuration file:

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
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                scheme_header_transformation:
                  scheme_to_overwrite: https
                stat_prefix: ingress_http
                # Outputs envoy logs to stdout
                access_log:
                  - name: envoy.access_loggers.stdout
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog
                # Modify as needed
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: local_service
                      domains: [ "*" ]
                      routes:
                        - match:
                            prefix: "/"
                          route:
                            cluster: claude
                            timeout: 300s
                http_filters:
                  - name: claude
                    typed_config:
                      "@type": type.googleapis.com/udpa.type.v1.TypedStruct
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
                            "@type": "type.googleapis.com/google.protobuf.StringValue"
                            value: | # Plugin configuration
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
                      address: api.anthropic.com # Service address
                      port_value: 443
      transport_socket:
        name: envoy.transport_sockets.tls
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
          "sni": "api.anthropic.com"
```

Access Example:

```bash
curl "http://localhost:10000/v1/chat/completions"  -H "Content-Type: application/json"  -d '{
  "model": "claude-3-opus-20240229",
  "max_tokens": 1024,
  "messages": [
    {
      "role": "user",
      "content": "hello, who are you?"
    }
  ]
}'
```
