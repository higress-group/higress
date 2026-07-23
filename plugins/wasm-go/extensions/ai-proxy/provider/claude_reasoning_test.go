package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// B1 (issue #4107, Bug 1): when reasoning_effort="high" maps to a fixed
// budget_tokens of 16384 but the client also sends max_tokens=16384, Claude
// rejects the request with 400 "max_tokens must be greater than
// thinking.budget_tokens". The built request must keep budget_tokens
// strictly below max_tokens.
func TestClaudeProvider_ReasoningEffortHigh_ClampsBudgetBelowMaxTokens(t *testing.T) {
	provider := &claudeProvider{
		config: ProviderConfig{claudeCodeMode: false},
	}
	request := &chatCompletionRequest{
		Model:           "claude-sonnet-4-5-20250929",
		MaxTokens:       16384,
		ReasoningEffort: "high",
		Messages: []chatMessage{
			{Role: roleUser, Content: "Hello"},
		},
	}

	claudeReq := provider.buildClaudeTextGenRequest(request)

	require.NotNil(t, claudeReq.Thinking)
	assert.Equal(t, "enabled", claudeReq.Thinking.Type)
	// Claude requires max_tokens > thinking.budget_tokens (strict inequality).
	assert.Less(t, claudeReq.Thinking.BudgetTokens, claudeReq.MaxTokens,
		"thinking.budget_tokens must be strictly less than max_tokens, otherwise Claude returns 400")
}

// B2 (issue #4107, Bug 2): standard OpenAI SDKs (e.g. langchain) send the
// thinking budget via extra_body.thinking.budget_tokens (plural). It must be
// parsed (model.go wrongly tagged it singular "budget_token") and mapped onto
// the Claude thinking config, instead of being silently dropped and overwritten
// by reasoning_effort.
func TestClaudeProvider_StandardThinkingBudgetTokens_ParsedAndMapped(t *testing.T) {
	provider := &claudeProvider{
		config: ProviderConfig{claudeCodeMode: false},
	}
	// Standard OpenAI SDK payload: thinking.budget_tokens (plural).
	body := []byte(`{"model":"claude-sonnet-4-5-20250929","max_tokens":16384,"messages":[{"role":"user","content":"Hi"}],"thinking":{"type":"enabled","budget_tokens":8192}}`)
	request := &chatCompletionRequest{}
	require.NoError(t, json.Unmarshal(body, request))
	// The plural "budget_tokens" must be parsed into the thinking param.
	require.NotNil(t, request.Thinking)
	assert.Equal(t, 8192, request.Thinking.BudgetTokens,
		"standard OpenAI field thinking.budget_tokens must be parsed (plural)")

	claudeReq := provider.buildClaudeTextGenRequest(request)
	require.NotNil(t, claudeReq.Thinking)
	assert.Equal(t, "enabled", claudeReq.Thinking.Type)
	assert.Equal(t, 8192, claudeReq.Thinking.BudgetTokens,
		"explicit thinking.budget_tokens must be honored, not overwritten by reasoning_effort")
}

// B3 (issue #4107): an explicit thinking budget (via the standard OpenAI
// "thinking" field) that is >= max_tokens must also be clamped strictly
// below max_tokens, exactly like the reasoning_effort-derived path.
func TestClaudeProvider_ExplicitThinkingBudgetTokens_ClampedBelowMaxTokens(t *testing.T) {
	provider := &claudeProvider{
		config: ProviderConfig{claudeCodeMode: false},
	}
	// Explicit standard OpenAI thinking budget >= max_tokens.
	body := []byte(`{"model":"claude-sonnet-4-5-20250929","max_tokens":16384,"messages":[{"role":"user","content":"Hi"}],"thinking":{"type":"enabled","budget_tokens":16384}}`)
	request := &chatCompletionRequest{}
	require.NoError(t, json.Unmarshal(body, request))
	claudeReq := provider.buildClaudeTextGenRequest(request)
	require.NotNil(t, claudeReq.Thinking)
	assert.Equal(t, "enabled", claudeReq.Thinking.Type)
	// Claude requires max_tokens > thinking.budget_tokens (strict inequality).
	assert.Less(t, claudeReq.Thinking.BudgetTokens, claudeReq.MaxTokens,
		"explicit thinking.budget_tokens must be clamped below max_tokens")
}
