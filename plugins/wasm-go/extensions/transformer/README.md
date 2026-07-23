---
title: 请求响应转换
keywords: [higress,transformer]
description: 请求响应转换插件配置参考
---


## 功能说明
`transformer` 插件可以对请求/响应头、请求查询参数、请求/响应体参数进行转换，支持的转换操作类型包括删除、重命名、更新、添加、追加、映射、去重。

## 运行属性

插件执行阶段：`认证阶段`
插件执行优先级：`410`

## 配置字段

| 名称 |  数据类型   | 填写要求 | 默认值  | 描述 |
| :----: |:-------:| :----: |:----:| -------- |
|  reroute | boolean | 选填 | true |  是否在请求转换过程中对路由目标进行重新选择  |
|  reqRules | string  | 选填，reqRules和respRules至少填一个 |  -   |  请求转换器配置，指定转换操作类型以及请求头、请求查询参数、请求体的转换规则 |
|  respRules | string  | 选填，reqRules和respRules至少填一个 |  -   |  响应转换器配置，指定转换操作类型以及响应头、响应体的转换规则 |

`reqRules`和`respRules`中每一项的配置字段说明如下：

| 名称 | 数据类型 | 填写要求 |  默认值 | 描述 |
| :----: | :----: | :----: | :----: | -------- |
| operate |  string  | 必填，可选值为 `remove`, `rename`, `replace`, `add`, `append`, `map`, `dedupe` |   -  |  指定转换操作类型，支持的操作类型有删除 (remove)、重命名 (rename)、更新 (replace)、添加 (add)、追加 (append)、映射 (map)、去重 (dedupe)，当存在多项不同类型的转换规则时，按照上述操作类型顺序依次执行  |
|  mapSource  | string  | 选填，可选值为`headers`, `querys`,`body` |  -  | 仅在operate为`map`时有效。指定映射来源，若不填该字段，则默认映射来源为自身 |
|  headers  |  array of object  | 选填     |  -  | 指定请求/响应头转换规则 |
| querys |  array of object  | 选填     |   -  | 指定请求查询参数转换规则 |
| body | array of object | 选填 | - | 指定请求/响应体参数转换规则，请求体转换允许 content-type 为 `application/json`, `application/x-www-form-urlencoded`, `multipart/form-data`；响应体转换仅允许 content-type 为 `application/json` |

`headers`, `querys`, `body`中每一项的配置字段说明如下：

