# Shared Higress MCP Lab Environment

[中文](./README.md) | [Back to MCP Demo](../README_EN.md)

This directory starts the protocol-independent environment shared by every MCP demo. It uses Kind and Helm to install the complete Higress Controller, Gateway, CRDs, and Console, then deploys an observable ordinary HTTP weather service.

The shared environment mounts `.runtime/plugins` into `/opt/plugins` in the Gateway. Build the protocol plugin with `plugin/build.sh` under the corresponding version directory before starting the environment.

## Prerequisites

- Docker or Podman;
- Kind;
- kubectl;
- Helm;
- curl;
- jq for the assertions in each demo;
- approximately 4 GiB of available memory;
- access to the Higress Helm and image registries.

## Start

From the Higress repository root, run:

```bash
cd samples/mcp
./protocol/2026-07-28/plugin/build.sh
./environment/scripts/up.sh
```

The script:

1. creates the `higress-mcp-demo` Kind cluster;
2. mounts `.runtime/plugins` at `/opt/plugins` in the Kind node;
3. installs Higress with the pinned `2.2.3` Helm chart;
4. builds and deploys the protocol-neutral `observable-weather` service;
5. starts local port forwards for the Gateway, Console, and backend.

The scripts record the container engine, cluster name, and instance ID under `.runtime`, with a matching ownership marker inside the cluster. Start and cleanup commands refuse to operate on a cluster with the same name unless it was created by this demo. Set `MCP_DEMO_CLUSTER` to choose another name.

| Address | Purpose |
| --- | --- |
| `http://127.0.0.1:18080` | Higress Gateway |
| `http://127.0.0.1:18081` | Higress Console |
| `http://127.0.0.1:18082` | Observable HTTP backend |
| `http://127.0.0.1:18082/__state` | Inspect backend calls |
| `POST http://127.0.0.1:18082/__reset` | Reset backend calls |

## Status

```bash
./environment/scripts/status.sh
```

This displays pods in `higress-system` and `mcp-demo` and checks all three port forwards.

## Clean up

```bash
./environment/scripts/down.sh
```

This stops port forwards and deletes the Kind cluster created by this demo. Built plugins remain under `.runtime/plugins` for reuse and are ignored by Git.

## Shared-backend boundary

`observable-weather` implements ordinary HTTP only:

- `GET /weather?location=<city>`;
- `GET /healthz`;
- `GET /__state`;
- `POST /__reset`.

It knows nothing about MCP versions, JSON-RPC methods, or sessions. Each demo provides any MCP-version-specific upstream fixture it needs.

## Overrides

```bash
MCP_DEMO_CLUSTER=my-mcp-demo \
MCP_DEMO_GATEWAY_PORT=28080 \
MCP_DEMO_CONSOLE_PORT=28081 \
MCP_DEMO_BACKEND_PORT=28082 \
./environment/scripts/up.sh
```

`MCP_DEMO_KIND_NODE_IMAGE` and `MCP_DEMO_HIGRESS_CHART_VERSION` can also be overridden.
