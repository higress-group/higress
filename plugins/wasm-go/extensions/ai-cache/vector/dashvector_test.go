package vector

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// noopLog satisfies log.Log without touching the wasm host, so the DashVector
// response parser can be driven directly from a unit test.
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

func TestDvGetStringValueIgnoresNonStringFields(t *testing.T) {
	fields := map[string]interface{}{
		"query":  "q",
		"answer": float64(42),
		"flag":   true,
		"object": map[string]interface{}{"nested": "v"},
		"empty":  nil,
	}

	require.Equal(t, "q", getStringValue(fields, "query"))
	require.Equal(t, "", getStringValue(fields, "answer"))
	require.Equal(t, "", getStringValue(fields, "flag"))
	require.Equal(t, "", getStringValue(fields, "object"))
	require.Equal(t, "", getStringValue(fields, "empty"))
	require.Equal(t, "", getStringValue(fields, "absent"))
}

func TestDvParseQueryResponseKeepsStringFields(t *testing.T) {
	provider := &DvProvider{}

	results, err := provider.ParseQueryResponse(
		[]byte(`{"code":0,"output":[{"id":"1","fields":{"query":"q","answer":"a"},"score":0.9}]}`), nil, noopLog{})
	require.NoError(t, err)
	require.Equal(t, []QueryResult{{Text: "q", Score: 0.9, Answer: "a"}}, results)

	// a collection populated by another pipeline can store these fields with
	// non-string types; the lookup must degrade instead of panicking.
	results, err = provider.ParseQueryResponse(
		[]byte(`{"code":0,"output":[{"id":"1","fields":{"query":"q","answer":42},"score":0.9}]}`), nil, noopLog{})
	require.NoError(t, err)
	require.Equal(t, []QueryResult{{Text: "q", Score: 0.9}}, results)
}
