package provider

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoonshotValidateConfig(t *testing.T) {
	initializer := &moonshotProviderInitializer{}

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

	t.Run("moonshotFileId and context mutual exclusion", func(t *testing.T) {
		config := &ProviderConfig{
			apiTokens:      []string{"sk-test"},
			moonshotFileId: "file-123",
			context:        &ContextConfig{},
		}
		err := initializer.ValidateConfig(config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be configured at the same time")
	})

	t.Run("moonshotFileId alone is valid", func(t *testing.T) {
		config := &ProviderConfig{
			apiTokens:      []string{"sk-test"},
			moonshotFileId: "file-123",
		}
		err := initializer.ValidateConfig(config)
		assert.NoError(t, err)
	})
}

func TestMoonshotDefaultCapabilities(t *testing.T) {
	initializer := &moonshotProviderInitializer{}
	caps := initializer.DefaultCapabilities()

	assert.Equal(t, PathOpenAIChatCompletions, caps[string(ApiNameChatCompletion)])
	assert.Equal(t, PathOpenAIModels, caps[string(ApiNameModels)])
	assert.Len(t, caps, 2)
}

func TestMoonshotGetProviderType(t *testing.T) {
	p := &moonshotProvider{}
	assert.Equal(t, providerTypeMoonshot, p.GetProviderType())
}

func TestMoonshotOnStreamingEvent(t *testing.T) {
	p := &moonshotProvider{}

	t.Run("non-chat API returns nil", func(t *testing.T) {
		events, err := p.OnStreamingEvent(nil, ApiNameModels, StreamEvent{Data: "{}"})
		assert.NoError(t, err)
		assert.Nil(t, events)
	})

	t.Run("moves usage from choices to top level", func(t *testing.T) {
		data := `{"choices":[{"usage":{"prompt_tokens":10,"completion_tokens":20}}],"id":"chatcmpl-1"}`
		events, err := p.OnStreamingEvent(nil, ApiNameChatCompletion, StreamEvent{Data: data})
		require.NoError(t, err)
		require.Len(t, events, 1)

		// usage should be moved to top level
		assert.Contains(t, events[0].Data, `"usage"`)
		assert.Contains(t, events[0].Data, `"prompt_tokens"`)
		// choices.0.usage should be removed
		assert.NotContains(t, events[0].Data, `"choices":[{"usage"`)
	})

	t.Run("passes through events without usage", func(t *testing.T) {
		data := `{"choices":[{"delta":{"content":"hello"}}],"id":"chatcmpl-1"}`
		events, err := p.OnStreamingEvent(nil, ApiNameChatCompletion, StreamEvent{Data: data})
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, data, events[0].Data)
	})
}

func TestMoonshotAppendStreamEvent(t *testing.T) {
	p := &moonshotProvider{}
	var builder strings.Builder

	event := &StreamEvent{Data: `{"content":"hello"}`}
	p.appendStreamEvent(&builder, event)

	result := builder.String()
	assert.True(t, strings.HasPrefix(result, streamDataItemKey))
	assert.Contains(t, result, `{"content":"hello"}`)
	assert.True(t, strings.HasSuffix(result, "\n\n"))
}
