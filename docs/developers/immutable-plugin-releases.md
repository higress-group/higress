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
- Scope the credential able to write `plugins/<plugin>:<VERSION>` and `:latest`
  as narrowly as GitHub still allows, and remove equivalent public-write access
  from legacy Wasm workflows and users. Since SPEC-4634005 only the routine
  latest-move job keeps the protected `plugin-release-production` environment,
  both the version-tag phase and the self-authorizing emergency workflow read
  that credential without an environment gate. It must therefore be a
  repository-scoped secret, which means every workflow in this repository can
  read it. Review that exposure before each release, keep the credential
  limited to the plugin namespaces, and never reference it from a
  pull-request-triggered workflow.
- Give the protected `plugin-server-production` environment the sole
  credential able to write `higress/plugin-server:<gateway-version>`. The
  plugin-server repository publisher may write only its development/candidate
  namespace.
- Configure `PLUGIN_CANDIDATE_REGISTRY` and
  `CANDIDATE_REGISTRY_USERNAME` / `CANDIDATE_REGISTRY_PASSWORD` as
  repository-scoped secrets/variables. The preparation job no longer uses the
  `plugin-release-candidate` environment: candidate builds publish only into the
  content-addressed `candidates/` namespace, which nothing public resolves, so
  that phase requests no approval. Bootstrap capture reuses
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
  tag, or release. Configure the emergency evidence publisher as
  `RELEASE_PR_APP_ID` and `RELEASE_PR_APP_PRIVATE_KEY`; its installation on
  `higress-group/higress` needs repository contents and pull-request write
  permission, but no merge, tag, release, or registry permission.
- Grant the release preparation App label write permission in addition to its
  contents and pull-request permissions. It applies `release/<gateway-version>`
  to the preparation PR it opens, and promote refuses to run without that label
  on the merged PR; a missing label permission fails the preparation run rather
  than publishing an unauthorized PR.
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
   source commit, and input hash. Candidate and emergency publication execute
   the same fail-closed manifest contract: exactly two ordered OCI layers,
   `config.json` as `application/vnd.module.wasm.config.v1+json` followed by
   `plugin.wasm` as
   `application/vnd.module.wasm.content.layer.v1+wasm`, with OCI image-spec v1.0,
   `created` fixed to the source commit time, and identical revision, version,
   and input-hash annotations. Reuse first performs the normal deterministic
   source build, then resolves and pulls the digest-pinned candidate, requires
   its config bytes to be exactly `{}`, compares its Wasm bytes with that build,
   and applies the same strict manifest and Proxy-Wasm validation as promotion;
   metadata alone is never sufficient. Publication binds the layer descriptors
   to the local bytes and returns only a validated SHA-256 manifest digest.
   Before the snapshot is rendered,
   the workflow sweeps the public registry for every planned version tag and
   compares each occupied tag with the candidate digest just built; see
   "Onboarding migration preflight" below. Render the snapshot only after every
   changed entry has matching
   evidence. A later source or `VERSION` edit invalidates that evidence.
4. Review and merge the one preparation PR containing `VERSION` edits and
   `plugins/release/snapshots/<gateway-version>.json`. The snapshot carries all
   release-eligible plugins, including unchanged entries carried forward by
   digest. It is the only downstream input. Merging it is one of the two human
   gates in a routine release: the preparation App labels the PR
   `release/<gateway-version>`, and that label on the merged PR is what
   authorizes the version-tag phase of promotion. A PR whose migration report
   excludes plugins is still mergeable; the excluded plugins are simply not
   published by this release.
