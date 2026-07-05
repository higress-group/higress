---
name: issue-spec-workflow
description: Use issue-spec to run an issue-native OpenSpec-style workflow with GitHub issues, typed comments, PR review comments, final verification, and durable spec archive PRs.
license: MIT
compatibility: Requires issue-spec CLI.
metadata:
  author: issue-spec
  version: "1.0"
  generatedBy: "issue-spec"
---

# Issue Spec Workflow

Use this skill for issue-native OpenSpec work. Active change artifacts live in GitHub issues and issue comments; durable specs are repository files created after implementation merge.

## Start

1. Run issue-spec auth status --json and confirm the active auth source and GitHub backend.
2. Run issue-spec status --repo higress-group/higress --proposal <issue> --design <issue> --implement <issue> --json when issues already exist.
3. For new work, create proposal, design, and implement issues with issue-spec issue create and pass --body-file with concrete markdown content.
4. When an issue body changes, update it in place with issue-spec issue update --body-file and include --summary for the human-readable audit trail.
5. Store requirements, tasks, process ownership, review, and verify evidence as typed comments.

## GitHub Backend

- Local agents may rely on native GitHub CLI support: when no ISSUE_SPEC_TOKEN, GH_TOKEN, GITHUB_TOKEN, keyring token, or issue-spec config token is present and gh auth status --active succeeds for the target host, issue-spec auto-selects the gh backend.
- Explicit env or stored issue-spec tokens keep the rest backend under auto selection. Set ISSUE_SPEC_GITHUB_BACKEND=rest or ISSUE_SPEC_GITHUB_BACKEND=gh only when a workflow needs deterministic backend selection.
- The gh backend proxies GitHub API operations through gh api and uses gh --hostname for Enterprise hosts. It does not replace local git commands.
- ISSUE_SPEC_API_URL applies to the rest backend. Forced gh mode should be used only with hosts that gh can address.
- Use ISSUE_SPEC_TOKEN="$(gh auth token)" only for older issue-spec versions or when deliberately forcing rest while sourcing the token from gh.

## Rules

- Create SPEC comments before design; each SPEC must be testable and include WHEN/THEN scenarios.
- Do not leave active proposal/design/implement issue bodies as TBD placeholders.
- Resolve blocking QUESTION comments before design/tasks, or explicitly record accepted assumptions.
- Link SPEC <-> TASK and TASK <-> PROCESS with issue-spec link.
- Link every PROCESS to the implementation PR with issue-spec pr link-process.
- For non-trivial changes, include review PROCESS nodes in the DAG; review agents are scheduled like worker agents and can run in parallel when their review scopes are independent.
- Small changes may stay coordinator-only, but record the serial execution decision in the implement or VERIFY evidence.
- Before human review, add PR rationale comments with issue-spec pr rationale for every active PROCESS.
- Use issue-spec review finding for PR line findings and issue-spec review reply to close the original thread.
- Run issue-spec review sync and issue-spec verify before declaring ready.
- After the implementation PR merges, create the separate durable spec PR with issue-spec archive durable-spec --create-pr.

## Coordinator DAG Execution

1. Treat PROCESS comments as DAG nodes with explicit owner, dependencies, write or review scope, PR link, and evidence.
2. Select ready PROCESS nodes whose dependencies are done and whose scopes do not overlap.
3. Dispatch independent worker PROCESS nodes in parallel when their file/module ownership is disjoint.
4. Dispatch independent review PROCESS nodes in parallel for non-trivial PRs after PR rationale exists.
5. Integrate completed worker outputs by dependency order; route P0/P1 review findings back to the owner PROCESS.
6. Mark PROCESS nodes done only after their implementation or review evidence is recorded and blocking findings are resolved.
