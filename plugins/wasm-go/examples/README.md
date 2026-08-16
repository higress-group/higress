# Go Wasm plugin examples

This directory contains reference implementations and development examples.
They are intentionally excluded from official plugin release discovery, which
only scans `plugins/wasm-go/extensions/`.

The following examples are Go counterparts of official C++ plugins:

- `basic-auth`
- `bot-detect`
- `custom-response`
- `jwt-auth`
- `key-auth`
- `request-block`
- `sni-misdirect`

`model-mapper` and `model-router` remain official Go plugins under
`extensions/`; their C++ counterparts do not make them reference-only.

Build one from `plugins/wasm-go/` with:

```bash
PLUGIN_ROOT=examples PLUGIN_NAME=request-block make build
```

These examples may keep historical `VERSION` and `.buildrc` files for local
compatibility. Their presence does not make them release artifacts.
