package provider

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractStreamingEvents(t *testing.T) {
	t.Run("empty_chunk", func(t *testing.T) {
		ctx := newMapCtx()
		events := ExtractStreamingEvents(ctx, nil)
		assert.Empty(t, events)
	})

	t.Run("crlf_normalized", func(t *testing.T) {
		ctx := newMapCtx()
		chunk := "event:msg\r\ndata:{\"k\":1}\r\n\r\n"
		events := ExtractStreamingEvents(ctx, []byte(chunk))
		require.NotEmpty(t, events)
	})

	t.Run("qwen_style_block", func(t *testing.T) {
		ctx := newMapCtx()
		chunk := "event:result\n:HTTP_STATUS/200\ndata:{\"output\":1}\n\n"
		events := ExtractStreamingEvents(ctx, []byte(chunk))
		require.NotEmpty(t, events)
		foundData := false
		for _, e := range events {
			if strings.Contains(e.RawEvent, "data:") {
				foundData = true
			}
		}
		assert.True(t, foundData, "expected a data line in parsed events: %#v", events)
	})

	t.Run("split_chunk_buffers_incomplete", func(t *testing.T) {
		ctx := newMapCtx()
		part1 := []byte("event:a\n")
		_ = ExtractStreamingEvents(ctx, part1)
		buf, has := ctx.GetContext(ctxKeyStreamingBody).([]byte)
		require.True(t, has, "expected streaming body buffer after incomplete chunk")
		require.NotEmpty(t, buf)

		part2 := []byte("data:{}\n\n")
		events := ExtractStreamingEvents(ctx, part2)
		require.NotEmpty(t, events)
	})
}
