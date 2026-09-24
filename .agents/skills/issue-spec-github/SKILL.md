---
name: issue-spec-github
description: Use GitHub CLI for GitHub issues, pull requests, CI runs, and API queries that issue-spec does not wrap.
license: MIT
compatibility: Requires GitHub CLI (gh).
metadata:
  author: issue-spec
  version: "1.0"
  generatedBy: "issue-spec"
---

# GitHub CLI

Use the gh CLI only for GitHub operations outside issue-spec's workflow and discussion surfaces.

## Use

- Inspect PR status, reviews, mergeability, CI, workflow runs, releases, labels, and repository metadata.
- Use structured --json/--jq output. Use git directly for local repository operations.
- After the code writer returns valuable line-rationale drafts, validate each stable path/symbol/changed-line anchor against the pushed exact head and confirm the rationale still applies and contains no secret, raw payload, or credential. Return invalid, stale, or sensitive drafts to the writer, or drop them with an explanation; never rewrite them while claiming worker authorship. Publish valid unchanged text as a GitHub-native inline PR comment by resolving `commit_id`, `path`, right-side `line`, and `side=RIGHT` after push. Writers need no GitHub access and never guess diff positions. Publish no filler.
- Before human-review handoff, publish or refresh the ordinary GitHub PR discussion headed `### Implementation Rationale` through `gh pr comment <pr> --body-file <file>` and use it as the summary/index for inline rationale. If a safe inline comment cannot be created, retain `path:symbol/line` plus the writer-authored rationale in this top-level discussion. Report requested write failure and retain the body without treating any rationale comment as evidence or delivery acceptance.
- Ordinary issue discussion writes: write a body file and run issue-spec comment create --repo owner/repo --issue 42 --body-file reply.md --json. The selected issue backend owns the write. Never use GitHub CLI or a raw issue-comment API write.
- issue-spec owns optional planning, implementation coordination, durable projection, PR context, and human handoff. The human and code host own current review, checks, approval, merge, and closing behavior. Do not use GitHub endpoints for non-GitHub providers.

## Setup and examples

    gh auth login
    gh auth status
    gh pr view 17 --repo owner/repo --json number,title,state,url
    gh pr checks 17 --repo owner/repo
    gh run view <run-id> --repo owner/repo --log-failed
    gh api repos/owner/repo/labels --jq '.[].name'
