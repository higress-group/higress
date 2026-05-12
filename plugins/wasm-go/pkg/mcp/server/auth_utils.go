// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"
)

// setOrReplaceHeader sets or replaces a header in the headers slice.
// If the header exists (case-insensitive comparison), it replaces the value.
// If the header doesn't exist, it appends a new header.
func setOrReplaceHeader(headers *[][2]string, key, value string) {
	lowerKey := strings.ToLower(key)

	// Check if header already exists
	for i, header := range *headers {
		if strings.ToLower(header[0]) == lowerKey {
			// Replace existing header value
			(*headers)[i][1] = value
			return
		}
	}

	// Header doesn't exist, append new one
	*headers = append(*headers, [2]string{key, value})
}

// SecurityScheme defines a security scheme for the REST API
type SecurityScheme struct {
	ID                string `json:"id"`
	Type              string `json:"type"`             // http, apiKey
	Scheme            string `json:"scheme,omitempty"` // basic, bearer (for type: http)
	In                string `json:"in,omitempty"`     // header, query (for type: apiKey)
	Name              string `json:"name,omitempty"`   // Header or query parameter name (for type: apiKey)
	DefaultCredential string `json:"defaultCredential,omitempty"`
}

// SecurityRequirement specifies a security scheme requirement for a tool
type SecurityRequirement struct {
	ID          string `json:"id"`                    // References a security scheme ID
	Credential  string `json:"credential,omitempty"`  // Overrides default credential
	Passthrough bool   `json:"passthrough,omitempty"` // If true, credentials from client request will be passed through
}

// AuthRequestContext holds the data needed for applying security schemes.
type AuthRequestContext struct {
	Method                string
	Headers               [][2]string // Direct slice, modifications within applySecurity will update this field in the struct instance
	ParsedURL             *url.URL    // Pointer to allow modification (e.g., RawQuery)
	RequestBody           []byte      // For future security types that might inspect the body
	PassthroughCredential string      // Credential extracted from client request for passthrough
}

// SecuritySchemeProvider provides access to security schemes
type SecuritySchemeProvider interface {
	GetSecurityScheme(id string) (SecurityScheme, bool)
}

// ExtractAndRemoveIncomingCredential extracts a credential from the current incoming HTTP request
// and removes it. It uses global proxywasm functions to access request details.
// For query parameters, "removal" is conceptual as we build a new request;
// this function primarily extracts the value for potential passthrough.
func ExtractAndRemoveIncomingCredential(scheme SecurityScheme) (string, error) {
	credentialValue := ""
	var err error

	switch scheme.Type {
	case "http":
		authHeader, _ := proxywasm.GetHttpRequestHeader("Authorization") // Error ignored, check content
		if authHeader == "" {
			// If no header, it's not an error for extraction if not required, but indicates not found.
			// For removal, there's nothing to remove.
			return "", nil // Or a specific "not found" error if scheme implies it must be there.
		}

		if scheme.Scheme == "bearer" {
			if !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				return "", fmt.Errorf("incoming Authorization header is not Bearer auth: %s", authHeader)
			}
			credentialValue = strings.TrimSpace(authHeader[len("Bearer "):])
		} else if scheme.Scheme == "basic" {
			if !strings.HasPrefix(strings.ToLower(authHeader), "basic ") {
				return "", fmt.Errorf("incoming Authorization header is not Basic auth: %s", authHeader)
			}
			credentialValue = strings.TrimSpace(authHeader[len("Basic "):])
		} else {
			return "", fmt.Errorf("unsupported http scheme for credential extraction/removal: %s", scheme.Scheme)
		}
		proxywasm.RemoveHttpRequestHeader("Authorization")
		log.Debugf("Extracted and removed Authorization header for incoming %s scheme.", scheme.Scheme)

	case "apiKey":
		if scheme.In == "header" {
			if scheme.Name == "" {
				return "", errors.New("apiKey in header requires a name for the header")
			}
			headerValue, _ := proxywasm.GetHttpRequestHeader(scheme.Name) // Error ignored, check content
			if headerValue == "" {
				return "", nil // Not found, not necessarily an error for extraction.
			}
			credentialValue = headerValue
			proxywasm.RemoveHttpRequestHeader(scheme.Name)
			log.Debugf("Extracted and removed %s header for incoming apiKey auth.", scheme.Name)
		} else if scheme.In == "query" {
			if scheme.Name == "" {
				return "", errors.New("apiKey in query requires a name for the query parameter")
			}
			pathHeader, _ := proxywasm.GetHttpRequestHeader(":path") // Error ignored, check content
			if pathHeader == "" {
				// This case might be an error as :path should generally exist.
				return "", fmt.Errorf("no :path header found in incoming request for apiKey in query")
			}

			requestURL, parseErr := url.Parse(pathHeader)
			if parseErr != nil {
				return "", fmt.Errorf("failed to parse incoming :path header '%s': %v", pathHeader, parseErr)
			}

			queryValues := requestURL.Query()
			apiKeyValue := queryValues.Get(scheme.Name)
			if apiKeyValue == "" {
				return "", nil // Not found
			}
			credentialValue = apiKeyValue
			log.Debugf("Extracted %s query parameter from incoming request. Removal from original :path is implicit.", scheme.Name)
		} else {
			return "", fmt.Errorf("unsupported apiKey 'in' value: %s", scheme.In)
		}
	default:
		return "", fmt.Errorf("unsupported security scheme type for credential extraction/removal: %s", scheme.Type)
	}

	return credentialValue, err
}

