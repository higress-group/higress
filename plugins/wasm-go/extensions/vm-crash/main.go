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

// Package main implements the vm-crash test plugin used to exercise
// failStrategy behavior in WasmPlugin e2e tests. The plugin always
// crashes the wasm VM during request header processing, which lets us
// verify that failStrategy=FAIL_OPEN passes the request through to the
// backend while failStrategy=FAIL_CLOSE returns 503 wasm_fail_stream.
package main

import (
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"vm-crash",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
	)
}

type VMCrashConfig struct{}

func parseConfig(json gjson.Result, config *VMCrashConfig, log log.Log) error {
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config VMCrashConfig, log log.Log) types.Action {
	// Force a wasm VM trap. Nil-pointer deref produces an unrecoverable
	// runtime panic in the Go wasip1 runtime, which the host treats as a
	// VM crash and routes through the configured failStrategy.
	var p *int
	_ = *p
	return types.ActionContinue
}
