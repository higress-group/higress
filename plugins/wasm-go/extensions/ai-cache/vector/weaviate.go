package vector

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

type weaviateProviderInitializer struct{}

func (c *weaviateProviderInitializer) ValidateConfig(config ProviderConfig) error {
	if len(config.collectionID) == 0 {
		return errors.New("[Weaviate] collectionID is required")
	}
	if len(config.serviceName) == 0 {
		return errors.New("[Weaviate] serviceName is required")
	}
	return nil
}

func (c *weaviateProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	return &WeaviateProvider{
		config: config,
		client: wrapper.NewClusterClient(wrapper.FQDNCluster{
			FQDN: config.serviceName,
			Host: config.serviceHost,
			Port: int64(config.servicePort),
		}),
	}, nil
}

type WeaviateProvider struct {
	config ProviderConfig
	client wrapper.HttpClient
}

func (c *WeaviateProvider) GetProviderType() string {
	return PROVIDER_TYPE_WEAVIATE
}

func (d *WeaviateProvider) QueryEmbedding(
	emb []float64,
	ctx wrapper.HttpContext,
	log log.Log,
	callback func(results []QueryResult, ctx wrapper.HttpContext, log log.Log, err error)) error {
	consumer := ConsumerFromContext(ctx)
	if consumer == "" {
		return ErrConsumerRequired
	}
	// 最少需要填写的参数为 class, vector
	// 下面是一个例子
	// {"query": "{ Get { Higress ( limit: 2 nearVector: { vector: [0.1, 0.2, 0.3] } ) { question _additional { distance } } } }"}
	embString, err := json.Marshal(emb)
	if err != nil {
		log.Errorf("[Weaviate] Failed to marshal query embedding: %v", err)
		return err
	}
	consumerString, err := json.Marshal(consumer)
	if err != nil {
		return err
	}
	// 这里默认按照 distance 进行升序，所以不用再次排序
	graphql := fmt.Sprintf(`
	{
	  Get {
	    %s (
	      limit: %d
	      nearVector: {
	        vector: %s
	      }
	      where: {
	        path: ["consumer"]
	        operator: Equal
	        valueString: %s
	      }
	    ) {
		  question
		  answer
		  consumer
	      _additional {
	        distance
	      }
	    }
	  }
	}
	`, d.config.collectionID, d.config.topK, embString, consumerString)

	requestBody, err := json.Marshal(weaviateQueryRequest{
		Query: graphql,
	})

	if err != nil {
		log.Errorf("[Weaviate] Failed to marshal query embedding request body: %v", err)
		return err
	}

	err = d.client.Post(
		"/v1/graphql",
		[][2]string{
			{"Content-Type", "application/json"},
			{"Authorization", fmt.Sprintf("Bearer %s", d.config.apiKey)},
		},
		requestBody,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			log.Debugf("[Weaviate] Query embedding response: %d, %s", statusCode, responseBody)
			results, err := d.parseQueryResponse(responseBody, log)
			if err != nil {
				err = fmt.Errorf("[Weaviate] Failed to parse query response: %v", err)
			}
			callback(results, ctx, log, err)
		},
		d.config.timeout,
	)
	return err
}

func (d *WeaviateProvider) UploadAnswerAndEmbedding(
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
	// 最少需要填写的参数为 class, vector 和 question 和 answer
	// 下面是一个例子
	// {"class": "Higress", "vector": [0.1, 0.2, 0.3], "properties": {"question": "这里是问题", "answer": "这里是答案"}}
	requestBody, err := json.Marshal(weaviateInsertRequest{
		Class:      d.config.collectionID,
		Vector:     queryEmb,
		Properties: weaviateProperties{Question: queryString, Answer: queryAnswer, Consumer: consumer}, // queryString 指的是用户查询的问题
	})

	if err != nil {
		log.Errorf("[Weaviate] Failed to marshal upload embedding request body: %v", err)
		return err
	}

	return d.client.Post(
		"/v1/objects",
		[][2]string{
			{"Content-Type", "application/json"},
			{"Authorization", fmt.Sprintf("Bearer %s", d.config.apiKey)},
		},
		requestBody,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			log.Debugf("[Weaviate] statusCode: %d, responseBody: %s", statusCode, string(responseBody))
			callback(ctx, log, err)
		},
		d.config.timeout,
	)
}

type weaviateProperties struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Consumer string `json:"consumer"`
}

type weaviateInsertRequest struct {
	Class      string             `json:"class"`
	Vector     []float64          `json:"vector"`
	Properties weaviateProperties `json:"properties"`
}

type weaviateQueryRequest struct {
	Query string `json:"query"`
}

func (d *WeaviateProvider) parseQueryResponse(responseBody []byte, log log.Log) ([]QueryResult, error) {
	log.Infof("[Weaviate] queryResp: %s", string(responseBody))

	if !gjson.GetBytes(responseBody, fmt.Sprintf("data.Get.%s.0._additional.distance", d.config.collectionID)).Exists() {
		log.Errorf("[Weaviate] No distance found in response body: %s", responseBody)
		return nil, errors.New("[Weaviate] No distance found in response body")
	}

	if !gjson.GetBytes(responseBody, fmt.Sprintf("data.Get.%s.0.question", d.config.collectionID)).Exists() {
		log.Errorf("[Weaviate] No question found in response body: %s", responseBody)
		return nil, errors.New("[Weaviate] No question found in response body")
	}

	if !gjson.GetBytes(responseBody, fmt.Sprintf("data.Get.%s.0.answer", d.config.collectionID)).Exists() {
		log.Errorf("[Weaviate] No answer found in response body: %s", responseBody)
		return nil, errors.New("[Weaviate] No answer found in response body")
	}

	resultNum := gjson.GetBytes(responseBody, fmt.Sprintf("data.Get.%s.#", d.config.collectionID)).Int()
	results := make([]QueryResult, 0, resultNum)
	for i := 0; i < int(resultNum); i++ {
		result := QueryResult{
			Text:     gjson.GetBytes(responseBody, fmt.Sprintf("data.Get.%s.%d.question", d.config.collectionID, i)).String(),
			Score:    gjson.GetBytes(responseBody, fmt.Sprintf("data.Get.%s.%d._additional.distance", d.config.collectionID, i)).Float(),
			Answer:   gjson.GetBytes(responseBody, fmt.Sprintf("data.Get.%s.%d.answer", d.config.collectionID, i)).String(),
			Consumer: gjson.GetBytes(responseBody, fmt.Sprintf("data.Get.%s.%d.consumer", d.config.collectionID, i)).String(),
		}
		results = append(results, result)
	}

	return results, nil
}
