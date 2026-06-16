package provider

import (
	"net/http"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAppendOrReplaceAPIKey(t *testing.T) {
	t.Run("empty apiKey returns path unchanged", func(t *testing.T) {
		path := "/v1/publishers/google/models/gemini:generateContent"
		assert.Equal(t, path, appendOrReplaceAPIKey(path, ""))
	})

	t.Run("path without query appends ?key=", func(t *testing.T) {
		result := appendOrReplaceAPIKey("/v1/models/gemini:generateContent", "my-key")
		assert.Equal(t, "/v1/models/gemini:generateContent?key=my-key", result)
	})

	t.Run("path with existing query appends &key=", func(t *testing.T) {
		result := appendOrReplaceAPIKey("/v1/models/gemini:streamGenerateContent?alt=sse", "my-key")
		assert.Contains(t, result, "alt=sse")
		assert.Contains(t, result, "key=my-key")
	})

	t.Run("existing key parameter is replaced", func(t *testing.T) {
		result := appendOrReplaceAPIKey("/v1/models/gemini:generateContent?key=old-key&trace=1", "new-key")
		assert.Contains(t, result, "key=new-key")
		assert.NotContains(t, result, "old-key")
		assert.Contains(t, result, "trace=1")
	})

	t.Run("unparseable path without query falls back to ?key= append", func(t *testing.T) {
		// A bare string with no leading slash is not a valid RequestURI
		result := appendOrReplaceAPIKey("not-a-valid-uri", "my-key")
		assert.Equal(t, "not-a-valid-uri?key=my-key", result)
	})

	t.Run("unparseable path with query falls back to &key= append", func(t *testing.T) {
		result := appendOrReplaceAPIKey("not-a-valid-uri?foo=bar", "my-key")
		assert.Equal(t, "not-a-valid-uri?foo=bar&key=my-key", result)
	})
}

func TestVertexProviderBuildChatRequestStructuredOutputMapping(t *testing.T) {
	t.Run("json_object response format", func(t *testing.T) {
		v := &vertexProvider{}
		req := &chatCompletionRequest{
			Model: "gemini-2.5-flash",
			Messages: []chatMessage{
				{Role: roleUser, Content: "hello"},
			},
			ResponseFormat: map[string]interface{}{
				"type": "json_object",
			},
		}

		vertexReq, err := v.buildVertexChatRequest(req)
		require.NoError(t, err)
		require.NotNil(t, vertexReq)

		assert.Equal(t, util.MimeTypeApplicationJson, vertexReq.GenerationConfig.ResponseMimeType)
		assert.Nil(t, vertexReq.GenerationConfig.ResponseSchema)
	})

	t.Run("json_schema response format with nested schema", func(t *testing.T) {
		v := &vertexProvider{}
		schema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"answer": map[string]interface{}{
					"type": "string",
				},
			},
			"required": []interface{}{"answer"},
		}
		req := &chatCompletionRequest{
			Model: "gemini-2.5-flash",
			Messages: []chatMessage{
				{Role: roleUser, Content: "hello"},
			},
			ResponseFormat: map[string]interface{}{
				"type": "json_schema",
				"json_schema": map[string]interface{}{
					"name":   "response",
					"strict": true,
					"schema": schema,
				},
			},
		}

		vertexReq, err := v.buildVertexChatRequest(req)
		require.NoError(t, err)
		require.NotNil(t, vertexReq)

		assert.Equal(t, util.MimeTypeApplicationJson, vertexReq.GenerationConfig.ResponseMimeType)
		assert.Equal(t, schema, vertexReq.GenerationConfig.ResponseSchema)
	})

	t.Run("json_schema response format with direct schema object", func(t *testing.T) {
		v := &vertexProvider{}
		schema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"city": map[string]interface{}{
					"type": "string",
				},
			},
			"required": []interface{}{"city"},
		}
		req := &chatCompletionRequest{
			Model: "gemini-2.5-flash",
			Messages: []chatMessage{
				{Role: roleUser, Content: "hello"},
			},
			ResponseFormat: map[string]interface{}{
				"type":        "json_schema",
				"json_schema": schema,
			},
		}

		vertexReq, err := v.buildVertexChatRequest(req)
		require.NoError(t, err)
		require.NotNil(t, vertexReq)

		assert.Equal(t, util.MimeTypeApplicationJson, vertexReq.GenerationConfig.ResponseMimeType)
		assert.Equal(t, schema, vertexReq.GenerationConfig.ResponseSchema)
	})

	t.Run("json_schema response format without valid schema should return error", func(t *testing.T) {
		v := &vertexProvider{}
		req := &chatCompletionRequest{
			Model: "gemini-2.5-flash",
			Messages: []chatMessage{
				{Role: roleUser, Content: "hello"},
			},
			ResponseFormat: map[string]interface{}{
				"type":        "json_schema",
				"json_schema": "invalid",
			},
		}

		vertexReq, err := v.buildVertexChatRequest(req)
		require.Error(t, err)
		assert.Nil(t, vertexReq)
		assert.Contains(t, err.Error(), "invalid response_format.json_schema")
	})

	t.Run("direct schema in response_format for compatibility", func(t *testing.T) {
		v := &vertexProvider{}
		schema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"result": map[string]interface{}{
					"type": "string",
				},
			},
		}
		req := &chatCompletionRequest{
			Model: "gemini-2.5-flash",
			Messages: []chatMessage{
				{Role: roleUser, Content: "hello"},
			},
			ResponseFormat: schema,
		}

		vertexReq, err := v.buildVertexChatRequest(req)
		require.NoError(t, err)
		require.NotNil(t, vertexReq)

		assert.Equal(t, util.MimeTypeApplicationJson, vertexReq.GenerationConfig.ResponseMimeType)
		assert.Equal(t, schema, vertexReq.GenerationConfig.ResponseSchema)
	})

	t.Run("text response format keeps default text output", func(t *testing.T) {
		v := &vertexProvider{}
		req := &chatCompletionRequest{
			Model: "gemini-2.5-flash",
			Messages: []chatMessage{
				{Role: roleUser, Content: "hello"},
			},
			ResponseFormat: map[string]interface{}{
				"type": "text",
			},
		}

		vertexReq, err := v.buildVertexChatRequest(req)
		require.NoError(t, err)
		require.NotNil(t, vertexReq)

		assert.Empty(t, vertexReq.GenerationConfig.ResponseMimeType)
		assert.Nil(t, vertexReq.GenerationConfig.ResponseSchema)
	})

	t.Run("unknown response format does not inject schema config", func(t *testing.T) {
		v := &vertexProvider{}
		req := &chatCompletionRequest{
			Model: "gemini-2.5-flash",
			Messages: []chatMessage{
				{Role: roleUser, Content: "hello"},
			},
			ResponseFormat: map[string]interface{}{
				"type": "xml",
			},
		}

		vertexReq, err := v.buildVertexChatRequest(req)
		require.NoError(t, err)
		require.NotNil(t, vertexReq)

		assert.Empty(t, vertexReq.GenerationConfig.ResponseMimeType)
		assert.Nil(t, vertexReq.GenerationConfig.ResponseSchema)
	})

	t.Run("gemini 2.0 json_schema is ignored for stability", func(t *testing.T) {
		v := &vertexProvider{}
		schema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"answer": map[string]interface{}{
					"type": "string",
				},
			},
		}
		req := &chatCompletionRequest{
			Model: "gemini-2.0-flash",
			Messages: []chatMessage{
				{Role: roleUser, Content: "hello"},
			},
			ResponseFormat: map[string]interface{}{
				"type": "json_schema",
				"json_schema": map[string]interface{}{
					"name":   "response",
					"strict": true,
					"schema": schema,
				},
			},
		}

		vertexReq, err := v.buildVertexChatRequest(req)
		require.NoError(t, err)
		require.NotNil(t, vertexReq)
		assert.Empty(t, vertexReq.GenerationConfig.ResponseMimeType)
		assert.Nil(t, vertexReq.GenerationConfig.ResponseSchema)
	})

	t.Run("gemini 2.0 malformed json_schema is also ignored", func(t *testing.T) {
		v := &vertexProvider{}
		req := &chatCompletionRequest{
			Model: "gemini-2.0-flash",
			Messages: []chatMessage{
				{Role: roleUser, Content: "hello"},
			},
			ResponseFormat: map[string]interface{}{
				"type":        "json_schema",
				"json_schema": "invalid",
			},
		}

		vertexReq, err := v.buildVertexChatRequest(req)
		require.NoError(t, err)
		require.NotNil(t, vertexReq)
		assert.Empty(t, vertexReq.GenerationConfig.ResponseMimeType)
		assert.Nil(t, vertexReq.GenerationConfig.ResponseSchema)
	})

	t.Run("gemini 2.0 json_object is ignored", func(t *testing.T) {
		v := &vertexProvider{}
		req := &chatCompletionRequest{
			Model: "gemini-2.0-flash",
			Messages: []chatMessage{
				{Role: roleUser, Content: "hello"},
			},
			ResponseFormat: map[string]interface{}{
				"type": "json_object",
			},
		}

		vertexReq, err := v.buildVertexChatRequest(req)
		require.NoError(t, err)
		require.NotNil(t, vertexReq)
		assert.Empty(t, vertexReq.GenerationConfig.ResponseMimeType)
		assert.Nil(t, vertexReq.GenerationConfig.ResponseSchema)
	})
}

