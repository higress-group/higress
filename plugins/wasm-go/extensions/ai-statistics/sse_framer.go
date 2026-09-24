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
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const maxIncompleteSSEEventBytes = 1 * 1024 * 1024 // 1 MiB

// maxSSEDelimiterBytes is the length of the longest event delimiter
// ("\r\n\r\n"). A delimiter split across a callback boundary can therefore
// have at most maxSSEDelimiterBytes-1 bytes in the previous callback, which is
// why the RESYNC matcher carry never exceeds 3 bytes.
const maxSSEDelimiterBytes = 4

// sseFramer is a request-scoped SSE event framer. NOT safe for concurrent use;
// one instance lives in HttpContext for the lifetime of a streaming response.
//
// The framer is a read-only observer: it never mutates the callback chunk and
// never blocks the downstream response. It operates on raw bytes only (no
// provider-specific JSON parsing) and is deliberately provider-neutral so it
// can later be extracted to github.com/higress-group/wasm-go/pkg/sseframe with
// a mechanical mapping: frameCallback -> (*Framer).Frame, drain ->
// (*Framer).Drain, maxIncompleteSSEEventBytes -> WithMaxIncompleteEventBytes.
type sseFramer struct {
	tail          []byte // raw incomplete-event bytes (FRAMING); len <= maxIncompleteSSEEventBytes
	resyncing     bool   // true while in RESYNC
	resyncCarry   []byte // <= 3 bytes of delimiter-matcher state (RESYNC); never content
	overflowCount int    // bounded diagnostic: number of RESYNC entries
}

// frameCallback scans tail‖chunk incrementally and returns the ordered complete
// SSE events found, each normalized with wrapper.UnifySSEChunk after its
// boundary was determined on raw bytes. The final incomplete suffix is
// retained as the new raw tail; if it would exceed maxIncompleteSSEEventBytes
// the framer enters RESYNC instead. In RESYNC, bytes are discarded through the
// next delimiter and scanning resumes within the same callback.
//
// Postconditions: tail is nil or len(tail) <= maxIncompleteSSEEventBytes;
// resyncCarry is nil or len(resyncCarry) <= 3.
func (f *sseFramer) frameCallback(chunk []byte) (events [][]byte) {
	if f.resyncing {
		w := byteWindow{a: f.resyncCarry, b: chunk}
		ends := w.scanEventEnds(1)
		if len(ends) == 0 {
			// No complete delimiter yet (it may be split across this
			// boundary). Retain only the matcher state; discarded content is
			// never stored.
			f.resyncCarry = w.lastBytes(0, maxSSEDelimiterBytes-1)
			return nil
		}
		// Delimiter found: discard everything through it and immediately
		// process the remainder of this same callback as FRAMING input, so
		// valid events following the delimiter are not lost.
		end := ends[0]
		f.resyncing = false
		f.resyncCarry = nil
		f.tail = nil
		if end > len(w.a) {
			chunk = chunk[end-len(w.a):]
		}
		// Otherwise the delimiter completed within the carry (e.g. a carried
		// "\n\r" resolved as LF + bare CR by this chunk's first byte) and the
		// whole chunk remains to be processed.
	}

	w := byteWindow{a: f.tail, b: chunk}
	ends := w.scanEventEnds(0)
	n := w.len()
	prev := 0
	for _, end := range ends {
		var event []byte
		if prev >= len(f.tail) {
			// Event lies entirely inside the current chunk: sub-slice, no
			// copy. The tail is delimiter-free by invariant, so only the
			// first event of a callback can span the tail/chunk boundary.
			event = chunk[prev-len(f.tail) : end-len(f.tail)]
		} else {
			// Event spans the tail/chunk boundary: materialize it, bounded
			// by the event's actual size (complete events are not capped).
			event = w.sliceRange(prev, end)
		}
		// Normalize at emission only; the retained tail is never normalized.
		events = append(events, wrapper.UnifySSEChunk(event))
		prev = end
	}

	// The cap applies only to the final incomplete suffix after all complete
	// events have been emitted; it is enforced before the tail copy so the
	// retained state never exceeds maxIncompleteSSEEventBytes.
	switch suffixLen := n - prev; {
	case suffixLen == 0:
		f.tail = nil
	case suffixLen <= maxIncompleteSSEEventBytes:
		// Bounded copy: the tail must not alias the caller-owned chunk.
		f.tail = w.sliceRange(prev, n)
	default:
		// Drop the oversized suffix and enter RESYNC. Complete events
		// already emitted above are still delivered; only the incomplete
		// suffix is discarded.
		f.tail = nil
		f.resyncing = true
		f.resyncCarry = w.lastBytes(prev, maxSSEDelimiterBytes-1)
		f.overflowCount++
	}
	return events
}

