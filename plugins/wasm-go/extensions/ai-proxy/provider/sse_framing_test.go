package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func feedAll(t *testing.T, key string, chunks ...string) []string {
	t.Helper()
	ctx := newMapCtx()
	var events []string
	for i, c := range chunks {
		events = append(events, frameSSEEvents(ctx, key, []byte(c), i == len(chunks)-1)...)
	}
	return events
}

func TestFrameSSEEvents(t *testing.T) {
	t.Run("empty_chunk_no_events", func(t *testing.T) {
		ctx := newMapCtx()
		assert.Empty(t, frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, nil, false))
	})

	t.Run("single_complete_event", func(t *testing.T) {
		events := feedAll(t, ctxKeyClaudeSSEFraming, "data: {\"a\":1}\n\n")
		require.Equal(t, []string{`data: {"a":1}`}, events)
	})

	t.Run("multiple_events_one_callback_in_order", func(t *testing.T) {
		events := feedAll(t, ctxKeyClaudeSSEFraming, "data: {\"a\":1}\n\ndata: {\"b\":2}\n\ndata: [DONE]\n\n")
		require.Equal(t, []string{`data: {"a":1}`, `data: {"b":2}`, "data: [DONE]"}, events)
	})

	t.Run("split_at_every_byte_offset_matches_intact", func(t *testing.T) {
		intact := "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":42}}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
		want := feedAll(t, ctxKeyClaudeSSEFraming, intact)
		require.Len(t, want, 2)
		for i := 0; i < len(intact); i++ {
			got := feedAll(t, ctxKeyClaudeSSEFraming, intact[:i], intact[i:])
			require.Equal(t, want, got, "split at offset %d", i)
		}
	})

	t.Run("one_event_across_many_callbacks", func(t *testing.T) {
		event := "data: {\"type\":\"message_stop\",\"usage\":{\"output_tokens\":7}}\n\n"
		ctx := newMapCtx()
		var events []string
		for i := 0; i < len(event); i++ {
			events = append(events, frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte{event[i]}, false)...)
		}
		events = append(events, frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, nil, true)...)
		require.Equal(t, []string{strings.TrimRight(event, "\n")}, events)
	})

	t.Run("crlf_and_mixed_delimiters", func(t *testing.T) {
		events := feedAll(t, ctxKeyClaudeSSEFraming, "data: {\"a\":1}\r\n\r\ndata: {\"b\":2}\n\r\n")
		require.Equal(t, []string{`data: {"a":1}`, `data: {"b":2}`}, events)
	})

	t.Run("crlf_delimiter_split_between_cr_and_lf", func(t *testing.T) {
		events := feedAll(t, ctxKeyClaudeSSEFraming, "data: {\"a\":1}\r\n\r", "\ndata: {\"b\":2}\n\n")
		require.Equal(t, []string{`data: {"a":1}`, `data: {"b":2}`}, events)
	})

	t.Run("pending_cr_at_window_end_not_misread", func(t *testing.T) {
		ctx := newMapCtx()
		// First callback ends with a bare CR that may be the first half of CRLF.
		events := frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte("data: {\"a\":1}\n\r"), false)
		assert.Empty(t, events, "pending CR must not be classified yet")
		events = frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte("\n"), false)
		require.Equal(t, []string{`data: {"a":1}`}, events)
	})

	t.Run("multi_line_event_preserves_lines", func(t *testing.T) {
		events := feedAll(t, ctxKeyClaudeSSEFraming, "event: message_start\ndata: {\"a\":1}\n\n")
		require.Equal(t, []string{"event: message_start\ndata: {\"a\":1}"}, events)
	})

	t.Run("empty_events_between_delimiters_skipped_cleanly", func(t *testing.T) {
		events := feedAll(t, ctxKeyClaudeSSEFraming, "\n\ndata: {\"a\":1}\n\n\n\n")
		require.Equal(t, []string{"", `data: {"a":1}`, ""}, events)
	})

	t.Run("isLastChunk_flushes_unterminated_tail", func(t *testing.T) {
		ctx := newMapCtx()
		events := frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte("data: {\"a\":1}\n\n"), false)
		require.Equal(t, []string{`data: {"a":1}`}, events)
		events = frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte(`data: {"final":true}`), true)
		require.Equal(t, []string{`data: {"final":true}`}, events)
	})

	t.Run("isLastChunk_empty_chunk_empty_tail_no_output", func(t *testing.T) {
		ctx := newMapCtx()
		events := frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte("data: {\"a\":1}\n\n"), false)
		require.Len(t, events, 1)
		assert.Empty(t, frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, nil, true))
	})

	t.Run("oversized_tail_dropped_fail_open_then_resyncs", func(t *testing.T) {
		ctx := newMapCtx()
		big := strings.Repeat("x", maxIncompleteSSEEventBytes+1)
		events := frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte("data: "+big), false)
		assert.Empty(t, events, "oversized incomplete event must not be delivered")
		// Next delimiter resynchronizes within the same callback and the
		// following complete event is still delivered.
		events = frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte("more\n\ndata: {\"ok\":1}\n\n"), false)
		require.Equal(t, []string{`data: {"ok":1}`}, events)
	})

	t.Run("per_callsite_keys_do_not_alias", func(t *testing.T) {
		ctx := newMapCtx()
		// Simulate provider framing and converter framing on the same request.
		providerEvents := frameSSEEvents(ctx, ctxKeyGeminiSSEFraming, []byte("data: {\"p\":1"), false)
		converterEvents := frameSSEEvents(ctx, ctxKeyClaudeConvertSSEFraming, []byte("data: {\"c\":1"), false)
		assert.Empty(t, providerEvents)
		assert.Empty(t, converterEvents)
		providerEvents = frameSSEEvents(ctx, ctxKeyGeminiSSEFraming, []byte("}\n\n"), false)
		converterEvents = frameSSEEvents(ctx, ctxKeyClaudeConvertSSEFraming, []byte("}\n\n"), false)
		require.Equal(t, []string{`data: {"p":1}`}, providerEvents)
		require.Equal(t, []string{`data: {"c":1}`}, converterEvents)
	})

	t.Run("key_isolated_from_extract_streaming_events", func(t *testing.T) {
		ctx := newMapCtx()
		// Interleave the shared helper with ExtractStreamingEvents on the same
		// request context; neither may observe the other's retained tail.
		_ = frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte("data: {\"helper\":"), false)
		_ = ExtractStreamingEvents(ctx, []byte("data: {\"extract\":"))
		helperEvents := frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte("1}\n\n"), false)
		extractEvents := ExtractStreamingEvents(ctx, []byte("1}\n\n"))
		require.Equal(t, []string{`data: {"helper":1}`}, helperEvents)
		require.Len(t, extractEvents, 1)
		assert.Contains(t, extractEvents[0].Data, `"extract":1`)
	})

	t.Run("partial_garbage_tail_flushed_once_for_provider_to_reject", func(t *testing.T) {
		ctx := newMapCtx()
		events := frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte("data: {\"truncat"), true)
		require.Equal(t, []string{"data: {\"truncat"}, events,
			"flush delivers the raw tail once; the provider's strict unmarshal rejects it")
	})

	t.Run("flush_while_resyncing_emits_nothing", func(t *testing.T) {
		ctx := newMapCtx()
		big := strings.Repeat("x", maxIncompleteSSEEventBytes+1)
		events := frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte("data: "+big), false)
		require.Empty(t, events)
		framer, _ := ctx.GetContext(ctxKeyClaudeSSEFraming).(*sseFramer)
		require.NotNil(t, framer)
		require.True(t, framer.resyncing, "oversized tail must enter RESYNC")
		// End-of-stream while resyncing: the dropped suffix is never emitted.
		events = frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte("tail-bytes-without-delimiter"), true)
		assert.Empty(t, events)
		assert.False(t, framer.resyncing, "flush must reset RESYNC state")
	})

	t.Run("resync_recovers_complete_event_in_same_callback", func(t *testing.T) {
		ctx := newMapCtx()
		big := strings.Repeat("x", maxIncompleteSSEEventBytes+1)
		events := frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte("data: "+big), false)
		require.Empty(t, events)
		// The next callback carries the delimiter ending the dropped event,
		// followed by a valid event that must still be delivered.
		events = frameSSEEvents(ctx, ctxKeyClaudeSSEFraming, []byte("tail\n\ndata: {\"ok\":1}\n\n"), false)
		require.Equal(t, []string{`data: {"ok":1}`}, events)
	})
}

