// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package a2a

import (
	"bytes"
	"errors"
)

// SSEParser incrementally observes complete data events while callers forward
// every input chunk unchanged. Its retained memory is capped by maxEventBytes.
type SSEParser struct {
	maxEventBytes int
	carry         []byte
	oversized     bool
	lineHasData   bool
	skipLF        bool
}

type SSEEvent struct {
	Metadata  Metadata
	Oversized bool
}

func NewSSEParser(maxEventBytes int) *SSEParser {
	if maxEventBytes <= 0 {
		maxEventBytes = DefaultMaxSSEEventBytes
	}
	return &SSEParser{maxEventBytes: maxEventBytes}
}

// Feed returns metadata for complete events. The supplied chunk is not
// retained or changed after Feed returns; callers can forward it immediately.
func (p *SSEParser) Feed(chunk []byte, endOfStream bool, version, method string) []SSEEvent {
	var events []SSEEvent
	for _, b := range chunk {
		if p.oversized {
			if p.scanOversizedByte(b) {
				events = append(events, SSEEvent{Metadata: Metadata{Version: version, Binding: "jsonrpc", Method: method, ParseStatus: "oversized"}, Oversized: true})
			}
			continue
		}
		p.carry = append(p.carry, b)
		if len(p.carry) > p.maxEventBytes {
			p.oversized = true
			p.lineHasData = false
			p.skipLF = false
			complete := false
			for _, buffered := range p.carry {
				complete = p.scanOversizedByte(buffered) || complete
			}
			p.carry = nil
			if complete {
				events = append(events, SSEEvent{Metadata: Metadata{Version: version, Binding: "jsonrpc", Method: method, ParseStatus: "oversized"}, Oversized: true})
			}
			continue
		}
		if (b == '\n' || b == '\r') && eventComplete(p.carry) {
			events = append(events, parseSSEEvent(p.carry, version, method))
			p.carry = nil
		}
	}
	if endOfStream && len(p.carry) > 0 {
		events = append(events, parseSSEEvent(p.carry, version, method))
		p.carry = nil
	}
	if endOfStream && p.oversized {
		events = append(events, SSEEvent{Metadata: Metadata{Version: version, Binding: "jsonrpc", Method: method, ParseStatus: "oversized"}, Oversized: true})
		p.oversized = false
		p.lineHasData = false
		p.skipLF = false
	}
	return events
}

func (p *SSEParser) scanOversizedByte(b byte) bool {
	if b == '\n' && p.skipLF {
		p.skipLF = false
		return false
	}
	if b == '\n' || b == '\r' {
		complete := !p.lineHasData
		p.lineHasData = false
		p.skipLF = b == '\r'
		if complete {
			p.oversized = false
		}
		return complete
	}
	p.skipLF = false
	p.lineHasData = true
	return false
}

func eventComplete(raw []byte) bool {
	return bytes.HasSuffix(raw, []byte("\n\n")) || bytes.HasSuffix(raw, []byte("\r\n\r\n")) || bytes.HasSuffix(raw, []byte("\r\r"))
}

func parseSSEEvent(raw []byte, version, method string) SSEEvent {
	meta := Metadata{Version: version, Binding: "jsonrpc", Method: method, ParseStatus: "skipped"}
	var data []byte
	for len(raw) > 0 {
		line, rest := nextSSELine(raw)
		raw = rest
		if len(line) == 0 || line[0] == ':' {
			continue
		}
		field, value, ok := bytes.Cut(line, []byte{':'})
		if !ok || !bytes.Equal(field, []byte("data")) {
			continue
		}
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}
		if len(data) > 0 {
			data = append(data, '\n')
		}
		data = append(data, value...)
	}
	if len(data) == 0 {
		return SSEEvent{Metadata: meta}
	}
	parsed, err := ParseResponse(data, len(data), version, method)
	if err != nil {
		meta.ParseStatus = "invalid"
		if errors.Is(err, ErrOversized) {
			meta.ParseStatus = "oversized"
		}
		return SSEEvent{Metadata: meta}
	}
	parsed.StreamEventType = classifyStreamEvent(parsed)
	return SSEEvent{Metadata: parsed}
}

func nextSSELine(raw []byte) ([]byte, []byte) {
	for i, b := range raw {
		if b != '\r' && b != '\n' {
			continue
		}
		next := i + 1
		if b == '\r' && next < len(raw) && raw[next] == '\n' {
			next++
		}
		return raw[:i], raw[next:]
	}
	return raw, nil
}

func classifyStreamEvent(meta Metadata) string {
	if meta.StreamEventType != "" {
		return meta.StreamEventType
	}
	if meta.ErrorCode != "" {
		return "error"
	}
	if meta.TaskState != "" {
		return "status"
	}
	if meta.MessageID != "" {
		return "message"
	}
	if meta.TaskID != "" {
		return "task"
	}
	return "unknown"
}
