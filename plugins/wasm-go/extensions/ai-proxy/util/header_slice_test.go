package util

import (
	"net/http"
	"reflect"
	"testing"
)

func TestCreateHeaders(t *testing.T) {
	h := CreateHeaders("Content-Type", "application/json", ":status", "200")
	if len(h) != 2 {
		t.Fatalf("len=%d", len(h))
	}
	if h[0][0] != "Content-Type" || h[0][1] != "application/json" {
		t.Fatalf("first pair: %v", h[0])
	}
}

func TestHeaderToSliceAndSliceToHeader_roundTrip(t *testing.T) {
	src := make(http.Header)
	src.Set("A", "1")
	src.Add("A", "2")
	src.Set("B", "3")

	slice := HeaderToSlice(src)
	round := SliceToHeader(slice)

	if !reflect.DeepEqual(src["A"], round["A"]) || !reflect.DeepEqual(src["B"], round["B"]) {
		t.Fatalf("roundTrip mismatch: %#v vs %#v", src, round)
	}
}

func TestOverwriteRequestPathHeader(t *testing.T) {
	h := make(http.Header)
	OverwriteRequestPathHeader(h, "/v1/chat/completions")
	if h.Get(":path") != "/v1/chat/completions" {
		t.Fatalf("path=%q", h.Get(":path"))
	}
}
