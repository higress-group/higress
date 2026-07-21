# Higress Community

This document is the authoritative inventory of official Higress project
communication channels, including channels used by its subprojects. Official
technical and governance decisions are recorded in public GitHub issues,
discussions, pull requests, or published meeting notes.

## Public channels

| Channel | Scope and purpose |
| --- | --- |
| [GitHub Issues](https://github.com/higress-group/higress/issues) | Bug reports, feature requests, and work tracking for the primary repository |
| [GitHub Pull Requests](https://github.com/higress-group/higress/pulls) | Public change proposals, reviews, and decision records |
| [GitHub Discussions](https://github.com/higress-group/higress/discussions) | User questions, ideas, announcements, and longer-form community discussion |
| [Discord](https://discord.gg/tSbww9VDaM) | Public real-time user and contributor chat; decisions arising there must be recorded on GitHub |
| [Higress mailing list](mailto:higress@googlegroups.com) | Community questions and contributor contact by email |
| [Chinese-language community group](./README_ZH.md#%E4%BA%A4%E6%B5%81%E7%BE%A4) | Publicly advertised Chinese-language user and contributor chat |
| [Higress WeChat Official Account](./README_ZH.md#%E6%8A%80%E6%9C%AF%E5%88%86%E4%BA%AB) | Chinese-language technical articles and project announcements; broadcast rather than a decision channel |
| [Higress website and documentation](https://higress.cn/en/) | Published user and contributor documentation and project announcements |

Each subproject uses its own public GitHub issues and pull requests for work
specific to that repository:

| Subproject | Issues | Pull requests |
| --- | --- | --- |
| `higress-console` | [Issues](https://github.com/higress-group/higress-console/issues) | [Pull requests](https://github.com/higress-group/higress-console/pulls) |
| `higress-standalone` | [Issues](https://github.com/higress-group/higress-standalone/issues) | [Pull requests](https://github.com/higress-group/higress-standalone/pulls) |
| `plugin-server` | [Issues](https://github.com/higress-group/plugin-server/issues) | [Pull requests](https://github.com/higress-group/plugin-server/pulls) |
| `wasm-go` | [Issues](https://github.com/higress-group/wasm-go/issues) | [Pull requests](https://github.com/higress-group/wasm-go/pulls) |

Cross-subproject and project-governance decisions are recorded in the primary
Higress repository.

## Non-public channels

Non-public channels are limited to reports whose confidentiality protects
reporters or users:

| Channel | Special purpose |
| --- | --- |
| [GitHub Private Security Advisories](https://github.com/higress-group/higress/security/advisories/new) | Confidential vulnerability reporting, triage, remediation, and disclosure coordination under [`SECURITY.md`](./SECURITY.md) |
| [Alibaba Security Response Center](https://security.alibaba.com/) | Optional alternate private intake if a reporter cannot use GitHub; reports are transferred to the project Security Response Team and do not give Alibaba governance authority |
| [CNCF Code of Conduct reporting](mailto:conduct@cncf.io) | Confidential Code of Conduct incident reporting under [`CODE_OF_CONDUCT.md`](./CODE_OF_CONDUCT.md) |
| [CNCF TOC private mailing list](mailto:cncf-private-toc@lists.cncf.io) | Confidential escalation when project leadership cannot resolve a security or governance conflict without conflicted participants |

Personal messages, employer-internal systems, and informal maintainer chats are
not official project decision channels. If they inform project work, the
non-sensitive decision and rationale must be recorded publicly.

## Community meetings

Higress does not currently run a recurring public community meeting and does
not yet have a CNCF calendar entry. Establishing an up-to-date public scheduler
and a durable location for agendas and notes is tracked as an Incubation
readiness item in [`docs/cncf/governance-review.md`](./docs/cncf/governance-review.md).

When a recurring meeting is established, its schedule and joining information
will be published here and on the CNCF calendar. Agendas, notes, recordings,
and decisions will be public, with confidential security and Code of Conduct
matters excluded.

## Contributor activity and recruitment

Public, continuously updated evidence is available through:

- [GitHub contributor activity](https://github.com/higress-group/higress/graphs/contributors)
- [Recent repository activity](https://github.com/higress-group/higress/pulse)
- [CNCF DevStats for Higress](https://higress.devstats.cncf.io/)
- [`help wanted` issues](https://github.com/higress-group/higress/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22)
- [`good first issue` issues](https://github.com/higress-group/higress/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)
- the contributor path in [`CONTRIBUTING_EN.md`](./CONTRIBUTING_EN.md)
- the active-maintainer evidence and annual roster review in
  [`MAINTAINERS.md`](./MAINTAINERS.md)

Contributors can start through an issue, discussion, documentation update, bug
fix, test, plugin, or other pull request. Maintainers and code owners recruit
and mentor contributors through the public channels above and identify
approachable work with contribution labels.
