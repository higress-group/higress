# Agent-assisted contributions

This document defines two related scopes. The mandatory issue-spec planning and
traceability gate applies only when an AI or coding agent materially
participates in a Higress contribution. The runtime-verification requirements
apply to every bug-fix PR, including human-only PRs.

The planning gate applies prospectively to material agent work begun after this
policy is merged. In-flight materially agent-assisted contributions should add
the new traceability and evidence where feasible, but authors must not fabricate
retroactive approval.

## Scope

Material participation means that an AI or coding agent does one or more of the
following:

- performs substantive analysis or design;
- selects an implementation or verification approach;
- produces or materially transforms code, tests, documentation, or
  configuration;
- interprets test results; or
- prepares substantive pull-request content.

Mechanical autocomplete that the author independently directs and verifies is
not automatically material participation.

The only exception is agent use limited to spelling, punctuation, whitespace,
or formatting corrections, with no substantive choice or behavioral effect.
The author must declare and explain this trivial-edit exception in the pull
request. Human-only contributions are outside the mandatory planning gate, but
human-only bug fixes remain subject to the runtime-verification requirements
below.

### Verified maintainer or administrator exception

A repository maintainer or administrator may elect to bypass the mandatory
planning and traceability gate for a materially agent-assisted contribution.
This is a role-based exception, not an exception to disclosure, review, merge,
or runtime-verification requirements. It does not apply merely because an
author claims a role or has write access to a fork.

Before relying on this exception, the author must use an authenticated GitHub
CLI session to verify both identity and permission against the canonical
repository:

```bash
set -euo pipefail
env -u GH_TOKEN -u GITHUB_TOKEN gh auth status --hostname github.com
LOGIN=$(env -u GH_TOKEN -u GITHUB_TOKEN \
  gh api --hostname github.com user --jq .login)
ROLE=$(env -u GH_TOKEN -u GITHUB_TOKEN gh api --hostname github.com \
  "repos/higress-group/higress/collaborators/${LOGIN}/permission" \
  --jq .role_name)
case "$ROLE" in
  maintain|admin) printf 'login=%s role_name=%s\n' "$LOGIN" "$ROLE" ;;
  *) printf 'ineligible canonical-repository role_name: %s\n' "$ROLE" >&2; exit 1 ;;
esac
# After creating this PR but before requesting review, set its number:
PR=123
PR_AUTHOR=$(env -u GH_TOKEN -u GITHUB_TOKEN gh pr view "$PR" \
  --repo higress-group/higress --json author --jq .author.login)
test "$PR_AUTHOR" = "$LOGIN" || {
  printf 'verified login %s does not match PR author %s\n' "$LOGIN" "$PR_AUTHOR" >&2
  exit 1
}
```

The returned `role_name` must be `maintain` or `admin`, and the actual author
of that PR must match `LOGIN` before review is requested. The explicit
environment unsets ensure verification uses the GitHub.com account configured by
`gh auth`, not a transient `GH_TOKEN` or `GITHUB_TOKEN` override. Record the
current PR number or URL, authenticated login, returned `role_name`, PR author,
an attestation that both `GH_TOKEN` and `GITHUB_TOKEN` were unset for every
verification command, and bypass rationale in the pull request without
credentials or tokens. Select the verified-maintainer/administrator exception
and the corresponding verified-exception gate status in the PR template. A
maintainer or administrator may still require the normal workflow for any
contribution. All bug-fix PRs, including those using this exception, must meet
the runtime-verification requirements below.
The PR declaration is evidence, not authority. Before accepting this exception
or merging the PR, the reviewing or merging maintainer must independently use
an authenticated GitHub.com `gh` session with `GH_TOKEN` and `GITHUB_TOKEN`
unset to query the actual PR author and that author's current canonical-
repository `role_name`. The maintainer must confirm that the result is
`maintain` or `admin` and that the PR author matches the recorded authenticated
login. A missing, stale, failed, or mismatched result makes the exception
unavailable and the normal issue-spec gate applies. This is a review-time
governance check, not a CI classifier or a grant of merge authority.
## Mandatory planning and traceability gate

When material participation applies, the contribution **must enter the Higress
issue-spec workflow before implementation begins**:

1. A Higress maintainer must approve a Proposal Issue and a Design Issue before
   implementation begins. Approval must be explicit in human-visible GitHub
   discussion or another linked repository governance record; an author or
   agent cannot self-approve.
2. The approved Design must define the implementation direction and contain a
   concrete Verification Plan before implementation verification begins.
3. Implementation must follow authorized implementation TASK comments and be
   traceable to the approved Design and the applicable SPEC comments.
