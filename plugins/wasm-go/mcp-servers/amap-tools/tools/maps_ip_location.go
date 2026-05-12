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

	"amap-tools/config"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/server"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
)

var _ server.Tool = IPLocationRequest{}

type IPLocationRequest struct {
	IP string `json:"ip" jsonschema_description:"IP地址,获取不到则填写unknow,服务端将根据socket地址来获取IP"`
}

func (t IPLocationRequest) Description() string {
	return "通过IP定位所在的国家和城市等位置信息"
}

func (t IPLocationRequest) InputSchema() map[string]any {
	return server.ToInputSchema(&IPLocationRequest{})
}

func (t IPLocationRequest) Create(params []byte) server.Tool {
	request := &IPLocationRequest{}
	json.Unmarshal(params, &request)
	return request
}

func (t IPLocationRequest) Call(ctx server.HttpContext, s server.Server) error {
	serverConfig := &config.AmapServerConfig{}
	s.GetConfig(serverConfig)
	if serverConfig.ApiKey == "" {
		return errors.New("amap API-KEY is not configured")
	}
	if t.IP == "" || strings.Contains(t.IP, "unknow") {
		var bs []byte
		var ipStr string
		fromHeader := false
		bs, _ = proxywasm.GetProperty([]string{"source", "address"})
		if len(bs) > 0 {
			ipStr = string(bs)
		} else {
			ipStr, _ = proxywasm.GetHttpRequestHeader("x-forwarded-for")
			fromHeader = true
		}
		t.IP = parseIP(ipStr, fromHeader)
	}
	url := fmt.Sprintf("https://restapi.amap.com/v3/ip?ip=%s&key=%s&source=ts_mcp", url.QueryEscape(t.IP), serverConfig.ApiKey)
	return ctx.RouteCall(http.MethodGet, url,
		[][2]string{{"Accept", "application/json"}}, nil, func(statusCode int, responseHeaders [][2]string, responseBody []byte) {
			if statusCode != http.StatusOK {
				utils.OnMCPToolCallError(ctx, fmt.Errorf("ip location call failed, status: %d", statusCode))
				return
			}
			utils.SendMCPToolTextResult(ctx, string(responseBody))
		})
}

// parseIP 解析IP
func parseIP(source string, fromHeader bool) string {

	if fromHeader {
		source = strings.Split(source, ",")[0]
	}
	source = strings.Trim(source, " ")
	if strings.Contains(source, ".") {
		// parse ipv4
		return strings.Split(source, ":")[0]
	}
	//parse ipv6
	if strings.Contains(source, "]") {
		return strings.Split(source, "]")[0][1:]
	}
	return source
}
