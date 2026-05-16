---
title: AI 数据脱敏
keywords: [higress,ai data masking]
description: AI 数据脱敏插件配置参考
---

## 功能说明

  对请求/返回中的敏感词拦截、替换

![image](https://img.alicdn.com/imgextra/i4/O1CN0156Wtko1T9JO0RiWow_!!6000000002339-0-tps-1314-638.jpg)

### 处理数据范围
  - openai协议：请求/返回对话内容
  - jsonpath：只处理指定字段
  - raw：整个请求/返回body

### 敏感词拦截
  - 处理数据范围中出现敏感词直接拦截，返回预设错误信息
  - 支持系统内置敏感词库和自定义敏感词

### 敏感词替换
  - 将请求数据中出现的敏感词替换为脱敏字符串，传递给后端服务。可保证敏感数据不出域
  - 部分脱敏数据在后端服务返回后可进行还原
  - 自定义规则支持标准正则和grok规则，替换字符串支持变量替换

## 运行属性

插件执行阶段：`认证阶段`
插件执行优先级：`991`

## 配置字段

| 名称 | 数据类型 | 默认值 | 描述 |
| -------- | --------  | -------- | -------- |
|  deny_openai            | bool            | true  |  对openai协议进行拦截 |
|  deny_jsonpath          | string          |   []  |  对指定jsonpath拦截 |
|  deny_raw               | bool            | false |  对原始body拦截 |
|  system_deny            | bool            | false |  开启内置拦截规则  |
|  deny_code              | int             | 200   |  拦截时http状态码   |
|  deny_message           | string          | 提问或回答中包含敏感词，已被屏蔽 |  拦截时ai返回消息   |
|  deny_raw_message       | string          | {"errmsg":"提问或回答中包含敏感词，已被屏蔽"} |  非openai拦截时返回内容   |
|  deny_content_type      | string          | application/json  |  非openai拦截时返回content_type头 |
|  deny_words             | array of string | []    |  自定义敏感词列表  |
|  replace_roles          | array           |   -   |  自定义敏感词正则替换  |
|  replace_roles.regex    | string          |   -   |  规则正则(内置GROK规则) |
|  replace_roles.type     | [replace, hash] |   -   |  替换类型  |
|  replace_roles.restore  | bool            | false |  是否恢复  |
|  replace_roles.value    | string          |   -   |  替换值（支持正则变量）  |

## 配置示例

```yaml
system_deny: true
deny_openai: true
deny_jsonpath:
  - "$.messages[*].content"
deny_raw: true
deny_code: 200
deny_message: "提问或回答中包含敏感词，已被屏蔽"
deny_raw_message: "{\"errmsg\":\"提问或回答中包含敏感词，已被屏蔽\"}"
deny_content_type: "application/json"
deny_words: 
  - "自定义敏感词1"
  - "自定义敏感词2"
replace_roles:
  - regex: "%{MOBILE}"
    type: "replace"
    value: "****"
    # 手机号  13800138000 -> ****
  - regex: "%{EMAILLOCALPART}@%{HOSTNAME:domain}"
    type: "replace"
    restore: true
    value: "****@$domain"
    # 电子邮箱  admin@gmail.com -> ****@gmail.com
  - regex: "%{IP}"
    type: "replace"
    restore: true
    value: "***.***.***.***"
    # ip 192.168.0.1 -> ***.***.***.***
  - regex: "%{IDCARD}"
    type: "replace"
    value: "****"
    # 身份证号 110000000000000000 -> ****
  - regex: "sk-[0-9a-zA-Z]*"
    restore: true
    type: "hash"
    # hash sk-12345 -> 9cb495455da32f41567dab1d07f1973d
    # hash后的值提供给大模型，从大模型返回的数据中会将hash值还原为原始值
```

## 敏感词替换样例

### 用户请求内容

  请将 `curl http://172.20.5.14/api/openai/v1/chat/completions -H "Authorization: sk-12345" -H "Auth: test@gmail.com"` 改成post方式

### 处理后请求大模型内容

  `curl http://***.***.***.***/api/openai/v1/chat/completions -H "Authorization: 48a7e98a91d93896d8dac522c5853948" -H "Auth: ****@gmail.com"` 改成post方式

### 大模型返回内容

  您想要将一个 `curl` 的 GET 请求转换为 POST 请求，并且这个请求是向一个特定的 API 发送数据。下面是修改后的 `curl` 命令，以 POST 方式发送：

```sh
curl -X POST \
     -H "Authorization: 48a7e98a91d93896d8dac522c5853948" \
     -H "Auth: ****@gmail.com" \
     -H "Content-Type: application/json" \
     -d '{"key":"value"}' \
     http://***.***.***.***/api/openai/v1/chat/completions
```

这里做了如下几个修改:

- `-X POST` 设置请求方式为 POST。
- `-H "Content-Type: application/json"` 设置请求头中的 `Content-Type` 为 `application/json`，这通常用来告诉服务器您发送的数据格式是 JSON。
- `-d '{"key":"value"}'` 这里设置了要发送的数据，`'{"key":"value"}'` 是一个简单的 JSON 对象示例。您需要将其替换为您实际想要发送的数据。

请注意，您需要将 `"key":"value"` 替换为您实际要发送的数据内容。如果您的 API 接受不同的数据结构或者需要特定的字段，请根据实际情况调整这部分内容。

### 处理后返回用户内容

  您想要将一个 `curl` 的 GET 请求转换为 POST 请求，并且这个请求是向一个特定的 API 发送数据。下面是修改后的 `curl` 命令，以 POST 方式发送：

```sh
curl -X POST \
     -H "Authorization: sk-12345" \
     -H "Auth: test@gmail.com" \
     -H "Content-Type: application/json" \
     -d '{"key":"value"}' \
     http://172.20.5.14/api/openai/v1/chat/completions
```

这里做了如下几个修改:

- `-X POST` 设置请求方式为 POST。
- `-H "Content-Type: application/json"` 设置请求头中的 `Content-Type` 为 `application/json`，这通常用来告诉服务器您发送的数据格式是 JSON。
- `-d '{"key":"value"}'` 这里设置了要发送的数据，`'{"key":"value"}'` 是一个简单的 JSON 对象示例。您需要将其替换为您实际想要发送的数据。

请注意，您需要将 `"key":"value"` 替换为您实际要发送的数据内容。如果您的 API 接受不同的数据结构或者需要特定的字段，请根据实际情况调整这部分内容。


## 相关说明

 - 流模式中如果脱敏后的词被多个chunk拆分，可能无法进行还原
 - 流模式中，如果敏感词语被多个chunk拆分，可能会有敏感词的一部分返回给用户的情况
 - grok 内置规则列表 https://help.aliyun.com/zh/sls/user-guide/grok-patterns
 - 内置敏感词库数据来源 https://github.com/houbb/sensitive-word-data/tree/main/src/main/resources
 - `deny_words` 支持含空格的多词关键词（如 `hello world`、`credit card`），插件会自动对含空格关键词使用子串匹配方式进行检测
