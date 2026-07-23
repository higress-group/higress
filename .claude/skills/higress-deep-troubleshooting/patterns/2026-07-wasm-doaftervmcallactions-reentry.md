# Wasm `doAfterVmCallActions` reentry — CPU-spin and stage-mismatch, fixed by drain-to-local

> Source: [#4034](https://github.com/higress-group/higress/issues/4034) — Wasm 插件死循环：onRedisCallFailure + injectEncodedDataToFilterChain 嵌套导致 Envoy CPU 100% 跑满
> Upstream relative: [proxy-wasm/proxy-wasm-cpp-host#326](https://github.com/proxy-wasm/proxy-wasm-cpp-host/issues/326)
> Date: 2026-07
> Status: fix designed (SPEC-001), implementation pending

## Symptom

`release-2.2.2` Envoy pods CPU 100% on all 6 pods within ~3 minutes under concurrent load. Trigger: Redis unreachable + plugin calls `InjectEncodedDataToFilterChain` from an HTTP/Redis callback. perf:

| Overhead | Function |
| --- | --- |
| 39.21% | `Envoy::Extensions::Common::Wasm::Context::onRedisCallFailure` |
| 25.41% | `proxy_wasm::WasmBase::doAfterVmCallActions` |
| **合计** | **64.62%** |

## Root cause

`WasmBase::doAfterVmCallActions` (`higress-group/proxy-wasm-cpp-host` `include/proxy-wasm/wasm.h:158-169`) drains `after_vm_call_actions_` in an unguarded `while(!empty())` loop:

```cpp
while (!self->after_vm_call_actions_.empty()) {
  auto f = std::move(self->after_vm_call_actions_.front());
  self->after_vm_call_actions_.pop_front();
  f();  // if f() re-adds an action, loop never terminates
}
```

Several Envoy-side callbacks re-defer themselves when `current_context_ != nullptr` (meaning: "VM is still on the stack, defer until it unwinds"):

- `Context::onRedisCallFailure` (envoy `context.cc:1273-1277`)
- `Context::onRedisCallSuccess` (envoy `context.cc:1243-1249`)
- `Context::onHttpCallSuccess` (envoy `context.cc:2352-2358`)
- `Context::onHttpCallFailure` (envoy `context.cc:2382-2386`)

Under a synchronous reentry path — `injectEncodedDataToFilterChain` (envoy `context.cc:2085-2091`) is called from inside an HTTP/Redis callback — the outer `SaveRestoreContext` keeps `current_context_` non-null even after the inner `~SaveRestoreContext` unwinds. So the drain loop pops a self-reenqueueing callback, fires it, it sees `current_context_ != nullptr`, re-adds itself, loop iterates again → infinite → CPU 100%.

Full call chain (10 hops, each with file:line) is in [the #4034 proposal issue](https://github.com/higress-group/higress/issues/4064) body and SPEC-001.

## Fix

Swap the member queue to a local deque before iterating, so any action re-added during the drain lands in the (now-empty) member queue and is picked up by the next-outer `DeferAfterCallActions` frame — not re-fired within the same drain.

```cpp
void doAfterVmCallActions() {
  if (!after_vm_call_actions_.empty()) {
    auto self = shared_from_this();
    std::deque<std::function<void()>> local;
    local.swap(self->after_vm_call_actions_);
    for (auto& f : local) f();
  }
}
```

`std::deque::swap` is O(1) whole-content exchange. After swap: `local` has the pre-existing queue, `self->after_vm_call_actions_` is empty. The for-loop iterates `local`; any `addAfterVmCallAction` call during iteration appends to the empty member, invisible to this drain.

## Coverage

The same primitive covers BOTH:

- **#4034 (CPU-spin)**: re-added actions go to the empty member queue → picked up by next outer frame → loop terminates.
- **#326 (stage-mismatch)**: pre-existing actions stay in `local` → inner drain (from a nested `~DeferAfterCallActions`) sees empty member → does nothing → pre-existing actions fire in the outer drain's correct phase.

The coverage of #326 is non-obvious — see Meta-lessons below.

## Evidence

| Claim | Citation |
| --- | --- |
| Unguarded drain loop | `higress-group/proxy-wasm-cpp-host` `include/proxy-wasm/wasm.h:158-169` |
| Self-reenqueue guard `if (current_context_ != nullptr)` | `higress-group/envoy` `source/extensions/common/wasm/context.cc:1273-1277` (onRedisCallFailure); also lines 1243, 2352, 2382 |
| Sync `injectEncodedDataToFilterChain` (the trigger) | `higress-group/envoy` `source/extensions/common/wasm/context.cc:2085-2091` |
| Async sibling `injectEncodedDataToFilterChainOnHeader` (the contrast) | `higress-group/envoy` `source/extensions/common/wasm/context.cc:2093-2103` |
| `sendLocalReply` is deferred via `addAfterVmCallAction` | `higress-group/envoy` `source/extensions/common/wasm/context.cc:1638-1657` |
| `continueDecoding` is deferred via `addAfterVmCallAction` | `higress-group/envoy` `source/extensions/common/wasm/context.cc:1508-1514` |
| `~DeferAfterCallActions` drains unconditionally | `higress-group/proxy-wasm-cpp-host` `src/context.cc` (top, `~DeferAfterCallActions` calls `wasm_->doAfterVmCallActions()`) |
| `SaveRestoreContext` save/restore semantics | `higress-group/proxy-wasm-cpp-host` `include/proxy-wasm/wasm_vm.h:392-405` |
| `current_context_` is thread_local | `higress-group/proxy-wasm-cpp-host` `src/exports.cc:26` |

## Meta-lessons

This investigation had three rounds of analysis. The meta-lessons are the most valuable part for future investigators.

### Lesson 1: a single primitive can cover two bug classes — check coverage explicitly

The first analysis round concluded "Layer A fixes #4034 but NOT #326" and proposed them as separate work items. That was wrong. The `swap-to-local` primitive covers BOTH because:

- For #4034: re-added actions go to the empty member.
- For #326: pre-existing actions stay in `local`, invisible to the nested drain.

**Generalization**: whenever a fix changes how a queue/collection is iterated (swap-to-local, snapshot-then-iterate, copy-on-iterate), check whether it ALSO changes cross-frame or cross-stage visibility. These primitives often cover more than the primary bug.

### Lesson 2: model the PATCHED code, not the unpatched code

When analyzing whether a fix covers a related bug, it's easy to accidentally reason about the current (unpatched) behavior instead of the patched behavior. The first analysis round said "the inner drain sees `[continueDecoding]`" — which is true for the CURRENT code but FALSE for the patched code, because the patch's swap empties the member before the inner drain runs.

**Generalization**: when someone proposes a fix and asks "does this also cover X?", re-read the PATCHED code, not just the diff. The patch changes program state at moments you might not be tracking.

### Lesson 3: "X is deferred" vs "X is direct" is a load-bearing distinction

Several rounds of analysis hinged on whether `sendLocalReply` and `continueDecoding` are called directly or deferred via `addAfterVmCallAction`. Both are deferred — and that determines the queue order that produces the bug.

**Generalization**: when a function's name suggests "does X immediately" (like `sendLocalReply`), don't trust the name. Read the function body and check whether it actually calls X directly or wraps it in `addAfterVmCallAction` / `dispatcher().post(...)` / similar defer primitives. The defer-vs-direct distinction changes everything about reentrancy analysis.

### Lesson 4: maintainer pushback is a signal to re-verify, not restate

When the maintainer challenged the "Layer A doesn't fix #326" conclusion, the right move was to re-read the code with their specific objection in mind — not to restate the conclusion with different words. The re-read found that the swap-to-local empties the member queue, which the first analysis had missed.

**Generalization**: maintainer pushback is high-signal information. They know the codebase. If they say "I think X", dispatch a fresh worker to verify X with code evidence. Don't confirm your own previous answer; that's confirmation bias.

### Lesson 5: the #326 reproducer's plugin call order matters

The #326 reproducer calls `SendHttpResponse` THEN `ResumeHttpRequest`. If the order were reversed, the queue would be `[continueDecoding, sendLocalReply]`, FIFO drain would pop `continueDecoding` first (in the correct phase), then `sendLocalReply` (triggering encoder reentry with empty queue) — and there would be no stage mismatch even WITHOUT Layer A.

**Generalization**: when analyzing a reproducer, note the exact order of plugin calls. Stage-mismatch bugs often depend on call order, and a "harmless" reorder can mask the bug.
