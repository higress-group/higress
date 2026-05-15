---
title: Nginx Rewrite 兼容迁移
keywords: [higress, nginx, rewrite, set, migration]
description: nginx rewrite + set 安全迁移插件说明
---

## 功能说明

`nginx-rewrite-compatible` 插件提供与 `nginx rewrite + set` 指令组合等价的常见能力，包括路径重写、查询串追加或替换、正则捕获组变量保存，以及通过请求头将变量传递给上游服务。

这个插件面向从 Nginx 迁移到 Higress 的场景，作为安全替代方案，避免继续依赖受 `CVE-2026-42945` 影响的重写链路。

## 安全背景

`CVE-2026-42945` 是一个与 Nginx `rewrite` 和 `set` 指令组合相关的长期堆溢出问题，漏洞存在约 18 年。其触发条件集中在以下模式：

1. `rewrite` 使用带 `?` 的替换目标，在一次 rewrite pass 中同时修改 URI 和 query string。
2. `set` 在后续步骤中继续引用前一次正则匹配得到的捕获组，如 `$1`、`$2`。
3. 两次 pass 之间，rewrite 状态和捕获组状态没有保持一致，导致后续 `set` 读取了不匹配的捕获组元数据，最终触发越界访问和堆溢出。

Higress 的 WASM 插件没有这个问题，原因是：

1. 每个请求都在独立的 WASM 上下文中处理。
2. 本插件在一次请求回调内完成“匹配、重写、变量保存、向上游透传”全过程，不依赖 Nginx rewrite module 的两阶段状态机。
3. 捕获组结果只存在当前请求的内存和请求属性中，不会跨 pass 泄漏，也不会复用失配状态。

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`100`

## 配置字段

| 字段名 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `rules` | array of object | 是 | - | 重写规则列表，按顺序执行 |

### `rules` 配置说明

| 字段名 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `regex` | string | 是 | - | 匹配请求 path 的正则表达式，不包含 query string |
| `replacement` | string | 是 | - | 新的路径模板，支持 `$1`、`$2` 等捕获组引用 |
| `query_append` | string | 否 | - | 追加到原 query string 的片段，支持 `$1`、`$2` |
| `query_template` | string | 否 | - | 完全替换原 query string 的模板，支持 `$1`、`$2` |
| `set_vars` | array of object | 否 | - | 将捕获组写入请求级变量，或按变量名前缀改写 query/header/cookie |
| `pass_to_upstream` | bool | 否 | `false` | 是否把当前规则的变量同时写入请求头传给上游 |
| `mode` | string | 否 | `last` | 规则流转模式，支持 `break` 和 `last` |

说明：

1. `query_append` 和 `query_template` 不能同时配置。
2. `mode: break` 表示命中当前规则后停止继续匹配后续规则。
3. `mode: last` 表示命中当前规则后，使用重写后的 path 继续匹配后续规则。
4. `set_vars` 中，`arg_` 前缀会修改请求 query parameter，`http_` 前缀会修改请求 header，`cookie_` 前缀会修改请求 cookie，其他变量名会通过 `proxywasm.SetProperty([]string{"nginx_rewrite_compatible","vars",name})` 保存。
5. `http_` 前缀对应的 header 名称会去掉前缀，并把下划线转换成横杠。
6. 当 `pass_to_upstream: true` 时，变量还会额外写入请求头 `x-higress-rewrite-var-<name>`。

### `set_vars` 配置说明

| 字段名 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `name` | string | 是 | 变量名。`arg_`/`http_`/`cookie_` 前缀分别表示写 query parameter、header、cookie；其他名称表示自定义属性 |
| `capture_group` | int | 是 | 捕获组编号，`0` 表示整个匹配，`1` 表示第一个分组 |

## Nginx 配置对照表

### 1. 简单路径重写

Nginx:

```nginx
rewrite ^/old/(.*)$ /new/$1;
```

插件配置:

```yaml
rules:
  - regex: ^/old/(.*)$
    replacement: /new/$1
```

### 2. 正则捕获组替换

Nginx:

```nginx
rewrite ^/product/([0-9]+)$ /detail/$1;
```

插件配置:

```yaml
rules:
  - regex: ^/product/([0-9]+)$
    replacement: /detail/$1
```

### 3. Query String 操作

追加 query:

```nginx
rewrite ^/api/(.*)$ /internal?migrated=true;
```

```yaml
rules:
  - regex: ^/api/(.*)$
    replacement: /internal
    query_append: migrated=true
```

替换 query:

```nginx
rewrite ^/x/(.*)/(.*)$ /y?a=$1&b=$2;
```

```yaml
rules:
  - regex: ^/x/(.*)/(.*)$
    replacement: /y
    query_template: a=$1&b=$2
```

### 4. 特殊变量前缀

```yaml
rules:
  - regex: "^/api/(.*)/(.*)$"
    replacement: "/internal"
    set_vars:
      - name: original_path
        capture_group: 1
      - name: http_x_original
        capture_group: 1
      - name: arg_source
        capture_group: 2
      - name: cookie_track_id
        capture_group: 1
```

语义：

1. `original_path` 保存为请求属性，可供后续插件通过 `GetProperty(["nginx_rewrite_compatible","vars","original_path"])` 读取。
2. `http_x_original` 设置请求头 `x-original`。
3. `arg_source` 设置 query parameter `source`。
4. `cookie_track_id` 设置 cookie `track_id`。

### 5. 变量保存与传递

Nginx:

```nginx
rewrite ^/api/(.*)$ /internal?migrated=true;
set $original_endpoint $1;
```

插件配置:

```yaml
rules:
  - regex: ^/api/(.*)$
    replacement: /internal
    query_append: migrated=true
    set_vars:
      - name: original_endpoint
        capture_group: 1
    pass_to_upstream: true
```

### 6. 多规则组合

Nginx:

```nginx
rewrite ^/stage/(.*)$ /mid/$1;
rewrite ^/mid/(.*)$ /final/$1;
```

插件配置:

```yaml
rules:
  - regex: ^/stage/(.*)$
    replacement: /mid/$1
    mode: last
  - regex: ^/mid/(.*)$
    replacement: /final/$1
```

### 7. break / last 控制

Nginx `break`:

```nginx
rewrite ^/stage/(.*)$ /mid/$1 break;
```

```yaml
rules:
  - regex: ^/stage/(.*)$
    replacement: /mid/$1
    mode: break
```

Nginx `last`:

```nginx
rewrite ^/stage/(.*)$ /mid/$1 last;
rewrite ^/mid/(.*)$ /final/$1;
```

```yaml
rules:
  - regex: ^/stage/(.*)$
    replacement: /mid/$1
    mode: last
  - regex: ^/mid/(.*)$
    replacement: /final/$1
```

## 使用示例

```yaml
rules:
  - regex: ^/api/(.*)$
    replacement: /internal
    query_append: migrated=true
    set_vars:
      - name: original_endpoint
        capture_group: 1
      - name: http_x_original_endpoint
        capture_group: 1
      - name: arg_source
        capture_group: 1
      - name: cookie_track_id
        capture_group: 1
    pass_to_upstream: true
    mode: break

  - regex: ^/old/(.*)$
    replacement: /new/$1

  - regex: ^/x/(.*)/(.*)$
    replacement: /y
    query_template: a=$1&b=$2
    set_vars:
      - name: first
        capture_group: 1
      - name: second
        capture_group: 2
```
