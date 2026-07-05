---
name: issue-spec-archive
description: Create the post-merge durable spec archive PR for an issue-spec change.
license: MIT
compatibility: Requires issue-spec CLI.
metadata:
  author: issue-spec
  version: "1.0"
  generatedBy: "issue-spec"
---

# Issue Spec Archive

Use when the user asks for /issue-spec:archive, issue-spec archive, or creating the post-merge durable spec PR.

## Steps

1. Confirm the implementation PR is merged.
2. Create the durable spec PR:

       issue-spec archive durable-spec --repo higress-group/higress --proposal <issue> --capability <capability> --create-pr --branch issue-spec/durable-spec-<capability> --json

3. Review the durable spec PR for long-lived behavior only. Do not copy process records, review findings, or verification logs into durable specs.
4. After durable spec PR merge, keep proposal/design/implement issues as audit history unless the project policy says to close them.