// ApplySecurity applies the configured security scheme to the request.
// It modifies reqCtx.Headers and reqCtx.ParsedURL (specifically RawQuery) in place if necessary.
func ApplySecurity(securityConfig SecurityRequirement, provider SecuritySchemeProvider, reqCtx *AuthRequestContext) error {
	if securityConfig.ID == "" {
		return nil // No security scheme defined
	}
	if reqCtx.ParsedURL == nil {
		return errors.New("ParsedURL in AuthRequestContext cannot be nil for ApplySecurity")
	}

	upstreamScheme, schemeOk := provider.GetSecurityScheme(securityConfig.ID)
	if !schemeOk {
		return fmt.Errorf("upstream security scheme with id '%s' not found", securityConfig.ID)
	}

	var credentialToUse string
	if reqCtx.PassthroughCredential != "" {
		// Use the passthrough credential value.
		// The upstreamScheme dictates how this value is formatted and applied.
		credentialToUse = reqCtx.PassthroughCredential
		log.Debugf("Using passthrough credential for upstream request with scheme %s.", upstreamScheme.ID)
	} else {
		// Use configured credential for the upstream request.
		credentialToUse = upstreamScheme.DefaultCredential
		if securityConfig.Credential != "" {
			credentialToUse = securityConfig.Credential
		}
		if credentialToUse == "" {
			return fmt.Errorf("no credential found or configured for upstream security scheme '%s'", upstreamScheme.ID)
		}
		log.Debugf("Using configured credential for upstream request with scheme %s.", upstreamScheme.ID)
	}

	switch upstreamScheme.Type {
	case "http":
		authValue := credentialToUse
		if upstreamScheme.Scheme == "basic" {
			if !strings.HasPrefix(authValue, "Basic ") {
				if reqCtx.PassthroughCredential != "" { // Came from passthrough, it's the base64 token part
					authValue = "Basic " + credentialToUse
				} else { // Came from config
					if strings.Contains(credentialToUse, ":") { // Assumed to be "user:pass"
						authValue = "Basic " + base64.StdEncoding.EncodeToString([]byte(credentialToUse))
					} else { // Assumed to be already base64 encoded string (token part)
						authValue = "Basic " + credentialToUse
					}
				}
			}
		} else if upstreamScheme.Scheme == "bearer" {
			// Passthrough for Bearer gives the token part. Configured credential is the token.
			if !strings.HasPrefix(authValue, "Bearer ") {
				authValue = "Bearer " + credentialToUse
			}
		} else {
			return fmt.Errorf("unsupported http scheme type for upstream: %s", upstreamScheme.Scheme)
		}
		setOrReplaceHeader(&reqCtx.Headers, "Authorization", authValue)
	case "apiKey":
		if upstreamScheme.In == "header" {
			if upstreamScheme.Name == "" {
				return errors.New("apiKey in header requires a name for the header for upstream")
			}
			setOrReplaceHeader(&reqCtx.Headers, upstreamScheme.Name, credentialToUse)
		} else if upstreamScheme.In == "query" {
			if upstreamScheme.Name == "" {
				return errors.New("apiKey in query requires a name for the query parameter for upstream")
			}
			queryValues := reqCtx.ParsedURL.Query()
			queryValues.Set(upstreamScheme.Name, credentialToUse)
			reqCtx.ParsedURL.RawQuery = queryValues.Encode()
		} else {
			return fmt.Errorf("unsupported apiKey 'in' value for upstream: %s", upstreamScheme.In)
		}
	default:
		return fmt.Errorf("unsupported security scheme type: %s", upstreamScheme.Type)
	}
	return nil
}
