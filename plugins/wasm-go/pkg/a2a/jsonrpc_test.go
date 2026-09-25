// Copyright (c) 2026 Alibaba Group Holding Ltd.
// SPDX-License-Identifier: Apache-2.0

package a2a

import (
	"errors"
	"strings"
	"testing"
)

func TestParseRequestMethodsAndIdentifiers(t *testing.T) {
	methods := []string{"SendMessage", "SendStreamingMessage", "GetTask", "ListTasks", "CancelTask", "SubscribeToTask", "CreateTaskPushNotificationConfig", "GetTaskPushNotificationConfig", "ListTaskPushNotificationConfigs", "DeleteTaskPushNotificationConfig", "GetExtendedAgentCard"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":"request-1","method":"` + method + `","params":{"id":"task-from-id","taskId":"task-1","contextId":"context-1","message":{"messageId":"message-1"}}}`
			meta, err := ParseRequest([]byte(body), DefaultMaxRequestBytes, "1.0", false)
			if err != nil {
				t.Fatal(err)
			}
			if meta.Method != method || meta.RequestID != "request-1" || meta.ContextID != "context-1" || meta.MessageID != "message-1" {
				t.Fatalf("unexpected metadata: %#v", meta)
			}
		})
	}
}

func TestObsoletePushNotificationMethodNamesAreRejected(t *testing.T) {
	for _, method := range []string{"SetTaskPushNotificationConfiguration", "GetTaskPushNotificationConfiguration", "ListTaskPushNotificationConfigurations", "DeleteTaskPushNotificationConfiguration"} {
		body := `{"jsonrpc":"2.0","id":1,"method":"` + method + `"}`
		if _, err := ParseRequest([]byte(body), 1024, "1.0", false); !errors.Is(err, ErrUnknownMethod) {
			t.Fatalf("%s: expected unknown method, got %v", method, err)
		}
	}
}

func TestParseRequestExtractsTenant(t *testing.T) {
	meta, err := ParseRequest([]byte(`{"jsonrpc":"2.0","id":"r1","method":"GetTask","params":{"tenant":"tenant-a","id":"task-a"}}`), DefaultMaxRequestBytes, "1.0", false)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Tenant != "tenant-a" || meta.TaskID != "task-a" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
}

func TestParseV1StreamResponseVariants(t *testing.T) {
	tests := []struct {
		name      string
		result    string
		taskID    string
		contextID string
		state     string
		eventType string
	}{
		{"task", `{"task":{"id":"t1","contextId":"c1","status":{"state":"working"}}}`, "t1", "c1", "working", ""},
		{"message", `{"message":{"taskId":"t2","contextId":"c2","messageId":"m2"}}`, "t2", "c2", "", ""},
		{"status update", `{"statusUpdate":{"taskId":"t3","contextId":"c3","status":{"state":"completed"},"final":true}}`, "t3", "c3", "completed", "status"},
		{"artifact update", `{"artifactUpdate":{"taskId":"t4","contextId":"c4","artifact":{"artifactId":"a4"},"append":false}}`, "t4", "c4", "", "artifact"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":"r1","result":` + tt.result + `}`
			meta, err := ParseResponse([]byte(body), 4096, "1.0", "SendStreamingMessage")
			if err != nil {
				t.Fatal(err)
			}
			if meta.TaskID != tt.taskID || meta.ContextID != tt.contextID || meta.TaskState != tt.state || meta.StreamEventType != tt.eventType {
				t.Fatalf("unexpected metadata: %#v", meta)
			}
		})
	}
}

func TestLegacyAliasesAreCanonicalizedWithoutPayloadTranslation(t *testing.T) {
	meta, err := ParseRequest([]byte(`{"jsonrpc":"2.0","id":7,"method":"message/stream","params":{"message":{"messageId":"m-1"}}}`), 1024, "0.3", true)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Method != "SendStreamingMessage" || meta.ParseStatus != "legacy" || meta.RequestID != "7" || meta.MessageID != "m-1" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	if _, err := ParseRequest([]byte(`{"jsonrpc":"2.0","id":7,"method":"message/stream"}`), 1024, "1.0", false); !errors.Is(err, ErrUnknownMethod) {
		t.Fatalf("expected unknown method, got %v", err)
	}
}

func TestParseRequestIsBounded(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"GetTask","params":{"id":"` + strings.Repeat("x", 200) + `"}}`)
	meta, err := ParseRequest(body, 64, "1.0", false)
	if !errors.Is(err, ErrOversized) || meta.ParseStatus != "oversized" {
		t.Fatalf("expected oversized, got %#v %v", meta, err)
	}
}

func TestParseResponseStateAndError(t *testing.T) {
	meta, err := ParseResponse([]byte(`{"jsonrpc":"2.0","id":"r1","result":{"id":"t1","contextId":"c1","status":{"state":"completed"}}}`), 1024, "1.0", "GetTask")
	if err != nil {
		t.Fatal(err)
	}
	if meta.TaskID != "t1" || meta.ContextID != "c1" || meta.TaskState != "completed" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	meta, err = ParseResponse([]byte(`{"jsonrpc":"2.0","id":"r1","error":{"code":-32001,"message":"not found"}}`), 1024, "1.0", "GetTask")
	if err != nil || meta.ErrorCode != "-32001" || meta.StreamEventType != "error" {
		t.Fatalf("unexpected error metadata: %#v %v", meta, err)
	}
}

func FuzzParseRequestBounded(f *testing.F) {
	f.Add([]byte(`{"jsonrpc":"2.0","id":1,"method":"GetTask","params":{"id":"t1"}}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		_, _ = ParseRequest(body, 4096, "1.0", true)
	})
}
