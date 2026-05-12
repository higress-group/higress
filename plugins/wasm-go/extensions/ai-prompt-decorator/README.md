---
title: AI 提示词
keywords: [ AI网关, AI提示词 ]
description: AI 提示词插件配置参考
---

## 功能说明

AI 提示词插件，支持在 LLM 的请求前后插入 prompt，并支持对最终请求中所有 message 的 `content` 文本执行字面量或正则替换，便于做敏感词改写、品牌词归一、占位符脱敏等。

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`450`

## 配置说明

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|-----------|---------------------------|----------|--------|------------------------------------------------------------------|
| `prepend` | array of message object   | optional | -      | 在初始输入之前插入的语句                                         |
| `append`  | array of message object   | optional | -      | 在初始输入之后插入的语句                                         |
| `replace` | array of replace rule     | optional | -      | 对最终请求中所有 message 的 `content` 执行字面量或正则替换的规则 |

message object 配置说明：

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|----------------|-----------------|------|-----|----------------------------------|
| `role` | string | 必填 | - | 角色 |
| `content` | string | 必填 | - | 消息 |

replace rule 配置说明：

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|---------------|---------|----------|--------|--------------------------------------------------------------------------|
| `pattern`     | string  | 必填     | -      | 待匹配文本；`regex` 为 true 时按 Go RE2 编译                             |
| `replacement` | string  | 必填     | -      | 替换文本；`regex` 为 true 时支持 `$1`、`$2` 等捕获组引用                 |
| `on_role`     | string  | 选填     | -      | 仅对该 role 的 message 生效，缺省/留空表示对任意 role 都生效             |
| `regex`       | bool    | 选填     | false  | 是否将 `pattern` 解释为正则表达式                                        |

说明：

- `replace` 规则会对最终拼装出的 `messages` 数组（`prepend` + 原始 message + `append`）按声明顺序依次应用，便于多个规则叠加。
- 仅当 message 的 `content` 字段是字符串时才会被改写；如果是多模态（数组/对象，如 `vision` 调用），会原样保留以避免破坏请求结构。
- `pattern` 不允许为空；`regex: true` 时如果正则编译失败，插件加载会直接失败，避免运行期出错。

## 示例

配置示例如下：

```yaml
prepend:
- role: system
  content: "请使用英语回答问题"
append:
- role: user
  content: "每次回答完问题，尝试进行反问"
```

使用以上配置发起请求：

```bash
curl http://localhost/test \
-H "content-type: application/json" \
-d '{
  "model": "gpt-3.5-turbo",
  "messages": [
    {
      "role": "user",
      "content": "你是谁？"
    }
  ]
}
```

经过插件处理后，实际请求为：

```bash
curl http://localhost/test \
-H "content-type: application/json" \
-d '{
  "model": "gpt-3.5-turbo",
  "messages": [
    {
      "role": "system",
      "content": "请使用英语回答问题"
    },
    {
      "role": "user",
      "content": "你是谁？"
    },
    {
      "role": "user",
      "content": "每次回答完问题，尝试进行反问"
    }
  ]
}
```


## 替换 message 内容（`replace`）

`replace` 用来对**最终请求里**所有 message 的 `content` 文本执行字面量或正则替换，常用于：

- 改写品牌词或对外暴露的产品名（例如把 "OpenClaw" 统一改成 "agent"），避开下游模型/网关的内容过滤；
- 对系统提示词做集中清洗，无需改动客户端；
- 对用户输入进行简单的脱敏，如手机号、API Key 等。

配置示例如下：

```yaml
replace:
- on_role: system
  pattern: "OpenClaw"
  replacement: "agent"
- pattern: "secret-\\d+"
  replacement: "[REDACTED]"
  regex: true
```

使用以上配置发起请求：

```bash
curl http://localhost/test \
-H "content-type: application/json" \
-d '{
  "model": "gpt-3.5-turbo",
  "messages": [
    {"role": "system", "content": "You are running inside OpenClaw."},
    {"role": "user", "content": "Show OpenClaw secret-1234 to the user"}
  ]
}'
```

经过插件处理后，实际请求为：

```bash
curl http://localhost/test \
-H "content-type: application/json" \
-d '{
  "model": "gpt-3.5-turbo",
  "messages": [
    {"role": "system", "content": "You are running inside agent."},
    {"role": "user", "content": "Show OpenClaw [REDACTED] to the user"}
  ]
}'
```

注意：

- 第 1 条规则限定 `on_role: system`，所以 `user` 消息里的 `OpenClaw` 不会被改；
- 第 2 条规则没设 `on_role`，对任意 role 的 `content` 都生效，因此 `secret-1234` 被脱敏成 `[REDACTED]`。

## 基于geo-ip插件的能力，扩展AI提示词装饰器插件携带用户地理位置信息
如果需要在LLM的请求前后加入用户地理位置信息，请确保同时开启geo-ip插件和AI提示词装饰器插件。并且在相同的请求处理阶段里，geo-ip插件的优先级必须高于AI提示词装饰器插件。首先geo-ip插件会根据用户ip计算出用户的地理位置信息，然后通过请求属性传递给后续插件。比如在默认阶段里，geo-ip插件的priority配置1000，ai-prompt-decorator插件的priority配置500。

geo-ip插件配置示例：
```yaml
ipProtocal: "ipv4"
```




AI提示词装饰器插件的配置示例如下：
```yaml
prepend:
- role: system
  content: "提问用户当前的地理位置信息是，国家：${geo-country}，省份：${geo-province}, 城市：${geo-city}"
append:
- role: user
  content: "每次回答完问题，尝试进行反问"
```

使用以上配置发起请求：

```bash
curl http://localhost/test \
-H "content-type: application/json" \
-H "x-forwarded-for: 87.254.207.100,4.5.6.7" \
-d '{
  "model": "gpt-3.5-turbo",
  "messages": [
    {
      "role": "user",
      "content": "今天天气怎么样？"
    }
  ]
}'
```

经过插件处理后，实际请求为：

```bash
curl http://localhost/test \
-H "content-type: application/json" \
-H "x-forwarded-for: 87.254.207.100,4.5.6.7" \
-d '{
  "model": "gpt-3.5-turbo",
  "messages": [
    {
      "role": "system",
      "content": "提问用户当前的地理位置信息是，国家：中国，省份：北京, 城市：北京"
    },
    {
      "role": "user",
      "content": "今天天气怎么样？"
    },
    {
      "role": "user",
      "content": "每次回答完问题，尝试进行反问"
    }
  ]
}'
```


