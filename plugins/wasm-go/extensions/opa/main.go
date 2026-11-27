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

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"opa",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
	)
}

type Metadata struct {
	Input map[string]interface{} `json:"input"`
}

func parseConfig(json gjson.Result, config *OpaConfig, log log.Log) error {
	policy := json.Get("policy").String()
	if strings.TrimSpace(policy) == "" {
		return errors.New("policy not allow empty")
	}

	timeout := json.Get("timeout").String()
	if strings.TrimSpace(timeout) == "" {
		return errors.New("timeout not allow empty")
	}

	duration, err := time.ParseDuration(timeout)
	if err != nil {
		return errors.New("timeout parse fail: " + err.Error())
	}

	var uint32Duration uint32

	if duration.Milliseconds() > int64(^uint32(0)) {
	} else {
		uint32Duration = uint32(duration.Milliseconds())
	}
	config.timeout = uint32Duration

	client, err := Client(json)
	if err != nil {
		return err
	}
	config.client = client
	config.policy = policy

	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config OpaConfig, log log.Log) types.Action {
	act := opaCall(ctx, config, nil, log)
	if act == types.ActionPause {
		return types.HeaderStopAllIterationAndWatermark
	}
	return types.HeaderContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config OpaConfig, body []byte, log log.Log) types.Action {
	return opaCall(ctx, config, body, log)
}

func opaCall(ctx wrapper.HttpContext, config OpaConfig, body []byte, log log.Log) types.Action {
	request := make(map[string]interface{}, 6)
	headers, _ := proxywasm.GetHttpRequestHeaders()

	request["method"] = ctx.Method()
	request["scheme"] = ctx.Scheme()
	request["path"] = ctx.Path()
	request["headers"] = headers
	if len(body) != 0 {
		request["body"] = body
	}
	parse, _ := url.Parse(ctx.Path())
	query, _ := url.ParseQuery(parse.RawQuery)
	request["query"] = query

	data, _ := json.Marshal(Metadata{Input: map[string]interface{}{"request": request}})
	if err := config.client.Post(fmt.Sprintf("/v1/data/%s/allow", config.policy),
		[][2]string{{"Content-Type", "application/json"}},
		data, rspCall, config.timeout); err != nil {
		log.Errorf("client opa fail %v", err)
		proxywasm.SendHttpResponseWithDetail(
			http.StatusServiceUnavailable,
			"opa.dispatch_fail",
			nil,
			[]byte("opa dispatch failed"),
			-1,
		)
		return types.ActionPause
	}
	return types.ActionPause
}

func rspCall(statusCode int, _ http.Header, responseBody []byte) {
	if statusCode != http.StatusOK {
		proxywasm.SendHttpResponseWithDetail(uint32(statusCode), "opa.status_ne_200", nil, []byte("opa state not is 200"), -1)
		return
	}
	var rsp map[string]interface{}
	if err := json.Unmarshal(responseBody, &rsp); err != nil {
		proxywasm.SendHttpResponseWithDetail(http.StatusInternalServerError, "opa.bad_response_body", nil, []byte(fmt.Sprintf("opa parse rsp fail %+v", err)), -1)
		return
	}

	result, ok := rsp["result"].(bool)
	if !ok {
		proxywasm.SendHttpResponseWithDetail(http.StatusInternalServerError, "opa.conversion_fail", nil, []byte("rsp type conversion fail"), -1)
		return
	}

	if !result {
		proxywasm.SendHttpResponseWithDetail(http.StatusUnauthorized, "opa.server_not_allowed", nil, []byte("opa server not allowed"), -1)
		return
	}
	proxywasm.ResumeHttpRequest()
}
