# Higress Governance Review

This is the Higress project's self-assessed Governance Review for its CNCF
Incubation application. It follows the current CNCF TOC
[Governance Review Template](https://github.com/cncf/toc/blob/main/toc_subprojects/project-reviews-subproject/governance-review-template.md).
A CNCF Project Reviews reviewer may amend the assessment and findings during
the public review.

- **Review state:** Project working draft; maintainer approval and CNCF Project
  Reviews verification pending
- **Evidence branch:**
  [`higress-group/higress@main`](https://github.com/higress-group/higress/tree/main)
- **Template verified:** 2026-07-21 against `cncf/toc` `main`
- **Intended TOC snapshot:**
  `projects/higress/governance-review/YYYY-MM-DD.md`

This is the project-maintained working copy. CNCF reviewers may revise the
status and findings before a dated snapshot is archived in `cncf/toc`.
The reviewer should freeze evidence links to the reviewed revision when that
snapshot is archived.

## Summary and Assessment

**Status: Mostly Satisfactory**

Higress has discoverable governance, maintainer, code-owner, contribution,
community, security, and Code of Conduct documents. The project defines
project-wide maintainer responsibility, delegated code ownership, subproject
scope, vendor-neutral decisions, maintainer lifecycle, communication channels,
and security-response roles. The maintainer list includes seven publicly active
people affiliated with four organizations.

The documentation and public evidence satisfy all Governance Review criteria
except one Incubation-required item: Higress does not currently publish an
up-to-date public meeting scheduler or integrate its meetings with the CNCF
calendar. The project should establish that real public meeting infrastructure
before asserting that the Governance Review has no remaining Must-Fix item.

### Executing the Assessment

The self-assessment reviewed the following repository evidence as it exists on
2026-07-21:

- [`GOVERNANCE.md`](https://github.com/higress-group/higress/blob/main/GOVERNANCE.md)
- [`MAINTAINERS.md`](https://github.com/higress-group/higress/blob/main/MAINTAINERS.md)
- [`CODEOWNERS`](https://github.com/higress-group/higress/blob/main/CODEOWNERS)
- [`CONTRIBUTING_EN.md`](https://github.com/higress-group/higress/blob/main/CONTRIBUTING_EN.md)
- [`CODE_OF_CONDUCT.md`](https://github.com/higress-group/higress/blob/main/CODE_OF_CONDUCT.md)
- [`COMMUNITY.md`](https://github.com/higress-group/higress/blob/main/COMMUNITY.md)
- [`SECURITY.md`](https://github.com/higress-group/higress/blob/main/SECURITY.md)
- [`README.md`](https://github.com/higress-group/higress/blob/main/README.md)
- Git history for the governance, maintainer, and ownership files

The review distinguishes documented policy from observed repository evidence.
It does not infer private processes or settings that are not publicly recorded.

### Must-Fix Items

The following issues need to be resolved before the Higress Incubation
application asserts completion of its Governance Review:

1. Publish an up-to-date public meeting scheduler and/or integrate the project
   meetings with the CNCF calendar.

### Points of Excellence

- The maintainer roster is public and shows affiliation diversity across
  Alibaba Cloud, Trip.com, XinYe Technology, and NVIDIA, with linked public
  activity evidence for every listed maintainer.
- Governance distinguishes delegated code ownership from project-wide
  maintainer authority and defines role and subproject lifecycle processes.
- The project publishes an authoritative channel inventory and separates
  public project decisions from narrowly scoped confidential reporting.
- The named Security Response Team has defined incident roles, independent
  review, conflict handling, and escalation.
- Governance explicitly protects vendor-neutral direction through individual
  seats, equal voting weight, and conflict disclosure.

### Areas for Improvement

The following are non-blocking at Incubation but would improve long-term
governance maturity:

- Record governance evolution and link decisions to issues or pull requests.
- Publish examples demonstrating the maintainer lifecycle in practice.
- Add objective progression expectations for appointment to the code-owner
  role.
- Extend `CODEOWNERS` or equivalent ownership records to each active
  subproject and periodically audit coverage.
- Track contributor growth and recruitment trends using the linked CNCF
  DevStats metrics, not only point-in-time activity.

---

## Review

This review audits the project's current governance evidence. The project's
Incubation application has not yet been opened; this file is the project's
self-assessment to accompany that application.

### Governance Summary

Higress uses maintainer-led lazy consensus. Significant decisions are recorded
in public issues or pull requests. When consensus cannot be reached,
non-conflicted maintainers may vote publicly and a simple majority of votes
cast decides. Governance defines contributor, code-owner, and maintainer
authority; the maintainer and subproject lifecycles; vendor neutrality; and
function-based teams. Confidential security work is handled by a named
Security Response Team under a public mandate.

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

**Governance reflects actual activities — Incubating: Suggested — Satisfied.**

Public issue/PR collaboration and lazy consensus are consistent with the
repository workflow. Code-owner authority, security-team operation, and the
absence of a current recurring public meeting are documented without claiming
unobserved processes.

**Vendor-neutral direction — Incubating: Suggested — Satisfied.**

Governance states that roles are held by individuals, prohibits guaranteed
employer seats, vetoes, or preferred decision weight, requires material
conflict disclosure, and evaluates contributions and integrations on community
and technical merit.

### Decisions and Role Assignments

**Leadership, contribution, CNCF, governance, and goal decisions — Incubating:
Suggested — Satisfied.**

Governance applies public lazy consensus and non-conflicted maintainer voting
to leadership, contribution acceptance, goals, requests to CNCF, subproject
scope, and governance changes. The contribution guide and `CODEOWNERS` provide
the day-to-day change-review path.

**Function-based teams — Incubating: Suggested — Satisfied.**

Governance defines the creation and retirement requirements for function-based
teams. `SECURITY.md` names the Security Response Team and defines onboarding,
offboarding, incident assignments, conflicts, independent review, and
escalation.

### Maintainers and Maintainer Lifecycle

**Complete lifecycle — Incubating: Suggested — Satisfied.**

The project documents nomination, public activity review, voluntary departure,
inactivity, removal, emeritus status, and return.

**Lifecycle demonstrated — Incubating: Suggested — Not demonstrated.**

The maintainer file has only one introducing commit in the available history.
No public example of adding, replacing, or moving a maintainer to emeritus is
linked.

**Names, contact, responsibility, affiliation — Incubating: Required —
Satisfied.**

The roster provides names, GitHub contacts, project-wide responsibility
domains, and affiliations for all maintainers.

**Appropriate number of active maintainers — Incubating: Required —
Satisfied.**

Seven maintainers cover the project and its four subprojects. The roster
defines active status and annual public review, and its 2026 review links
public issue and pull-request activity for every listed maintainer during the
preceding 12 months.

**Maintainers from at least two organizations — Incubating: N/A, but met.**

The roster names four affiliations.

### Ownership

**Code and documentation ownership matches governance roles — Incubating:
Required — Satisfied.**

Governance defines code owners as delegated path reviewers, documents their
authority and public appointment/removal process, and makes clear that code
ownership neither grants project-wide governance authority nor limits a
maintainer's project-wide responsibility. This matches `CODEOWNERS`, which
includes maintainers and additional area reviewers.

### Code of Conduct

**Adoption and adherence — Incubating: Required — Satisfied as documented.**

The project adopts the CNCF Code of Conduct in `CODE_OF_CONDUCT.md` and links it
from the README and governance.

**Cross-link from governance — Incubating: Required — Satisfied.**

`GOVERNANCE.md` links both the CNCF Code of Conduct and the project copy.

### Subprojects

**All subprojects listed — Incubating: Required — Satisfied.**

Governance identifies the primary repository and lists `higress-console`,
`higress-standalone`, `plugin-server`, and `wasm-go` as active subprojects. It
also distinguishes upstream forks, integrations, examples, websites, and
other organization repositories from the CNCF project scope.

**Subproject lifecycle — Incubating: Suggested — Satisfied.**

Governance applies the project-wide maintainer roster and public decision
process to subprojects, records each repository's responsibility and status,
and requires leadership, contribution, communication, and lifecycle details
when a subproject is added, removed, transferred, or archived.

### Contributors and Community

**Contributor ladder — Incubating: Suggested — Partially satisfied.**

Governance defines contributor, delegated code-owner, and project-wide
maintainer roles. The contribution and maintainer documents define how
contributors participate and how sustained contributors progress to
maintainership; code-owner assignments are changed publicly through
`CODEOWNERS`. Objective progression expectations for the intermediate
code-owner role can be made more explicit.

**Issue and change submission — Incubating: Required — Satisfied.**

`CONTRIBUTING_EN.md`, issue templates, and the pull-request template document
the process.

**At least one public communication channel — Incubating: Required —
Satisfied.**

GitHub Issues, pull requests, and Discord are publicly documented.

**All public/private channels documented — Incubating: Required — Satisfied.**

`COMMUNITY.md` is the authoritative inventory for project and subproject GitHub
channels, Discord, mailing and localized community channels, documentation,
and narrowly scoped private security and Code of Conduct reporting. It states
that informal or employer-internal conversations are not project decision
channels and requires public decision records.

**Public meeting scheduler/CNCF calendar — Incubating: Required — Not
satisfied.**

No current public meeting scheduler or CNCF calendar integration is linked from
the reviewed repository documents.

**Contribution documentation — Incubating: Required — Satisfied.**

The contribution guide covers issues, pull requests, branches, commits, tests,
style, and AI-assisted contribution requirements.

**Contributor activity and recruitment — Incubating: Required — Satisfied.**

`COMMUNITY.md` links continuously updated GitHub contributor and repository
activity, CNCF DevStats, active-maintainer evidence, `help wanted` and `good
first issue` recruitment queues, the contribution path, and public mentoring
channels.
