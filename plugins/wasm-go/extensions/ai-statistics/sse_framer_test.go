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

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/stretchr/testify/require"
)

// sseUsageEvent is a realistic ~220-byte single-line OpenAI-style usage event
// (including its "\n\n" delimiter) used for split/reassembly tests.
var sseUsageEvent = []byte(`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}` + "\n\n")

// frameChecked feeds one callback through the framer and asserts the pinned
// postconditions after every callback (Design T-22): the retained tail never
// exceeds maxIncompleteSSEEventBytes and the RESYNC carry never exceeds 3
// bytes. It also asserts the structural invariants that the carry exists only
// while resyncing and that no tail is retained while resyncing.
func frameChecked(t *testing.T, f *sseFramer, chunk []byte) [][]byte {
	t.Helper()
	events := f.frameCallback(chunk)
	require.LessOrEqualf(t, len(f.tail), maxIncompleteSSEEventBytes, "tail len %d exceeds cap", len(f.tail))
	require.LessOrEqualf(t, len(f.resyncCarry), maxSSEDelimiterBytes-1, "resyncCarry len %d exceeds 3", len(f.resyncCarry))
	if f.resyncing {
		require.Nil(t, f.tail, "tail must be nil while resyncing")
	} else {
		require.Nil(t, f.resyncCarry, "resyncCarry must be nil while framing")
	}
	return events
}

// T-01 (framer level): a single intact event in one callback is emitted
// immediately; CRLF forms are normalized at emission only.
func TestSseFramerSingleIntactEvent(t *testing.T) {
	t.Run("lf", func(t *testing.T) {
		f := &sseFramer{}
		chunk := []byte("data: a\n\n")
		orig := append([]byte(nil), chunk...)
		events := frameChecked(t, f, chunk)
		require.Equal(t, [][]byte{[]byte("data: a\n\n")}, events)
		require.Equal(t, orig, chunk, "framer must not mutate the callback chunk")
		require.Nil(t, f.tail)
	})
	t.Run("crlf_normalized_at_emission", func(t *testing.T) {
		f := &sseFramer{}
		events := frameChecked(t, f, []byte("data: a\r\nb\r\n\r\n"))
		require.Equal(t, [][]byte{[]byte("data: a\nb\n\n")}, events)
		require.Nil(t, f.tail)
	})
}

// T-02 (framer level): splits inside the JSON body at the pinned offsets
// (50/100/120/130/140 of the ~220-byte event) reassemble to the intact event;
// exhaustively verified for every split offset.
func TestSseFramerSplitInsideJSONBody(t *testing.T) {
	require.Greater(t, len(sseUsageEvent), 141, "test event must cover the pinned offsets")
	for _, off := range []int{50, 100, 120, 130, 140} {
		t.Run(fmt.Sprintf("offset_%d", off), func(t *testing.T) {
			f := &sseFramer{}
			require.Empty(t, frameChecked(t, f, sseUsageEvent[:off]))
			events := frameChecked(t, f, sseUsageEvent[off:])
			require.Equal(t, [][]byte{wrapper.UnifySSEChunk(sseUsageEvent)}, events)
		})
	}
	t.Run("every_offset", func(t *testing.T) {
		for off := 1; off < len(sseUsageEvent); off++ {
			f := &sseFramer{}
			frameChecked(t, f, sseUsageEvent[:off])
			events := frameChecked(t, f, sseUsageEvent[off:])
			require.Equalf(t, [][]byte{wrapper.UnifySSEChunk(sseUsageEvent)}, events, "split at offset %d", off)
		}
	})
}

// T-03 (framer level): a split inside the "data:" prefix reassembles by byte
// shape, not by "data:" parsing.
func TestSseFramerSplitInsideDataPrefix(t *testing.T) {
	f := &sseFramer{}
	require.Empty(t, frameChecked(t, f, []byte("da")))
	events := frameChecked(t, f, []byte("ta: {\"usage\":{\"total_tokens\":30}}\n\n"))
	require.Equal(t, [][]byte{[]byte("data: {\"usage\":{\"total_tokens\":30}}\n\n")}, events)
}

// T-04 (framer level): an LF delimiter split across callbacks emits the event
// only after the second "\n" arrives.
func TestSseFramerLFDelimiterSplit(t *testing.T) {
	f := &sseFramer{}
	require.Empty(t, frameChecked(t, f, []byte("data: a\n")))
	require.Equal(t, []byte("data: a\n"), f.tail, "raw tail retained un-normalized")
	events := frameChecked(t, f, []byte("\n"))
	require.Equal(t, [][]byte{[]byte("data: a\n\n")}, events)
	require.Nil(t, f.tail)
}

