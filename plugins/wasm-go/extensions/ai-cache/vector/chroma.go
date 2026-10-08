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

	if log != nil {
		log.Debugf("[Chroma] queryResp Ids len: %d", len(queryResp.Ids))
	}

	// Chroma reports request failures such as an unknown collection as a body
	// like {"error":"..."}, and reports a query that matched nothing as
	// {"ids":[]}. Both leave queryResp.Ids empty, so indexing queryResp.Ids[0]
	// without validating the shape first panics with "index out of range".
	// That panic happens inside the HTTP callback created by QueryEmbedding, so
	// it aborts the callback before it can invoke handleQueryResults and the
	// request ai-cache paused to await the vector search never resumes. Return
	// a descriptive error instead: handleQueryResults forwards it to
	// handleInternalError, which resumes the request.
	//
	// Distances and Documents are declared with `omitempty` in
	// chromaQueryResponse, so they may legitimately be absent or shorter than
	// Ids even for a well-formed response; they must be length-checked too.
	ids := queryResp.Ids
	if len(ids) == 0 || len(ids[0]) == 0 {
		return nil, errors.New("[Chroma] no query results found in response")
	}
	n := len(ids[0])

	// Flatten the optional per-batch slices, tolerating their absence so the
	// length checks below can report the actual shape instead of panicking.
	var distances []float64
	if len(queryResp.Distances) > 0 {
		distances = queryResp.Distances[0]
	}
	var documents []string
	if len(queryResp.Documents) > 0 {
		documents = queryResp.Documents[0]
	}

	if len(distances) < n {
		return nil, fmt.Errorf("[Chroma] distances are missing or shorter than ids: got %d, want at least %d", len(distances), n)
	}
	if len(documents) < n {
		return nil, fmt.Errorf("[Chroma] documents are missing or shorter than ids: got %d, want at least %d", len(documents), n)
	}

	results := make([]QueryResult, 0, n)
	for i := 0; i < n; i++ {
		results = append(results, QueryResult{
			Text:   ids[0][i],
			Score:  distances[i],
			Answer: documents[i],
		})
	}
	return results, nil
}
