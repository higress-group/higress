# Investigation Report Template

Use this structure when starting a new deep investigation. Fill in the brackets; remove sections that don't apply. The goal is a report that a maintainer can audit end-to-end: every claim has a citation, every fix has a side-effect analysis.

---

# Investigation: <one-line bug summary>

> Source: #ISSUE_NUMBER
> Investigator: <name / agent session>
> Date: YYYY-MM-DD
> Status: <in-progress / root-caused / fix-designed / fixed>

## Symptom

<What observable behavior is wrong? Include:>

- **Observable**: <what the user sees — CPU spin, crash, wrong response, hang>
- **Trigger conditions**: <minimal repro path>
- **perf / stack data**: <paste perf sampling, stack trace, or relevant telemetry>
- **Blast radius**: <which versions, which components, how many users>

## Code surfaces

<Which repos and files are involved? Use a table:>

| Concern | Repo | File | Why it matters |
| --- | --- | --- | --- |
| <concern> | <repo URL> | <path:line> | <one-line role> |

<For Wasm bugs, always check the envoy fork's `bazel/repository_locations.bzl` for the pinned proxy-wasm-cpp-host commit. Note it here.>

## Worker dispatch

<If you dispatched parallel workers, list them with their brief and key finding:>

- **Worker A (<repo>) — <one-line brief>**: <VERDICT — one-line finding>. Key evidence: <file:line>.
- **Worker B (<repo>) — <one-line brief>**: ...

<If the investigation was single-threaded (no workers), say so and explain why.>

## Root cause

<One-paragraph explanation. Be specific about the mechanism.>

<Numbered call-chain trace, one hop per line:>

1. <Function> (`repo file.cc:NN`) — <what it does>
2. <Function> (`repo file.cc:NN`) — <what it does>
3. ...

<For queue/state bugs, include a state-trace table here. See `code-trace-verification.md` for the format.>

**Root cause (one sentence)**: <X happens because Y, which is unguarded because Z>.

## Fix

<The primitive. Code snippet if non-trivial.>

```cpp
// Before
<current code>

// After
<fixed code>
```

**Why it fixes the bug**: <causal chain, 2-3 sentences.>

**Side effects**: <what else the change touches. Be honest — even a 5-line fix can have ABI or lifecycle implications. If there are none, say "none identified" and explain the verification.>

## Coverage analysis

<Does the fix cover related bug classes? Check upstream issues, related callbacks, similar patterns.>

| Bug class | Covered? | Why |
| --- | --- | --- |
| <primary bug> | yes | <mechanism> |
| <related class 1> | yes/no | <mechanism or reason> |
| <related class 2> | yes/no | ... |

<For each "no", note where it's tracked (separate issue, deferred SPEC, etc.).>

## Evidence index

<All file:line citations grouped by claim. This is the audit trail.>

| Claim | Citation |
| --- | --- |
| <claim> | `repo/path/file.cc:NN` |
| ... | ... |

## Open questions

<Any unresolved questions for the maintainer. Each should be specific and actionable.>

- **Q1**: <question>
- **Q2**: <question>

## Next steps

<What happens next. Be concrete:>

- [ ] <action 1>
- [ ] <action 2>

---

<After the investigation concludes, distill the meta-lessons and add a pattern file to `patterns/`. See `patterns/README.md` for the format.>
