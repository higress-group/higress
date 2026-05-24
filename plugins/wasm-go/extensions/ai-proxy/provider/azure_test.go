package provider

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAzureValidateServiceURLAPIVersionByPathMode(t *testing.T) {
	tests := []struct {
		name       string
		serviceURL string
		wantErr    bool
	}{
		{
			name:       "v1 base path without api-version is accepted",
			serviceURL: "https://resource.openai.azure.com/openai/v1",
		},
		{
			name:       "v1 base path with trailing slash without api-version is accepted",
			serviceURL: "https://resource.openai.azure.com/openai/v1/",
		},
		{
			name:       "v1 child path without api-version is accepted",
			serviceURL: "https://resource.openai.azure.com/openai/v1/chat/completions",
		},
		{
			name:       "v1 child path with duplicate slash without api-version is accepted",
			serviceURL: "https://resource.openai.azure.com/openai/v1//chat/completions",
		},
		{
			name:       "legacy deployment path with api-version is accepted",
			serviceURL: "https://resource.openai.azure.com/openai/deployments/gpt-4/chat/completions?api-version=2024-10-21",
		},
		{
			name:       "legacy domain-only path with api-version is accepted",
			serviceURL: "https://resource.openai.azure.com?api-version=2024-10-21",
		},
		{
			name:       "legacy deployment path without api-version is rejected",
			serviceURL: "https://resource.openai.azure.com/openai/deployments/gpt-4/chat/completions",
			wantErr:    true,
		},
		{
			name:       "legacy domain-only path without api-version is rejected",
			serviceURL: "https://resource.openai.azure.com",
			wantErr:    true,
		},
		{
			name:       "legacy path with empty api-version value is rejected",
			serviceURL: "https://resource.openai.azure.com/openai/deployments/gpt-4/chat/completions?api-version=",
			wantErr:    true,
		},
		{
			name:       "lookalike v10 path without api-version is rejected",
			serviceURL: "https://resource.openai.azure.com/openai/v10/chat/completions",
			wantErr:    true,
		},
		{
			name:       "lookalike v1beta path without api-version is rejected",
			serviceURL: "https://resource.openai.azure.com/openai/v1beta/chat/completions",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.serviceURL)
			require.NoError(t, err)

			err = validateAzureServiceURLAPIVersion(u, tt.serviceURL)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), queryAzureApiVersion)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestAzureOpenAIV1PathClassification(t *testing.T) {
	t.Run("v1 path mode accepts only exact v1 path and children", func(t *testing.T) {
		tests := []struct {
			path string
			want bool
		}{
			{path: "/openai/v1", want: true},
			{path: "/openai/v1/", want: true},
			{path: "/openai//v1", want: true},
			{path: "/openai/v1/chat/completions", want: true},
			{path: "/openai/v1//chat/completions", want: true},
			{path: "", want: false},
			{path: "/", want: false},
			{path: "/openai", want: false},
			{path: "/openai/deployments/gpt-4/chat/completions", want: false},
			{path: "/openai/v10/chat/completions", want: false},
			{path: "/openai/v1beta/chat/completions", want: false},
		}

		for _, tt := range tests {
			t.Run(tt.path, func(t *testing.T) {
				assert.Equal(t, tt.want, isAzureOpenAIV1Path(tt.path))
			})
		}
	})

	t.Run("v1 base url mode accepts only exact v1 base path", func(t *testing.T) {
		tests := []struct {
			path string
			want bool
		}{
			{path: "/openai/v1", want: true},
			{path: "/openai/v1/", want: true},
			{path: "/openai//v1", want: true},
			{path: "/openai/v1/chat/completions", want: false},
			{path: "/openai/v10", want: false},
			{path: "/openai/v1beta", want: false},
			{path: "", want: false},
			{path: "/", want: false},
		}

		for _, tt := range tests {
			t.Run(tt.path, func(t *testing.T) {
				assert.Equal(t, tt.want, isAzureOpenAIV1BasePath(tt.path))
			})
		}
	})
}

func TestAppendAzureServiceURLRawQuery(t *testing.T) {
	tests := []struct {
		name     string
		basePath string
		rawQuery string
		want     string
	}{
		{
			name:     "empty raw query keeps path unchanged",
			basePath: "/openai/v1/chat/completions",
			want:     "/openai/v1/chat/completions",
		},
		{
			name:     "path without query gets question mark",
			basePath: "/openai/deployments/gpt-4/chat/completions",
			rawQuery: "api-version=2024-10-21",
			want:     "/openai/deployments/gpt-4/chat/completions?api-version=2024-10-21",
		},
		{
			name:     "path ending with question mark appends raw query directly",
			basePath: "/openai/files?",
			rawQuery: "api-version=2024-10-21",
			want:     "/openai/files?api-version=2024-10-21",
		},
		{
			name:     "path with existing query appends raw query with ampersand",
			basePath: "/openai/files?limit=10",
			rawQuery: "api-version=2024-10-21",
			want:     "/openai/files?limit=10&api-version=2024-10-21",
		},
		{
			name:     "raw query order is preserved",
			basePath: "/openai/v1/chat/completions",
			rawQuery: "foo=bar&api-version=preview",
			want:     "/openai/v1/chat/completions?foo=bar&api-version=preview",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, appendAzureServiceURLRawQuery(tt.basePath, tt.rawQuery))
		})
	}
}

