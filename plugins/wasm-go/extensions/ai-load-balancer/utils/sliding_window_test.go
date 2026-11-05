package utils

import (
	"testing"
	"time"
)

func TestSlidingWindow_PushAndSize(t *testing.T) {
	w := NewSlidingWindow[string](10, 60)
	if w.Size() != 0 {
		t.Fatalf("expected size 0, got %d", w.Size())
	}
	w.Push("a")
	w.Push("b")
	w.Push("a")
	if w.Size() != 3 {
		t.Fatalf("expected size 3, got %d", w.Size())
	}
}

func TestSlidingWindow_CountInWindow(t *testing.T) {
	w := NewSlidingWindow[string](100, 60)
	w.Push("pod-a")
	w.Push("pod-b")
	w.Push("pod-a")
	w.Push("pod-a")
	w.Push("pod-b")

	countA := w.CountInWindow(func(item string) bool { return item == "pod-a" })
	if countA != 3 {
		t.Fatalf("expected 3 for pod-a, got %d", countA)
	}
	countB := w.CountInWindow(func(item string) bool { return item == "pod-b" })
	if countB != 2 {
		t.Fatalf("expected 2 for pod-b, got %d", countB)
	}
}

func TestSlidingWindow_PurgeExpired(t *testing.T) {
	w := NewSlidingWindow[string](100, 1) // 1 second window
	w.Push("old-entry")
	// Wait for the entry to expire
	time.Sleep(1100 * time.Millisecond)
	w.Push("new-entry")
	size := w.Size()
	if size != 1 {
		t.Fatalf("expected size 1 after purge, got %d", size)
	}
	countOld := w.CountInWindow(func(item string) bool { return item == "old-entry" })
	if countOld != 0 {
		t.Fatalf("expected 0 for old-entry after purge, got %d", countOld)
	}
	countNew := w.CountInWindow(func(item string) bool { return item == "new-entry" })
	if countNew != 1 {
		t.Fatalf("expected 1 for new-entry, got %d", countNew)
	}
}

func TestSlidingWindow_MaxSizeCap(t *testing.T) {
	w := NewSlidingWindow[int](3, 60)
	w.Push(1)
	w.Push(2)
	w.Push(3)
	w.Push(4) // should evict 1
	if w.Size() != 3 {
		t.Fatalf("expected size 3 after cap, got %d", w.Size())
	}
	// Verify oldest entry (1) is gone
	count1 := w.CountInWindow(func(item int) bool { return item == 1 })
	if count1 != 0 {
		t.Fatalf("expected 0 for evicted entry 1, got %d", count1)
	}
	count4 := w.CountInWindow(func(item int) bool { return item == 4 })
	if count4 != 1 {
		t.Fatalf("expected 1 for entry 4, got %d", count4)
	}
}

func TestSlidingWindow_IsEmpty(t *testing.T) {
	w := NewSlidingWindow[string](10, 60)
	if !w.IsEmpty() {
		t.Fatal("expected empty window")
	}
	w.Push("x")
	if w.IsEmpty() {
		t.Fatal("expected non-empty window")
	}
}

func TestSlidingWindow_Clear(t *testing.T) {
	w := NewSlidingWindow[string](10, 60)
	w.Push("a")
	w.Push("b")
	w.Clear()
	if w.Size() != 0 {
		t.Fatalf("expected size 0 after clear, got %d", w.Size())
	}
}

func TestSlidingWindow_Defaults(t *testing.T) {
	w := NewSlidingWindow[string](0, 0)
	w.Push("test")
	if w.Size() != 1 {
		t.Fatalf("expected size 1, got %d", w.Size())
	}
}

func TestSlidingWindow_ForEach(t *testing.T) {
	w := NewSlidingWindow[string](10, 60)
	w.Push("a")
	w.Push("b")
	w.Push("c")
	seen := []string{}
	w.ForEach(func(_ int, item string) {
		seen = append(seen, item)
	})
	if len(seen) != 3 {
		t.Fatalf("expected 3 items, got %d", len(seen))
	}
	if seen[0] != "a" || seen[1] != "b" || seen[2] != "c" {
		t.Fatalf("unexpected order: %v", seen)
	}
}
