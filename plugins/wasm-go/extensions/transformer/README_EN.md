---
title: Request Response Transformation
keywords: [higress,transformer]
description: Request response transformation plugin configuration reference
---
## Function Description
The `transformer` plugin can transform request/response headers, request query parameters, and request/response body parameters. Supported transformation operation types include deletion, renaming, updating, adding, appending, mapping, and deduplication.

## Execution Attributes
Plugin execution phase: `authentication phase`  
Plugin execution priority: `410`

## Configuration Fields
| Name | Data Type |                        Fill Requirement                        | Default Value | Description |
| :----: | :----: |:--------------------------------------------------------------:| :----: | -------- |
| reroute | boolean |                            Optional                            | true | Whether to reselect the routing target during request transformation |
| reqRules | string | Optional, at least one of reqRules or respRules must be filled | - | Request transformer configuration, specifying the transformation operation type and rules for transforming request headers, request query parameters, and request body |
| respRules | string | Optional, at least one of reqRules or respRules must be filled | - | Response transformer configuration, specifying the transformation operation type and rules for transforming response headers and response body |

The configuration fields for each item in `reqRules` and `respRules` are as follows:

| Name | Data Type | Fill Requirement | Default Value | Description |
| :----: | :----: | :----: | :----: | -------- |
| operate | string | Required, optional values are `remove`, `rename`, `replace`, `add`, `append`, `map`, `dedupe` | - | Specifies the transformation operation type. Supported operation types include remove (remove), rename (rename), replace (replace), add (add), append (append), map (map), dedupe (dedupe). When there are multiple transformation rules of different types, they are executed in the order of the above operation types. |
| mapSource | string | Optional, optional values are `headers`, `querys`, `body` | - | Valid only when operate is `map`. Specifies the mapping source. If this field is not filled, the default mapping source is itself. |
| headers | array of object | Optional | - | Specifies transformation rules for request/response headers. |
| querys | array of object | Optional | - | Specifies transformation rules for request query parameters. |
| body | array of object | Optional | - | Specifies transformation rules for request/response body parameters. Request body transformations allow content-types of `application/json`, `application/x-www-form-urlencoded`, and `multipart/form-data` while response body transformations only allow content-type of `application/json`. |

The configuration fields for each item in `headers`, `querys`, `body` are as follows:

