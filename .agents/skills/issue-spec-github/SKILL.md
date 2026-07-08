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

Use the `gh` CLI to interact with GitHub repositories, issues, pull requests, CI, and API endpoints.

## When To Use

- Checking PR status, reviews, mergeability, or CI checks.
- Creating, viewing, updating, closing, or commenting on GitHub issues.
- Listing or inspecting pull requests, workflow runs, releases, labels, or repository metadata.
- Calling GitHub API endpoints with `gh api` when issue-spec does not provide a dedicated command.

## When Not To Use

- Local git operations such as commit, branch, fetch, merge, or push. Use `git` directly.
- Non-GitHub repositories. Use the matching provider CLI instead.
- Complex code review across local diffs. Read the repository files directly and use issue-spec review commands for traceable findings.

## Setup

```bash
gh auth login
gh auth status
```

## Common Commands

```bash
gh issue list --repo owner/repo --state open
gh issue view 42 --repo owner/repo --json number,title,state,url,body
gh issue comment 42 --repo owner/repo --body "Comment body"

gh pr list --repo owner/repo
gh pr view 17 --repo owner/repo --json number,title,state,headRefName,baseRefName,url
gh pr checks 17 --repo owner/repo

gh run list --repo owner/repo --limit 10
gh run view <run-id> --repo owner/repo --log-failed

gh api repos/owner/repo/labels --jq '.[].name'
```

## Notes

- Always pass `--repo owner/repo` when the current directory is not definitely inside the target repository.
- Use GitHub URLs directly when convenient, for example `gh pr view https://github.com/owner/repo/pull/17`.
- Prefer structured output with `--json` and `--jq` when another command or agent step consumes the result.
- issue-spec owns the proposal, design, implement, typed comment, review, verify, and archive workflow state. Use `gh` for adjacent GitHub operations that are outside issue-spec's command surface.
