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

var _ server.Tool = AroundSearchRequest{}

type AroundSearchRequest struct {
	Location string `json:"location" jsonschema_description:"中心点经度纬度"`
	Radius   string `json:"radius" jsonschema_description:"搜索半径"`
	Keywords string `json:"keywords" jsonschema_description:"搜索关键词"`
}

func (t AroundSearchRequest) Description() string {
	return "周边搜，根据用户传入关键词以及坐标location，搜索出radius半径范围的POI"
}

func (t AroundSearchRequest) InputSchema() map[string]any {
	return server.ToInputSchema(&AroundSearchRequest{})
}

func (t AroundSearchRequest) Create(params []byte) server.Tool {
	request := &AroundSearchRequest{}
	json.Unmarshal(params, &request)
	return request
}

func (t AroundSearchRequest) Call(ctx server.HttpContext, s server.Server) error {
	serverConfig := &config.AmapServerConfig{}
	s.GetConfig(serverConfig)
	if serverConfig.ApiKey == "" {
		return errors.New("amap API-KEY is not configured")
	}

	url := fmt.Sprintf("http://restapi.amap.com/v3/place/around?key=%s&location=%s&radius=%s&keywords=%s&source=ts_mcp", serverConfig.ApiKey, url.QueryEscape(t.Location), url.QueryEscape(t.Radius), url.QueryEscape(t.Keywords))
	return ctx.RouteCall(http.MethodGet, url,
		[][2]string{{"Accept", "application/json"}}, nil, func(statusCode int, responseHeaders [][2]string, responseBody []byte) {
			if statusCode != http.StatusOK {
				utils.OnMCPToolCallError(ctx, fmt.Errorf("around search call failed, status: %d", statusCode))
				return
			}
			utils.SendMCPToolTextResult(ctx, string(responseBody))
		})
}
