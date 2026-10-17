// Copyright (c) 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.
package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"ext-auth/config"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const preservedAuthHeadersContext = "ext-auth.preserved-auth-headers"

// This state is owned by the gateway, not by an HTTP header. Request lifespan
// survives an internal redirect but must not survive a new downstream request.
type preservedAuthHeaders map[string][]string

var (
	identityGetProperty = proxywasm.GetProperty
	identitySetProperty = proxywasm.SetProperty
	identityGetHeaders  = proxywasm.GetHttpRequestHeaders
	identityForeignCall = proxywasm.CallForeignFunction
)

func identityPropertyName() (string, error) {
	plugin, err := identityGetProperty([]string{"plugin_name"})
	if err != nil || len(plugin) == 0 {
		return "", errors.New("missing plugin identity")
	}
	return "higress.ext_auth.original_headers." + string(plugin), nil
}

func parseConfigWithPreservedIdentity(ctx wrapper.PluginContext, json gjson.Result, cfg *config.ExtAuthConfig) error {
	if err := config.ParseConfig(json, cfg); err != nil {
		return err
	}
	if len(cfg.HttpService.AuthorizationRequest.PreserveHeadersOnInternalRedirect) == 0 {
		return nil
	}
	property, err := identityPropertyName()
	if err != nil {
		return err
	}
	if declared, ok := ctx.GetContext(property).(bool); ok && declared {
		return nil
	}
	// declare_property must run on the plugin/root context. Envoy SetProperty
	// looks up the prototype on rootContext(); declaring from an HTTP context
	// silently leaves the default FilterChain lifespan and loses it on redirect.
	if _, err := identityForeignCall("declare_property", identityPropertyDeclaration(property)); err != nil {
		return errors.New("cannot declare request-lifespan identity state")
	}
	ctx.SetContext(property, true)
	return nil
}

// Envoy DeclarePropertyArguments wire format: name=1, readonly=2, span=5.
// type defaults to Bytes; span=DownstreamRequest(1), not FilterChain(0) or
// DownstreamConnection(2). No protobuf runtime is needed for this tiny message.
func identityPropertyDeclaration(name string) []byte {
	args := []byte{0x0a}
	args = binary.AppendUvarint(args, uint64(len(name)))
	args = append(args, name...)
	return append(args, 0x10, 0x01, 0x28, 0x01)
}

func preparePreservedAuthHeaders(names []string) (preservedAuthHeaders, error) {
	property, err := identityPropertyName()
	if err != nil {
		return nil, err
	}
	data, err := identityGetProperty([]string{property})
	if err == nil {
		var original preservedAuthHeaders
		if json.Unmarshal(data, &original) != nil || original == nil {
			return nil, errors.New("invalid trusted identity state")
		}
		// A fallback route may request additional preserved header names. Only
		// names captured on the first pass are trusted; do not capture rewritten
		// provider credentials on later passes.
		for _, name := range names {
			if _, exists := original[name]; !exists {
				return nil, errors.New("preserved header set changed during redirect")
			}
		}
		return original, nil
	}
	if !errors.Is(err, types.ErrorStatusNotFound) {
		return nil, errors.New("cannot read trusted identity state")
	}
	headers, err := identityGetHeaders()
	if err != nil {
		return nil, errors.New("cannot read original request headers")
	}
	original := preservedAuthHeaders{}
	for _, name := range names {
		original[name] = nil
	}
	for _, header := range headers {
		name := strings.ToLower(header[0])
		if _, exists := original[name]; exists {
			original[name] = append(original[name], header[1])
		}
	}
	data, err = json.Marshal(original)
	if err != nil {
		return nil, errors.New("cannot encode trusted identity state")
	}
	if identitySetProperty([]string{property}, data) != nil {
		return nil, errors.New("cannot save trusted identity state")
	}
	return original, nil
}

func restorePreservedHeaders(current [][2]string, original preservedAuthHeaders) [][2]string {
	result := make([][2]string, 0, len(current))
	for _, header := range current {
		if _, preserved := original[strings.ToLower(header[0])]; !preserved {
			result = append(result, header)
		}
	}
	for name, values := range original {
		for _, value := range values {
			result = append(result, [2]string{name, value})
		}
	}
	return result
}