// T-05 (framer level): a CRLF delimiter split at every byte boundary is
// recognized and the event is emitted exactly once.
func TestSseFramerCRLFDelimiterSplitEveryBoundary(t *testing.T) {
	normalized := []byte("data: a\n\n")
	splits := []struct {
		name          string
		first, second string
	}{
		{"after_cr", "data: a\r", "\n\r\n"},
		{"after_crlf", "data: a\r\n", "\r\n"},
		{"after_crlf_cr", "data: a\r\n\r", "\n"},
	}
	for _, tc := range splits {
		t.Run(tc.name, func(t *testing.T) {
			f := &sseFramer{}
			require.Empty(t, frameChecked(t, f, []byte(tc.first)))
			events := frameChecked(t, f, []byte(tc.second))
			require.Equal(t, [][]byte{normalized}, events)
			require.Nil(t, f.tail)
		})
	}
}

// Exhaustive boundary sweep (T-05/T-06 framer level): a two-event CRLF stream
// with a mid-event CRLF line ending, split at every byte offset, always
// yields the same two normalized events.
func TestSseFramerCRLFStreamSplitEveryBoundary(t *testing.T) {
	stream := []byte("data: one\r\n\r\ndata: two\r\nthree\r\n\r\n")
	want := [][]byte{[]byte("data: one\n\n"), []byte("data: two\nthree\n\n")}
	for off := 0; off <= len(stream); off++ {
		f := &sseFramer{}
		var got [][]byte
		got = append(got, frameChecked(t, f, stream[:off])...)
		got = append(got, frameChecked(t, f, stream[off:])...)
		require.Equalf(t, want, got, "split at offset %d", off)
		require.Nilf(t, f.tail, "split at offset %d", off)
	}
}

// T-06 (framer level): a CRLF line ending split mid-event ("...foo\r" |
// "\nbar...") must NOT produce a false event boundary (regression for
// premature normalization of a trailing raw CR).
func TestSseFramerCRLFLineEndingSplitMidEvent(t *testing.T) {
	f := &sseFramer{}
	require.Empty(t, frameChecked(t, f, []byte("data: foo\r")))
	events := frameChecked(t, f, []byte("\nbar\r\n\r\n"))
	require.Equal(t, [][]byte{[]byte("data: foo\nbar\n\n")}, events)
	require.Nil(t, f.tail)
}

// T-07 (framer level): a trailing raw '\r' is held un-normalized in the tail
// and classified when the next callback's first byte arrives.
func TestSseFramerPendingTrailingCR(t *testing.T) {
	t.Run("classified_as_crlf", func(t *testing.T) {
		f := &sseFramer{}
		require.Empty(t, frameChecked(t, f, []byte("data: a\r")))
		require.Equal(t, []byte("data: a\r"), f.tail, "trailing CR stays raw and unclassified")
		// "\n" completes the CRLF line ending; it is not yet a delimiter.
		require.Empty(t, frameChecked(t, f, []byte("\n")))
		require.Equal(t, []byte("data: a\r\n"), f.tail, "tail is never normalized")
		events := frameChecked(t, f, []byte("\n"))
		require.Equal(t, [][]byte{[]byte("data: a\n\n")}, events)
	})
	t.Run("classified_as_bare_cr", func(t *testing.T) {
		f := &sseFramer{}
		require.Empty(t, frameChecked(t, f, []byte("data: a\r")))
		require.Equal(t, []byte("data: a\r"), f.tail)
		// A non-"\n" first byte classifies the CR as a bare CR line ending
		// (SSE spec): a mid-event line break, not a delimiter.
		events := frameChecked(t, f, []byte("b\n\n"))
		require.Equal(t, [][]byte{[]byte("data: a\nb\n\n")}, events)
	})
}

// T-08 (framer level): one callback larger than 1 MiB containing many complete
// small events emits all of them in order and does NOT enter RESYNC; the cap
// applies only to the final incomplete suffix.
func TestSseFramerLargeCallbackOfCompleteEvents(t *testing.T) {
	event := []byte("data: " + strings.Repeat("x", 1016) + "\n\n") // 1024 bytes
	const count = 2049                                             // > 2 MiB of complete events
	buf := make([]byte, 0, len(event)*count)
	for i := 0; i < count; i++ {
		buf = append(buf, event...)
	}
	buf = append(buf, []byte("data: incomplete-tail")...)
	require.Greater(t, len(buf), maxIncompleteSSEEventBytes)

	f := &sseFramer{}
	events := frameChecked(t, f, buf)
	require.Len(t, events, count)
	for i, ev := range events {
		require.Equalf(t, event, ev, "event %d", i)
	}
	require.False(t, f.resyncing, "a large callback of complete events must not trigger RESYNC")
	require.Equal(t, 0, f.overflowCount)
	require.Equal(t, []byte("data: incomplete-tail"), f.tail)
}

