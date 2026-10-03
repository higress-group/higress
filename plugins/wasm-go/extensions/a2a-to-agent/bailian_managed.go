// Copyright 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
)

func cloudMessage(c config, text string) []byte {
	message := map[string]any{"type": "user.message", "content": []any{map[string]any{"type": "text", "text": text}}}
	key := "events"
	if c.Provider == "bailian-managed" {
		key = "input"
		message["type"] = "message"
		message["role"] = "user"
	}
	b, _ := json.Marshal(map[string]any{key: []any{message}})
	return b
}

func managedThread(v gjson.Result) string {
	if id := v.Get("thread_id").String(); id != "" {
		return id
	}
	return v.Get("metadata.thread_id").String()
}

func (m *streamMapper) bailianManagedEvent(v gjson.Result) []byte {
	for _, path := range []string{"session_id", "event.session_id"} {
		if id := v.Get(path).String(); id != "" && id != m.r.Session {
			return m.discardManagedPreview(v)
		}
	}
	typ := v.Get("type").String()
	thread := managedThread(v)
	if typ == "event_start" && thread == "" {
		thread = managedThread(v.Get("event"))
	}
	// Fresh sessions have one submitted user message. Its echo identifies the
	// root thread; status and preview frames legitimately omit thread and role.
	if typ == "message" && v.Get("role").String() == "user" {
		if m.managedRootThread == "" {
			m.managedRootThread = thread
		}
		return nil
	}
	if thread != "" && thread != m.managedRootThread {
		return m.discardManagedPreview(v)
	}
	switch typ {
	case "session_status":
		for _, block := range v.Get("content").Array() {
			if block.Get("type").String() != "data" {
				continue
			}
			data := block.Get("data")
			switch data.Get("session_status").String() {
			case "running":
				m.running = true
				return m.progress(typ, v)
			case "idle":
				// Ignore the initial idle state from before our submitted turn.
				if !m.running {
					return nil
				}
				switch data.Get("stop_reason.type").String() {
				case "end_turn":
					return m.finish("TASK_STATE_COMPLETED", "")
				case "requires_action":
					return m.finish("TASK_STATE_INPUT_REQUIRED", "Agent requires external action; tool-result continuation is unsupported")
				default:
					return m.finish("TASK_STATE_FAILED", "Bailian Managed agent stopped without successful completion")
				}
			case "terminated", "failed":
				return m.finish("TASK_STATE_FAILED", "Bailian Managed agent session failed")
			}
		}
		return m.finish("TASK_STATE_FAILED", "invalid Bailian Managed session status")
	case "event_start":
		e := v.Get("event")
		if !m.running || e.Get("type").String() != "message" || (e.Get("role").String() != "" && e.Get("role").String() != "assistant") {
			return nil
		}
		id := e.Get("id").String()
		if id == "" {
			return m.finish("TASK_STATE_FAILED", "agent message is missing its event id")
		}
		if m.managedPreviews == nil {
			m.managedPreviews = map[string]map[int]string{}
		}
		if m.managedPreviews[id] == nil && !m.managedCompleted[id] {
			m.managedPreviews[id] = map[int]string{}
		}
	case "event_delta":
		id := v.Get("event_id").String()
		blocks := m.managedPreviews[id]
		if !m.running || blocks == nil || m.managedCompleted[id] || v.Get("delta.type").String() != "content_delta" || v.Get("delta.content.type").String() != "text" {
			return nil
		}
		index := v.Get("delta.index")
		if index.Type != gjson.Number || index.Int() < 0 || index.Float() != float64(index.Int()) || index.Int() > int64(m.c.MaxBodyBytes) {
			return m.finish("TASK_STATE_FAILED", "invalid agent message content index")
		}
		if v.Get("delta.content.text").Type != gjson.String {
			return m.finish("TASK_STATE_FAILED", "invalid agent message text")
		}
		blocks[int(index.Int())] += v.Get("delta.content.text").String()
		indices := make([]int, 0, len(blocks))
		for i := range blocks {
			indices = append(indices, i)
		}
		sort.Ints(indices)
		var text strings.Builder
		for _, i := range indices {
			text.WriteString(blocks[i])
		}
		return m.messageTextUpdate(id, text.String(), false)
	case "message":
		if !m.running || v.Get("role").String() != "assistant" {
			return nil
		}
		id := v.Get("id").String()
		if !v.Get("content").IsArray() {
			return m.finish("TASK_STATE_FAILED", "invalid agent message content")
		}
		for _, block := range v.Get("content").Array() {
			if block.Get("type").String() == "text" && block.Get("text").Type != gjson.String {
				return m.finish("TASK_STATE_FAILED", "invalid agent message text")
			}
		}
		if m.managedCompleted == nil {
			m.managedCompleted = map[string]bool{}
		}
		m.managedCompleted[id] = true
		delete(m.managedPreviews, id)
		return m.messageTextUpdate(id, cloudText(v), false)
	case "tool_call", "tool_call_output", "tool_approval_request", "tool_approval_response", "function_call", "function_call_output", "mcp_call", "mcp_call_output", "model_request_start", "model_request_end", "reasoning", "error":
		if m.running {
			return m.progress(typ, v)
		}
	}
	return nil
}

func (m *streamMapper) discardManagedPreview(v gjson.Result) []byte {
	id := v.Get("id").String()
	if v.Get("type").String() == "event_start" {
		id = v.Get("event.id").String()
	}
	if id == "" || m.managedPreviews[id] == nil {
		return nil
	}
	// Previews can omit thread identity. If the authoritative event identifies
	// a child, retract its provisional text rather than keeping it in the answer.
	delete(m.managedPreviews, id)
	return m.messageTextUpdate(id, "", false)
}
