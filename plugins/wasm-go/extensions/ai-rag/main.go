package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"ai-rag/dashscope"
	"ai-rag/dashvector"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-rag",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
	)
}

type AIRagConfig struct {
	DashScopeClient      wrapper.HttpClient
	DashScopeAPIKey      string
	DashVectorClient     wrapper.HttpClient
	DashVectorAPIKey     string
	DashVectorCollection string
	DashVectorTopK       int32
	DashVectorThreshold  float64
	DashVectorField      string
}

type Request struct {
	Model            string    `json:"model"`
	Messages         []Message `json:"messages"`
	FrequencyPenalty float64   `json:"frequency_penalty"`
	PresencePenalty  float64   `json:"presence_penalty"`
	Stream           bool      `json:"stream"`
	Temperature      float64   `json:"temperature"`
	Topp             int32     `json:"top_p"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func parseConfig(json gjson.Result, config *AIRagConfig, log log.Log) error {
	checkList := []string{
		"dashscope.apiKey",
		"dashscope.serviceFQDN",
		"dashscope.servicePort",
		"dashscope.serviceHost",
		"dashvector.apiKey",
		"dashvector.collection",
		"dashvector.serviceFQDN",
		"dashvector.servicePort",
		"dashvector.serviceHost",
		"dashvector.topk",
		"dashvector.threshold",
		"dashvector.field",
	}
	for _, checkEntry := range checkList {
		if !json.Get(checkEntry).Exists() {
			return fmt.Errorf("%s not found in plugin config!", checkEntry)
		}
	}
	config.DashScopeAPIKey = json.Get("dashscope.apiKey").String()

	config.DashScopeClient = wrapper.NewClusterClient(wrapper.FQDNCluster{
		FQDN: json.Get("dashscope.serviceFQDN").String(),
		Port: json.Get("dashscope.servicePort").Int(),
		Host: json.Get("dashscope.serviceHost").String(),
	})
	config.DashVectorAPIKey = json.Get("dashvector.apiKey").String()
	config.DashVectorCollection = json.Get("dashvector.collection").String()
	config.DashVectorClient = wrapper.NewClusterClient(wrapper.FQDNCluster{
		FQDN: json.Get("dashvector.serviceFQDN").String(),
		Port: json.Get("dashvector.servicePort").Int(),
		Host: json.Get("dashvector.serviceHost").String(),
	})
	config.DashVectorTopK = int32(json.Get("dashvector.topk").Int())
	config.DashVectorThreshold = json.Get("dashvector.threshold").Float()
	config.DashVectorField = json.Get("dashvector.field").String()
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config AIRagConfig, log log.Log) types.Action {
	ctx.DisableReroute()
	proxywasm.RemoveHttpRequestHeader("content-length")
	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config AIRagConfig, body []byte, log log.Log) types.Action {
	var rawRequest Request
	_ = json.Unmarshal(body, &rawRequest)
	messageLength := len(rawRequest.Messages)
	if messageLength == 0 {
		return types.ActionContinue
	}
	rawContent := rawRequest.Messages[messageLength-1].Content
	requestEmbedding := dashscope.Request{
		Model: "text-embedding-v2",
		Input: dashscope.Input{
			Texts: []string{rawContent},
		},
		Parameter: dashscope.Parameter{
			TextType: "query",
		},
	}
	headers := [][2]string{{"Content-Type", "application/json"}, {"Authorization", "Bearer " + config.DashScopeAPIKey}}
	reqEmbeddingSerialized, _ := json.Marshal(requestEmbedding)
	err := config.DashScopeClient.Post(
		"/api/v1/services/embeddings/text-embedding/text-embedding",
		headers,
		reqEmbeddingSerialized,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			if statusCode != http.StatusOK {
				log.Debugf("DashScope embedding request failed, statusCode: %d", statusCode)
				proxywasm.ResumeHttpRequest()
				return
			}
			var responseEmbedding dashscope.Response
			if err := json.Unmarshal(responseBody, &responseEmbedding); err != nil {
				log.Debugf("DashScope embedding response unmarshal failed: %s", err.Error())
				proxywasm.ResumeHttpRequest()
				return
			}
			if len(responseEmbedding.Output.Embeddings) == 0 {
				log.Debug("DashScope embedding response has no embeddings")
				proxywasm.ResumeHttpRequest()
				return
			}
			embedding := responseEmbedding.Output.Embeddings[0].Embedding
			if len(embedding) == 0 {
				log.Debug("DashScope embedding vector is empty")
				proxywasm.ResumeHttpRequest()
				return
			}
			requestQuery := dashvector.Request{
				TopK:         config.DashVectorTopK,
				OutputFileds: []string{config.DashVectorField},
				Vector:       embedding,
			}
			requestQuerySerialized, _ := json.Marshal(requestQuery)
			err := config.DashVectorClient.Post(
				fmt.Sprintf("/v1/collections/%s/query", config.DashVectorCollection),
				[][2]string{{"Content-Type", "application/json"}, {"dashvector-auth-token", config.DashVectorAPIKey}},
				requestQuerySerialized,
				func(statusCode int, responseHeaders http.Header, responseBody []byte) {
					if statusCode != http.StatusOK {
						log.Debugf("DashVector query failed, statusCode: %d", statusCode)
						proxywasm.ResumeHttpRequest()
						return
					}
					var response dashvector.Response
					if err := json.Unmarshal(responseBody, &response); err != nil {
						log.Debugf("DashVector query response unmarshal failed: %s", err.Error())
						proxywasm.ResumeHttpRequest()
						return
					}
					recallDocIds := []string{}
					recallDocs := []string{}
					for _, output := range response.Output {
						log.Debugf("Score: %f, Doc: %s", output.Score, output.Fields.Raw)
						if output.Score <= float32(config.DashVectorThreshold) {
							recallDocs = append(recallDocs, output.Fields.Raw)
							recallDocIds = append(recallDocIds, output.ID)
						}
					}
					if len(recallDocs) > 0 {
						rawRequest.Messages = rawRequest.Messages[:messageLength-1]
						traceStr := strings.Join(recallDocIds, ", ")
						proxywasm.SetProperty([]string{"trace_span_tag.rag_docs"}, []byte(traceStr))
						for _, doc := range recallDocs {
							rawRequest.Messages = append(rawRequest.Messages, Message{"user", doc})
						}
						rawRequest.Messages = append(rawRequest.Messages, Message{"user", fmt.Sprintf("现在，请回答以下问题：\n%s", rawContent)})
						newBody, _ := json.Marshal(rawRequest)
						proxywasm.ReplaceHttpRequestBody(newBody)
						ctx.SetContext("x-envoy-rag-recall", true)
					}
					proxywasm.ResumeHttpRequest()
				},
			)
			if err != nil {
				log.Debugf("DashVector query dispatch failed: %s", err.Error())
				proxywasm.ResumeHttpRequest()
			}
		},
		50000,
	)
	if err != nil {
		log.Debugf("DashScope embedding dispatch failed: %s", err.Error())
		return types.ActionContinue
	}
	return types.ActionPause
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config AIRagConfig, log log.Log) types.Action {
	recall, ok := ctx.GetContext("x-envoy-rag-recall").(bool)
	if ok && recall {
		proxywasm.AddHttpResponseHeader("x-envoy-rag-recall", "true")
	} else {
		proxywasm.AddHttpResponseHeader("x-envoy-rag-recall", "false")
	}
	return types.ActionContinue
}
