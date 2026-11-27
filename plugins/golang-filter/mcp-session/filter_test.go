package mcp_session

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/stretchr/testify/require"
)

// Mock implementation of CommonCAPI for testing
type mockCommonCAPI struct {
	logs []string
}

func (m *mockCommonCAPI) Log(level api.LogType, message string) {
	fmt.Printf("[%s] %s", level, message)
	m.logs = append(m.logs, message)
}

func (m *mockCommonCAPI) LogLevel() api.LogType {
	return api.Debug
}

// Test helper to create a filter instance for testing
func createTestFilter() *filter {
	return &filter{}
}

// Test helper to create a match rule for testing
func createTestMatchRule() common.MatchRule {
	return common.MatchRule{
		UpstreamType:      common.SSEUpstream,
		EnablePathRewrite: true,
		PathRewritePrefix: "/api/v1",
		MatchRulePath:     "/mcp",
		MatchRuleType:     common.PrefixMatch,
		MatchRuleDomain:   "example.com",
	}
}

// TestFindEndpointUrl_ValidEndpointMessage tests the current behavior with a valid endpoint message
func TestFindEndpointUrl_ValidEndpointMessage(t *testing.T) {
	// Setup mock API
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createTestFilter()

	// Test with valid endpoint message
	sseData := "event: endpoint\ndata: https://api.example.com/chat\n\n"

	err, endpointUrl := f.findEndpointUrl(sseData)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	expectedUrl := "https://api.example.com/chat"
	if endpointUrl != expectedUrl {
		t.Errorf("Expected endpoint URL '%s', got '%s'", expectedUrl, endpointUrl)
	}
}

// TestFindEndpointUrl_NonEndpointFirstMessage tests improved behavior with non-endpoint first message
func TestFindEndpointUrl_NonEndpointFirstMessage(t *testing.T) {
	// Setup mock API
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createTestFilter()

	// Test with ping message first (this should now succeed with improved implementation)
	sseData := "event: ping\ndata: alive\n\nevent: endpoint\ndata: https://api.example.com/chat\n\n"

	err, endpointUrl := f.findEndpointUrl(sseData)

	// Improved implementation should handle non-endpoint first message
	if err != nil {
		t.Errorf("Expected no error for non-endpoint first message, got: %v", err)
	}

	expectedUrl := "https://api.example.com/chat"
	if endpointUrl != expectedUrl {
		t.Errorf("Expected endpoint URL '%s', got '%s'", expectedUrl, endpointUrl)
	}

	// Check that the non-endpoint event was logged
	found := false
	for _, log := range mockAPI.logs {
		if log == "Skipping non-endpoint event: ping" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected log message about skipping ping event not found")
	}
}

