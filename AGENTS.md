# AGENTS.md

Guidance for AI agents working in this repository.

Higress is a cloud-native API gateway built on Istio and Envoy. The control
plane extends Istio/pilot (Go); the data plane is Envoy extended with WASM
plugins (Go/Rust/C++/AssemblyScript) and a Go-based `golang-filter`. It supports
Ingress/Gateway API and ships a rich plugin ecosystem (including AI gateway
plugins).

## Repository layout

Top-level directories (all paths relative to repo root):

- `cmd/higress/` — main entrypoint (`main.go`) for the Higress controller binary.
- `pkg/` — core Go control-plane packages: `bootstrap/`, `cert/`, `cmd/`,
  `common/`, `config/`, `ingress/` (Ingress/Gateway config translation),
  `kube/`.
- `api/` — protobuf/CRD API definitions; Higress CRDs live in
  `api/extensions/v1alpha1` (e.g. the `WasmPlugin` type). Generated with
  `make gen-api` / `make gen-client` (see `api/gen.sh`, `buf.*`).
- `client/` — generated Go clientset for Higress CRDs.
- `istio/` — git submodules of higress-group forks of Istio (`api`, `istio`,
  `client-go`, `pkg`, `proxy`); see `.gitmodules`. Pulled via `make submodule`
  (part of `prebuild`).
- `envoy/` — Envoy + `go-control-plane` submodules (higress-group forks).
- `external/` — vendored/external mirror dirs used during build (istio, envoy,
  proxy, etc.).
- `plugins/` — all data-plane plugins (see "Plugins" below).
- `registry/` — service-discovery registry integrations (nacos, consul, eureka,
  zookeeper, direct, mcp, ...).
- `hgctl/` — the `hgctl` CLI (separate Go module) for managing Higress.
- `helm/` — Helm charts: `helm/core` (the dev/install chart) and `helm/higress`.
- `test/` — `test/e2e/` (conformance/e2e, see "Build & test") and
  `test/gateway/`.
- `tools/` — build/CI scripting: `tools/hack/` (build scripts), `tools/bin/`,
  `tools/linter/`, `*.mk`.
- `samples/` — example manifests (gateway-api, hello-world, wasmplugin, ...).
- `docker/`, `docs/`, `release-notes/` — packaging, docs, and release notes.
- `Makefile` — istio common-files wrapper (supports `BUILD_WITH_CONTAINER`);
  real targets live in `Makefile.core.mk` (+ `Makefile.overrides.mk`).

## Plugins

All plugins live under `plugins/`. See `plugins/README.md` for the contributor
overview. Prebuilt plugin images are published to
`higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins`.

### plugins/wasm-go/ (primary WASM plugin framework, Go)

- `extensions/<name>/` — one directory per plugin (~59 plugins, many `ai-*`).
  Each plugin is its own Go module: `main.go`, `go.mod`/`go.sum`, `VERSION`,
  `README.md`(+`README_EN.md`), often `config/`, `util/`, `main_test.go`.
  Optional `.buildrc` sets `EXTRA_TAGS`; optional `prepare.sh`/`prepare.sh`.
  `plugin.wasm` is a build artifact and is **not** committed.
- Shared SDK: plugins depend on external modules
  `github.com/higress-group/wasm-go` and
  `github.com/higress-group/proxy-wasm-go-sdk` (NOT an in-repo SDK dir).
  In-repo, `plugins/wasm-go/pkg/mcp/` provides MCP helpers and
  `plugins/wasm-go/mcp-servers/` holds MCP server plugins.
- `examples/` — minimal reference plugins (custom-log, custom-span-attribute,
  test-foreign-function).
- Build: `plugins/wasm-go/Makefile`. `PLUGIN_NAME=<name> make build` builds a
  wasm file (output to `extensions/<name>/plugin.wasm`) + image via
  `Dockerfile`/`DockerfileBuilder` (uses a `wasm-go-builder` image, Go 1.24,
  TinyGo optional). `make build-push` pushes the image; `make local-build`
  builds locally with `GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared`.
- Conventions: `VERSION` is the image tag; the CI/e2e batch builder
  (`tools/hack/build-wasm-plugins.sh`) only compiles a wasm-go plugin whose
  `VERSION` ends in `-alpha` (see the section at the bottom of this file).

### plugins/wasm-rust/ (Rust WASM plugins)

- Workspace-style: root `Cargo.toml`/`Cargo.lock`, shared `src/`,
  `extensions/<name>/` per plugin (e.g. `ai-data-masking`, `ai-intent`,
  `request-block`, `say-hello`, `demo-wasm`), `example/`.