4. Corresponding verification TASK comments must exist. Before maintainers are
   asked to accept the verification or review, applicable implementation and
   verification TASKs must be `done`, with their canonical summary/checklist
   containing or linking the exact commands, results, evidence locations, and
   hashes.

Every PR that used an AI or coding agent must disclose the prompts or
instructions and provide an AI-assisted work summary covering key decisions,
major changes, and important limitations. A verified maintainer or administrator
using the exception above must also record the `gh` identity/`role_name` result,
actual PR author, token-override attestation, and bypass rationale. The
accepting or merging maintainer independently validates the live evidence.

Agent-assisted PRs that declare material participation but do not satisfy this
gate receive low review priority, and timely maintainer review is not
guaranteed. The author declaration in the pull-request template is the
deterministic signal used for this prioritization. This is not an assertion
that CI can detect hidden agent use or automatically reject a PR.

### Authoritative Design Issue and optional durable specs

For contributions subject to this gate, the maintainer-approved issue-spec
Design Issue is the authoritative design carrier. A plugin-local `design/`
document is not required for the gate and is not authoritative.

A durable capability spec is optional and serves a separate, long-lived
purpose: recording stable capability requirements rather than change-specific
design. If maintainers request one, use
`issue-spec/specs/<plugin-qualified-capability>/spec.md`, where
`<plugin-qualified-capability>` is a unique lowercase, hyphen-separated slug
that identifies the plugin and capability. The durable spec does not replace
or duplicate the approved Design Issue. Workflow configuration supports only
`durable_specs.mode: none` or `durable_specs.mode: repository`. Repository mode
is project-wide: it cannot be scoped to selected plugins and can materialize
only at the canonical path above, or at an already-existing legacy
`openspec/specs/<plugin-qualified-capability>/spec.md`, never inside a
plugin-owned directory. Higress leaves `durable_specs` unset. Do not invent a
path field or enable repository projection without explicit maintainer
direction.

## Use the current issue-spec workflow

The commands below show the current portable CLI workflow. They do not depend
on a private runner or agent-session mechanism.

Check authentication and validate the selected project workflow before
authoring artifacts:

```bash
issue-spec auth status --json
issue-spec workflow validate --repo higress-group/higress --json
issue-spec search issues --repo higress-group/higress \
  --query "relevant topic or symbol" --state all --source all
```

Continue related Proposal or Design Issues when they already exist. Otherwise,
create phase Issues from reviewed body files:

```bash
issue-spec issue create proposal --repo higress-group/higress \
  --change descriptive-change-name --body-file proposal.md

issue-spec issue create design --repo higress-group/higress \
  --change descriptive-change-name --proposal 123 --body-file design.md
```

Generate canonical typed comments from structured JSON instead of hand-writing
their markers or layouts. New IDs use `<TYPE>-<issue><three-digit sequence>`;
for example, Proposal Issue `123` starts with `SPEC-123001`, and Design Issue
`124` starts with `TASK-124001`.

```bash
issue-spec comment generate --type SPEC --id SPEC-123001 --status draft \
  --scope "proposal requirements" --input-file spec.json \
  | issue-spec comment upsert --repo higress-group/higress --issue 123 \
      --type SPEC --id SPEC-123001 --status draft \
      --scope "proposal requirements" --body-file -

issue-spec comment generate --type TASK --id TASK-124001 --status draft \
  --scope "implementation" --input-file implementation-task.json \
  | issue-spec comment upsert --repo higress-group/higress --issue 124 \
      --type TASK --id TASK-124001 --status draft --scope "implementation" \
      --body-file - --covers-issue 123

issue-spec comment generate --type TASK --id TASK-124002 --status draft \
  --scope "verification" --input-file verification-task.json \
  | issue-spec comment upsert --repo higress-group/higress --issue 124 \
      --type TASK --id TASK-124002 --status draft --scope "verification" \
      --body-file - --covers-issue 123
```

A SPEC commonly moves from `draft` to `confirmed` when its requirement and
scenarios are settled. A TASK moves through the applicable `draft`, `ready`, or
`in-progress` states and reaches `done` only when its work and evidence are
complete. A CLI status is not maintainer approval.

The TASK generator renders every structured checklist item as unchecked; it
does not have a completion-checkbox input. To complete a TASK, read its current
canonical body, update the summary with exact commands, results, evidence links,
and hashes, mark only completed checklist items as `[x]`, and set the visible
status to `done`. Then upsert that completed canonical body with the same status
and scope:

