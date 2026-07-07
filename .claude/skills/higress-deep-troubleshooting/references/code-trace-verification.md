# Code-Trace Verification

The format and rules for verifying claims with code evidence during a deep investigation. Maintainers trust analysis they can audit; file:line citations are the audit trail.

## Core rule

**Every claim must cite `file:line`.** If you can't cite it, you haven't read it; if you haven't read it, you don't know.

## Acceptable evidence

| Claim type | What to cite |
| --- | --- |
| "Function X does Y" | `repo/path/file.cc:NN` + ≤5-line excerpt |
| "Function X is synchronous" | The call site that lacks `dispatcher().post(...)` (or equivalent) + the sibling function that DOES use it, as contrast |
| "The queue state is `[a, b]` at this point" | The sequence of `push_back` / `pop_front` calls leading to that state, each with file:line |
| "The maintainer's claim is wrong" | Re-state their claim, then cite the specific code that contradicts it |
| "This fix covers bug class Y too" | The mechanism by which it covers Y, with the call chain showing why |

## Unacceptable evidence

- "I think" / "probably" / "should be" — read the code.
- "Similar to upstream" — check the fork commit pin in `bazel/repository_locations.bzl`.
- "The function name implies X" — names lie. `injectEncodedDataToFilterChain` sounds innocuous; it's the trigger for a CPU-spin bug.
- Reasoning from the bug report without code verification — the report describes symptoms.

## State-trace table format

The most effective format for communicating queue/state transitions. Use it whenever the bug involves a sequence of state changes (queue operations, `current_context_` saves/restores, reference-counting transitions).

Template:

| Step | Action | `<state variable 1>` | `<state variable 2>` |
| --- | --- | --- | --- |
| 1 | Enter context | `[]` | `nullptr` |
| 2 | Operation X | `[x]` | `nullptr` |
| 3 | Operation Y | `[x, y]` | `ctx` |
| ... | ... | ... | ... |

Rules:

- One row per discrete state change.
- State columns should be the variables that matter for the bug (queue contents, `current_context_`, depth counter, etc.).
- Mark the critical transition with ★ and explain it below the table.
- If a state is "unknown" or "depends on plugin behavior", say so — don't fake precision.

## Worked example (from #4034)

The #4034 investigation hinged on the queue state at the moment of a nested drain. The state-trace table that settled the disagreement:

| Step | Action | `self->after_vm_call_actions_` | outer drain's `local` |
| --- | --- | --- | --- |
| 1 | Enter `onHttpCallResponse` | `[]` | — |
| 2 | Plugin calls `SendHttpResponse` | `[sendLocalReply]` | — |
| 3 | Plugin calls `ResumeHttpRequest` | `[sendLocalReply, continueDecoding]` | — |
| 4 | Outer `~DeferAfterCallActions` fires | same | — |
| 5 | **Layer A: `local.swap(after_vm_call_actions_)`** ★ | `[]` | `[sendLocalReply, continueDecoding]` |
| 6 | iter 1: fire `sendLocalReply` → `encodeHeaders` → `onResponseHeaders` | `[]` | same |
| 7 | Inner `~DeferAfterCallActions` fires | `[]` (empty → no-op) | same |
| 8 | iter 2: fire `continueDecoding` in outer context | `[]` | same |

The ★ at step 5 is the key insight: `std::deque::swap` empties the member, so the inner drain at step 7 sees an empty queue and does nothing. The earlier (wrong) analysis confused "queue before swap" with "queue at inner-drain time".

## Re-verification protocol (when the maintainer pushes back)

When a maintainer disagrees with your conclusion:

1. **Don't restate the conclusion.** Restating without new evidence wastes their time.
2. **Identify the specific point of disagreement.** What exactly do they claim?
3. **Dispatch a fresh worker** with the specific objection. The worker brief should say: "Maintainer claims X. Verify whether X is true. Cite file:line."
4. **If the worker confirms the maintainer**: acknowledge the error publicly, explain what you missed, update artifacts.
5. **If the worker refutes the maintainer**: respond with the specific code evidence, walk through the state-trace table, be respectful.

The third verification (yours) doesn't count if it just restates the second (also yours). You need to actually re-read the code with the maintainer's objection in mind, not just re-derive your previous answer.
