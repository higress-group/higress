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
	"errors"
	"net/http"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/tidwall/gjson"

	"github.com/higress-group/wasm-go/pkg/wrapper"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"http-call",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
	)
}

type HttpCallConfig struct {
	client      wrapper.HttpClient
	requestPath string
	bodyHeader  string
	tokenHeader string
}

func parseConfig(json gjson.Result, config *HttpCallConfig, log log.Log) error {
	config.bodyHeader = json.Get("bodyHeader").String()
	if config.bodyHeader == "" {
		return errors.New("missing bodyHeader in config")
	}
	config.tokenHeader = json.Get("tokenHeader").String()
	if config.tokenHeader == "" {
		return errors.New("missing tokenHeader in config")
	}
	config.requestPath = json.Get("requestPath").String()
	if config.requestPath == "" {
		return errors.New("missing requestPath in config")
	}
	serviceSource := json.Get("serviceSource").String()
	serviceName := json.Get("serviceName").String()
	servicePort := json.Get("servicePort").Int()
	if serviceName == "" || servicePort == 0 {
		return errors.New("invalid service config")
	}
	switch serviceSource {
	case "k8s":
		namespace := json.Get("namespace").String()
		config.client = wrapper.NewClusterClient(wrapper.K8sCluster{
			ServiceName: serviceName,
			Namespace:   namespace,
			Port:        servicePort,
		})
		return nil
	case "nacos":
		namespace := json.Get("namespace").String()
		config.client = wrapper.NewClusterClient(wrapper.NacosCluster{
			ServiceName: serviceName,
			NamespaceID: namespace,
			Port:        servicePort,
		})
		return nil
	case "ip":
		config.client = wrapper.NewClusterClient(wrapper.StaticIpCluster{
			ServiceName: serviceName,
			Port:        servicePort,
		})
		return nil
	case "dns":
		domain := json.Get("domain").String()
		config.client = wrapper.NewClusterClient(wrapper.DnsCluster{
			ServiceName: serviceName,
			Port:        servicePort,
			Domain:      domain,
		})
		return nil
	default:
		return errors.New("unknown service source: " + serviceSource)
	}
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config HttpCallConfig, log log.Log) types.Action {
	err := config.client.Get(config.requestPath, nil,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			defer proxywasm.ResumeHttpRequest()
			if statusCode != http.StatusOK {
				log.Errorf("http call failed, status: %d", statusCode)
				return
			}
			// avoid protocol error
			body := strings.ReplaceAll(string(responseBody), "\n", "#")
			// set body to the original request header
			proxywasm.AddHttpRequestHeader(config.bodyHeader, body)
			// set service response token header to the original request header
			token := responseHeaders.Get(config.tokenHeader)
			if token != "" {
				proxywasm.AddHttpRequestHeader(config.tokenHeader, token)
			}
		})
	if err != nil {
		// A synchronous dispatch failure means the callback above will never
		// run, so the request would stay paused at the watermark forever.
		// Keep the plugin's fail-open behaviour (a non-200 upstream response
		// is logged and the request is forwarded anyway) and let the request
		// continue without the extra headers.
		log.Errorf("http call dispatch failed: %v", err)
		return types.ActionContinue
	}
	return types.HeaderStopAllIterationAndWatermark
}