5. Run `promote-plugin-release` for that exact merged commit and snapshot
   hash, first with `dry_run=true`; the dry run resolves the exact committed
   plan, previous snapshot, and every candidate manifest without logging in or
   mutating registry state. The version-tag phase requests no environment
   approval: it fails closed unless `source_commit` descends from the
   code-freeze commit the snapshot records, is reachable from `main`, and
   belongs to exactly one merged preparation PR from
   `release/plugin-snapshot-<gateway-version>` carrying the
   `release/<gateway-version>` label. A production publisher may create a
   missing public version tag or accept an identical existing digest only. It
   must fail before mutation on a conflicting digest, and it skips every entry
   the migration preflight excluded. Before any `latest` write, it fetches and
   pulls every non-blocked public artifact remotely by its snapshot digest. The
   local gate verifies the OCI schema and two-layer order, provenance,
   descriptor sizes and digests, canonical empty JSON config, and complete Wasm
   validity. It also requires exported memory, exact `(i32,i32)->i32`
   signatures for `proxy_on_vm_start` and `proxy_on_configure`, and a
   `proxy_abi_version_0_2_*` function with signature `()->()`. All pulls finish
   before the first
   mutable copy. A failed plugin is journaled and skipped while passing plugins
   are processed; the persisted journal is marked `latest-partial`, the job
   then fails, and no completion marker is published. Advance `latest` only
   when its stable SemVer cannot move backwards; this move is the second human
   gate and still waits for the protected `plugin-release-production`
   environment approval. An existing `latest` already serving the desired
   digest is accepted before reading legacy version annotations and is never
   rewritten.
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

### What the pull gate proves today

The remotely pulled gate has exactly two levels, and both are static with
respect to plugin behavior:

- **Structural.** The pulled manifest must be a canonical OCI v1.0 schema 2
  image manifest in the deterministic two-layer form Envoy loads: the
  empty-JSON config descriptor, exactly four provenance annotations (`created`,
  `revision`, `version`, `io.higress.plugin.input-hash`), layer 0 `config.json`
  with media type `application/vnd.module.wasm.config.v1+json`, and layer 1
  `plugin.wasm` with media type
  `application/vnd.module.wasm.content.layer.v1+wasm`. Every descriptor size and
  digest must equal the pulled bytes, and `config.json` must be the canonical
  empty JSON object. This is the defect class behind the one-layer `latest`
  incident this change responds to: such a manifest is not loadable by Envoy,
  and the gate rejects it.
- **Compile level.** The pulled `plugin.wasm` bytes are compiled with wazero
  (`CompileModule`, interpreter backend) and the compiled module's exports are
  inspected: exported memory, exact `(i32,i32)->i32` signatures for
  `proxy_on_vm_start` and `proxy_on_configure`, and a `proxy_abi_version_0_2_*`
  export with signature `()->()`.

The module is compiled but never instantiated, so no plugin code executes. The
gate proves that the registry serves a well-formed, digest-consistent,
compilable Proxy-Wasm module; it does not prove that the plugin initializes
inside Envoy or serves a request. That runtime coverage remains where it already
is: each plugin's own `go test ./...` inside the build contract, and the
pre-release e2e suites — `make higress-wasmplugin-test` installs the built
plugins into a cluster and drives the WASM conformance cases under
`test/e2e/conformance/tests/`. The hosted release runner cannot instantiate a
generic Envoy/Proxy-Wasm host for every plugin inside the release job, and the
public pull gate makes no runtime compatibility claim.

## Runtime smoke gate (future)

Placeholder for the runtime half of the pull gate, deliberately not implemented
by this change. The design it would follow:

- A minimal, digest-pinned Higress/Envoy runner image — no cluster, no Helm, no
  kind. It is `envoy` plus a static bootstrap that loads exactly one plugin from
  `oci://<registry>/<image>@<digest>`, the digest the release just published.
- The runner boots the plugin through the real Proxy-Wasm host, so
  `proxy_on_vm_start` and `proxy_on_configure` execute against the plugin's
  configuration, and asserts a host log without plugin errors plus a live admin
  endpoint.
- One request-path smoke assertion: a single listener and route with the plugin
  attached, one request, and a check that a response is produced instead of the
  `500` Envoy returns for a rejected Wasm configuration.
- Wiring: the position today's pull gate already holds — before any `latest`
  write in promote, and before the stable-tag copy in the emergency channel —
  with per-plugin granularity so one failing plugin is journaled and skipped
  while the rest proceed, and the phase fails before any mutable alias moves.