// TestFindEndpointUrl_MultipleNonEndpointMessages tests multiple non-endpoint messages before endpoint
func TestFindEndpointUrl_MultipleNonEndpointMessages(t *testing.T) {
	// Setup mock API
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createTestFilter()

	// Test with multiple non-endpoint messages before endpoint
	sseData := "event: ping\ndata: alive\n\nevent: status\ndata: connecting\n\nevent: info\ndata: ready\n\nevent: endpoint\ndata: https://api.example.com/chat\n\n"

	err, endpointUrl := f.findEndpointUrl(sseData)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	expectedUrl := "https://api.example.com/chat"
	if endpointUrl != expectedUrl {
		t.Errorf("Expected endpoint URL '%s', got '%s'", expectedUrl, endpointUrl)
	}

	// Check that all non-endpoint events were logged
	expectedLogs := []string{
		"Skipping non-endpoint event: ping",
		"Skipping non-endpoint event: status",
		"Skipping non-endpoint event: info",
	}

	for _, expectedLog := range expectedLogs {
		found := false
		for _, log := range mockAPI.logs {
			if log == expectedLog {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected log message '%s' not found", expectedLog)
		}
	}
}

// TestFindEndpointUrl_EndpointInMiddle tests endpoint message in the middle of other messages
func TestFindEndpointUrl_EndpointInMiddle(t *testing.T) {
	// Setup mock API
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createTestFilter()

	// Test with endpoint message in the middle
	sseData := "event: ping\ndata: alive\n\nevent: endpoint\ndata: https://api.example.com/chat\n\nevent: status\ndata: ready\n\n"

	err, endpointUrl := f.findEndpointUrl(sseData)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	expectedUrl := "https://api.example.com/chat"
	if endpointUrl != expectedUrl {
		t.Errorf("Expected endpoint URL '%s', got '%s'", expectedUrl, endpointUrl)
	}

	// Check that the ping event was logged as skipped
	found := false
	for _, log := range mockAPI.logs {
		if log == "Skipping non-endpoint event: ping" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected log message about skipping ping event not found")
	}
}

// TestFindEndpointUrl_NoEndpointMessage tests when no endpoint message is present
func TestFindEndpointUrl_NoEndpointMessage(t *testing.T) {
	// Setup mock API
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createTestFilter()

	// Test with no endpoint message
	sseData := "event: ping\ndata: alive\n\nevent: status\ndata: connecting\n\nevent: info\ndata: ready\n\n"

	err, endpointUrl := f.findEndpointUrl(sseData)

	if err != nil {
		t.Errorf("Expected no error when no endpoint found, got: %v", err)
	}

	if endpointUrl != "" {
		t.Errorf("Expected empty endpoint URL when no endpoint found, got '%s'", endpointUrl)
	}

	// Check that all non-endpoint events were logged
	expectedLogs := []string{
		"Skipping non-endpoint event: ping",
		"Skipping non-endpoint event: status",
		"Skipping non-endpoint event: info",
	}

	for _, expectedLog := range expectedLogs {
		found := false
		for _, log := range mockAPI.logs {
			if log == expectedLog {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected log message '%s' not found", expectedLog)
		}
	}
}

// TestFindEndpointUrl_IncompleteEndpointMessage tests incomplete endpoint message
func TestFindEndpointUrl_IncompleteEndpointMessage(t *testing.T) {
	// Setup mock API
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createTestFilter()

	// Test with incomplete endpoint message (missing final line break)
	sseData := "event: ping\ndata: alive\n\nevent: endpoint\ndata: https://api.example.com/chat"

	err, endpointUrl := f.findEndpointUrl(sseData)

	if err != nil {
		t.Errorf("Expected no error for incomplete endpoint message, got: %v", err)
	}

	if endpointUrl != "" {
		t.Errorf("Expected empty endpoint URL for incomplete message, got '%s'", endpointUrl)
	}
}

// TestFindEndpointUrl_IncompleteNonEndpointMessage tests incomplete non-endpoint message
func TestFindEndpointUrl_IncompleteNonEndpointMessage(t *testing.T) {
	// Setup mock API
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createTestFilter()

	// Test with incomplete non-endpoint message
	sseData := "event: ping\ndata: alive"

	err, endpointUrl := f.findEndpointUrl(sseData)

	if err != nil {
		t.Errorf("Expected no error for incomplete non-endpoint message, got: %v", err)
	}

	if endpointUrl != "" {
		t.Errorf("Expected empty endpoint URL for incomplete message, got '%s'", endpointUrl)
	}
}

// TestFindEndpointUrl_MalformedEndpointData tests malformed endpoint data
func TestFindEndpointUrl_MalformedEndpointData(t *testing.T) {
	// Setup mock API
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createTestFilter()

	// Test with malformed endpoint data (missing data field)
	sseData := "event: ping\ndata: alive\n\nevent: endpoint\nnotdata: https://api.example.com/chat\n\n"

	err, endpointUrl := f.findEndpointUrl(sseData)

	// Should return error for malformed endpoint data
	if err == nil {
		t.Errorf("Expected error for malformed endpoint data, but got none")
	}

	if endpointUrl != "" {
		t.Errorf("Expected empty endpoint URL when error occurs, got '%s'", endpointUrl)
	}
}

// TestFindEndpointUrl_DifferentLineBreaks tests different line break formats with improved version
func TestFindEndpointUrl_DifferentLineBreaks(t *testing.T) {
	testCases := []struct {
		name     string
		sseData  string
		expected string
	}{
		{
			name:     "CRLF line breaks with ping first",
			sseData:  "event: ping\r\ndata: alive\r\n\r\nevent: endpoint\r\ndata: https://api.example.com/chat\r\n\r\n",
			expected: "https://api.example.com/chat",
		},
		{
			name:     "CR line breaks with status first",
			sseData:  "event: status\rdata: ready\r\revent: endpoint\rdata: https://api.example.com/chat\r\r",
			expected: "https://api.example.com/chat",
		},
		{
			name:     "LF line breaks with info first",
			sseData:  "event: info\ndata: starting\n\nevent: endpoint\ndata: https://api.example.com/chat\n\n",
			expected: "https://api.example.com/chat",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock API
			mockAPI := &mockCommonCAPI{}
			api.SetCommonCAPI(mockAPI)

			f := createTestFilter()

			err, endpointUrl := f.findEndpointUrl(tc.sseData)

			if err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if endpointUrl != tc.expected {
				t.Errorf("Expected endpoint URL '%s', got '%s'", tc.expected, endpointUrl)
			}
		})
	}
}

// TestFindEndpointUrl_WithWhitespace tests improved version with whitespace
func TestFindEndpointUrl_WithWhitespace(t *testing.T) {
	// Setup mock API
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createTestFilter()

	// Test with whitespace around event names and data
	sseData := "event:  ping  \ndata:  alive  \n\nevent:  endpoint  \ndata:  https://api.example.com/chat  \n\n"

	err, endpointUrl := f.findEndpointUrl(sseData)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	expectedUrl := "https://api.example.com/chat"
	if endpointUrl != expectedUrl {
		t.Errorf("Expected endpoint URL '%s', got '%s'", expectedUrl, endpointUrl)
	}
}

// TestFindEndpointUrl_NoEventFound tests behavior when no event is found
func TestFindEndpointUrl_NoEventFound(t *testing.T) {
	// Setup mock API
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createTestFilter()

	// Test with data that doesn't contain event
	sseData := "some random data without event"

	err, endpointUrl := f.findEndpointUrl(sseData)

	if err != nil {
		t.Errorf("Expected no error when no event found, got: %v", err)
	}

	if endpointUrl != "" {
		t.Errorf("Expected empty endpoint URL when no event found, got '%s'", endpointUrl)
	}
}

// TestFindEndpointUrl_MalformedData tests behavior with malformed SSE data
func TestFindEndpointUrl_MalformedData(t *testing.T) {
	// Setup mock API
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createTestFilter()

	// Test with malformed data (missing data field)
	sseData := "event: endpoint\nnotdata: https://api.example.com/chat\n\n"

	err, endpointUrl := f.findEndpointUrl(sseData)

	// Should return error for malformed data
	if err == nil {
		t.Errorf("Expected error for malformed data, but got none")
	}

	if endpointUrl != "" {
		t.Errorf("Expected empty endpoint URL when error occurs, got '%s'", endpointUrl)
	}
}

// TestFindNextLineBreak tests the line break detection functionality
func TestFindNextLineBreak(t *testing.T) {
	// Setup mock API
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createTestFilter()

	testCases := []struct {
		name          string
		input         string
		expectedBreak string
		expectedError bool
	}{
		{
			name:          "LF only",
			input:         "some text\nmore text",
			expectedBreak: "\n",
			expectedError: false,
		},
		{
			name:          "CR only",
			input:         "some text\rmore text",
			expectedBreak: "\r",
			expectedError: false,
		},
		{
			name:          "CRLF",
			input:         "some text\r\nmore text",
			expectedBreak: "\r\n",
			expectedError: false,
		},
		{
			name:          "No line break",
			input:         "some text without break",
			expectedBreak: "",
			expectedError: false,
		},
		{
			name:          "LF before CR (separate)",
			input:         "some text\n\rmore text",
			expectedBreak: "",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err, lineBreak := f.findNextLineBreak(tc.input)

			if tc.expectedError && err == nil {
				t.Errorf("Expected error, but got none")
			}

			if !tc.expectedError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if lineBreak != tc.expectedBreak {
				t.Errorf("Expected line break '%v', got '%v'", []byte(tc.expectedBreak), []byte(lineBreak))
			}
		})
	}
}

