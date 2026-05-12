## 功能说明
`model-router`插件实现了基于LLM协议中的model参数路由的功能

## 配置字段

| 名称                 | 数据类型        | 填写要求                | 默认值                   | 描述                                                  |
| -----------          | --------------- | ----------------------- | ------                   | -------------------------------------------           |
| `modelKey`           | string          | 选填                    | model                    | 请求body中model参数的位置                             |
| `addProviderHeader`  | string          | 选填                    | -                        | 从model参数中解析出的provider名字放到哪个请求header中 |
| `modelToHeader`      | string          | 选填                    | -                        | 直接将model参数放到哪个请求header中                   |
| `enableOnPathSuffix` | array of string | 选填                    | ["/completions","/embeddings","/images/generations","/audio/speech","/fine_tuning/jobs","/moderations","/image-synthesis","/video-synthesis","/rerank","/messages"] | 只对这些特定路径后缀的请求生效，可以配置为 "*" 以匹配所有路径 |
| `autoRouting`        | object          | 选填                    | -                        | 自动路由配置，详见下方说明                            |

### autoRouting 配置

| 名称           | 数据类型        | 填写要求 | 默认值 | 描述                                                         |
| -------------- | --------------- | -------- | ------ | ------------------------------------------------------------ |
| `enable`       | bool            | 必填     | false  | 是否启用自动路由功能                                         |
| `defaultModel` | string          | 选填     | -      | 当没有规则匹配时使用的默认模型                               |
| `rules`        | array of object | 选填     | -      | 路由规则数组，按顺序匹配                                     |

### rules 配置

| 名称      | 数据类型 | 填写要求 | 描述                                                         |
| --------- | -------- | -------- | ------------------------------------------------------------ |
| `pattern` | string   | 必填     | 正则表达式，用于匹配用户消息内容                             |
| `model`   | string   | 必填     | 匹配成功时设置的模型名称，将设置到 `x-higress-llm-model` 请求头 |

## 运行属性

插件执行阶段：认证阶段
插件执行优先级：900

## 效果说明

### 基于 model 参数进行路由

需要做如下配置：

```yaml
modelToHeader: x-higress-llm-model
```

插件会将请求中 model 参数提取出来，设置到 x-higress-llm-model 这个请求 header 中，用于后续路由，举例来说，原生的 LLM 请求体是：

```json
{
    "model": "qwen-long",
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

经过这个插件后，将添加下面这个请求头(可以用于路由匹配)：

x-higress-llm-model: qwen-long

### 提取 model 参数中的 provider 字段用于路由

> 注意这种模式需要客户端在 model 参数中通过`/`分隔的方式，来指定 provider

需要做如下配置：

```yaml
addProviderHeader: x-higress-llm-provider
```

插件会将请求中 model 参数的 provider 部分（如果有）提取出来，设置到 x-higress-llm-provider 这个请求 header 中，用于后续路由，并将 model 参数重写为模型名称部分。举例来说，原生的 LLM 请求体是：

```json
{
    "model": "dashscope/qwen-long",
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

经过这个插件后，将添加下面这个请求头(可以用于路由匹配)：

x-higress-llm-provider: dashscope

原始的 LLM 请求体将被改成：

```json
{
    "model": "qwen-long",
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

### 自动路由模式（基于用户消息内容）

当请求中的 model 参数设置为 `higress/auto` 时，插件会自动分析用户消息内容，并根据配置的正则规则选择合适的模型进行路由。

配置示例：

```yaml
autoRouting:
  enable: true
  defaultModel: "qwen-turbo"
  rules:
    - pattern: "(?i)(画|绘|生成图|图片|image|draw|paint)"
      model: "qwen-vl-max"
    - pattern: "(?i)(代码|编程|code|program|function|debug)"
      model: "qwen-coder"
    - pattern: "(?i)(翻译|translate|translation)"
      model: "qwen-turbo"
    - pattern: "(?i)(数学|计算|math|calculate)"
      model: "qwen-math"
```

#### 工作原理

1. 当检测到请求体中的 model 参数值为 `higress/auto` 时，触发自动路由逻辑
2. 从请求体的 `messages` 数组中提取最后一个 `role` 为 `user` 的消息内容
3. 按配置的规则顺序，依次使用正则表达式匹配用户消息
4. 匹配成功时，将对应的 model 值设置到 `x-higress-llm-model` 请求头
5. 如果所有规则都未匹配，则使用 `defaultModel` 配置的默认模型
6. 如果未配置 `defaultModel` 且无规则匹配，则不设置路由头（会记录警告日志）

#### 使用示例

客户端请求：

```json
{
    "model": "higress/auto",
    "messages": [
        {
            "role": "system",
            "content": "你是一个有帮助的助手"
        },
        {
            "role": "user",
            "content": "请帮我画一只可爱的小猫"
        }
    ]
}
```

由于用户消息中包含"画"关键词，匹配到第一条规则，插件会设置请求头：

```
x-higress-llm-model: qwen-vl-max
```

#### 支持的消息格式

自动路由支持两种常见的 content 格式：

1. **字符串格式**（标准文本消息）：
```json
{
    "role": "user",
    "content": "用户消息内容"
}
```

2. **数组格式**（多模态消息，如包含图片）：
```json
{
    "role": "user",
    "content": [
        {"type": "text", "text": "用户消息内容"},
        {"type": "image_url", "image_url": {"url": "..."}}
    ]
}
```

对于数组格式，插件会提取最后一个 `type` 为 `text` 的内容进行匹配。

#### 正则表达式说明

- 规则按配置顺序依次匹配，第一个匹配成功的规则生效
- 支持标准 Go 正则语法
- 推荐使用 `(?i)` 标志实现大小写不敏感匹配
- 使用 `|` 可以匹配多个关键词
