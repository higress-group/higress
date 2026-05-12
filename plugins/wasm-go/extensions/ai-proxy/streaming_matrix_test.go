package main

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/config"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/provider"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/test"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type streamBodyStub struct {
	out []byte
	err error
}

func (s *streamBodyStub) GetProviderType() string { return "stream-body-stub" }

func (s *streamBodyStub) OnStreamingResponseBody(ctx wrapper.HttpContext, name provider.ApiName, chunk []byte, isLastChunk bool) ([]byte, error) {
	_ = ctx
	_ = name
	_ = chunk
	_ = isLastChunk
	return s.out, s.err
}

type streamEventStub struct {
	eventsOut []provider.StreamEvent
	err       error
}

func (s *streamEventStub) GetProviderType() string { return "stream-event-stub" }

func (s *streamEventStub) OnStreamingEvent(ctx wrapper.HttpContext, name provider.ApiName, event provider.StreamEvent) ([]provider.StreamEvent, error) {
	_ = ctx
	_ = name
	_ = event
	return s.eventsOut, s.err
}

func pluginConfigWithStubProvider(t *testing.T, p provider.Provider) config.PluginConfig {
	t.Helper()
	var c config.PluginConfig
	c.FromJson(gjson.Parse(`{"provider":{"type":"generic","genericHost":"http://127.0.0.1:9","apiTokens":["tok"]}}`))
	require.NoError(t, c.Validate())
	require.NoError(t, c.Complete())
	c.SetActiveProviderForTest(p)
	return c
}

func TestOnStreamingResponseBody_matrix(t *testing.T) {
	wasmtest.RunGoTest(t, func(t *testing.T) {
		bootstrap, st := wasmtest.NewTestHost(json.RawMessage(`{"provider":{"type":"generic","genericHost":"http://127.0.0.1:1","apiTokens":["bootstrap"]}}`))
		require.Equal(t, types.OnPluginStartStatusOK, st)
		defer bootstrap.Reset()

		t.Run("nil_provider_returns_chunk", func(t *testing.T) {
			var c config.PluginConfig
			c.FromJson(gjson.Parse(`{"providers":[{"id":"x","type":"generic","genericHost":"http://127.0.0.1:9","apiTokens":["t"]}]}`))
			require.NoError(t, c.Validate())
			require.NoError(t, c.Complete())
			ctx := test.NewMockHttpContext()
			in := []byte("keep")
			out := OnStreamingResponseBodyForTest(ctx, c, in, false)
			require.Equal(t, in, out)
		})

		t.Run("streaming_body_handler_err_returns_original_chunk", func(t *testing.T) {
			stub := &streamBodyStub{out: []byte("x"), err: errors.New("handler failed")}
			pc := pluginConfigWithStubProvider(t, stub)
			ctx := test.NewMockHttpContext()
			ctx.SetContext(provider.CtxKeyApiName, provider.ApiNameChatCompletion)
			in := []byte("original")
			out := OnStreamingResponseBodyForTest(ctx, pc, in, false)
			require.Equal(t, in, out)
		})

		t.Run("streaming_body_handler_nil_modified_returns_original_chunk", func(t *testing.T) {
			stub := &streamBodyStub{out: nil, err: nil}
			pc := pluginConfigWithStubProvider(t, stub)
			ctx := test.NewMockHttpContext()
			ctx.SetContext(provider.CtxKeyApiName, provider.ApiNameChatCompletion)
			in := []byte("original")
			out := OnStreamingResponseBodyForTest(ctx, pc, in, false)
			require.Equal(t, in, out)
		})

		t.Run("streaming_body_handler_ok_returns_modified", func(t *testing.T) {
			stub := &streamBodyStub{out: []byte("modified"), err: nil}
			pc := pluginConfigWithStubProvider(t, stub)
			ctx := test.NewMockHttpContext()
			ctx.SetContext(provider.CtxKeyApiName, provider.ApiNameChatCompletion)
			in := []byte("in")
			out := OnStreamingResponseBodyForTest(ctx, pc, in, false)
			require.Equal(t, "modified", string(out))
		})

		t.Run("streaming_event_handler_zero_events_returns_empty", func(t *testing.T) {
			stub := &streamEventStub{}
			pc := pluginConfigWithStubProvider(t, stub)
			ctx := test.NewMockHttpContext()
			ctx.SetContext(provider.CtxKeyApiName, provider.ApiNameChatCompletion)
			out := OnStreamingResponseBodyForTest(ctx, pc, []byte("incomplete"), false)
			require.Equal(t, []byte(""), out)
		})

		t.Run("streaming_event_handler_on_event_err_returns_chunk", func(t *testing.T) {
			stub := &streamEventStub{err: errors.New("event failed")}
			pc := pluginConfigWithStubProvider(t, stub)
			ctx := test.NewMockHttpContext()
			ctx.SetContext(provider.CtxKeyApiName, provider.ApiNameChatCompletion)
			chunk := []byte("data: {\"x\":1}\n\n")
			out := OnStreamingResponseBodyForTest(ctx, pc, chunk, false)
			require.Equal(t, chunk, out)
		})

		t.Run("no_handler_no_flags_returns_chunk", func(t *testing.T) {
			var c config.PluginConfig
			c.FromJson(gjson.Parse(`{"provider":{"type":"generic","genericHost":"http://127.0.0.1:9","apiTokens":["t"]}}`))
			require.NoError(t, c.Validate())
			require.NoError(t, c.Complete())
			ctx := test.NewMockHttpContext()
			ctx.SetContext(provider.CtxKeyApiName, provider.ApiNameChatCompletion)
			in := []byte("passthrough")
			out := OnStreamingResponseBodyForTest(ctx, c, in, false)
			require.Equal(t, in, out)
		})
	})
}
