package provider

import (
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/proxytest"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApiKeyAffinityDefaultsToDisabled(t *testing.T) {
	var config ProviderConfig
	config.FromJson(gjson.Parse(`{
		"type": "openai",
		"apiTokens": ["sk-a", "sk-b"]
	}`))

	require.False(t, config.ApiKeyAffinityEnabled())
	require.NoError(t, config.Validate())
}

func TestApiKeyAffinityEnabledRequiresRedisServiceName(t *testing.T) {
	var config ProviderConfig
	config.FromJson(gjson.Parse(`{
		"type": "openai",
		"apiTokens": ["sk-a", "sk-b"],
		"apiKeyAffinity": {
			"enabled": true
		}
	}`))

	err := config.Validate()
	require.EqualError(t, err, "apiKeyAffinity.redis.serviceName must not be empty when apiKeyAffinity is enabled")
}

func TestApiKeyAffinityAppliesDocumentedDefaults(t *testing.T) {
	var config ProviderConfig
	config.FromJson(gjson.Parse(`{
		"type": "openai",
		"apiTokens": ["sk-a", "sk-b"],
		"apiKeyAffinity": {
			"enabled": true,
			"redis": {
				"serviceName": "redis.dns"
			}
		}
	}`))

	require.NoError(t, config.Validate())
	require.Equal(t, 6379, config.apiKeyAffinity.redis.servicePort)
	require.Equal(t, int64(1000), config.apiKeyAffinity.redis.timeout)
	require.Equal(t, 7, config.apiKeyAffinity.bindingTTLDays)
	require.Equal(t, 3, config.apiKeyAffinity.maxRetries)
	require.Equal(t, int64(30_000), config.apiKeyAffinity.retryTimeout)
}

func TestApiKeyAffinityUsesStaticServiceDefaultPort(t *testing.T) {
	var config ProviderConfig
	config.FromJson(gjson.Parse(`{
		"type": "openai",
		"apiTokens": ["sk-a"],
		"apiKeyAffinity": {
			"enabled": true,
			"redis": {
				"serviceName": "redis.static"
			}
		}
	}`))

	require.Equal(t, 80, config.apiKeyAffinity.redis.servicePort)
}

func TestApiKeyAffinityRoundsRenewalWindowToWholeDays(t *testing.T) {
	require.Equal(t, 6, affinityRenewAfterDays(7))
	require.Equal(t, 24, affinityRenewAfterDays(30))
}

func TestApiKeyAffinityKeyAndBindingDoNotContainPlaintextIdentity(t *testing.T) {
	config := ProviderConfig{
		id:             "provider-a",
		typ:            providerTypeOpenAI,
		providerDomain: "api.example.com",
		apiTokens:      []string{"sk-sensitive-a", "sk-sensitive-b"},
	}
	consumer := "consumer-sensitive"
	token := config.apiTokens[0]

	key := config.affinityBindingKey(consumer)
	binding := affinityBinding{
		TokenFingerprint: tokenFingerprint(token),
		RenewAfter:       123,
	}
	encoded, err := binding.marshal()
	require.NoError(t, err)

	for _, plaintext := range []string{consumer, token, config.apiTokens[1]} {
		require.False(t, strings.Contains(key, plaintext))
		require.False(t, strings.Contains(encoded, plaintext))
	}
	require.NotEqual(t, tokenFingerprint(config.apiTokens[0]), tokenFingerprint(config.apiTokens[1]))
}

func TestApiKeyAffinityProviderScopeChangesWithTokenPool(t *testing.T) {
	first := ProviderConfig{id: "provider-a", typ: providerTypeOpenAI, apiTokens: []string{"sk-a", "sk-b"}}
	second := ProviderConfig{id: "provider-a", typ: providerTypeOpenAI, apiTokens: []string{"sk-a", "sk-c"}}

	require.NotEqual(t, first.affinityBindingKey("consumer-a"), second.affinityBindingKey("consumer-a"))
}

func TestApiKeyAffinityInitializesConfiguredRedisClient(t *testing.T) {
	_, reset := proxytest.NewHostEmulator(proxytest.NewEmulatorOption())
	defer reset()

	var config ProviderConfig
	config.FromJson(gjson.Parse(`{
		"type": "openai",
		"apiTokens": ["sk-a"],
		"apiKeyAffinity": {
			"enabled": true,
			"redis": {
				"serviceName": "redis.dns",
				"database": 2
			}
		}
	}`))

	require.NoError(t, config.InitApiKeyAffinity())
	require.NotNil(t, config.apiKeyAffinity.redisClient)
	require.True(t, config.apiKeyAffinity.redisClient.Ready())
}

func TestApiKeyAffinitySelectionOnlyUsesAvailableAndUntriedTokens(t *testing.T) {
	tokens := []string{"sk-a", "sk-b", "sk-c"}

	t.Run("bound token removed by failover", func(t *testing.T) {
		got := selectAvailableAffinityToken(
			[]string{"sk-b", "sk-c"},
			"consumer-a",
			tokenFingerprint("sk-a"),
			nil,
		)
		require.Contains(t, []string{"sk-b", "sk-c"}, got)
	})

	t.Run("failed token is excluded", func(t *testing.T) {
		got := selectAvailableAffinityToken(
			tokens,
			"consumer-a",
			tokenFingerprint("sk-a"),
			map[string]struct{}{tokenFingerprint("sk-a"): {}},
		)
		require.NotEqual(t, "sk-a", got)
		require.Contains(t, []string{"sk-b", "sk-c"}, got)
	})

	t.Run("all tokens exhausted", func(t *testing.T) {
		excluded := map[string]struct{}{}
		for _, token := range tokens {
			excluded[tokenFingerprint(token)] = struct{}{}
		}
		got := selectAvailableAffinityToken(tokens, "consumer-a", "", excluded)
		require.Empty(t, got)
	})
}

func TestApiKeyAffinityInitialMissUsesExistingSelectionAlgorithm(t *testing.T) {
	config := ProviderConfig{
		apiTokens: []string{"sk-a", "sk-b", "sk-c"},
		failover:  &failover{ctxAvailableApiTokensInRequest: "available-tokens"},
		apiKeyAffinity: &apiKeyAffinity{
			enabled: true,
		},
	}
	ctx := newMapCtx()
	ctx.SetContext(config.failover.ctxAvailableApiTokensInRequest, config.apiTokens)
	ctx.SetContext(ctxAffinityConsumer, "consumer-a")
	ctx.SetContext(CtxKeyApiName, ApiNameChatCompletion)
	require.False(t, config.isFailoverEnabled())
	require.Equal(t, "consumer-a", ctx.GetStringContext(ctxAffinityConsumer, ""))
	require.Nil(t, ctx.GetContext(ctxAffinityBinding))
	require.Nil(t, ctx.GetContext(ctxAffinityExcludedTokens))

	selected := config.GetApiKeyAffinityToken(ctx)
	require.Contains(t, config.apiTokens, selected)
	require.Equal(t, selected, ctx.GetContext(ctxKeyApiKey), "the existing selector should store the initial choice in request context")
}

func TestAnthropicMessagesAffinityIsNotEnabledByDefault(t *testing.T) {
	require.False(t, isStatefulAPI(string(ApiNameAnthropicMessages)))
}
