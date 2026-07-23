# Investigation Methodology

The step-by-step process for deep-diving a Higress bug. This is the canonical workflow; adapt it to the bug at hand but don't skip the evidence-collection steps.

## 0. Before you start: scope the investigation

Confirm the bug is real and worth a deep dive. Signs it's worth it:

- Reproducer exists (even if intermittent).
- perf data or stack trace points at specific functions.
- The fix likely requires code changes (not config/docs).
- The blast radius is significant (production-affecting, multiple users).

If the bug is a one-off without a reproducer, or clearly a user-config issue, don't burn a deep investigation on it.

## 1. Symptom → code surface

Read the bug report and extract:

- **Symptom**: what observable behavior is wrong? (CPU spin, crash, wrong response, hang.)
- **Trigger conditions**: what's the minimal repro path?
- **perf / stack data**: which functions dominate?

Then map symptoms to code surfaces. For Wasm-related bugs, the surface is almost always split across:

- `higress-group/envoy` `source/extensions/common/wasm/context.cc` — Higress-specific `Context` subclass, foreign functions like `injectEncodedDataToFilterChain`.
- `higress-group/proxy-wasm-cpp-host` `include/proxy-wasm/wasm.h` + `src/context.cc` — VM host runtime, `WasmBase`, `ContextBase`, deferred-action queue.

**Always check the envoy fork's `bazel/repository_locations.bzl` for the pinned `proxy-wasm-cpp-host` commit.** The fork diverges from upstream; reasoning about upstream code without checking the pin leads to wrong conclusions.

## 2. Parallel investigation (worker dispatch)

For non-trivial bugs, dispatch focused worker agents IN PARALLEL for independent aspects. Each worker gets a self-contained brief (it has no shared context with you). Example dispatch for a Wasm reentrancy bug:

- **Worker A (envoy side)**: verify the trigger path in `higress-group/envoy`. Find the synchronous call, the self-reenqueueing callbacks, the foreign-function registration. Return file:line evidence.
- **Worker B (proxy-wasm-cpp-host side)**: verify the engine. Find the queue, the drain loop, the `SaveRestoreContext` semantics. Return file:line evidence.
- **Worker C (upstream references)**: fetch any linked upstream issues/PRs, determine if they're related/resolved/portable.

Worker rules:

- Each worker is **research-only** unless you explicitly authorize code changes. Don't let workers edit code on their own initiative.
- Each worker must **cite file:line for every claim**. Reject worker reports that say "the function does X" without a citation.
- Cap report length (e.g., 600-800 words) to keep synthesis tractable.
- Use `general-purpose` agent type for investigations that need to clone repos and reason across files; use `Explore` for narrow single-repo lookups.

## 3. Code-trace verification

Every claim in the final analysis must reduce to a file:line citation. The output format that works best for maintainer communication is the **step-by-step state-trace table**:

| Step | Action | Member queue state | Local queue state |
| --- | --- | --- | --- |
| 1 | Enter callback | `[]` | — |
| 2 | Plugin calls X | `[x]` | — |
| ... | ... | ... | ... |

See [`code-trace-verification.md`](code-trace-verification.md) for the full format and a worked example.

Anti-patterns to avoid:

- **"I think the function does X"** — unacceptable. Read the code and cite the line.
- **"This is probably similar to upstream"** — check the fork commit pin.
- **"The drain loop terminates because of course it does"** — verify the actual loop condition.
- **Reasoning from the bug report without reading the code** — the report describes symptoms, not causes.

## 4. Synthesize root cause

Combine worker findings into a numbered call-chain trace. For each hop:

- The function name.
- The file:line.
- What state it reads/writes (e.g., `current_context_` value, queue contents).

End with a one-sentence root cause: "X happens because Y, which is unguarded because Z."

## 5. Fix design + side effects

Propose a fix primitive. For each candidate:

- **What it changes**: the specific code transformation.
- **Why it fixes the bug**: the causal chain.
- **Side effects**: what else it touches. Be honest — even a 5-line fix can have ABI or lifecycle implications.
- **Coverage**: does it also cover related bug classes? (E.g., drain-to-local covers both CPU-spin and stage-mismatch.)

If a fix has unacceptable side effects, document it as "deferred" and explain why. Don't sneak a behavior-change through as a "bug fix".

## 6. Cross-check related bug classes

Before declaring done, check: does the fix also cover bug classes that share the same machinery?

- Look at upstream issues in `proxy-wasm/proxy-wasm-cpp-host` that reference the same functions.
- Look at the Higress fork's divergence from upstream — are there Higress-specific code paths that have the same bug?
- Look at the `addAfterVmCallAction` callsites in envoy — are there other self-reenqueueing or cross-stage patterns?

Document the coverage explicitly: "Fixes X. Does NOT fix Y because Z. Y tracked separately as ..."

## 7. Verify when challenged

If the maintainer pushes back on a conclusion, **do not just restate the conclusion**. Re-read the code with their specific objection in mind.

Common failure modes in the first analysis round:

- **Confusing "state before operation" with "state after operation"**. (E.g., "the queue is `[X]`" — but is that before or after the swap?)
- **Assuming a function is sync/async without checking**. (E.g., assuming `sendLocalReply` is direct when it's actually deferred via `addAfterVmCallAction`.)
- **Reasoning about the patched code as if it were the unpatched code**. (E.g., analyzing Layer A's `doAfterVmCallActions` as if it still iterates the member queue in place, when actually it swaps to a local first.)
- **Assuming plugin call order**. (E.g., assuming `ResumeHttpRequest` is called before `SendHttpResponse` — verify from the reproducer.)

When you re-verify, dispatch a fresh worker with the specific objection. The worker should be told: "the maintainer claims X; verify whether X is true with code evidence." This avoids confirmation bias.

## 8. Document the pattern

After the investigation concludes, add a pattern file to `patterns/` capturing:

- The bug class (one-line summary).
- The root cause (with file:line evidence).
- The fix primitive (with the code snippet).
- The meta-lesson (what tripped up the analysis, what the maintainer caught, what generalizes).

This is what makes the skill accumulate value over time. See [`patterns/README.md`](../patterns/README.md) for the format.
