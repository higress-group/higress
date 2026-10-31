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

func main() {}

func init() {
	wrapper.SetCtx(
		"replay-protection",
		wrapper.ParseConfig(config.ParseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
	)
}

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

	redisKey := fmt.Sprintf("%s:%s", cfg.Redis.KeyPrefix, nonce)

	// Check if the nonce already exists
	err := cfg.Redis.Client.SetNX(redisKey, "1", cfg.NonceTTL, func(response resp.Value) {
		if err := response.Error(); err != nil {
			log.Errorf("redis call error: %v", err)
			handleRedisFailure(cfg, true)
			return
		}
		if response.IsNull() {
			log.Warnf("duplicate nonce detected: %s", nonce)
			proxywasm.SendHttpResponse(cfg.RejectCode, nil, []byte(cfg.RejectMsg), -1)
			return
		}
		if response.Type() == resp.SimpleString && response.String() == "OK" {
			proxywasm.ResumeHttpRequest()
			return
		}
		log.Errorf("unexpected redis response type %s", response.Type())
		handleRedisFailure(cfg, true)
	})

	if err != nil {
		return handleRedisDispatchError(cfg, err)
	}
	return types.ActionPause
}

func handleRedisDispatchError(cfg config.ReplayProtectionConfig, err error) types.Action {
	log.Errorf("redis call failed: %v", err)
	return handleRedisFailure(cfg, false)
}

func handleRedisFailure(cfg config.ReplayProtectionConfig, resumeOnOpen bool) types.Action {
	if cfg.FailureMode == config.FailureModeFailClosed {
		proxywasm.SendHttpResponse(503, nil, []byte("Service Unavailable"), -1)
		return types.ActionPause
	}
	if resumeOnOpen {
		proxywasm.ResumeHttpRequest()
	}
	return types.ActionContinue
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