// drain is called when endOfStream == true. It discards any retained tail (an
// unterminated event at EOS is not recoverable, including any pending trailing
// '\r') and returns overflowCount.
func (f *sseFramer) drain() (overflowCount int) {
	f.tail = nil
	f.resyncing = false
	f.resyncCarry = nil
	return f.overflowCount
}

// byteWindow is a read-only view over the virtual concatenation a‖b. It lets
// the scanner run index arithmetic over the retained tail (or RESYNC carry)
// and the current callback chunk without materializing a combined buffer.
type byteWindow struct {
	a []byte // prefix: retained tail (FRAMING) or resyncCarry (RESYNC)
	b []byte // current callback chunk
}

func (w byteWindow) len() int { return len(w.a) + len(w.b) }

func (w byteWindow) at(i int) byte {
	if i < len(w.a) {
		return w.a[i]
	}
	return w.b[i-len(w.a)]
}

// lineEndingEnd classifies the line ending starting at offset i, whose byte
// must be '\r' or '\n', and returns the offset just past it:
//   - '\n'            -> i+1 (LF)
//   - '\r' + '\n'     -> i+2 (CRLF)
//   - '\r' + any other -> i+1 (bare CR line ending, per the SSE spec)
//
// pending is true when the byte at i is a '\r' at the very end of the window:
// the CR is not yet classifiable (it may be the first half of a CRLF), so the
// caller must stop scanning and leave the CR unclassified in the tail until
// the next callback's first byte arrives.
func (w byteWindow) lineEndingEnd(i int) (end int, pending bool) {
	if w.at(i) == '\n' {
		return i + 1, false
	}
	if i+1 == w.len() {
		return i, true
	}
	if w.at(i+1) == '\n' {
		return i + 2, false
	}
	return i + 1, false
}

// scanEventEnds scans the window from offset 0 and returns the end offset of
// each complete event, where a complete event includes its terminating
// delimiter: two consecutive line endings ("\n\n", "\r\n\r\n", the mixed forms
// "\n\r\n"/"\r\n\n", and forms involving bare CR line endings). At most limit
// offsets are returned when limit > 0. Everything after the last returned
// offset is the incomplete suffix: bytes containing no full delimiter yet,
// possibly ending in a pending '\r' whose classification depends on the next
// callback.
func (w byteWindow) scanEventEnds(limit int) []int {
	var ends []int
	n := w.len()
	pos := 0
	for pos < n {
		c := w.at(pos)
		if c != '\n' && c != '\r' {
			pos++
			continue
		}
		firstEnd, pending := w.lineEndingEnd(pos)
		if pending {
			break
		}
		if firstEnd < n {
			if c2 := w.at(firstEnd); c2 == '\n' || c2 == '\r' {
				secondEnd, pending := w.lineEndingEnd(firstEnd)
				if pending {
					break
				}
				// Two consecutive line endings: event delimiter.
				ends = append(ends, secondEnd)
				if limit > 0 && len(ends) >= limit {
					return ends
				}
				pos = secondEnd
				continue
			}
		}
		pos = firstEnd
	}
	return ends
}

// sliceRange returns a fresh copy of w[from:to]. Used for the retained tail,
// the RESYNC carry, and boundary-spanning events; the results must never alias
// the caller-owned chunk (the host reuses that buffer across callbacks).
func (w byteWindow) sliceRange(from, to int) []byte {
	out := make([]byte, 0, to-from)
	if from >= len(w.a) {
		return append(out, w.b[from-len(w.a):to-len(w.a)]...)
	}
	if to <= len(w.a) {
		return append(out, w.a[from:to]...)
	}
	out = append(out, w.a[from:]...)
	return append(out, w.b[:to-len(w.a)]...)
}

// lastBytes returns a fresh copy of the last min(k, len-from) bytes of
// w[from:]. It derives the <= 3-byte RESYNC matcher carry from a dropped
// suffix (or from the not-yet-matched carry‖chunk window).
func (w byteWindow) lastBytes(from, k int) []byte {
	n := w.len()
	start := n - k
	if start < from {
		start = from
	}
	return w.sliceRange(start, n)
}
