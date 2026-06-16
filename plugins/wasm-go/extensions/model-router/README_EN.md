## Feature Description
The `model-router` plugin implements routing functionality based on the model parameter in LLM protocols.

## Configuration Fields

| Name                 | Data Type        | Requirement               | Default Value            | Description                                                  |
| -----------          | --------------- | ----------------------- | ------                   | -------------------------------------------           |
| `modelKey`           | string          | Optional                | model                    | Location of the model parameter in the request body          |
| `addProviderHeader`  | string          | Optional                | -                        | Which request header to add the provider name parsed from the model parameter |
| `modelToHeader`      | string          | Optional                | -                        | Which request header to directly add the model parameter to  |
| `enableOnPathSuffix` | array of string | Optional                | ["/completions","/embeddings","/images/generations","/audio/speech","/fine_tuning/jobs","/moderations","/image-synthesis","/video-synthesis","/rerank","/messages"] | Only effective for requests with these specific path suffixes, can be configured as "*" to match all paths |
| `keepOriginalModelName` | bool         | Optional                | false                    | Used with `addProviderHeader`. When set to true, the provider is still extracted into the header, but the model field in the request body is not rewritten |

## Runtime Properties

Plugin execution phase: Authentication phase
Plugin execution priority: 900

## Effect Description

### Routing Based on Model Parameter

The following configuration is needed:

```yaml
modelToHeader: x-higress-llm-model
```

The plugin extracts the model parameter from the request and sets it to the x-higress-llm-model request header for subsequent routing. For example, the original LLM request body is:

```json
{
    "model": "qwen-long",
    "frequency_penalty": 0,
    "max_tokens": 800,
    "stream": false,
    "messages": [{
        "role": "user",
        "content": "What is the GitHub address of the Higress project's main repository?"
    }],
    "presence_penalty": 0,
    "temperature": 0.7,
    "top_p": 0.95
}
```

After processing by this plugin, the following request header will be added (can be used for route matching):

x-higress-llm-model: qwen-long

### Extracting Provider Field from Model Parameter for Routing

> Note that this mode requires the client to specify the provider in the model parameter using the `/` delimiter

The following configuration is needed:

```yaml
addProviderHeader: x-higress-llm-provider
```

The plugin extracts the provider part (if any) from the model parameter in the request, sets it to the x-higress-llm-provider request header for subsequent routing, and rewrites the model parameter to only contain the model name part. For example, the original LLM request body is:

```json
{
    "model": "dashscope/qwen-long",
    "frequency_penalty": 0,
    "max_tokens": 800,
    "stream": false,
    "messages": [{
        "role": "user",
        "content": "What is the GitHub address of the Higress project's main repository?"
    }],
    "presence_penalty": 0,
    "temperature": 0.7,
    "top_p": 0.95
}
```

After processing by this plugin, the following request header will be added (can be used for route matching):

x-higress-llm-provider: dashscope

The original LLM request body will be changed to:

```json
{
    "model": "qwen-long",
    "frequency_penalty": 0,
    "max_tokens": 800,
    "stream": false,
    "messages": [{
        "role": "user",
        "content": "What is the GitHub address of the Higress project's main repository?"
    }],
    "presence_penalty": 0,
    "temperature": 0.7,
    "top_p": 0.95
}
```

### Preserving Original Model Name (keepOriginalModelName)

When using AI model aggregation platforms (e.g. Bailian/DashScope) with third-party models, some model names inherently contain `/` (e.g. `MiniMax/MiniMax-M2.7`) and are not in `provider/model` format. In this case, using `addProviderHeader` would incorrectly rewrite the model field in the request body.

By setting `keepOriginalModelName: true`, you can keep the provider header extraction while preserving the original model field in the request body:

```yaml
addProviderHeader: x-higress-llm-provider
keepOriginalModelName: true
```

For example, with model `MiniMax/MiniMax-M2.7`, after processing by the plugin:
- Request header `x-higress-llm-provider` is set to `MiniMax`
- The model field in the request body remains `MiniMax/MiniMax-M2.7` (not rewritten)
