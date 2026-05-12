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

var _ server.Tool = SearchDetailRequest{}

type SearchDetailRequest struct {
	ID string `json:"id" jsonschema_description:"关键词搜或者周边搜获取到的POI ID"`
}

func (t SearchDetailRequest) Description() string {
	return "查询关键词搜或者周边搜获取到的POI ID的详细信息"
}

func (t SearchDetailRequest) InputSchema() map[string]any {
	return server.ToInputSchema(&SearchDetailRequest{})
}

func (t SearchDetailRequest) Create(params []byte) server.Tool {
	request := &SearchDetailRequest{}
	json.Unmarshal(params, &request)
	return request
}

func (t SearchDetailRequest) Call(ctx server.HttpContext, s server.Server) error {
	serverConfig := &config.AmapServerConfig{}
	s.GetConfig(serverConfig)
	if serverConfig.ApiKey == "" {
		return errors.New("amap API-KEY is not configured")
	}

	url := fmt.Sprintf("http://restapi.amap.com/v3/place/detail?id=%s&key=%s&source=ts_mcp", url.QueryEscape(t.ID), serverConfig.ApiKey)
	return ctx.RouteCall(http.MethodGet, url,
		[][2]string{{"Accept", "application/json"}}, nil, func(statusCode int, responseHeaders [][2]string, responseBody []byte) {
			if statusCode != http.StatusOK {
				utils.OnMCPToolCallError(ctx, fmt.Errorf("search detail call failed, status: %d", statusCode))
				return
			}
			utils.SendMCPToolTextResult(ctx, string(responseBody))
		})
}
