package provider

import (
	"errors"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/proxytest"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/stretchr/testify/require"
)

type testFailoverProvider struct{}

func (testFailoverProvider) GetProviderType() string {
	return providerTypeOpenAI
}

func setSharedDataWithCurrentCAS(t *testing.T, key string, value []byte) {
	t.Helper()

	_, cas, err := proxywasm.GetSharedData(key)
	if err != nil && !errors.Is(err, types.ErrorStatusNotFound) {
		require.NoError(t, err)
	}
	require.NoError(t, proxywasm.SetSharedData(key, value, cas))
}

func TestSetApiTokensFailoverResetsSharedDataOnReinit(t *testing.T) {
	_, reset := proxytest.NewHostEmulator(proxytest.NewEmulatorOption())
	defer reset()

	config := &ProviderConfig{
		id:        "config-update-test",
		typ:       providerTypeOpenAI,
		apiTokens: []string{"sk-token-a", "sk-token-b"},
		failover: &failover{
			enabled:             true,
			cooldownDuration:    100,
			healthCheckInterval: 1000,
		},
	}

	require.NoError(t, config.SetApiTokensFailover(testFailoverProvider{}))

	setSharedDataWithCurrentCAS(t, config.failover.ctxVmLease, []byte(`{"vmID":"stale","timestamp":1}`))
	setSharedDataWithCurrentCAS(t, config.failover.ctxUnavailableApiTokens, []byte(`["sk-stale"]`))
	setSharedDataWithCurrentCAS(t, config.failover.ctxApiTokenRequestSuccessCount, []byte(`{"sk-token-a":1}`))
	setSharedDataWithCurrentCAS(t, config.failover.ctxApiTokenRequestFailureCount, []byte(`{"sk-token-a":2}`))
	setSharedDataWithCurrentCAS(t, config.failover.ctxApiTokenUnavailableSince, []byte(`{"sk-token-a":123}`))
	setSharedDataWithCurrentCAS(t, config.failover.ctxHealthCheckEndpoint, []byte(`{"host":"stale.example.com","path":"/v1/chat/completions","cluster":"stale-cluster"}`))

	require.NoError(t, config.SetApiTokensFailover(testFailoverProvider{}))

	apiTokens, _, err := proxywasm.GetSharedData(config.failover.ctxApiTokens)
	require.NoError(t, err)
	require.JSONEq(t, `["sk-token-a","sk-token-b"]`, string(apiTokens))

	for _, key := range []string{
		config.failover.ctxVmLease,
		config.failover.ctxUnavailableApiTokens,
		config.failover.ctxApiTokenRequestSuccessCount,
		config.failover.ctxApiTokenRequestFailureCount,
		config.failover.ctxApiTokenUnavailableSince,
		config.failover.ctxHealthCheckEndpoint,
	} {
		value, _, err := proxywasm.GetSharedData(key)
		require.NoError(t, err)
		require.Empty(t, value, "expected %s to be cleared during failover reinitialization", key)
	}
}
