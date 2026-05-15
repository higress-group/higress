---
title: Nginx Rewrite Compatibility Migration
keywords: [higress, nginx, rewrite, set, migration]
description: Secure migration plugin for nginx rewrite + set
---

## Features

The `nginx-rewrite-compatible` plugin provides the common behavior of `nginx rewrite + set`, including path rewrites, query append or replacement, capture-group variable storage, and optional variable propagation to upstream services through request headers.

It is designed as a secure migration alternative when moving from Nginx to Higress, so users do not need to keep relying on the rewrite path affected by `CVE-2026-42945`.

## Security Background

`CVE-2026-42945` is a long-standing heap overflow issue related to the interaction between Nginx `rewrite` and `set`. The vulnerable pattern is:

1. A `rewrite` rule uses a replacement containing `?`, so URI and query string are updated during one rewrite pass.
2. A later `set` still references capture groups such as `$1` or `$2`.
3. The state kept across rewrite passes becomes inconsistent, so `set` reads capture-group metadata from a mismatched state and eventually triggers out-of-bounds access and heap corruption.

The Higress WASM approach does not have this problem because:

1. Each request is handled in an isolated WASM request context.
2. This plugin performs match, rewrite, variable extraction, and upstream propagation in one request callback instead of relying on Nginx's multi-pass rewrite state machine.
3. Capture-group data lives only inside the current request and request properties, so there is no cross-pass state leakage.

## Runtime Properties

Plugin execution phase: `UNSPECIFIED`
Plugin execution priority: `100`

## Configuration Fields

| Field Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `rules` | array of object | Yes | - | Ordered rewrite rules |

### `rules`

| Field Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `regex` | string | Yes | - | Regular expression that matches the request path without the query string |
| `replacement` | string | Yes | - | New path template. Supports capture references such as `$1` and `$2` |
| `query_append` | string | No | - | Query fragment appended to the existing query string. Supports `$1`, `$2` |
| `query_template` | string | No | - | Query template that replaces the existing query string. Supports `$1`, `$2` |
| `set_vars` | array of object | No | - | Stores capture groups as request-scoped variables, or rewrites query/header/cookie based on variable prefixes |
| `pass_to_upstream` | bool | No | `false` | Whether variables from the current rule should also be written into upstream request headers |
| `mode` | string | No | `last` | Rule flow mode. Supported values: `break`, `last` |

Notes:

1. `query_append` and `query_template` are mutually exclusive.
2. `mode: break` stops evaluation after the current matching rule.
3. `mode: last` continues evaluating the following rules with the rewritten path.
4. In `set_vars`, the `arg_` prefix rewrites a request query parameter, `http_` rewrites a request header, `cookie_` rewrites a request cookie, and any other name is stored with `proxywasm.SetProperty([]string{"nginx_rewrite_compatible","vars",name})`.
5. For `http_`, the header name is derived by removing the prefix and converting underscores to hyphens.
6. When `pass_to_upstream: true`, variables are also written to `x-higress-rewrite-var-<name>`.

### `set_vars`

| Field Name | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | Yes | Variable name. `arg_`/`http_`/`cookie_` mean query parameter, header, and cookie updates respectively; other names become custom properties |
| `capture_group` | int | Yes | Capture-group index. `0` means the whole match and `1` means the first group |

## Nginx Mapping Table

### 1. Simple Path Rewrite

Nginx:

```nginx
rewrite ^/old/(.*)$ /new/$1;
```

Plugin:

```yaml
rules:
  - regex: ^/old/(.*)$
    replacement: /new/$1
```

### 2. Capture-Group Replacement

Nginx:

```nginx
rewrite ^/product/([0-9]+)$ /detail/$1;
```

Plugin:

```yaml
rules:
  - regex: ^/product/([0-9]+)$
    replacement: /detail/$1
```

### 3. Query String Operations

Append query:

```nginx
rewrite ^/api/(.*)$ /internal?migrated=true;
```

```yaml
rules:
  - regex: ^/api/(.*)$
    replacement: /internal
    query_append: migrated=true
```

Replace query:

```nginx
rewrite ^/x/(.*)/(.*)$ /y?a=$1&b=$2;
```

```yaml
rules:
  - regex: ^/x/(.*)/(.*)$
    replacement: /y
    query_template: a=$1&b=$2
```

### 4. Special Variable Prefixes

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

Semantics:

1. `original_path` is stored as a request property and can be read later with `GetProperty(["nginx_rewrite_compatible","vars","original_path"])`.
2. `http_x_original` sets the request header `x-original`.
3. `arg_source` sets the query parameter `source`.
4. `cookie_track_id` sets the cookie `track_id`.

### 5. Variable Preservation and Propagation

Nginx:

```nginx
rewrite ^/api/(.*)$ /internal?migrated=true;
set $original_endpoint $1;
```

Plugin:

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

### 6. Multiple Rules

Nginx:

```nginx
rewrite ^/stage/(.*)$ /mid/$1;
rewrite ^/mid/(.*)$ /final/$1;
```

Plugin:

```yaml
rules:
  - regex: ^/stage/(.*)$
    replacement: /mid/$1
    mode: last
  - regex: ^/mid/(.*)$
    replacement: /final/$1
```

### 7. `break` / `last`

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

## Example

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
