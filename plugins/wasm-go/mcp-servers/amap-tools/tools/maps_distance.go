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

var _ server.Tool = DistanceRequest{}

type DistanceRequest struct {
	Origins     string `json:"origins" jsonschema_description:"起点经度，纬度，可以传多个坐标，使用分号隔离，比如120,30;120,31，坐标格式为：经度，纬度"`
	Destination string `json:"destination" jsonschema_description:"终点经度，纬度，坐标格式为：经度，纬度"`
	Type        string `json:"type" jsonschema_description:"距离测量类型,1代表驾车距离测量，0代表直线距离测量，3步行距离测量"`
}

func (t DistanceRequest) Description() string {
	return "距离测量 API 可以测量两个经纬度坐标之间的距离,支持驾车、步行以及球面距离测量"
}

func (t DistanceRequest) InputSchema() map[string]any {
	return server.ToInputSchema(&DistanceRequest{})
}
func (t DistanceRequest) Create(params []byte) server.Tool {
	request := &DistanceRequest{}
	json.Unmarshal(params, &request)
	return request
}

func (t DistanceRequest) Call(ctx server.HttpContext, s server.Server) error {
	serverConfig := &config.AmapServerConfig{}
	s.GetConfig(serverConfig)
	if serverConfig.ApiKey == "" {
		return errors.New("amap API-KEY is not configured")
	}

	url := fmt.Sprintf("http://restapi.amap.com/v3/distance?key=%s&origins=%s&destination=%s&type=%s&source=ts_mcp", serverConfig.ApiKey, url.QueryEscape(t.Origins), url.QueryEscape(t.Destination), url.QueryEscape(t.Type))
	return ctx.RouteCall(http.MethodGet, url,
		[][2]string{{"Accept", "application/json"}}, nil, func(statusCode int, responseHeaders [][2]string, responseBody []byte) {
			if statusCode != http.StatusOK {
				utils.OnMCPToolCallError(ctx, fmt.Errorf("distance call failed, status: %d", statusCode))
				return
			}
			utils.SendMCPToolTextResult(ctx, string(responseBody))
		})
}
