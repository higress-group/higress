# 功能说明
`static-files`插件支持nginx静态文件的转发配置，static files插件配置支持指令：
- `root`: 定义root路径，转发请求为root path + request path。
- `alias`: 定义alias路径，转发请求为alias path。
- `index`: 默认首页文件列表，比如["index.html", "index.php"]等。
- `try paths`: 请求基于不同的路径进行重试，直到请求到正确返回的请求。

# 配置字段

| 名称 | 数据类型 | 填写要求 |  默认值 | 描述 |
| -------- | -------- | -------- | -------- | -------- |
| `root`          | string  | 非必填    | -          | root路径     |
| `alias_path`    | string  | 非必填    | -          | alias待替换的路径 |
| `alias`         | string  | 非必填    | -          | alias路径     |
| `index`         | array of string  | 非必填    | -          | 首页文件列表     |
| `try_paths`       | array of string  | 必填    | -          | 转发路径列表，比如`index.html`，`$uri`, `index.html`     |
| `try_codes`       | array of int     | 非必填  | [403, 404] | 重试状态码，可自定义                                  |
| `timeout`        | int              | 非必填  | 1000       | 重试请求的超时时间，单位ms                             |


# 配置示例

## root

```yaml
root: "/b"

```

基于该配置开启插件，触发插件的请求`http://a.com/a`, 请求会转发为`http://upstream/b/a`。

## alias

```yaml
alias_path: "/a"
alias: "/b"

```

基于该配置开启插件，触发插件的请求`http://a.com/a`, 请求会转发为`http://upstream/b`。

## index

```yaml
alias_path: "/a"
alias: "/b"
index: 
- index.html

```

基于该配置开启插件，触发插件的请求`http://a.com/a`, 请求会依次转发为:
- http://upstream/b
- http://upstream/b/index.html


## try_paths

```yaml
try_paths:
- "$uri/"
- "$uri.html"
- "/index.html"

```

基于该配置开启插件，触发插件的请求`http://a.com/a`, 请求会依次转发为:
- http://upstream/a/
- http://upstream/a.html
- http://upstream/index.html
如果请求返回码不是重试状态码，会直接返回该请求体，否则继续转发下一个请求，所有请求都不是重试状态码，会继续转发到默认后端服务。
