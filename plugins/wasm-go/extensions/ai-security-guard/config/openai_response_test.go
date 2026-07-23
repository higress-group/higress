package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshalOpenAIDenyDataEscapesUntrustedContent(t *testing.T) {
	maliciousContent := "blocked\"}],\"injected\":true,\"message\":{\"content\":\"owned\n\\tail"
	guardrail := []byte(`{"code":403,"denyMessage":"blocked","blockedDetails":[]}`)

	for _, tc := range []struct {
		name      string
		isStream  bool
		guardrail []byte
	}{
		{name: "completion legacy", isStream: false},
		{name: "completion structured", isStream: false, guardrail: guardrail},
		{name: "stream legacy", isStream: true},
		{name: "stream structured", isStream: true, guardrail: guardrail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := marshalOpenAIDenyData(maliciousContent, tc.guardrail, "chatcmpl-test", 123, tc.isStream)
			require.NoError(t, err)

			if !tc.isStream {
				body := decodeOpenAIFrame(t, data)
				choice := firstOpenAIChoice(t, body)
				message := choice["message"].(map[string]any)
				require.Equal(t, maliciousContent, message["content"])
				require.NotContains(t, body, "injected")
				require.NotContains(t, choice, "injected")
				assertGuardrailPresence(t, choice, tc.guardrail != nil)
				return
			}

			frames := strings.Split(string(data), "\n\n")
			require.Len(t, frames, 3)
			require.Equal(t, "data: [DONE]", frames[2])

			firstFrame := decodeOpenAIFrame(t, []byte(strings.TrimPrefix(frames[0], "data:")))
			firstChoice := firstOpenAIChoice(t, firstFrame)
			delta := firstChoice["delta"].(map[string]any)
			require.Equal(t, maliciousContent, delta["content"])
			require.NotContains(t, firstFrame, "injected")
			require.NotContains(t, firstChoice, "injected")
			require.NotContains(t, firstChoice, "x_higress_guardrail")

			finalFrame := decodeOpenAIFrame(t, []byte(strings.TrimPrefix(frames[1], "data:")))
			finalChoice := firstOpenAIChoice(t, finalFrame)
			require.Equal(t, "stop", finalChoice["finish_reason"])
			require.NotContains(t, finalFrame, "injected")
			require.NotContains(t, finalChoice, "injected")
			assertGuardrailPresence(t, finalChoice, tc.guardrail != nil)
		})
	}
}

func decodeOpenAIFrame(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var frame map[string]any
	require.NoError(t, json.Unmarshal(data, &frame))
	return frame
}

func firstOpenAIChoice(t *testing.T, frame map[string]any) map[string]any {
	t.Helper()
	choices, ok := frame["choices"].([]any)
	require.True(t, ok)
	require.Len(t, choices, 1)
	choice, ok := choices[0].(map[string]any)
	require.True(t, ok)
	return choice
}

func assertGuardrailPresence(t *testing.T, choice map[string]any, expected bool) {
	t.Helper()
	guardrail, exists := choice["x_higress_guardrail"]
	require.Equal(t, expected, exists)
	if expected {
		require.IsType(t, map[string]any{}, guardrail)
	}
}
