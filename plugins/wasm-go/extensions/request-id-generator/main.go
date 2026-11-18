// Copyright (c) 2025 Alibaba Group Holding Ltd.
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
	"crypto/rand"
	"fmt"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"request-id-generator",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
	)
}

// RequestIDConfig holds the plugin configuration
type RequestIDConfig struct {
	Enable           bool   `json:"enable"`
	RequestHeader    string `json:"request_header"`
	ResponseHeader   string `json:"response_header"`
	OverrideExisting bool   `json:"override_existing"`
}

// parseConfig parses the plugin configuration from JSON
func parseConfig(json gjson.Result, config *RequestIDConfig, logger log.Log) error {
	// Set default values
	config.Enable = true
	config.RequestHeader = "X-Request-Id"
	config.ResponseHeader = ""
	config.OverrideExisting = false

	// Parse enable flag
	if enable := json.Get("enable"); enable.Exists() {
		config.Enable = enable.Bool()
	}

	// Parse request header name
	if requestHeader := json.Get("request_header"); requestHeader.Exists() {
		config.RequestHeader = requestHeader.String()
		if config.RequestHeader == "" {
			config.RequestHeader = "X-Request-Id"
		}
	}

	// Parse response header name
	if responseHeader := json.Get("response_header"); responseHeader.Exists() {
		config.ResponseHeader = responseHeader.String()
	}

	// Parse override_existing flag
	if overrideExisting := json.Get("override_existing"); overrideExisting.Exists() {
		config.OverrideExisting = overrideExisting.Bool()
	}

	if logger != nil {
		logger.Infof("Request ID Generator config loaded: enable=%v, request_header=%s, response_header=%s, override_existing=%v",
			config.Enable, config.RequestHeader, config.ResponseHeader, config.OverrideExisting)
	}

	return nil
}

// generateUUID generates a UUID v4 string
func generateUUID() (string, error) {
	uuid := make([]byte, 16)

	// Read random bytes
	_, err := rand.Read(uuid)
	if err != nil {
		return "", err
	}

	// Set version (4) and variant bits
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant 10

	// Format as UUID string
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4],
		uuid[4:6],
		uuid[6:8],
		uuid[8:10],
		uuid[10:16]), nil
}

// onHttpRequestHeaders processes request headers and injects request ID
func onHttpRequestHeaders(ctx wrapper.HttpContext, config RequestIDConfig, logger log.Log) types.Action {
	// Check if plugin is enabled
	if !config.Enable {
		return types.ActionContinue
	}

	var requestID string

	// Check if request ID already exists
	existingID, err := proxywasm.GetHttpRequestHeader(config.RequestHeader)
	if err == nil && existingID != "" && !config.OverrideExisting {
		// Use existing request ID
		requestID = existingID
		logger.Debugf("Using existing request ID: %s", requestID)
	} else {
		// Generate new request ID
		newID, err := generateUUID()
		if err != nil {
			logger.Errorf("Failed to generate UUID: %v", err)
			return types.ActionContinue
		}
		requestID = newID

		// Add request ID to request headers
		err = proxywasm.ReplaceHttpRequestHeader(config.RequestHeader, requestID)
		if err != nil {
			logger.Errorf("Failed to set request header: %v", err)
			return types.ActionContinue
		}
		logger.Debugf("Generated and injected new request ID: %s", requestID)
	}

	// Store request ID in context for response phase
	if config.ResponseHeader != "" {
		ctx.SetContext("request_id", requestID)
	}

	return types.ActionContinue
}

// onHttpResponseHeaders processes response headers and optionally adds request ID
func onHttpResponseHeaders(ctx wrapper.HttpContext, config RequestIDConfig, logger log.Log) types.Action {
	// Check if plugin is enabled and response header is configured
	if !config.Enable || config.ResponseHeader == "" {
		return types.ActionContinue
	}

	// Retrieve request ID from context
	requestID := ctx.GetContext("request_id")
	if requestID == nil {
		logger.Debug("No request ID found in context")
		return types.ActionContinue
	}

	requestIDStr, ok := requestID.(string)
	if !ok {
		logger.Error("Request ID in context is not a string")
		return types.ActionContinue
	}

	// Add request ID to response headers
	err := proxywasm.AddHttpResponseHeader(config.ResponseHeader, requestIDStr)
	if err != nil {
		logger.Errorf("Failed to add response header: %v", err)
	} else {
		logger.Debugf("Added request ID to response header: %s", requestIDStr)
	}

	return types.ActionContinue
}