- Build via `plugins/wasm-rust/Makefile` (`PLUGIN_NAME=<name> make build`, plus
  `lint`/`test`); the batch builder runs it when `PLUGIN_TYPE=RUST`.

### plugins/wasm-cpp/ (C++ WASM plugins, Bazel)

- Bazel project: `WORKSPACE`, `BUILD`, `bazel/`, `common/`, `scripts/`,
  `extensions/<name>/` (e.g. `basic_auth`, `jwt_auth`, `key_rate_limit`,
  `model_router`, ...). Build via `plugins/wasm-cpp/Makefile`
  (`PLUGIN_NAME=<name> make build`), invoked with `PLUGIN_TYPE=CPP`.

### plugins/wasm-assemblyscript/ (AssemblyScript WASM plugins)

- Node/AssemblyScript project: `asconfig.json`, `package.json`, `assembly/`,
  `extensions/`.

### plugins/golang-filter/ (Envoy Go HTTP filter, NOT WASM)

- A native Envoy Golang HTTP filter (`main.go`, `mcp-server/`, `mcp-session/`);
  compiled as a shared object (`.so`) independent of Envoy — no Envoy rebuild
  needed. Requires Higress >= 2.1.0. Plugins register in `main.go`'s `init()`
  via `RegisterHttpFilterFactoryAndConfigParser`. See
  `plugins/golang-filter/README.md`.
- Build: `plugins/golang-filter/Makefile` (docker build, outputs
  `golang-filter_<arch>.so`). Wired into the gateway image build via
  `Makefile.core.mk` targets `build-golang-filter[-amd64|-arm64]`.

### How plugins are loaded

`WasmPlugin` CRDs (`extensions.higress.io/v1alpha1`) reference a plugin by
`url:` — either `oci://.../plugins/<name>:<version>` (image) or
`file:///opt/plugins/.../plugin.wasm` (local mount used in e2e). The dev install
`make install-dev-wasmplugin` sets Helm `global.volumeWasmPlugins=true` to mount
locally built wasm files into the gateway.

## Build & test

Run targets from the repo root; `Makefile` delegates to `Makefile.core.mk`.
Common ones:

- `make build` / `make build-linux` — build the Higress controller binary
  (`prebuild` first fetches submodules).
- `make build-hgctl` — build the `hgctl` CLI.
- `make build-gateway` / `make build-istio` / `make build-envoy` — data-plane
  and control-plane images (gateway pulls in the golang-filter).
- `make build-wasmplugins` — runs `tools/hack/build-wasm-plugins.sh` to batch
  build WASM plugins (respects `PLUGIN_TYPE` / `PLUGIN_NAME`; Go plugins require
  a `-alpha` VERSION).
- `make gen-api` / `make gen-client` — regenerate API/client code.

### Conformance / e2e tests (`test/e2e/`)

- Entrypoint `test/e2e/e2e_test.go`, run with build tag `conformance` and
  `--test-area` / `--execute-tests` flags.
- Cases live in `test/e2e/conformance/tests/` as **paired `<name>.go` +
  `<name>.yaml`** files (~68 cases; WASM cases are prefixed by language, e.g.
  `go-wasm-*`, `cpp-wasm-*`). Support code: `conformance/base/`,
  `conformance/utils/`, `conformance/embed.go`.
- Key Make targets (each spins up a kind cluster):
  - `make higress-conformance-test` — Ingress/Gateway conformance.
  - `make higress-wasmplugin-test` — WASM plugin e2e (uses
    `install-dev-wasmplugin`, which builds plugins and mounts them).
  - `*-prepare` / `*-skip-docker-build` / `*-clean` variants exist for
    iterating; `run-higress-e2e-test[-wasmplugin]` runs `go test` against an
    already-prepared cluster (filter with `TEST_SHORTNAME`).
- For the specifics of authoring a wasm-go e2e test, see the section below.

## Writing e2e conformance tests with wasm-go plugins

When adding an e2e conformance test that ships its own wasm-go plugin under
`plugins/wasm-go/extensions/<name>/`:

- The plugin's `VERSION` file **must end in `-alpha`** (e.g. `1.0.0-alpha`).
  CI's `tools/hack/build-wasm-plugins.sh` only compiles a wasm-go plugin when
  its version ends in `-alpha`; otherwise it silently skips it.
- `plugin.wasm` is a build artifact and is **not** committed. If the plugin
  isn't built, the `file:///opt/plugins/.../plugin.wasm` URL in the test's
  `WasmPlugin` manifest resolves to a missing file, envoy rejects the wasm
  config and fails closed, and every request on that route returns HTTP 500.
  Locally this can be masked because a previously built `plugin.wasm` still
  exists on disk — so a test can pass locally yet 500 in CI.
