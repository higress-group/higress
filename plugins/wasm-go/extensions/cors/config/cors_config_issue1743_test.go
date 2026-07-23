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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCorsConfigIssue1743FillDefaultValuesSplitsMethodsAndHeaders(t *testing.T) {
	c := &CorsConfig{}
	c.FillDefaultValues()

	assert.Equal(t, []string{"GET", "PUT", "POST", "DELETE", "PATCH", "OPTIONS"}, c.allowMethods)
	assert.Contains(t, c.allowHeaders, "Content-Type")
	assert.Contains(t, c.allowHeaders, "Authorization")
	assert.NotContains(t, c.allowHeaders, defaultAllowHeaders)

	allowMethods, methodOk := c.checkMethods("POST")
	assert.True(t, methodOk)
	assert.Equal(t, "GET,PUT,POST,DELETE,PATCH,OPTIONS", allowMethods)

	allowHeaders, headerOk := c.checkHeaders("Content-Type, Authorization")
	assert.True(t, headerOk)
	assert.Contains(t, allowHeaders, "Content-Type")
	assert.Contains(t, allowHeaders, "Authorization")
}

func TestCorsConfigIssue1743CheckHeadersTrimsRequestTokens(t *testing.T) {
	c := &CorsConfig{
		allowHeaders: []string{"Content-Type", "Authorization"},
	}

	allowHeaders, ok := c.checkHeaders("Content-Type, Authorization")

	assert.True(t, ok)
	assert.Equal(t, "Content-Type,Authorization", allowHeaders)
}

func TestCorsConfigIssue1743WildcardMethodsEchoRequestedMethod(t *testing.T) {
	c := &CorsConfig{
		allowMethods: []string{"*"},
	}

	allowMethods, ok := c.checkMethods("PROPFIND")

	assert.True(t, ok)
	assert.Equal(t, "PROPFIND", allowMethods)
}

func TestCorsConfigIssue1743WildcardHeadersEchoTrimmedRequestHeaders(t *testing.T) {
	c := &CorsConfig{
		allowHeaders: []string{"*"},
	}

	allowHeaders, ok := c.checkHeaders(" Content-Type, Authorization, ")

	assert.True(t, ok)
	assert.Equal(t, "Content-Type,Authorization", allowHeaders)
}

func TestCorsConfigIssue1743WildcardHeadersOmitAllowHeadersWhenNoRequestHeaders(t *testing.T) {
	c := &CorsConfig{
		allowHeaders: []string{"*"},
	}

	allowHeaders, ok := c.checkHeaders("")

	assert.True(t, ok)
	assert.Empty(t, allowHeaders)
}

func TestCorsConfigIssue1743ProcessWildcardPreflight(t *testing.T) {
	c := &CorsConfig{
		allowOrigins: []string{"*"},
		allowMethods: []string{"*"},
		allowHeaders: []string{"*"},
	}

	httpCorsContext, err := c.Process("https", "api.example.com", "OPTIONS", [][2]string{
		{HeaderOrigin, "https://client.example.com"},
		{HeaderControlRequestMethod, "PROPFIND"},
		{HeaderControlRequestHeaders, " Content-Type, Authorization, "},
	})

	require.NoError(t, err)
	assert.True(t, httpCorsContext.IsValid)
	assert.True(t, httpCorsContext.IsPreFlight)
	assert.True(t, httpCorsContext.IsCorsRequest)
	assert.Equal(t, "PROPFIND", httpCorsContext.AllowMethods)
	assert.Equal(t, "Content-Type,Authorization", httpCorsContext.AllowHeaders)
}

func TestCorsConfigIssue1743ProcessWildcardPreflightOmitsAllowHeadersWithoutRequestHeaders(t *testing.T) {
	c := &CorsConfig{
		allowOrigins: []string{"*"},
		allowMethods: []string{"*"},
		allowHeaders: []string{"*"},
	}

	httpCorsContext, err := c.Process("https", "api.example.com", "OPTIONS", [][2]string{
		{HeaderOrigin, "https://client.example.com"},
		{HeaderControlRequestMethod, "PROPFIND"},
	})

	require.NoError(t, err)
	assert.True(t, httpCorsContext.IsValid)
	assert.Equal(t, "PROPFIND", httpCorsContext.AllowMethods)
	assert.Empty(t, httpCorsContext.AllowHeaders)
}

func TestCorsConfigIssue1743OriginPatternAnchored(t *testing.T) {
	c := &CorsConfig{
		allowOriginPatterns: []OriginPattern{
			newOriginPatternFromString("http://*.example.com"),
		},
	}

	allowOrigin, ok := c.checkOrigin("http://api.example.com")
	assert.True(t, ok)
	assert.Equal(t, "http://api.example.com", allowOrigin)

	allowOrigin, ok = c.checkOrigin("http://api.example.com.evil.com")
	assert.False(t, ok)
	assert.Empty(t, allowOrigin)
}

func TestCorsConfigIssue1743OriginPatternWithPortsAnchored(t *testing.T) {
	c := &CorsConfig{
		allowOriginPatterns: []OriginPattern{
			newOriginPatternFromString("http://*.example.com:[8080,9090]"),
		},
	}

	allowOrigin, ok := c.checkOrigin("http://api.example.com:8080")
	assert.True(t, ok)
	assert.Equal(t, "http://api.example.com:8080", allowOrigin)

	allowOrigin, ok = c.checkOrigin("http://api.example.com:8080.evil.com")
	assert.False(t, ok)
	assert.Empty(t, allowOrigin)
}

func TestCorsConfigIssue1743ExposeHeadersWildcardWithCredentialsAccepted(t *testing.T) {
	c := &CorsConfig{}

	c.AddExposeHeader("*")
	err := c.SetAllowCredentials(true)

	require.NoError(t, err)
	assert.Equal(t, []string{"*"}, c.exposeHeaders)
	assert.True(t, c.allowCredentials)
}