| 名称 | 数据类型 | 填写要求 |  默认值 | 描述                                                |
| :----: | :----: | :----: | -------- |---------------------------------------------------|
| key |  string  | 选填 |   -  | 在operate为`remove`时使用，用法详见[转换操作类型](#转换操作类型) |
| oldKey | string | 选填 | - |在operate为`rename`时使用，用法详见[转换操作类型](#转换操作类型) |
| newKey |  string  | 选填 |   -  | 在operate为`rename`时使用，用法详见[转换操作类型](#转换操作类型) |
| key | string | 选填 | - | 在operate为`replace`时使用，用法详见[转换操作类型](#转换操作类型) |
| newValue |  string  | 选填 |   -  | 在operate为`replace`时使用，用法详见[转换操作类型](#转换操作类型) |
| key | string | 选填 | - | 在operate为`add`时使用，用法详见[转换操作类型](#转换操作类型) |
| value | string | 选填 | - | 在operate为`add`时使用，用法详见[转换操作类型](#转换操作类型) |
| key |  string  | 选填 |   -  | 在operate为`append`时使用，用法详见[转换操作类型](#转换操作类型) |
| appendValue | string | 选填 | - | 在operate为`append`时使用，用法详见[转换操作类型](#转换操作类型) |
| fromKey |  string  | 选填 |   -  | 在operate为`map`时使用，用法详见[转换操作类型](#转换操作类型) |
| toKey |  string  | 选填 |   -  | 在operate为`map`时使用，用法详见[转换操作类型](#转换操作类型) |
| key |  string  | 选填 |   -  | 在operate为`dedupe`时使用，用法详见[转换操作类型](#转换操作类型) |
| strategy |  string  | 选填 |   -  | 在operate为`dedupe`时使用，用法详见[转换操作类型](#转换操作类型) |
|  value_type  |  string  | 选填，可选值为 `object`, `boolean`, `number`, `string` |  string  | 当`content-type: application/json`时，该字段指定请求/响应体参数的值类型 |
| host_pattern |  string  | 选填     |   -  | 指定请求主机名匹配规则，当转换操作类型为 `replace`, `add`, `append` 时有效 |
| path_pattern | string | 选填 | - | 指定请求路径匹配规则，当转换操作类型为 `replace`, `add`, `append` 时有效 |

注意：

* `request transformer` 支持以下转换对象：请求头部、请求查询参数、请求体（application/json, application/x-www-form-urlencoded, multipart/form-data）
* `response transformer` 支持以下转换对象：响应头部、响应体（application/json）
* 插件支持双向转换能力，即单个插件能够完成对请求和响应都做转换
* 转换操作类型的执行顺序，为配置文件中编写的顺序，如：remove → rename → replace → add → append → map → dedupe或者dedupe → map → append → add → replace → rename → remove等
* 当转换对象为 headers 时，` key` 不区分大小写；当为 headers 且为 `rename`, `map` 操作时，`value` 也不区分大小写（因为此时该字段具有 key 含义）；而 querys 和 body 的 `key`, `value` 字段均区分大小写
* `value_type` 仅对 content-type 为 application/json 的请求/响应体有效
* `host_pattern` 和 `path_pathern` 支持 [RE2 语法](https://pkg.go.dev/regexp/syntax)，仅对 `replace`, `add`, `append` 操作有效，且在一项转换规则中两者只能选填其一，若均填写，则 `host_pattern` 生效，而 `path_pattern` 失效
* 当配置了 `host_pattern` 或 `path_pattern`，但当前请求的主机名或路径不匹配该正则时，对应的转换项将被跳过（不执行）。借助这一点，可以为同一个 `key` 配置多条带有不同匹配规则的转换项，使其在不同的主机名/路径下生效不同的值



## 转换操作类型

| 操作类型      | key 字段含义 | value 字段含义     | 描述                                                                                                                                                                                                                                                                                    |
| :----: | :----: | :----: |---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 删除 remove   | 目标 key     |无需设置| 若存在指定的 `key`，则删除；否则无操作                                                                                                                                                                                                                                                                |
| 重命名 rename | 目标 oldKey |新的 key 名称 newKey| 若存在指定的 `oldKey:value`，则将其键名重命名为 `newKey`，得到 `newKey:value`；否则无操作                                                                                                                                                                                                                      |
| 更新 replace  | 目标 key |新的 value 值 newValue| 若存在指定的 `key:value`，则将其 value 更新为 `newValue`，得到 `key:newValue`；否则等效于 add 操作                                                                                                                                                                                                            |
| 添加 add      | 添加的 key | 添加的 value | 若不存在指定的 `key:value`，则添加；否则无操作                                                                                                                                                                                                                                                         |
| 追加 append   | 目标 key |追加的 value值 appendValue| 若存在指定的 `key:value`，则追加 appendValue 得到 `key:[value..., appendValue]`；否则相当于执行 add 操作，得到 `key:appendValue`                                                                                                                                                                               |
| 映射 map      | 映射来源 fromKey |映射目标 toKey| 若存在指定的 `fromKey:fromValue`，则将其值 fromValue 映射给 toKey 的值，得到 `toKey:fromValue`，同时保留 `fromKey:fromValue`（注：若 toKey 已存在则其值会被覆盖）；否则无操作                                                                                                                                                      |
| 去重 dedupe   | 目标 key |指定去重策略 strategy| `strategy` 可选值为：<br>`RETAIN_UNIQUE`: 按顺序保留所有唯一值，如 `k1:[v1,v2,v3,v3,v2,v1]`，去重后得到 `k1:[v1,v2,v3]` <br>`RETAIN_LAST`: 保留最后一个值，如 `k1:[v1,v2,v3]`，去重后得到 `k1:v3` <br>`RETAIN_FIRST` (default): 保留第一个值，如 `k1:[v1,v2,v3]`，去重后得到 `k1:v1`<br>`SPLIT_AND_RETAIN_FIRST`: 对值按逗号进行切割，并保留第一个值，如 `k1:"v1,v2,v3"`，去重后得到 `k1:v1`<br>`SPLIT_AND_RETAIN_LAST`: 对值按逗号进行切割，并保留最后一个值，如 `k1:"v1,v2,v3"`，去重后得到 `k1:v3`<br>（注：若去重后只剩下一个元素 v1 时，键值对变为 `k1:v1`, 而不是 `k1:[v1]`） |




## 配置示例

### 实现基于Body参数路由

配置示例：

```yaml
reqRules:
- operate: map
  headers:
  - fromKey: userId
    toKey: x-user-id
  mapSource: body
```

此规则将请求body中的`userId`解析出后，设置到请求Header`x-user-id`中，这样就可以基于Higress请求Header匹配路由的能力来实现基于Body参数的路由了。

此配置同时支持`application/json`和`application/x-www-form-urlencoded`两种类型的请求Body。

举例来说：

**对于application/json类型的body**

```bash
curl localhost -d '{"userId":12, "userName":"johnlanni"}' -H 'content-type:application/json'
```

将从json中提取出`userId`字段的值，设置到`x-user-id`中，后端服务收到的请求头将增加:`x-usr-id: 12`。

因为在插件新增这个Header后，网关将重新计算路由，所以可以实现网关路由配置根据这个请求头来匹配路由到特定的目标服务。


**对于application/x-www-form-urlencoded类型的body**

```bash
curl localhost -d 'userId=12&userName=johnlanni'
```

将从`k1=v1&k2=v2`这样的表单格式中提取出`userId`字段的值，设置到`x-user-id`中，后端服务收到的请求头将增加:`x-usr-id: 12`。

因为在插件新增这个Header后，网关将重新计算路由，所以可以实现网关路由配置根据这个请求头来匹配路由到特定的目标服务。

#### json path 支持

可以根据 [GJSON Path 语法](https://github.com/tidwall/gjson/blob/master/SYNTAX.md)，从复杂的 json 中提取出字段。

比较常用的操作举例，对于以下 json:

```json
{
  "name": {"first": "Tom", "last": "Anderson"},
  "age":37,
  "children": ["Sara","Alex","Jack"],
  "fav.movie": "Deer Hunter",
  "friends": [
    {"first": "Dale", "last": "Murphy", "age": 44, "nets": ["ig", "fb", "tw"]},
    {"first": "Roger", "last": "Craig", "age": 68, "nets": ["fb", "tw"]},
    {"first": "Jane", "last": "Murphy", "age": 47, "nets": ["ig", "tw"]}
  ]
}
```

可以实现这样的提取:

```text
name.last              "Anderson"
name.first             "Tom"
age                    37
children               ["Sara","Alex","Jack"]
children.0             "Sara"
children.1             "Alex"
friends.1              {"first": "Roger", "last": "Craig", "age": 68}
friends.1.first        "Roger"
```

现在如果想从上面这个 json 格式的 body 中提取出 friends 中第二项的 first 字段，来设置到 Header `x-first-name` 中，同时抽取 last 字段，来设置到 Header `x-last-name` 中，则可以使用这份插件配置:

```yaml
reqRules:
- operate: map
  headers:
  - fromKey: friends.1.first
    toKey: x-first-name
  - fromKey: friends.1.last
    toKey: x-last-name
  mapSource: body
```

### Request Transformer

#### 转换请求头部

```yaml
reqRules:
- operate: remove
  headers:
  - key: X-remove
- operate: rename
  headers:
  - oldKey: X-not-renamed
    newKey: X-renamed
- operate: replace
  headers:
  - key: X-replace
    newValue: replaced
- operate: add
  headers:
  - key: X-add-append
    value: host-$1
    host_pattern: ^(.*)\.com$
- operate: append
  headers:
  - key: X-add-append
    appendValue: path-$1
    path_pattern: ^.*?\/(\w+)[\?]{0,1}.*$
- operate: map
  headers:
  - fromKey: X-add-append
    toKey: X-map
- operate: dedupe
  headers:
  - key: X-dedupe-first
    strategy: RETAIN_FIRST
  - key: X-dedupe-last
    strategy: RETAIN_LAST
  - key: X-dedupe-unique
    strategy: RETAIN_UNIQUE
```

发送请求

```bash
$ curl -v console.higress.io/get -H 'host: foo.bar.com' \
-H 'X-remove: exist' -H 'X-not-renamed:test' -H 'X-replace:not-replaced' \
-H 'X-dedupe-first:1' -H 'X-dedupe-first:2' -H 'X-dedupe-first:3' \
-H 'X-dedupe-last:a' -H 'X-dedupe-last:b' -H 'X-dedupe-last:c' \
-H 'X-dedupe-unique:1' -H 'X-dedupe-unique:2' -H 'X-dedupe-unique:3' \
-H 'X-dedupe-unique:3' -H 'X-dedupe-unique:2' -H 'X-dedupe-unique:1'

# httpbin 响应结果
{
  "args": {},
  "headers": {
    ...
    "X-Add-Append": "host-foo.bar,path-get",
    ...
    "X-Dedupe-First": "1",
    "X-Dedupe-Last": "c",
    "X-Dedupe-Unique": "1,2,3",
    ...
    "X-Map": "host-foo.bar,path-get",
    "X-Renamed": "test",
    "X-Replace": "replaced"
  },
  ...
}
```

#### 转换请求查询参数

```yaml
reqRules:
- operate: remove
  querys:
  - key: k1
- operate: rename
  querys:
  - oldKey: k2
    newKey: k2-new
- operate: replace
  querys:
  - key: k2-new
    newValue: v2-new
- operate: add
  querys:
  - key: k3
    value: v31-$1
    path_pattern: ^.*?\/(\w+)[\?]{0,1}.*$
- operate: append
  querys:
  - key: k3
    appendValue: v32
- operate: map
  querys:
  - fromKey: k3
    toKey: k4
- operate: dedupe
  querys:
  - key: k4
    strategy: RETAIN_FIRST
```

发送请求

```bash
$ curl -v "console.higress.io/get?k1=v11&k1=v12&k2=v2"

# httpbin 响应结果
{
  "args": {
    "k2-new": "v2-new",
    "k3": [
      "v31-get",
      "v32"
    ],
    "k4": "v31-get"
  },
  ...
  "url": "http://foo.bar.com/get?k2-new=v2-new&k3=v31-get&k3=v32&k4=v31-get"
}
```

#### 转换请求体

```yaml
reqRules:
- operate: remove
  body:
  - key: a1
- operate: rename
  body: 
  - oldKey: a2
    newKey: a2-new
- operate: replace
  body:
  - key: a3
    newValue: t3-new
    value_type: string
- operate: add
  body:
  - key: a1-new
    value: t1-new
    value_type: string
- operate: append
  body:
  - key: a1-new
    appendValue: t1-$1-append
    value_type: string
    host_pattern: ^(.*)\.com$
- operate: map
  body:
  - fromKey: a1-new
    toKey: a4
- operate: dedupe
  body:
  - key: a4
    strategy: RETAIN_FIRST
```

发送请求：

**1. Content-Type: application/json**

```bash
$ curl -v -x POST console.higress.io/post -H 'host: foo.bar.com' \
-H 'Content-Type: application/json' -d '{"a1":"t1","a2":"t2","a3":"t3"}'

# httpbin 响应结果
{
  ...
  "headers": {
    ...
    "Content-Type": "application/json",
    ...
  },
  "json": {
    "a1-new": [
      "t1-new",
      "t1-foo.bar-append"
    ],
    "a2-new": "t2",
    "a3": "t3-new",
    "a4": "t1-new"
  },
  ...
}
```

**2. Content-Type: application/x-www-form-urlencoded**

```bash
$ curl -v -X POST console.higress.io/post -H 'host: foo.bar.com' \
-d 'a1=t1&a2=t2&a3=t3'

# httpbin 响应结果
{
  ...
  "form": {
    "a1-new": [
      "t1-new",
      "t1-foo.bar-append"
    ],
    "a2-new": "t2",
    "a3": "t3-new",
    "a4": "t1-new"
  },
  "headers": {
    ...
    "Content-Type": "application/x-www-form-urlencoded",
    ...
  },
  ...
}
```

**3. Content-Type:  multipart/form-data**

```bash
$ curl -v -X POST console.higress.io/post -H 'host: foo.bar.com' \
-F a1=t1 -F a2=t2 -F a3=t3

# httpbin 响应结果
{
  ...
  "form": {
    "a1-new": [
      "t1-new",
      "t1-foo.bar-append"
    ],
    "a2-new": "t2",
    "a3": "t3-new",
    "a4": "t1-new"
  },
  "headers": {
    ...
    "Content-Type": "multipart/form-data; boundary=------------------------1118b3fab5afbc4e",
    ...
  },
  ...
}
```

#### 禁止重新选择路由目标

在请求转换过程中，默认情况下会对路由目标进行重新选择。如果不希望重新选择路由目标，可以设置 `reroute` 为 `false`：

```yaml
reroute: false
reqRules:
  - operate: replace
    headers:
      - key: reroute
        newValue: true
```

假设路由配置为：

- path 前缀匹配 /，header 精准匹配 reroute: false，路由的目标服务响应为 200 "no rerouting"
- path 前缀匹配 /，header 精准匹配 reroute: true，路由的目标服务响应为 200 "rerouting"

那么根据上述配置，请求得到的结果为：

```bash
# 插件将 header 从 reroute: false 替换为 reroute: true，但禁止了重新选择路由目标
$ curl console.higress.io/get -H 'host: foo.bar.com' -H 'reroute: false'
no rerouting%

# 去掉插件中的 reroute 配置或将其设置为 true，则
# 插件将 header 从 reroute: false 替换为 reroute: true，并重新选择路由目标
$ curl console.higress.io/get -H 'host: foo.bar.com' -H 'reroute: false'
rerouting% 
```


### Response Transformer

与 Request Transformer 类似，在此仅说明转换 JSON 形式的请求/响应体时的注意事项：

#### key 嵌套 `.`

1.通常情况下，指定的 key 中含有 `.` 表示嵌套含义，如下：

```yaml
respRules:
- operate: add
  body:
  - key: foo.bar
    value: value
```

```bash
$ curl -v console.higress.io/get

# httpbin 响应结果
{
 ...
 "foo": {
  "bar": "value"
 },
 ...
}
```

2.当使用 `\.` 对 key 中的 `.` 进行转义后，表示非嵌套含义，如下：

> 当使用双引号括住字符串时使用 `\\.` 进行转义

```yaml
respRules:
- operate: add
  body:
  - key: foo\.bar
    value: value
```

```bash
$ curl -v console.higress.io/get

# httpbin 响应结果
{
 ...
 "foo.bar": "value",
 ...
}
```

#### 访问数组元素 `.index`

可以通过数组下标 `array.index 访问数组元素，下标从 0 开始：

```json
{
  "users": [
    {
      "123": { "name": "zhangsan", "age": 18 }
    },
    {
      "456": { "name": "lisi", "age": 19 }
    }
  ]
}
```

1.移除 `user` 第一个元素：

```yaml
reqRules:
- operate: remove
  body:
  - key: users.0
```

```bash
$ curl -v -X POST console.higress.io/post \
-H 'Content-Type: application/json' \
-d '{"users":[{"123":{"name":"zhangsan"}},{"456":{"name":"lisi"}}]}'

# httpbin 响应结果
{
  ...
  "json": {
    "users": [
      {
        "456": {
          "name": "lisi"
        }
      }
    ]
  },
  ...
}
```

2.将 `users` 第一个元素的 key 为 `123` 重命名为 `msg`:

```yaml
reqRules:
- operate: rename
  body:
  - oldKey: users.0.123
    newKey: users.0.first
```

```bash
$ curl -v -X POST console.higress.io/post \
-H 'Content-Type: application/json' \
-d '{"users":[{"123":{"name":"zhangsan"}},{"456":{"name":"lisi"}}]}'


# httpbin 响应结果
{
  ...
  "json": {
    "users": [
      {
        "msg": {
          "name": "zhangsan"
        }
      },
      {
        "456": {
          "name": "lisi"
        }
      }
    ]
  },
  ...
}
```

#### 遍历数组元素 `.#`

可以使用 `array.#` 对数组进行遍历操作：

> ❗️该操作目前只能用在 replace 上，请勿在其他转换中尝试该操作，以免造成无法预知的结果

```json
{
  "users": [
    {
      "name": "zhangsan", 
      "age": 18
    },
    {
      "name": "lisi",
      "age": 19
    }
  ]
}
```

```yaml
reqRules:
- operate: replace
  body:
  - key: users.#.age
    newValue: 20
```

```bash
$ curl -v -X POST console.higress.io/post \
-H 'Content-Type: application/json' \
-d '{"users":[{"name":"zhangsan","age":18},{"name":"lisi","age":19}]}'


# httpbin 响应结果
{
  ...
  "json": {
    "users": [
      {
        "age": "20",
        "name": "zhangsan"
      },
      {
        "age": "20",
        "name": "lisi"
      }
    ]
  },
  ...
}
```