func TestAzureDefaultOpenAIV1Capabilities(t *testing.T) {
	capabilities := (&azureProviderInitializer{}).DefaultOpenAIV1Capabilities(pathAzureOpenAIV1)

	assert.Equal(t, "/openai/v1/chat/completions", capabilities[string(ApiNameChatCompletion)])
	assert.Equal(t, "/openai/v1/embeddings", capabilities[string(ApiNameEmbeddings)])
	assert.Equal(t, "/openai/v1/models", capabilities[string(ApiNameModels)])
	assert.Equal(t, "/openai/v1/responses", capabilities[string(ApiNameResponses)])
	for apiName, capabilityPath := range capabilities {
		assert.True(t, strings.HasPrefix(capabilityPath, pathAzureOpenAIV1+"/"), "capability %s should use Azure v1 base path", apiName)
		assert.NotContains(t, capabilityPath, pathAzureModelPlaceholder, "capability %s should not use deployment placeholder", apiName)
		assert.NotContains(t, capabilityPath, queryAzureApiVersion, "capability %s should not synthesize api-version", apiName)
	}
}

func TestDefaultOpenAIV1CapabilitiesSkipsUnexpectedPath(t *testing.T) {
	capabilities := defaultOpenAIV1Capabilities(pathAzureOpenAIV1, map[string]string{
		string(ApiNameChatCompletion): PathOpenAIChatCompletions,
		"bad-capability":              "/bad/chat/completions",
	})

	assert.Equal(t, "/openai/v1/chat/completions", capabilities[string(ApiNameChatCompletion)])
	assert.NotContains(t, capabilities, "bad-capability")
}

func TestAzureValidateConfigCoversErrorBranches(t *testing.T) {
	initializer := &azureProviderInitializer{}

	t.Run("missing azureServiceUrl is rejected before URL parsing", func(t *testing.T) {
		err := initializer.ValidateConfig(&ProviderConfig{
			apiTokens: []string{"sk-test"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing azureServiceUrl")
	})

	t.Run("invalid azureServiceUrl is rejected", func(t *testing.T) {
		err := initializer.ValidateConfig(&ProviderConfig{
			azureServiceUrl: "https://[invalid-host",
			apiTokens:       []string{"sk-test"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid azureServiceUrl")
	})

	t.Run("legacy URL without api-version is rejected", func(t *testing.T) {
		err := initializer.ValidateConfig(&ProviderConfig{
			azureServiceUrl: "https://resource.openai.azure.com/openai/deployments/gpt-4/chat/completions",
			apiTokens:       []string{"sk-test"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), queryAzureApiVersion)
	})

	t.Run("v1 URL without api token is rejected after path mode validation", func(t *testing.T) {
		err := initializer.ValidateConfig(&ProviderConfig{
			azureServiceUrl: "https://resource.openai.azure.com/openai/v1",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no apiToken")
	})

	t.Run("v1 URL with api token is accepted without api-version", func(t *testing.T) {
		err := initializer.ValidateConfig(&ProviderConfig{
			azureServiceUrl: "https://resource.openai.azure.com/openai/v1",
			apiTokens:       []string{"sk-test"},
		})
		require.NoError(t, err)
	})
}

func TestAzureCreateProviderClassifiesV1ServiceURLMode(t *testing.T) {
	initializer := &azureProviderInitializer{}

	t.Run("invalid URL returns create error", func(t *testing.T) {
		_, err := initializer.CreateProvider(ProviderConfig{
			azureServiceUrl: "https://[invalid-host",
			apiTokens:       []string{"sk-test"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid azureServiceUrl")
	})

	t.Run("v1 base url uses OpenAI capability mapping without api-version", func(t *testing.T) {
		p, err := initializer.CreateProvider(ProviderConfig{
			azureServiceUrl: "https://resource.openai.azure.com/openai/v1",
			apiTokens:       []string{"sk-test"},
		})
		require.NoError(t, err)

		azureProvider, ok := p.(*azureProvider)
		require.True(t, ok)
		assert.Equal(t, azureServiceUrlTypeOpenAIV1Base, azureProvider.serviceUrlType)
		assert.Equal(t, "/openai/v1", azureProvider.serviceUrlFullPath)
		assert.Equal(t, "", azureProvider.apiVersion)
		assert.Equal(t, "/openai/v1/chat/completions", azureProvider.config.capabilities[string(ApiNameChatCompletion)])
	})

	t.Run("v1 base url with trailing slash is still base mode", func(t *testing.T) {
		p, err := initializer.CreateProvider(ProviderConfig{
			azureServiceUrl: "https://resource.openai.azure.com/openai/v1/",
			apiTokens:       []string{"sk-test"},
		})
		require.NoError(t, err)

		azureProvider, ok := p.(*azureProvider)
		require.True(t, ok)
		assert.Equal(t, azureServiceUrlTypeOpenAIV1Base, azureProvider.serviceUrlType)
		assert.Equal(t, "/openai/v1/", azureProvider.serviceUrlFullPath)
	})

	t.Run("v1 full path remains full path mode", func(t *testing.T) {
		p, err := initializer.CreateProvider(ProviderConfig{
			azureServiceUrl: "https://resource.openai.azure.com/openai/v1/chat/completions",
			apiTokens:       []string{"sk-test"},
		})
		require.NoError(t, err)

		azureProvider, ok := p.(*azureProvider)
		require.True(t, ok)
		assert.Equal(t, azureServiceUrlTypeFull, azureProvider.serviceUrlType)
		assert.Equal(t, "/openai/v1/chat/completions", azureProvider.serviceUrlFullPath)
		assert.Equal(t, "/openai/deployments/{model}/chat/completions", azureProvider.config.capabilities[string(ApiNameChatCompletion)])
	})
}
