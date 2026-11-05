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

## 工具定义和参数（显式开启）

三个新开关默认都是 `false`，互不隐含。升级后不配置它们，继续使用原来的 OpenAI、JSONPath 和 raw 处理。原有 `replace_roles.restore: true` 不会自动开启工具参数还原。

| 字段 | 类型 | 默认值 | 作用 |
| --- | --- | --- | --- |
| `tool_arguments.check` | bool | false | 请求参数执行 deny 和 mask；响应参数执行 deny |
| `tool_arguments.restore` | bool | false | 按独立白名单还原响应参数 |
| `tool_arguments.restore_allowlist` | array | [] | `{tool, paths}` 列表，开启工具还原时不能为空 |
| `tool_definitions.check` | bool | false | 检查请求中的 tools/functions 定义，命中即拒绝，不做新增替换 |
| `tool_arguments.limits.max_call_bytes` | 正整数 | 1048576 | 单 call 的输入和改写结果上限，按解开外层转义后的 UTF-8 字节计 |
| `tool_arguments.limits.max_buffer_bytes` | 正整数 | 4194304 | 每个响应保留的参数总量，含原文、改写结果、排队参数及 name/id |
| `tool_arguments.limits.max_calls` | 正整数 | 128 | 每个响应登记过的需处理 call 数，结束后仍计数 |

以上三项数值是当前开发分支的候选值，需完成容量验证后才能确定发布默认值。总量预算不等于网关 RSS 上限；解析树、协议编码、旧文字窗口和其他 HTTP 缓冲另有开销。请求历史只检查单 call 上限，不套用响应总量和 call 数上限。关闭新功能时，这些上限不影响旧路径。

新检查复用 `deny_words`、`system_deny` 和 `replace_roles`。参数覆盖现代 `tool_calls[].function.arguments` 与 legacy `function_call.arguments`，包括请求历史、普通响应和 SSE delta；不依赖旧 `deny_openai` 开关。只开定义检查不接管响应。定义检查涵盖字符串叶子以及 schema 的 properties、patternProperties、$defs、definitions 等动态名称。

```yaml
deny_openai: false
tool_arguments:
  check: true
  restore: true
  restore_allowlist:
    - tool: lookup_customer
      paths: ["/email", "/contacts/0/email"]
  limits:
    max_call_bytes: 1048576
    max_buffer_bytes: 4194304
    max_calls: 128
tool_definitions:
  check: false
replace_roles:
  - regex: "%{EMAILADDRESS}"
    type: hash
    restore: true
```

白名单工具名区分大小写。路径使用 RFC 6901 原始 JSON Pointer，`~0` 代表 `~`，`~1` 代表 `/`；空路径只选择根字符串。仅还原路径精确指向的字符串值，不支持通配符、子树授权、URI fragment 或数组追加 `-`，数组下标不能带前导零。重复工具、重复路径和无效转义会导致配置失败。不存在或非字符串的目标不还原。

参数内部的 JSON 字符串会先解码再检查，key 只检查、不改写。只有实际改变的值重新编码；其他 key 顺序、空白、转义、重复 key 和数字写法保留。重复 key 影响白名单路径唯一定位时拒绝。外层 arguments 缺失、null 或空字符串视为无参数；需要处理的非空 arguments 必须是包含合法 JSON 的字符串。

SSE 参数按 choice/call 分别保存，等该 choice 的非 null `finish_reason`（包括空串）、`[DONE]` 或 HTTP end 再处理。id/name/type/index 等 metadata 仍尽早发送。完整名称收齐后才授权还原；同一 choice 的待发文字先于参数，参数先于结束标记。此功能会增加完整参数的等待时间。choice index 必须是原生非负 i64；tool index 还可使用可解析为 u64 的十进制字符串。缺 tool index 只能用已知 id 关联，不能猜数组位置；legacy call 单独归属。

仅支持 delta 契约，不猜测或去重 cumulative 片段；无法可靠识别的累计响应仍可能拼成错误内容，应在接入前确认上游协议。未授权工具的仅还原请求保留原参数，不承担新增还原预算；仍可能命中白名单的 call 超限会失败。

开启对应处理后，坏 JSON/SSE、无法归属的 call、歧义还原或超限会明确失败。请求沿用 `deny_code`（默认 200）和现有拒绝正文；普通响应替换整包；SSE 丢弃待发内容，并在后续回调中保持拒绝状态。已下发的 metadata/文字无法撤回。合法 SSE 注释、心跳、event/id/retry 和上游 error 保留，不补造成功结束。

新参数 mask 与旧 raw/JSONPath/content 改写只各执行一次规则表；旧 deny 仍检查原范围。响应受管参数只按新的还原开关和白名单决定，旧全局还原不能越界。旧改写跨越受管参数边界时拒绝。

工具还原只读取同请求、明确配置 `restore: true` 的实际映射。新参数产生的映射不用于普通回答还原；同值对应多个原值时不任选一条，也不递归还原。建议使用 hash 或足够独特的占位值，静态 `***` 可能误命中。白名单也不能防止模型或 prompt injection 把秘密填入一个已允许的参数；工具执行方仍需自行校验权限、用途和目的地址。

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
 - 由于敏感词列表是在文本分词后进行匹配的，所以请将 `deny_words` 设置为单个单词，英文多单词情况如 `hello word` 可能无法匹配
