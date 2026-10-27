package demo

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNativeSDKDirectReadsInstructionsAndCallsTools(t *testing.T) {
	instructions := "先调用 get_weather 查询天气。\n中文、空白与换行必须保持。\n"
	fixture, handler := NewFixture(instructions)
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	report, err := Run(ctx, Options{Endpoint: server.URL + "/mcp", AdminURL: server.URL,
		ExpectedInstructions: instructions, UpstreamInstructions: instructions, ServerName: UpstreamName,
		Phase: "native-direct", ExpectedMethods: []string{"server/discover", "tools/list", "tools/call"}})
	if err != nil {
		t.Fatal(err)
	}
	if report.SDKResult.Instructions != instructions || len(fixture.Snapshot().Events) != 3 {
		t.Fatalf("SDK string or method ledger changed: %+v", report)
	}
}

func TestNativeSDKRejectsWrongInstructions(t *testing.T) {
	_, handler := NewFixture("expected\n")
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := Run(ctx, Options{Endpoint: server.URL + "/mcp", AdminURL: server.URL,
		ExpectedInstructions: "different\n", UpstreamInstructions: "expected\n", ServerName: UpstreamName, Phase: "negative"})
	if err == nil || !strings.Contains(err.Error(), "instructions mismatch") {
		t.Fatalf("wrong instructions accepted: %v", err)
	}
}

func TestNativeSDKObjectOmitsEmptyInstructions(t *testing.T) {
	_, handler := NewFixture("")
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	report, err := Run(ctx, Options{Endpoint: server.URL + "/mcp", AdminURL: server.URL,
		ServerName: UpstreamName, Phase: "empty", ExpectedMethods: []string{"server/discover", "tools/list", "tools/call"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report.SDKResult)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if _, present := fields["instructions"]; present {
		t.Fatalf("empty SDK instructions unexpectedly present: %s", data)
	}
}
