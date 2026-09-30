package mcp_session

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
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

// TestRestUpstreamSkipsBodyBufferingWithoutRateLimit verifies that plain
// REST/streamable MCP proxying does not buffer the request body, so that large
// requests are not rejected with a 413. See #3238.
func TestRestUpstreamSkipsBodyBufferingWithoutRateLimit(t *testing.T) {
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	for _, upstreamType := range []common.UpstreamType{common.RestUpstream, common.StreamableUpstream} {
		t.Run(string(upstreamType), func(t *testing.T) {
			parsedURL, _ := url.Parse("http://example.com/mcp/api")
			f := &filter{
				config:      &config{enableUserLevelServer: false},
				matchedRule: common.MatchRule{UpstreamType: upstreamType},
				needProcess: true,
				req:         &http.Request{Method: http.MethodPost, URL: parsedURL},
			}
			header := testHeaderMap{values: map[string]string{}}

			// endStream=false would previously return StopAndBuffer and buffer
			// the whole body. Without rate limiting we should stream instead.
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

// TestRestUpstreamBuffersBodyWhenRateLimitEnabled verifies that the body is
// still buffered when user-level rate limiting is enabled, since that path
// needs to inspect the JSON-RPC method in the body.
func TestRestUpstreamBuffersBodyWhenRateLimitEnabled(t *testing.T) {
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	// Use a path with fewer than 3 segments so the config handler (which is
	// nil in this test) is never queried; f.ratelimit is still set to true
	// because enableUserLevelServer is enabled.
	parsedURL, _ := url.Parse("http://example.com/mcp")
	f := &filter{
		config:      &config{enableUserLevelServer: true},
		matchedRule: common.MatchRule{UpstreamType: common.RestUpstream},
		needProcess: true,
		req:         &http.Request{Method: http.MethodPost, URL: parsedURL},
	}

	header := testHeaderMap{values: map[string]string{}}
	status := f.processMcpRequestHeadersForRestUpstream(header, false)
	if status != api.StopAndBuffer {
		t.Errorf("expected api.StopAndBuffer when rate limiting is enabled, got %v", status)
	}
	if f.skipRequestBody {
		t.Errorf("expected skipRequestBody to be false when buffering is required")
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