- Preconditions to settle first: a published, versioned runner image with its own
  digest pin and provenance; a decision on how plugin-specific configuration is
  supplied for plugins that reject an empty config; and the runner's cost inside
  the release job budget. Until those exist, the compile and structural gate
  above is the release-blocking check and the e2e suites carry runtime coverage.

## Onboarding migration preflight

A plugin that the pipeline starts managing can already have a manually published
artifact at the exact version tag its plan selects. Promotion then hit an
immutable tag conflict and a maintainer had to delete that tag in the registry by
hand, one plugin at a time (three times during 2.2.5). Prepare now classifies
every planned tag before the preparation PR is opened.

- The sweep resolves each planned `plugins/<plugin>:<VERSION>` reference
  read-only through the same registry path the rest of the pipeline uses. It
  calibrates first: a known-good control tag from the reviewed previous snapshot
  must resolve to exactly its recorded digest. If it does not, no classification
  from that sweep is trusted and the run fails. An authorization failure, or any
  lookup that is not explicit registry absence evidence, also fails the run and
  is never treated as an absent tag.
- A planned tag that is absent, or that already serves exactly the candidate
  digest, is clear. With no conflicts the release is byte-identical to the
  pre-preflight behaviour: no snapshot marker, no PR body section.
- A tag occupied by a different digest puts that one plugin in migration mode.
  The snapshot records `migration.state=blocked` on its entry together with the
  existing digest, the planned digest, the occupant's annotations, a source
  comparison, and exactly one recommended disposition:
  - `delete-legacy` — the occupant carries no pipeline annotation at all, so it
    is presumed a pre-pipeline manual artifact. This is a presumption, not
    proof of origin: the disposition is conditional on a human confirming the
    occupant's provenance (e.g. matching build history or maintainer records)
    before a registry administrator deletes that tag. After deletion, a later
    release plans and promotes the plugin normally.
  - `bump-version` — a different managed build already occupies this exact
    version. Re-run prepare with `version_overrides` for that plugin set to a
    reviewed stable version greater than the occupied one.
  - `adopt-public` — the occupant was built from the same source commit and the
    same input hash as this plan, so the published tag already serves these
    inputs and may be left alone. Candidate and emergency publication now share
    byte-identical manifest construction, so matching inputs produce the same
    digest and do not create a synthetic migration conflict.
  Deleting a tag is only ever recommended for an unannotated occupant. An
  artifact the pipeline cannot classify is never deleted automatically; the
  disposition is re-derived from the recorded annotations during snapshot
  rendering and again during verification, so a hand-edited recommendation does
  not survive validation.
- The exclusion is per plugin. Every other planned plugin promotes normally, the
  excluded entry keeps its candidate and snapshot record, and both promotion
  journals document the exclusion: `preflight: migration-excluded` in the version
  batch and `migrationExcluded` in the latest batch. The excluded plugin's
  version tag and `latest` alias are left exactly as they are. A batch in which
  every plugin is excluded fails closed instead of publishing an empty marker.
- The exclusion is durable. Later snapshots carry the marked entry forward
  verbatim, so it stays out of every batch until a release re-plans that plugin
  after its disposition completes. Verification tolerates the divergent public
  tag the marker records and never resolves it; candidate resolution for the same
  entry is unchanged.
- The report is embedded in the preparation PR body and uploaded as the
  `plugin-release-migration-report-<gateway-version>` artifact. A dry run builds
  no candidates, so it lists occupied planned tags as suspected conflicts without
  excluding anything; the non-dry-run sweep classifies them. A suspected report
  can never be bound into a snapshot.

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

### Emergency same-version overwrite

