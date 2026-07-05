---
name: "Issue Spec: Propose"
description: "Create or continue proposal, SPEC, QUESTION, design, and TASK artifacts for an issue-spec change."
category: "Workflow"
tags: ["workflow", "issue-spec"]
---

# Issue Spec Propose

Use when the user asks for /issue-spec:propose, issue-spec propose, creating a change proposal, drafting SPEC comments, or preparing design/tasks after questions converge.

## Steps

1. Create the proposal issue:

       issue-spec issue create proposal --repo higress-group/higress --change <change-name> --body-file <proposal.md>

2. If the proposal body needs revision after discussion, update it in place:

       issue-spec issue update --repo higress-group/higress --issue <proposal-issue> --body-file <proposal.md> --summary "<what changed>"

3. Add SPEC comments with issue-spec comment upsert --type SPEC. SPEC comments must use MUST/SHALL and WHEN/THEN scenarios.
4. Add QUESTION comments for unresolved behavior with issue-spec question create and resolve blocking questions before design.
5. Create the design issue after SPEC/QUESTION convergence:

       issue-spec issue create design --repo higress-group/higress --change <change-name> --proposal <proposal-issue-or-url> --body-file <design.md>

6. Add TASK comments with issue-spec comment upsert --type TASK and link every TASK to covered SPEC comments with issue-spec link.
7. Create the implement issue once tasks are ready:

       issue-spec issue create implement --repo higress-group/higress --change <change-name> --proposal <proposal-issue-or-url> --design <design-issue-or-url> --body-file <implement.md>

8. Run issue-spec verify-links and fix missing backlinks before implementation.