// minimal RequestHeaderMap mock for header-phase tests
type testHeaderMap struct {
	api.RequestHeaderMap
	values map[string]string
}

func (h testHeaderMap) Get(key string) (string, bool) {
	v, ok := h.values[key]
	return v, ok
}

func (h testHeaderMap) Set(key, value string) { h.values[key] = value }
func (h testHeaderMap) Del(key string)        { delete(h.values, key) }

func newTestRequestHeaders(path string) testHeaderMap {
	return testHeaderMap{values: map[string]string{
		":method":    http.MethodPost,
		":scheme":    "http",
		":authority": "example.com",
		":path":      path,
	}}
}

func TestDecodeHeadersRemovesClientConfigOnEveryPath(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})
	matchList := []common.MatchRule{{
		MatchRuleDomain: "*", HostMatcher: common.ParseHostPattern("*"),
		MatchRuleType: common.PrefixMatch,
		MatchRulePath: "/mcp",
		UpstreamType:  common.RestUpstream,
	}}
	tests := []struct {
		name   string
		path   string
		config *config
		status api.StatusType
	}{
		{name: "malformed URL", path: "/%", status: api.Continue},
		{name: "unmatched path", path: "/other", config: &config{matchList: matchList}, status: api.Continue},
		{name: "static REST", path: "/mcp", config: &config{matchList: matchList}, status: api.Continue},
		{name: "static streamable", path: "/mcp", config: &config{matchList: []common.MatchRule{{MatchRuleDomain: "*", HostMatcher: common.ParseHostPattern("*"), MatchRuleType: common.PrefixMatch, MatchRulePath: "/mcp", UpstreamType: common.StreamableUpstream}}}, status: api.Continue},
		{name: "static SSE", path: "/mcp", config: &config{matchList: []common.MatchRule{{MatchRuleDomain: "*", HostMatcher: common.ParseHostPattern("*"), MatchRuleType: common.PrefixMatch, MatchRulePath: "/mcp", UpstreamType: common.SSEUpstream}}}, status: api.Continue},
		{name: "unknown upstream", path: "/mcp", config: &config{matchList: []common.MatchRule{{MatchRuleDomain: "*", HostMatcher: common.ParseHostPattern("*"), MatchRuleType: common.PrefixMatch, MatchRulePath: "/mcp"}}}, status: api.Continue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := newTestRequestHeaders(tt.path)
			header.Set(mcpServerConfigHeader, "client-value")
			f := &filter{config: tt.config}
			if status := f.DecodeHeaders(header, true); status != tt.status {
				t.Fatalf("expected status %v, got %v", tt.status, status)
			}
			if value, ok := header.Get(mcpServerConfigHeader); ok {
				t.Fatalf("expected client config header to be removed, got %q", value)
			}
		})
	}
}

