# Patterns — How to Add a New Investigation Case-Study

This directory accumulates case-studies from past deep investigations. Each pattern captures both the technical root cause AND the meta-lessons (what tripped up the analysis, what the maintainer caught, what the fix primitive generalizes to). The library grows by one entry per major investigation.

## When to add a pattern

Add a pattern after an investigation that:

- Required cross-repo code-trace evidence (envoy + proxy-wasm-cpp-host, or similar).
- Resulted in a non-trivial fix (not just a one-line config change).
- Has a meta-lesson worth carrying forward (a gotcha, a verification failure, a primitive that generalizes).

If the investigation was routine (single-file bug, obvious fix, no pushback), skip the pattern — the diff IS the documentation.

## File naming

`YYYY-MM-short-slug.md` where the date is when the investigation concluded. Examples:

- `2026-07-wasm-doaftervmcallactions-reentry.md`
- `2026-09-filter-chain-headers-double-dispatch.md`
- `2026-11-redis-pool-exhaustion-under-failover.md`

## Pattern structure

Use this template. Sections in `[brackets]` are required; sections in `<angle brackets>` are optional but encouraged.

```markdown
# <Bug class one-line summary>

> Source: #ISSUE_NUMBER — <issue title>
> Date: YYYY-MM
> Status: <fixed / investigating / deferred>

## Symptom

<What observable behavior was wrong? Include perf data / repro steps if relevant.>

## Root cause

<One-paragraph explanation. Cite file:line for key claims.>

<Numbered call-chain trace, one hop per line, with file:line.>

## Fix

<The primitive. Code snippet if non-trivial.>

<Why it fixes the bug — the causal chain.>

## Coverage

<What bug classes does this fix cover? What does it NOT cover? Why?>

## Evidence

<File:line citations grouped by claim.>

## Meta-lessons

<What tripped up the first analysis round? What did the maintainer catch? What generalizes to other bugs in this area?>

<If the analysis was wrong initially and then corrected, document BOTH the wrong reasoning AND the correction — this is the most valuable part for future investigators.>
```

## Worked example

See [`2026-07-wasm-doaftervmcallactions-reentry.md`](2026-07-wasm-doaftervmcallactions-reentry.md) for a complete pattern that follows this structure, including a meta-lesson section documenting an initial wrong conclusion that was corrected after maintainer pushback.

## Linking from new investigations

When a new investigation reuses a primitive or hits a similar gotcha to a past pattern, link to the pattern in the new investigation's report. Patterns are most valuable when they're cross-referenced — the second time you see a `std::deque::swap`-style fix, you should be able to find the #4034 pattern and reason by analogy.
