# MCP 2026-07-28 Demos

[中文](./README.md) | [Back to MCP Demo](../../README_EN.md)

This directory contains runnable Higress demos for implemented MCP `2026-07-28` features. The version directory builds the `mcp-server` plugin from a pinned Higress commit, while each feature README provides an independent step-by-step lab.

## Prepare the environment and plugin

From the Higress repository root, run:

```bash
cd samples/mcp
./protocol/2026-07-28/plugin/build.sh
./environment/scripts/up.sh
```

The default source is:

```text
https://github.com/higress-group/higress.git
14c36d9bd70b3dc38237cda6175b3f9dede0dccd
```

The build writes:

```text
.runtime/plugins/mcp-server/2026-07-28/plugin.wasm
```

The shared environment exposes this file to the Kind node and Higress Gateway. Every demo references:

```text
file:///opt/plugins/mcp-server/2026-07-28/plugin.wasm
```

## Demo catalog

| Demo | Feature |
| --- | --- |
| [01 Stateless HTTP](./01-stateless-http/README_EN.md) | Discover, list, and call without initialize or a protocol session |
| [02 REST-to-MCP](./02-rest-to-mcp/README_EN.md) | Translate one MCP Tool call into one ordinary REST request |
| [03 Modern-to-Legacy](./03-modern-to-legacy/README_EN.md) | Request-scoped legacy handshakes, result adaptation, and header isolation |
| [04 Request Validation](./04-request-validation/README_EN.md) | Argument, Origin, and header/body validation with zero backend calls |

Each demo can be run independently. Enter any demo directory and follow its README to deploy resources, send requests, inspect evidence, and clean up.

## Use another source revision

Edit `plugin/source.env` or override its variables:

```bash
MCP_DEMO_HIGRESS_REPOSITORY=https://github.com/<owner>/higress.git \
MCP_DEMO_HIGRESS_REF=<commit-sha> \
./protocol/2026-07-28/plugin/build.sh
```
