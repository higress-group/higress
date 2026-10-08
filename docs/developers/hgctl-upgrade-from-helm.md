# hgctl recovered Helm upgrade

`hgctl upgrade --from-helm` is an opt-in source mode for clusters where the
current Higress installation was created by Helm and the next upgrade should be
applied through hgctl. It reconstructs an ephemeral Profile from the selected
Helm release metadata and Helm-retained user values, applies explicit overlays,
validates the final Profile, and then uses hgctl's normal render/apply path.

This mode does not persist the reconstructed Profile, does not update Helm
release history, and does not remove legacy hgctl Profile files.

## Selection and consent

Only deployed releases whose parent chart is exactly `higress` are candidates.
The release name is not used as identity, and the `higress-core` dependency is
not a candidate chart.

Use selectors when more than one candidate exists:

```bash
hgctl upgrade --from-helm --helm-release custom-higress --helm-namespace higress-system
```

When running without a terminal, selectors must resolve to exactly one release
and `--yes` is required:

```bash
hgctl upgrade --from-helm \
  --helm-release custom-higress \
  --helm-namespace higress-system \
  --yes
```

`--yes` only approves mutation after discovery, reconstruction, collision
checks, protected-field checks, and Profile validation succeed. It does not skip
those checks. Terminal cancellation or EOF exits before rendering or cluster
writes.

## Overlay precedence

Inputs are applied in this order:

1. Reconstructed Helm state from retained user values.
2. Each ordered `-f` values file.
3. Final `--set` values.

Typed Profile fields and their equivalent raw Helm value paths are normalized
after every layer. For example, retained
`higress-core.gateway.replicas=2` followed by
`--set gateway.replicas=3` renders the gateway Deployment with three replicas.

The recovered source identity is protected. Overlays must not set `profile`,
change `global.install`, or change the selected release namespace.

## Collision behavior

Recovered Helm mode fails closed when an existing hgctl Profile targets the
selected namespace. The check includes current profile files, the legacy
`~/.hgctl/profiles/install.yaml`, and the `higress-profile` ConfigMap.
Unreadable or malformed Profile files and Kubernetes API, authentication, or
permission failures also stop before confirmation.

Use normal `hgctl upgrade` for stored Profiles, or remove stale Profile state
only after verifying it is no longer authoritative.

## Compatibility window

The guaranteed source chart window is `>=2.1.0, <2.3.0`. Exact deployed
`higress` releases outside this window remain candidates, but recovery is
best-effort and must not be described as guaranteed or lossless.

Changing the guaranteed window requires a reviewed change that updates the
range, historical mappings, boundary fixtures, real Helm fixtures, tests, and
this documentation together.

## Partial apply recovery

If apply fails after some resources were updated, hgctl does not roll back
automatically. Retry with the same selected release, the same pinned target
chart version, and identical ordered `-f` and `--set` inputs. A dynamic `latest`
target, missing overlay, or changed overlay is not equivalent recovery evidence.

Because Helm release history is not rewritten, a later Helm operation may
overwrite the applied state. Later hgctl upgrades must use `--from-helm` again
and repeat any explicit overlays that are not present in Helm-retained values.

## Verification notes

Verification evidence must avoid raw retained values. Include exact commands,
source revision, chart versions, tool versions, artifact hashes, sanitized
results, and cleanup steps. Sensitive canaries may appear in legitimate target
resources or pre-existing Helm release storage, but not in command output,
generic errors, Profile stores, temporary evidence copies, or published
evidence.
