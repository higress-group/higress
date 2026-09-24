<!--
Please read the contributing guidelines before submitting this pull request.
Complete every applicable field. For each non-applicable category, write N/A
and explain why; one explanation may cover multiple fields when it is clear.
-->

## I. Summary

<!-- What changed, why, and any compatibility or migration impact? -->


## II. Related issue

<!-- Use "Fixes #123" when this PR closes an issue, or explain N/A. -->


## III. Change type

<!-- Check all that apply. -->

- [ ] Bug fix
- [ ] Feature or enhancement
- [ ] Documentation or configuration only
- [ ] Refactoring, tests, or maintenance


## IV. Agent participation and issue-spec gate

See the [agent-assisted contribution policy](https://github.com/higress-group/higress/blob/main/docs/developers/agent-assisted-contributions.md).
Materially agent-assisted PRs that miss the gate receive low review priority,
and timely maintainer review is not guaranteed; this author declaration is the
deterministic prioritization signal.
Choose exactly one participation declaration:

- [ ] **No agent participation:** This PR is human-only.
- [ ] **Trivial agent-use exception:** Agent use was limited to spelling,
      punctuation, whitespace, or formatting with no substantive choice or
      behavioral effect; the explanation is below.
- [ ] **Verified maintainer/administrator exception:** Material agent
      participation occurred, and authenticated `gh` verification confirmed
      a `role_name` of `maintain` or `admin` on `higress-group/higress`, and the
      verified login matches this PR's GitHub author; record the required
      evidence and rationale below.
- [ ] **Material agent participation:** An AI or coding agent materially
      participated in analysis, design, implementation, testing, or PR
      preparation.

For material participation that does not use the verified maintainer/administrator
exception, check each timed obligation:

- [ ] **Before implementation began:** A Higress maintainer had approved the
      Proposal and Design Issues, and authorized implementation TASKs linked the
      work to the applicable SPECs.
- [ ] **Before verification began:** The Design contained a concrete
      Verification Plan.
- [ ] **Before requesting maintainer review or acceptance:** Applicable
      implementation and verification TASKs were `done` with exact commands,
      results, evidence links, and hashes.

Then choose exactly one overall gate status:

- [ ] Complete: all three timed obligations above were satisfied.
- [ ] Noncompliant: one or more obligations were not satisfied; explain below.
- [ ] Verified exception: material agent participation used the
      verified-maintainer/administrator exception; provide the required evidence
      and rationale below.
- [ ] N/A because the PR is human-only or qualifies for the trivial exception.

**Gate-status explanation (or N/A):**

**Trivial-exception explanation (or N/A):**


**Verified maintainer/administrator exception evidence (or N/A):**
<!-- Required when the verified exception is checked; otherwise write N/A. Run the policy's commands with GH_TOKEN and GITHUB_TOKEN unset after this PR is created but before requesting review. Record this PR's number or URL, the authenticated login, returned `role_name` of `maintain` or `admin`, and the author returned by `gh pr view`; the two logins must match. Explicitly attest that both token override variables were unset for every verification command. Never paste a token. The accepting or merging maintainer independently validates the current live evidence. -->

- This PR's number or URL:
- Authenticated `gh` login:
- Canonical-repository `role_name`:
- Actual PR author from `gh pr view`:
- Login/author match:
- Token override attestation (`GH_TOKEN` and `GITHUB_TOKEN` were unset for every verification command; required: `yes`):
- Bypass rationale:
- Maintainer live validation (review URL or N/A until performed):


**Approved Proposal Issue URL (or N/A with explanation):**


**Approved Design Issue URL (or N/A with explanation):**


**Maintainer approval link(s) (or N/A with explanation):**


**Authorized implementation TASK link(s) (or N/A with explanation):**


**Verification Plan and verification TASK/evidence link(s) (or N/A with explanation):**


## V. Testing and verification

**Test and deterministic rerun commands (or N/A with explanation):**

```text

```

**Environment and configuration (or N/A with explanation):**
<!-- Include OS/architecture and relevant Kubernetes, kind/k3s, Envoy, Go, Rust, or other tool versions. -->


**Exact tested revision, version, image tag/digest, manifests, and values (or N/A with explanation):**


**Baseline reproduction evidence for bug fixes (or N/A with explanation):**
<!-- Link affected/pre-fix results using the same inputs and configuration as the fixed run. -->


**Fixed-version verification evidence for bug fixes (or N/A with explanation):**
<!-- Link expected/actual results, assertions, logs/metrics, and artifact hashes. -->


**Wasm source revision and SHA-256 (or N/A with explanation):**


## VI. Special notes for reviewers

<!-- Call out risk, compatibility, limitations, follow-up work, or review focus. -->


## VII. AI coding disclosure

<!--
Required whenever an AI or coding agent was used, including the trivial-use
exception. Human-only PRs should write N/A. Do not include credentials or
sensitive data.
-->

**Tool(s) used (or N/A):**


### Prompts or instructions

<!-- Paste or link the prompts/instructions given to the tool, with secrets removed. -->


### AI-assisted work summary

<!-- Include key decisions, major changes, and important considerations or limitations. -->
