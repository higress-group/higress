---
name: issue-spec-review
description: Review an issue-spec implementation PR, create PR line findings, reply after fixes, and sync REVIEW comments.
license: MIT
compatibility: Requires issue-spec CLI.
metadata:
  author: issue-spec
  version: "1.0"
  generatedBy: "issue-spec"
---

# Issue Spec Review

Use when the user asks for /issue-spec:review, issue-spec review, or a PR review gate for an issue-spec implementation.

## Steps

1. Run issue-spec review sync --repo higress-group/higress --pr <number> --implement <issue> --id REVIEW-<n> --json to capture current rationale comments, findings, and checks.
2. For non-trivial PRs, spawn or assign dedicated review agents as review PROCESS owners. Multiple review agents can run in parallel when their review scopes are independent.
3. Give each review agent a concrete scope and expected output: actionable findings only, severity, file/line, linked SPEC, owner PROCESS, and suggested fix.
4. Create actionable PR line findings with issue-spec review finding. Use P0/P1 for blockers and P2 for non-blocking follow-up.
5. Assign every finding to a PROCESS owner. If no findings are found, record that result in REVIEW or VERIFY evidence.
6. After the worker fixes a finding, reply to the original thread with issue-spec review reply --status resolved.
7. Re-run review sync. P0/P1 findings must be resolved before final verify/archive.

## Review DAG Policy

1. Every non-trivial PR should have at least one dedicated review PROCESS node before final verify.
2. Use multiple review agents in parallel when scopes are independent, for example CLI/API behavior, workflow docs, tests, compatibility, or security-sensitive surfaces.
3. A review agent reports findings only; the coordinator converts actionable line findings into issue-spec review finding comments.
4. P0/P1 findings block final verify until the owner PROCESS fixes them and issue-spec review reply records the resolution on the original thread.
5. If a review agent finds no issues, record that result in REVIEW or VERIFY evidence before marking the review PROCESS done.