// TestRestUpstreamSkipsBodyBuffering verifies that plain REST/streamable MCP
// proxying does not buffer the request body. See #3238.
func TestRestUpstreamSkipsBodyBuffering(t *testing.T) {
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	for _, upstreamType := range []common.UpstreamType{common.RestUpstream, common.StreamableUpstream} {
		t.Run(string(upstreamType), func(t *testing.T) {
			parsedURL, _ := url.Parse("http://example.com/mcp/api")
			f := &filter{
				config:      &config{},
				matchedRule: common.MatchRule{UpstreamType: upstreamType},
				needProcess: true,
				req:         &http.Request{Method: http.MethodPost, URL: parsedURL},
			}
			header := testHeaderMap{values: map[string]string{}}

			// endStream=false must still stream rather than buffer the full body.
			status := f.processMcpRequestHeadersForRestUpstream(header, false)
			if status != api.Continue {
				t.Errorf("expected api.Continue, got %v", status)
			}
			if !f.skipRequestBody {
				t.Errorf("expected skipRequestBody to be true")
			}

			// DecodeData must then pass the body through untouched.
			if got := f.DecodeData(nil, false); got != api.Continue {
				t.Errorf("expected DecodeData to Continue when body is skipped, got %v", got)
			}
		})
	}
}

