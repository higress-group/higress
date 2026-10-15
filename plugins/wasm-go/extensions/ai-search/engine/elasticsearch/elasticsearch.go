package elasticsearch

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-search/engine"
)

type ElasticsearchSearch struct {
	client             wrapper.HttpClient
	index              string
	contentField       string
	semanticTextField  string
	linkField          string
	titleField         string
	start              int
	count              int
	timeoutMillisecond uint32
	username           string
	password           string
}

func NewElasticsearchSearch(config *gjson.Result, needReference bool) (*ElasticsearchSearch, error) {
	engine := &ElasticsearchSearch{}
	serviceName := config.Get("serviceName").String()
	if serviceName == "" {
		return nil, errors.New("serviceName not found")
	}
	servicePort := config.Get("servicePort").Int()
	if servicePort == 0 {
		if strings.HasSuffix(serviceName, ".static") {
			servicePort = 80
		} else if strings.HasSuffix(serviceName, ".dns") {
			servicePort = 443
		} else {
			return nil, errors.New("servicePort not found")
		}
	}
	engine.client = wrapper.NewClusterClient(wrapper.FQDNCluster{
		FQDN: serviceName,
		Port: servicePort,
	})
	engine.index = config.Get("index").String()
	if engine.index == "" {
		return nil, errors.New("index not found")
	}

	engine.contentField = config.Get("contentField").String()
	if engine.contentField == "" {
		return nil, errors.New("contentField not found")
	}
	engine.semanticTextField = config.Get("semanticTextField").String()
	if engine.semanticTextField == "" {
		return nil, errors.New("semanticTextField not found")
	}

	if needReference {
		engine.linkField = config.Get("linkField").String()
		if engine.linkField == "" {
			return nil, errors.New("linkField not found")
		}
		engine.titleField = config.Get("titleField").String()
		if engine.titleField == "" {
			return nil, errors.New("titleField not found")
		}
	}

	engine.timeoutMillisecond = uint32(config.Get("timeoutMillisecond").Uint())
	if engine.timeoutMillisecond == 0 {
		engine.timeoutMillisecond = 5000
	}
	engine.start = int(config.Get("start").Uint())
	engine.count = int(config.Get("count").Uint())
	if engine.count == 0 {
		engine.count = 10
	}

	engine.username = config.Get("username").String()
	engine.password = config.Get("password").String()

	return engine, nil
}

func (e ElasticsearchSearch) NeedExectue(ctx engine.SearchContext) bool {
	return ctx.EngineType == "private" || ctx.EngineType == ""
}

func (e ElasticsearchSearch) Client() wrapper.HttpClient {
	return e.client
}

func (e ElasticsearchSearch) generateAuthorizationHeader() string {
	return fmt.Sprintf(`Basic %s`, base64.StdEncoding.EncodeToString([]byte(e.username+":"+e.password)))
}

func (e ElasticsearchSearch) generateQueryBody(ctx engine.SearchContext) string {
	queryText := strings.Join(ctx.Querys, " ")
	// Build the query document with json.Marshal instead of fmt.Sprintf so that
	// the client-controlled query text cannot terminate the surrounding JSON
	// document or be reinterpreted through JSON escape sequences.
	query := map[string]interface{}{
		"_source": map[string]interface{}{
			"excludes": e.contentField,
		},
		"retriever": map[string]interface{}{
			"rrf": map[string]interface{}{
				"retrievers": []interface{}{
					map[string]interface{}{
						"standard": map[string]interface{}{
							"query": map[string]interface{}{
								"match": map[string]interface{}{
									e.contentField: queryText,
								},
							},
						},
					},
					map[string]interface{}{
						"standard": map[string]interface{}{
							"query": map[string]interface{}{
								"semantic": map[string]interface{}{
									"field": e.semanticTextField,
									"query": queryText,
								},
							},
						},
					},
				},
			},
		},
	}
	body, err := json.Marshal(query)
	if err != nil {
		// All values are strings and maps, so json.Marshal cannot fail; keep
		// the request body a well-formed JSON object if that ever changes.
		return "{}"
	}
	return string(body)
}

func (e ElasticsearchSearch) CallArgs(ctx engine.SearchContext) engine.CallArgs {
	queryBody := e.generateQueryBody(ctx)
	return engine.CallArgs{
		Method: http.MethodPost,
		Url:    fmt.Sprintf("/%s/_search?from=%d&size=%d", e.index, e.start, e.count),
		Headers: [][2]string{
			{"Content-Type", "application/json"},
			{"Authorization", e.generateAuthorizationHeader()},
		},
		Body:               []byte(queryBody),
		TimeoutMillisecond: e.timeoutMillisecond,
	}
}

func (e ElasticsearchSearch) ParseResult(ctx engine.SearchContext, response []byte) []engine.SearchResult {
	jsonObj := gjson.ParseBytes(response)
	var results []engine.SearchResult
	for _, hit := range jsonObj.Get("hits.hits").Array() {
		source := hit.Get("_source")
		result := engine.SearchResult{
			Title:   source.Get(e.titleField).String(),
			Link:    source.Get(e.linkField).String(),
			Content: source.Get(e.contentField).String(),
		}
		results = append(results, result)
	}
	return results
}
