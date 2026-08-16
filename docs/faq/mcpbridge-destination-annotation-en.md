# Higress McpBridge destination annotation

> Chinese is the canonical source; this document is its synchronized English translation.

`higress.io/destination` supports multiple destinations, one per line. Each line
is parsed independently with the following format:

```text
[weight%] [http://|https://]<host>[:port] [subset]
```

## URI syntax

- Only `http://` and `https://` schemes are supported.
- The scheme applies only to the current destination line and does not affect other destinations in the same annotation.
- If the scheme is omitted, Higress does not record a per-destination protocol
  for that entry; the entry falls back to `higress.io/backend-protocol`.
- If `higress.io/backend-protocol` is also unset, the entry uses the default HTTP behavior.

## Precedence

- A destination line with an explicit scheme overrides `higress.io/backend-protocol` for that line only.
- A line without a scheme continues to inherit `higress.io/backend-protocol` as its fallback.
- A single `higress.io/destination` annotation can mix HTTP and HTTPS destinations.

## Example

```yaml
metadata:
  annotations:
    higress.io/backend-protocol: HTTPS
    higress.io/destination: |
      34% http://plain.example.com:80
      33% https://secure.example.com:443
      33% inherited.example.com:8443
```

In this configuration:

- The first destination is forced to use HTTP.
- The second destination explicitly uses HTTPS.
- The third destination has no scheme, so it inherits `higress.io/backend-protocol: HTTPS`.
