---
title: 自定义应答
keywords: [higress,customn response]
description: 自定义应答插件配置参考
---


## 功能说明
`custom-response`插件支持配置自定义的响应，包括自定义 HTTP 应答状态码、HTTP 应答头，以及 HTTP 应答 Body。可以用于 Mock 响应，也可以用于判断特定状态码后给出自定义应答，例如在触发网关限流策略时实现自定义响应。

## 运行属性

插件执行阶段：`认证阶段`
插件执行优先级：`910`

## 配置字段
### 新版本-支持多种返回
| 名称    | 数据类型             | 填写要求 | 默认值 | 描述                                  |
|-------|------------------|------|-----|-------------------------------------|
| rules | array of object  | 必填   | -   | 规则组                                 |

`rules`的配置字段说明如下：

| 名称                 | 数据类型                      | 填写要求 | 默认值 | 描述                                                                                                                                                   |
|--------------------|---------------------------|------|-----|------------------------------------------------------------------------------------------------------------------------------------------------------|
| `status_code`      | number                    | 选填   | 200 | 自定义 HTTP 应答状态码                                                                                                                                       |
| `headers`          | array of string           | 选填   | -   | 自定义 HTTP 应答头，key 和 value 用`=`分隔                                                                                                                      |
| `body`             | string                    | 选填   | -   | 自定义 HTTP 应答 Body                                                                                                                                     |
| `enable_on_status` | array of string or number | 选填   | -   | 匹配原始状态码，生成自定义响应。可填写精确值如:`200`,`404`等，也可以模糊匹配例如：`2xx`来匹配200-299之间的状态码，`20x`来匹配200-209之间的状态码，x代表任意一位数字。不填写时，不判断原始状态码,取第一个`enable_on_status`为空的规则作为默认规则 |

#### 模糊匹配规则：
* 长度为3
* 至少一位数字
* 至少一位x(不区分大小写)

| 规则  | 匹配内容                                                                                     |
|-----|------------------------------------------------------------------------------------------|
| 40x | 400-409；前两位为40的情况                                                                        |
| 1x4 | 104,114,124,134,144,154,164,174,184,194；第一位和第三位分别为1和4的情况                                 |
| x23 | 023,123,223,323,423,523,623,723,823,923；第二位和第三位为23的情况                                    |  
| 4xx | 400-499；第一位为4的情况                                                                         |
| x4x | 040-049,140-149,240-249,340-349,440-449,540-549,640-649,740-749,840-849,940-949；第二位为4的情况 |
| xx4 | 尾数为4的情况                                                                                  |

### 老版本-只支持一种返回
| 名称 | 数据类型 | 填写要求 |  默认值 | 描述                              |
| -------- | -------- |------| -------- |---------------------------------|
|  `status_code`    |  number     | 选填   |   200  | 自定义 HTTP 应答状态码                  |
|  `headers`     |  array of string      | 选填   |   -  | 自定义 HTTP 应答头，key 和 value 用`=`分隔 |
|  `body`      |  string    | 选填   |   -   | 自定义 HTTP 应答 Body                |
|  `enable_on_status`   |  array of number    | 选填   |  -  | 匹配原始状态码，生成自定义响应，不填写时，不判断原始状态码      |

匹配优先级：精确匹配 > 模糊匹配 > 默认配置(第一个enable_on_status为空的配置)

## 配置示例

### 新版本-不同状态码不同应答场景

```yaml
rules:
  - body: '{"hello":"world 200"}'
    enable_on_status:
      - 200
      - 201
    headers:
      - key1=value1
      - key2=value2
    status_code: 200
  - body: '{"hello":"world 404"}'
    enable_on_status:
      - 404
    headers:
      - key1=value1
      - key2=value2
    status_code: 200
```

根据该配置，200、201请求将返回自定义应答如下：

```text
HTTP/1.1 200 OK
Content-Type: application/json
key1: value1
key2: value2
Content-Length: 21

{"hello":"world 200"}
```
根据该配置，404请求将返回自定义应答如下：

```text
HTTP/1.1 200 OK
Content-Type: application/json
key1: value1
key2: value2
Content-Length: 21

{"hello":"world 400"}
```

### 新版本-模糊匹配场景

```yaml
rules:
  - body: '{"hello":"world 200"}'
    enable_on_status:
      - 200
    headers:
      - key1=value1
      - key2=value2
    status_code: 200
  - body: '{"hello":"world 40x"}'
    enable_on_status:
      - '40x'
    headers:
      - key1=value1
      - key2=value2
    status_code: 200
```

根据该配置，200状态码将返回自定义应答如下：

```text
HTTP/1.1 200 OK
Content-Type: application/json
key1: value1
key2: value2
Content-Length: 21

{"hello":"world 200"}
```
根据该配置，401-409之间的状态码将返回自定义应答如下：

```text
HTTP/1.1 200 OK
Content-Type: application/json
key1: value1
key2: value2
Content-Length: 21

{"hello":"world 40x"}
```

### 老版本-不同状态码相同应答场景

```yaml
enable_on_status:
  - 200
status_code: 200
headers:
  - Content-Type=application/json
  - Hello=World
body: "{\"hello\":\"world\"}"
```
根据该配置，200请求将返回自定义应答如下：

```text
HTTP/1.1 200 OK
Content-Type: application/json
key1: value1
key2: value2
Content-Length: 21

{"hello":"world"}
```

### 触发限流时自定义响应

```yaml
enable_on_status: 
- 429
status_code: 302
headers:
- Location=https://example.com
```

触发网关限流时一般会返回 `429` 状态码，这时请求将返回自定义应答如下：

```text
HTTP/1.1 302 Found
Location: https://example.com
```

从而实现基于浏览器 302 重定向机制，将限流后的用户引导到其他页面，比如可以是一个 CDN 上的静态页面。

如果希望触发限流时，正常返回其他应答，参考 Mock 应答场景配置相应的字段即可。
