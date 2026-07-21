# Higress Governance

Higress is a [Cloud Native Computing Foundation (CNCF)](https://www.cncf.io/)
sandbox project. This document describes the project's open governance model.

All community members must follow the
[**CNCF Code of Conduct**](https://github.com/cncf/foundation/blob/main/code-of-conduct.md)
and the project's adopted policy in [`CODE_OF_CONDUCT.md`](./CODE_OF_CONDUCT.md).

## Values

Higress governance is guided by the following values:

- **Openness**: Communication and decision making happen in public channels and repositories whenever possible.
- **Fairness**: Contributions are evaluated on technical merit rather than company affiliation.
- **Community First**: Long-term community health has priority over short-term product goals.
- **Inclusivity**: We welcome contributors from different regions and backgrounds.
- **Participation**: Project responsibilities are earned through sustained contribution.

## Roles

Higress has the following project-wide roles:

- **Contributor**: anyone who participates through issues, discussions,
  documentation, code, testing, or other community work. Contributors do not
  need a formal appointment.
- **Code owner**: a contributor delegated to review changes in one or more
  paths in [`CODEOWNERS`](./CODEOWNERS). Code owners assess technical quality,
  request changes, and help maintain their assigned area. Code ownership does
  not grant project-wide governance authority or a permanent seat. A code
  owner is added or removed by a public pull request updating `CODEOWNERS`,
  using the decision process below. Maintainers remain accountable for the
  resulting ownership coverage and may merge only in accordance with the
  repository's configured permissions and review rules.
- **Maintainer**: a project-wide leadership role responsible for technical
  direction, releases, governance, community health, and the delegation of
  code ownership. Maintainers are not limited to the paths where they are
  named as code owners.

The current roster, responsibilities, and lifecycle are documented in:

- [`MAINTAINERS.md`](./MAINTAINERS.md)
- [`CONTRIBUTING_EN.md`](./CONTRIBUTING_EN.md)

## Project Scope and Subprojects

The following repositories comprise the CNCF Higress project. Unless a
subproject documents an additional local rule, this governance and the
project-wide maintainer roster apply to it.

| Repository | Responsibility | Status |
| --- | --- | --- |
| [`higress-group/higress`](https://github.com/higress-group/higress) | Core control plane, data plane integration, APIs, Helm charts, CLI, plugins, documentation, and releases | Active; primary repository |
| [`higress-group/higress-console`](https://github.com/higress-group/higress-console) | Web management console | Active subproject |
| [`higress-group/higress-standalone`](https://github.com/higress-group/higress-standalone) | Standalone and local deployment packaging | Active subproject |
| [`higress-group/plugin-server`](https://github.com/higress-group/plugin-server) | Plugin distribution service used by Higress | Active subproject |
| [`higress-group/wasm-go`](https://github.com/higress-group/wasm-go) | Go SDK for Higress Wasm plugins | Active subproject |

Forks of upstream dependencies, integrations, examples, experiments, websites,
and other repositories in the `higress-group` organization are not Higress
subprojects unless they are added to this table.

A proposal to add, remove, transfer, or archive a subproject must be made in a
public issue or pull request. It must identify the repository, purpose,
maintainers or delegated code owners, contribution path, communication
channels, and lifecycle status. The proposal follows the project-direction
decision process below. An archived or removed subproject remains listed here
with its final status and successor, if any.

## Decision Making

Higress uses **lazy consensus** by default.

Technical proposals, contribution acceptance, project goals, leadership and
role assignments, subproject changes, requests made on behalf of Higress to
the CNCF, and governance changes are discussed in public issues or pull
requests. Maintainers are responsible for ensuring that significant decisions
record their rationale and outcome in the relevant public thread.

When consensus cannot be reached, a maintainer may start a vote on the public
issue or pull request. A simple majority of non-conflicted maintainer votes
cast decides the outcome. A maintainer with a material personal or employer
conflict must disclose it and abstain from the decision. If all maintainers are
conflicted, the project will request guidance from the CNCF TOC.

For governance or project direction changes, maintainers should allow adequate
time for public discussion before finalizing decisions.

## Vendor Neutrality

Higress project direction and governance are independent of any single
company. Maintainer and code-owner roles are held by individuals, not reserved
for employers. No employer has a guaranteed seat, veto, or preferred decision
weight. Contributions, integrations, defaults, and roadmap priorities are
evaluated on community and technical merit. Company-specific products may be
supported, but they do not receive privileged governance treatment.

Affiliation changes do not by themselves add or remove a role. Maintainers
disclose affiliations in `MAINTAINERS.md` and disclose material conflicts in
the public decision record.

## Function-Based Teams

Maintainers may create a function-based team, such as a release or security
team, through a public governance pull request. The change must document the
team's purpose, authority, membership or selection method, onboarding,
conflict handling, removal, and retirement. Sensitive operational details may
remain private, but the team's mandate and decision authority must be public.

The current Security Response Team and its private report-handling process are
defined in [`SECURITY.md`](./SECURITY.md).

## Community and Meetings

The authoritative inventory of public and non-public communication channels,
subproject channels, contribution activity, and meeting information is in
[`COMMUNITY.md`](./COMMUNITY.md). Project decisions must be recorded in a
public issue, discussion, pull request, or published meeting notes even when
preliminary conversation occurred elsewhere.

## Governance Updates

Changes to this document are made through pull requests and approved by
maintainers.

## Security

Security reporting and response follow [`SECURITY.md`](./SECURITY.md).

---

Higress is a [Cloud Native Computing Foundation](https://www.cncf.io/) sandbox
project.
