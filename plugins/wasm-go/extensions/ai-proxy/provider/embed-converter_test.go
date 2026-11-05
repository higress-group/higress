package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseEmbeddingsInput(t *testing.T) {
	// string form
	input := parseEmbeddingsInput(json.RawMessage(`"hello"`))
	assert.Equal(t, []string{"hello"}, input)

	// array form
	input = parseEmbeddingsInput(json.RawMessage(`["a","b"]`))
	assert.Equal(t, []string{"a", "b"}, input)

	// missing / invalid
	assert.Nil(t, parseEmbeddingsInput(nil))
	assert.Nil(t, parseEmbeddingsInput(json.RawMessage(`42`)))
}

func TestBuildOpenAIEmbeddingsResponse(t *testing.T) {
	body := []byte(`{"Response":{"RequestId":"req-1","Embeddings":[
		{"Embedding":[0.1,0.2],"Index":0},
		{"Embedding":[0.3,0.4],"Index":1}],"Usage":{"TotalTokens":7}}}`)
	hunyuanResponse := &hunyuanEmbeddingsResponse{}
	err := json.Unmarshal(body, hunyuanResponse)
	assert.NoError(t, err)

	response := buildOpenAIEmbeddingsResponse("hunyuan-embedding", hunyuanResponse)
	assert.Equal(t, "list", response.Object)
	assert.Equal(t, "hunyuan-embedding", response.Model)
	assert.Len(t, response.Data, 2)
	assert.Equal(t, "embedding", response.Data[0].Object)
	assert.InDeltaSlice(t, []float64{0.1, 0.2}, response.Data[0].Embedding, 1e-9)
	assert.Equal(t, 0, response.Data[0].Index)
	assert.Equal(t, 1, response.Data[1].Index)
	assert.Equal(t, 7, response.Usage.PromptTokens)
	assert.Equal(t, 7, response.Usage.TotalTokens)

	// round-trips to valid OpenAI-shaped JSON
	encoded, err := json.Marshal(response)
	assert.NoError(t, err)
	assert.Contains(t, string(encoded), `"object":"list"`)
	assert.Contains(t, string(encoded), `"prompt_tokens":7`)
}
