## Function Description
The `model-mapper` plugin supports bidirectional mapping of the `model` field in LLM protocols:

- Request direction: rewrites client model names to upstream model names.
- Response direction: rewrites upstream model names back to the original client model names (supports both JSON and SSE responses).

## Configuration Fields

| Name                 | Data Type        | Filling Requirement                | Default Value                   | Description                                                                                                                                                                                                                                                         |
| -----------          | --------------- | -----------------------            | ------                          | -------------------------------------------                                                                                                                                                                                                                  |
| `modelKey`           | string          | Optional                           | model                           | The location of the model parameter in the request body.                                                                                                                                                                                                            |
| `enableResponseMapping` | bool         | Optional                           | false                           | Whether to enable response-side model rewrite. When enabled, if request model rewrite happened, upstream model names in responses are rewritten back to client model names (supports JSON and SSE). When disabled, only request rewrite is applied. |
| `responseModelPaths`  | array of string | Optional                           | ["model", "body.model", "response.model", "message.model", "modelVersion"] | Gjson paths for locating the model field in responses, tried in order. User-configured paths are appended to the defaults for non-standard providers. |
| `modelMapping`       | map of string   | Optional                           | -                               | AI model mapping table, used to map the model names in the request to the model names supported by the service provider.<br/>1. Supports prefix matching. For example, use "gpt-3-*" to match all models whose names start with “gpt-3-”;<br/>2. Supports using "*" as the key to configure a generic fallback mapping relationship;<br/>3. If the target name in the mapping is an empty string "", it means to keep the original model name. |
| `enableOnPathSuffix` | array of string | Optional                           | ["/completions","/embeddings","/images/generations","/audio/speech","/fine_tuning/jobs","/moderations","/image-synthesis","/video-synthesis"]        | Only applies to requests with these specific path suffixes.                                                                                                                                           |

## Runtime Properties

Plugin execution phase: Authentication phase
Plugin execution priority: 800

## Effect Description

With the following configuration:

```yaml
modelMapping:
  'gpt-4-*': "qwen-max"
  'gpt-4o': "qwen-vl-plus"
  '*': "qwen-turbo"
```

After enabling, model parameters starting with `gpt-4-` will be rewritten to `qwen-max`, `gpt-4o` will be rewritten to `qwen-vl-plus`, and all other models will be rewritten to `qwen-turbo`.

For example, if the original request was:

```json
{
    "model": "gpt-4o",
    "frequency_penalty": 0,
    "max_tokens": 800,
    "stream": false,
    "messages": [{
        "role": "user",
        "content": "What is the GitHub address of the main repository for the higress project?"
    }],
    "presence_penalty": 0,
    "temperature": 0.7,
    "top_p": 0.95
}
```


After processing by this plugin, the original LLM request body will be modified to:

```json
{
    "model": "qwen-vl-plus",
    "frequency_penalty": 0,
    "max_tokens": 800,
    "stream": false,
    "messages": [{
        "role": "user",
        "content": "What is the GitHub address of the main repository for the higress project?"
    }],
    "presence_penalty": 0,
    "temperature": 0.7,
    "top_p": 0.95
}
```

If the upstream response `model` is `qwen-vl-plus`, the plugin will rewrite it back to `gpt-4o` before returning to the client.

### Streaming Response (SSE)

For `text/event-stream` responses, the plugin incrementally processes JSON in SSE `data:` lines and rewrites the `model` field.

For example, upstream event:

```text
event: message_start
data: {"type":"message_start","message":{"model":"qwen-vl-plus"}}
```

Returned to the client:

```text
event: message_start
data: {"type":"message_start","message":{"model":"gpt-4o"}}
```

## Notes

- Response rewrite is enabled only when request model rewrite happened for the same stream.
- Response rewrite is applied only when the response model value exactly matches the mapped upstream model, to avoid unintended rewrites.
- Avoid enabling multiple plugins that rewrite `model` simultaneously, or make sure plugin execution order is explicitly controlled.