// minimal BufferInstance mock that records what would be sent to the client
type testBuffer struct {
	api.BufferInstance
	data []byte
}

func (b *testBuffer) Bytes() []byte  { return b.data }
func (b *testBuffer) String() string { return string(b.data) }
func (b *testBuffer) Len() int       { return len(b.data) }
func (b *testBuffer) Reset()         { b.data = nil }
func (b *testBuffer) Set(data []byte) error {
	b.data = append([]byte(nil), data...)
	return nil
}
func (b *testBuffer) SetString(s string) error {
	b.data = []byte(s)
	return nil
}

// feedSSEChunks pushes the given upstream chunks through EncodeData in order and
// returns the concatenation of what the client received.
func feedSSEChunks(t *testing.T, f *filter, chunks ...string) string {
	t.Helper()
	received := ""
	for i, chunk := range chunks {
		buffer := &testBuffer{data: []byte(chunk)}
		endStream := i == len(chunks)-1
		if status := f.EncodeData(buffer, endStream); status != api.Continue {
			t.Fatalf("chunk %d: expected api.Continue, got %v", i, status)
		}
		received += buffer.String()
	}
	return received
}

func createSSETestFilter(matchedRule common.MatchRule) *filter {
	return &filter{needProcess: true, matchedRule: matchedRule}
}

func createPathRewriteDisabledRule() common.MatchRule {
	rule := createTestMatchRule()
	rule.EnablePathRewrite = false
	rule.PathRewritePrefix = ""
	return rule
}

// TestEncodeDataFromSSEUpstream_FragmentedEndpointWithoutPathRewrite verifies
// that an endpoint message split across two upstream chunks reaches the client
// in full when path rewriting is disabled. Previously the assembled message was
// only written back inside the rewrite branch, so the client saw the truncated
// tail of the message and never learned the endpoint to POST to. See #4651.
func TestEncodeDataFromSSEUpstream_FragmentedEndpointWithoutPathRewrite(t *testing.T) {
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createSSETestFilter(createPathRewriteDisabledRule())
	chunks := []string{
		"event: endpoint\ndata: https://api.example.com/mes",
		"sions?sessionId=demo\n\n",
	}
	want := chunks[0] + chunks[1]

	got := feedSSEChunks(t, f, chunks...)
	if got != want {
		t.Errorf("expected client to receive the complete message %q, got %q", want, got)
	}
	if f.cachedResponseBody != nil {
		t.Errorf("expected cachedResponseBody to be cleared, got %q", string(f.cachedResponseBody))
	}
	if f.needProcess {
		t.Errorf("expected needProcess to be false after the endpoint event was handled")
	}
}

// TestEncodeDataFromSSEUpstream_FragmentedEndpointWithPathRewrite guards the
// existing behavior of the enabled-and-matching rewrite path.
func TestEncodeDataFromSSEUpstream_FragmentedEndpointWithPathRewrite(t *testing.T) {
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createSSETestFilter(createTestMatchRule())
	chunks := []string{
		"event: endpoint\ndata: https://api.example.com/api/v1/mes",
		"sages?sessionId=demo\n\n",
	}
	want := "event: endpoint\ndata: /mcp/messages?sessionId=demo\n\n"

	got := feedSSEChunks(t, f, chunks...)
	if got != want {
		t.Errorf("expected rewritten message %q, got %q", want, got)
	}
	if f.cachedResponseBody != nil {
		t.Errorf("expected cachedResponseBody to be cleared, got %q", string(f.cachedResponseBody))
	}
}

// TestEncodeDataFromSSEUpstream_FragmentedEndpointWithRewritePrefixMismatch
// verifies that an endpoint URL outside the rewrite prefix is still forwarded
// verbatim instead of being dropped.
func TestEncodeDataFromSSEUpstream_FragmentedEndpointWithRewritePrefixMismatch(t *testing.T) {
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createSSETestFilter(createTestMatchRule())
	chunks := []string{
		"event: endpoint\ndata: https://api.example.com/other/mes",
		"sages?sessionId=demo\n\n",
	}
	want := chunks[0] + chunks[1]

	got := feedSSEChunks(t, f, chunks...)
	if got != want {
		t.Errorf("expected client to receive the complete original message %q, got %q", want, got)
	}
	if f.cachedResponseBody != nil {
		t.Errorf("expected cachedResponseBody to be cleared, got %q", string(f.cachedResponseBody))
	}
}

