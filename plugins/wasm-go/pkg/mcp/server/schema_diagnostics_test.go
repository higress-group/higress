// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package server

import (
	"fmt"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type diagnosticCaptureLogger struct {
	messages []string
	warnings []string
}

func (l *diagnosticCaptureLogger) record(message string) { l.messages = append(l.messages, message) }
func (l *diagnosticCaptureLogger) Trace(message string)  { l.record(message) }
func (l *diagnosticCaptureLogger) Tracef(format string, args ...interface{}) {
	l.record(fmt.Sprintf(format, args...))
}
func (l *diagnosticCaptureLogger) Debug(message string) { l.record(message) }
func (l *diagnosticCaptureLogger) Debugf(format string, args ...interface{}) {
	l.record(fmt.Sprintf(format, args...))
}
func (l *diagnosticCaptureLogger) Info(message string) { l.record(message) }
func (l *diagnosticCaptureLogger) Infof(format string, args ...interface{}) {
	l.record(fmt.Sprintf(format, args...))
}
func (l *diagnosticCaptureLogger) Warn(message string) {
	l.record(message)
	l.warnings = append(l.warnings, message)
}
func (l *diagnosticCaptureLogger) Warnf(format string, args ...interface{}) {
	l.Warn(fmt.Sprintf(format, args...))
}
func (l *diagnosticCaptureLogger) Error(message string) { l.record(message) }
func (l *diagnosticCaptureLogger) Errorf(format string, args ...interface{}) {
	l.record(fmt.Sprintf(format, args...))
}
func (l *diagnosticCaptureLogger) Critical(message string) { l.record(message) }
func (l *diagnosticCaptureLogger) Criticalf(format string, args ...interface{}) {
	l.record(fmt.Sprintf(format, args...))
}
func (l *diagnosticCaptureLogger) ResetID(string) {}

func TestDegradedPublicationWarningIsDeterministicBoundedAndSanitized(t *testing.T) {
	diagnostics := make([]directToolDiagnostic, 0, maxSchemaDiagnosticRecords+3)
	for i := maxSchemaDiagnosticRecords + 2; i >= 0; i-- {
		diagnostics = append(diagnostics, directToolDiagnostic{
			name:   fmt.Sprintf("tool-%02d", i),
			reason: schemaDiagnosticUnsupportedKeyword,
			detail: "unsupported schema keyword oneOf",
		})
	}
	diagnostics[len(diagnostics)-1].name += "\n\t\x1b\u202e\u2028" + strings.Repeat("工", maxSchemaDiagnosticToolNameRunes+20)
	diagnostics[len(diagnostics)-1].detail += "\r\x00" + strings.Repeat("x", maxSchemaDiagnosticDetailRunes+20)
	snapshot := directToolSnapshot{degraded: diagnostics}

	warning := snapshot.degradedPublicationWarning("server\n\x1b" + strings.Repeat("s", maxSchemaDiagnosticServerRunes+20))
	assert.Contains(t, warning, "total=11")
	assert.Contains(t, warning, "omitted=3")
	assert.Contains(t, warning, "modern tools/call will be rejected; legacy calls remain available")
	assert.Equal(t, maxSchemaDiagnosticRecords, strings.Count(warning, "{tool="))
	assert.GreaterOrEqual(t, strings.Count(warning, "..."), 2, "both the included tool and detail must be truncated")
	for i := 0; i < maxSchemaDiagnosticRecords; i++ {
		assert.Contains(t, warning, fmt.Sprintf("tool-%02d", i))
	}
	assert.NotContains(t, warning, "tool-08")
	assert.LessOrEqual(t, utf8.RuneCountInString(warning), maxSchemaDiagnosticWarningRunes)
	for _, r := range warning {
		assert.False(t, unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp), "warning contains control rune %U", r)
	}

	reversed := append([]directToolDiagnostic(nil), diagnostics...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	assert.Equal(t, warning, (directToolSnapshot{degraded: reversed}).degradedPublicationWarning("server\n\x1b"+strings.Repeat("s", maxSchemaDiagnosticServerRunes+20)))
	assert.Equal(t,
		`Direct tools published without local schema validation: server="server" total=11 records=[] omitted=11; modern tools/call will be rejected; legacy calls remain available`,
		snapshot.degradedPublicationWarningWithin("server", 1),
		"the defensive fallback must retain the fixed impact guidance",
	)
}

func TestDegradedPublicationWarningIncludesActionableSchemaPathAndExcludesLegacyOnly(t *testing.T) {
	registered := newValidationTestServer()
	registered.AddMCPTool("getTransactionRecordListV2", &validationTestTool{
		counters: &validationToolCounters{},
		schema: map[string]any{
			"type":        "object",
			"description": "credential=must-not-be-logged",
			"properties": map[string]any{
				"businessType": map[string]any{"type": "array", "enum": []any{"A"}},
			},
		},
	})
	rest := NewRestMCPServer("legacy")
	require.NoError(t, rest.AddRestTool(RestTool{
		Name:        "legacy-secret-tool",
		Description: "legacy",
		LegacyOnly:  true,
		Args: []RestToolArg{{
			Name:  "value",
			Type:  "array",
			Items: map[string]any{"oneOf": []any{}},
		}},
		RequestTemplate: RestToolRequestTemplate{URL: "/legacy", Method: "POST"},
	}))
	registered.AddMCPTool("legacy-secret-tool", rest.GetMCPTools()["legacy-secret-tool"])

	snapshot := compileDirectToolSnapshot(registered)
	warning := snapshot.degradedPublicationWarning("transactions")
	assert.Contains(t, warning, `server="transactions" total=1`)
	assert.Contains(t, warning, `tool="getTransactionRecordListV2"`)
	assert.Contains(t, warning, `reason=contradictory_constraint`)
	assert.Contains(t, warning, `$.properties[\"businessType\"].enum: requires a primitive type`)
	assert.Contains(t, warning, "omitted=0")
	assert.NotContains(t, warning, "legacy-secret-tool")
	assert.NotContains(t, warning, "must-not-be-logged")
}

func TestSchemaDiagnosticWarnsOnceAtPublicationAndNotDuringModernCall(t *testing.T) {
	savedGlobalContext := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	counters := &validationToolCounters{}
	registered := newValidationTestServer()
	registered.AddMCPTool("degraded", &validationTestTool{
		counters: counters,
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"businessType": map[string]any{"type": "array", "enum": []any{"A"}},
			},
		},
	})
	registered.AddMCPTool("unlistable", &validationTestTool{
		counters: counters,
		schema: map[string]any{
			"type":     "object",
			"callback": func() {},
		},
	})
	Load(AddMCPServer("diagnostic-server", registered))
	Initialize()
	capture := &diagnosticCaptureLogger{}
	log.SetPluginLog(capture)
	t.Cleanup(func() {
		log.SetPluginLog(&testLogger{})
		globalContext = savedGlobalContext
	})

	host, status := wasmtest.NewTestHost([]byte(`{"server":{"name":"diagnostic-server"}}`))
	require.Equal(t, types.OnPluginStartStatusOK, status)
	t.Cleanup(host.Reset)
	require.Len(t, capture.warnings, 1)
	assert.Contains(t, capture.warnings[0], `server="diagnostic-server" total=2`)
	assert.Contains(t, capture.warnings[0], "modern tools/call will be rejected; legacy calls remain available")

	listBody := []byte(`{"jsonrpc":"2.0","id":"list","method":"tools/list","params":{"_meta":{"` +
		protocol.MetaProtocolVersion + `":"2026-07-28","` + protocol.MetaClientCapabilities + `":{}}}}`)
	for range 2 {
		require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(modernToolListHeaders()))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(listBody))
		response := host.GetLocalResponse()
		require.NotNil(t, response)
		assert.Equal(t, int64(1), gjson.GetBytes(response.Data, "result.tools.#").Int())
		assert.Equal(t, "degraded", gjson.GetBytes(response.Data, "result.tools.0.name").String())
		host.CompleteHttp()
	}
	assert.Len(t, capture.warnings, 1, "repeated tools/list must not amplify publication diagnostics")

	require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(modernToolHeaders("degraded")))
	require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(modernToolCallBody("degraded", `{"businessType":["A"]}`)))
	response := host.GetLocalResponse()
	require.NotNil(t, response)
	assert.Equal(t, int64(protocol.CodeInternalError), gjson.GetBytes(response.Data, "error.code").Int())
	assert.Zero(t, counters.create)
	assert.Zero(t, counters.call)
	assert.Len(t, capture.warnings, 1, "tools/call must not repeat publication diagnostics")
	detailOccurrences := 0
	for _, message := range capture.messages {
		if strings.Contains(message, `$.properties[\"businessType\"].enum`) {
			detailOccurrences++
		}
	}
	assert.Equal(t, 1, detailOccurrences, "the actionable compiler detail belongs only to publication")
}
