# 功能说明
`model-mapper` 插件支持对 LLM 协议中的 `model` 字段进行双向映射：

- 请求方向：将客户端请求中的模型名映射为上游模型名
- 响应方向：将上游返回的模型名回写为客户端原始模型名（支持 JSON 与 SSE）

# 配置字段

| 名称                 | 数据类型        | 填写要求                | 默认值                   | 描述                                                                                                                                                                                                                                                         |
| -----------          | --------------- | ----------------------- | ------                   | -------------------------------------------                                                                                                                                                                                                                  |
| `modelKey`           | string          | 选填                    | model                    | 请求body中model参数的位置                                                                                                                                                                                                                                    |
| `enableResponseMapping` | bool         | 选填                    | false                    | 是否启用响应方向模型名回写。开启时会在请求发生映射后，将响应中的上游模型名回写为客户端原模型名（支持 JSON 与 SSE）；关闭时仅改写请求，不处理响应。 |
| `responseModelPaths`  | array of string | 选填                   | ["model", "body.model", "response.model", "message.model", "modelVersion"] | 响应中定位模型名的 gjson 路径列表，按顺序优先匹配。用户配置的路径会追加到默认列表之后，用于支持非标准 provider。 |
| `modelMapping`       | map of string   | 选填                    | -                        | AI 模型映射表，用于将请求中的模型名称映射为服务提供商支持模型名称。<br/>1. 支持前缀匹配。例如用 "gpt-3-*" 匹配所有名称以“gpt-3-”开头的模型；<br/>2. 支持使用 "*" 为键来配置通用兜底映射关系；<br/>3. 如果映射的目标名称为空字符串 ""，则表示保留原模型名称。 |
| `enableOnPathSuffix` | array of string | 选填                    | ["/completions","/embeddings","/images/generations","/audio/speech","/fine_tuning/jobs","/moderations","/image-synthesis","/video-synthesis"] | 只对这些特定路径后缀的请求生效|


## 效果说明

如下配置：

```yaml
modelMapping:
  'gpt-4-*': "qwen-max"
  'gpt-4o': "qwen-vl-plus"
  '*': "qwen-turbo"
```

开启后，`gpt-4-` 开头的模型参数会被改写为 `qwen-max`，`gpt-4o` 会被改写为 `qwen-vl-plus`，其他所有模型会被改写为 `qwen-turbo`。

例如原本的请求是：

```json
{
    "model": "gpt-4o",
    "frequency_penalty": 0,
    "max_tokens": 800,
    "stream": false,
    "messages": [{
        "role": "user",
        "content": "higress项目主仓库的github地址是什么"
    }],
    "presence_penalty": 0,
    "temperature": 0.7,
    "top_p": 0.95
}
```


经过这个插件后，原始的 LLM 请求体将被改成：

```json
{
    "model": "qwen-vl-plus",
    "frequency_penalty": 0,
    "max_tokens": 800,
    "stream": false,
    "messages": [{
        "role": "user",
        "content": "higress项目主仓库的github地址是什么"
    }],
    "presence_penalty": 0,
    "temperature": 0.7,
    "top_p": 0.95
}
```

如果上游响应体中的 `model` 为 `qwen-vl-plus`，则该插件会将其回写为 `gpt-4o` 再返回给客户端。

### 流式响应（SSE）示例

对于 `text/event-stream` 响应，插件会按事件增量处理 `data:` 行中的 JSON，并替换其中的 `model` 字段。

例如上游事件：

```text
event: message_start
data: {"type":"message_start","message":{"model":"qwen-vl-plus"}}
```

返回给客户端会变为：

```text
event: message_start
data: {"type":"message_start","message":{"model":"gpt-4o"}}
```

## 注意事项

- 响应回写仅在请求方向发生了模型映射时生效。
- 仅当响应中的模型值与请求映射后的目标模型值一致时才会回写，避免误改。
- 建议避免与其他会修改 `model` 的插件同时启用，或确保执行顺序可控，否则可能出现映射链冲突。
