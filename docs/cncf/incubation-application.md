# Higress CNCF Incubation Application — Working Draft

This working draft follows the CNCF TOC
[Project Incubation Application v1.6](https://github.com/cncf/toc/blob/main/.github/ISSUE_TEMPLATE/template-incubation-application.md),
verified on 2026-07-21. It is not ready to file while any item labelled
**BLOCKED** below remains unresolved.

## Review Project Moving Level Evaluation

- [ ] I have reviewed the TOC moving-level readiness triage guide, ensured the
  criteria are met before opening the issue, and understand that unmet criteria
  will result in closure.

This box remains unchecked because the public meeting, access-control evidence,
OpenSSF Passing badge, adopter interviews/verification, and remaining
vendor-neutral resource work are incomplete.

## Project information

- **Project repositories:**
  [`higress`](https://github.com/higress-group/higress),
  [`higress-console`](https://github.com/higress-group/higress-console),
  [`higress-standalone`](https://github.com/higress-group/higress-standalone),
  [`plugin-server`](https://github.com/higress-group/plugin-server), and
  [`wasm-go`](https://github.com/higress-group/wasm-go)
- **Project site:** <https://higress.ai/en/>
- **Subprojects:** `higress-console`, `higress-standalone`, `plugin-server`,
  and `wasm-go`; authoritative scope is in
  [`GOVERNANCE.md`](https://github.com/higress-group/higress/blob/main/GOVERNANCE.md)
- **Related projects:** confirm before submission whether any project point of
  contact operates a technically related project in another foundation
- **Communication:**
  [`COMMUNITY.md`](https://github.com/higress-group/higress/blob/main/COMMUNITY.md)
- **Project points of contact:** Yuanxiao Zhao and Yiquan Dong; **TODO:** confirm
  direct contact emails before filing (public community contact:
  [higress@googlegroups.com](mailto:higress@googlegroups.com))

## Incubation Criteria Summary

### Application Level Assertion

- [x] Higress is currently Sandbox, accepted on 2026-03-15, and is applying to
  Incubation.
- [ ] Higress is applying to join CNCF directly at Incubation level. Not
  applicable; remove this option from the filed issue.

### Adoption Assertion

Public production adopters in
[`ADOPTERS.md`](https://github.com/higress-group/higress/blob/main/ADOPTERS.md)
include Ant Digital, Kuaishou, Trip.com, Vipshop, and Labring.

- [ ] **BLOCKED — Adopter interviews:** submit 5–7 willing adopters through the
  official CNCF Adopter Interview Questionnaire before DD assignment. This
  requires adopter consent and current contact details; a repository edit
  cannot complete it.

## Application Process Principles

### Suggested

- [ ] Engage with the appropriate domain-specific TAG(s) to present the
  technical architecture. This is suggested, not a filing prerequisite in
  v1.6. A future presentation should be linked here if completed.

### Required

- [x] Complete a General Technical Review. The project self-assessment is in
  [`general-technical-review.md`](https://github.com/higress-group/higress/blob/main/docs/cncf/general-technical-review.md);
  maintainer approval and CNCF Project Reviews verification are requested.
- [ ] Complete a Governance Review. The self-assessment is in
  [`governance-review.md`](https://github.com/higress-group/higress/blob/main/docs/cncf/governance-review.md),
  but its public-meeting Must-Fix remains open.
- [ ] **BLOCKED — All project metadata and resources are vendor-neutral.**
  Governance and the primary README now document vendor-neutral direction and
  no longer promote a single commercial product, but release images remain
  hosted only on Alibaba Cloud registry domains, the historical Go module path
  contains `github.com/alibaba`, and the website roadmap still includes a
  single-vendor enterprise mapping. The application needs a maintainer/CNCF-
  reviewed compatibility and infrastructure plan.
- [x] Review and acknowledgement of Sandbox expectations and maturity-level
  requirements. Higress was accepted as a Sandbox project on 2026-03-15 and
  this application explicitly acknowledges the current requirements.
- [ ] Due Diligence Review. This is completed through CNCF review, resolution
  of concerns, and public comment after a ready application is filed.
- [x] Appropriate installation, end-user, reference, and sample documentation
  is linked from the project README and website.

## Governance and Maintainers

### Suggested

- [ ] Complete the Governance Review with the CNCF Project Reviews subproject;
  external verification is pending.
- [x] Governance is discoverable and version controlled.
- [x] Governance documents actual public decision, leadership, role, and
  security-team processes without claiming a recurring meeting that does not
  exist.
- [x] Governance documents vendor-neutral project direction and conflicts.
- [x] Leadership, contribution, CNCF request, governance, goal, and subproject
  decisions use a public process.
- [x] Function-based team assignment, onboarding, removal, conflicts, and
  retirement are documented.
- [x] The complete maintainer lifecycle is documented.
- [ ] Demonstrate a completed maintainer lifecycle event. Suggested; no public
  addition, replacement, or emeritus transition is yet linked.
- [x] Subproject scope, leadership model, contribution, status, and lifecycle
  are documented.

### Required

- [x] Current maintainers include names, GitHub contact, responsibility domain,
  and affiliation in
  [`MAINTAINERS.md`](https://github.com/higress-group/higress/blob/main/MAINTAINERS.md).
- [x] Seven active maintainers are appropriate to the primary repository and
  four subprojects; the annual activity review links public evidence for each.
- [x] Code and documentation ownership matches the documented code-owner and
  maintainer roles.
- [x] The CNCF Code of Conduct is adopted and linked.
- [x] The Code of Conduct is cross-linked from governance.
- [x] All current subprojects are listed in governance.

## Contributors and Community

### Suggested

- [x] Contributor, code-owner, and maintainer roles form a contributor ladder;
  objective code-owner progression criteria remain an improvement item.

### Required

- [x] Issue and change submission are documented.
- [x] Public GitHub and Discord communication channels are documented.
- [x] All official project, subproject, and narrowly scoped non-public channels
  are inventoried in `COMMUNITY.md`.
- [ ] **BLOCKED — Public meeting scheduler/CNCF calendar.** Higress does not
  currently publish an up-to-date public meeting scheduler or integrate its
  meetings with the CNCF calendar. A real schedule or calendar integration must
  be established rather than represented by documentation alone.
- [x] Contribution documentation is maintained.
- [x] Contributor activity and recruitment are demonstrated through GitHub,
  DevStats, contribution labels, and maintainer activity links.

## Engineering Principles

### Suggested

- [x] The roadmap change process is documented in
  [`ROADMAP.md`](https://github.com/higress-group/higress/blob/main/ROADMAP.md).
- [x] Regular release history is public in GitHub Releases and versioned
  release notes.

### Required

- [x] Project goals, differentiation, need, and cloud-native use cases are
  documented in the README and GTR.
- [x] What Higress does and why it exists are documented in the README and GTR.
- [x] A maintained public roadmap and change process are linked from
  `ROADMAP.md`.
- [x] Architecture and software design are documented in
  [`docs/architecture.md`](https://github.com/higress-group/higress/blob/main/docs/architecture.md)
  and the GTR.
- [x] The release process is documented in
  [`RELEASE.md`](https://github.com/higress-group/higress/blob/main/RELEASE.md).

## Security

### Suggested

- [ ] Complete a joint assessment with CNCF TAG Security and Compliance. This
  is suggested, not a v1.6 filing prerequisite.

### Required

- [x] Vulnerability reporting is documented in
  [`SECURITY.md`](https://github.com/higress-group/higress/blob/main/SECURITY.md).
- [ ] **BLOCKED — Enforced repository access-control evidence.** Public files
  do not prove organization 2FA, least-privilege team access, protected default
  branch/rulesets, required non-author review, or protected release
  environments. An organization owner must verify/configure these settings and
  publish reviewable evidence appropriate for CNCF DD.
- [x] Named security-response membership, incident roles, two-person review,
  conflicts, report handling, and escalation are documented.
- [x] The Security Self-Assessment is documented in
  [`security-self-assessment.md`](https://github.com/higress-group/higress/blob/main/docs/cncf/security-self-assessment.md).
- [ ] **BLOCKED — OpenSSF Best Practices Passing badge.** The public entry is
  currently 96%. Daily/main-push CodeQL is addressed by this preparation work,
  but warning enforcement/remediation, confirmed CodeQL alert disposition, and
  project-level dynamic analysis remain before the badge can be certified as
  Passing.

## Ecosystem

### Required

- [x] The public adopter list records organizations, contacts, environment, and
  use cases.
- [x] At least three independent adopters report production use; five are
  publicly listed.
- [ ] **BLOCKED — TOC adopter verification.** Complete the official interviews
  and allow CNCF/TOC reviewers to verify the adopter evidence.
- [x] Integrations and compatibility with Kubernetes, Gateway API, Envoy,
  Istio, Prometheus/OpenTelemetry, OCI, service registries, and model providers
  are documented in the GTR, architecture, README, and user documentation.

## Current filing decision

**Do not file yet.** The top-level readiness attestation would be false while
the following required items remain open:

1. a real public meeting scheduler and/or CNCF calendar integration;
2. enforced repository access-control evidence;
3. OpenSSF Best Practices Passing, including alert disposition and dynamic
   analysis;
4. vendor-neutral resource/website remediation or an accepted migration plan;
5. 5–7 adopter interview submissions and later TOC verification; and
6. direct project point-of-contact emails confirmed for the issue.
