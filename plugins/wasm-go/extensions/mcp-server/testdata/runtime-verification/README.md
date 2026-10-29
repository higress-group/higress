# MCP compatibility runtime verification

The default `run.sh` entry delegates to `run-compat.sh`. This loop builds a clean
exact candidate and two pinned sources, uses the existing Compose backend,
Envoy renderer, bounded lifecycle helpers and evidence finalizer, then runs the
existing routing regression runner. It does not certify native, official-client
or kind acceptance; those checks remain separate verification commands.

From `plugins/wasm-go/extensions/mcp-server`:

```bash
env -u GH_TOKEN -u GITHUB_TOKEN \
  COMPAT_PYTHON=/opt/homebrew/bin/python3.12 \
  bash testdata/runtime-verification/run-compat.sh \
  --candidate HEAD \
  --output /Users/<user>/mcp-compat-evidence
```

The output directory must be empty and outside the checkout. Python 3.12 or
newer is required for safe archive extraction. On other hosts select an
appropriate `COMPAT_PYTHON`; the default is `python3`. Go must support the
source modules' pinned toolchain. `podman compose` and cached or available
container images are required. The default gateway image is
`higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:v2.2.3`; the backend
and verifier use `docker.io/library/python:3.12-alpine`. The runner resolves and
records immutable digests before using the same images for every source.

Arguments are `--candidate`, `--output`, `--oracle`, `--control`,
`--gateway-image`, `--backend-image` and development-only `--skip-routing`.
`RUNTIME_EVIDENCE` can provide `--output`. The oracle/control arguments must
match the approved pins. A dirty tree or a candidate other than checkout HEAD
is rejected. `--skip-routing` deliberately cannot produce a passing manifest.
Every subprocess inherits an environment with `GH_TOKEN` and `GITHUB_TOKEN`
unset. The runner only removes resources bearing its generated Compose project
name; it neither reads provider credentials nor operates on a Kubernetes cluster.

## Source roles

| Source | Exact revision | Role |
| --- | --- | --- |
| Business oracle | `958b5e7f8e0062d1e26963d008e7be4b707fa715` | Corresponding historical Legacy business path |
| Control/scope baseline | `bde990b4bfc7ecb581338e2d0ae858aaedb2fd69` | Retained probe/handshake/cancellation and instructions scope |
| Candidate | Clean checkout HEAD | Independent acceptance assertions and corresponding business comparison |

The control binary also runs five explicitly labelled diagnostic red cases:
Modern full body above 1 MiB, schema-gated call, `legacyOnly: true`, cross-Origin
request and fractional client ID. These reproduce the removed gates/defects;
they are not normative business expectations. The candidate's matching positive
cases are independently enumerated in `compat_contract.registry`.

Each source archive builds a plain Wasm. The **same unchanged bytes** of
`registered_fixture.go.txt` are appended to a second build for every source.
The fixture supplies serializable RawMessage/custom-Marshaler/outputSchema
values, long names, raw Create length/hash results and execution counters.
Plain REST and proxy listeners do not load it. Source, fixture, module/toolchain,
harness and plain/registered Wasm hashes are recorded separately. Oracle
production files are never edited. The actual e2bdev and firecrawl YAMLs are
exported with their descriptors/arguments intact; only test server identity,
external request target/method and vendor response templates are adapted to the
observable local backend. Their source and derived fixture hashes and exact
adaptations are recorded in `builtin-manifest.json`.

## Independent cases and observability

`compat_contract.py` enumerates unique case IDs and expected per-case downstream
exchange counts before traffic runs. Missing/duplicate expected or observed IDs,
client exchanges or access records fail finalization. `verify_compat.py` writes
a snapshot after each case, so an interrupted prefix cannot appear complete.

