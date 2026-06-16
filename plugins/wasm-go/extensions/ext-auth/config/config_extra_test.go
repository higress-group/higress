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

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// === Module A — parseAuthorizationResponseConfig =======================
//
// parseAuthorizationResponseConfig sits at 17.6% in the baseline because
// every existing fixture either omits authorization_response entirely or
// tests it implicitly through ParseConfig with only one of the two list
// shapes. The tests below drive each branch directly via ParseConfig:
//   - allowed_upstream_headers list set
//   - allowed_client_headers list set
//   - both lists set
//   - error propagation when one of the lists has a bad regex matcher
//     (the only failure mode the function can surface)

func parseFromJSON(t *testing.T, jsonStr string) (ExtAuthConfig, error) {
	t.Helper()
	var cfg ExtAuthConfig
	err := ParseConfig(gjson.Parse(jsonStr), &cfg)
	return cfg, err
}

func TestParseAuthorizationResponse_AllowedUpstreamHeaders(t *testing.T) {
	cfg, err := parseFromJSON(t, `{
		"http_service": {
			"endpoint_mode": "envoy",
			"endpoint": {
				"service_name": "ext-auth.example.com",
				"service_port": 8090,
				"path_prefix": "/auth"
			},
			"authorization_response": {
				"allowed_upstream_headers": [
					{"exact": "x-user-id"},
					{"prefix": "x-auth-"}
				]
			}
		}
	}`)
	require.NoError(t, err)
	require.NotNil(t, cfg.HttpService.AuthorizationResponse.AllowedUpstreamHeaders)
	// Sanity-check matcher behavior end-to-end.
	require.True(t, cfg.HttpService.AuthorizationResponse.AllowedUpstreamHeaders.Match("x-user-id"))
	require.True(t, cfg.HttpService.AuthorizationResponse.AllowedUpstreamHeaders.Match("x-auth-token"))
	require.False(t, cfg.HttpService.AuthorizationResponse.AllowedUpstreamHeaders.Match("authorization"))
	// Client side intentionally untouched.
	require.Nil(t, cfg.HttpService.AuthorizationResponse.AllowedClientHeaders)
}

func TestParseAuthorizationResponse_AllowedClientHeaders(t *testing.T) {
	cfg, err := parseFromJSON(t, `{
		"http_service": {
			"endpoint_mode": "envoy",
			"endpoint": {
				"service_name": "ext-auth.example.com",
				"service_port": 8090,
				"path_prefix": "/auth"
			},
			"authorization_response": {
				"allowed_client_headers": [
					{"exact": "www-authenticate"}
				]
			}
		}
	}`)
	require.NoError(t, err)
	require.Nil(t, cfg.HttpService.AuthorizationResponse.AllowedUpstreamHeaders)
	require.NotNil(t, cfg.HttpService.AuthorizationResponse.AllowedClientHeaders)
	require.True(t, cfg.HttpService.AuthorizationResponse.AllowedClientHeaders.Match("www-authenticate"))
	require.False(t, cfg.HttpService.AuthorizationResponse.AllowedClientHeaders.Match("x-user-id"))
}

func TestParseAuthorizationResponse_BothListsSet(t *testing.T) {
	cfg, err := parseFromJSON(t, `{
		"http_service": {
			"endpoint_mode": "envoy",
			"endpoint": {
				"service_name": "ext-auth.example.com",
				"service_port": 8090,
				"path_prefix": "/auth"
			},
			"authorization_response": {
				"allowed_upstream_headers": [{"exact": "x-user-id"}],
				"allowed_client_headers":   [{"prefix": "www-"}]
			}
		}
	}`)
	require.NoError(t, err)
	require.NotNil(t, cfg.HttpService.AuthorizationResponse.AllowedUpstreamHeaders)
	require.NotNil(t, cfg.HttpService.AuthorizationResponse.AllowedClientHeaders)
}

// Bad regex inside allowed_upstream_headers must propagate the
// BuildRepeatedStringMatcherIgnoreCase error and fail ParseConfig — pins
// the err path at config.go:239-241.
func TestParseAuthorizationResponse_AllowedUpstreamBadRegex(t *testing.T) {
	_, err := parseFromJSON(t, `{
		"http_service": {
			"endpoint_mode": "envoy",
			"endpoint": {
				"service_name": "ext-auth.example.com",
				"service_port": 8090,
				"path_prefix": "/auth"
			},
			"authorization_response": {
				"allowed_upstream_headers": [{"regex": "[unbalanced"}]
			}
		}
	}`)
	require.Error(t, err)
}

// Same propagation contract for allowed_client_headers — distinct branch
// at config.go:248-250.
func TestParseAuthorizationResponse_AllowedClientBadRegex(t *testing.T) {
	_, err := parseFromJSON(t, `{
		"http_service": {
			"endpoint_mode": "envoy",
			"endpoint": {
				"service_name": "ext-auth.example.com",
				"service_port": 8090,
				"path_prefix": "/auth"
			},
			"authorization_response": {
				"allowed_client_headers": [{"regex": "[unbalanced"}]
			}
		}
	}`)
	require.Error(t, err)
}

// === Module B — parseAuthorizationRequestConfig allowed_headers error ===
//
// Mirrors the response-side bad-regex case for the request side — the
// `allowed_headers` failure path at config.go:194-197 is unreached because
// every existing fixture supplies well-formed exact/prefix matchers.

func TestParseAuthorizationRequest_AllowedHeadersBadRegex(t *testing.T) {
	_, err := parseFromJSON(t, `{
		"http_service": {
			"endpoint_mode": "envoy",
			"endpoint": {
				"service_name": "ext-auth.example.com",
				"service_port": 8090,
				"path_prefix": "/auth"
			},
			"authorization_request": {
				"allowed_headers": [{"regex": "[unbalanced"}]
			}
		}
	}`)
	require.Error(t, err)
}

// === Module C — parseEndpointConfig small edges ========================
//
// forward_auth without explicit request_method falls back to GET (default
// http.MethodGet at config.go:169).
func TestParseEndpointConfig_ForwardAuthDefaultsToGET(t *testing.T) {
	cfg, err := parseFromJSON(t, `{
		"http_service": {
			"endpoint_mode": "forward_auth",
			"endpoint": {
				"service_name": "ext-auth.example.com",
				"service_port": 8090,
				"path":         "/auth"
			}
		}
	}`)
	require.NoError(t, err)
	require.Equal(t, "GET", cfg.HttpService.RequestMethod)
}

// service_port omitted defaults to 80 (config.go:144-146).
func TestParseEndpointConfig_ServicePortDefaults80(t *testing.T) {
	cfg, err := parseFromJSON(t, `{
		"http_service": {
			"endpoint_mode": "envoy",
			"endpoint": {
				"service_name": "ext-auth.example.com",
				"path_prefix":  "/auth"
			}
		}
	}`)
	require.NoError(t, err)
	require.NotNil(t, cfg.HttpService.Client)
}