- An ACR admin temporarily lifts the tag-immutability rule for the one plugin
  repository. A maintainer dispatches `emergency-overwrite-plugin-tag` with the
  required stable `gateway_version`, logical ID, existing stable plugin
  version, and exact merged source commit, first with `dry_run=true` and then
  with `dry_run=false`. The workflow has no environment gate: it queries the
  collaborators API for `github.triggering_actor` (including the actor who
  initiates a rerun), records `role_name` in the step summary, and proceeds only
  for `maintain` or `admin`; malformed actors, legacy `write` role names, API
  failures, and malformed responses deny access. The publishing job repeats that
  identical check as its first step, before App-token creation, checkout, and
  registry login, because "Re-run failed jobs" resumes the publishing job without
  running `authorize` again and the triggering actor of a resumed run is whoever
  clicked re-run. The admin re-arms immutability immediately after the run
  reports the new digest.
- Before any registry login or write, the workflow binds the selected catalog
  entry, candidate evidence, and release snapshot to that gateway/plugin/
  version/public-reference tuple. The catalog-derived registry and repository
  are passed into publication, and the configured production registry must
  equal that registry. It builds the release tool from current `main` before
  checking out the older fix commit, then computes the current input hash,
  tests and builds that exact source. The artifact passed to the publishing job
  contains both that current tool and the built Wasm.
- Runs are serialized by logical ID and version. Publication first writes the
  desired deterministic two-layer manifest to an emergency candidate tag keyed
  by the source commit and input hash. Before changing the stable tag, the
  workflow re-pulls that staged candidate from the public registry by digest and
  runs the same `verify-pulled-plugin` gate promote applies before a `latest`
  move, so the incident channel cannot publish an artifact Envoy will not load.
  The gate inspects what the registry serves, never the local build output, and
  a failure aborts before any stable-tag mutation. The workflow then stages and
  validates the lineage update and resolves the committed predecessor (the last
  lineage digest, or the original candidate digest when no lineage exists). The
  stable tag must exist and serve that predecessor; the workflow then copies the
  staged digest to the stable tag, re-resolves it, and re-checks the published
  manifest against the deterministic two-layer contract. Any absent or
  conflicting stable tag fails without an overwrite.
- The release GitHub App immediately creates or verifies
  `release/plugin-emergency-evidence-<gateway>-<plugin>-<run-id>` and its pull
  request from the prevalidated lineage bytes. Only after that succeeds may the
  explicit `move_latest` option copy the digest to `latest`. A rerun of the
  same workflow run may find the stable tag already at the desired digest; a
  later run attempt records the missing evidence without copying the stable tag
  again. A first attempt cannot use that path, and a reused run ID with
  different evidence fails closed.
- Staging tags are retained permanently as provenance. The
  `emergency-<source-commit>-<input-hash>` tag is content-addressed by commit
  and input-hash (one per overwrite attempt, collision-free) and serves the same
  manifest as the stable tag. Registries commonly implement tag deletion as
  manifest deletion (ACR included), so untagging the staging reference could
  destroy the manifest the stable tag serves, and oras 1.2.3 provides no
  portable tag-only unlink. The committed lineage and its PR remain the durable
  record of an overwrite; the staging tag is the auditable public pointer to
  exactly what was verified before promotion. Do not attempt manual deletion of
  these tags.
- Registry mutation and evidence publication cannot be atomic. If the stable
  copy succeeds but lineage commit, push, or PR creation fails, the workflow
  remains failed. Keep immutability lifted only long enough to rerun the same
  workflow run so it verifies/reproduces the same bytes and completes the same
  deterministic evidence branch; do not dispatch a different run as a
  substitute. Merge the evidence PR before treating the incident as closed.
- A plugin whose tag was overwritten must receive a version bump at the next
  gateway release. This happens automatically: the fix commit changes the
  input hash, so plan re-plans it. Bundled plugin-server/Console images embed
  snapshot bytes and are healed only by the next gateway release; the overwrite
  heals tag-pulling (Envoy-direct) consumers immediately.

## Exact Standalone packaging

`deploy-standalone-to-oss` is manual and requires one verified Standalone
`vX.Y.Z` tag plus SHA-256 values for both its archive and
`src/get-higress.sh`. It downloads both from that exact tag and fails for a
missing tag or checksum mismatch. It never selects a release from HTML or
downloads an installer from `main`.
