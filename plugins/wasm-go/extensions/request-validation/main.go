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
	"fmt"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/santhosh-tekuri/jsonschema"
	"github.com/tidwall/gjson"
)

const (
	defaultHeaderSchema = "header"
	defaultBodySchema   = "body"
	defaultRejectedCode = 403
)

func main() {}

func init() {
	wrapper.SetCtx(
		"request-validation",
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
		wrapper.ParseConfigBy(parseConfig),
	)
}

// Config is the config for request validation.
type Config struct {
	// compiler is the compiler for json schema.
	compiler *jsonschema.Compiler
	// rejectedCode is the code for rejected request.
	rejectedCode uint32
	// rejectedMsg is the message for rejected request.
	rejectedMsg string
	// draft is the draft version of json schema.
	draft *jsonschema.Draft
	// enableBodySchema is the flag for enable body schema.
	enableBodySchema bool
	// enableHeaderSchema is the flag for enable header schema.
	enableHeaderSchema bool
}

func parseConfig(result gjson.Result, config *Config, log log.Log) error {
	headerSchema := result.Get("header_schema").String()
	bodySchema := result.Get("body_schema").String()
	enableSwagger := result.Get("enable_swagger").Bool()
	enableOas3 := result.Get("enable_oas3").Bool()
	code := result.Get("rejected_code").Int()
	msg := result.Get("rejected_msg").String()

	// set config default value
	config.enableBodySchema = false
	config.enableHeaderSchema = false

	// check enable_swagger and enable_oas3
	if enableSwagger && enableOas3 {
		return fmt.Errorf("enable_swagger and enable_oas3 can not be true at the same time")
	}

	// set draft version
	if enableSwagger {
		config.draft = jsonschema.Draft4
	}
	if enableOas3 {
		config.draft = jsonschema.Draft7
	}
	if !enableSwagger && !enableOas3 {
		config.draft = jsonschema.Draft7
	}

	// create compiler
	compiler := jsonschema.NewCompiler()
	compiler.Draft = config.draft
	config.compiler = compiler

	// add header schema to compiler
	if headerSchema != "" {
		err := config.compiler.AddResource(defaultHeaderSchema, strings.NewReader(headerSchema))
		if err != nil {
			return err
		}
		config.enableHeaderSchema = true
	}

	// add body schema to compiler
	if bodySchema != "" {
		err := config.compiler.AddResource(defaultBodySchema, strings.NewReader(bodySchema))
		if err != nil {
			return err
		}
		config.enableBodySchema = true
	}

	// check rejected_code is valid
	if code != 0 && code > 100 && code < 600 {
		config.rejectedCode = uint32(code)
	} else {
		config.rejectedCode = defaultRejectedCode
	}
	config.rejectedMsg = msg

	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config Config, log log.Log) types.Action {
	if !config.enableHeaderSchema {
		return types.ActionContinue
	}

	// get headers
	headers, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		log.Errorf("get request headers failed: %v", err)
		return types.ActionContinue
	}

	// convert to schema
	schema := make(map[string]interface{})
	for _, header := range headers {
		schema[header[0]] = header[1]
	}

	// convert to json string
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		log.Errorf("marshal schema failed: %v", err)
		return types.ActionContinue
	}

	// validate
	document := strings.NewReader(string(schemaBytes))
	compile, err := config.compiler.Compile(defaultHeaderSchema)
	if err != nil {
		log.Errorf("compile schema failed: %v", err)
		return types.ActionContinue
	}
	err = compile.Validate(document)
	if err != nil {
		log.Errorf("validate request headers failed: %v", err)
		proxywasm.SendHttpResponseWithDetail(config.rejectedCode, "request-validation.invalid_headers", nil, []byte(config.rejectedMsg), -1)
		return types.ActionPause
	}

	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config Config, body []byte, log log.Log) types.Action {
	if !config.enableBodySchema {
		return types.ActionContinue
	}

	// convert to schema
	schema := make(map[string]interface{})
	err := json.Unmarshal(body, &schema)
	if err != nil {
		log.Errorf("unmarshal body failed: %v", err)
		return types.ActionContinue
	}

	// convert to json string
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		log.Errorf("marshal schema failed: %v", err)
		return types.ActionContinue
	}

	// validate
	document := strings.NewReader(string(schemaBytes))
	compile, err := config.compiler.Compile(defaultBodySchema)
	if err != nil {
		log.Errorf("compile schema failed: %v", err)
		return types.ActionContinue
	}
	err = compile.Validate(document)
	if err != nil {
		log.Errorf("validate request body failed: %v", err)
		proxywasm.SendHttpResponseWithDetail(config.rejectedCode, "request-validation.invalid_body", nil, []byte(config.rejectedMsg), -1)
		return types.ActionPause
	}

	return types.ActionContinue
}