```bash
issue-spec comment get --repo higress-group/higress --issue 124 \
  --type TASK --id TASK-124002 --include-body --json

# Save the returned canonical body as verification-task-done.md, update its
# summary/checklist/evidence, and set its visible Status to done before upsert.
issue-spec comment upsert --repo higress-group/higress --issue 124 \
  --type TASK --id TASK-124002 --status done --scope "verification" \
  --body-file verification-task-done.md --covers-issue 123
```

Inspect the selected planning state with:

```bash
issue-spec status --repo higress-group/higress \
  --proposal 123 --design 124 --json
```

An Implement Issue is optional unless it improves implementation coordination.
If one is selected, create it with `issue-spec issue create implement` and then
check the declared three-Issue lineage with:

```bash
issue-spec verify-links --repo higress-group/higress \
  --proposal 123 --design 124 --implement 125 --json
```

A PROCESS comment is also optional and should be used only for a concrete
managed-coordination need. The current generator supports SPEC, TASK, and
PROCESS comments. Historical REVIEW and VERIFY generators, final-verification
and rationale-evidence gates, and Archive delivery gates are retired or
audit-only and must not be used as the active workflow.

## Enforcement boundary

The issue-spec CLI natively validates the selected workflow configuration,
phase markers, canonical typed artifacts, and declared lineage. The built-in
phase order and artifact carriers remain authoritative; project configuration
can only add constraints within those steps.

Maintainers enforce whether this policy applies, whether Proposal and Design
approval is sufficient, whether TASK authorization and evidence are credible,
whether the verified maintainer or administrator exception is appropriate,
review priority, provider-native checks, review, and merge. The CLI does not
grant maintainer approval, infer undisclosed agent use, judge evidence quality,
or confer merge authority. `.issue-spec/config.json` remains connection/profile
metadata and does not carry governance gates.

## Runtime verification for bug fixes

Every bug fix requires a red/green comparison using the same pinned inputs and
configuration:

1. reproduce the bug on the affected or pre-fix baseline; and
2. confirm the expected behavior on the fixed revision or image.

For both variants, record:

- exact source revisions, versions, image tags and digests where available;
- manifests, Helm values, configuration, requests, and fixtures;
- expected and actual results;
- relevant logs and metrics;
- deterministic rerun and cleanup commands;
- machine-checkable assertions where practical; and
- evidence URLs plus hashes for uploaded artifacts.

Required unit, mutation, and regression tests remain part of the applicable
test suites, but they do not replace runtime evidence when the bug claim
concerns runtime behavior.

### Higress control-plane and data-plane fixes

Run both the baseline reproduction and fixed confirmation with Higress
installed in a real `kind` or `k3s` cluster. Record the Kubernetes and
`kind`/`k3s` versions, exact Higress revision/image, installation manifests or
Helm values, all relevant configuration, the triggering request or input,
expected and actual behavior, controller/gateway logs and relevant metrics, the
assertions, and deterministic cluster cleanup and rerun instructions.

### Wasm plugin fixes

Run both variants through a real proxy-Wasm data path. Go helper/parser tests
and native mocks alone are not sufficient runtime evidence. Record:

- the exact baseline and fixed source SHAs;
- the SHA-256 of each built Wasm module;
- the exact Envoy revision/version or pinned Higress gateway release image;
- the exact plugin configuration and triggering requests;
- client-response identity when relevant, such as status, selected headers,
  body bytes, and body SHA-256;
- Envoy access logs, plugin logs, and relevant metrics;
- repeated deterministic results when timing, streaming, caching, concurrency,
  or lifecycle behavior is relevant; and
- cleanup proof showing that no test ports, containers, or processes remain.

On Linux, contributors may use the Envoy binary from the relevant Higress
release with a local mock-upstream harness. On macOS, contributors may run
Envoy from a pinned affected/relevant Higress gateway release image and mount
the exact `envoy.yaml` and Wasm module. The repository's reusable
[Compose harness](./wasm-runtime-verification/README.md) demonstrates the
container option. Any tag shown there is illustrative only; actual evidence
must pin the affected or otherwise relevant release.

Concrete plugin-local references remain useful when adapting the generic
harness, including the
[Wasm Go custom-response example](../../plugins/wasm-go/examples/custom-response/docker-compose.yaml)
and the
[Wasm Rust SSE timing example](../../plugins/wasm-rust/example/sse-timing/docker-compose.yaml).

## Pull-request reporting

Complete every field in the pull-request template. Write `N/A` and explain why
for each category that does not apply; a blank field is not an explanation.
Link the approved Proposal and Design, implementation TASKs, the Design's
Verification Plan, verification TASKs, and their evidence. Bug-fix PRs must
link both baseline and fixed-version runtime evidence.
