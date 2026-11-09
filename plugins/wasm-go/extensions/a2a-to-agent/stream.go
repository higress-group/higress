// Copyright 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/tidwall/gjson"
)

// SSE decoding retains only an incomplete frame, including split CRLF boundaries.
type sseDecoder struct {
	pending []byte
	limit   int
}
type sseEvent struct{ Name, Data string }

func (s *sseDecoder) feed(chunk []byte, last bool) ([]sseEvent, error) {
	s.pending = append(s.pending, chunk...)
	events := []sseEvent{}
	for {
		end := -1
		sep := 0
		for i := 0; i < len(s.pending); i++ {
			if i+1 < len(s.pending) && s.pending[i] == '\n' && s.pending[i+1] == '\n' {
				end = i
				sep = 2
				break
			}
			if i+3 < len(s.pending) && bytes.Equal(s.pending[i:i+4], []byte("\r\n\r\n")) {
				end = i
				sep = 4
				break
			}
		}
		if end < 0 {
			break
		}
		if end > s.limit {
			return events, errors.New("upstream SSE event exceeds limit")
		}
		frame := s.pending[:end]
		name := ""
		data := []string{}
		for _, line := range strings.Split(strings.ReplaceAll(string(frame), "\r\n", "\n"), "\n") {
			if strings.HasPrefix(line, ":") {
				continue
			}
			field, value, ok := strings.Cut(line, ":")
			if !ok {
				value = ""
			}
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "event":
				name = value
			case "data":
				data = append(data, value)
			}
		}
		if len(data) > 0 {
			events = append(events, sseEvent{name, strings.Join(data, "\n")})
		}
		s.pending = s.pending[end+sep:]
	}
	if len(s.pending) > s.limit {
		return events, errors.New("upstream SSE event exceeds limit")
	}
	// SSE specification dispatches only blank-line-terminated messages.
	if last && len(bytes.TrimSpace(s.pending)) > 0 {
		return events, errors.New("truncated upstream SSE frame")
	}
	if len(s.pending) == 0 {
		s.pending = nil
	}
	return events, nil
}

type streamMapper struct {
	c                      config
	r                      *request
	decoder                sseDecoder
	started, done, running bool
	state                  string
	text                   string
	received               int
	cloudEventID           string
	managedRootThread      string
	managedPreviews        map[string]map[int]string
	managedCompleted       map[string]bool
	messageTexts           map[string]string
	messageOrder           []string
	deferred               []sseEvent
	deferredBytes          int
}

