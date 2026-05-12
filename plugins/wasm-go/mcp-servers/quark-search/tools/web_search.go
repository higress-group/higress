// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"quark-search/config"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/server"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/tidwall/gjson"
)

var _ server.Tool = WebSearch{}

type SearchResult struct {
	Title   string
	Link    string
	Content string
}

func (result SearchResult) Valid() bool {
	return result.Title != "" && result.Link != "" && result.Content != ""
}

func (result SearchResult) Format() string {
	return fmt.Sprintf(`
## Title: %s

### Reference URL
%s

### Content
%s
`, result.Title, result.Link, result.Content)
}

type WebSearch struct {
	Query       string `json:"query" jsonschema_description:"Search query, please use Chinese" jsonschema:"example=黄金价格走势"`
	ContentMode string `json:"contentMode,omitempty" jsonschema_description:"Return the level of content detail, choose to use summary or full text" jsonschema:"enum=full,enum=summary,default=summary"`
	Number      uint32 `json:"number,omitempty" jsonschema_description:"Number of results" jsonschema:"default=5"`
}

// Description returns the description field for the MCP tool definition.
// This corresponds to the "description" field in the MCP tool JSON response,
// which provides a human-readable explanation of the tool's purpose and usage.
func (t WebSearch) Description() string {
	return `Performs a web search using the Quark Search API, ideal for general queries, news, articles, and online content.
Use this for broad information gathering, recent events, or when you need diverse web sources.
Because Quark search performs poorly for English searches, please use Chinese for the query parameters.`
}

// InputSchema returns the inputSchema field for the MCP tool definition.
// This corresponds to the "inputSchema" field in the MCP tool JSON response,
// which defines the JSON Schema for the tool's input parameters, including
// property types, descriptions, and required fields.
func (t WebSearch) InputSchema() map[string]any {
	return server.ToInputSchema(&WebSearch{})
}

// Create instantiates a new WebSearch tool instance based on the input parameters
// from an MCP tool call.
func (t WebSearch) Create(params []byte) server.Tool {
	webSearch := &WebSearch{
		ContentMode: "summary",
		Number:      5,
	}
	json.Unmarshal(params, &webSearch)
	return webSearch
}

// Call implements the core logic for handling an MCP tool call. This method is executed
// when the tool is invoked through the MCP framework. It processes the configured parameters,
// makes the actual API request to the service, parses the response,
// and formats the results to be returned to the caller.
func (t WebSearch) Call(ctx server.HttpContext, s server.Server) error {
	serverConfig := &config.QuarkServerConfig{}
	s.GetConfig(serverConfig)
	if serverConfig.ApiKey == "" {
		return errors.New("Quark search API key not configured")
	}
	return ctx.RouteCall(http.MethodGet, fmt.Sprintf("https://cloud-iqs.aliyuncs.com/search/genericSearch?query=%s", url.QueryEscape(t.Query)),
		[][2]string{{"Accept", "application/json"},
			{"X-API-Key", serverConfig.ApiKey}}, nil, func(statusCode int, responseHeaders [][2]string, responseBody []byte) {
			if statusCode != http.StatusOK {
				utils.OnMCPToolCallError(ctx, fmt.Errorf("quark search call failed, status: %d", statusCode))
				return
			}
			jsonObj := gjson.ParseBytes(responseBody)
			var results []string
			for index, item := range jsonObj.Get("pageItems").Array() {
				var content string
				if t.ContentMode == "full" {
					content = item.Get("markdownText").String()
					if content == "" {
						content = item.Get("mainText").String()
					}
				} else if t.ContentMode == "summary" {
					content = item.Get("snippet").String()
				}
				result := SearchResult{
					Title:   item.Get("title").String(),
					Link:    item.Get("link").String(),
					Content: content,
				}
				if result.Valid() && index < int(t.Number) {
					results = append(results, result.Format())
				}
			}
			utils.SendMCPToolTextResult(ctx, fmt.Sprintf("# Search Results\n\n%s", strings.Join(results, "\n\n")))
		})
}