func TestFrameSSEEventsClaudeUsageSurvivesSplitMessageStart(t *testing.T) {
	// End-to-end at the provider level: message_start split across callbacks
	// must still land its input_tokens in the final usage chunk.
	messageStart := `data: {"type":"message_start","message":{"id":"msg_1","role":"assistant","content":[],"model":"claude-3","usage":{"input_tokens":42,"output_tokens":1}}}`
	textDelta := `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`
	messageDelta := `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":9}}`
	messageStop := `data: {"type":"message_stop"}`
	intact := messageStart + "\n\n" + textDelta + "\n\n" + messageDelta + "\n\n" + messageStop + "\n\n"

	render := func(t *testing.T, chunks ...string) string {
		t.Helper()
		p := &claudeProvider{}
		ctx := newMapCtx()
		var out strings.Builder
		for i, c := range chunks {
			b, err := p.OnStreamingResponseBody(ctx, ApiNameChatCompletion, []byte(c), i == len(chunks)-1)
			require.NoError(t, err)
			out.Write(b)
		}
		return out.String()
	}

	baseline := render(t, intact)
	require.Contains(t, baseline, `"prompt_tokens":42`)
	// completion follows the provider's existing accumulation: message_start's
	// output_tokens (1) plus message_delta's output_tokens (9).
	require.Contains(t, baseline, `"completion_tokens":10`)
	require.Contains(t, baseline, `"total_tokens":52`)

	for i := 1; i < len(intact); i++ {
		got := render(t, intact[:i], intact[i:])
		require.Equal(t, baseline, got, "split at offset %d", i)
	}
}