func newMapper(c config, r *request) *streamMapper {
	if r.Session != "" && r.ContextID == "" {
		r.ContextID = c.signContext(r.Session, r.Consumer)
	}
	return &streamMapper{c: c, r: r, decoder: sseDecoder{limit: c.MaxBodyBytes}, state: "TASK_STATE_WORKING", messageTexts: map[string]string{}}
}
func (m *streamMapper) context(session string) {
	if session != "" && session != m.r.Session {
		m.r.Session = session
		m.r.ContextID = m.c.signContext(session, m.r.Consumer)
	}
}
func (m *streamMapper) envelope(result any) []byte {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m.r.ID, "result": result})
	return append(append([]byte("data: "), b...), []byte("\n\n")...)
}
func (m *streamMapper) base() map[string]any {
	b := map[string]any{"taskId": m.r.TaskID}
	if m.r.ContextID != "" {
		b["contextId"] = m.r.ContextID
	}
	return b
}
func (m *streamMapper) task() map[string]any {
	t := map[string]any{"id": m.r.TaskID, "status": map[string]any{"state": m.state}}
	if m.r.ContextID != "" {
		t["contextId"] = m.r.ContextID
	}
	return t
}
func (m *streamMapper) begin() []byte {
	if m.started {
		return nil
	}
	m.started = true
	return m.envelope(map[string]any{"task": m.task()})
}
func (m *streamMapper) status(state, msg string) []byte {
	b := m.base()
	s := map[string]any{"state": state}
	if msg != "" {
		s["message"] = map[string]any{"messageId": m.r.TaskID + "-status", "role": "ROLE_AGENT", "parts": []any{map[string]any{"text": msg}}}
	}
	b["status"] = s
	return m.envelope(map[string]any{"statusUpdate": b})
}
func (m *streamMapper) artifact(text string, appendText, last bool) []byte {
	b := m.base()
	b["artifact"] = map[string]any{"artifactId": m.r.TaskID + "-answer", "parts": []any{map[string]any{"text": text}}}
	b["append"] = appendText
	b["lastChunk"] = last
	return m.envelope(map[string]any{"artifactUpdate": b})
}
func (m *streamMapper) delta(text string, replace bool) []byte {
	if text == "" && !replace {
		return nil
	}
	if replace {
		m.text = text
	} else {
		m.text += text
	}
	if len(m.text) > m.c.MaxBodyBytes {
		return m.finish("TASK_STATE_FAILED", "upstream answer exceeds limit")
	}
	return m.artifact(text, !replace, false)
}
func (m *streamMapper) finish(state, msg string) []byte {
	if m.r.ContextID == "" {
		if m.done {
			return nil
		}
		m.done = true
		m.state = "TASK_STATE_FAILED"
		return append(append([]byte("data: "), rpcError(m.r.ID, -32603, msg)...), []byte("\n\n")...)
	}
	if m.done {
		return nil
	}
	m.done = true
	out := m.begin()
	m.state = state
	if state == "TASK_STATE_COMPLETED" {
		out = append(out, m.artifact("", true, true)...)
	}
	return append(out, m.status(state, msg)...)
}
func (m *streamMapper) feed(chunk []byte, last bool) []byte {
	if m.done {
		return nil
	}
	m.received += len(chunk)
	if m.received > 16*m.c.MaxBodyBytes {
		return m.finish("TASK_STATE_FAILED", "upstream stream exceeds limit")
	}
	events, err := m.decoder.feed(chunk, last)
	var out []byte
	for _, ev := range events {
		if m.done {
			break
		}
		if ev.Data == "[DONE]" || ev.Name == "done" {
			continue
		}
		if !gjson.Valid(ev.Data) {
			out = append(out, m.finish("TASK_STATE_FAILED", "invalid upstream event JSON")...)
			break
		}
		v := gjson.Parse(ev.Data)
		nativeID := nativeSession(m.c.Provider, v)
		if nativeID != "" && m.r.Session != "" && nativeID != m.r.Session {
			out = append(out, m.finish("TASK_STATE_FAILED", "upstream session identity changed")...)
			break
		}
		m.context(nativeID)
		if m.r.ContextID == "" {
			if v.Get("event").String() == "error" || v.Get("code").String() != "" || ev.Name == "error" || ev.Name == "conversation.chat.failed" {
				out = append(out, m.finish("TASK_STATE_FAILED", "upstream agent failed before session creation")...)
				break
			}
			m.deferredBytes += len(ev.Data)
			if m.deferredBytes > m.c.MaxBodyBytes {
				out = append(out, m.finish("TASK_STATE_FAILED", "upstream did not provide session identity within limit")...)
				break
			}
			m.deferred = append(m.deferred, ev)
			continue
		}
		out = append(out, m.begin()...)
		for _, pending := range m.deferred {
			if !m.done {
				out = append(out, m.event(pending.Name, gjson.Parse(pending.Data))...)
			}
		}
		m.deferred = nil
		m.deferredBytes = 0
		if !m.done {
			out = append(out, m.event(ev.Name, v)...)
		}
	}
	if err != nil && !m.done {
		out = append(out, m.finish("TASK_STATE_FAILED", err.Error())...)
	}
	if last && !m.done {
		out = append(out, m.finish("TASK_STATE_FAILED", "upstream closed without a terminal event")...)
	}
	return out
}
func nativeSession(provider string, v gjson.Result) string {
	switch provider {
	case "dify", "coze":
		return v.Get("conversation_id").String()
	case "bailian":
		return v.Get("output.session_id").String()
	}
	return ""
}
func (m *streamMapper) event(name string, v gjson.Result) []byte {
	switch m.c.Provider {
	case "dify":
		switch v.Get("event").String() {
		case "message", "agent_message":
			return m.delta(v.Get("answer").String(), false)
		case "message_replace":
			return m.delta(v.Get("answer").String(), true)
		case "agent_thought":
			return m.progress("agent_thought", v)
		case "message_end":
			return m.finish("TASK_STATE_COMPLETED", "")
		case "error":
			return m.finish("TASK_STATE_FAILED", "Dify agent failed")
		}
	case "bailian":
		if v.Get("code").String() != "" {
			return m.finish("TASK_STATE_FAILED", "Bailian agent failed")
		}
		out := m.delta(v.Get("output.text").String(), false)
		if v.Get("output.thoughts").IsArray() {
			out = append(out, m.progress("thoughts", v.Get("output.thoughts"))...)
		}
		reason := v.Get("output.finish_reason").String()
		if reason == "stop" {
			out = append(out, m.finish("TASK_STATE_COMPLETED", "")...)
		} else if reason != "" && reason != "null" {
			out = append(out, m.finish("TASK_STATE_FAILED", "Bailian stopped: "+reason)...)
		}
		return out
	case "coze":
		switch name {
		case "conversation.message.delta":
			if v.Get("type").String() == "answer" {
				if id := v.Get("id").String(); id != "" {
					return m.messageTextUpdate(id, v.Get("content").String(), true)
				}
				return m.delta(v.Get("content").String(), false)
			}
			return m.progress(v.Get("type").String(), v)
		case "conversation.message.completed":
			if v.Get("type").String() == "answer" {
				if id := v.Get("id").String(); id != "" {
					return m.messageTextUpdate(id, v.Get("content").String(), false)
				}
				full := v.Get("content").String()
				if full == m.text {
					return nil
				}
				if strings.HasPrefix(full, m.text) {
					return m.delta(strings.TrimPrefix(full, m.text), false)
				}
				return m.delta(full, true)
			}
		case "conversation.chat.completed":
			return m.finish("TASK_STATE_COMPLETED", "")
		case "conversation.chat.failed", "error":
			return m.finish("TASK_STATE_FAILED", "Coze agent failed")
		case "conversation.chat.requires_action":
			return m.finish("TASK_STATE_INPUT_REQUIRED", "External tool results are required; this adapter does not submit tool results")
		}
	case "qoder", "claude-managed":
		return m.cloudEvent(v)
	case "bailian-managed":
		return m.bailianManagedEvent(v)
	}
	return nil
}
func (m *streamMapper) progress(event string, v gjson.Result) []byte {
	b := m.base()
	b["status"] = map[string]any{"state": "TASK_STATE_WORKING"}
	var data any
	_ = json.Unmarshal([]byte(v.Raw), &data)
	b["metadata"] = map[string]any{"provider": m.c.Provider, "event": event, "data": data}
	return m.envelope(map[string]any{"statusUpdate": b})
}
func (m *streamMapper) cloudEvent(v gjson.Result) []byte {
	if id := v.Get("session_id").String(); id != "" && id != m.r.Session {
		return nil
	}
	// Thread completion belongs to a child task, not the whole agent run.
	typ := v.Get("type").String()
	if id := v.Get("event.session_id").String(); id != "" && id != m.r.Session {
		return nil
	}
	switch typ {
	case "session.status_running":
		m.running = true
		return m.progress(typ, v)
	case "session.status_idle":
		if !m.running {
			return nil
		}
		switch v.Get("stop_reason.type").String() {
		case "end_turn":
			return m.finish("TASK_STATE_COMPLETED", "")
		case "requires_action":
			return m.finish("TASK_STATE_INPUT_REQUIRED", "Agent requires external action; tool-result continuation is unsupported")
		default:
			return m.finish("TASK_STATE_FAILED", "Agent stopped: "+v.Get("stop_reason.type").String())
		}
	case "session.status_terminated":
		return m.finish("TASK_STATE_FAILED", "Agent session terminated")
	case "session.error":
		return m.progress(typ, v)
	case "event_start":
		if v.Get("event.type").String() == "agent.message" {
			m.cloudEventID = v.Get("event.id").String()
		}
		return nil
	case "event_delta":
		if !m.running {
			return nil
		}
		id := v.Get("event_id").String()
		if id == "" || id != m.cloudEventID || v.Get("delta.type").String() != "content_delta" || v.Get("delta.content.type").String() != "text" {
			return nil
		}
		return m.messageTextUpdate(id, v.Get("delta.content.text").String(), true)
	case "agent.message":
		if !m.running {
			return nil
		}
		return m.messageTextUpdate(v.Get("id").String(), cloudText(v), false)
	case "agent.thinking", "agent.tool_use", "agent.tool_result", "agent.mcp_tool_use", "agent.mcp_tool_result", "agent.custom_tool_use", "agent.custom_tool_result", "agent.artifact_delivered", "span.model_request_start", "span.model_request_end", "session.status_rescheduled":
		if m.running {
			return m.progress(typ, v)
		}
	}
	return nil
}