// Multiple events in one callback are emitted in stream order; mixed
// delimiter forms ("\n\n", "\r\n\r\n", "\n\r\n", "\r\n\n") are recognized.
func TestSseFramerMultipleEventsOneCallback(t *testing.T) {
	f := &sseFramer{}
	events := frameChecked(t, f, []byte("data: 1\n\ndata: 2\r\n\r\ndata: 3\n\r\ndata: 4\r\n\n"))
	require.Equal(t, [][]byte{
		[]byte("data: 1\n\n"),
		[]byte("data: 2\n\n"),
		[]byte("data: 3\n\n"),
		[]byte("data: 4\n\n"),
	}, events)
	require.Nil(t, f.tail)
}

// T-23 (framer level): one event delivered across many 1-byte callbacks is
// emitted exactly once, on the completing callback.
func TestSseFramerOneEventAcrossOneByteCallbacks(t *testing.T) {
	t.Run("lf", func(t *testing.T) {
		f := &sseFramer{}
		var got [][]byte
		for i := 0; i < len(sseUsageEvent); i++ {
			events := frameChecked(t, f, sseUsageEvent[i:i+1])
			if i < len(sseUsageEvent)-1 {
				require.Emptyf(t, events, "callback %d emitted prematurely", i)
			} else {
				got = events
			}
		}
		require.Equal(t, [][]byte{wrapper.UnifySSEChunk(sseUsageEvent)}, got)
		require.Nil(t, f.tail)
	})
	t.Run("crlf_pending_cr_every_step", func(t *testing.T) {
		stream := []byte("data: a\r\nb\r\n\r\n")
		want := []byte("data: a\nb\n\n")
		f := &sseFramer{}
		var got [][]byte
		for i := 0; i < len(stream); i++ {
			events := frameChecked(t, f, stream[i:i+1])
			if i < len(stream)-1 {
				require.Emptyf(t, events, "callback %d emitted prematurely", i)
			} else {
				got = events
			}
		}
		require.Equal(t, [][]byte{want}, got)
	})
}

// T-25 (framer level): an incomplete suffix of exactly 1 MiB is retained as
// the tail (no RESYNC); the completed event (larger than the cap) is still
// delivered once its delimiter arrives, because the cap applies only to the
// retained suffix, never to complete events.
func TestSseFramerSuffixExactlyOneMiB(t *testing.T) {
	f := &sseFramer{}
	suffix := make([]byte, maxIncompleteSSEEventBytes)
	copy(suffix, "data: ")
	for i := 6; i < len(suffix); i++ {
		suffix[i] = 'a'
	}
	require.Empty(t, frameChecked(t, f, suffix))
	require.False(t, f.resyncing)
	require.Equal(t, 0, f.overflowCount)
	require.Len(t, f.tail, maxIncompleteSSEEventBytes)
	require.Equal(t, suffix, f.tail)

	intact := append(append([]byte(nil), suffix...), '\n', '\n')
	events := frameChecked(t, f, []byte("\n\n"))
	require.Equal(t, [][]byte{wrapper.UnifySSEChunk(intact)}, events)
	require.Nil(t, f.tail)
}

// T-26 (framer level): an incomplete suffix of 1 MiB+1 enters RESYNC (suffix
// dropped, tail nil, carry <= 3 bytes); a subsequent valid event after the
// next delimiter is still recovered.
func TestSseFramerSuffixOneMiBPlusOne(t *testing.T) {
	f := &sseFramer{}
	suffix := make([]byte, maxIncompleteSSEEventBytes+1)
	copy(suffix, "data: ")
	for i := 6; i < len(suffix); i++ {
		suffix[i] = 'a'
	}
	require.Empty(t, frameChecked(t, f, suffix))
	require.True(t, f.resyncing)
	require.Nil(t, f.tail)
	require.Equal(t, 1, f.overflowCount)
	require.Equal(t, []byte("aaa"), f.resyncCarry)

	events := frameChecked(t, f, []byte("bbb\n\ndata: ok\n\n"))
	require.Equal(t, [][]byte{[]byte("data: ok\n\n")}, events)
	require.False(t, f.resyncing)
	require.Nil(t, f.tail)
	require.Equal(t, 1, f.drain())
}

