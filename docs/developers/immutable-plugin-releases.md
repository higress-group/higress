# Immutable Go/Rust plugin release operation

This procedure applies only to official Go extensions in
`plugins/wasm-go/extensions/` and official Rust extensions in
`plugins/wasm-rust/extensions/`. C++, AssemblyScript, Go examples, Rust
examples, and explicitly unmanaged downstream defaults are not republished or
rewritten by this path.

## Required administrative cutover

Before a non-dry-run release, an administrator must configure all of the
following. The checked-in workflows deliberately fail closed until these
controls exist.

- Make every public plugin tag other than `latest` immutable in the registry.
  Retain candidate artifacts only in a separate content-addressed namespace
  with an expiry policy.
- Give the protected `plugin-release-production` environment the sole
  credential able to write `plugins/<plugin>:<VERSION>` and `:latest`; remove
  equivalent public-write access from legacy Wasm workflows and users.
- Give the protected `plugin-server-production` environment the sole
  credential able to write `higress/plugin-server:<gateway-version>`. The
  plugin-server repository publisher may write only its development/candidate
  namespace.
- Configure `PLUGIN_CANDIDATE_REGISTRY` and
  `CANDIDATE_REGISTRY_USERNAME` / `CANDIDATE_REGISTRY_PASSWORD` on the
  protected `plugin-release-candidate` environment. Bootstrap capture reuses
  this existing ACR login, but its command path only resolves public manifests
  and never pushes, retags, or deletes. The uncredentialed ORAS preflight stays
  read-only and outside any protected environment. If that preflight receives
  401/403, it records that authenticated capture is required and continues
  checking the remaining deterministic references; it does not claim absence
  or abort. The protected capture then makes the definitive present/missing
  classification: authorization/configuration failures remain fatal, and only
  explicit registry absence evidence is treated as missing.
- Create a scoped GitHub App for each downstream repository dispatch/PR
  receiver. It may update the deterministic dependency PR but may not merge,
  tag, or release.
- Protect `higress-release-manager` with human approval and a tag ruleset that
  permits only its release-manager App to create `vMAJOR.MINOR.PATCH` tags.
- Record negative checks for every retained credential: a conflicting public
  version tag and gateway-version image overwrite must be refused.

Environment protection, registry ACLs, lifecycle policy, tag rulesets, and
credential revocation are external administration. They cannot be asserted by
repository source alone.

## Preparation and promotion

