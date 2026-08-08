package main

import (
	"errors"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/prefixcache"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
)

type requestBodyControlStub struct {
	dontRead bool
}

func TestOverrideAndRecordOnlyLearnsAfterSuccess(t *testing.T) {
	chains := [][]prefixcache.Block{{{Hash: 1, EstimatedTokens: 32}}}
	index := prefixcache.NewIndex(10)
	failure := errors.New("override failed")
	if err := overrideAndRecord(func([]byte) error { return failure }, index, "a", chains, 16); !errors.Is(err, failure) {
		t.Fatalf("override error=%v want %v", err, failure)
	}
	if index.Len("a") != 0 {
		t.Fatal("failed override recorded prefix")
	}
	var overridden string
	if err := overrideAndRecord(func(address []byte) error {
		overridden = string(address)
		return nil
	}, index, "a", chains, 16); err != nil {
		t.Fatal(err)
	}
	if overridden != "a" || index.Len("a") != 1 || index.UsedCost("a") != 2 {
		t.Fatalf("success boundary endpoint=%q len=%d cost=%d", overridden, index.Len("a"), index.UsedCost("a"))
	}
}

func (s *requestBodyControlStub) DontReadRequestBody() {
	s.dontRead = true
}

func TestRequestHeadersAction(t *testing.T) {
	tests := []struct {
		name    string
		framing requestBodyFraming
		want    types.Action
	}{
		{
			name:    "header-only POST with no framing",
			framing: requestBodyFraming{contentType: "application/json"},
			want:    types.HeaderContinue,
		},
		{
			name:    "explicit zero content length",
			framing: requestBodyFraming{contentLength: "0", contentType: "application/json"},
			want:    types.HeaderContinue,
		},
		{
			name: "compressed JSON",
			framing: requestBodyFraming{
				contentLength: "128", contentType: "application/json", contentEncoding: "gzip",
			},
			want: types.HeaderContinue,
		},
		{
			name: "gRPC",
			framing: requestBodyFraming{
				contentLength: "128", contentType: "application/grpc+proto",
			},
			want: types.HeaderContinue,
		},
		{
			name: "octet stream",
			framing: requestBodyFraming{
				contentLength: "128", contentType: "application/octet-stream",
			},
			want: types.HeaderContinue,
		},
		{
			name: "websocket upgrade",
			framing: requestBodyFraming{
				contentLength: "128", contentType: "application/json", connection: "keep-alive, Upgrade", upgrade: "websocket",
			},
			want: types.HeaderContinue,
		},
		{
			name: "positive content length JSON",
			framing: requestBodyFraming{
				contentLength: "128", contentType: "application/json",
			},
			want: types.HeaderStopIteration,
		},
		{
			name: "chunked JSON",
			framing: requestBodyFraming{
				transferEncoding: "gzip, chunked", contentType: "application/json",
			},
			want: types.HeaderStopIteration,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			control := &requestBodyControlStub{}
			if got := requestHeadersAction(control, test.framing); got != test.want {
				t.Fatalf("requestHeadersAction() = %v, want %v", got, test.want)
			}
			wantDontRead := test.want == types.HeaderContinue
			if control.dontRead != wantDontRead {
				t.Fatalf("DontReadRequestBody called = %v, want %v", control.dontRead, wantDontRead)
			}
		})
	}
}
