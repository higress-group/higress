// Copyright (c) 2026 Alibaba Group Holding Ltd.
// SPDX-License-Identifier: Apache-2.0

package a2a

import (
	"bytes"
	"testing"
)

func TestSSEChunkBoundariesAndUnknownBytes(t *testing.T) {
	input := []byte(": keep-alive\r\nid: 7\r\nunknown: untouched\r\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"id\":\"t1\",\"status\":{\"state\":\"working\"}}}\r\n\r\n")
	for split := 0; split <= len(input); split++ {
		parser := NewSSEParser(4096)
		forwarded := append([]byte(nil), input[:split]...)
		events := parser.Feed(input[:split], false, "1.0", "SendStreamingMessage")
		forwarded = append(forwarded, input[split:]...)
		events = append(events, parser.Feed(input[split:], true, "1.0", "SendStreamingMessage")...)
		if !bytes.Equal(forwarded, input) {
			t.Fatalf("split %d changed forwarded bytes", split)
		}
		if len(events) != 1 || events[0].Metadata.TaskID != "t1" || events[0].Metadata.TaskState != "working" || events[0].Metadata.StreamEventType != "status" {
			t.Fatalf("split %d: unexpected events %#v", split, events)
		}
	}
}

func TestSSEOversizedEventReleasesMemoryAndResynchronizes(t *testing.T) {
	parser := NewSSEParser(128)
	input := append([]byte("data: "), bytes.Repeat([]byte("x"), 200)...)
	input = append(input, []byte("\n\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"id\":\"t2\"}}\n\n")...)
	events := parser.Feed(input, true, "1.0", "SubscribeToTask")
	if len(events) != 2 || !events[0].Oversized || events[1].Metadata.TaskID != "t2" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestSSEAcceptsCRLineEndings(t *testing.T) {
	parser := NewSSEParser(4096)
	events := parser.Feed([]byte("event: status\rdata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"id\":\"t3\"}}\r\r"), true, "1.0", "SubscribeToTask")
	if len(events) != 1 || events[0].Metadata.TaskID != "t3" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestSSEClassifiesArtifactUpdateWithoutCopyingArtifact(t *testing.T) {
	parser := NewSSEParser(4096)
	events := parser.Feed([]byte("data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"kind\":\"task-artifact-update\",\"taskId\":\"t4\",\"artifact\":{\"parts\":[{\"raw\":\"secret\"}]}}}\n\n"), true, "1.0", "SendStreamingMessage")
	if len(events) != 1 || events[0].Metadata.StreamEventType != "artifact" || events[0].Metadata.TaskID != "t4" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestSSEParsesV1StatusAndArtifactUpdateOneof(t *testing.T) {
	parser := NewSSEParser(4096)
	input := []byte("data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"statusUpdate\":{\"taskId\":\"t5\",\"contextId\":\"c5\",\"status\":{\"state\":\"working\"}}}}\n\n" +
		"data: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"artifactUpdate\":{\"taskId\":\"t5\",\"contextId\":\"c5\",\"artifact\":{\"parts\":[{\"raw\":\"secret\"}]}}}}\n\n")
	events := parser.Feed(input, true, "1.0", "SendStreamingMessage")
	if len(events) != 2 || events[0].Metadata.StreamEventType != "status" || events[0].Metadata.TaskState != "working" || events[1].Metadata.StreamEventType != "artifact" || events[1].Metadata.TaskID != "t5" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func FuzzSSEChunkBoundaries(f *testing.F) {
	f.Add([]byte("data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n"), uint8(8))
	f.Fuzz(func(t *testing.T, input []byte, rawSplit uint8) {
		if len(input) > 8192 {
			t.Skip()
		}
		split := 0
		if len(input) > 0 {
			split = int(rawSplit) % (len(input) + 1)
		}
		parser := NewSSEParser(1024)
		parser.Feed(input[:split], false, "1.0", "SendStreamingMessage")
		parser.Feed(input[split:], true, "1.0", "SendStreamingMessage")
	})
}
