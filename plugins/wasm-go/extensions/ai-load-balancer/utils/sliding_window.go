package utils

import (
	"time"
)

// TimedEntry holds a value with its insertion timestamp.
type TimedEntry[T any] struct {
	Value     T
	Timestamp int64 // UnixMilli
}

// SlidingWindow implements a time-based sliding window.
// Unlike FixedQueue which keeps the last N items regardless of time,
// SlidingWindow purges entries older than the configured window duration,
// providing accurate rate calculation for bursty or low-QPS traffic.
type SlidingWindow[T any] struct {
	entries []TimedEntry[T]
	maxSize int   // hard cap to prevent unbounded memory growth
	window  int64 // window duration in milliseconds
}

// NewSlidingWindow creates a SlidingWindow with the given size cap and
// window duration (in seconds).
func NewSlidingWindow[T any](maxSize int, windowSeconds int) *SlidingWindow[T] {
	if maxSize <= 0 {
		maxSize = 100
	}
	if windowSeconds <= 0 {
		windowSeconds = 60
	}
	return &SlidingWindow[T]{
		entries: make([]TimedEntry[T], 0, maxSize),
		maxSize: maxSize,
		window:  int64(windowSeconds) * 1000,
	}
}

// Push adds a value with the current timestamp, then purges expired entries.
func (w *SlidingWindow[T]) Push(value T) {
	now := time.Now().UnixMilli()
	w.entries = append(w.entries, TimedEntry[T]{Value: value, Timestamp: now})
	w.purge(now)
	// Enforce hard cap: if still over maxSize after purge (high QPS burst),
	// remove oldest entries.
	if len(w.entries) > w.maxSize {
		excess := len(w.entries) - w.maxSize
		w.entries = w.entries[excess:]
	}
}

// purge removes entries older than the window duration.
func (w *SlidingWindow[T]) purge(now int64) {
	cutoff := now - w.window
	idx := 0
	for idx < len(w.entries) && w.entries[idx].Timestamp < cutoff {
		idx++
	}
	if idx > 0 {
		w.entries = w.entries[idx:]
	}
}

// CountInWindow returns how many entries in the current window match the predicate.
func (w *SlidingWindow[T]) CountInWindow(match func(item T) bool) int {
	now := time.Now().UnixMilli()
	w.purge(now)
	count := 0
	for _, e := range w.entries {
		if match(e.Value) {
			count++
		}
	}
	return count
}

// Size returns the number of entries currently in the window.
func (w *SlidingWindow[T]) Size() int {
	now := time.Now().UnixMilli()
	w.purge(now)
	return len(w.entries)
}

// Clear removes all entries.
func (w *SlidingWindow[T]) Clear() {
	w.entries = w.entries[:0]
}

// IsEmpty returns true if the window has no entries.
func (w *SlidingWindow[T]) IsEmpty() bool {
	return w.Size() == 0
}

// ForEach iterates over all in-window entries.
func (w *SlidingWindow[T]) ForEach(fn func(index int, item T)) {
	now := time.Now().UnixMilli()
	w.purge(now)
	for i, e := range w.entries {
		fn(i, e.Value)
	}
}
