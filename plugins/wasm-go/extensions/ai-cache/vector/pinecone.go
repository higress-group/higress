package vector

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

type pineconeProviderInitializer struct{}

func (c *pineconeProviderInitializer) ValidateConfig(config ProviderConfig) error {
	if len(config.serviceHost) == 0 {
		return errors.New("[Pinecone] serviceHost is required")
	}
	if len(config.serviceName) == 0 {
		return errors.New("[Pinecone] serviceName is required")
	}
	if len(config.apiKey) == 0 {
		return errors.New("[Pinecone] apiKey is required")
	}
	return nil
}

func (c *pineconeProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	return &pineconeProvider{
		config: config,
		client: wrapper.NewClusterClient(wrapper.FQDNCluster{
			FQDN: config.serviceName,
			Host: config.serviceHost,
			Port: int64(config.servicePort),
		}),
	}, nil
}

type pineconeProvider struct {
	config ProviderConfig
	client wrapper.HttpClient
}

func (c *pineconeProvider) GetProviderType() string {
	return PROVIDER_TYPE_PINECONE
}

type pineconeMetadata struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Consumer string `json:"consumer"`
}

type pineconeVector struct {
	ID         string           `json:"id"`
	Values     []float64        `json:"values"`
	Properties pineconeMetadata `json:"metadata"`
}

type pineconeInsertRequest struct {
	Vectors   []pineconeVector `json:"vectors"`
	Namespace string           `json:"namespace"`
}

func (d *pineconeProvider) UploadAnswerAndEmbedding(
	queryString string,
	queryEmb []float64,
	queryAnswer string,
	ctx wrapper.HttpContext,
	log log.Log,
	callback func(ctx wrapper.HttpContext, log log.Log, err error)) error {
	consumer := ConsumerFromContext(ctx)
	if consumer == "" {
		return ErrConsumerRequired
	}
	// 最少需要填写的参数为 vector 和 question
	// 下面是一个例子
	// {
	// 	"vectors": [
	// 	  {
	// 		"id": "A",
	// 		"values": [0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1],
	// 		"metadata": {"question": "你好", "answer": "你也好"}
	// 	  }
	// 	]
	// }
	requestBody, err := json.Marshal(pineconeInsertRequest{
		Vectors: []pineconeVector{
			{
				ID:         uuid.New().String(),
				Values:     queryEmb,
				Properties: pineconeMetadata{Question: queryString, Answer: queryAnswer, Consumer: consumer},
			},
		},
		Namespace: d.config.collectionID,
	})

	if err != nil {
		log.Errorf("[Pinecone] Failed to marshal upload embedding request body: %v", err)
		return err
	}

	return d.client.Post(
		"/vectors/upsert",
		[][2]string{
			{"Content-Type", "application/json"},
			{"Api-Key", d.config.apiKey},
		},
		requestBody,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			log.Debugf("[Pinecone] statusCode:%d, responseBody:%s", statusCode, string(responseBody))
			callback(ctx, log, err)
		},
		d.config.timeout,
	)
}

type pineconeQueryRequest struct {
	Namespace       string                       `json:"namespace"`
	Vector          []float64                    `json:"vector"`
	TopK            int                          `json:"topK"`
	IncludeMetadata bool                         `json:"includeMetadata"`
	IncludeValues   bool                         `json:"includeValues"`
	Filter          map[string]map[string]string `json:"filter"`
}

func (d *pineconeProvider) QueryEmbedding(
	emb []float64,
	ctx wrapper.HttpContext,
	log log.Log,
	callback func(results []QueryResult, ctx wrapper.HttpContext, log log.Log, err error)) error {
	consumer := ConsumerFromContext(ctx)
	if consumer == "" {
		return ErrConsumerRequired
	}
	// 最少需要填写的参数为 vector
	// 下面是一个例子
	// {
	// 	"namespace": "higress",
	// 	"vector": [0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1],
	// 	"topK": 1,
	// 	"includeMetadata": false
	// }
	requestBody, err := json.Marshal(pineconeQueryRequest{
		Namespace:       d.config.collectionID,
		Vector:          emb,
		TopK:            d.config.topK,
		IncludeMetadata: true,
		IncludeValues:   false,
		Filter:          map[string]map[string]string{TenantField: {"$eq": consumer}},
	})
	if err != nil {
		log.Errorf("[Pinecone] Failed to marshal query embedding: %v", err)
		return err
	}

	return d.client.Post(
		"/query",
		[][2]string{
			{"Content-Type", "application/json"},
			{"Api-Key", d.config.apiKey},
		},
		requestBody,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			log.Debugf("[Pinecone] Query embedding response: %d, %s", statusCode, responseBody)
			results, err := d.parseQueryResponse(responseBody, log)
			if err != nil {
				err = fmt.Errorf("[Pinecone] Failed to parse query response: %v", err)
			}
			callback(results, ctx, log, err)
		},
		d.config.timeout,
	)
}

func (d *pineconeProvider) parseQueryResponse(responseBody []byte, log log.Log) ([]QueryResult, error) {
	if !gjson.GetBytes(responseBody, "matches.0.score").Exists() {
		log.Errorf("[Pinecone] No distance found in response body: %s", responseBody)
		return nil, errors.New("[Pinecone] No distance found in response body")
	}

	if !gjson.GetBytes(responseBody, "matches.0.metadata.question").Exists() {
		log.Errorf("[Pinecone] No question found in response body: %s", responseBody)
		return nil, errors.New("[Pinecone] No question found in response body")
	}

	if !gjson.GetBytes(responseBody, "matches.0.metadata.answer").Exists() {
		log.Errorf("[Pinecone] No answer found in response body: %s", responseBody)
		return nil, errors.New("[Pinecone] No answer found in response body")
	}

	resultNum := gjson.GetBytes(responseBody, "matches.#").Int()
	results := make([]QueryResult, 0, resultNum)
	for i := 0; i < int(resultNum); i++ {
		result := QueryResult{
			Text:     gjson.GetBytes(responseBody, fmt.Sprintf("matches.%d.metadata.question", i)).String(),
			Score:    gjson.GetBytes(responseBody, fmt.Sprintf("matches.%d.score", i)).Float(),
			Answer:   gjson.GetBytes(responseBody, fmt.Sprintf("matches.%d.metadata.answer", i)).String(),
			Consumer: gjson.GetBytes(responseBody, fmt.Sprintf("matches.%d.metadata.consumer", i)).String(),
		}
		results = append(results, result)
	}

	return results, nil
}
