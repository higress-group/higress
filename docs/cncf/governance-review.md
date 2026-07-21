# Higress Governance Review

This is the Higress project's self-assessed Governance Review for its CNCF
Incubation application. It follows the current CNCF TOC
[Governance Review Template](https://github.com/cncf/toc/blob/main/toc_subprojects/project-reviews-subproject/governance-review-template.md).
A CNCF Project Reviews reviewer may amend the assessment and findings during
the public review.

## Summary and Assessment

**Status: Needs Work**

Higress has discoverable governance, maintainer, code-owner, contribution,
security, and Code of Conduct documents. Decisions default to public lazy
consensus, and the maintainer list includes seven people affiliated with four
organizations.

The review nevertheless finds several Incubation-required evidence gaps: the
maintainer roster does not state responsibility domains; the relationship
between `CODEOWNERS` and governance roles is not defined; the CNCF project
subproject scope is ambiguous; all communication channels are not inventoried;
and there is no documented current public meeting scheduler. These findings
must be resolved before the project claims the Governance Review criterion is
satisfied.

### Executing the Assessment

The self-assessment reviewed the following repository evidence as it exists on
2026-07-21:

- [`GOVERNANCE.md`](../../GOVERNANCE.md)
- [`MAINTAINERS.md`](../../MAINTAINERS.md)
- [`CODEOWNERS`](../../CODEOWNERS)
- [`CONTRIBUTING_EN.md`](../../CONTRIBUTING_EN.md)
- [`CODE_OF_CONDUCT.md`](../../CODE_OF_CONDUCT.md)
- [`SECURITY.md`](../../SECURITY.md)
- [`README.md`](../../README.md)
- Git history for the governance, maintainer, and ownership files

The review distinguishes documented policy from observed repository evidence.
It does not infer private processes or settings that are not publicly recorded.

### Must-Fix Items

The following issues need to be resolved before the Higress Incubation
application asserts completion of its Governance Review:

1. Add each maintainer's domain of responsibility to `MAINTAINERS.md` and
   confirm the listed people are currently active.
2. Define the authority and lifecycle of code owners and explain how
   `CODEOWNERS` relates to project-wide maintainer authority.
3. Document a complete maintainer lifecycle, including onboarding, voluntary
   departure, inactivity/removal, emeritus status, and return.
4. Explicitly document vendor-neutral project direction and conflict-of-
   interest handling.
5. Define the repositories/subprojects in the CNCF Higress project scope and
   the process for adding, removing, or archiving a subproject.
6. Inventory all public and non-public communication channels and state the
   limited purpose of each non-public channel.
7. Publish an up-to-date public meeting scheduler or CNCF calendar entry and a
   discoverable location for agendas and notes.
8. Document how the project approves requests to CNCF and how function-based
   teams such as the Security Response Team are assigned and removed.

### Points of Excellence

- The maintainer roster is public and shows affiliation diversity across
  Alibaba Cloud, Trip.com, XinYe Technology, and NVIDIA.
- The project has a public contribution path, code-review ownership file,
  vulnerability reporting process, and CNCF-aligned Code of Conduct.
- Governance declares openness, fairness, community-first decision-making,
  inclusivity, participation, public lazy consensus, and public voting when
  consensus fails.

### Areas for Improvement

The following are non-blocking at Incubation but would improve long-term
governance maturity:

- Introduce a contributor ladder with intermediate roles and objective
  progression expectations.
- Record governance evolution and link decisions to issues or pull requests.
- Publish examples demonstrating the maintainer lifecycle in practice.
- Periodically audit maintainer activity and code-owner coverage across all
  project repositories.
- Track contributor growth and recruitment using public CNCF/DevStats metrics.

---

## Review

This review audits the project's current governance evidence. The project's
Incubation application has not yet been opened; this file is the project's
self-assessment to accompany that application.

### Governance Summary

Higress uses maintainer-led lazy consensus. When consensus cannot be reached,
maintainers may vote publicly and a simple majority of votes cast decides. The
governance document delegates role detail to the maintainer, code-owner, and
contribution documents. Security reports are handled by the maintainers. The
model is understandable at a high level, but role boundaries and lifecycle
details are incomplete.

### Governance Evolution

**Incubating: Suggested — Partially satisfied.**

`GOVERNANCE.md` and `MAINTAINERS.md` were introduced in April 2026, and
`CODEOWNERS` has changed repeatedly since 2022. This demonstrates change over
time, but the repository does not explain why governance evolved or connect
those changes to project experience and outcomes.

### Discoverability

**Incubating: Suggested — Satisfied.**

The README links the governance, maintainer, contribution, Code of Conduct, and
security documents from its Community section.

### Accuracy and Clarity

**Governance reflects actual activities — Incubating: Suggested — Partially
satisfied.**

Public issue/PR collaboration and lazy consensus are consistent with the
repository workflow. No election or recurring meeting is claimed. However,
actual code-owner authority, security-team operation, and meeting practices are
not fully documented.

**Vendor-neutral direction — Incubating: Suggested — Not satisfied.**

The values say contributions are evaluated without regard to company
affiliation, but governance does not explicitly prohibit employer seats,
employer vetoes, vendor-favored defaults, or undisclosed conflicts of interest.

### Decisions and Role Assignments

**Leadership, contribution, CNCF, governance, and goal decisions — Incubating:
Suggested — Partially satisfied.**

Maintainer nomination and general lazy consensus/voting are documented.
Contribution acceptance is described in the contribution guide and CODEOWNERS.
Requests to CNCF, project-scope changes, and detailed governance-change notice
or quorum requirements are not documented.

**Function-based teams — Incubating: Suggested — Not satisfied.**

`SECURITY.md` states that all maintainers handle security response, but does
not define onboarding, removal, conflicts, escalation ownership, or a change
process for that function.

### Maintainers and Maintainer Lifecycle

**Complete lifecycle — Incubating: Suggested — Not satisfied.**

The project documents nomination and lazy-consensus acceptance. It does not
document offboarding, inactivity, removal, emeritus status, or return.

**Lifecycle demonstrated — Incubating: Suggested — Not demonstrated.**

The maintainer file has only one introducing commit in the available history.
No public example of adding, replacing, or moving a maintainer to emeritus is
linked.

**Names, contact, responsibility, affiliation — Incubating: Required —
Partially satisfied.**

Names, GitHub contacts, and affiliations are present. Responsibility domains
are absent.

**Appropriate number of active maintainers — Incubating: Required — Evidence
incomplete.**

Seven maintainers are listed, which is plausible for the project scope, but the
roster alone does not prove current review, release, and governance activity.
The application should link recent evidence for each active maintainer or
update the roster.

**Maintainers from at least two organizations — Incubating: N/A, but met.**

The roster names four affiliations.

### Ownership

**Code and documentation ownership matches governance roles — Incubating:
Required — Evidence incomplete.**

`CODEOWNERS` assigns directory review to maintainers and additional GitHub
users. Governance does not define “code owner,” its authority, appointment, or
removal, so the review cannot establish that the file matches documented roles.

### Code of Conduct

**Adoption and adherence — Incubating: Required — Satisfied as documented.**

The project adopts the CNCF Code of Conduct in `CODE_OF_CONDUCT.md` and links it
from the README and governance.

**Cross-link from governance — Incubating: Required — Satisfied.**

`GOVERNANCE.md` links both the CNCF Code of Conduct and the project copy.

### Subprojects

**All subprojects listed — Incubating: Required — Not satisfied.**

The README labels `higress-console`, `higress-standalone`, `plugin-server`, and
`wasm-go` as “Related Repositories,” but does not distinguish CNCF subprojects
from integrations or independently governed related tools. No authoritative
scope list exists in governance.

**Subproject lifecycle — Incubating: Suggested — Not satisfied.**

Leadership, contribution, maturity status, and add/remove/archive processes are
not documented for the related repositories.

### Contributors and Community

**Contributor ladder — Incubating: Suggested — Partially satisfied.**

Contributor and maintainer paths exist, and CODEOWNERS implies an intermediate
review role, but the role is not governed and no complete ladder is documented.

**Issue and change submission — Incubating: Required — Satisfied.**

`CONTRIBUTING_EN.md`, issue templates, and the pull-request template document
the process.

**At least one public communication channel — Incubating: Required —
Satisfied.**

GitHub Issues, pull requests, and Discord are publicly documented.

**All public/private channels documented — Incubating: Required — Not
satisfied.**

The README documents Discord and GitHub contribution paths, and `SECURITY.md`
documents two private vulnerability-reporting channels. There is no single
inventory asserting completeness, covering subprojects, or explaining all
non-public purposes.

**Public meeting scheduler/CNCF calendar — Incubating: Required — Not
satisfied.**

No current scheduler, CNCF calendar entry, agenda, or meeting-notes location is
linked from the reviewed repository documents.

**Contribution documentation — Incubating: Required — Satisfied.**

The contribution guide covers issues, pull requests, branches, commits, tests,
style, and AI-assisted contribution requirements.

**Contributor activity and recruitment — Incubating: Required — Evidence
incomplete.**

The README displays the contributor graph and the repository has recent
multi-author activity, but the Incubation application should provide public
metrics demonstrating sustained contributor growth, review participation, and
recruitment beyond raw commit counts.
