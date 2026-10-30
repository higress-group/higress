package utils

import (
	"errors"
	"strings"
	"testing"

	"github.com/higress-group/wasm-go/pkg/wrapper"
)

type incrCall struct {
	key   string
	field string
	delta int
}

type fakeCounter struct {
	calls []incrCall
	err   error
}

func (f *fakeCounter) HIncrBy(key, field string, delta int, _ wrapper.RedisResponseCallback) error {
	f.calls = append(f.calls, incrCall{key: key, field: field, delta: delta})
	return f.err
}

func TestCompensateCounterUndoesOneIncrement(t *testing.T) {
	client := &fakeCounter{}

	err := CompensateCounter(client, "higress:global_least_request_table:route:cluster", "10.0.0.1:80")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.calls) != 1 {
		t.Fatalf("expected exactly one HIncrBy call, got %d: %+v", len(client.calls), client.calls)
	}
	got := client.calls[0]
	if got.key != "higress:global_least_request_table:route:cluster" ||
		got.field != "10.0.0.1:80" || got.delta != -1 {
		t.Fatalf("unexpected compensation call: %+v", got)
	}
}

func TestCompensateCounterRejectsUnknownHost(t *testing.T) {
	client := &fakeCounter{}

	err := CompensateCounter(client, "key", "")
	if err == nil {
		t.Fatal("expected an error when the incremented host is unknown")
	}

	if len(client.calls) != 0 {
		t.Fatalf("expected no compensation when the host is unknown, got %+v", client.calls)
	}
}

func TestCompensateCounterReportsRedisFailures(t *testing.T) {
	client := &fakeCounter{err: errors.New("redis unavailable")}

	err := CompensateCounter(client, "key", "10.0.0.1:80")
	if err == nil {
		t.Fatal("expected the Redis failure to be reported to the caller")
	}
	if !strings.Contains(err.Error(), "redis unavailable") {
		t.Fatalf("expected the Redis error to be wrapped, got %v", err)
	}

	if len(client.calls) != 1 {
		t.Fatalf("expected the compensation to be attempted once, got %d", len(client.calls))
	}
}
