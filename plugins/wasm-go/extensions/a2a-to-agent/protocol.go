// Copyright 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type config struct {
	Provider          string         `json:"provider"`
	APIKey            string         `json:"apiKey"`
	AgentID           string         `json:"agentId"`
	APIPath           string         `json:"apiPath"`
	APIBasePath       string         `json:"apiBasePath"`
	ConsumerHeader    string         `json:"consumerHeader"`
	ContextSecret     string         `json:"contextSecret"`
	UpstreamCluster   string         `json:"upstreamCluster"`
	UpstreamAuthority string         `json:"upstreamAuthority"`
	UpstreamScheme    string         `json:"upstreamScheme"`
	SessionConfig     map[string]any `json:"sessionConfig"`
	Inputs            map[string]any `json:"inputs"`
	AgentCard         map[string]any `json:"agentCard"`
	MaxBodyBytes      int            `json:"maxBodyBytes"`
	ContextTTLSeconds int64          `json:"contextTTLSeconds"`
}

func (c *config) validate() error {
	switch c.Provider {
	case "dify":
		if c.APIPath == "" {
			c.APIPath = "/v1/chat-messages"
		}
	case "bailian":
		if c.AgentID == "" {
			return errors.New("agentId required")
		}
		if c.APIPath == "" {
			c.APIPath = "/api/v1/apps/" + url.PathEscape(c.AgentID) + "/completion"
		}
	case "coze":
		if c.AgentID == "" {
			return errors.New("agentId required")
		}
		if c.APIPath == "" {
			c.APIPath = "/v3/chat"
		}
	case "qoder", "claude-managed", "bailian-managed":
		if c.AgentID == "" || c.SessionConfig["environment_id"] == nil {
			return errors.New("cloud providers require agentId and sessionConfig.environment_id")
		}
		if c.UpstreamCluster == "" || c.UpstreamAuthority == "" {
			return errors.New("cloud providers require upstreamCluster and upstreamAuthority")
		}
		if c.APIBasePath == "" {
			if c.Provider == "qoder" {
				c.APIBasePath = "/api/v1/cloud"
			} else if c.Provider == "bailian-managed" {
				c.APIBasePath = "/api/v1/agentstudio"
			} else {
				c.APIBasePath = "/v1"
			}
		}
	default:
		return errors.New("unsupported provider")
	}
	if c.APIKey == "" || len(c.ContextSecret) < 32 || c.ConsumerHeader == "" {
		return errors.New("apiKey, contextSecret (at least 32 bytes), and consumerHeader required")
	}
	if c.UpstreamScheme == "" {
		c.UpstreamScheme = "https"
	}
	if c.UpstreamScheme != "http" && c.UpstreamScheme != "https" {
		return errors.New("invalid upstreamScheme")
	}
	if c.MaxBodyBytes == 0 {
		c.MaxBodyBytes = 1024 * 1024
	}
	if c.MaxBodyBytes < 1024 || c.MaxBodyBytes > 16*1024*1024 {
		return errors.New("maxBodyBytes must be 1024..16777216")
	}
	if c.ContextTTLSeconds == 0 {
		c.ContextTTLSeconds = 86400
	}
	if c.ContextTTLSeconds < 1 {
		return errors.New("invalid contextTTLSeconds")
	}
	if c.AgentCard == nil || c.AgentCard["name"] == nil || c.AgentCard["url"] == nil {
		return errors.New("agentCard.name and agentCard.url required")
	}
	if c.Inputs == nil {
		c.Inputs = map[string]any{}
	}
	return nil
}
func (c config) cloud() bool {
	return c.Provider == "qoder" || c.Provider == "claude-managed" || c.Provider == "bailian-managed"
}
func (c config) card() map[string]any {
	out := map[string]any{}
	for k, v := range c.AgentCard {
		if k != "url" {
			out[k] = v
		}
	}
	out["supportedInterfaces"] = []any{map[string]any{"url": c.AgentCard["url"], "protocolBinding": "JSONRPC", "protocolVersion": "1.0"}}
	if out["description"] == nil {
		out["description"] = "Agent API exposed through Higress"
	}
	if out["skills"] == nil {
		out["skills"] = []any{map[string]any{"id": "chat", "name": "Chat", "description": "Send text to the configured agent", "tags": []string{"agent"}}}
	}
	out["capabilities"] = map[string]any{"streaming": true}
	out["defaultInputModes"] = []string{"text/plain"}
	out["defaultOutputModes"] = []string{"text/plain"}
	if out["version"] == nil {
		out["version"] = "0.1.0"
	}
	return out
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  struct {
		Message struct {
			MessageID string                       `json:"messageId"`
			Role      string                       `json:"role"`
			ContextID string                       `json:"contextId"`
			TaskID    string                       `json:"taskId"`
			Parts     []map[string]json.RawMessage `json:"parts"`
		} `json:"message"`
		Configuration map[string]json.RawMessage `json:"configuration"`
	} `json:"params"`
}
type request struct {
	ID                                         json.RawMessage
	Text, Consumer, Session, TaskID, ContextID string
	Streaming                                  bool
}

