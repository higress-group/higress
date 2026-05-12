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

var _ server.Tool = TransitIntegratedRequest{}

type TransitIntegratedRequest struct {
	Origin      string `json:"origin" jsonschema_description:"出发点经纬度，坐标格式为：经度，纬度"`
	Destination string `json:"destination" jsonschema_description:"目的地经纬度，坐标格式为：经度，纬度"`
	City        string `json:"city" jsonschema_description:"公共交通规划起点城市"`
	Cityd       string `json:"cityd" jsonschema_description:"公共交通规划终点城市"`
}

func (t TransitIntegratedRequest) Description() string {
	return "公交路径规划 API 可以根据用户起终点经纬度坐标规划综合各类公共（火车、公交、地铁）交通方式的通勤方案，并且返回通勤方案的数据，跨城场景下必须传起点城市与终点城市, 起点城市名称可以通过基于ip定位位置的mcp工具获取"
}

func (t TransitIntegratedRequest) InputSchema() map[string]any {
	return server.ToInputSchema(&TransitIntegratedRequest{})
}

func (t TransitIntegratedRequest) Create(params []byte) server.Tool {
	request := &TransitIntegratedRequest{}
	json.Unmarshal(params, &request)
	return request
}

func (t TransitIntegratedRequest) Call(ctx server.HttpContext, s server.Server) error {
	serverConfig := &config.AmapServerConfig{}
	s.GetConfig(serverConfig)
	if serverConfig.ApiKey == "" {
		return errors.New("amap API-KEY is not configured")
	}

	url := fmt.Sprintf("http://restapi.amap.com/v3/direction/transit/integrated?key=%s&origin=%s&destination=%s&city=%s&cityd=%s&source=ts_mcp", serverConfig.ApiKey, url.QueryEscape(t.Origin), url.QueryEscape(t.Destination), url.QueryEscape(t.City), url.QueryEscape(t.Cityd))
	return ctx.RouteCall(http.MethodGet, url,
		[][2]string{{"Accept", "application/json"}}, nil, func(statusCode int, responseHeaders [][2]string, responseBody []byte) {
			if statusCode != http.StatusOK {
				utils.OnMCPToolCallError(ctx, fmt.Errorf("transit integrated call failed, status: %d", statusCode))
				return
			}
			utils.SendMCPToolTextResult(ctx, string(responseBody))
		})
}
