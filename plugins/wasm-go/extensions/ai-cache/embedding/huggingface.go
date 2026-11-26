package embedding

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	HUGGINGFACE_DOMAIN             = "router.huggingface.co"
	HUGGINGFACE_PORT               = 443
	HUGGINGFACE_DEFAULT_MODEL_NAME = "sentence-transformers/all-MiniLM-L6-v2"
	HUGGINGFACE_ENDPOINT           = "/hf-inference/models/{modelId}/pipeline/feature-extraction"
)

type huggingfaceProviderInitializer struct {
}

var huggingfaceConfig huggingfaceProviderConfig

type huggingfaceProviderConfig struct {
	// @Title zh-CN 文本特征提取服务 API Key
	// @Description zh-CN 文本特征提取服务 API Key。在HuggingFace定义为 hf_token
	apiKey string
}

func (c *huggingfaceProviderInitializer) InitConfig(json gjson.Result) {
	huggingfaceConfig.apiKey = json.Get("apiKey").String()
}

func (c *huggingfaceProviderInitializer) ValidateConfig() error {
	if huggingfaceConfig.apiKey == "" {
		return errors.New("[HuggingFace] hfTokens is required")
	}
	return nil
}

func (t *huggingfaceProviderInitializer) CreateProvider(c ProviderConfig) (Provider, error) {
	if c.servicePort == 0 {
		c.servicePort = HUGGINGFACE_PORT
	}
	if c.serviceHost == "" {
		c.serviceHost = HUGGINGFACE_DOMAIN
	}

	if c.model == "" {
		c.model = HUGGINGFACE_DEFAULT_MODEL_NAME
	}

	return &HuggingFaceProvider{
		config: c,
		client: wrapper.NewClusterClient(wrapper.FQDNCluster{
			FQDN: c.serviceName,
			Host: c.serviceHost,
			Port: c.servicePort,
		}),
	}, nil
}

func (t *HuggingFaceProvider) GetProviderType() string {
	return PROVIDER_TYPE_HUGGINGFACE
}

type HuggingFaceProvider struct {
	config ProviderConfig
	client wrapper.HttpClient
}

type HuggingFaceEmbeddingRequest struct {
	Inputs  string `json:"inputs"`
	Options struct {
		WaitForModel bool `json:"wait_for_model"`
	} `json:"options"`
}

func (t *HuggingFaceProvider) constructParameters(text string) (string, [][2]string, []byte, error) {
	if text == "" {
		err := errors.New("queryString text cannot be empty")
		return "", nil, nil, err
	}

	data := HuggingFaceEmbeddingRequest{
		Inputs: text,
		Options: struct {
			WaitForModel bool `json:"wait_for_model"`
		}{
			WaitForModel: true,
		},
	}

	requestBody, err := json.Marshal(data)
	if err != nil {
		log.Errorf("failed to marshal request data: %v", err)
		return "", nil, nil, err
	}

	modelId := t.config.model
	if modelId == "" {
		modelId = HUGGINGFACE_DEFAULT_MODEL_NAME
	}

	// 拼接 endpoint
	endpoint := strings.Replace(HUGGINGFACE_ENDPOINT, "{modelId}", modelId, 1)

	headers := [][2]string{
		{"Authorization", "Bearer " + huggingfaceConfig.apiKey},
		{"Content-Type", "application/json"},
	}

	return endpoint, headers, requestBody, err
}

func (t *HuggingFaceProvider) parseTextEmbedding(responseBody []byte) ([]float64, error) {
	var embedding []float64
	err := json.Unmarshal(responseBody, &embedding)
	if err != nil {
		return nil, err
	}
	return embedding, nil
}

func (t *HuggingFaceProvider) GetEmbedding(
	queryString string,
	ctx wrapper.HttpContext,
	callback func(emb []float64, err error)) error {
	embUrl, embHeaders, embRequestBody, err := t.constructParameters(queryString)
	if err != nil {
		log.Errorf("failed to construct parameters: %v", err)
		return err
	}

	err = t.client.Post(embUrl, embHeaders, embRequestBody,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {

			if statusCode != http.StatusOK {
				err = errors.New("failed to get embedding due to status code: " + strconv.Itoa(statusCode))
				callback(nil, err)
				return
			}

			var resp []float64
			resp, err = t.parseTextEmbedding(responseBody)
			if err != nil {
				err = fmt.Errorf("failed to parse response: %v", err)
				callback(nil, err)
				return
			}

			log.Debugf("get embedding response: %d, %s", statusCode, responseBody)

			if len(resp) == 0 {
				err = errors.New("no embedding found in response")
				callback(nil, err)
				return
			}

			callback(resp, nil)

		}, t.config.timeout)
	return err
}
