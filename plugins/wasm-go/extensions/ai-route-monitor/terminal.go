package main

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const terminalContextKey = "ai-route-monitor-terminal"
const inspectionLimit = 16 * 1024

// Only configured values can become dimensions. Sanitizing arbitrary model or
// header text alone would not bound cardinality.
type dimensionConfig struct {
	headerName string
	allowed    map[string]bool
	fallback   string
	normalize  bool
}

func (d dimensionConfig) value(raw string) string {
	if d.allowed[raw] {
		if d.normalize {
			return sanitizeMetricLabel(raw)
		}
		return raw
	}
	return d.fallback
}

func parseTerminalConfig(json gjson.Result, c *RouteMonitorConfig) error {
	c.legacyMetrics = true
	c.terminalMetrics = false
	for name, target := range map[string]*bool{"legacyMetrics": &c.legacyMetrics, "terminalMetrics": &c.terminalMetrics} {
		if value := json.Get(name); value.Exists() {
			if value.Type != gjson.True && value.Type != gjson.False {
				return fmt.Errorf("%s must be boolean", name)
			}
			*target = value.Bool()
		}
	}
	var err error
	c.source, err = parseDimension(json.Get("source"), 16, false)
	if err != nil {
		return fmt.Errorf("source: %w", err)
	}
	c.source.headerName = "x-wujie-source"
	if value := json.Get("source.headerName"); value.Exists() {
		if value.Type != gjson.String || !validHeaderName(value.String()) {
			return fmt.Errorf("source.headerName must be an HTTP header token")
		}
		c.source.headerName = strings.ToLower(value.String())
	}
	c.models, err = parseDimension(json.Get("models"), 128, true)
	if err != nil {
		return fmt.Errorf("models: %w", err)
	}
	c.asyncSubmissionPaths = make(map[string]bool)
	if paths := json.Get("asyncSubmissionPaths"); paths.Exists() {
		if !paths.IsArray() || len(paths.Array()) > 32 {
			return fmt.Errorf("asyncSubmissionPaths must be an array of at most 32 exact paths")
		}
		for _, path := range paths.Array() {
			if path.Type != gjson.String || !strings.HasPrefix(path.String(), "/") || strings.ContainsAny(path.String(), "?#") {
				return fmt.Errorf("asyncSubmissionPaths entries must be absolute paths without query or fragment")
			}
			c.asyncSubmissionPaths[path.String()] = true
		}
	}
	return nil
}

func parseDimension(value gjson.Result, limit int, normalize bool) (dimensionConfig, error) {
	d := dimensionConfig{allowed: make(map[string]bool), fallback: "unknown", normalize: normalize}
	if value.Exists() && !value.IsObject() {
		return d, fmt.Errorf("must be an object")
	}
	if fallback := value.Get("fallbackValue"); fallback.Exists() {
		if fallback.Type != gjson.String || !validDimension(fallback.String()) {
			return d, fmt.Errorf("fallbackValue must match [A-Za-z0-9_-]{1,64}")
		}
		d.fallback = fallback.String()
	}
	if allowed := value.Get("allowedValues"); allowed.Exists() {
		if !allowed.IsArray() || len(allowed.Array()) > limit {
			return d, fmt.Errorf("allowedValues must be an array of at most %d values", limit)
		}
		for _, item := range allowed.Array() {
			valid := validDimension(item.String())
			if normalize {
				valid = len(item.String()) > 0 && len(item.String()) <= 64
			}
			if item.Type != gjson.String || !valid {
				return d, fmt.Errorf("allowedValues entries must be 1-64 bytes; source values must match [A-Za-z0-9_-]+")
			}
			d.allowed[item.String()] = true
		}
	}
	return d, nil
}

func validDimension(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, ch := range s {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}

func validHeaderName(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", ch)) {
			return false
		}
	}
	return true
}

type requestTerminal struct {
	source      string
	submission  bool
	counted     bool
	status      int
	stream      bool
	jsonBody    bool
	inspectable bool
	ended       bool
	streamEnd   bool
	streamError bool
	uncertain   bool
	line        []byte
	data        []byte
	event       string
	dropping    bool
}

func terminalState(ctx wrapper.HttpContext) *requestTerminal {
	state, _ := ctx.GetContext(terminalContextKey).(*requestTerminal)
	return state
}

func startTerminalRequest(ctx wrapper.HttpContext, c RouteMonitorConfig) {
	if !c.terminalMetrics {
		return
	}
	source, _ := proxywasm.GetHttpRequestHeader(c.source.headerName)
	path, _ := proxywasm.GetHttpRequestHeader(":path")
	path, _, _ = strings.Cut(path, "?")
	ctx.SetContext(terminalContextKey, &requestTerminal{source: c.source.value(source), submission: c.asyncSubmissionPaths[path]})
}

func observeResponseHeaders(ctx wrapper.HttpContext) {
	s := terminalState(ctx)
	if s == nil {
		return
	}
	status, _ := proxywasm.GetHttpResponseHeader(":status")
	s.status, _ = strconv.Atoi(status)
	ct, _ := proxywasm.GetHttpResponseHeader("content-type")
	ct = strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	s.stream = ct == "text/event-stream"
	s.jsonBody = ct == "application/json" || strings.HasSuffix(ct, "+json")
	s.inspectable = !ctx.IsBinaryResponseBody()
	s.ended = !ctx.HasResponseBody()
}

func onStreamingResponseBody(ctx wrapper.HttpContext, c RouteMonitorConfig, body []byte, end bool, logger log.Log) []byte {
	if state := terminalState(ctx); state != nil {
		state.observeBody(body, end)
	}
	return body
}

