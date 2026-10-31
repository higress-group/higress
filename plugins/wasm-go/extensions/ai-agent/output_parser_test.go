package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// discardLog satisfies log.Log without touching the wasm host, so outputParser
// can be exercised in a native unit test.
type discardLog struct{}

func (discardLog) Trace(string)                  {}
func (discardLog) Tracef(string, ...interface{}) {}
func (discardLog) Debug(string)                  {}
func (discardLog) Debugf(string, ...interface{}) {}
func (discardLog) Info(string)                   {}
func (discardLog) Infof(string, ...interface{})  {}
func (discardLog) Warn(string)                   {}
func (discardLog) Warnf(string, ...interface{})  {}
func (discardLog) Error(string)                  {}
func (discardLog) Errorf(string, ...interface{}) {}
func (discardLog) Critical(string)               {}
func (discardLog) Criticalf(string, ...interface{}) {
}
func (discardLog) ResetID(string) {}

func TestOutputParserSurvivesASingleCodeFence(t *testing.T) {
	// A completion truncated right after the opening fence used to slice
	// response[start+3 : end] with start == end.
	require.NotPanics(t, func() {
		action, input := outputParser("```json {\"a\":1}", discardLog{})
		require.Equal(t, "", action)
		require.Equal(t, "", input)
	})
}

func TestOutputParserSurvivesJSONWithoutAnInputField(t *testing.T) {
	// json.Unmarshal succeeds, so err is nil, but neither an action name nor an
	// input can be extracted; the old code called err.Error() on that nil error.
	require.NotPanics(t, func() {
		action, input := outputParser("{\"action\":\"foo\"}", discardLog{})
		require.Equal(t, "", action)
		require.Equal(t, "", input)
	})
}

func TestOutputParserSurvivesJSONWithoutAnActionField(t *testing.T) {
	require.NotPanics(t, func() {
		action, input := outputParser("{\"action_input\":\"bar\"}", discardLog{})
		require.Equal(t, "", action)
		require.Equal(t, "", input)
	})
}

func TestOutputParserKeepsWorkingForWellFormedOutput(t *testing.T) {
	action, input := outputParser("{\"action\":\"foo\",\"action_input\":\"bar\"}", discardLog{})
	require.Equal(t, "foo", action)
	require.Equal(t, "bar", input)

	action, input = outputParser("```\n{\"action\":\"foo\",\"action_input\":\"bar\"}\n```", discardLog{})
	require.Equal(t, "foo", action)
	require.Equal(t, "bar", input)

	action, input = outputParser(`{"action":"Final Answer","action_input":"done"}`, discardLog{})
	require.Equal(t, "Final Answer", action)
	require.Equal(t, "done", input)
}