// TestEncodeDataFromSSEUpstream_SingleChunkWithoutPathRewrite verifies that an
// endpoint message delivered in one chunk passes through untouched.
func TestEncodeDataFromSSEUpstream_SingleChunkWithoutPathRewrite(t *testing.T) {
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createSSETestFilter(createPathRewriteDisabledRule())
	want := "event: endpoint\ndata: https://api.example.com/messages?sessionId=demo\n\n"

	got := feedSSEChunks(t, f, want)
	if got != want {
		t.Errorf("expected unchanged passthrough %q, got %q", want, got)
	}
	if f.cachedResponseBody != nil {
		t.Errorf("expected cachedResponseBody to stay empty, got %q", string(f.cachedResponseBody))
	}
}

// TestEncodeDataFromSSEUpstream_FragmentedMalformedEndpoint verifies that a
// fragmented message which fails endpoint parsing is still forwarded in full
// rather than losing the already drained fragment.
func TestEncodeDataFromSSEUpstream_FragmentedMalformedEndpoint(t *testing.T) {
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createSSETestFilter(createPathRewriteDisabledRule())
	chunks := []string{
		"event: endpoint\nnotda",
		"ta: https://api.example.com/chat\n\n",
	}
	want := chunks[0] + chunks[1]

	got := feedSSEChunks(t, f, chunks...)
	if got != want {
		t.Errorf("expected client to receive the complete message %q, got %q", want, got)
	}
	if f.needProcess {
		t.Errorf("expected needProcess to be false after the parsing failure")
	}
}

// TestEncodeDataFromSSEUpstream_FragmentedNonEndpointEvent verifies that
// non-endpoint events sharing the same cache are forwarded together with the
// endpoint message that follows them.
func TestEncodeDataFromSSEUpstream_FragmentedNonEndpointEvent(t *testing.T) {
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	f := createSSETestFilter(createPathRewriteDisabledRule())
	chunks := []string{
		"event: ping\ndata: ali",
		"ve\n\nevent: endpoint\ndata: https://api.example.com/chat\n\n",
	}
	want := chunks[0] + chunks[1]

	got := feedSSEChunks(t, f, chunks...)
	if got != want {
		t.Errorf("expected client to receive the complete message %q, got %q", want, got)
	}
}

// redisCapture is a tiny RESP server that records the raw bytes of every
// command it receives and answers with ":0". It lets a test observe which Redis
// backend a publication was sent to without running a real Redis server.
type redisCapture struct {
	listener net.Listener
	mu       sync.Mutex
	received string
	getValue string
}

func newRedisCapture(t *testing.T) *redisCapture {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	capture := &redisCapture{listener: listener}
	go capture.serve()
	t.Cleanup(func() { _ = listener.Close() })
	return capture
}

func (c *redisCapture) Addr() string {
	return c.listener.Addr().String()
}

func (c *redisCapture) Received() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.received
}

func (c *redisCapture) setGetValue(value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getValue = value
}

func (c *redisCapture) serve() {
	for {
		conn, err := c.listener.Accept()
		if err != nil {
			return
		}
		go c.handle(conn)
	}
}

