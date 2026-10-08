// Copyright (c) 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.

package server

import (
	"fmt"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

func TestRouteCallErrorAndCancellationPreventSubmission(t *testing.T) {
	for _, mode := range []string{"error", "cancel", "success"} {
		t.Run(mode, func(t *testing.T) {
			host := newProxyBridgeHost(t, ProtocolStrategyModern)
			host.CallOnHttpRequestHeaders(modernProxyListHeaders())
			ctx := modernProxyTestContext("tools/list", modernProxyListBody(1))
			ctx.SetContext(utils.CtxNeedPause, true)
			registerProxyCancellation(ctx)
			e := ctx.GetContext(CtxMcpProxyCancel).(*proxyExchange)
			if mode == "error" {
				ctx.routeError = assert.AnError
			}
			if mode == "cancel" {
				ctx.beforeReturn = func() { e.mu.Lock(); e.cancelled = true; e.mu.Unlock() }
			}
			called := false
			err := routeProxyBusiness(ctx, "http://backend/mcp", nil, modernProxyListBody(1), func(int, [][2]string, []byte) { called = true })
			if mode == "success" {
				require.NoError(t, err)
				assert.True(t, e.submitted)
				assert.Equal(t, false, ctx.GetContext(utils.CtxNeedPause))
			} else {
				require.Error(t, err)
				assert.False(t, e.submitted)
				assert.Equal(t, true, ctx.GetContext(utils.CtxNeedPause))
			}
			finishProxyRequest(ctx)
			ctx.responseCallback(200, nil, []byte(`{"raw":"must-not-escape"}`))
			assert.False(t, called, "completion/stream-done suppresses late callback")
			assert.Nil(t, host.GetLocalResponse())
		})
	}
}

func TestLiveControlCancellationTerminatesWithoutResume(t *testing.T) {
	for _, auto := range []bool{false, true} {
		t.Run(fmt.Sprint(auto), func(t *testing.T) {
			host := newProxyBridgeHost(t, ProtocolStrategyModern)
			host.CallOnHttpRequestHeaders(modernProxyListHeaders())
			ctx := modernProxyTestContext("tools/list", modernProxyListBody(1))
			if auto {
				e := &AutoExchange{phase: autoProbing, ctx: ctx, prepared: &PreparedProxyRequest{}}
				e.cancel()
				e.replyCancelled(ctx)
				e.replyCancelled(ctx)
			} else {
				registerProxyCancellation(ctx)
				e := ctx.GetContext(CtxMcpProxyCancel).(*proxyExchange)
				e.mu.Lock()
				e.cancelled = true
				e.mu.Unlock()
				assert.False(t, e.controlActive(ctx))
				assert.False(t, e.controlActive(ctx))
			}
			require.NotNil(t, host.GetLocalResponse())
			assert.Equal(t, uint32(499), host.GetLocalResponse().StatusCode)
			assert.Equal(t, types.ActionPause, host.GetHttpStreamAction())
			host.CompleteHttp()
		})
	}
}

func TestProxyCancellationAndCompletionRace(t *testing.T) {
	for i := 0; i < 100; i++ {
		e := &proxyExchange{}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); e.mu.Lock(); e.cancelled = true; e.mu.Unlock() }()
		go func() { defer wg.Done(); e.finish() }()
		wg.Wait()
		assert.True(t, e.cancelled)
		assert.True(t, e.closed)
	}
}
