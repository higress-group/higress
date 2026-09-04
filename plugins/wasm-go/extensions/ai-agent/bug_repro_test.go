package main

import (
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/stretchr/testify/require"
)

type noopLog struct{}

func (noopLog) Trace(string)                     {}
func (noopLog) Tracef(string, ...interface{})    {}
func (noopLog) Debug(string)                     {}
func (noopLog) Debugf(string, ...interface{})    {}
func (noopLog) Info(string)                      {}
func (noopLog) Infof(string, ...interface{})     {}
func (noopLog) Warn(string)                      {}
func (noopLog) Warnf(string, ...interface{})     {}
func (noopLog) Error(string)                     {}
func (noopLog) Errorf(string, ...interface{})    {}
func (noopLog) Critical(string)                  {}
func (noopLog) Criticalf(string, ...interface{}) {}
func (noopLog) ResetID(string)                   {}

func TestOutputParserValidButUnexpectedJSONDoesNotPanic(t *testing.T) {
	require.NotPanics(t, func() {
		action, input := outputParser(`{"foo":"bar"}`, noopLog{})
		require.Empty(t, action)
		require.Empty(t, input)
	})
}

func TestOnHttpResponseBodyEmptyChoicesDoesNotPanic(t *testing.T) {
	require.NotPanics(t, func() {
		action := onHttpResponseBody(nil, PluginConfig{}, []byte(`{"choices":[]}`), noopLog{})
		require.Equal(t, types.ActionContinue, action)
	})
}
