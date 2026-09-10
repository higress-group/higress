# replay-protection 插件修复带请求体请求的重放漏放

## 问题

`replay-protection` 插件仅在请求头阶段（`ProcessRequestHeaders`）发起 Redis `SETNX` 异步调用并返回 `ActionPause` 等待回调。对于携带请求体的请求（如 POST），请求头阶段的 `ActionPause` 无法挂住后续请求体阶段，导致 Redis 回调常常在请求已被放行之后才到达，重放请求未被拦截（应返回 429，实际放行为 200）。

无请求体的请求（如 GET）因请求头阶段即请求的全部，`ActionPause` 能挂住请求直到回调返回，重放拦截正常。

详见 issue #4601。

## 修复

按请求有无 body 分阶段处理：

1. 额外注册请求体阶段处理器 `ProcessRequestBody(onHttpRequestBody)`。
2. 请求头阶段只做不依赖 Redis 的快速校验（缺失/长度/base64），然后：
   - 无 body 请求（`!wrapper.HasRequestBody()`，如 GET）：在请求头阶段直接做 Redis 重放检查。
   - 有 body 请求（如 POST）：将 nonce 通过 `ctx.SetContext` 暂存，返回 `ActionPause` 暂停 header 流转，交由请求体阶段做 Redis 检查。
3. 请求体阶段取出暂存的 nonce，发起 Redis `SETNX` 并返回 `ActionPause` 等待回调。请求体阶段是处理请求的最后一道工序，在此挂住能确保异步回调先于响应到达。
4. 将 Redis 重放检查逻辑抽为公共函数 `checkNonceReplay`，供两个阶段复用。

## 影响范围

- 插件：`plugins/wasm-go/extensions/replay-protection`
- 配置结构不变，无兼容性影响。
- 行为变化：带请求体请求的重放检查从请求头阶段移至请求体阶段，修复重放漏放。
