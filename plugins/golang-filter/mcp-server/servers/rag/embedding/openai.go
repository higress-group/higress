package embedding

import (
	"context"
	"errors"
	"fmt"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/rag/config"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
)

const (
	OPENAI_DEFAULT_MODEL_NAME = "text-embedding-ada-002"
)

type openAIProviderInitializer struct {
}

func (c *openAIProviderInitializer) validateConfig(config *config.EmbeddingConfig) error {
	if config.APIKey == "" {
		return errors.New("[openai embedding] apiKey is required")
	}
	if config.Model == "" {
		config.Model = OPENAI_DEFAULT_MODEL_NAME
	}
	if config.Dimensions <= 0 {
		config.Dimensions = 1536
	}

	return nil
}

func (c *openAIProviderInitializer) CreateProvider(config config.EmbeddingConfig) (Provider, error) {
	if err := c.validateConfig(&config); err != nil {
		return nil, err
	}
	// 创建 OpenAI 客户端
	var clientOptions []option.RequestOption
	clientOptions = append(clientOptions, option.WithAPIKey(config.APIKey))

	// 如果设置了自定义 baseURL，则使用它
	if config.BaseURL != "" {
		clientOptions = append(clientOptions, option.WithBaseURL(config.BaseURL))
	}
	// 创建 OpenAI 客户端
	client := openai.NewClient(clientOptions...)

	return &OpenAIProvider{
		client:     &client,
		model:      config.Model,
		dimensions: config.Dimensions,
	}, nil
}

// EmbeddingClient handles vector embedding generation using OpenAI-compatible APIs
type OpenAIProvider struct {
	client     *openai.Client
	model      string
	dimensions int
}

func (e *OpenAIProvider) GetProviderType() string {
	return PROVIDER_TYPE_OPENAI
}

// GetEmbedding generates vector embedding for the given text
func (e *OpenAIProvider) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	params := openai.EmbeddingNewParams{
		Model: e.model,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String(text),
		},
		Dimensions:     openai.Int(int64(e.dimensions)),
		EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
	}

	embeddingResp, err := e.client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	if len(embeddingResp.Data) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}

	// Convert []float64 to []float32
	embedding := make([]float32, len(embeddingResp.Data[0].Embedding))
	for i, v := range embeddingResp.Data[0].Embedding {
		embedding[i] = float32(v)
	}

	return embedding, nil
}