func (s *requestTerminal) observeBody(body []byte, end bool) {
	s.ended = end
	if s.stream {
		for _, ch := range body {
			if ch == '\n' {
				s.consumeLine(strings.TrimSuffix(string(s.line), "\r"))
				s.line = s.line[:0]
			} else if len(s.line) < inspectionLimit && !s.dropping {
				s.line = append(s.line, ch)
			} else {
				s.uncertain = true
				s.dropping = true
			}
		}
		if end && (len(s.line) > 0 || len(s.data) > 0 || s.event != "") {
			s.uncertain = true
		}
		return
	}
	if !s.jsonBody {
		return
	}
	if len(s.data)+len(body) <= inspectionLimit {
		s.data = append(s.data, body...)
	} else {
		s.uncertain = true
	}
	if end && !s.uncertain {
		if !gjson.ValidBytes(s.data) {
			s.uncertain = true
			return
		}
		s.streamError = errorPayload(gjson.ParseBytes(s.data))
	}
}

func (s *requestTerminal) consumeLine(line string) {
	if line == "" {
		if !s.dropping {
			s.consumeEvent()
		}
		s.data = s.data[:0]
		s.event = ""
		s.dropping = false
		return
	}
	if s.dropping || strings.HasPrefix(line, ":") {
		return
	}
	field, value, _ := strings.Cut(line, ":")
	value = strings.TrimPrefix(value, " ")
	switch field {
	case "event":
		// Only these event names affect classification; retain no arbitrary text.
		s.event = ""
		if value == "error" || value == "response.failed" || value == "response.incomplete" {
			s.event = value
		}
	case "data":
		if len(s.data)+len(value)+1 > inspectionLimit {
			s.dropping = true
			s.uncertain = true
			return
		}
		if len(s.data) > 0 {
			s.data = append(s.data, '\n')
		}
		s.data = append(s.data, value...)
	}
}

func errorPayload(json gjson.Result) bool {
	if value := json.Get("error"); value.Exists() && value.Type != gjson.Null {
		return true
	}
	switch json.Get("type").String() {
	case "error", "response.failed", "response.incomplete":
		return true
	}
	if json.Get("choices").IsArray() {
		for _, choice := range json.Get("choices").Array() {
			if choice.Get("finish_reason").String() == "content_filter" {
				return true
			}
		}
	}
	return false
}

func (s *requestTerminal) consumeEvent() {
	if s.event == "error" || s.event == "response.failed" || s.event == "response.incomplete" {
		s.streamError = true
	}
	data := strings.TrimSpace(string(s.data))
	if data == "" {
		return
	}
	if data == "[DONE]" {
		s.streamEnd = true
		return
	}
	if !gjson.Valid(data) {
		s.uncertain = true
		return
	}
	json := gjson.Parse(data)
	if errorPayload(json) {
		s.streamError = true
	}
	switch json.Get("type").String() {
	case "message_stop":
		s.streamEnd = true
	case "response.completed":
		if json.Get("response.status").String() == "completed" {
			s.streamEnd = true
		} else {
			s.uncertain = true
		}
	}
}

// Envoy serializes response.flags as a little-endian uint64. Only explicit
// timeout/reset flags are classified; absent observations remain unknown.
func (s *requestTerminal) outcome(flags uint64) string {
	// Stable Envoy response flag bit positions (stream_info.h, through DR).
	const timeoutFlags = uint64(1<<2 | 1<<16 | 1<<19 | 1<<22)
	const resetFlags = uint64(1<<3 | 1<<4 | 1<<5 | 1<<6 | 1<<14 | 1<<18 | 1<<23 | 1<<28)
	if flags&timeoutFlags != 0 || s.status == 408 || s.status == 504 {
		return "timeout"
	}
	if flags&resetFlags != 0 {
		return "interrupted"
	}
	if s.status >= 400 {
		return "http_error"
	}
	if s.streamError {
		return "stream_error"
	}
	// Delay injection and a response served from cache are informational flags.
	if flags & ^uint64(1<<9|1<<20) != 0 || s.status < 200 || s.status >= 300 || !s.ended {
		return "unknown"
	}
	if s.submission {
		return "success"
	}
	if !s.inspectable || s.uncertain || s.stream && !s.streamEnd {
		return "unknown"
	}
	return "success"
}

func terminalMetricName(submission bool, source, model, outcome string) string {
	metric := "wujie_model_requests_total"
	if submission {
		metric = "wujie_model_submissions_total"
	}
	return fmt.Sprintf("%s.wujie_source=%s.wujie_model=%s.wujie_outcome=%s", metric, source, model, outcome)
}

func onStreamDone(ctx wrapper.HttpContext, c RouteMonitorConfig, logger log.Log) {
	s := terminalState(ctx)
	if s == nil || s.counted {
		return
	}
	s.counted = true
	// Local replies can skip our encode-headers callback on some filter paths.
	if raw, err := proxywasm.GetProperty([]string{"response", "code"}); err == nil && len(raw) == 8 {
		s.status = int(binary.LittleEndian.Uint64(raw))
	}
	var flags uint64
	if raw, err := proxywasm.GetProperty([]string{"response", "flags"}); err == nil && len(raw) == 8 {
		flags = binary.LittleEndian.Uint64(raw)
	}
	model := getModelFromRequest() // ai-proxy can set the property after our decode headers.
	c.incrementCounter(terminalMetricName(s.submission, s.source, c.models.value(model), s.outcome(flags)), 1)
}
