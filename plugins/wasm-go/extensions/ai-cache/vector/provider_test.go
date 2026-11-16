package vector

import (
	"net/http"
	"testing"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type consumerTestContext struct {
	wrapper.HttpContext
	consumer string
}

func (c consumerTestContext) GetContext(key string) interface{} {
	if key == ConsumerContextKey {
		return c.consumer
	}
	return nil
}

type recordingClient struct {
	wrapper.HttpClient
	method string
	body   []byte
}

func (c *recordingClient) Post(_ string, _ [][2]string, body []byte, _ wrapper.ResponseCallback, _ ...uint32) error {
	c.method = http.MethodPost
	c.body = append([]byte(nil), body...)
	return nil
}

func (c *recordingClient) Put(_ string, _ [][2]string, body []byte, _ wrapper.ResponseCallback, _ ...uint32) error {
	c.method = http.MethodPut
	c.body = append([]byte(nil), body...)
	return nil
}

type discardLog struct {
	log.Log
}

func (discardLog) Debugf(string, ...interface{}) {}
func (discardLog) Infof(string, ...interface{})  {}
func (discardLog) Errorf(string, ...interface{}) {}

type tenantProvider interface {
	Provider
	EmbeddingQuerier
	AnswerAndEmbeddingUploader
}

func TestProvidersUploadAndFilterByConsumer(t *testing.T) {
	providerConfig := ProviderConfig{topK: 1, collectionID: "Higress"}
	for _, tc := range []struct {
		name        string
		newProvider func(wrapper.HttpClient) tenantProvider
		queryPath   string
		uploadPath  string
		queryJSON   bool
	}{
		{name: "dashvector", newProvider: func(client wrapper.HttpClient) tenantProvider {
			return &DvProvider{config: providerConfig, client: client}
		}, queryPath: "filter", uploadPath: "docs.0.fields.consumer"},
		{name: "chroma", newProvider: func(client wrapper.HttpClient) tenantProvider {
			return &ChromaProvider{config: providerConfig, client: client}
		}, queryPath: "where.consumer", uploadPath: "metadatas.0.consumer"},
		{name: "elasticsearch", newProvider: func(client wrapper.HttpClient) tenantProvider {
			return &ESProvider{config: providerConfig, client: client}
		}, queryPath: "knn.filter.term.consumer", uploadPath: "consumer"},
		{name: "weaviate", newProvider: func(client wrapper.HttpClient) tenantProvider {
			return &WeaviateProvider{config: providerConfig, client: client}
		}, queryPath: "query", uploadPath: "properties.consumer", queryJSON: true},
		{name: "pinecone", newProvider: func(client wrapper.HttpClient) tenantProvider {
			return &pineconeProvider{config: providerConfig, client: client}
		}, queryPath: "filter.consumer.$eq", uploadPath: "vectors.0.metadata.consumer"},
		{name: "qdrant", newProvider: func(client wrapper.HttpClient) tenantProvider {
			return &qdrantProvider{config: providerConfig, client: client}
		}, queryPath: "filter.must.0.match.value", uploadPath: "points.0.payload.consumer"},
		{name: "milvus", newProvider: func(client wrapper.HttpClient) tenantProvider {
			return &milvusProvider{config: providerConfig, client: client}
		}, queryPath: "filter", uploadPath: "data.0.consumer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &recordingClient{}
			provider := tc.newProvider(client)
			authenticatedCtx := consumerTestContext{consumer: "consumer-a"}
			require.NoError(t, provider.QueryEmbedding([]float64{0.1}, authenticatedCtx, discardLog{}, nil))
			queryValue := gjson.GetBytes(client.body, tc.queryPath).String()
			if tc.queryJSON || tc.name == "dashvector" || tc.name == "milvus" {
				require.Contains(t, queryValue, "consumer-a")
			} else {
				require.Equal(t, "consumer-a", queryValue)
			}
			if tc.name == "weaviate" {
				require.Contains(t, queryValue, `path: ["consumer"]`)
			}

			client.body = nil
			require.NoError(t, provider.UploadAnswerAndEmbedding("question", []float64{0.1}, "answer", authenticatedCtx, discardLog{}, nil))
			require.Equal(t, "consumer-a", gjson.GetBytes(client.body, tc.uploadPath).String())

			client.body = nil
			anonymousCtx := consumerTestContext{}
			require.ErrorIs(t, provider.QueryEmbedding([]float64{0.1}, anonymousCtx, discardLog{}, nil), ErrConsumerRequired)
			require.ErrorIs(t, provider.UploadAnswerAndEmbedding("question", []float64{0.1}, "answer", anonymousCtx, discardLog{}, nil), ErrConsumerRequired)
			require.Nil(t, client.body)
		})
	}
}

func TestQueryResponsesRetainConsumerMetadata(t *testing.T) {
	for _, tc := range []struct {
		name  string
		parse func([]byte) ([]QueryResult, error)
		body  string
	}{
		{name: "dashvector", parse: func(body []byte) ([]QueryResult, error) {
			return (&DvProvider{}).ParseQueryResponse(body, nil, discardLog{})
		}, body: `{"output":[{"fields":{"query":"q","answer":"a","consumer":"consumer-a"},"score":0.9}]}`},
		{name: "chroma", parse: func(body []byte) ([]QueryResult, error) {
			return (&ChromaProvider{}).parseQueryResponse(body, discardLog{})
		}, body: `{"ids":[["q"]],"distances":[[0.1]],"documents":[["a"]],"metadatas":[[{"consumer":"consumer-a"}]]}`},
		{name: "elasticsearch", parse: func(body []byte) ([]QueryResult, error) { return (&ESProvider{}).parseQueryResponse(body, nil) }, body: `{"hits":{"total":{"value":1},"hits":[{"_score":0.9,"_source":{"question":"q","answer":"a","consumer":"consumer-a"}}]}}`},
		{name: "weaviate", parse: func(body []byte) ([]QueryResult, error) {
			return (&WeaviateProvider{config: ProviderConfig{collectionID: "Higress"}}).parseQueryResponse(body, discardLog{})
		}, body: `{"data":{"Get":{"Higress":[{"question":"q","answer":"a","consumer":"consumer-a","_additional":{"distance":0.1}}]}}}`},
		{name: "pinecone", parse: func(body []byte) ([]QueryResult, error) {
			return (&pineconeProvider{}).parseQueryResponse(body, discardLog{})
		}, body: `{"matches":[{"score":0.9,"metadata":{"question":"q","answer":"a","consumer":"consumer-a"}}]}`},
		{name: "qdrant", parse: func(body []byte) ([]QueryResult, error) {
			return (&qdrantProvider{}).parseQueryResponse(body, discardLog{})
		}, body: `{"result":[{"score":0.9,"payload":{"question":"q","answer":"a","consumer":"consumer-a"}}]}`},
		{name: "milvus", parse: func(body []byte) ([]QueryResult, error) {
			return (&milvusProvider{}).parseQueryResponse(body, discardLog{})
		}, body: `{"data":[{"distance":0.1,"question":"q","answer":"a","consumer":"consumer-a"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results, err := tc.parse([]byte(tc.body))
			require.NoError(t, err)
			require.Len(t, results, 1)
			require.Equal(t, "consumer-a", results[0].Consumer)
		})
	}
}
