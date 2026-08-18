# 功能说明

`rewrite` 插件可以用于修改请求域名（Host）以及请求路径（Path），通常用于后端服务的域名/路由与网关侧域名/路由不一致时的配置，与 [Rewrite Annotation](https://higress.io/zh-cn/docs/user/annotation-use-case/#rewrite%E9%87%8D%E5%86%99path%E5%92%8Chost) 实现的效果一致。


# 配置字段

| 名称            | 数据类型            | 填写要求 | 默认值 | 描述                           |
|---------------|-----------------|------|-----|------------------------------|
| rewrite_rules | array of object | 选填   | -   | 配置请求域名（Host）与请求路径（Path）的重写规则 |

`rewrite_rules` 中配置字段说明如下：

| 名称              | 数据类型            | 填写要求 | 默认值   | 描述                                                                            |
|-----------------|-----------------|------|-------|-------------------------------------------------------------------------------|
| match_path_type | string          | 选填   | -     | 配置请求路径的匹配类型，可选值为：前缀匹配 prefix, 精确匹配 exact, 正则匹配 regex                          |
| case_sensitive  | bool            | 选填   | false | 配置匹配时是否区分大小写，默认不区分                                                            |
| match_hosts     | array of string | 选填   | -     | 配置会被重写的请求域名列表，支持精确匹配（hello.world.com），最左通配（\*.world.com）和最右通配（hello.world.\*） |
| match_paths     | array of string | 选填   | -     | 配置会被重写的请求路径列表，支持 [RE2](https://pkg.go.dev/regexp/syntax) 正则表达式语法              |
| rewrite_host    | string          | 选填   | -     | 配置重写的目标域名                                                                     |
| rewrite_path    | string          | 选填   | -     | 配置重写的目标路径                                                                     |

**注意：**
- 只有当请求域名（Host）和请求路径（Path）都对应匹配到某条重写规则的 `match_hosts`、`match_paths` 中的一项时，才会将请求域名和请求路径分别重写为该条重写规则的 `rewrite_host`、`rewrite_path`；
- 当配置多条重写规则，将按照规则编写顺序进行匹配；
- 在一条重写规则中，`match_hosts` 和 `match_paths` 按照编写顺序进行匹配;
- `case_sensitive` 也会作用到正则表达式的匹配上。


# 配置示例

## 前缀匹配请求路径

以下配置将请求路径的匹配类型设置为前缀匹配（prefix）：

```yaml
rewrite_rules:
  - match_path_type: prefix # 前缀匹配
    case_sensitive: false
    match_hosts:
      - foo.bar.com
    match_paths:
      - /v1/api/get
    rewrite_host: prefix.example.com
    rewrite_path: /get
```

示例请求 `foo.bar.com/v1/api/get/something` 将被重写为 `prefix.example.com/get`。

## 正则匹配请求路径

以下配置将请求路径的匹配类型设置为正则匹配（regex）：

```yaml
rewrite_rules:
  - match_path_type: regex # 正则匹配
    case_sensitive: false
    match_hosts:
      - aa.bb.cc
      - foo.bar.com
    match_paths:
      - /abc/(get)
      - /(get)/.*\.html
    rewrite_host: regex.example.com
    rewrite_path: /$1
```

以下示例请求将被重写为 `regex.example.com/get`：
- `foo.bar.com/abc/get`；
- `aa.bb.cc/get/index.html`。


## 通配请求域名

以下配置演示请求域名的精准匹配和最左、最右通配：

```yaml
rewrite_rules:
  - match_path_type: exact
    case_sensitive: false
    match_hosts:
      - "hello.world.com"  # 精准匹配
      - "*.example.com"    # 最左通配
      - "aa.bb.*"          # 最右通配
    match_paths:
      - /v1/get
      - /abc/get/
    rewrite_host: wildcard.example.com
    rewrite_path: /get
```

以下示例请求将被重写为 `wildcard.example.com/get`：
- `hello.world.com/abc/get/`；
- `my.example.com/v1/get`；
- `aa.bb.com/v1/get`。

## 大小写敏感

以下配置将请求域名和请求路径的匹配设置为大小写敏感：

```yaml
rewrite_rules:
  - match_path_type: regex
    case_sensitive: true # 大小写敏感
    match_hosts:
      - xx.yy.ZZ
    match_paths:
      - /API/(get)
      - /test/(get)/.*\.HTML
    rewrite_host: case-sensitive.example.com
    rewrite_path: /$1
```

以下示例请求将被重写为 `case-sensitive.example.com/get`：
- `xx.yy.ZZ/API/get`；
- `xx.yy.ZZ/test/get/index.HTML`。

而以下示例请求则因为大小写敏感而无法正确匹配：
- `xx.yy.ZZ/api/get`；
- `xx.yy.zz/test/get/index.HTML`。