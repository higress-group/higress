// Copyright (c) 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.
package main

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestIdentityDeclaredAtPluginConfiguration(t *testing.T) {
	get, foreign := identityGetProperty, identityForeignCall
	t.Cleanup(func() { identityGetProperty, identityForeignCall = get, foreign })
	identityGetProperty = func(path []string) ([]byte, error) {
		if path[0] == "plugin_name" {
			return []byte("test-ext-auth"), nil
		}
		return nil, types.ErrorStatusNotFound
	}
	declarations := 0
	identityForeignCall = func(name string, args []byte) ([]byte, error) {
		require.Equal(t, "declare_property", name)
		require.Equal(t, identityPropertyDeclaration("higress.ext_auth.original_headers.test-ext-auth"), args)
		declarations++
		return nil, nil
	}
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(forwardAuthConfig, &data))
	data["http_service"].(map[string]interface{})["authorization_request"] = map[string]interface{}{
		"preserve_headers_on_internal_redirect": []string{"authorization", "x-api-key"},
	}
	raw, err := json.Marshal(data)
	require.NoError(t, err)
	host, status := test.NewTestHost(raw)
	defer host.Reset()
	require.Equal(t, types.OnPluginStartStatusOK, status)
	// No HTTP request has entered yet: declaration belongs to the root context.
	require.Equal(t, 1, declarations)
}

func TestPreservedIdentityRequestState(t *testing.T) {
	get, set, headers, foreign := identityGetProperty, identitySetProperty, identityGetHeaders, identityForeignCall
	t.Cleanup(func() {
		identityGetProperty, identitySetProperty, identityGetHeaders, identityForeignCall = get, set, headers, foreign
	})
	state := map[string][]byte{}
	plugin := "gateway.ext-auth-a"
	current := [][2]string{{"Authorization", "Bearer client"}, {"x-hi-original-auth", "Bearer forged"}, {"x-higress-fallback-from", "forged-route"}}
	identityGetProperty = func(path []string) ([]byte, error) {
		if path[0] == "plugin_name" {
			return []byte(plugin), nil
		}
		if value, exists := state[path[0]]; exists {
			return value, nil
		}
		return nil, types.ErrorStatusNotFound
	}
	identitySetProperty = func(path []string, value []byte) error { state[path[0]] = value; return nil }
	identityGetHeaders = func() ([][2]string, error) { return current, nil }
	identityForeignCall = func(name string, args []byte) ([]byte, error) {
		require.Equal(t, "declare_property", name)
		require.Equal(t, []byte{0x10, 0x01, 0x28, 0x01}, args[len(args)-4:])
		return nil, nil
	}
	original, err := preparePreservedAuthHeaders([]string{"authorization", "x-api-key"})
	require.NoError(t, err)
	require.Equal(t, []string{"Bearer client"}, original["authorization"])
	require.Contains(t, original, "x-api-key")
	require.Nil(t, original["x-api-key"])
	current = [][2]string{{"authorization", "Bearer provider"}, {"x-api-key", "provider-api-key"}, {"x-hi-original-auth", "Bearer forged"}}
	reentered, err := preparePreservedAuthHeaders([]string{"authorization", "x-api-key"})
	require.NoError(t, err)
	require.Equal(t, original, reentered)
	restored := restorePreservedHeaders(current, reentered)
	require.Contains(t, restored, [2]string{"authorization", "Bearer client"})
	require.NotContains(t, restored, [2]string{"x-api-key", "provider-api-key"})
	// Different plugin instance must not inherit another instance's identity.
	plugin = "gateway.ext-auth-b"
	other, err := preparePreservedAuthHeaders([]string{"authorization"})
	require.NoError(t, err)
	require.Equal(t, []string{"Bearer provider"}, other["authorization"])
	// Same connection, new downstream request: Request-lifespan state is gone.
	state = map[string][]byte{}
	current = [][2]string{{"authorization", "Bearer second-client"}}
	second, err := preparePreservedAuthHeaders([]string{"authorization"})
	require.NoError(t, err)
	require.Equal(t, []string{"Bearer second-client"}, second["authorization"])
	_, err = preparePreservedAuthHeaders([]string{"authorization", "x-new-auth"})
	require.Error(t, err)
	// Missing Authorization remains absent on redirect (x-api-key-only client).
	state = map[string][]byte{}
	current = [][2]string{{"x-api-key", "client-api-key"}}
	apiOnly, err := preparePreservedAuthHeaders([]string{"authorization", "x-api-key"})
	require.NoError(t, err)
	require.Nil(t, apiOnly["authorization"])
	require.Equal(t, []string{"client-api-key"}, apiOnly["x-api-key"])
	// State corruption or host errors fail closed, not via header fallback.
	for key := range state {
		state[key] = []byte("invalid-json")
	}
	_, err = preparePreservedAuthHeaders([]string{"authorization"})
	require.Error(t, err)
	identityGetProperty = func([]string) ([]byte, error) { return nil, errors.New("host error") }
	_, err = preparePreservedAuthHeaders([]string{"authorization"})
	require.Error(t, err)
}

func TestPreservedHeaderRestoreDuplicatesAndAbsence(t *testing.T) {
	original := preservedAuthHeaders{"authorization": {"Bearer original"}, "x-api-key": nil, "x-auth": {"a", "b"}}
	data, err := json.Marshal(original)
	require.NoError(t, err)
	var decoded preservedAuthHeaders
	require.NoError(t, json.Unmarshal(data, &decoded))
	result := restorePreservedHeaders([][2]string{{"AUTHORIZATION", "Bearer provider"}, {"x-api-key", "provider"}, {"content-type", "application/json"}}, decoded)
	require.Len(t, result, 4)
	require.Contains(t, result, [2]string{"authorization", "Bearer original"})
	require.Contains(t, result, [2]string{"content-type", "application/json"})
	require.Contains(t, result, [2]string{"x-auth", "a"})
	require.Contains(t, result, [2]string{"x-auth", "b"})
}
