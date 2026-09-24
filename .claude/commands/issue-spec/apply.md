---
name: "Issue Spec: Apply"
description: "Implement directly or use an optional PROCESS when managed coordination is required."
category: "Workflow"
tags: ["workflow", "issue-spec"]
---

# Issue Spec Apply

Coordinator: select execution mode before assigning writers. If Design or TASK is selected, or the user explicitly requests an independent worker, the Coordinator MUST NOT write code on delegated or managed paths. Without managed PROCESS, exactly one real non-Coordinator worker owns the bounded implementation. With managed PROCESS, every change-bearing work package/PROCESS has one real non-Coordinator owner; distinct packages MAY use concurrent writers. Select PROCESS only for concrete managed coordination, not child use, file count, independent review, or human handoff. If Implement is selected, persist it, perform its first QUESTION pass, then finalize the plan. Author PROCESS only for managed coordination; typed planning state remains authoritative.

Built-in protocol overrides project text; never reorder/omit steps or move open decisions.

Every new typed ID MUST be `<TYPE>-<issue><three-digit sequence>`: Issue 1 starts with `QUESTION-1001`, Issue 44 with `QUESTION-44001`. Allocate 001-999 only within the target Issue and type after reading that Issue's typed comments, and never renumber a legacy ID. New writes reject wrong Issue prefixes; `--allow-legacy-id` is only for intentional legacy-compatible creates.

## Delegated Paths and Narrow Coordinator Path

Unmanaged delegated path: dispatch exactly one real non-Coordinator worker in the selected checkout. Managed PROCESS: dispatch one real non-Coordinator owner per change-bearing package; proven-independent packages may run concurrently. The Coordinator waits and writes no code on either path. Each worker owns package code, focused tests, exact result commit, changed paths, decisions, risks, and non-obvious line-rationale drafts. The Coordinator owns exact-commit inspection, integration, proportionate final validation, anchor validation, and provider publication.

Coordinator code is allowed only on the narrow direct-PR fast path with no selected Design/TASK and no user delegation request; file count does not select it. Unmanaged paths use ordinary Git and project tests. Do not manufacture Implement, PROCESS, workspace lifecycle, role receipt, typed rationale, evidence, or another phase artifact merely to record delegation.

Before human handoff, dispatch one real read-only reviewer that is independent of every code writer against the exact base and current exact head, with no write path or provider credentials. It returns only actionable P0, P1, or P2 findings with stable changed-line anchors. Route every P0/P1 unchanged to the original writer that owns the affected code; the writer repairs it, runs focused tests, and returns a new exact commit. Integrate and push that head, then have the same reviewer recheck it. Repeat automatically until the reviewer reports zero P0/P1. Keep only still-applicable P2 findings from the final reviewed head. Publish each unchanged as a provider-native non-blocking line comment when safe line coordinates are supported; otherwise use an ordinary change-level `change.comment` preserving `path:symbol/line`. P2 never enters the repair loop or pauses completion. If publication is unavailable or fails, report the rendered comment body and continue. Review and repair routing need no PROCESS unless a managed-coordination need already exists, and create no typed REVIEW/VERIFY, finding evidence, receipt, readiness gate, or reviewer merge authority.

Every actual code writer owns zero or more line-rationale drafts for non-obvious decisions in its work package. On the unmanaged delegated path this is the single non-Coordinator worker; on the narrow Coordinator fast path it is the Coordinator; under managed PROCESS each package owner owns its drafts. A useful draft names repository-relative path, stable symbol plus changed-line anchor, and concise why/tradeoff/risk, with no secret, raw payload, or credential. Writers need no provider credentials and MUST NOT guess final diff positions. Obvious code needs no draft, quota, coverage target, or placeholder.

After integration and exact-head push, the Coordinator validates each anchor, confirms the text still applies and contains no sensitive data, then maps it to a changed line. Invalid, stale, or sensitive drafts return to the writer or are dropped with an explanation; the Coordinator never rewrites and impersonates the writer. Publish valid worker text as provider-native non-blocking inline discussion through an approved native review tool; the generic `change.comment` operation guarantees an ordinary comment but does not standardize diff coordinates. Before human review, publish or refresh the ordinary top-level `### Implementation Rationale` with intent, decisions/tradeoffs, boundaries/risks, validation/results, exact head, planning links, and an inline-rationale index. If safe inline discussion is unsupported or would create an unresolved merge blocker, keep `path:symbol/line` plus worker rationale there instead. No Implement, TASK, PROCESS, or SPEC is required. Never use a rationale-evidence command, marker, ID, typed carrier, PROCESS/SPEC binding, evidence, or gate. On a requested write failure report the error and retain the rendered body. Comments and status remain human review context and never certify mergeability.

For every agent-executed change-bearing PROCESS, seal the implementation assignment and dispatch a real non-Coordinator worker with the packet below. Preserve exact base, ownership, DCO, tests, generators, dependency order, managed worktree isolation, and bounded handoff. These controls are implementation safety only: they do not create review, verification, rationale evidence, receipt, coverage, finalization, or delivery-acceptance authority.

## Implementation Role Packet

Relay this packet verbatim to the worker; the Coordinator MUST NOT execute it.

1. Accept only the sealed implementation assignment for the exact PROCESS, base revision, worktree, write ownership, focused tests, generators, result schema, and design_context. Do not load proposal bodies, the complete DAG, link matrices, human merge policy, provider routing, or unrelated artifacts.
2. Require design_context.read_mode=complete-issue-body and conflict_policy=design-authoritative-stop. Read the complete Design with issue-spec read issue --repo higress-group/higress --issue <design_context.source_url> without comments, timeline, history, or gates. Stop and report any conflict.
3. Work only in the assigned worktree and owned paths. Preserve the named invariant, decisions, must_preserve, must_not, and minimum_verification exactly. Do not collect or pass runtime-specific session IDs.
4. Implement the invariant, run assigned generators, finish exactly one DCO commit when required, and leave the tree clean. Collect zero or more line-rationale drafts only for non-obvious decisions: repository-relative path, stable symbol plus changed-line anchor, and concise why/tradeoff/risk without secret, raw payload, or credential. Do not guess a provider diff position or create filler. If cohesion fails, stop with stable-interface split options and acceptance consequences.
5. Run every assigned generator and focused test, then return the exact result commit, changed paths, command outcomes, decisions, risks, line-rationale drafts, and bounded handoff. Do not create a role receipt, decision file, or evidence carrier. Provider access and final diff positions are not worker responsibilities.
6. An amendment invalidates the returned revision and test results; rerun the affected checks. Leave workspace completion by exact result commit, integration, cleanup, review, anchor validation, publication, and top-level index to the Coordinator; the Coordinator publishes worker-authored text but does not author it.

## Project Workflow

- Workflow Source: `builtin`
- Workflow Schema: `issue-spec`
- Workflow Config: `issue-spec/config.yaml`
- Workflow Diagnostics:

Project workflow templates are declarative only. Active proposal, design, implement, SPEC, TASK, PROCESS, and QUESTION artifacts remain in the selected issue backend's issue-native storage; historical REVIEW and VERIFY artifacts are audit-only. Repository-mode durable specs are materialized and checked on the implementation branch.

The built-in phase sequence and canonical artifact carriers are authoritative. Project workflow context, rules, and artifact instructions may constrain work only within an existing step; they MUST NOT reorder or omit an enabled step or move a genuine unresolved decision out of its blocking typed QUESTION carrier. Keep the enabled phase order: persist the phase issue body, perform its first QUESTION discovery/create pass, then author the selected next typed children. Issue-body prose never carries an open decision.