func TestFrameSSEEventsClaudeTerminalEventInLastChunk(t *testing.T) {
	// The terminal message_stop arriving (a) in the last callback and
	// (b) without its trailing blank line must still be converted.
	p := &claudeProvider{}
	ctx := newMapCtx()
	out1, err := p.OnStreamingResponseBody(ctx, ApiNameChatCompletion,
		[]byte(`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":5,"output_tokens":0}}}`+"\n\n"), false)
	require.NoError(t, err)
	require.NotEmpty(t, out1)
	out2, err := p.OnStreamingResponseBody(ctx, ApiNameChatCompletion,
		[]byte(`data: {"type":"message_stop"}`), true)
	require.NoError(t, err)
	require.Contains(t, string(out2), `"prompt_tokens":5`, "flushed message_stop must carry accumulated usage")
}

func TestFrameSSEEventsClaudeNonChatPassthroughUnchanged(t *testing.T) {
	p := &claudeProvider{}
	ctx := newMapCtx()
	chunk := []byte("data: raw-non-sse-bytes")
	out, err := p.OnStreamingResponseBody(ctx, ApiNameCompletion, chunk, false)
	require.NoError(t, err)
	assert.Equal(t, chunk, out, "non-chat paths pass through untouched")
	out, err = p.OnStreamingResponseBody(ctx, ApiNameCompletion, chunk, true)
	require.NoError(t, err)
	assert.Equal(t, chunk, out, "non-chat last chunk passes through untouched")
}

func BenchmarkFrameSSEEvents(b *testing.B) {
	ctx := newMapCtx()
	chunk := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello world\"}}]}\n\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = frameSSEEvents(ctx, fmt.Sprintf("bench-%d", i), chunk, false)
	}
}