func TestVertexProviderApplyResponseFormatNilSafety(t *testing.T) {
	v := &vertexProvider{}
	require.NoError(t, v.applyResponseFormatToGenerationConfig(map[string]interface{}{"type": "json_object"}, nil, "gemini-2.5-flash"))
	require.NoError(t, v.applyResponseFormatToGenerationConfig(nil, &vertexChatGenerationConfig{}, "gemini-2.5-flash"))
	require.NoError(t, v.applyResponseFormatToGenerationConfig(map[string]interface{}{}, &vertexChatGenerationConfig{}, "gemini-2.5-flash"))
}

// newAnthropicVertexProvider builds a vertexProvider with project/region/modelMapping
// suitable for exercising onAnthropicMessagesRequestBody without OAuth or wasm runtime.
func newAnthropicVertexProvider(openAICompat bool) *vertexProvider {
	cfg := ProviderConfig{
		vertexProjectId:        "test-proj",
		vertexRegion:           "us-east5",
		vertexOpenAICompatible: openAICompat,
		modelMapping: map[string]string{
			"claude-sonnet-4":   "claude-sonnet-4@20250514",
			"claude-sonnet-4-5": "claude-sonnet-4-5@20250929",
		},
	}
	return &vertexProvider{config: cfg}
}

// TestVertexAnthropicPassthrough_BuiltinTool_TypePreserved is the core regression test
// for the original bug: builtin Anthropic tools (e.g. web_search_20250305) carry only
// a `type` discriminator and no `name`. The previous Anthropic→OpenAI→Anthropic round
// trip lost the type field, producing `tools.0.custom.name: String should have at least
// 1 character` from vertex. After the fix, the body is passthrough — `type` survives.
func TestVertexAnthropicPassthrough_BuiltinTool_TypePreserved(t *testing.T) {
	v := newAnthropicVertexProvider(false)
	ctx := newMapCtx()
	headers := http.Header{}
	body := []byte(`{
		"model": "claude-sonnet-4",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "search the web"}],
		"tools": [
			{"type": "web_search_20250305"},
			{"type": "bash_20250124"},
			{"type": "text_editor_20250124"}
		]
	}`)

	out, err := v.onAnthropicMessagesRequestBody(ctx, body, headers)
	require.NoError(t, err)

	// Path: non-stream → :rawPredict, model fully-qualified via modelMapping.
	assert.Equal(t,
		"/v1/projects/test-proj/locations/us-east5/publishers/anthropic/models/claude-sonnet-4@20250514:rawPredict",
		headers.Get(":path"))

	// Body: model stripped, anthropic_version injected.
	assert.False(t, gjson.GetBytes(out, "model").Exists(), "model must be stripped (vertex :rawPredict rejects it)")
	assert.Equal(t, vertexAnthropicVersion, gjson.GetBytes(out, "anthropic_version").String())

	// The bug-defining assertion: builtin tool `type` survives verbatim, and we did
	// NOT manufacture a `name` for it. If a future change re-introduces the lossy
	// conversion, the type will disappear or a synthetic name will appear and this
	// test will fail.
	tools := gjson.GetBytes(out, "tools").Array()
	require.Len(t, tools, 3)
	assert.Equal(t, "web_search_20250305", tools[0].Get("type").String())
	assert.False(t, tools[0].Get("name").Exists(), "builtin tool must not have a synthetic name")
	assert.Equal(t, "bash_20250124", tools[1].Get("type").String())
	assert.Equal(t, "text_editor_20250124", tools[2].Get("type").String())
}