func (c *redisCapture) handle(conn net.Conn) {
	defer conn.Close()
	buffer := make([]byte, 4096)
	for {
		n, err := conn.Read(buffer)
		if n > 0 {
			request := string(buffer[:n])
			c.mu.Lock()
			c.received += request
			getValue := c.getValue
			c.mu.Unlock()
			response := ":0\r\n"
			upperRequest := strings.ToUpper(request)
			switch {
			case strings.Contains(upperRequest, "\r\nPING\r\n"):
				response = "+PONG\r\n"
			case strings.Contains(upperRequest, "\r\nGET\r\n"):
				if getValue == "" {
					response = "$-1\r\n"
				} else {
					response = fmt.Sprintf("$%d\r\n%s\r\n", len(getValue), getValue)
				}
			case strings.Contains(upperRequest, "\r\nSET\r\n"):
				response = "+OK\r\n"
			}
			if _, writeErr := conn.Write([]byte(response)); writeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// newTestSessionRedisClient builds a RedisClient for the mcp-session filter
// pointing at the given address. The address does not have to be a real Redis
// server; these tests only care about which backend a publication is routed to.
func newTestSessionRedisClient(t *testing.T, addr string) *common.RedisClient {
	t.Helper()
	redisConfig, err := common.ParseRedisConfig(map[string]interface{}{"address": addr})
	require.NoError(t, err)
	client, err := common.NewRedisClient(redisConfig)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// TestResolveSessionRedisClientKeepsLiveSessionOnItsOwnBackend covers the
// selection rule: a session with a live SSE subscription keeps using the client
// that opened the subscription even after mcpServer.redis was hot-updated,
// while sessions without a subscription use the configured client.
func TestResolveSessionRedisClientKeepsLiveSessionOnItsOwnBackend(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})

	configuredClient := newTestSessionRedisClient(t, "127.0.0.1:6390")
	sessionClient := newTestSessionRedisClient(t, "127.0.0.1:6391")

	f := &filter{config: &config{redisClient: configuredClient}}

	const sessionID = "live-session"
	common.GlobalSessionRegistry.Register(sessionID, sessionClient)
	t.Cleanup(func() { common.GlobalSessionRegistry.Unregister(sessionID) })

	require.Same(t, sessionClient, f.resolveSessionRedisClient(sessionID))
	require.Same(t, configuredClient, f.resolveSessionRedisClient("unknown-session"))
	require.Nil(t, f.resolveSessionRedisClient(""))
}

func TestLiveSessionPostAcrossRedisHotUpdate(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})

	backendA := newRedisCapture(t)
	backendB := newRedisCapture(t)
	sessionClient := newTestSessionRedisClient(t, backendA.Addr())
	configuredClient := newTestSessionRedisClient(t, backendB.Addr())
	matchedRule := common.MatchRule{
		ServerName:      "server-a",
		MatchRuleDomain: "*",
		HostMatcher:     common.ParseHostPattern("*"),
		MatchRuleType:   common.PrefixMatch,
		MatchRulePath:   "/mcp",
		UpstreamType:    common.RestUpstream,
	}
	backendA.setGetValue(matchedRule.SessionServerName())

	const sessionID = "session-after-hot-update"
	common.GlobalSessionRegistry.Register(sessionID, sessionClient)
	t.Cleanup(func() { common.GlobalSessionRegistry.Unregister(sessionID) })

	decoder := &registryDecoderCallbacks{}
	f := &filter{
		callbacks: &registryFilterCallbacks{decoder: decoder},
		config: &config{
			matchList:       []common.MatchRule{matchedRule},
			redisClient:     configuredClient,
			sessionRegistry: common.NewRedisSessionRegistry(configuredClient),
		},
	}

	header := newTestRequestHeaders("/mcp?sessionId=" + sessionID)
	status := f.DecodeHeaders(header, true)
	require.Zero(t, decoder.statusCode)
	require.Equal(t, api.Continue, status)

	buffer := &testBuffer{data: []byte(`{"jsonrpc":"2.0","id":1,"result":"ok"}`)}
	require.Equal(t, api.Continue, f.EncodeData(buffer, true))

	// Redis command names are case insensitive, so compare the command without
	// depending on the casing the client happens to use, and check the session
	// channel separately.
	receivedA := backendA.Received()
	upperReceivedA := strings.ToUpper(receivedA)
	require.Contains(t, upperReceivedA, "\r\nGET\r\n")
	require.Contains(t, upperReceivedA, "\r\nSET\r\n")
	require.Contains(t, upperReceivedA, "PUBLISH")
	require.Contains(t, receivedA, "mcp-server-sse:"+sessionID)
	upperReceivedB := strings.ToUpper(backendB.Received())
	require.NotContains(t, upperReceivedB, "\r\nGET\r\n")
	require.NotContains(t, upperReceivedB, "PUBLISH")
}