// T-13 + T-14 (framer level): when a callback emits complete events and then
// overflows on the final suffix, the complete events are still delivered; and
// after the overflow, a delimiter arriving mid-callback followed by valid
// events emits those events from that same callback.
func TestSseFramerOversizedSuffixResumeSameCallback(t *testing.T) {
	f := &sseFramer{}
	big := make([]byte, 0, maxIncompleteSSEEventBytes+16)
	big = append(big, []byte("data: first\n\n")...)
	big = append(big, []byte("data: ")...)
	for len(big) < maxIncompleteSSEEventBytes+16 {
		big = append(big, 'a')
	}
	events := frameChecked(t, f, big)
	require.Equal(t, [][]byte{[]byte("data: first\n\n")}, events,
		"complete events emitted before the overflow are still delivered")
	require.True(t, f.resyncing)
	require.Equal(t, 1, f.overflowCount)

	// The delimiter arrives mid-callback, followed by valid events.
	events = frameChecked(t, f, []byte("aa\n\ndata: second\n\ndata: third\n\n"))
	require.Equal(t, [][]byte{[]byte("data: second\n\n"), []byte("data: third\n\n")}, events,
		"valid events after the delimiter must be emitted from the same callback")
	require.False(t, f.resyncing)
	require.Nil(t, f.tail)
}

// T-15 (framer level): while in RESYNC, a delimiter split across callbacks is
// recognized; processing resumes only after the full delimiter.
func TestSseFramerResyncDelimiterSplitAcrossCallbacks(t *testing.T) {
	newOverflowedFramer := func(t *testing.T, suffixEnd []byte) *sseFramer {
		f := &sseFramer{}
		big := make([]byte, maxIncompleteSSEEventBytes+1)
		for i := range big {
			big[i] = 'a'
		}
		copy(big, "data: ")
		copy(big[len(big)-len(suffixEnd):], suffixEnd)
		require.Empty(t, frameChecked(t, f, big))
		require.True(t, f.resyncing)
		require.Equal(t, 1, f.overflowCount)
		return f
	}

	t.Run("crlf_cr_then_lf", func(t *testing.T) {
		// Suffix ends with "\r\n\r"; the delimiter completes only when the
		// next callback's "\n" arrives.
		f := newOverflowedFramer(t, []byte("\r\n\r"))
		require.Equal(t, []byte("\r\n\r"), f.resyncCarry)
		require.Empty(t, frameChecked(t, f, []byte("\n")))
		require.False(t, f.resyncing)
		events := frameChecked(t, f, []byte("data: ok\n\n"))
		require.Equal(t, [][]byte{[]byte("data: ok\n\n")}, events)
	})
	t.Run("lf_then_lf", func(t *testing.T) {
		f := newOverflowedFramer(t, []byte("x"))
		// A single LF is not a delimiter: still resyncing.
		require.Empty(t, frameChecked(t, f, []byte("\n")))
		require.True(t, f.resyncing)
		events := frameChecked(t, f, []byte("\ndata: ok\n\n"))
		require.Equal(t, [][]byte{[]byte("data: ok\n\n")}, events)
		require.False(t, f.resyncing)
	})
	t.Run("cr_then_split_crlf_crlf", func(t *testing.T) {
		f := newOverflowedFramer(t, []byte("\r"))
		// "\r\n" + a pending "\r": not a full delimiter yet, no resume.
		require.Empty(t, frameChecked(t, f, []byte("\n\r")))
		require.True(t, f.resyncing)
		// "\r\n\r\n" completes across three callbacks.
		require.Empty(t, frameChecked(t, f, []byte("\n")))
		require.False(t, f.resyncing)
		events := frameChecked(t, f, []byte("data: ok\n\n"))
		require.Equal(t, [][]byte{[]byte("data: ok\n\n")}, events)
	})
}

