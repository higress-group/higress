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
	"net/http"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"hello-world",
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
	)
}

type HelloWorldConfig struct {
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config HelloWorldConfig) types.Action {
	err := proxywasm.AddHttpRequestHeader("hello", "world")
	if err != nil {
		log.Critical("failed to set request header")
	}
	proxywasm.SendHttpResponseWithDetail(http.StatusOK, "hello-world", nil, []byte("hello world"), -1)
	return types.ActionContinue
}
