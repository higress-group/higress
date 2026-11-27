# Frozen hotfix rework RED baseline

Source commit: `cbe73567ab2e2092761d169c3c9418e9a5d7165e` in
`higress-group/higress`, before the H01-H04 rework on PR #5400.

These fixtures preserve the historical failures without requiring the old
commit to be reachable after a squash merge or present in a shallow checkout.
They are test inputs only, not active workflows or release authorization.

| Fixture | Source | SHA-256 of exact file bytes |
| --- | --- | --- |
| `promote-plugin-release.yaml` | `.github/workflows/promote-plugin-release.yaml` at the source commit | `25641862a33c4e09c74404c7a2f1a1739d55f3edc36ee672d29a5f5437e1b518` |
| `cpp-hotfix-release-contract.yaml` | `.github/workflows/cpp-hotfix-release-contract.yaml` at the source commit | `9c29a4245862eaa87f5832966c538f5bdccdba0a68c4095bd250302bd278a8cd` |
| `delivery-files.txt` | `git ls-tree -r --name-only cbe73567ab2e2092761d169c3c9418e9a5d7165e -- delivery`, including the final newline | `17282997eec2bb8d52ff33a398612b543977f5afbb570869385666aba8838e53` |
| `SOURCE.md` | This provenance document | Pinned as `hotfixReworkSourceSHA256` in `hotfix_promotion_step_test.go` and recorded in the untracked `REBASE-FIX-HANDOFF.md` |

All four complete-file SHA-256 values are frozen as test constants and checked
before use. This document's own full-byte digest is stored outside itself to
avoid a self-referential checksum; the table records the three payload hashes.

The YAML files are byte-for-byte copies made with `git show` against the locally
available source object, retaining its existing Apache-2.0 headers. The delivery
manifest contains exactly the three tracked paths, one per line. Markdown and
these testdata paths use the repository's existing license-check exclusions;
the checker configuration is unchanged.

- H01: the old promotion workflow instantiates the hotfix environment and lacks
  the early input guard.
- H02: execute both historical verification step bodies with isolated spies to
  reproduce mismatched arguments, then compare against the current GREEN bodies.
- H03: the old contract workflow accepts the floating plugin-server `main` ref.
- H04: the source tree tracks `delivery/HANDOFF.json`, `delivery/PROGRESS.json`,
  and `delivery/RESULT.md`; assert these exact names, not only their count.

Current-tree guards, exact-SHA gates, negative write spies, the nine-version
journal, no-latest checks, and untracked-delivery checks remain independent.
Never execute these frozen workflows as CI definitions or publish from them.
