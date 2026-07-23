# Code-Trace Table Template

Use this template for the state-trace table that communicates queue/state transitions to maintainers. This format was developed during the #4034 investigation and proved effective for settling disagreements about program state at specific moments.

## When to use

- The bug involves a sequence of state changes (queue operations, `current_context_` saves/restores, reference-count transitions).
- You need to communicate "what the program state is at moment X" to someone who wasn't in your head.
- A maintainer is challenging a conclusion about state at a specific point in the call chain.

## Template

```markdown
| Step | Action | `<state variable 1>` | `<state variable 2>` | ... |
| --- | --- | --- | --- | ... |
| 1 | <entry point> | `<initial value>` | `<initial value>` | ... |
| 2 | <operation> | `<value after>` | `<value after>` | ... |
| 3 | <operation> | `<value after>` | `<value after>` | ... |
| ... | ... | ... | ... | ... |
| N ★ | <critical transition> | `<value>` | `<value>` | ... |
```

## Rules

1. **One row per discrete state change.** Don't combine multiple operations into one row.
2. **State columns are the variables that matter for the bug.** Common ones for Wasm bugs:
   - `self->after_vm_call_actions_` (the deferred-action queue)
   - `current_context_` (the thread_local ContextBase*)
   - `SaveRestoreContext` depth (how many nested VM frames)
   - `local` (the outer drain's swap target, post-Layer-A)
3. **Mark the critical transition with ★** and explain it below the table. This is the moment where the bug happens (or doesn't).
4. **Show the queue contents explicitly** — `[sendLocalReply, continueDecoding]`, not "non-empty". Order matters for FIFO drains.
5. **If state is unknown / depends on plugin behavior, say so.** Don't fake precision. Better: "depends on whether the plugin called X before Y; both branches shown below".
6. **Pair the table with a prose explanation of the ★ transition.** The table shows WHAT; the prose explains WHY.

## Worked example

From #4034 — the table that settled whether Layer A's swap-to-local covers #326:

```markdown
| Step | Action | `self->after_vm_call_actions_` | outer drain's `local` |
| --- | --- | --- | --- |
| 1 | Enter `onHttpCallResponse` | `[]` | — |
| 2 | Plugin calls `SendHttpResponse` | `[sendLocalReply]` | — |
| 3 | Plugin calls `ResumeHttpRequest` | `[sendLocalReply, continueDecoding]` | — |
| 4 | Outer `~DeferAfterCallActions` fires | `[sendLocalReply, continueDecoding]` | — |
| 5 ★ | **Layer A: `local.swap(after_vm_call_actions_)`** | `[]` | `[sendLocalReply, continueDecoding]` |
| 6 | iter 1: fire `sendLocalReply` → `encodeHeaders` → `onResponseHeaders` | `[]` | `[sendLocalReply, continueDecoding]` |
| 7 | Inner `~DeferAfterCallActions` fires | `[]` (empty → no-op) | `[sendLocalReply, continueDecoding]` |
| 8 | iter 2: fire `continueDecoding` in outer context | `[]` | (iterating) |
```

**Why the ★ matters**: `std::deque::swap` is O(1) whole-content exchange. After step 5, the member queue is empty — it's NOT `[continueDecoding]` anymore. So when step 7's inner drain checks `if (!after_vm_call_actions_.empty())`, it sees `false` and returns. `continueDecoding` stays in the outer drain's `local` and fires in step 8, in the outer drain's context (correct phase).

The earlier (wrong) analysis had assumed the member queue still contained `continueDecoding` at step 7 — confusing "queue before swap" with "queue after swap".

## Anti-patterns

Don't do these:

- **Combining steps**: "steps 5-8: drain happens, continueDecoding fires" — too coarse; the reader can't see the state at each moment.
- **Hiding the critical transition**: burying the ★ in a paragraph of prose. The table IS the argument; make it sharp.
- **Vague state**: "queue has some items" — which items? In what order? FIFO drains care about order.
- **No prose explanation**: the table alone isn't enough; pair it with 2-3 sentences explaining the ★.
