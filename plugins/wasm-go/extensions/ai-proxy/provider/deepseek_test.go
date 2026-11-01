package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepseekValidateConfig(t *testing.T) {
	initializer := &deepseekProviderInitializer{}

	t.Run("no api tokens", func(t *testing.T) {
		config := &ProviderConfig{}
		err := initializer.ValidateConfig(config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no apiToken")
	})

	t.Run("with api tokens", func(t *testing.T) {
		config := &ProviderConfig{
			apiTokens: []string{"sk-test-token"},
		}
		err := initializer.ValidateConfig(config)
		assert.NoError(t, err)
	})

	t.Run("with multiple api tokens", func(t *testing.T) {
		config := &ProviderConfig{
			apiTokens: []string{"sk-token-1", "sk-token-2"},
		}
		err := initializer.ValidateConfig(config)
		assert.NoError(t, err)
	})
}

func TestDeepseekDefaultCapabilities(t *testing.T) {
	initializer := &deepseekProviderInitializer{}
	caps := initializer.DefaultCapabilities()

	assert.Equal(t, PathOpenAIChatCompletions, caps[string(ApiNameChatCompletion)])
	assert.Equal(t, PathOpenAIModels, caps[string(ApiNameModels)])
	assert.Equal(t, deepseekAnthropicMessagesPath, caps[string(ApiNameAnthropicMessages)])
	assert.Len(t, caps, 3)
}

func TestDeepseekCreateProvider(t *testing.T) {
	initializer := &deepseekProviderInitializer{}

	t.Run("creates provider successfully", func(t *testing.T) {
		config := ProviderConfig{
			apiTokens: []string{"sk-test"},
		}
		p, err := initializer.CreateProvider(config)
		require.NoError(t, err)
		require.NotNil(t, p)
		assert.Equal(t, providerTypeDeepSeek, p.GetProviderType())
	})

	t.Run("provider has correct type", func(t *testing.T) {
		config := ProviderConfig{
			apiTokens: []string{"sk-test"},
		}
		p, err := initializer.CreateProvider(config)
		require.NoError(t, err)

		dp, ok := p.(*deepseekProvider)
		require.True(t, ok)
		assert.NotNil(t, dp.config.capabilities)
	})
}

func TestDeepseekGetProviderType(t *testing.T) {
	p := &deepseekProvider{}
	assert.Equal(t, providerTypeDeepSeek, p.GetProviderType())
}

func TestDeepseekDomain(t *testing.T) {
	assert.Equal(t, "api.deepseek.com", deepseekDomain)
}

func TestDeepseekAnthropicPath(t *testing.T) {
	assert.Equal(t, "/anthropic/v1/messages", deepseekAnthropicMessagesPath)
}