1. At code freeze, record the exact 40-character commit at the current `main`
   head. Do not open or update the normal release metadata/Helm/submodule PR
   (for example, a PR like #4019) yet. `prepare-plugin-release` verifies that
   `target_ref` is exactly this main head; its generated branch therefore adds
   only plugin preparation changes when opened back against `main`.
2. Run `prepare-plugin-release` using that exact main/code-freeze commit, a
   stable gateway version, and `dry_run=true`. It validates the complete
   catalog and writes a deterministic plan. Version overrides are only
   proposed stable `VERSION` edits in the generated review branch; a publisher
   must never trust the dispatch payload as a public tag. For the first managed
   release only, set `bootstrap_comparison_base` to the exact 40-character
   commit of the last stable Higress release (`v2.2.3` is
   `39ec41aab6eb1d40499bed2847085696de0ebb96`). The workflow captures current
   public refs/digests read-only, then compares artifact inputs from that
   ancestor; ordinary snapshots reject this one-time override. The capture
   classifies every release-eligible entry as public, missing, or deferred;
   see "Bootstrap deferral and backfill" below.
3. Build/test every plan entry through the protected candidate publisher. Each
   candidate is keyed by plan ID/input hash and reports its manifest digest,
   source commit, and input hash. Render the snapshot only after every changed
   entry has matching evidence. A later source or `VERSION` edit invalidates
   that evidence.
4. Review and merge the one preparation PR containing `VERSION` edits and
   `plugins/release/snapshots/<gateway-version>.json`. The snapshot carries all
   release-eligible plugins, including unchanged entries carried forward by
   digest. It is the only downstream input.
5. Run `promote-plugin-release` for that exact merged commit and snapshot
   hash, first with `dry_run=true`; the dry run resolves the exact committed
   plan, previous snapshot, and every candidate manifest without logging in or
   mutating registry state. A production publisher may create a missing public version tag or
   accept an identical existing digest only. It must fail before mutation on a
   conflicting digest. Advance `latest` only after the full batch verifies and
   only when its stable SemVer cannot move backwards. An existing `latest`
   already serving the desired digest is accepted before reading legacy
   version annotations and is never rewritten.
6. Build `higress/plugin-server:<gateway-version>` from the exact approved
   plugin-server commit and snapshot. Its dry run checks out and tests that
   exact plugin-server source, binds the gateway version/path/plan/previous
   snapshot, and resolves candidate provenance before any image tooling or
   registry write is enabled. Verify image platforms, labels, response content
   hashes, and the snapshot hash before downstream dispatch.
7. Dispatch Console from the exact snapshot-carrying Higress commit, then merge
   its generated PR and wait for the Console chart containing that lock to be
   released. Only now create or update the normal #4019-style release PR from
   current `main`, limiting it to release metadata, Helm dependency, release
   notes, and intended submodule pins. Set the released Console dependency and
   run `helm dependency update helm/higress` so `Chart.lock` is regenerated;
   review both `Chart.yaml` and `Chart.lock`. Never manually edit managed plugin
   `VERSION` files there. If this release PR changes any catalog-declared plugin
   artifact input, stop and reprepare the snapshot/plugin-server/Console chain
   from the new exact main commit. Merge the release PR only after these exact
   dependencies and readiness checks converge, immediately before exact tag
   authorization. After the Higress release, dispatch Standalone with
   immutable release/chart/image identities. It derives a deterministic PR key
   and must not read a newer branch head.

## Bootstrap deferral and backfill

The first managed bootstrap compares artifact inputs from the exact
`bootstrap_comparison_base` commit and classifies every release-eligible
catalog entry through reviewed bootstrap evidence instead of failing on an
absent public tag:

- A `VERSION` whose prerelease begins with the `alpha` identifier (for
  example `1.0.0-alpha` or `1.0.0-alpha.1`) is deferred in every release. The
  alpha build never blocks the bootstrap, produces no candidate, receives no
  public stable tag, no `latest` movement, and no new snapshot entry; a
  plugin that already has a stable release simply carries that earlier entry
  forward. Other prerelease families are not deferred by this rule, and a
  non-alpha prerelease whose public artifact is absent fails closed.
- A stable `VERSION` whose public tag already exists is imported
  idempotently: the capture resolves its digest with the read-only credential
  and the snapshot carries the exact historical public reference.
- A stable `VERSION` whose public tag is genuinely absent becomes a backfill
  entry. Absence is recognized only on explicit registry 404-class evidence
  (`404`, `manifest unknown`, `name unknown`) or a provider-structured registry
  error that contains both `Error response from registry:` and the exact fully
  qualified reference followed by `: not found`; an authorization failure, a
  local executable or file failure, an unrelated reference, or generic "not
  found" text aborts the capture and is never absence evidence, and a stale
  `missing` claim whose tag resolves at preparation time is rejected. The exact target commit is
  built once as a content-addressed candidate; the reviewed snapshot binds
  its digest, source commit, and input hash. Promotion creates the public tag
  from that candidate when absent, accepts an identical existing digest,
  fails on any conflict, and never rebuilds. The `backfill` flag is
  bootstrap-only provenance and migration state: plan and snapshot must
  record it exactly alike, and after the complete version batch verifies the
  entry joins the same serialized monotonic `latest` policy as every selected
  stable plugin (create when absent, accept identical, advance an older
  reliably annotated stable version, fail closed otherwise).

The first managed snapshot therefore mixes provenance: historical public
entries plus candidate backfill entries. It also carries an explicit
`bootstrapEvidence` marker naming the deterministic committed evidence file
`plugins/release/bootstrap-evidence/<gateway-version>.json`; validation
recomputes that committed file's digest against the marker and binds every
imported public digest, backfill claim, and deferred classification to it,
never inferring bootstrap mode from a missing previous-snapshot or temporary
baseline file. Preparation PR validation resolves the candidate references;
post-promotion verification resolves the public tags. Historical public
artifacts carry no invented provenance annotations. A new catalog plugin in a
later managed release follows the same build-once/promote-once path, but it
is a genuine new release rather than imported history, so it is not marked
`backfill` and `latest` advances normally for it.

The pre-tag `authorize-higress-release-tag` workflow rechecks the exact merged
release commit and promoted snapshot. It derives the identical snapshot-source
Higress commit from every plugin-server platform label, requires it to be an
ancestor of the release commit, and proves snapshot bytes and committed plugin
inputs at both commits are unchanged. It also resolves the released Console
chart through the HTTPS repository declared identically by `Chart.yaml` and
`Chart.lock`, then requires those package bytes to equal the unique Helm content
layer under the provenance-pinned OCI manifest digest. Human approval and the
tag ruleset are mandatory; ordinary tag-triggered workflows are post-tag
publishers and cannot satisfy this gate.

## Canonical release evidence and downstream order

Release-critical dispatches use canonical JSON (`jq -cS`) and a SHA-256 key of
the canonical bytes. The Higress `release.published` sender resolves its own
release ID/tag/peeled commit, snapshot hash, Console release ID/tag/commit and
chart digest, and plugin-server ref/digest before it sends that evidence to
Standalone. Standalone re-resolves each identity and records the exact
Standalone base SHA in `release-provenance.json`; retries reuse only a branch
whose provenance bytes match. It never rebases onto a newer `main`.

The required configuration is `PLUGIN_SERVER_REGISTRY`, protected environment
registry credentials, and the scoped App variables/secrets named by the
workflows. The Console chart must be published before tag authorization. The
operator supplies its immutable GitHub release ID plus the pinned plugin-server
source/digest; readiness re-resolves the release, provenance asset, both chart
distribution paths, plugin-server index, and child labels and emits canonical
release-evidence JSON. The dry-run artifact/hash is reviewed first. Formal
authorization requires that approved SHA-256 and refuses if re-resolved
canonical bytes differ. Neither path accepts hand-assembled evidence. A
Standalone `release.published` event carries
the release ID, dereferenced tag commit, archive and installer checksums and a
canonical key back to Higress OSS packaging. Repository-dispatch is production
by design; only the manual workflow path honours `dry_run=true`.

## Recovery and rollback

Retries use the same immutable identifiers. A retry may accept an existing
tag only when it resolves to the expected digest. Never overwrite a version
tag. Recover by creating a new reviewed snapshot/release that refers to an
earlier verified digest set. If `latest` needs repair, use the serialized
promotion flow with an audited recovery approval; released Console, gateway,
and Standalone defaults remain pinned and do not depend on `latest`.

## Exact Standalone packaging

`deploy-standalone-to-oss` is manual and requires one verified Standalone
`vX.Y.Z` tag plus SHA-256 values for both its archive and
`src/get-higress.sh`. It downloads both from that exact tag and fails for a
missing tag or checksum mismatch. It never selects a release from HTML or
downloads an installer from `main`.

## Incident record: 2026-08-13 v2.2.4 broken Wasm OCI layouts

Incident [#4528](https://github.com/higress-group/higress/issues/4528);
remediation proposal
[#4529](https://github.com/higress-group/higress/issues/4529).

Timeline:

- 2026-08-13: the first release published by the immutable plugin release
  pipeline (PR
  [#4449](https://github.com/higress-group/higress/pull/4449),
  promote runs
  [31740993111](https://github.com/higress-group/higress/actions/runs/31740993111)
  and
  [31746736542](https://github.com/higress-group/higress/actions/runs/31746736542))
  published each plugin candidate as a single-layer image whose only layer
  had media type `application/vnd.module.wasm.content.layer.v1+wasm`, then
  copied those manifests verbatim with `oras cp` to public version tags and
  `latest`.
- 2026-08-13 21:56: gateways began failing to fetch plugin modules with
  `the given image is in invalid format as an OCI image: could not parse as
  compat variant ... could not parse as oci variant: number of layers must
  be 2 but got 1`, breaking 43 plugin repositories (43 `latest` tags and 36
  version tags).
- 2026-08-13/14: all affected `latest` and version tags were restored with
  the legacy pipeline from the pre-#4449 commit `b97225f7`, whose
  `oras push` layout (optional spec/doc layers, final
  `application/vnd.oci.image.layer.v1.tar+gzip` layer) is loadable by every
  gateway version.
- 2026-08-15: registry-side finding recorded on #4529: re-enabling
  repo-level tag overwrite in ACR did not unblock the three remaining
  broken `1.0.0` tags; re-pushes still failed with 409 tag conflict and the
  release credential could not delete manifests (401). Their `latest`
  aliases were already restored.
- 2026-08-15 (later the same day): the three remaining tags were repaired
  through console-side tag deletion plus a compat-layout re-push; live
  public-pull verification (below) now resolves all of them to
  Envoy-loadable compat manifests.

Root-cause chain: (1) `prepare-plugin-release.yaml` pushed candidates in a
wasm-pkg single-layer layout; (2) promotion copies candidates verbatim, so
the public tags inherited that layout; (3) no machine check anywhere —
candidate publish, snapshot verification, or promotion — compared the
manifest layer composition against the layouts Envoy actually accepts
(the 2-layer OCI variant, or the compat variant whose final layer is
`application/vnd.oci.image.layer.v1.tar+gzip`).

Detection gap: `verifyOCI` validated only the resolved digest and the
provenance annotations, so a structurally unloadable manifest passed every
gate; the first failure appeared at gateway fetch time in production.

Gates added by #4529:

- `tools/plugin-release` now has a single Envoy layout predicate
  (`envoyWasmLayout`, exposed as `verify-oci-layout`) that accepts exactly
  the two loadable layouts and rejects anything else naming the layer media
  types. The compat rule requires the final layer to be tar+gzip, which is
  the strictest common denominator of the runtime resolvers; a manifest
  that passes the gate is loadable by every gateway version in the wild.
- Preparation publishes candidates only in the 2-layer OCI variant
  (deterministic `{}` wasm-config layer, then the raw wasm layer), keeps the
  `org.opencontainers.image.revision`, `org.opencontainers.image.version`,
  and `io.higress.plugin.input-hash` annotations, and re-resolves the pushed
  manifest by digest to assert the layout before the digest becomes
  evidence. An existing candidate tag that does not resolve to that exact
  layout fails closed instead of being reused.
- `verify-snapshot --resolve` rejects a candidate-provenance manifest in any
  non-loadable layout. Historical public imports keep digest-only
  verification: they predate this pipeline and include documented
  docker-format tags, which are explicitly out of scope.
- Promotion gates every candidate-provenance public ref with the same
  predicate after copy, and again in the latest preflight before any
  `latest` alias moves. A "gateway layout smoke gate" step runs after the
  version batch completes and strictly before the latest phase: because CI
  runners cannot instantiate the full gateway integration stack, it is a
  layout-equivalence check against the exact public ref using the same
  parser rules as `verifyOCI`, and it records a `layoutGate` verdict per
  entry in the version journal plus a completeness assertion that the gate
  ran and passed for every candidate-provenance entry. This predicate-based
  check is the smoke test; it does not execute a real gateway.

## Repair runbook: blocked immutable tags and recovery digests

`ai-context-limit:1.0.0`, `gw-error-format:1.0.0`, and
`nginx-rewrite-compatible:1.0.0` were the last tags still serving the broken
single-layer manifests, because their immutability is tag-scoped on this ACR
instance: tag overwrite is refused (409) and the release credential cannot
delete manifests (401). The audited procedure below is how they — and,
during the main recovery, the other 43 `latest` and 36 version tags — were
repaired. Keep it for the next blocked tag.

Live public-pull evidence (registry
`higress-registry.cn-hangzhou.cr.aliyuncs.com`, anonymous pull; digests
captured 2026-08-15 ~08:51 UTC, after the final repair wave):

| Repository | Broken digest (pre-repair, 2026-08-13) | Restored digest (current, compat layout) |
| --- | --- | --- |
| `plugins/ai-context-limit:1.0.0` | `sha256:6325ca94b20a3f5ab5e9df4014b60daca8d86e7d0999e74abb2c1ddd29b36f83` | `sha256:d212fbb3b498f45b7e18eba461d4aa7fc3a12e50fb710270a9c3156491e4c63a` |
| `plugins/gw-error-format:1.0.0` | `sha256:a0b80b448e995cc3c5697397167e81d3d557205f8ec00e44b16b00997c4082e9` | `sha256:1025d9e051c0fb5d1fb6e358b11e853151b882bf1337817e23736317d0187d91` |
| `plugins/nginx-rewrite-compatible:1.0.0` | `sha256:9ab80c0c7e9abd30e099ef92964efc0291f1161175ea2a6873019d1d47a06922` | `sha256:61a5684328800e4d07055cb3cef09614fa2436032b2a2ae1f6a8a04119ca9226` |

Audited recovery procedure for a blocked tag:

1. Record the pre-repair state (read-only; anonymous pull is sufficient):
   `oras manifest fetch higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/<name>:<tag> --descriptor | jq -r .digest`
2. Delete the broken tag through the ACR console (or a delete-capable
   credential). Tag overwrite remains refused (409) and the release
   credential cannot delete (401), so this step is an audited registry-side
   action with maintainer approval.
3. Re-push the restored compat-layout manifest from the legacy pipeline at
   the pre-#4449 ref `b97225f7`, then verify both layout and digest:
   `cd tools/plugin-release && go run . verify-oci-layout --ref higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/<name>:<tag>`
   must print `compat`.
4. Refresh the checked-in evidence to the restored digests. The Console
   recovery manifest is validated against the snapshot, so both files must
   move together: update the affected `digest` values in
   `plugins/release/snapshots/2.2.4.json`, recompute `snapshotSha256` with
   `sha256sum plugins/release/snapshots/2.2.4.json`, and update the same
   `digest` values plus `snapshotSha256` in
   `plugins/release/console-recovery/2.2.4.json`. Never guess digests; take
   them from step 3 or a fresh public pull.
5. Prove the refreshed manifest still binds:
   `cd tools/plugin-release && go run . validate-console-recovery --root ../.. --catalog ../../plugins/release/catalog.json --manifest ../../plugins/release/console-recovery/2.2.4.json`.
6. Commit the refreshed snapshot/recovery bytes through a reviewed PR that
   links this runbook, the deletion record, and the digest table.

The v2.2.4 recovery refresh executed with this change: every one of the
eight Console-recovery plugin tags had been restored to a compat-layout
manifest, so the eight `digest` values plus the rebound `snapshotSha256`
were refreshed in `plugins/release/snapshots/2.2.4.json` and
`plugins/release/console-recovery/2.2.4.json` from live public pulls, and
`validate-console-recovery` passes on the refreshed bytes. The remaining
2.2.4 snapshot entries whose version tags were also restored during the
recovery were deliberately left untouched here: refreshing them is a
separate reviewed decision (the next managed release's snapshot re-baselines
them naturally), because the Console recovery validator binds only the
eight refreshed entries.
