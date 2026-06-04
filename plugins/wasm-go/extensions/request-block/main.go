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
	"fmt"
	"strings"

	"regexp"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/tidwall/gjson"

	"github.com/higress-group/wasm-go/pkg/wrapper"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"request-block",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
	)
}

type RequestBlockConfig struct {
	blockedCode      uint32
	blockedMessage   string
	caseSensitive    bool
	blockUrls        []string
	blockExactUrls   []string
	blockHeaders     []string
	blockBodies      []string
	blockRegExpArray []*regexp.Regexp
}

func parseConfig(json gjson.Result, config *RequestBlockConfig, log log.Log) error {
	code := json.Get("blocked_code").Int()
	if code != 0 && code > 100 && code < 600 {
		config.blockedCode = uint32(code)
	} else {
		config.blockedCode = 403
	}
	config.blockedMessage = json.Get("blocked_message").String()
	config.caseSensitive = json.Get("case_sensitive").Bool()
	for _, item := range json.Get("block_urls").Array() {
		url := item.String()
		if url == "" {
			continue
		}
		if config.caseSensitive {
			config.blockUrls = append(config.blockUrls, url)
		} else {
			config.blockUrls = append(config.blockUrls, strings.ToLower(url))
		}
	}
	for _, item := range json.Get("block_exact_urls").Array() {
		url := item.String()
		if url == "" {
			continue
		}
		if config.caseSensitive {
			config.blockExactUrls = append(config.blockExactUrls, url)
		} else {
			config.blockExactUrls = append(config.blockExactUrls, strings.ToLower(url))
		}
	}
	for _, item := range json.Get("block_regexp_urls").Array() {
		regexpUrl := item.String()
		if regexpUrl == "" {
			continue
		}
		var reg *regexp.Regexp
		var err error
		if config.caseSensitive {
			reg, err = regexp.Compile(regexpUrl)
		} else {
			reg, err = regexp.Compile(strings.ToLower(regexpUrl))
		}
		if err != nil {
			return fmt.Errorf("invalid regexp pattern %q: %w", regexpUrl, err)
		}
		config.blockRegExpArray = append(config.blockRegExpArray, reg)
	}
	for _, item := range json.Get("block_headers").Array() {
		header := item.String()
		if header == "" {
			continue
		}
		if config.caseSensitive {
			config.blockHeaders = append(config.blockHeaders, header)
		} else {
			config.blockHeaders = append(config.blockHeaders, strings.ToLower(header))
		}
	}
	for _, item := range json.Get("block_bodies").Array() {
		body := item.String()
		if body == "" {
			continue
		}
		if config.caseSensitive {
			config.blockBodies = append(config.blockBodies, body)
		} else {
			config.blockBodies = append(config.blockBodies, strings.ToLower(body))
		}
	}
	if len(config.blockUrls) == 0 && len(config.blockHeaders) == 0 &&
		len(config.blockBodies) == 0 {
		return errors.New("there is no block rules")
	}
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config RequestBlockConfig, log log.Log) types.Action {
	if len(config.blockUrls) > 0 {
		requestUrl, err := proxywasm.GetHttpRequestHeader(":path")
		if err != nil {
			log.Warnf("get path failed: %v", err)
			return types.ActionContinue
		}
		if !config.caseSensitive {
			requestUrl = strings.ToLower(requestUrl)
		}
		for _, blockExactUrl := range config.blockExactUrls {
			if requestUrl == blockExactUrl {
				proxywasm.SendHttpResponseWithDetail(config.blockedCode, "request-block.url_blocked.exact", nil, []byte(config.blockedMessage), -1)
				return types.ActionContinue
			}
		}
		for _, blockUrl := range config.blockUrls {
			if strings.Contains(requestUrl, blockUrl) {
				proxywasm.SendHttpResponseWithDetail(config.blockedCode, "request-block.url_blocked.keyword", nil, []byte(config.blockedMessage), -1)
				return types.ActionContinue
			}
		}
		for _, regExpObj := range config.blockRegExpArray {
			if regExpObj.MatchString(requestUrl) {
				proxywasm.SendHttpResponseWithDetail(config.blockedCode, "request-block.url_blocked.regexp", nil, []byte(config.blockedMessage), -1)
				return types.ActionContinue
			}
		}
	}
	if len(config.blockHeaders) > 0 {
		headers, err := proxywasm.GetHttpRequestHeaders()
		if err != nil {
			log.Warnf("get headers failed: %v", err)
			return types.ActionContinue
		}
		var headerPairs []string
		for _, kv := range headers {
			headerPairs = append(headerPairs, fmt.Sprintf("%s\n%s", kv[0], kv[1]))
		}
		headerStr := strings.Join(headerPairs, "\n")
		if !config.caseSensitive {
			headerStr = strings.ToLower(headerStr)
		}
		for _, blockHeader := range config.blockHeaders {
			if strings.Contains(headerStr, blockHeader) {
				proxywasm.SendHttpResponseWithDetail(config.blockedCode, "request-block.body_blocked", nil, []byte(config.blockedMessage), -1)
				return types.ActionContinue
			}
		}
	}
	if len(config.blockBodies) == 0 {
		ctx.DontReadRequestBody()
	}
	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config RequestBlockConfig, body []byte, log log.Log) types.Action {
	log.Infof("My request-block body: %s\n", string(body))
	bodyStr := string(body)

	if !config.caseSensitive {
		bodyStr = strings.ToLower(bodyStr)
	}
	for _, blockBody := range config.blockBodies {
		if strings.Contains(bodyStr, blockBody) {
			proxywasm.SendHttpResponseWithDetail(config.blockedCode, "request-block.body_blocked", nil, []byte(config.blockedMessage), -1)
			return types.ActionContinue
		}
	}
	return types.ActionContinue

}