func parseRequest(body []byte, c config, consumer string) (*request, int, error) {
	var rpc rpcRequest
	if !json.Valid(body) {
		return nil, -32700, errors.New("invalid JSON")
	}
	if err := json.Unmarshal(body, &rpc); err != nil || rpc.JSONRPC != "2.0" || len(rpc.ID) == 0 || string(rpc.ID) == "null" {
		return nil, -32600, errors.New("JSON-RPC 2.0 request with non-null id required")
	}
	var id any
	_ = json.Unmarshal(rpc.ID, &id)
	switch id.(type) {
	case string, float64:
	default:
		return nil, -32600, errors.New("id must be string or number")
	}
	if rpc.Method != "SendMessage" && rpc.Method != "SendStreamingMessage" {
		return nil, -32601, errors.New("method not supported; only SendMessage and SendStreamingMessage are available")
	}
	if c.cloud() && rpc.Method == "SendMessage" {
		return nil, -32004, errors.New("this provider requires SendStreamingMessage")
	}
	if consumer == "" || len(consumer) > 256 {
		return nil, -32602, errors.New("authenticated consumer identity required")
	}
	m := rpc.Params.Message
	if m.Role != "ROLE_USER" || m.MessageID == "" || len(m.Parts) == 0 || m.TaskID != "" {
		return nil, -32602, errors.New("messageId, ROLE_USER and text parts required; task continuation is unsupported")
	}
	for k, v := range rpc.Params.Configuration {
		if k == "returnImmediately" {
			if string(v) != "false" {
				return nil, -32602, errors.New("nonblocking tasks are unsupported")
			}
		} else {
			return nil, -32602, fmt.Errorf("configuration.%s is unsupported", k)
		}
	}
	texts := []string{}
	for _, p := range m.Parts {
		if len(p) != 1 || p["text"] == nil {
			return nil, -32602, errors.New("only text parts are supported")
		}
		var s string
		if string(p["text"]) == "null" || json.Unmarshal(p["text"], &s) != nil {
			return nil, -32602, errors.New("part.text must be string")
		}
		texts = append(texts, s)
	}
	text := strings.Join(texts, "\n")
	if strings.TrimSpace(text) == "" {
		return nil, -32602, errors.New("empty message")
	}
	r := &request{ID: rpc.ID, Text: text, Consumer: consumer, Streaming: rpc.Method == "SendStreamingMessage"}
	if m.ContextID != "" {
		if c.cloud() {
			return nil, -32602, errors.New("cloud context continuation is unsupported; start a new session")
		}
		s, err := c.verifyContext(m.ContextID, consumer)
		if err != nil {
			return nil, -32602, err
		}
		r.Session = s
		r.ContextID = m.ContextID
	}
	return r, 0, nil
}

type contextToken struct {
	Provider string `json:"p"`
	Agent    string `json:"a"`
	Consumer string `json:"u"`
	Session  string `json:"s"`
	Expiry   int64  `json:"e"`
}

func (c config) contextScope() string {
	b, _ := json.Marshal([]string{c.AgentID, c.APIPath, c.APIBasePath, c.UpstreamAuthority, c.APIKey})
	sum := sha256.Sum256(b)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
func (c config) signContext(session, consumer string) string {
	b, _ := json.Marshal(contextToken{c.Provider, c.contextScope(), consumer, session, time.Now().Unix() + c.ContextTTLSeconds})
	p := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, []byte(c.ContextSecret))
	mac.Write([]byte(p))
	return p + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (c config) verifyContext(token, consumer string) (string, error) {
	fail := errors.New("invalid or expired contextId")
	p := strings.Split(token, ".")
	if len(p) != 2 || len(token) > 4096 {
		return "", fail
	}
	sig, e := base64.RawURLEncoding.DecodeString(p[1])
	if e != nil {
		return "", fail
	}
	mac := hmac.New(sha256.New, []byte(c.ContextSecret))
	mac.Write([]byte(p[0]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return "", fail
	}
	b, e := base64.RawURLEncoding.DecodeString(p[0])
	if e != nil {
		return "", fail
	}
	var t contextToken
	if json.Unmarshal(b, &t) != nil || t.Provider != c.Provider || t.Agent != c.contextScope() || t.Consumer != consumer || t.Session == "" || t.Expiry < time.Now().Unix() {
		return "", fail
	}
	return t.Session, nil
}
func nativeRequest(c config, r *request) (string, []byte) {
	var body map[string]any
	path := c.APIPath
	switch c.Provider {
	case "dify":
		mode := "streaming"
		body = map[string]any{"inputs": c.Inputs, "query": r.Text, "user": r.Consumer, "response_mode": mode}
		if r.Session != "" {
			body["conversation_id"] = r.Session
		}
	case "bailian":
		in := map[string]any{"prompt": r.Text}
		if r.Session != "" {
			in["session_id"] = r.Session
		}
		body = map[string]any{"input": in, "parameters": map[string]any{"incremental_output": true, "has_thoughts": true}}
	case "coze":
		body = map[string]any{"bot_id": c.AgentID, "user_id": r.Consumer, "stream": true, "auto_save_history": true, "additional_messages": []any{map[string]any{"role": "user", "content": r.Text, "content_type": "text"}}}
		if r.Session != "" {
			path += "?conversation_id=" + url.QueryEscape(r.Session)
		}
	}
	b, _ := json.Marshal(body)
	return path, b
}
func rpcError(id json.RawMessage, code int, msg string) []byte {
	var value any
	if json.Unmarshal(id, &value) != nil {
		id = nil
	}
	switch value.(type) {
	case string, float64:
	default:
		id = nil
	}
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
	return b
}
