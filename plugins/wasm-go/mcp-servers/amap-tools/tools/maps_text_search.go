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

	"amap-tools/config"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/server"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
)

var _ server.Tool = TextSearchRequest{}

type TextSearchRequest struct {
	Keywords  string `json:"keywords" jsonschema_description:"搜索关键词"`
	City      string `json:"city" jsonschema_description:"查询城市"`
	Citylimit string `json:"citylimit" jsonschema_description:"是否强制限制在设置的城市内搜索，默认值为false"`
}

func (t TextSearchRequest) Description() string {
	return "关键词搜，根据用户传入关键词，搜索出相关的POI，城市名称可以通过基于ip定位位置的mcp工具获取"
}

func (t TextSearchRequest) InputSchema() map[string]any {
	return server.ToInputSchema(&TextSearchRequest{})
}

func (t TextSearchRequest) Create(params []byte) server.Tool {
	request := &TextSearchRequest{}
	json.Unmarshal(params, &request)
	return request
}

func (t TextSearchRequest) Call(ctx server.HttpContext, s server.Server) error {
	serverConfig := &config.AmapServerConfig{}
	s.GetConfig(serverConfig)
	if serverConfig.ApiKey == "" {
		return errors.New("amap API-KEY is not configured")
	}

	url := fmt.Sprintf("http://restapi.amap.com/v3/place/text?key=%s&keywords=%s&city=%s&citylimit=%s&source=ts_mcp", serverConfig.ApiKey, url.QueryEscape(t.Keywords), url.QueryEscape(t.City), url.QueryEscape(t.Citylimit))
	return ctx.RouteCall(http.MethodGet, url,
		[][2]string{{"Accept", "application/json"}}, nil, func(statusCode int, responseHeaders [][2]string, responseBody []byte) {
			if statusCode != http.StatusOK {
				utils.OnMCPToolCallError(ctx, fmt.Errorf("text search call failed, status: %d", statusCode))
				return
			}
			utils.SendMCPToolTextResult(ctx, string(responseBody))
		})
}
