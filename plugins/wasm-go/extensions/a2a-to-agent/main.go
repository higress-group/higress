// Copyright 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"mime"
	"net/url"
	"strconv"
	"strings"
)

func main() {}
func init() {
	wrapper.SetCtx("a2a-to-agent", wrapper.ParseConfig(parseConfig), wrapper.ProcessRequestHeaders(onRequestHeaders), wrapper.ProcessRequestBody(onRequestBody), wrapper.ProcessResponseHeaders(onResponseHeaders), wrapper.ProcessStreamingResponseBody(onStream), wrapper.ProcessResponseBody(onResponseBody))
}
func parseConfig(v gjson.Result, c *config) error {
	if err := json.Unmarshal([]byte(v.Raw), c); err != nil {
		return err
	}
	return c.validate()
}
func local(ctx wrapper.HttpContext, status uint32, body []byte) {
	ctx.SetContext("a2a-local", true)
	_ = proxywasm.SendHttpResponse(status, [][2]string{{"content-type", "application/json"}, {"cache-control", "no-store"}}, body, -1)
}
func onRequestHeaders(ctx wrapper.HttpContext, c config) types.Action {
	ctx.DisableReroute()
	if strings.SplitN(ctx.Path(), "?", 2)[0] == "/.well-known/agent-card.json" && ctx.Method() == "GET" {
		b, _ := json.Marshal(c.card())
		local(ctx, 200, b)
		return types.HeaderStopIteration
	}
	if ctx.Method() != "POST" {
		local(ctx, 405, rpcError(nil, -32600, "POST required"))
		return types.HeaderStopIteration
	}
	contentType, _ := proxywasm.GetHttpRequestHeader("content-type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType != "application/json" {
		local(ctx, 415, rpcError(nil, -32600, "application/json required"))
		return types.HeaderStopIteration
	}
	consumer, _ := proxywasm.GetHttpRequestHeader(c.ConsumerHeader)
	ctx.SetContext("a2a-consumer", consumer)
	if consumer == "" {
		local(ctx, 401, rpcError(nil, -32602, "authenticated consumer identity required"))
		return types.HeaderStopIteration
	}
	if !ctx.HasRequestBody() {
		local(ctx, 400, rpcError(nil, -32600, "request body required"))
		return types.HeaderStopIteration
	}
	ctx.SetRequestBodyBufferLimit(uint32(c.MaxBodyBytes))
	ctx.BufferRequestBody()
	return types.HeaderStopIteration
}
func mapper(ctx wrapper.HttpContext) *streamMapper {
	m, _ := ctx.GetContext("a2a-mapper").(*streamMapper)
	return m
}
func onRequestBody(ctx wrapper.HttpContext, c config, body []byte) types.Action {
	if len(body) > c.MaxBodyBytes {
		local(ctx, 413, rpcError(nil, -32600, "request exceeds limit"))
		return types.DataStopIterationNoBuffer
	}
	r, code, err := parseRequest(body, c, ctx.GetStringContext("a2a-consumer", ""))
	if err != nil {
		id := json.RawMessage(gjson.GetBytes(body, "id").Raw)
		local(ctx, 400, rpcError(id, code, err.Error()))
		return types.DataStopIterationNoBuffer
	}
	b := make([]byte, 16)
	if _, err = rand.Read(b); err != nil {
		local(ctx, 500, rpcError(r.ID, -32603, "cannot generate task identifier"))
		return types.DataStopIterationNoBuffer
	}
	r.TaskID = hex.EncodeToString(b)
	m := newMapper(c, r)
	ctx.SetContext("a2a-mapper", m)
	if c.cloud() {
		if r.Session != "" {
			setCloudStream(c, r)
			return types.DataContinue
		}
		create := map[string]any{}
		for k, v := range c.SessionConfig {
			create[k] = v
		}
		if c.AgentID != "" {
			create["agent"] = c.AgentID
		}
		payload, _ := json.Marshal(create)
		err := dispatch(c, c.APIBasePath+"/sessions", payload, func(status int, b []byte) {
			if status < 200 || status >= 300 {
				local(ctx, 502, rpcError(r.ID, -32603, "agent session creation failed"))
				return
			}
			id := gjson.GetBytes(b, "id").String()
			if id == "" {
				local(ctx, 502, rpcError(r.ID, -32603, "agent session response missing id"))
				return
			}
			m.context(id)
			setCloudStream(c, r)
			_ = proxywasm.ResumeHttpRequest()
		})
		if err != nil {
			local(ctx, 502, rpcError(r.ID, -32603, "agent session dispatch failed"))
		}
		return types.DataStopIterationAndBuffer
	}
	path, payload := nativeRequest(c, r)
	setRequest(c, "POST", path, payload, true)
	return types.DataContinue
}
func authHeaders(c config) [][2]string {
	if c.Provider == "claude-managed" {
		return [][2]string{{"x-api-key", c.APIKey}, {"anthropic-version", "2023-06-01"}, {"anthropic-beta", "managed-agents-2026-04-01"}}
	}
	return [][2]string{{"authorization", "Bearer " + c.APIKey}}
}
func setRequest(c config, method, path string, body []byte, stream bool) {
	_ = proxywasm.ReplaceHttpRequestHeader(":method", method)
	_ = proxywasm.ReplaceHttpRequestHeader(":path", path)
	if c.UpstreamAuthority != "" {
		_ = proxywasm.ReplaceHttpRequestHeader(":authority", c.UpstreamAuthority)
	}
	for _, h := range []string{"authorization", "x-api-key", "cookie", "accept-encoding", "content-length", "last-event-id", c.ConsumerHeader} {
		_ = proxywasm.RemoveHttpRequestHeader(h)
	}
	_ = proxywasm.ReplaceHttpRequestHeader("content-type", "application/json")
	_ = proxywasm.ReplaceHttpRequestHeader("accept", "application/json")
	if stream {
		_ = proxywasm.ReplaceHttpRequestHeader("accept", "text/event-stream")
	}
	for _, h := range authHeaders(c) {
		_ = proxywasm.ReplaceHttpRequestHeader(h[0], h[1])
	}
	if c.Provider == "bailian" && stream {
		_ = proxywasm.ReplaceHttpRequestHeader("x-dashscope-sse", "enable")
	}
	_ = proxywasm.ReplaceHttpRequestBody(body)
}
func setCloudStream(c config, r *request) {
	eventType := "agent.message"
	if c.Provider == "bailian-managed" {
		eventType = "message"
	}
	path := c.APIBasePath + "/sessions/" + url.PathEscape(r.Session) + "/events/stream?event_deltas%5B%5D=" + eventType
	if c.Provider == "claude-managed" {
		path += "&beta=true"
	}
	setRequest(c, "GET", path, nil, true)
	// A bodyless SSE subscription must have explicit HTTP/1 request framing.
	// The original A2A POST body has been removed; leaving its length unknown
	// can make an upstream wait for a body before returning SSE headers.
	_ = proxywasm.RemoveHttpRequestHeader("transfer-encoding")
	_ = proxywasm.ReplaceHttpRequestHeader("content-length", "0")
}
func dispatch(c config, path string, body []byte, cb func(int, []byte)) error {
	headers := [][2]string{{":method", "POST"}, {":path", path}, {":authority", c.UpstreamAuthority}, {":scheme", c.UpstreamScheme}, {"content-type", "application/json"}}
	headers = append(headers, authHeaders(c)...)
	_, err := proxywasm.DispatchHttpCall(c.UpstreamCluster, headers, body, nil, 10000, func(_ int, size int, _ int) {
		h, e := proxywasm.GetHttpCallResponseHeaders()
		status := 0
		for _, p := range h {
			if p[0] == ":status" {
				status, _ = strconv.Atoi(p[1])
			}
		}
		if e != nil || size > c.MaxBodyBytes {
			cb(502, nil)
			return
		}
		b, e := proxywasm.GetHttpCallResponseBody(0, size)
		if e != nil {
			cb(502, nil)
			return
		}
		cb(status, b)
	})
	return err
}
func onResponseHeaders(ctx wrapper.HttpContext, c config) types.Action {
	m := mapper(ctx)
	if m == nil || ctx.GetBoolContext("a2a-local", false) {
		ctx.DontReadResponseBody()
		return types.HeaderContinue
	}
	status, _ := proxywasm.GetHttpResponseHeader(":status")
	ctx.SetContext("a2a-upstream-ok", status == "200")
	_ = proxywasm.RemoveHttpResponseHeader("content-length")
	_ = proxywasm.RemoveHttpResponseHeader("content-encoding")
	_ = proxywasm.RemoveHttpResponseHeader("set-cookie")
	_ = proxywasm.ReplaceHttpResponseHeader("cache-control", "no-store")

	if status != "200" {
		local(ctx, 502, rpcError(m.r.ID, -32603, "upstream agent returned HTTP "+status))
		return types.HeaderStopIteration
	}
	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType != "text/event-stream" {
		local(ctx, 502, rpcError(m.r.ID, -32603, "upstream did not return SSE"))
		return types.HeaderStopIteration
	}
	if !m.r.Streaming {
		ctx.SetResponseBodyBufferLimit(uint32(c.MaxBodyBytes))
		ctx.BufferResponseBody()
		_ = proxywasm.ReplaceHttpResponseHeader("content-type", "application/json")
		return types.HeaderStopIteration
	}
	_ = proxywasm.ReplaceHttpResponseHeader("content-type", "text/event-stream")
	_ = proxywasm.ReplaceHttpResponseHeader("x-accel-buffering", "no")
	if c.cloud() {
		path := c.APIBasePath + "/sessions/" + url.PathEscape(m.r.Session) + "/events"
		if c.Provider == "claude-managed" {
			path += "?beta=true"
		}
		payload := cloudMessage(c, m.r.Text)
		// Subscribe first: cloud APIs can tail live events and lose output if
		// a user message is submitted before the event stream is established.
		err := dispatch(c, path, payload, func(status int, _ []byte) {
			if status < 200 || status >= 300 {
				local(ctx, 502, rpcError(m.r.ID, -32603, "agent message submission failed"))
				return
			}
			_ = proxywasm.ResumeHttpResponse()
		})
		if err != nil {
			local(ctx, 502, rpcError(m.r.ID, -32603, "agent message dispatch failed"))
		}
		return types.HeaderStopAllIterationAndWatermark
	}
	return types.HeaderContinue
}
func onStream(ctx wrapper.HttpContext, c config, chunk []byte, last bool) []byte {
	m := mapper(ctx)
	if m == nil || ctx.GetBoolContext("a2a-local", false) {
		return chunk
	}
	if m.done {
		return nil
	}
	out := m.feed(chunk, last)
	if m.done && !last {
		if err := proxywasm.InjectEncodedDataToFilterChain(out, true); err == nil {
			ctx.NeedPauseStreamingResponse()
			return nil
		} else {
			proxywasm.LogErrorf("a2a-to-agent: unable to end response stream: %v", err)
		}
	}
	return out
}
func onResponseBody(ctx wrapper.HttpContext, c config, body []byte) types.Action {
	m := mapper(ctx)
	if m == nil || ctx.GetBoolContext("a2a-local", false) {
		return types.DataContinue
	}
	if len(body) > c.MaxBodyBytes {
		_ = proxywasm.ReplaceHttpResponseBody(rpcError(m.r.ID, -32603, "upstream response exceeds limit"))
		return types.DataContinue
	}
	_ = proxywasm.ReplaceHttpResponseHeader(":status", "200")
	_ = proxywasm.ReplaceHttpResponseBody(m.blocking(body, ctx.GetBoolContext("a2a-upstream-ok", false)))
	return types.DataContinue
}