// Delta previews may stop before the authoritative buffered event. Reconcile
// full messages by event ID instead of discarding snapshots or duplicating text.
func (m *streamMapper) messageTextUpdate(id, text string, delta bool) []byte {
	if id == "" {
		return m.finish("TASK_STATE_FAILED", "agent message is missing its event id")
	}
	if _, exists := m.messageTexts[id]; !exists {
		m.messageOrder = append(m.messageOrder, id)
	}
	if delta {
		m.messageTexts[id] += text
	} else {
		m.messageTexts[id] = text
	}
	var b strings.Builder
	for _, key := range m.messageOrder {
		b.WriteString(m.messageTexts[key])
	}
	full := b.String()
	if full == m.text {
		return nil
	}
	if strings.HasPrefix(full, m.text) {
		return m.delta(strings.TrimPrefix(full, m.text), false)
	}
	return m.delta(full, true)
}
func cloudText(v gjson.Result) string {
	var b strings.Builder
	for _, p := range v.Get("content").Array() {
		if p.Get("type").String() == "text" {
			b.WriteString(p.Get("text").String())
		}
	}
	return b.String()
}
func (m *streamMapper) blocking(body []byte, ok bool) []byte {
	if !ok {
		return rpcError(m.r.ID, -32603, "upstream agent request failed")
	}
	m.feed(body, true)
	if m.r.ContextID == "" {
		return rpcError(m.r.ID, -32603, "upstream did not provide a session identity")
	}
	t := m.task()
	if m.text != "" {
		t["artifacts"] = []any{map[string]any{"artifactId": m.r.TaskID + "-answer", "parts": []any{map[string]any{"text": m.text}}}}
	}
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m.r.ID, "result": map[string]any{"task": t}})
	return b
}
