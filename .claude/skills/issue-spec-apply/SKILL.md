---
name: issue-spec-apply
description: Implement PROCESS comments for an issue-spec change and keep PR traceability synchronized.
license: MIT
compatibility: Requires issue-spec CLI.
metadata:
  author: issue-spec
  version: "1.0"
  generatedBy: "issue-spec"
---

# Issue Spec Apply

Use when the user asks for /issue-spec:apply, issue-spec apply, or implementing PROCESS/TASK scopes from an issue-spec change.

## Steps

1. Read proposal/design/implement issue context and list typed comments with issue-spec comment list --json.
2. Confirm issue-spec auth status --json includes the expected GitHub backend. Local gh-authenticated sessions can use the native gh backend; keep ISSUE_SPEC_TOKEN="$(gh auth token)" only as an older-version or forced-rest compatibility path.
3. Create or update PROCESS comments with owner agent, scope, dependencies, write ownership, and status.
4. Split non-trivial work into independent worker PROCESS nodes when file/module ownership does not overlap; execute independent workers in parallel when available.
5. Add dedicated review PROCESS nodes for non-trivial changes. Review PROCESS nodes should own review scopes such as CLI/API behavior, workflow docs, tests, compatibility, or security-sensitive surfaces.
6. Link each PROCESS to its TASK comments with issue-spec link.
7. Implement the code changes for one PROCESS scope at a time, or integrate completed worker outputs by dependency order.
8. Link every worker and review PROCESS to the PR with issue-spec pr link-process.
9. Add PR rationale comments on key changed lines with issue-spec pr rationale, each linked to a SPEC comment.
10. Mark PROCESS comments done only after implementation/review work and focused verification evidence exist.

## Coordinator DAG Execution

1. Build the ready set from PROCESS nodes whose dependencies are done.
2. Keep immediate blocking work local when the next step depends on it.
3. Spawn or assign independent worker agents only when their write ownership is disjoint.
4. Spawn or assign independent review agents only when their review scopes are disjoint.
5. Integrate completed outputs by dependency order and update PROCESS evidence before marking done.