The sequential body cases use full UTF-8 JSON-RPC bodies above 1 MiB, 10 MiB,
100 MiB minus one byte, exactly 100 MiB with metadata padding, and 100 MiB plus
one byte with metadata padding. Envelope and `_meta` bytes count toward the
limit. The tool returns only raw argument length/SHA-256; evidence stores body
lengths/hashes rather than large payloads. Depth 70, 65537 values, long ASCII and
encoded UTF-8 names, ordinary duplicates, invalid/concatenated/batch messages,
schema mismatch and `allowTools` denial have separate IDs. Large requests and
source variants execute sequentially; elapsed times are diagnostic, with no
performance SLO.

Before/after direct-tool counters include a fixture-only VM generation and a
positive Create/Call increment. A VM recycle between observations fails the
zero-effect proof instead of being mistaken for unchanged zero counters. Denied
and invalid candidate requests must not execute Create/Call. The old oracle did
not enforce JSON framing; malformed, concatenated and batch results are observed
and classified only as the named retained basic-message-boundary exception.
Candidate 400/error/zero-side-effect assertions remain mandatory. No other
business differences receive that exception.

The backend ledger separately records transport GET, probe, control and business
attempts, request acceptance, exact ID token, typed parameter/argument hashes,
Authority/path/query recipient, backend origin and synthetic credential match
booleans. It never includes real credentials. A strict peer rejects duplicates
throughout the JSON tree, duplicate protocol/media headers, invalid method/name/
version/media, unsupported capability claims and IDs unacceptable to a strict
upstream. It understands canonical `=?base64?...?=` names. Ordinary repeated
headers remain legal. Numeric observations use `typed_canonical.JSONNumber`,
never a float-based comparison.

Joint strict-upstream and local-error cases cover string, both sides of 2^53, overflow integer,
decimal, exponent, `-0`, explicit null and missing Legacy IDs on tools/list and
tools/call success/error paths. Candidate client tokens and presence/default
semantics are asserted independently. The oracle's named numeric/null ID and
opaque result-number defects are recorded and excluded only at those fields.
Modern ordinary notifications require HTTP 202 with no JSON-RPC body; the
specific HTTP 401/403 authentication exception retains status and
`WWW-Authenticate` while still suppressing the body.

HTTP and SSE business cases cover ordinary/custom/missing media, results above the probe limit, duplicate/deep
results, RPC errors and HTTP failures. They use corresponding historical error,
status, data and header policies. Authentication fidelity and preserved SSE
ordinary-header multiplicity are named later fixes and independently asserted.
Probe/body budgets, control identity/correlation, terminal failures and zero
business attempts remain separate. `verify_auto.cases` supplies the existing
control, concurrency, cancellation, no-replay, route-deadline and phase counters;
its complete independently generated case registry is checked for both control
and candidate binaries.

SSE cases retain the initial main GET and existing control/business POST stages.
They cover relative, absolute, cross-Authority and externally named endpoints,
full/path-only configuration, configured Host rewrite, original custom route
cluster, ordinary headers, query/header/passthrough/removed credentials, and
GET/initialize/initialized 401/403 with and without downstream IDs. The original
route targets the same actual backend even when Authority changes; endpoint DNS
is not used directly. Path-only empty-Authority/unknownhost failures are observed
against the oracle instead of inventing a new resolution rule. This does not
claim a fixed TLS identity. Native mocks coalesce added GET headers; actual
network GET header and Host rewrite evidence belongs to runtime verification.

The existing instructions registry adds omitted/false local behavior, opt-in
single Modern HTTP discovery, success/error ID restoration, field isolation,
32768/32769 decoded UTF-8 bytes, auth, downstream filters, retries/deadlines,
concurrent isolation and disconnection. Its 2 baseline/2 exchange and 29
candidate/33 exchange registries and matching access records remain unchanged.
The new core also checks explicit discovery IDs against the strict peer.

## Evidence and failure handling

- `compat-inputs.json`: exact source pins, clean-tree/token attestations, image
  inspections/digests, Go/module identities, separate fixture/harness/Wasm hashes,
  verifier exit state and project-owned cleanup result.