| Name | Data Type | Fill Requirement | Default Value | Description |
| :----: | :----: | :----: | -------- | --------------------------------------------------- |
| key | string | Optional | - | Used when operate is `remove`, see [Transformation Operation Types](#转换操作类型) for details. |
| oldKey | string | Optional | - | Used when operate is `rename`, see [Transformation Operation Types](#转换操作类型) for details. |
| newKey | string | Optional | - | Used when operate is `rename`, see [Transformation Operation Types](#转换操作类型) for details. |
| key | string | Optional | - | Used when operate is `replace`, see [Transformation Operation Types](#转换操作类型) for details. |
| newValue | string | Optional | - | Used when operate is `replace`, see [Transformation Operation Types](#转换操作类型) for details. |
| key | string | Optional | - | Used when operate is `add`, see [Transformation Operation Types](#转换操作类型) for details. |
| value | string | Optional | - | Used when operate is `add`, see [Transformation Operation Types](#转换操作类型) for details. |
| key | string | Optional | - | Used when operate is `append`, see [Transformation Operation Types](#转换操作类型) for details. |
| appendValue | string | Optional | - | Used when operate is `append`, see [Transformation Operation Types](#转换操作类型) for details. |
| fromKey | string | Optional | - | Used when operate is `map`, see [Transformation Operation Types](#转换操作类型) for details. |
| toKey | string | Optional | - | Used when operate is `map`, see [Transformation Operation Types](#转换操作类型) for details. |
| key | string | Optional | - | Used when operate is `dedupe`, see [Transformation Operation Types](#转换操作类型) for details. |
| strategy | string | Optional | - | Used when operate is `dedupe`, see [Transformation Operation Types](#转换操作类型) for details. |
| value_type | string | Optional, optional values are `object`, `boolean`, `number`, `string` | string | When `content-type: application/json`, this field specifies the value type of request/response body parameters. |
| host_pattern | string | Optional | - | Specifies the request hostname matching rule. Valid when transformation operation type is `replace`, `add`, `append`. |
| path_pattern | string | Optional | - | Specifies the request path matching rule. Valid when transformation operation type is `replace`, `add`, `append`. |

Note:
* `request transformer` supports the following transformation objects: request headers, request query parameters, request body (application/json, application/x-www-form-urlencoded, multipart/form-data).
* `response transformer` supports the following transformation objects: response headers, response body (application/json).
* The plugin supports bidirectional conversion capability, meaning that a single plugin can perform transformations on both requests and responses.
* The execution order of transformation operation types is the order written in the configuration file, e.g., remove → rename → replace → add → append → map → dedupe or dedupe → map → append → add → replace → rename → remove.
* When the transformation object is headers, `key` is case-insensitive. When headers are operated and are `rename` or `map`, `value` is also case-insensitive (as this field has a key meaning). However, `key` and `value` fields in querys and body are case-sensitive.
* `value_type` is only effective for content type application/json for request/response bodies.
* `host_pattern` and `path_pattern` support [RE2 syntax](https://pkg.go.dev/regexp/syntax), valid only for `replace`, `add`, `append` operations. In a transformation rule, only one of the two can be optionally filled. If both are filled, then `host_pattern` takes effect while `path_pattern` becomes ineffective.
* When `host_pattern` or `path_pattern` is configured but the current request's hostname or path does not match the regular expression, the corresponding transformation item is skipped (not executed). This makes it possible to configure multiple transformation items for the same `key` with different matching patterns, so that different values take effect for different hostnames/paths.

## Transformation Operation Types
| Operation Type | Key Field Meaning | Value Field Meaning | Description |
| :----: | :----: | :----: | ------------------------------------------------------------ |
| Remove remove | Target key | Not required | If the specified `key` exists, delete it; otherwise, no operation |
| Rename rename | Target oldKey | New key name newKey | If the specified `oldKey:value` exists, rename its key to `newKey`, resulting in `newKey:value`; otherwise, no operation |
| Replace replace | Target key | New value newValue | If the specified `key:value` exists, update its value to `newValue`, resulting in `key:newValue`; otherwise, it is equivalent to performing add operation |
| Add add | Added key | Added value | If the specified `key:value` does not exist, add it; otherwise, no operation |
| Append append | Target key | Appending value appendValue | If the specified `key:value` exists, append appendValue to get `key:[value..., appendValue]`; otherwise, it is equivalent to performing add operation, resulting in `key:appendValue`. |
| Map map | Mapping source fromKey | Mapping target toKey | If the specified `fromKey:fromValue` exists, map its value fromValue to the value of toKey, resulting in `toKey:fromValue`, while retaining `fromKey:fromValue` (note: if toKey already exists, its value will be overwritten); otherwise, no operation. |
| Deduplicate dedupe | Target key | Specified deduplication strategy strategy | `strategy` optional values include: <br>`RETAIN_UNIQUE`: Retain all unique values in order, e.g., `k1:[v1,v2,v3,v3,v2,v1]`, deduplication results in `k1:[v1,v2,v3]`. <br>`RETAIN_LAST`: Retain the last value, e.g., `k1:[v1,v2,v3]`, deduplication results in `k1:v3`. <br>`RETAIN_FIRST` (default): Retain the first value, e.g., `k1:[v1,v2,v3]`, deduplication results in `k1:v1`. <br>`SPLIT_AND_RETAIN_FIRST`: Split the value by comma and retain the first value, e.g., `k1:"v1,v2,v3"`, deduplication results in `k1:v1`. <br>`SPLIT_AND_RETAIN_LAST`: Split the value by comma and retain the last value, e.g., `k1:"v1,v2,v3"`, deduplication results in `k1:v3`. <br>(Note: When deduplication results in only one element v1, the key-value pair becomes `k1:v1`, not `k1:[v1]`.) |

## Configuration Example

### Implement Routing Based on Body Parameters
Configuration example:
```yaml
reqRules:
- operate: map
  headers:
  - fromKey: userId
    toKey: x-user-id
  mapSource: body
```
This rule extracts the `userId` from the request body and sets it in the request header `x-user-id`. This allows routing based on body parameters using Higress's ability to match on request headers.

This configuration supports both `application/json` and `application/x-www-form-urlencoded` types of request bodies. 

For example:
**For application/json type body**
```bash
curl localhost -d '{"userId":12, "userName":"johnlanni"}' -H 'content-type:application/json'
```
The value of the `userId` field will be extracted from the JSON and set to `x-user-id`. The backend service will receive a request header with: `x-user-id: 12`.

After the plugin adds this header, the gateway will recalculate the routes, allowing the routing configuration to match the specific target service based on this request header.

**For application/x-www-form-urlencoded type body**
```bash
curl localhost -d 'userId=12&userName=johnlanni'
```
The value of the `userId` field will be extracted from the form format `k1=v1&k2=v2` and set to `x-user-id`. The backend service will receive a request header with: `x-user-id: 12`.

After the plugin adds this header, the gateway will recalculate the routes, allowing the routing configuration to match the specific target service based on this request header.

#### JSON Path Support
You can extract fields from complex JSON according to [GJSON Path syntax](https://github.com/tidwall/gjson/blob/master/SYNTAX.md). 

Common operations include, for the following JSON:
```json
{
  "name": {"first": "Tom", "last": "Anderson"},
  "age": 37,
  "children": ["Sara","Alex","Jack"],
  "fav.movie": "Deer Hunter",
  "friends": [
    {"first": "Dale", "last": "Murphy", "age": 44, "nets": ["ig", "fb", "tw"]},
    {"first": "Roger", "last": "Craig", "age": 68, "nets": ["fb", "tw"]},
    {"first": "Jane", "last": "Murphy", "age": 47, "nets": ["ig", "tw"]}
  ]
}
```
You can achieve such extractions:
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
Now, if you want to extract the `first` field from the second item in `friends` from the above JSON formatted body and set it to the header `x-first-name`, while also extracting the `last` field to set it to the header `x-last-name`, you can use this plugin configuration:
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
#### Transforming Request Headers
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
    value: host-\$1
    host_pattern: ^(.*)\.com$
- operate: append
  headers:
  - key: X-add-append
    appendValue: path-\$1
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
Send Request
```bash
$ curl -v console.higress.io/get -H 'host: foo.bar.com' \
-H 'X-remove: exist' -H 'X-not-renamed:test' -H 'X-replace:not-replaced' \
-H 'X-dedupe-first:1' -H 'X-dedupe-first:2' -H 'X-dedupe-first:3' \
-H 'X-dedupe-last:a' -H 'X-dedupe-last:b' -H 'X-dedupe-last:c' \
-H 'X-dedupe-unique:1' -H 'X-dedupe-unique:2' -H 'X-dedupe-unique:3' \
-H 'X-dedupe-unique:3' -H 'X-dedupe-unique:2' -H 'X-dedupe-unique:1'
# httpbin response result
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
#### Transforming Request Query Parameters
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
    value: v31-\$1
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
Send Request
```bash
$ curl -v "console.higress.io/get?k1=v11&k1=v12&k2=v2"
# httpbin response result
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
#### Transforming Request Body
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
    appendValue: t1-\$1-append
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
Send Requests:
**1. Content-Type: application/json**
```bash
$ curl -v -X POST console.higress.io/post -H 'host: foo.bar.com' \
-H 'Content-Type: application/json' -d '{"a1":"t1","a2":"t2","a3":"t3"}'
# httpbin response result
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
# httpbin response result
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
**3. Content-Type: multipart/form-data**
```bash
$ curl -v -X POST console.higress.io/post -H 'host: foo.bar.com' \
-F a1=t1 -F a2=t2 -F a3=t3
# httpbin response result
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

#### Prohibit rerouting of the target  By default, the target is rerouted during the request transformation process. If you do not want to reroute the target, you can set `reroute` to `false`:

```yaml
reroute: false
reqRules:
  - operate: replace
    headers:
      - key: reroute
        newValue: true
```

Assuming the routing configuration is:

- Path prefix matches /, header exactly matches reroute: false, the target service responds with 200 “no rerouting”
- Path prefix matches /, header exactly matches reroute: true, the target service responds with 200 “rerouting”

Then, based on the above configuration, the request results are:

```bash
# The plugin replaces the header from reroute: false to reroute: true, but prohibits re-selecting the route target
$ curl console.higress.io/get -H ‘host: foo.bar.com’ -H ‘reroute: false’
no rerouting%

# Remove the reroute configuration from the plugin or set it to true, then
# the plugin will replace the header from reroute: false to reroute: true and re-select the routing target
$ curl console.higress.io/get -H ‘host: foo.bar.com’ -H ‘reroute: false’
rerouting% 
```

Translated with DeepL.com (free version)


### Response Transformer
Similar to Request Transformer, this only describes the precautions for transforming JSON-formatted request/response bodies:

#### Key Nesting `.`
1. In general, a key containing `.` indicates a nested meaning, as follows:
```yaml
respRules:
- operate: add
  body:
  - key: foo.bar
    value: value
```
```bash
$ curl -v console.higress.io/get
# httpbin response result
{
 ...
 "foo": {
  "bar": "value"
 },
 ...
}
```
2. When using `\.` to escape `.` in the key, it indicates a non-nested meaning, as follows:
> When enclosing a string with double quotes, use `\\.` for escaping
```yaml
respRules:
- operate: add
  body:
  - key: foo\.bar
    value: value
```
```bash
$ curl -v console.higress.io/get
# httpbin response result
{
 ...
 "foo.bar": "value",
 ...
}
```
#### Accessing Array Elements `.index`
You can access array elements by their index `array.index`, where the index starts from 0:
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
1. Remove the first element of `user`:
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
# httpbin response result
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
2. Rename the key `123` of the first element of `users` to `msg`:
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
# httpbin response result
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
#### Iterating Array Elements `.#`
You can use `array.#` to iterate over an array:
> ❗️This operation can only be used in replace, do not attempt this operation in other transformations to avoid unpredictable results
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
# httpbin response result
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
