# Higress Release Process

This document defines the project-wide process for producing a Higress core
release from the primary repository. Subprojects may add repository-specific
steps, but must use the same public approval and security principles.

## Versioning and support

Higress uses [Semantic Versioning](https://semver.org/) with Git tags in the
form `vMAJOR.MINOR.PATCH`. A release-candidate tag may add an `-rc.N` suffix.

- A **major** release may contain incompatible changes and requires migration
  guidance.
- A **minor** release adds backward-compatible features and may contain
  announced deprecations.
- A **patch** release contains backward-compatible fixes, including security
  fixes when appropriate.

The supported major-version lines are listed in [`SECURITY.md`](./SECURITY.md).
A proposal to end support for a major line is made publicly through a pull
request updating `SECURITY.md`, normally at least 90 days before the stated end
of support. Already unsupported versions do not receive routine fixes.

## Roles and authorization

A maintainer acts as **release manager** for each release. The release manager
coordinates the tracking issue, version changes, validation, tag, automation,
and announcement. At least one other unconflicted maintainer reviews and
approves the release pull request. Only maintainers with the required GitHub
and protected-environment access may create release tags or approve publishing
jobs.

Security releases additionally follow [`SECURITY.md`](./SECURITY.md). Embargoed
details stay in the private advisory until the Security Response Team approves
disclosure.

## 1. Plan the release

The release manager opens or identifies a public tracking issue for a normal
release. It records:

- the target version and release type;
- intended scope and linked changes;
- the release manager and expected timing;
- known compatibility, migration, deprecation, and security considerations;
- validation status and unresolved blockers.

An embargoed security release uses a private advisory for planning until public
disclosure is safe.

## 2. Prepare the release pull request

Create a release pull request against `main` that keeps the version in
`VERSION`, `helm/core/Chart.yaml`, `helm/higress/Chart.yaml`, dependency
metadata, and release notes consistent. Update documentation, migration or
deprecation guidance, and supported-version information when they change.

The pull request must pass the repository's required build, unit, race,
conformance, plugin, license, and other configured checks relevant to its
changes. The release manager records any intentionally inapplicable check or
accepted exception in the pull request. A known release-blocking regression,
unresolved critical vulnerability, inconsistent version, or missing migration
guidance blocks the release.

The approving maintainer verifies that the release scope matches the public
plan, required checks passed, versioned dependencies are intentional, and the
release notes accurately describe user-visible changes.

## 3. Tag and publish

After the release pull request is merged, the release manager creates the
corresponding immutable `vMAJOR.MINOR.PATCH` or release-candidate tag from the
reviewed commit. Published tags are not moved or reused.

The tag triggers GitHub Actions that publish, as applicable:

- controller, pilot, and gateway container images;
- Helm charts and chart indexes;
- `hgctl` archives for supported platforms;
- generated CRDs;
- standalone distribution artifacts;
- GitHub release notes and repository release notes.

Manual workflow dispatch is for recovery or a documented exceptional case; it
does not bypass release approval or change the commit being released.

## 4. Verify and announce

The release manager verifies that publishing workflows succeeded and that the
release page, checksums or archives, image tags, Helm charts, CRDs, and release
notes refer to the intended version. Installation and representative gateway
traffic are smoke-tested with the published artifacts before the release is
announced as ready.

Release announcements link the GitHub release and call out breaking changes,
deprecations, migrations, known issues, and security impact. Confirmed security
issues are disclosed through a GitHub Security Advisory and CVE when
appropriate.

## Failure, rollback, and correction

If validation or publishing fails, stop promotion and document the failure in
the tracking issue or release pull request. Do not move or overwrite an
existing public tag. Fix the problem through normal review and publish a new
release-candidate or patch version.

An operator rollback uses the previously pinned Helm chart, configuration, and
images, normally through `helm rollback`. Because CRD schemas and stored
resources may not be reversible, release notes must identify migrations that
need special rollback handling. The project may withdraw a compromised
artifact from distribution, but it preserves a public advisory and audit trail
explaining the affected version and safe replacement.
