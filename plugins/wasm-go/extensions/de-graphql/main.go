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
	"fmt"
	"net/http"

	"de-graphql/config"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"de-graphql",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
		wrapper.ProcessResponseBodyBy(onHttpResponseBody),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
	)
}

func parseConfig(json gjson.Result, config *config.DeGraphQLConfig, log log.Log) error {
	log.Debug("parseConfig()")
	gql := json.Get("gql").String()
	endpoint := json.Get("endpoint").String()
	timeout := json.Get("timeout").Int()
	domain := json.Get("domain").String()
	log.Debugf("gql:%s endpoint:%s timeout:%d domain:%s", gql, endpoint, timeout, domain)
	err := config.SetGql(gql)
	if err != nil {
		return err
	}
	err = config.SetEndpoint(endpoint)
	if err != nil {
		return err
	}
	config.SetTimeout(uint32(timeout))
	config.SetDomain(domain)
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config config.DeGraphQLConfig, log log.Log) types.Action {
	ctx.DisableReroute()
	log.Debug("onHttpRequestHeaders()")
	log.Debugf("schema:%s host:%s path:%s", ctx.Scheme(), ctx.Host(), ctx.Path())
	requestUrl, _ := proxywasm.GetHttpRequestHeader(":path")
	method, _ := proxywasm.GetHttpRequestHeader(":method")
	log.Debugf("method:%s, request:%s", method, requestUrl)
	if err := proxywasm.RemoveHttpRequestHeader("content-length"); err != nil {
		log.Debug("can not reset content-length")
	}
	replaceBody, err := config.ParseGqlFromUrl(requestUrl)
	if err != nil {
		// Reject the request instead of forwarding a hand built body that may
		// be malformed or may carry variables the client was not allowed to
		// set, and let the caller know which variable was rejected.
		log.Warnf("failed to parse request url %s : %v", requestUrl, err)
		proxywasm.SendHttpResponseWithDetail(http.StatusBadRequest, "de-graphql.invalid_variable", nil, []byte(err.Error()), -1)
		return types.ActionContinue
	}
	log.Debugf("replace body:%s", replaceBody)

	// Pass headers to upstream cluster
	headers, _ := proxywasm.GetHttpRequestHeaders()
	for i := len(headers) - 1; i >= 0; i-- {
		key := headers[i][0]
		if key == ":method" || key == ":path" || key == ":authority" {
			headers = append(headers[:i], headers[i+1:]...)
		}
	}
	// Add header Content-Type: application/json
	headers = append(headers, [2]string{"Content-Type", "application/json"})
	client := wrapper.NewClusterClient(wrapper.RouteCluster{Host: config.GetDomain()})
	// Call upstream graphql endpoint
	client.Post(config.GetEndpoint(), headers, []byte(replaceBody),
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			// Pass response headers and body to client
			headers := make([][2]string, 0, len(responseHeaders)+3)
			for headK, headV := range responseHeaders {
				headers = append(headers, [2]string{headK, headV[0]})
			}
			// Add debug headers
			headers = append(headers, [2]string{"x-degraphql-endpoint", config.GetEndpoint()})
			headers = append(headers, [2]string{"x-degraphql-timeout", fmt.Sprintf("%d", config.GetTimeout())})
			headers = append(headers, [2]string{"x-degraphql-version", config.GetVersion()})
			proxywasm.SendHttpResponseWithDetail(uint32(statusCode), "de-graphql", headers, responseBody, -1)
			return
		}, config.GetTimeout())

	return types.ActionPause
}

func onHttpRequestBody(ctx wrapper.HttpContext, config config.DeGraphQLConfig, body []byte, log log.Log) types.Action {
	log.Debug("onHttpRequestBody()")
	return types.ActionContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config config.DeGraphQLConfig, log log.Log) types.Action {
	log.Debug("onHttpResponseHeaders()")
	return types.ActionContinue
}

func onHttpResponseBody(ctx wrapper.HttpContext, config config.DeGraphQLConfig, body []byte, log log.Log) types.Action {
	log.Debug("onHttpResponseBody()")
	return types.ActionContinue
}
