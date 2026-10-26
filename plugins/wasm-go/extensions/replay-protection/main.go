package main

import (
	"fmt"

	"replay-protection/config"
	"replay-protection/util"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/resp"
)

// nonceContextKey 用于在请求头阶段向请求体阶段传递 nonce。
const nonceContextKey = "replay-protection-nonce"

func main() {}

func init() {
	wrapper.SetCtx(
		"replay-protection",
		wrapper.ParseConfig(config.ParseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
	)
}

// onHttpRequestHeaders 处理请求头阶段。
//
// 该阶段只做不依赖 Redis 的快速校验（缺失/长度/base64），
// 然后按请求是否携带 body 分流：
//   - 无 body（如 GET）：ActionPause 在此阶段即可挂住整条请求，
//     直接做 Redis 重放检查。
//   - 有 body（如 POST）：将 nonce 暂存到 ctx，返回 ActionPause
//     暂停 header 流转，交由 onHttpRequestBody 在请求体阶段做
//     Redis 重放检查。请求体阶段是处理请求的最后一道工序，
//     在此挂住才能确保异步回调先于响应到达。
func onHttpRequestHeaders(ctx wrapper.HttpContext, cfg config.ReplayProtectionConfig) types.Action {
	nonce, _ := proxywasm.GetHttpRequestHeader(cfg.NonceHeader)
	if cfg.ForceNonce && nonce == "" {
		// In force mode, reject the request if a required header is missing.
		// Do not return the specific header name in the response.
		log.Warnf("missing nonce header")
		proxywasm.SendHttpResponse(400, nil, []byte("Missing Required Header"), -1)
		return types.ActionPause
	}

	// If there is no nonce, pass through directly (when not in force mode)
	if nonce == "" {
		return types.ActionContinue
	}

	if err := validateNonce(nonce, &cfg); err != nil {
		log.Warnf("invalid nonce: %v", err)
		proxywasm.SendHttpResponse(400, nil, []byte("Invalid Nonce"), -1)
		return types.ActionPause
	}

	// No request body: the header phase is the entire request, ActionPause
	// can hold it until the Redis callback returns.
	if !wrapper.HasRequestBody() {
		ctx.DontReadRequestBody()
		return checkNonceReplay(nonce, &cfg)
	}

	// Has request body: defer the Redis check to the body phase so that
	// ActionPause there can hold the entire request. Stash the nonce for
	// onHttpRequestBody to pick up.
	ctx.SetContext(nonceContextKey, nonce)
	return types.ActionPause
}

// onHttpRequestBody 处理请求体阶段。
//
// 仅当请求头阶段判定请求携带 body 时才会到达此处。取出暂存的 nonce，
// 在请求体阶段发起 Redis 重放检查。请求体阶段是处理请求的最后一道
// 工序，在此 ActionPause 能挂住整条请求直到异步回调返回。
func onHttpRequestBody(ctx wrapper.HttpContext, cfg config.ReplayProtectionConfig, body []byte) types.Action {
	nonce, ok := ctx.GetContext(nonceContextKey).(string)
	if !ok || nonce == "" {
		// No nonce stashed (e.g. request had no nonce and was allowed through
		// in the header phase). Nothing to do.
		return types.ActionContinue
	}
	// Clear the stashed nonce so a potential retry of the body phase
	// does not re-check a stale value.
	ctx.SetContext(nonceContextKey, nil)
	return checkNonceReplay(nonce, &cfg)
}

// checkNonceReplay 使用 Redis SETNX 检查 nonce 是否为重放。
//
// SETNX 是原子操作：key 不存在则写入（首次请求，放行），
// key 已存在则写入失败（重放，拒绝）。该方法发起异步 Redis 调用，
// 调用方必须返回 ActionPause 等待回调，回调内通过 ResumeHttpRequest
// 放行或 SendHttpResponse 拒绝。
//
// Redis 调用失败时采用 fail-open 策略（放行），避免 Redis 不可用
// 导致全部请求被拒绝。
func checkNonceReplay(nonce string, cfg *config.ReplayProtectionConfig) types.Action {
	redisKey := fmt.Sprintf("%s:%s", cfg.Redis.KeyPrefix, nonce)

	err := cfg.Redis.Client.SetNX(redisKey, "1", cfg.NonceTTL, func(response resp.Value) {
		if response.Error() != nil {
			log.Errorf("redis call error: %v", response.Error())
			proxywasm.ResumeHttpRequest()
		} else if response.String() != "OK" {
			log.Warnf("duplicate nonce detected: %s", nonce)
			proxywasm.SendHttpResponse(cfg.RejectCode, nil, []byte(cfg.RejectMsg), -1)
		} else {
			proxywasm.ResumeHttpRequest()
		}
	})

	if err != nil {
		log.Errorf("redis call failed: %v", err)
		return types.ActionContinue
	}
	return types.ActionPause
}

func validateNonce(nonce string, cfg *config.ReplayProtectionConfig) error {
	nonceLength := len(nonce)
	if nonceLength < cfg.NonceMinLen || nonceLength > cfg.NonceMaxLen {
		return fmt.Errorf("invalid nonce length: must be between %d and %d",
			cfg.NonceMinLen, cfg.NonceMaxLen)
	}

	if cfg.ValidateBase64 && !util.IsValidBase64(nonce) {
		return fmt.Errorf("invalid nonce format: must be base64 encoded")
	}

	return nil
}
