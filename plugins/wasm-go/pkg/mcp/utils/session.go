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

package utils

import (
	"net/url"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

func IsStatefulSession(ctx wrapper.HttpContext) bool {
	parse, err := url.Parse(ctx.Path())
	if err != nil {
		log.Errorf("failed to parse request path: %v", err)
		return false
	}
	query, err := url.ParseQuery(parse.RawQuery)
	if err != nil {
		log.Errorf("failed to parse query params: %v", err)
		return false
	}
	// Protocol version: 2024-11-05
	if query.Get("sessionId") != "" {
		return true
	}
	// Protocol version: 2025-03-26
	sessionHeader, err := proxywasm.GetHttpRequestHeader("mcp-session-id")
	if err != nil {
		log.Errorf("failed to get request header: %v", err)
		return false
	}
	if sessionHeader != "" {
		return true
	}
	return false
}
