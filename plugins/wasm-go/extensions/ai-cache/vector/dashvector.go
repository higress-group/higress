package vector

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

type dashVectorProviderInitializer struct {
}

func (d *dashVectorProviderInitializer) ValidateConfig(config ProviderConfig) error {
	if len(config.apiKey) == 0 {
		return errors.New("[DashVector] apiKey is required")
	}
	if len(config.collectionID) == 0 {
		return errors.New("[DashVector] collectionID is required")
	}
	if len(config.serviceName) == 0 {
		return errors.New("[DashVector] serviceName is required")
	}
	if len(config.serviceHost) == 0 {
		return errors.New("[DashVector] serviceHost is required")
	}
	return nil
}

func (d *dashVectorProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	return &DvProvider{
		config: config,
		client: wrapper.NewClusterClient(wrapper.FQDNCluster{
			FQDN: config.serviceName,
			Host: config.serviceHost,
			Port: int64(config.servicePort),
		}),
	}, nil
}

type DvProvider struct {
	config ProviderConfig
	client wrapper.HttpClient
}

func (d *DvProvider) GetProviderType() string {
	return PROVIDER_TYPE_DASH_VECTOR
}

// type embeddingRequest struct {
// 	Model      string `json:"model"`
// 	Input      input  `json:"input"`
// 	Parameters params `json:"parameters"`
// }

// type params struct {
// 	TextType string `json:"text_type"`
// }

// type input struct {
// 	Texts []string `json:"texts"`
// }

// queryResponse 定义查询响应的结构
type queryResponse struct {
	Code      int      `json:"code"`
	RequestID string   `json:"request_id"`
	Message   string   `json:"message"`
	Output    []result `json:"output"`
}

// queryRequest 定义查询请求的结构
type queryRequest struct {
	Vector        []float64 `json:"vector"`
	TopK          int       `json:"topk"`
	IncludeVector bool      `json:"include_vector"`
}

// result 定义查询结果的结构
type result struct {
	ID     string                 `json:"id"`
	Vector []float64              `json:"vector,omitempty"` // omitempty 使得如果 vector 是空，它将不会被序列化
	Fields map[string]interface{} `json:"fields"`
	Score  float64                `json:"score"`
}

func (d *DvProvider) constructEmbeddingQueryParameters(vector []float64) (string, []byte, [][2]string, error) {
	url := fmt.Sprintf("/v1/collections/%s/query", d.config.collectionID)

	requestData := queryRequest{
		Vector:        vector,
		TopK:          d.config.topK,
		IncludeVector: false,
	}

	requestBody, err := json.Marshal(requestData)
	if err != nil {
		return "", nil, nil, err
	}

	header := [][2]string{
		{"Content-Type", "application/json"},
		{"dashvector-auth-token", d.config.apiKey},
	}

	return url, requestBody, header, nil
}

func (d *DvProvider) parseQueryResponse(responseBody []byte) (queryResponse, error) {
	var queryResp queryResponse
	err := json.Unmarshal(responseBody, &queryResp)
	if err != nil {
		return queryResponse{}, err
	}
	return queryResp, nil
}

func (d *DvProvider) QueryEmbedding(
	emb []float64,
	ctx wrapper.HttpContext,
	log log.Log,
	callback func(results []QueryResult, ctx wrapper.HttpContext, log log.Log, err error)) error {
	url, body, headers, err := d.constructEmbeddingQueryParameters(emb)
	log.Debugf("url:%s, body:%s, headers:%v", url, string(body), headers)
	if err != nil {
		err = fmt.Errorf("failed to construct embedding query parameters: %v", err)
		return err
	}

	err = d.client.Post(url, headers, body,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			err = nil
			if statusCode != http.StatusOK {
				err = fmt.Errorf("failed to query embedding: %d", statusCode)
				callback(nil, ctx, log, err)
				return
			}
			log.Debugf("query embedding response: %d, %s", statusCode, responseBody)
			results, err := d.ParseQueryResponse(responseBody, ctx, log)
			if err != nil {
				err = fmt.Errorf("failed to parse query response: %v", err)
			}
			callback(results, ctx, log, err)
		},
		d.config.timeout)
	if err != nil {
		err = fmt.Errorf("failed to query embedding: %v", err)
	}
	return err
}

func getStringValue(fields map[string]interface{}, key string) string {
	val, ok := fields[key]
	if !ok {
		return ""
	}
	str, ok := val.(string)
	if !ok {
		return ""
	}
	return str
}

func (d *DvProvider) ParseQueryResponse(responseBody []byte, ctx wrapper.HttpContext, log log.Log) ([]QueryResult, error) {
	resp, err := d.parseQueryResponse(responseBody)
	if err != nil {
		return nil, err
	}

	if len(resp.Output) == 0 {
		return nil, errors.New("no query results found in response")
	}

	results := make([]QueryResult, 0, len(resp.Output))

	for _, output := range resp.Output {
		result := QueryResult{
			Text:      getStringValue(output.Fields, "query"),
			Embedding: output.Vector,
			Score:     output.Score,
			Answer:    getStringValue(output.Fields, "answer"),
		}
		results = append(results, result)
	}

	return results, nil
}

type document struct {
	Vector []float64         `json:"vector"`
	Fields map[string]string `json:"fields"`
}

type insertRequest struct {
	Docs []document `json:"docs"`
}

func (d *DvProvider) constructUploadParameters(emb []float64, queryString string, answer string) (string, []byte, [][2]string, error) {
	url := "/v1/collections/" + d.config.collectionID + "/docs"

	doc := document{
		Vector: emb,
		Fields: map[string]string{
			"query":  queryString,
			"answer": answer,
		},
	}

	requestBody, err := json.Marshal(insertRequest{Docs: []document{doc}})
	if err != nil {
		return "", nil, nil, err
	}

	header := [][2]string{
		{"Content-Type", "application/json"},
		{"dashvector-auth-token", d.config.apiKey},
	}

	return url, requestBody, header, err
}

func (d *DvProvider) UploadEmbedding(queryString string, queryEmb []float64, ctx wrapper.HttpContext, log log.Log, callback func(ctx wrapper.HttpContext, log log.Log, err error)) error {
	url, body, headers, err := d.constructUploadParameters(queryEmb, queryString, "")
	if err != nil {
		return err
	}
	err = d.client.Post(
		url,
		headers,
		body,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			log.Debugf("statusCode:%d, responseBody:%s", statusCode, string(responseBody))
			if statusCode != http.StatusOK {
				err = fmt.Errorf("failed to upload embedding: %d", statusCode)
			}
			callback(ctx, log, err)
		},
		d.config.timeout)
	return err
}

func (d *DvProvider) UploadAnswerAndEmbedding(queryString string, queryEmb []float64, queryAnswer string, ctx wrapper.HttpContext, log log.Log, callback func(ctx wrapper.HttpContext, log log.Log, err error)) error {
	url, body, headers, err := d.constructUploadParameters(queryEmb, queryString, queryAnswer)
	if err != nil {
		return err
	}
	err = d.client.Post(
		url,
		headers,
		body,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			log.Debugf("statusCode:%d, responseBody:%s", statusCode, string(responseBody))
			if statusCode != http.StatusOK {
				err = fmt.Errorf("failed to upload embedding: %d", statusCode)
			}
			callback(ctx, log, err)
		},
		d.config.timeout)
	return err
}
