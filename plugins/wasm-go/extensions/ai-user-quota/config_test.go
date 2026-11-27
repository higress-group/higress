// Copyright (c) 2024-2025
// Beijing Baolande Software Corporation
// All Rights Reserved

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestParseEnablePathSuffixes(t *testing.T) {
	t.Run("missing uses defaults", func(t *testing.T) {
		suffixes, err := parseEnablePathSuffixes(gjson.Parse(`{}`))
		assert.NoError(t, err)
		assert.Equal(t, defaultEnablePathSuffixes, suffixes)
	})

	t.Run("custom values are trimmed", func(t *testing.T) {
		suffixes, err := parseEnablePathSuffixes(gjson.Parse(`{"enable_path_suffixes":[" /v1/chat/completions ",""]}`))
		assert.NoError(t, err)
		assert.Equal(t, []string{"/v1/chat/completions"}, suffixes)
	})

	t.Run("non array is rejected", func(t *testing.T) {
		_, err := parseEnablePathSuffixes(gjson.Parse(`{"enable_path_suffixes":"/v1/chat/completions"}`))
		assert.Error(t, err)
	})

	t.Run("empty is rejected", func(t *testing.T) {
		_, err := parseEnablePathSuffixes(gjson.Parse(`{"enable_path_suffixes":[]}`))
		assert.Error(t, err)
	})
}

func TestParseSkipRoutePrefixes(t *testing.T) {
	t.Run("missing uses defaults", func(t *testing.T) {
		prefixes, err := parseSkipRoutePrefixes(gjson.Parse(`{}`))
		assert.NoError(t, err)
		assert.Equal(t, defaultSkipRoutePrefixes, prefixes)
	})

	t.Run("explicit empty is allowed", func(t *testing.T) {
		prefixes, err := parseSkipRoutePrefixes(gjson.Parse(`{"skip_route_prefixes":[]}`))
		assert.NoError(t, err)
		assert.Empty(t, prefixes)
	})

	t.Run("non array is rejected", func(t *testing.T) {
		_, err := parseSkipRoutePrefixes(gjson.Parse(`{"skip_route_prefixes":"svc-"}`))
		assert.Error(t, err)
	})
}

func TestMatchPathSuffix(t *testing.T) {
	suffixes := defaultEnablePathSuffixes
	assert.True(t, matchPathSuffix("/v1/chat/completions", suffixes))
	assert.True(t, matchPathSuffix("/api/v1/responses?stream=true", suffixes))
	assert.True(t, matchPathSuffix("/v1/messages", suffixes))
	// catchall 路由不计量
	assert.False(t, matchPathSuffix("/v1/embeddings", suffixes))
	assert.False(t, matchPathSuffix("/", suffixes))
}

func TestMatchRoutePrefix(t *testing.T) {
	prefixes := defaultSkipRoutePrefixes
	assert.True(t, matchRoutePrefix("baolande/svc-8563635d-rule-0-match-0", prefixes))
	assert.True(t, matchRoutePrefix("baolande/infer-8563635d-rule-0-match-0", prefixes))
	assert.False(t, matchRoutePrefix("baolande/ai-route-8563635d-rule-0-match-0", prefixes))
	assert.False(t, matchRoutePrefix("svc-8563635d", []string{}))
}