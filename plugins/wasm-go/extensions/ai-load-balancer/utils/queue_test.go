package utils

import "testing"

func TestFixedQueue_OverwriteAndOrder(t *testing.T) {
	q := NewFixedQueue[int](3)
	if !q.IsEmpty() || q.IsFull() {
		t.Fatalf("new queue state mismatch: empty=%v full=%v", q.IsEmpty(), q.IsFull())
	}

	q.Enqueue(1)
	q.Enqueue(2)
	q.Enqueue(3)
	q.Enqueue(4)

	if !q.IsFull() {
		t.Fatal("queue should be full after overwrite")
	}
	if q.Size() != 3 || q.Capacity() != 3 {
		t.Fatalf("size/capacity = %d/%d, want 3/3", q.Size(), q.Capacity())
	}
	if got := q.ToSlice(); !equalInts(got, []int{2, 3, 4}) {
		t.Fatalf("ToSlice() = %v, want [2 3 4]", got)
	}
	if oldest, err := q.Oldest(); err != nil || oldest != 2 {
		t.Fatalf("Oldest() = %v, %v; want 2, nil", oldest, err)
	}
	if newest, err := q.Newest(); err != nil || newest != 4 {
		t.Fatalf("Newest() = %v, %v; want 4, nil", newest, err)
	}

	var seen []int
	q.ForEach(func(_ int, item int) {
		seen = append(seen, item)
	})
	if !equalInts(seen, []int{2, 3, 4}) {
		t.Fatalf("ForEach saw %v, want [2 3 4]", seen)
	}
}

func TestFixedQueue_DequeuePeekAndClear(t *testing.T) {
	q := NewFixedQueue[string](0)
	if q.Capacity() != 16 {
		t.Fatalf("default capacity = %d, want 16", q.Capacity())
	}
	if _, err := q.Peek(); err == nil {
		t.Fatal("expected empty peek error")
	}
	if _, err := q.Dequeue(); err == nil {
		t.Fatal("expected empty dequeue error")
	}
	if _, err := q.Newest(); err == nil {
		t.Fatal("expected empty newest error")
	}

	q.Enqueue("a")
	q.Enqueue("b")
	if peek, err := q.Peek(); err != nil || peek != "a" {
		t.Fatalf("Peek() = %q, %v; want a, nil", peek, err)
	}
	if item, err := q.Dequeue(); err != nil || item != "a" {
		t.Fatalf("Dequeue() = %q, %v; want a, nil", item, err)
	}
	q.Clear()
	if !q.IsEmpty() || len(q.ToSlice()) != 0 {
		t.Fatalf("queue should be empty after clear, slice=%v", q.ToSlice())
	}
	if q.OverwriteCount() != 0 {
		t.Fatalf("OverwriteCount() = %d, want 0", q.OverwriteCount())
	}
}

func equalInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
