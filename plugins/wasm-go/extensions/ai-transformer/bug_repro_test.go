package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractHttpFrameMissingSeparatorReturnsError(t *testing.T) {
	require.NotPanics(t, func() {
		headers, body, err := extraceHttpFrame("plain model output without an HTTP separator")
		require.Error(t, err)
		require.Nil(t, headers)
		require.Nil(t, body)
	})
}

func TestExtractHttpFrameCRLFSeparator(t *testing.T) {
	headers, body, err := extraceHttpFrame("content-type: application/json\r\nx-test: ok\r\n\r\n{\"ok\":true}")
	require.NoError(t, err)
	require.Equal(t, [][2]string{
		{"content-type", " application/json"},
		{"x-test", " ok"},
	}, headers)
	require.Equal(t, []byte(`{"ok":true}`), body)
}
