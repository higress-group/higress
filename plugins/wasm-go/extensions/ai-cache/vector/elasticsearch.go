package vector

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

type esProviderInitializer struct{}

func (c *esProviderInitializer) ValidateConfig(config ProviderConfig) error {
	if len(config.collectionID) == 0 {
		return errors.New("[ES] collectionID is required")
	}
	if len(config.serviceName) == 0 {
		return errors.New("[ES] serviceName is required")
	}
	return nil
}

func (c *esProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	return &ESProvider{
		config: config,
		client: wrapper.NewClusterClient(wrapper.FQDNCluster{
			FQDN: config.serviceName,
			Host: config.serviceHost,
			Port: int64(config.servicePort),
		}),
	}, nil
}

type ESProvider struct {
	config ProviderConfig
	client wrapper.HttpClient
}

func (c *ESProvider) GetProviderType() string {
	return PROVIDER_TYPE_ES
}

func (d *ESProvider) QueryEmbedding(
	emb []float64,
	ctx wrapper.HttpContext,
	log log.Log,
	callback func(results []QueryResult, ctx wrapper.HttpContext, log log.Log, err error)) error {

	requestBody, err := json.Marshal(esQueryRequest{
		Source: Source{Excludes: []string{"embedding"}},
		Knn: knn{
			Field:       "embedding",
			QueryVector: emb,
			K:           d.config.topK,
		},
		Size: d.config.topK,
	})

	if err != nil {
		log.Errorf("[ES] Failed to marshal query embedding request body: %v", err)
		return err
	}

	return d.client.Post(
		fmt.Sprintf("/%s/_search", d.config.collectionID),
		[][2]string{
			{"Content-Type", "application/json"},
			{"Authorization", d.getCredentials()},
		},
		requestBody,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			log.Debugf("[ES] Query embedding response: %d, %s", statusCode, responseBody)
			results, err := d.parseQueryResponse(responseBody, log)
			if err != nil {
				err = fmt.Errorf("[ES] Failed to parse query response: %v", err)
			}
			callback(results, ctx, log, err)
		},
		d.config.timeout,
	)
}

// base64 编码 ES 身份认证字符串或使用 Apikey
func (d *ESProvider) getCredentials() string {
	if len(d.config.apiKey) != 0 {
		return fmt.Sprintf("ApiKey %s", d.config.apiKey)
	} else {
		credentials := fmt.Sprintf("%s:%s", d.config.esUsername, d.config.esPassword)
		encodedCredentials := base64.StdEncoding.EncodeToString([]byte(credentials))
		return fmt.Sprintf("Basic %s", encodedCredentials)
	}

}

func (d *ESProvider) UploadAnswerAndEmbedding(
	queryString string,
	queryEmb []float64,
	queryAnswer string,
	ctx wrapper.HttpContext,
	log log.Log,
	callback func(ctx wrapper.HttpContext, log log.Log, err error)) error {
	// 最少需要填写的参数为 index, embeddings 和 question
	// 下面是一个例子
	// POST /<index>/_doc
	// {
	// 	"embedding": [
	// 		  [1.1, 2.3, 3.2]
	// 	],
	// 	"question": [
	// 	  "你吃了吗？"
	// 	]
	// }
	requestBody, err := json.Marshal(esInsertRequest{
		Embedding: queryEmb,
		Question:  queryString,
		Answer:    queryAnswer,
	})
	if err != nil {
		log.Errorf("[ES] Failed to marshal upload embedding request body: %v", err)
		return err
	}

	return d.client.Post(
		fmt.Sprintf("/%s/_doc", d.config.collectionID),
		[][2]string{
			{"Content-Type", "application/json"},
			{"Authorization", d.getCredentials()},
		},
		requestBody,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			log.Debugf("[ES] statusCode:%d, responseBody:%s", statusCode, string(responseBody))
			callback(ctx, log, err)
		},
		d.config.timeout,
	)
}

type esInsertRequest struct {
	Embedding []float64 `json:"embedding"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
}

type knn struct {
	Field       string    `json:"field"`
	QueryVector []float64 `json:"query_vector"`
	K           int       `json:"k"`
}

type Source struct {
	Excludes []string `json:"excludes"`
}

type esQueryRequest struct {
	Source Source `json:"_source"`
	Knn    knn    `json:"knn"`
	Size   int    `json:"size"`
}

type esQueryResponse struct {
	Took     int  `json:"took"`
	TimedOut bool `json:"timed_out"`
	Hits     struct {
		Total struct {
			Value    int    `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		Hits []struct {
			Index  string                 `json:"_index"`
			ID     string                 `json:"_id"`
			Score  float64                `json:"_score"`
			Source map[string]interface{} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

func (d *ESProvider) parseQueryResponse(responseBody []byte, log log.Log) ([]QueryResult, error) {
	if log != nil {
		log.Infof("[ES] responseBody: %s", string(responseBody))
	}
	var queryResp esQueryResponse
	err := json.Unmarshal(responseBody, &queryResp)
	if err != nil {
		return []QueryResult{}, err
	}
	if log != nil {
		log.Debugf("[ES] queryResp Hits len: %d", len(queryResp.Hits.Hits))
	}
	if len(queryResp.Hits.Hits) == 0 {
		return nil, errors.New("no query results found in response")
	}
	results := make([]QueryResult, 0, queryResp.Hits.Total.Value)
	for _, hit := range queryResp.Hits.Hits {
		question, ok := hit.Source["question"].(string)
		if !ok {
			return nil, fmt.Errorf("[ES] hit %q has missing or non-string question", hit.ID)
		}
		answer, ok := hit.Source["answer"].(string)
		if !ok {
			return nil, fmt.Errorf("[ES] hit %q has missing or non-string answer", hit.ID)
		}
		result := QueryResult{
			Text:   question,
			Score:  hit.Score,
			Answer: answer,
		}
		results = append(results, result)
	}
	return results, nil
}
