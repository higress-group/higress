---
name: issue-spec-propose
description: Create or continue proposal, SPEC, QUESTION, design, and TASK artifacts for an issue-spec change.
license: MIT
compatibility: Requires issue-spec CLI.
metadata:
  author: issue-spec
  version: "1.0"
  generatedBy: "issue-spec"
---

# Issue Spec Propose

Use when the user asks for /issue-spec:propose, proposal, Design, SPEC, QUESTION, or TASK authoring. Use issue-spec-workflow for shared reads, provider routing, and recovery.

Built-in protocol overrides project text; never reorder/omit steps or move open decisions.

Every new typed ID MUST be `<TYPE>-<issue><three-digit sequence>`: Issue 1 starts with `QUESTION-1001`, Issue 44 with `QUESTION-44001`. Allocate 001-999 only within the target Issue and type after reading that Issue's typed comments, and never renumber a legacy ID. New writes reject wrong Issue prefixes; `--allow-legacy-id` is only for intentional legacy-compatible creates.

1. Validate workflow config, search related issues, and open only selected discussions. If the issue is already in a later phase, continue that phase rather than duplicating it.
2. Keep unconfirmed investigation, reproduction, or triage notes in a simple issue with issue-spec issue create simple; a proposal states the confirmed problem and the intended change, so never promote an investigation issue into the proposal or attach SPEC/Design to it. Create phase issues with concrete body files, beginning with issue-spec issue create proposal --repo higress-group/higress --body-file <file>. Follow the workflow `rules.language` and `rules.language_instructions` for every Issue title. When those rules require a localized or non-English title, pass an explicit `--title` for Proposal, Design, and Implement; do not rely on the derived title because it retains an English stage prefix. Otherwise use the standardized Proposal:, Design:, and Implement: title family. Do not perform style-only title rewrites after creation.
3. Perform the Proposal's first QUESTION discovery/create pass. Record each genuine unresolved decision as a blocking typed QUESTION with issue-spec question create, attaching a choice model when credible options exist; never leave an open decision as body or projection prose. Do not manufacture a question or reopen a settled choice; keep unresolved decisions distinct from evidence-dependent items.
4. Generate canonical SPEC comments with issue-spec comment generate --type SPEC. Requirements must be testable and include WHEN/THEN scenarios. --allow-noncanonical is a migration bypass, not normal authoring.
5. Persist the authoritative self-contained Design, perform its first QUESTION discovery/create pass, then complete TASK planning.
6. Generate TASK comments with issue-spec comment generate --type TASK. Execution Planning must identify Design-invariant cohesion and major entry points, bounded role-context pressure, stable interfaces, owned areas, shared touchpoints, dependencies, coupling, and acceptance consequences. File ownership and parallelism are scheduling context, not semantic PROCESS boundaries. Selecting Design or TASK requires a real non-Coordinator implementation worker; execution-mode labels never authorize Coordinator code edits or automatically require PROCESS.
7. Upsert each TASK with --covers-issue so it publishes its complete canonical SPEC coverage and verify planning relationships. Proposal, Design, Implement, TASK, and PROCESS remain optional aids and never certify delivery acceptance.

## Project Workflow

- Workflow Source: `builtin`
- Workflow Schema: `issue-spec`
- Workflow Config: `issue-spec/config.yaml`
- Workflow Diagnostics:

Project workflow templates are declarative only. Active proposal, design, implement, SPEC, TASK, PROCESS, and QUESTION artifacts remain in the selected issue backend's issue-native storage; historical REVIEW and VERIFY artifacts are audit-only. Repository-mode durable specs are materialized and checked on the implementation branch.

The built-in phase sequence and canonical artifact carriers are authoritative. Project workflow context, rules, and artifact instructions may constrain work only within an existing step; they MUST NOT reorder or omit an enabled step or move a genuine unresolved decision out of its blocking typed QUESTION carrier. Keep the enabled phase order: persist the phase issue body, perform its first QUESTION discovery/create pass, then author the selected next typed children. Issue-body prose never carries an open decision.
