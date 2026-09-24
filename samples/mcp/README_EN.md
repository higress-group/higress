# Higress MCP Demo

[中文](./README.md)

This directory maintains runnable and reproducible Higress MCP demos. It provides a complete local Higress environment and archives plugin builds and implemented capability walkthroughs by MCP protocol version.

## Goals

- **Complete environment**: use Kind and Helm to run the Higress Controller, Gateway, CRDs, and Console.
- **Reusable common dependencies**: keep protocol-independent cluster setup, Higress configuration, and mock business backends under `environment/`.
- **Explicit version boundaries**: keep protocol-specific plugin source commits, build instructions, and demos under `protocol/<version>/`.
- **Step-by-step verification**: make every demo an independent README that documents its goal, commands, expected responses, and gateway or backend evidence.
- **Independent execution**: readers can select a capability and follow its README.

## Quick start

Run these commands from the Higress repository root:

```bash
cd samples/mcp

# 1. Build the MCP Server plugin for the target protocol version
./protocol/2026-07-28/plugin/build.sh

# 2. Start the shared Higress environment
./environment/scripts/up.sh

# 3. Pick a demo and follow its README
less ./protocol/2026-07-28/01-stateless-http/README_EN.md

# 4. Tear down the environment after all experiments
./environment/scripts/down.sh
```

See the [shared environment guide](./environment/README_EN.md) for prerequisites, ports, startup behavior, and troubleshooting.

## Demo catalog

| MCP protocol version | Plugin source | Available demos |
| --- | --- | --- |
| [2026-07-28](./protocol/2026-07-28/README_EN.md) | Built locally from a pinned Higress source commit | Stateless HTTP, REST-to-MCP, modern-to-legacy, and early request validation |

## Layout

```text
samples/mcp/
├── README.md
├── README_EN.md
├── environment/                    # Shared, protocol-independent environment
│   ├── kind/                       # Kind cluster definition
│   ├── higress/                    # Higress Helm values
│   ├── apps/                       # Generic mock business backends
│   └── scripts/                    # Start, status, and teardown helpers
└── protocol/
    └── <protocol-version>/
        ├── README.md               # Version capability and demo index
        ├── plugin/                 # Plugin build pinned to a source commit
        └── <number>-<feature>/     # One independent implemented capability
            ├── README.md
            ├── README_EN.md
            ├── resources.yaml
            ├── requests/
            └── fixture/            # Present only when this demo needs one
```

## Placement rules

`environment/` contains Kind, Higress Helm configuration, and plain REST business backends that are independent of a specific MCP wire contract. Fixtures for legacy MCP Servers, version-specific client behavior, and protocol-specific messages live with their corresponding demos.

Every demo README should include at least:

1. the verification goal and out-of-scope behavior;
2. prerequisites and applicable protocol and plugin versions;
3. copyable step-by-step commands;
4. expected responses and assertion commands for each step;
5. necessary Gateway, plugin, or backend evidence;
6. commands that remove resources created by that demo.

## Adding a protocol version or demo

For a new protocol release, record the Higress commit that implements it and its build method under `protocol/<version>/plugin/`.

For a new demo:

- create it directly under its protocol version and use a numeric prefix for reading order;
- give the directory one primary verification goal and document cross-capability dependencies;
- provide Chinese and English READMEs, deterministic requests, and expected evidence;
- require no real credentials or private services, and do not commit generated WASM files, runtime logs, or temporary evidence.
