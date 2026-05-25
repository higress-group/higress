// Copyright (c) 2023 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0

package handler

import (
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/jwt-auth/config"
	"github.com/stretchr/testify/require"
)

func makeConsumer(headers []config.FromHeader, params, cookies []string) *config.Consumer {
	keep := true
	c := &config.Consumer{KeepToken: &keep}
	if headers != nil {
		c.FromHeaders = &headers
	}
	if params != nil {
		c.FromParams = &params
	}
	if cookies != nil {
		c.FromCookies = &cookies
	}
	return c
}

func TestExtractFromHeader_StripsPrefix(t *testing.T) {
	hp := &testProvider{headerMap: map[string]string{"authorization": "Bearer abc.def.ghi"}}
	got := extractToken(true, makeConsumer(
		[]config.FromHeader{{Name: "Authorization", ValuePrefix: "Bearer "}}, nil, nil,
	), hp, &testLogger{T: t})
	require.Equal(t, "abc.def.ghi", got)
	// keep_token=true → header should remain.
	require.Contains(t, hp.headerMap, "authorization")
}

func TestExtractFromHeader_KeepFalseRemovesHeader(t *testing.T) {
	hp := &testProvider{headerMap: map[string]string{"authorization": "Bearer tok"}}
	got := extractToken(false, makeConsumer(
		[]config.FromHeader{{Name: "Authorization", ValuePrefix: "Bearer "}}, nil, nil,
	), hp, &testLogger{T: t})
	require.Equal(t, "tok", got)
	require.NotContains(t, hp.headerMap, "authorization")
}

func TestExtractFromHeader_PrefixMismatchYieldsEmpty(t *testing.T) {
	hp := &testProvider{headerMap: map[string]string{"authorization": "Basic xyz"}}
	got := extractToken(true, makeConsumer(
		[]config.FromHeader{{Name: "Authorization", ValuePrefix: "Bearer "}}, nil, nil,
	), hp, &testLogger{T: t})
	require.Empty(t, got)
}

func TestExtractFromHeader_FallbackToNextHeader(t *testing.T) {
	hp := &testProvider{headerMap: map[string]string{"x-tok": "Bearer t"}}
	got := extractToken(true, makeConsumer(
		[]config.FromHeader{
			{Name: "Authorization", ValuePrefix: "Bearer "},
			{Name: "X-Tok", ValuePrefix: "Bearer "},
		}, nil, nil,
	), hp, &testLogger{T: t})
	require.Equal(t, "t", got)
}

func TestExtractFromParams_StripsQuery(t *testing.T) {
	hp := &testProvider{headerMap: map[string]string{":path": "/api?access_token=tok&x=1"}}
	got := extractToken(true, makeConsumer(nil, []string{"access_token"}, nil),
		hp, &testLogger{T: t})
	require.Equal(t, "tok", got)
}

func TestExtractFromParams_NotFoundYieldsEmpty(t *testing.T) {
	hp := &testProvider{headerMap: map[string]string{":path": "/api?x=1"}}
	got := extractToken(true, makeConsumer(nil, []string{"access_token"}, nil),
		hp, &testLogger{T: t})
	require.Empty(t, got)
}

func TestExtractFromCookies_PicksByName(t *testing.T) {
	hp := &testProvider{headerMap: map[string]string{"cookie": "a=1; jwt=tok; b=2"}}
	got := extractToken(true, makeConsumer(nil, nil, []string{"jwt"}),
		hp, &testLogger{T: t})
	require.Equal(t, "tok", got)
}

func TestExtractFromCookies_KeepFalseRewritesHeader(t *testing.T) {
	hp := &testProvider{headerMap: map[string]string{"cookie": "a=1; jwt=tok; b=2"}}
	got := extractToken(false, makeConsumer(nil, nil, []string{"jwt"}),
		hp, &testLogger{T: t})
	require.Equal(t, "tok", got)
	require.NotContains(t, hp.headerMap["cookie"], "jwt=tok")
	require.Contains(t, hp.headerMap["cookie"], "a=1")
	require.Contains(t, hp.headerMap["cookie"], "b=2")
}

func TestExtractToken_PriorityHeaderOverParamsOverCookies(t *testing.T) {
	hp := &testProvider{headerMap: map[string]string{
		"authorization": "Bearer from-header",
		":path":         "/api?access_token=from-params",
		"cookie":        "jwt=from-cookie",
	}}
	got := extractToken(true, makeConsumer(
		[]config.FromHeader{{Name: "Authorization", ValuePrefix: "Bearer "}},
		[]string{"access_token"},
		[]string{"jwt"},
	), hp, &testLogger{T: t})
	require.Equal(t, "from-header", got)
}