// T-24 (framer level): at end-of-stream, drain discards an incomplete tail
// (including a pending '\r') without fabricating an event; input remaining at
// EOS while resyncing is discarded as well.
func TestSseFramerDrainDiscardsIncompleteTail(t *testing.T) {
	t.Run("partial_event", func(t *testing.T) {
		f := &sseFramer{}
		events := frameChecked(t, f, []byte("data: done\n\ndata: partial"))
		require.Equal(t, [][]byte{[]byte("data: done\n\n")}, events)
		require.Equal(t, []byte("data: partial"), f.tail)
		require.Equal(t, 0, f.drain())
		require.Nil(t, f.tail, "unterminated event at EOS is discarded, not emitted")
		require.False(t, f.resyncing)
	})
	t.Run("pending_cr", func(t *testing.T) {
		f := &sseFramer{}
		require.Empty(t, frameChecked(t, f, []byte("data: x\r")))
		require.Equal(t, []byte("data: x\r"), f.tail)
		require.Equal(t, 0, f.drain())
		require.Nil(t, f.tail)
	})
	t.Run("while_resyncing", func(t *testing.T) {
		f := &sseFramer{}
		big := make([]byte, maxIncompleteSSEEventBytes+1)
		for i := range big {
			big[i] = 'a'
		}
		require.Empty(t, frameChecked(t, f, big))
		require.True(t, f.resyncing)
		require.Empty(t, frameChecked(t, f, []byte("still no delimiter")))
		require.Equal(t, 1, f.drain())
		require.False(t, f.resyncing)
		require.Nil(t, f.resyncCarry)
	})
}

// T-21 (framer level): drain returns the number of RESYNC entries.
func TestSseFramerDrainReturnsOverflowCount(t *testing.T) {
	f := &sseFramer{}
	overflow := func() {
		big := make([]byte, maxIncompleteSSEEventBytes+1)
		for i := range big {
			big[i] = 'a'
		}
		require.Empty(t, frameChecked(t, f, big))
		require.True(t, f.resyncing)
	}
	overflow()
	// Resume through a delimiter, then overflow a second time.
	require.Empty(t, frameChecked(t, f, []byte("\n\n")))
	require.False(t, f.resyncing)
	overflow()
	require.Equal(t, 2, f.drain())
}

// The retained tail is a bounded copy: mutating a delivered chunk (the host
// reuses callback buffers) must not corrupt retained state.
func TestSseFramerTailDoesNotAliasChunk(t *testing.T) {
	f := &sseFramer{}
	chunk1 := []byte("data: abc")
	require.Empty(t, frameChecked(t, f, chunk1))
	for i := range chunk1 {
		chunk1[i] = 'X'
	}
	events := frameChecked(t, f, []byte("def\n\n"))
	require.Equal(t, [][]byte{[]byte("data: abcdef\n\n")}, events)
}

// Bare CR line endings (SSE spec): a '\r' not followed by '\n' is one line
// ending; two consecutive line endings involving bare CRs still delimit an
// event. This follows from the pinned pending-CR classification rule.
func TestSseFramerBareCRLineEndings(t *testing.T) {
	t.Run("lf_then_bare_cr_split", func(t *testing.T) {
		f := &sseFramer{}
		require.Empty(t, frameChecked(t, f, []byte("data: a\n\r")))
		require.Equal(t, []byte("data: a\n\r"), f.tail)
		events := frameChecked(t, f, []byte("x\n\n"))
		require.Equal(t, [][]byte{[]byte("data: a\n\n"), []byte("x\n\n")}, events)
	})
	t.Run("cr_cr_single_callback", func(t *testing.T) {
		f := &sseFramer{}
		events := frameChecked(t, f, []byte("data: a\r\rb\n\n"))
		require.Equal(t, [][]byte{[]byte("data: a\n\n"), []byte("b\n\n")}, events)
	})
}

// Byte-shape only: empty events and comment (keepalive) events are emitted
// as-is; consumers gate on content.
func TestSseFramerEmptyAndCommentEvents(t *testing.T) {
	f := &sseFramer{}
	events := frameChecked(t, f, []byte("\n\n: keepalive\r\n\r\n"))
	require.Equal(t, [][]byte{[]byte("\n\n"), []byte(": keepalive\n\n")}, events)
	require.Nil(t, f.tail)
}

// Empty callbacks are no-ops and do not disturb retained state.
func TestSseFramerEmptyChunk(t *testing.T) {
	f := &sseFramer{}
	require.Empty(t, frameChecked(t, f, nil))
	require.Nil(t, f.tail)
	require.Empty(t, frameChecked(t, f, []byte("data: a\n")))
	require.Equal(t, []byte("data: a\n"), f.tail)
	require.Empty(t, frameChecked(t, f, nil))
	require.Equal(t, []byte("data: a\n"), f.tail)
	events := frameChecked(t, f, []byte("\n"))
	require.Equal(t, [][]byte{[]byte("data: a\n\n")}, events)
}
