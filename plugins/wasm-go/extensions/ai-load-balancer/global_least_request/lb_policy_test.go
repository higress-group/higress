package global_least_request

import (
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/utils"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/resp"
)

type incrCall struct {
	key   string
	field string
	delta int
}

type fakeCounter struct {
	calls []incrCall
}

func (f *fakeCounter) HIncrBy(key, field string, delta int, _ wrapper.RedisResponseCallback) error {
	f.calls = append(f.calls, incrCall{key: key, field: field, delta: delta})
	return nil
}

func TestCompensationHostFromReply(t *testing.T) {
	cases := []struct {
		name string
		// reply is what the Lua script left behind after it had already
		// incremented the host it selected.
		reply []resp.Value
		want  string
	}{
		{
			name:  "empty reply names no host",
			reply: nil,
			want:  "",
		},
		{
			name:  "truncated reply still names the incremented host",
			reply: []resp.Value{resp.StringValue("10.0.0.1:80")},
			want:  "10.0.0.1:80",
		},
		{
			name:  "complete reply names the same host",
			reply: []resp.Value{resp.StringValue("10.0.0.2:80"), resp.IntegerValue(3)},
			want:  "10.0.0.2:80",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := compensationHostFromReply(tc.reply); got != tc.want {
				t.Fatalf("compensationHostFromReply(%+v) = %q, want %q", tc.reply, got, tc.want)
			}
		})
	}
}

// The malformed-reply branch derives the host from the truncated reply and hands
// it to the shared helper; together they must undo exactly one increment.
func TestTruncatedReplyCompensatesExactlyOnce(t *testing.T) {
	client := &fakeCounter{}
	reply := []resp.Value{resp.StringValue("10.0.0.1:80")}

	err := utils.CompensateCounter(client, "higress:global_least_request_table:route:cluster",
		compensationHostFromReply(reply))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.calls) != 1 {
		t.Fatalf("expected exactly one compensation, got %d: %+v", len(client.calls), client.calls)
	}
	got := client.calls[0]
	if got.key != "higress:global_least_request_table:route:cluster" ||
		got.field != "10.0.0.1:80" || got.delta != -1 {
		t.Fatalf("unexpected compensation call: %+v", got)
	}
}

// Nothing was incremented for a host that the reply does not name, so the branch
// must not issue a blind decrement.
func TestEmptyReplyCompensatesNothing(t *testing.T) {
	client := &fakeCounter{}

	err := utils.CompensateCounter(client, "key", compensationHostFromReply(nil))
	if err == nil {
		t.Fatal("expected an error when the reply names no host")
	}

	if len(client.calls) != 0 {
		t.Fatalf("expected no compensation when the reply names no host, got %+v", client.calls)
	}
}
