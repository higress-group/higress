# Higress McpBridge destination 注解

> 中文版本是内容基准；[英文版本](mcpbridge-destination-annotation-en.md)为同步翻译。

`higress.io/destination` 支持按行配置多个目的地。每一行都是独立解析的，格式如下：

```text
[weight%] [http://|https://]<host>[:port] [subset]
```

## URI 语法

- 仅支持 `http://` 和 `https://` 两种 scheme。
- scheme 只作用于当前这一条目的地，不会影响同一个注解里的其他目的地。
- 如果省略 scheme，Higress 不会为该条目记录独立协议；
  该条目会回退到 `higress.io/backend-protocol`。
- 如果 `higress.io/backend-protocol` 也未配置，则该条目按默认 HTTP 行为处理。

## 优先级

- 带 scheme 的目的地条目优先于 `higress.io/backend-protocol`，但只对当前条目生效。
- 不带 scheme 的条目继续使用 `higress.io/backend-protocol` 作为回退。
- 同一个 `higress.io/destination` 中可以混用 HTTP 和 HTTPS 条目。

## 示例

```yaml
metadata:
  annotations:
    higress.io/backend-protocol: HTTPS
    higress.io/destination: |
      34% http://plain.example.com:80
      33% https://secure.example.com:443
      33% inherited.example.com:8443
```

上面的配置中：

- 第一条目的地强制使用 HTTP。
- 第二条目的地显式使用 HTTPS。
- 第三条目的地没有 scheme，因此继承 `higress.io/backend-protocol: HTTPS`。
