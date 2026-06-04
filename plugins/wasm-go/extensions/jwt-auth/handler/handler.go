// Copyright (c) 2023 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package handler

import (
	"time"

	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/jwt-auth/config"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// jwt-auth 插件认证逻辑与 basic-auth 一致：
// - global_auth == true 开启全局生效：
//   - 若当前 domain/route 未配置 allow 列表，即未配置该插件：则在所有 consumers 中查找，如果找到则认证通过，否则认证失败 (1*)
//   - 若当前 domain/route 配置了该插件：则在 allow 列表中查找，如果找到则认证通过，否则认证失败
//
// - global_auth == false 非全局生效：(2*)
//   - 若当前 domain/route 未配置该插件：则直接放行
//   - 若当前 domain/route 配置了该插件：则在 allow 列表中查找，如果找到则认证通过，否则认证失败
//
// - global_auth 未设置：
//   - 若没有一个 domain/route 配置该插件：则遵循 (1*)
//   - 若有至少一个 domain/route 配置该插件：则遵循 (2*)
//
// https://github.com/alibaba/higress/blob/e09edff827b94fa5bcc149bbeadc905361100c2a/plugins/wasm-go/extensions/basic-auth/main.go#L191
func OnHTTPRequestHeaders(ctx wrapper.HttpContext, config cfg.JWTAuthConfig, log log.Log) types.Action {
	var (
		noAllow            = len(config.Allow) == 0 // 未配置 allow 列表，表示插件在该 domain/route 未生效
		globalAuthNoSet    = config.GlobalAuthCheck() == cfg.GlobalAuthNoSet
		globalAuthSetFalse = config.GlobalAuthCheck() == cfg.GlobalAuthFalse
	)

	// 不需要认证而直接放行的情况：
	// - global_auth == false 且 当前 domain/route 未配置该插件
	// - global_auth 未设置 且 有至少一个 domain/route 配置该插件 且 当前 domain/route 未配置该插件
	if globalAuthSetFalse || (config.RuleSet && globalAuthNoSet) {
		if noAllow {
			log.Info("authorization is not required")
			return types.ActionContinue
		}
	}

	verifyTime := time.Now()
	decision := verifyConsumers(config, log, verifyTime)
	if decision.remoteConsumer != nil {
		return fetchRemoteJWKsAndVerify(decision.remoteConsumer, config, log, verifyTime)
	}
	return decision.action()
}

func fetchRemoteJWKsAndVerify(consumer *cfg.Consumer, config cfg.JWTAuthConfig, log log.Log, verifyTime time.Time) types.Action {
	err := fetchRemoteJWKs(consumer, log, func() {
		completeAuthenticationAfterRemoteFetch(config, log, verifyTime, 1)
	})
	if err != nil {
		log.Warnf("failed to dispatch remote jwks fetch, consumer:%s, reason:%s", consumer.Name, err.Error())
		return actionAfterRemoteFetch(config, log, verifyTime, 1)
	}
	return types.HeaderStopAllIterationAndWatermark
}

func completeAuthenticationAfterRemoteFetch(config cfg.JWTAuthConfig, log log.Log, verifyTime time.Time, attempts int) {
	decision := decisionAfterRemoteFetch(config, log, verifyTime, attempts)
	if decision.waitingRemoteFetch {
		return
	}
	_ = decision.action()
	if decision.resume {
		proxywasm.ResumeHttpRequest()
	}
}

func actionAfterRemoteFetch(config cfg.JWTAuthConfig, log log.Log, verifyTime time.Time, attempts int) types.Action {
	decision := decisionAfterRemoteFetch(config, log, verifyTime, attempts)
	if decision.waitingRemoteFetch {
		return types.HeaderStopAllIterationAndWatermark
	}
	return decision.action()
}

func decisionAfterRemoteFetch(config cfg.JWTAuthConfig, log log.Log, verifyTime time.Time, attempts int) authDecision {
	for {
		decision := verifyConsumers(config, log, verifyTime)
		if decision.remoteConsumer == nil {
			return decision
		}
		if attempts >= len(config.Consumers) {
			log.Warnf("remote jwks fetch chain exhausted after %d attempts", attempts)
			return authDecision{action: deniedJWTVerificationFails}
		}

		// Chained fetches only advance after each response has populated or rejected one cache entry.
		nextAttempts := attempts + 1
		err := fetchRemoteJWKs(decision.remoteConsumer, log, func() {
			completeAuthenticationAfterRemoteFetch(config, log, verifyTime, nextAttempts)
		})
		if err == nil {
			return authDecision{waitingRemoteFetch: true}
		}

		log.Warnf("failed to dispatch remote jwks fetch, consumer:%s, reason:%s", decision.remoteConsumer.Name, err.Error())
		attempts = nextAttempts
	}
}

type authDecision struct {
	action             func() types.Action
	resume             bool
	remoteConsumer     *cfg.Consumer
	waitingRemoteFetch bool
}

func verifyConsumers(config cfg.JWTAuthConfig, log log.Log, verifyTime time.Time) authDecision {
	header := &proxywasmProvider{}
	actionMap := map[string]func() types.Action{}
	unAuthzConsumer := ""
	var firstRemoteConsumer *cfg.Consumer

	// 匹配consumer
	for i := range config.Consumers {
		consumer := config.Consumers[i]
		verified, err := consumerVerify(consumer, verifyTime, header, log)
		if err != nil {
			if isRemoteJWKsCacheMiss(err) {
				if firstRemoteConsumer == nil && consumerAllowedForFetch(config, consumer.Name) {
					firstRemoteConsumer = consumer
				}
				continue
			}
			log.Warn(err.Error())
			if v, ok := err.(*ErrDenied); ok {
				actionMap[consumer.Name] = v.denied
			}
			continue
		}

		action, resume := actionForVerifiedConsumer(config, consumer.Name, log)
		if resume {
			applyConsumerSideEffects(consumer, verified, header, log)
			return authDecision{action: action, resume: true}
		}
		if action != nil {
			actionMap[consumer.Name] = action
			unAuthzConsumer = consumer.Name
			continue
		}
	}

	if firstRemoteConsumer != nil {
		return authDecision{remoteConsumer: firstRemoteConsumer}
	}
	if len(config.Allow) == 1 {
		if unAuthzConsumer != "" {
			log.Warnf("consumer %q denied", unAuthzConsumer)
			return authDecision{action: deniedUnauthorizedConsumer}
		}
		if v, ok := actionMap[config.Allow[0]]; ok {
			log.Warnf("consumer %q denied", config.Allow[0])
			return authDecision{action: v}
		}
	}

	// 拒绝兜底
	log.Warnf("all consumers verify failed")
	return authDecision{action: deniedNotAllow}
}

func actionForVerifiedConsumer(config cfg.JWTAuthConfig, name string, log log.Log) (func() types.Action, bool) {
	noAllow := len(config.Allow) == 0
	globalAuthNoSet := config.GlobalAuthCheck() == cfg.GlobalAuthNoSet
	globalAuthSetTrue := config.GlobalAuthCheck() == cfg.GlobalAuthTrue
	globalAuthSetFalse := config.GlobalAuthCheck() == cfg.GlobalAuthFalse

	if (globalAuthSetTrue && noAllow) || (globalAuthNoSet && !config.RuleSet) {
		log.Infof("consumer %q authenticated", name)
		return func() types.Action { return authenticated(name) }, true
	}

	if globalAuthSetTrue && !noAllow {
		if !contains(name, config.Allow) {
			log.Warnf("jwt verify failed, consumer %q not allow", name)
			return deniedUnauthorizedConsumer, false
		}
		log.Infof("consumer %q authenticated", name)
		return func() types.Action { return authenticated(name) }, true
	}

	if globalAuthSetFalse || (globalAuthNoSet && config.RuleSet) {
		if !noAllow {
			if !contains(name, config.Allow) {
				log.Warnf("jwt verify failed, consumer %q not allow", name)
				return deniedUnauthorizedConsumer, false
			}
			log.Infof("consumer %q authenticated", name)
			return func() types.Action { return authenticated(name) }, true
		}
	}

	return nil, false
}

func consumerAllowedForFetch(config cfg.JWTAuthConfig, name string) bool {
	return len(config.Allow) == 0 || contains(name, config.Allow)
}

func contains(str string, arr []string) bool {
	for _, i := range arr {
		if i == str {
			return true
		}
	}
	return false
}
