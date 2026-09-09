package vector

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

type chromaProviderInitializer struct{}

func (c *chromaProviderInitializer) ValidateConfig(config ProviderConfig) error {
	if len(config.collectionID) == 0 {
		return errors.New("[Chroma] collectionID is required")
	}
	if len(config.serviceName) == 0 {
		return errors.New("[Chroma] serviceName is required")
	}
	return nil
}

func (c *chromaProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	return &ChromaProvider{
		config: config,
		client: wrapper.NewClusterClient(wrapper.FQDNCluster{
			FQDN: config.serviceName,
			Host: config.serviceHost,
			Port: int64(config.servicePort),
		}),
	}, nil
}

type ChromaProvider struct {
	config ProviderConfig
	client wrapper.HttpClient
}

func (c *ChromaProvider) GetProviderType() string {
	return PROVIDER_TYPE_CHROMA
}

func (d *ChromaProvider) QueryEmbedding(
	emb []float64,
	ctx wrapper.HttpContext,
	log log.Log,
	callback func(results []QueryResult, ctx wrapper.HttpContext, log log.Log, err error)) error {
	// 最少需要填写的参数为 collection_id, embeddings 和 ids
	// 下面是一个例子
	// {
	// 	"where": {}, // 用于 metadata 过滤，可选参数
	// 	"where_document": {}, // 用于 document 过滤，可选参数
	// 	"query_embeddings": [
	// 	  [1.1, 2.3, 3.2]
	// 	],
	// 	"limit": 5,
	// 	"include": [
	// 	  "metadatas", // 可选
	// 	  "documents", // 如果需要答案则需要
	// 	  "distances"
	// 	]
	// }

	requestBody, err := json.Marshal(chromaQueryRequest{
		QueryEmbeddings: []chromaEmbedding{emb},
		Limit:           d.config.topK,
		Include:         []string{"distances", "documents"},
	})

	if err != nil {
		log.Errorf("[Chroma] Failed to marshal query embedding request body: %v", err)
		return err
	}

	return d.client.Post(
		fmt.Sprintf("/api/v1/collections/%s/query", d.config.collectionID),
		[][2]string{
			{"Content-Type", "application/json"},
		},
		requestBody,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			log.Debugf("[Chroma] Query embedding response: %d, %s", statusCode, responseBody)
			results, err := d.parseQueryResponse(responseBody, log)
			if err != nil {
				err = fmt.Errorf("[Chroma] Failed to parse query response: %v", err)
			}
			callback(results, ctx, log, err)
		},
		d.config.timeout,
	)
}

func (d *ChromaProvider) UploadAnswerAndEmbedding(
	queryString string,
	queryEmb []float64,
	queryAnswer string,
	ctx wrapper.HttpContext,
	log log.Log,
	callback func(ctx wrapper.HttpContext, log log.Log, err error)) error {
	// 最少需要填写的参数为 collection_id, embeddings 和 ids
	// 下面是一个例子
	// {
	// 	"embeddings": [
	// 		  [1.1, 2.3, 3.2]
	// 	],
	// 	"ids": [
	// 	  "你吃了吗？"
	// 	],
	//  "documents": [
	//    "我吃了。"
	//  ]
	// }
	// 如果要添加 answer，则按照以下例子
	// {
	// 	"embeddings": [
	// 	  [1.1, 2.3, 3.2]
	// 	],
	// 	"documents": [
	// 	  "answer1"
	// 	],
	// 	"ids": [
	// 	  "id1"
	// 	]
	// }
	requestBody, err := json.Marshal(chromaInsertRequest{
		Embeddings: []chromaEmbedding{queryEmb},
		IDs:        []string{queryString}, // queryString 指的是用户查询的问题
		Documents:  []string{queryAnswer}, // queryAnswer 指的是用户查询的问题的答案
	})

	if err != nil {
		log.Errorf("[Chroma] Failed to marshal upload embedding request body: %v", err)
		return err
	}

	err = d.client.Post(
		fmt.Sprintf("/api/v1/collections/%s/add", d.config.collectionID),
		[][2]string{
			{"Content-Type", "application/json"},
		},
		requestBody,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			log.Debugf("[Chroma] statusCode:%d, responseBody:%s", statusCode, string(responseBody))
			callback(ctx, log, err)
		},
		d.config.timeout,
	)
	return err
}

type chromaEmbedding []float64
type chromaMetadataMap map[string]string
type chromaInsertRequest struct {
	Embeddings []chromaEmbedding   `json:"embeddings"`
	Metadatas  []chromaMetadataMap `json:"metadatas,omitempty"` // 可选参数
	Documents  []string            `json:"documents,omitempty"` // 可选参数
	IDs        []string            `json:"ids"`
}

type chromaQueryRequest struct {
	Where           map[string]string `json:"where,omitempty"`          // 可选参数
	WhereDocument   map[string]string `json:"where_document,omitempty"` // 可选参数
	QueryEmbeddings []chromaEmbedding `json:"query_embeddings"`
	Limit           int               `json:"limit"`
	Include         []string          `json:"include"`
}

type chromaQueryResponse struct {
	Ids        [][]string          `json:"ids"`                  // 第一维是 batch query，第二维是查询到的多个 ids
	Distances  [][]float64         `json:"distances,omitempty"`  // 与 Ids 一一对应
	Metadatas  []chromaMetadataMap `json:"metadatas,omitempty"`  // 可选参数
	Embeddings []chromaEmbedding   `json:"embeddings,omitempty"` // 可选参数
	Documents  [][]string          `json:"documents,omitempty"`  // 与 Ids 一一对应
	Uris       []string            `json:"uris,omitempty"`       // 可选参数
	Data       []interface{}       `json:"data,omitempty"`       // 可选参数
	Included   []string            `json:"included"`
}

func (d *ChromaProvider) parseQueryResponse(responseBody []byte, log log.Log) ([]QueryResult, error) {
	var queryResp chromaQueryResponse
	err := json.Unmarshal(responseBody, &queryResp)
	if err != nil {
		return nil, err
	}

	log.Debugf("[Chroma] queryResp Ids len: %d", len(queryResp.Ids))
	if len(queryResp.Ids) != 1 {
		return nil, fmt.Errorf("invalid query response: expected exactly one ids batch, got %d", len(queryResp.Ids))
	}
	ids := queryResp.Ids[0]
	if len(ids) == 0 {
		return nil, errors.New("no query results found in response")
	}
	if len(queryResp.Distances) != len(queryResp.Ids) {
		return nil, fmt.Errorf("invalid query response: distances batch count %d does not match ids batch count %d", len(queryResp.Distances), len(queryResp.Ids))
	}
	if len(queryResp.Documents) != len(queryResp.Ids) {
		return nil, fmt.Errorf("invalid query response: documents batch count %d does not match ids batch count %d", len(queryResp.Documents), len(queryResp.Ids))
	}
	distances := queryResp.Distances[0]
	documents := queryResp.Documents[0]
	if len(distances) != len(ids) {
		return nil, fmt.Errorf("invalid query response: distances element count %d does not match ids element count %d", len(distances), len(ids))
	}
	if len(documents) != len(ids) {
		return nil, fmt.Errorf("invalid query response: documents element count %d does not match ids element count %d", len(documents), len(ids))
	}

	results := make([]QueryResult, 0, len(ids))
	for i := range ids {
		result := QueryResult{
			Text:   ids[i],
			Score:  distances[i],
			Answer: documents[i],
		}
		results = append(results, result)
	}
	return results, nil
}