// TestVertexAnthropicPassthrough_StreamPath verifies stream=true routes to
// :streamRawPredict and stream=false routes to :rawPredict.
func TestVertexAnthropicPassthrough_StreamPath(t *testing.T) {
	t.Run("stream true → streamRawPredict", func(t *testing.T) {
		v := newAnthropicVertexProvider(false)
		ctx := newMapCtx()
		headers := http.Header{}
		body := []byte(`{"model":"claude-sonnet-4","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)

		_, err := v.onAnthropicMessagesRequestBody(ctx, body, headers)
		require.NoError(t, err)

		assert.Equal(t,
			"/v1/projects/test-proj/locations/us-east5/publishers/anthropic/models/claude-sonnet-4@20250514:streamRawPredict",
			headers.Get(":path"))
	})

	t.Run("stream false → rawPredict", func(t *testing.T) {
		v := newAnthropicVertexProvider(false)
		ctx := newMapCtx()
		headers := http.Header{}
		body := []byte(`{"model":"claude-sonnet-4","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

		_, err := v.onAnthropicMessagesRequestBody(ctx, body, headers)
		require.NoError(t, err)

		assert.Equal(t,
			"/v1/projects/test-proj/locations/us-east5/publishers/anthropic/models/claude-sonnet-4@20250514:rawPredict",
			headers.Get(":path"))
	})
}

// TestVertexAnthropicPassthrough_ModelMappingUnconfigured verifies that when no
// mapping entry matches, the model name is left untouched (vertex will 404 — we
// don't second-guess the user's config here).
func TestVertexAnthropicPassthrough_ModelMappingUnconfigured(t *testing.T) {
	v := &vertexProvider{config: ProviderConfig{
		vertexProjectId: "test-proj",
		vertexRegion:    "us-east5",
		// no modelMapping configured
	}}
	ctx := newMapCtx()
	headers := http.Header{}
	body := []byte(`{"model":"claude-sonnet-4","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

	_, err := v.onAnthropicMessagesRequestBody(ctx, body, headers)
	require.NoError(t, err)

	// model name passes through as-is (no @date suffix)
	assert.Equal(t,
		"/v1/projects/test-proj/locations/us-east5/publishers/anthropic/models/claude-sonnet-4:rawPredict",
		headers.Get(":path"))
}

// TestVertexAnthropicPassthrough_CustomToolFieldsPreserved verifies that
// custom tool fields not in the OpenAI schema (cache_control, thinking config,
// arbitrary input_schema shapes) survive the passthrough — they were silently
// dropped by the old double-conversion path.
func TestVertexAnthropicPassthrough_CustomToolFieldsPreserved(t *testing.T) {
	v := newAnthropicVertexProvider(false)
	ctx := newMapCtx()
	headers := http.Header{}
	body := []byte(`{
		"model": "claude-sonnet-4",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "list files"}],
		"tools": [{
			"name": "Bash",
			"description": "run a shell command",
			"input_schema": {
				"type": "object",
				"properties": {"command": {"type": "string"}},
				"required": ["command"]
			},
			"cache_control": {"type": "ephemeral"}
		}],
		"thinking": {"type": "enabled", "budget_tokens": 1024}
	}`)

	out, err := v.onAnthropicMessagesRequestBody(ctx, body, headers)
	require.NoError(t, err)

	tool := gjson.GetBytes(out, "tools.0")
	assert.Equal(t, "Bash", tool.Get("name").String())
	assert.Equal(t, "ephemeral", tool.Get("cache_control.type").String(), "cache_control must survive passthrough")
	assert.Equal(t, "object", tool.Get("input_schema.type").String())
	assert.Equal(t, "command", tool.Get("input_schema.required.0").String())

	thinking := gjson.GetBytes(out, "thinking")
	assert.Equal(t, "enabled", thinking.Get("type").String(), "thinking config must survive passthrough")
	assert.Equal(t, int64(1024), thinking.Get("budget_tokens").Int())
}

// TestVertexAnthropicPassthrough_OpenAICompatibleConfigDoesNotInterfere verifies
// the contract from the plan: vertexOpenAICompatible: true affects ONLY
// chat/completions; /v1/messages still goes to the Anthropic native endpoint.
// (We exercise the handler that TransformRequestBodyHeaders dispatches to;
// see OnRequestBody:302 for the bypass condition itself.)
func TestVertexAnthropicPassthrough_OpenAICompatibleConfigDoesNotInterfere(t *testing.T) {
	v := newAnthropicVertexProvider(true) // vertexOpenAICompatible: true
	ctx := newMapCtx()
	headers := http.Header{}
	body := []byte(`{"model":"claude-sonnet-4","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

	out, err := v.onAnthropicMessagesRequestBody(ctx, body, headers)
	require.NoError(t, err)

	// Must be Anthropic native path, NOT /v1beta1/.../openai/chat/completions.
	assert.Contains(t, headers.Get(":path"), "publishers/anthropic/models/")
	assert.Contains(t, headers.Get(":path"), ":rawPredict")
	assert.NotContains(t, headers.Get(":path"), "/openai/")
	assert.Equal(t, vertexAnthropicVersion, gjson.GetBytes(out, "anthropic_version").String())
}

// TestVertexAnthropicPassthrough_DispatchedFromTransformRequestBodyHeaders covers
// the wiring step: TransformRequestBodyHeaders sees ApiNameAnthropicMessages and
// routes to the passthrough handler. Guards against accidental removal of the
// case branch.
func TestVertexAnthropicPassthrough_DispatchedFromTransformRequestBodyHeaders(t *testing.T) {
	v := newAnthropicVertexProvider(false)
	ctx := newMapCtx()
	headers := http.Header{}
	body := []byte(`{"model":"claude-sonnet-4","max_tokens":16,"messages":[{"role":"user","content":"hi"}],"tools":[{"type":"web_search_20250305"}]}`)

	out, err := v.TransformRequestBodyHeaders(ctx, ApiNameAnthropicMessages, body, headers)
	require.NoError(t, err)

	assert.Contains(t, headers.Get(":path"), ":rawPredict")
	assert.Equal(t, "web_search_20250305", gjson.GetBytes(out, "tools.0.type").String())
	assert.False(t, gjson.GetBytes(out, "model").Exists())
}

// TestVertexAnthropicPassthrough_ResponseBodyUnchanged verifies the non-stream
// response branch: TransformResponseBody returns the body verbatim for
// ApiNameAnthropicMessages, so vertex's native Anthropic JSON reaches the client
// without OpenAI→Anthropic re-translation.
func TestVertexAnthropicPassthrough_ResponseBodyUnchanged(t *testing.T) {
	v := newAnthropicVertexProvider(false)
	ctx := newMapCtx()
	body := []byte(`{"id":"msg_01","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],"model":"claude-sonnet-4@20250514","stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`)

	out, err := v.TransformResponseBody(ctx, ApiNameAnthropicMessages, body)
	require.NoError(t, err)
	assert.Equal(t, body, out, "vertex Anthropic response must be returned byte-for-byte")
}

// TestVertexAnthropicPassthrough_StreamingChunkUnchanged verifies the streaming
// counterpart: each SSE chunk is forwarded verbatim because vertex's
// :streamRawPredict already emits standard Anthropic SSE events.
func TestVertexAnthropicPassthrough_StreamingChunkUnchanged(t *testing.T) {
	v := newAnthropicVertexProvider(false)
	ctx := newMapCtx()
	chunk := []byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_01\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-sonnet-4@20250514\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n")

	out, err := v.OnStreamingResponseBody(ctx, ApiNameAnthropicMessages, chunk, false)
	require.NoError(t, err)
	assert.Equal(t, chunk, out, "vertex Anthropic SSE chunk must be returned byte-for-byte")
}

// TestVertexTransformRequestHeaders_StripsAnthropicHeaders ensures that
// Anthropic-specific headers (credentials + protocol) are NOT forwarded to
// vertex. The regular Anthropic SDK sends these but vertex's Anthropic
// endpoint rejects or misinterprets them:
//   - x-api-key / anthropic-api-key: credential leak to Google logs
//   - anthropic-beta: vertex 400 "Unexpected value(s) ... for the anthropic-beta header"
//   - anthropic-version: conflicts with body-level anthropic_version "vertex-2023-10-16"
func TestVertexTransformRequestHeaders_StripsAnthropicHeaders(t *testing.T) {
	v := newAnthropicVertexProvider(false)
	ctx := newMapCtx()
	headers := http.Header{}
	headers.Set("x-api-key", "sk-ant-api03-secret")
	headers.Set("anthropic-api-key", "sk-ant-api03-secret")
	headers.Set("anthropic-beta", "advanced-tool-use-2025-11-20,prompt-caching-scope-2026-01-05")
	headers.Set("anthropic-version", "2023-06-01")
	headers.Set("content-type", "application/json")

	v.TransformRequestHeaders(ctx, ApiNameAnthropicMessages, headers)

	assert.Empty(t, headers.Get("x-api-key"), "x-api-key must be stripped before forwarding to vertex")
	assert.Empty(t, headers.Get("anthropic-api-key"), "anthropic-api-key must be stripped before forwarding to vertex")
	assert.Empty(t, headers.Get("anthropic-beta"), "anthropic-beta must be stripped — vertex rejects unknown beta flags with 400")
	assert.Empty(t, headers.Get("anthropic-version"), "anthropic-version must be stripped — vertex uses body-level anthropic_version instead")
	// Sanity: unrelated headers untouched.
	assert.Equal(t, "application/json", headers.Get("content-type"))
}

// TestVertexAnthropicPassthrough_MaxTokensDefault ensures that when the client
// omits max_tokens, the passthrough handler injects claudeDefaultMaxTokens.
// Vertex's Anthropic endpoint rejects requests without max_tokens with a 400 —
// some SDKs (and lenient clients) leave it unset, expecting the upstream to
// default. Matches buildClaudeTextGenRequest's behavior in claude.go.
func TestVertexAnthropicPassthrough_MaxTokensDefault(t *testing.T) {
	t.Run("missing max_tokens gets defaulted", func(t *testing.T) {
		v := newAnthropicVertexProvider(false)
		ctx := newMapCtx()
		headers := http.Header{}
		body := []byte(`{
			"model": "claude-sonnet-4",
			"messages": [{"role": "user", "content": "hi"}]
		}`)

		out, err := v.onAnthropicMessagesRequestBody(ctx, body, headers)
		require.NoError(t, err)

		assert.Equal(t, int64(claudeDefaultMaxTokens), gjson.GetBytes(out, "max_tokens").Int())
	})

	t.Run("client-supplied max_tokens preserved", func(t *testing.T) {
		v := newAnthropicVertexProvider(false)
		ctx := newMapCtx()
		headers := http.Header{}
		body := []byte(`{
			"model": "claude-sonnet-4",
			"max_tokens": 1024,
			"messages": [{"role": "user", "content": "hi"}]
		}`)

		out, err := v.onAnthropicMessagesRequestBody(ctx, body, headers)
		require.NoError(t, err)

		assert.Equal(t, int64(1024), gjson.GetBytes(out, "max_tokens").Int(),
			"client-supplied max_tokens must not be overwritten by the default")
	})
}
