---
title: 流量染色
keywords: [higress,traffic tag]
description: 流量染色插件配置参考
---


## 功能说明

`traffic-tag` 插件允许根据权重或特定请求内容通过添加特定请求头的方式对请求流量进行染色。它支持复杂的逻辑来确定如何根据用户定义的标准染色流量。

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`400`


## 配置字段

此部分提供了配置字段的详细描述。

| 字段名称        | 类型     | 默认值 | 是否必填 | 描述                                                         |
|----------------|----------|-------|---------|-------------------------------------------------------------|
| `conditionGroups` | array of object  | -     | 否      | 定义基于内容的标记条件组，详细结构见**条件组配置**。             |
| `weightGroups`    | array of object  | -     | 否      | 定义基于权重的标记条件组，详细结构见**权重组配置**。             |
| `defaultTagKey`   | string   | -     | 否      | 默认的标记键名，当未匹配到任何条件时使用。当且仅当同时配置了**defaultTagVal**时生效      |
| `defaultTagVal` | string   | -     | 否      | 默认的标记值，当未匹配到任何条件时使用。当且仅当同时配置了**defaultTagKey**时生效      |

### 条件组配置
`conditionGroups` 中每一项的配置字段说明如下：

| 字段名称      | 类型   | 默认值 | 是否必填 | 描述                                                         |
|--------------|--------|-------|---------|-------------------------------------------------------------|
| `headerName` | string | -     | 是      | 要添加或修改的 HTTP 头名称。                                  |
| `headerValue`| string | -     | 是      | HTTP 头的值。                                                |
| `logic`      | string | -     | 是      | 条件组中的逻辑关系，支持 `and`、`or`，必须为小写字母。         |
| `conditions` | array of object  | -     | 是      | 描述具体的标记条件，详细结构如下。                    |
---

`conditions` 中每一项的配置字段说明如下：

| 字段名称        | 类型   | 默认值 | 是否必填 | 描述                                                                                            |
|----------------|--------|-------|---------|-----------------------------------------------------------------------------------------------|
| `conditionType`| string | -     | 是      | 条件类型，支持 `header`、`parameter`、`cookie`。                                                        |
| `key`          | string | -     | 是      | 条件的关键字。                                                                                       |
| `operator`     | string | -     | 是      | 操作符，支持 `equal`、`not_equal`、`prefix`、`in`、`not_in`、`regex`、`percentage`、`exists`、`not_exists`。 |
| `value`        | array of string  | -     | 是      | 条件的值，**仅当**操作符为 `in` 和 `not_in` 时支持配置多个值。                                                     |

> **说明：当 `operator` 为 `regex` 时，使用的正则表达式引擎是 [RE2](https://github.com/google/re2)。详情请参阅 [RE2 官方文档](https://github.com/google/re2/wiki/Syntax)。

### 权重组配置

`weightGroups` 中每一项的配置字段说明如下：

| 字段名称      | 类型     | 默认值 | 是否必填 | 描述                                                         |
|--------------|----------|-------|---------|-------------------------------------------------------------|
| `headerName` | string   | -     | 是      | 要添加或修改的 HTTP 头名称。                                  |
| `headerValue`| string   | -     | 是      | HTTP 头的值。                                                |
| `weight`     | integer  | -     | 是      | 流量权重百分比。                                             |                                           

### 操作符说明
| 操作符          | 描述                                               |
|--------------|--------------------------------------------------|
| `equal`      | 精确匹配，值需要完全相等                                     |
| `not_equal`  | 不等匹配，值不相等时满足条件                                   |
| `prefix`     | 前缀匹配，指定值是实际值的前缀时满足条件                             |
| `in`         | 包含匹配，实际值需要在指定的列表中                                |
| `not_in`     | 排除匹配，实际值不在指定的列表中时满足条件                            |
| `regex`      | 正则表达式匹配，按照正则表达式规则匹配                              |
| `percentage` | 百分比匹配，原理：`hash(get(key)) % 100 < value` 成立时满足条件  |
| `exists`     | 存在性匹配，指定的key存在时满足条件, 不需要传递value                  |
| `not_exists` | 存在性匹配，指定的key不存在时满足条件, 不需要传递value                 |

> **提示：关于`percentage`和`weight`的区别**
>
> - **`percentage`操作符**：用于条件表达式中，基于指定的百分比和指定的键值对来判断是否执行某个操作。对于一个相同的键值对，多次匹配的结果是幂等的，即这一次命中条件，下一次也会命中。
> - **`weight`字段**：用于定义不同处理路径的流量权重。在基于权重的流量标记中，`weight`确定了某个路径应接收的流量比例。与`percentage`不同的是，由于没有指定固定的对比依据而是基于随机权重分布，同一个请求的多次匹配可能匹配多个结果。
>
> 使用`percentage`进行条件匹配时，判断每个请求是否满足特定百分比条件；而`weight`则是静态随机分配整体流量的比例。

## 配置示例

**例1: 基于内容的匹配**

按照下例的配置，满足请求头`role` 的值是`user`、`viwer`、`editor`其中之一且存在查询参数`foo=bar`的请求将被添加请求头`x-mse-tag: gray`。由于配置了`defaultTagKey`和`defaultTagVal`，当未匹配到任何条件时，请求将被添加请求头`x-mse-tag: base`。

```yaml
defaultTagKey: x-mse-tag
defaultTagVal: base
conditionGroups:
  - headerName: x-mse-tag
    headerValue: gray
    logic: and
    conditions:
      - conditionType: header
        key: role
        operator: in
        value:
          - user
          - viewer
          - editor
      - conditionType: parameter
        key: foo
        operator: equal
        value:
        - bar
```
**例子2: 基于权重的匹配**

按照下列配置，请求将有30%几率被添加请求头`x-mse-tag: gray`，30%几率被添加请求头`x-mse-tag: blue`，40%几率不添加请求头。

```yaml
# 权重总和为100，下例中未配置的40权重将不添加header
weightGroups:
  - headerName: x-mse-tag
    headerValue: gray
    weight: 30
  - headerName: x-mse-tag
    headerValue: blue
    weight: 30
```
