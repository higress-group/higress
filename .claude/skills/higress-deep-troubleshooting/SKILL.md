---
name: higress-deep-troubleshooting
description: Deep-dive troubleshooting methodology for Higress gateway bugs that require code-trace evidence across the higress-group/envoy and higress-group/proxy-wasm-cpp-host forks — CPU spins, infinite loops, Wasm VM reentrancy, filter-chain stage-mismatch, callback-ordering issues, SaveRestoreContext/current_context_ bugs. Use when investigating a complex Higress/Wasm/proxy-wasm incident that needs root-cause analysis with file:line citations, when ramping up on a reported CPU-spin or crash, or when verifying whether a proposed fix covers related bug classes. Accumulates case-study patterns under patterns/ — add a new pattern after each major investigation.
---

# Higress Deep Troubleshooting

A methodology plus an accumulating pattern library for deep-diving into Higress gateway bugs that require code-level root cause analysis. The codebase spans two Higress-maintained forks — [`higress-group/envoy`](https://github.com/higress-group/envoy) (filter chain, Wasm `Context` integration) and [`higress-group/proxy-wasm-cpp-host`](https://github.com/higress-group/proxy-wasm-cpp-host) (Wasm VM host runtime, `WasmBase`, `ContextBase`) — and bugs often cross the boundary between them. This skill exists so that the next investigation doesn't start from scratch.

## When to use

Reach for this skill when ANY of these apply:

- A Higress issue reports a CPU spin, infinite loop, crash, or stage-mismatched behavior.
- The bug involves the Wasm plugin lifecycle, `addAfterVmCallAction` / `doAfterVmCallActions`, `SaveRestoreContext`, `current_context_`, or the filter chain reentry paths.
- You need to verify whether a one-repo fix also covers an upstream bug class (e.g., `proxy-wasm/proxy-wasm-cpp-host#326`).
- You're ramping up on an incident and need a structured way to collect evidence before proposing a fix.
- The maintainer is pushing back on a conclusion and you need to re-verify with code.

Do NOT reach for this skill for routine plugin development (`higress-wasm-go-plugin` covers that), config issues, or anything solvable by reading a single file without cross-repo context.

## Investigation methodology (brief)

1. **Symptom → code surface.** From perf data / repro steps / stack trace, identify which functions and files are involved. Note which fork each file lives in.
2. **Parallel investigation.** Dispatch focused worker agents for independent aspects (envoy side, proxy-wasm-cpp-host side, upstream references). Each worker returns file:line evidence — never narration.
3. **Code-trace verification.** Every claim must cite `file:line`. No "I think it does X" — only "see envoy context.cc:2085 where it calls ... synchronously".
4. **Synthesize root cause.** Combine worker findings into a numbered call-chain trace, showing program state (queue contents, `current_context_` value, etc.) at each hop.
5. **Fix design + side effects.** Propose a primitive, analyze what else it touches, check if it covers related bug classes upstream.
6. **Verify when challenged.** If the maintainer pushes back, re-verify with code — don't restate the conclusion. The earlier analysis is often wrong in some detail that only a fresh code read catches.

Full process: [`references/investigation-methodology.md`](references/investigation-methodology.md). Verification patterns: [`references/code-trace-verification.md`](references/code-trace-verification.md).

## Accumulated patterns

Each major investigation that uses this skill should add a pattern under `patterns/`. Patterns are the "living" part of this skill — they capture both the technical root cause AND the meta-lessons (what tripped up the analysis, what the maintainer caught, what the fix primitive generalizes to).

Current patterns:

- [`patterns/2026-07-wasm-doaftervmcallactions-reentry.md`](patterns/2026-07-wasm-doaftervmcallactions-reentry.md) — #4034 CPU 100% from `onRedisCallFailure` + `injectEncodedDataToFilterChain` nested reentry. Root cause: unguarded `while(!empty())` drain in `WasmBase::doAfterVmCallActions`. Fix: drain-to-local via `std::deque::swap`. Meta-lesson: the same primitive covers the upstream #326 stage-mismatch class, which the first analysis round missed.

To add a new pattern after an investigation, see [`patterns/README.md`](patterns/README.md).

## Templates

- [`templates/investigation-report.md`](templates/investigation-report.md) — structure for a new investigation's findings.
- [`templates/code-trace-table.md`](templates/code-trace-table.md) — step-by-step state-trace table format (very effective for communicating queue/state transitions to maintainers).

## Key code maps (quick orientation)

When investigating a Wasm-related Higress bug, the usual suspects:

| Concern | Repo | File |
| --- | --- | --- |
| Plugin callback entry points (`onRequestHeaders`, `onResponseBody`, `onHttpCallResponse`, etc.) | `higress-group/proxy-wasm-cpp-host` | `src/context.cc` |
| Deferred-action queue (`addAfterVmCallAction`, `doAfterVmCallActions`, `DeferAfterCallActions`) | `higress-group/proxy-wasm-cpp-host` | `include/proxy-wasm/wasm.h`, `src/context.cc` |
| `SaveRestoreContext` / `current_context_` | `higress-group/proxy-wasm-cpp-host` | `include/proxy-wasm/wasm_vm.h`, `src/exports.cc` |
| Higress envoy `Context` (filter chain integration, foreign functions) | `higress-group/envoy` | `source/extensions/common/wasm/context.cc` |
| Foreign-function registration (`proxy_inject_encoded_data_to_filter_chain`, etc.) | `higress-group/envoy` | `source/extensions/common/wasm/foreign.cc` |
| envoy fork's pinned proxy-wasm-cpp-host commit | `higress-group/envoy` | `bazel/repository_locations.bzl` |

The Higress envoy fork pins a specific `proxy-wasm-cpp-host` commit via bazel — always check which fork commit is pinned before reasoning about `WasmBase` behavior, because the fork diverges from upstream (RedisCall, rebuild/recover, weak_handle additions).