- `compat-{oracle,control,candidate}.json`: independent `expectedCases`, exactly
  one result per case, bounded request/response summaries and unique client
  exchange IDs. Case detail includes independent assertions, comparable business
  projections and phase-separated backend ledgers.
- `controls-{control,candidate}.json` and `instructions-{baseline,candidate}.json`:
  reused mechanism/scope results and exchange ledgers. Gateway logs retain all
  access records; finalization selects each explicitly named registry's records.
- `routing/manifest.json`: existing route/header/auth/log/deadline regression
  evidence, including the historical HTTP 200 business-timeout mapping for auto
  business while preserving strict control 504 behavior.
- `commands.jsonl`, build/module/tool logs, resolved Compose/Envoy configurations
  and gateway logs: reproducibility and diagnostics. Configs use synthetic keys.
- `compat-manifest.json`: A1–A12 coverage, classified exceptions, unclassified
  differences, failure reasons and final checksums. It cannot pass with a red,
  partial, duplicated, stale-source, missing-hash or uncleaned run. The finalizer
  command row and `finalize.log` are written before checksums are sealed, avoiding
  a self-recording hash mismatch. `nativeAndKindAcceptanceClaimed` stays false.

The loop stops/removes each variant gateway using existing bounded lifecycle
helpers and verifies no project containers remain. Failed runs retain the first
case failure, command exit results and available backend/access diagnostics.
Generated Wasm, evidence and temporary sources are not committed.

## Facility self-tests and other entries

CI's existing orchestration self-test invokes `compat_self_test.py`. It covers
strict peer negative controls (including nested opaque fields and repeated
headers), encoded names, exact numeric tokens, UTF-8 body accounting, registry
omissions/duplicates, real backend rejection/ledger recording, fixed-cluster
fixtures, incomplete evidence, Wasm hashes and post-finalizer checksum mutation.
The existing checks remain executable:

```bash
env -u GH_TOKEN -u GITHUB_TOKEN python3 orchestration_self_test.py
env -u GH_TOKEN -u GITHUB_TOKEN python3 auto_self_test.py
env -u GH_TOKEN -u GITHUB_TOKEN python3 instructions_self_test.py
env -u GH_TOKEN -u GITHUB_TOKEN python3 routing_self_test.py
env -u GH_TOKEN -u GITHUB_TOKEN python3 compose_config_self_test.py /path/to/resolved-compose.json
env -u GH_TOKEN -u GITHUB_TOKEN bash lifecycle_self_test.sh
```

Run these from this directory. `descriptor_self_test.py prepare|cleanup DIR`
continues to check typed descriptor hashes against generated corpus fixtures.
The default compatibility loop uses `run_routing.py --oracle <958b pin>
--skip-affected --candidate <exact HEAD> --output DIR`; standalone
`run-routing.sh` retains its explicit affected-revision diagnostic mode.

For separate legacy instructions investigation, `RUNTIME_VERIFY_INSTRUCTIONS=1
INSTRUCTIONS_BASELINE_SHA=<revision> bash run.sh` retains its existing focused
entry. `RUNTIME_HISTORICAL_MATRIX=1 bash run.sh` exposes the older schema/corpus
investigation only. Its v2.0.0 oracle `39ec...` and old schema-gate assertions are
not the approved business oracle or current acceptance matrix.

The official kind entry remains `go test -tags conformance
./test/e2e/e2e_test.go`. Test-only `--gateway-address` populates
`Suite.GatewayAddress` and defaults to `localhost`; a high-port forward can use
`--gateway-address=127.0.0.1:18080`. This adds no production configuration.
Use existing `WasmPluginsMCP20260728` and
`GoWasmMcpProxyRoutingCompatibility` cases through the entry's `--execute-tests`
filter. Keep kubeconfig credentials outside evidence uploads.
