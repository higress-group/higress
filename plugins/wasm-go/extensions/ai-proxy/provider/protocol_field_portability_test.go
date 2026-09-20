package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file covers the request fields that used to disappear when ai-proxy
// bridged a request between the OpenAI, Claude and Gemini protocols: the payload
// is either forwarded in the target protocol's own shape or reported through the
// gateway log instead of being dropped without a trace.

func newStandardClaudeProvider() *claudeProvider {
	return &claudeProvider{
		config: ProviderConfig{
			claudeCodeMode: false,
		},
	}
}

func TestOpenAIToClaudeKeepsStrictOnTools(t *testing.T) {
	provider := newStandardClaudeProvider()
	request := &chatCompletionRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 8192,
		Messages:  []chatMessage{{Role: roleUser, Content: "Book a flight to Lisbon."}},
		Tools: []tool{{
			Type: "function",
			Function: function{
				Name:        "book_flight",
				Description: "Books a flight.",
				Parameters:  map[string]interface{}{"type": "object"},
				Strict:      true,
			},
		}},
	}

	claudeReq := provider.buildClaudeTextGenRequest(request)

	require.Len(t, claudeReq.Tools, 1)
	assert.True(t, claudeReq.Tools[0].Strict, "OpenAI's strict flag must survive the conversion")

	body, err := json.Marshal(claudeReq)
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &decoded))
	tools, ok := decoded["tools"].([]interface{})
	require.True(t, ok)
	toolSpec, ok := tools[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, toolSpec["strict"])
}

func TestClaudeToOpenAIKeepsStrictOnTools(t *testing.T) {
	converter := &ClaudeToOpenAIConverter{}
	claudeRequest := `{
		"model": "claude-sonnet-4-5-20250929",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "Book a flight to Lisbon."}],
		"tools": [{
			"name": "book_flight",
			"description": "Books a flight.",
			"input_schema": {"type": "object"},
			"strict": true
		}]
	}`

	result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
	require.NoError(t, err)

	var openaiRequest chatCompletionRequest
	require.NoError(t, json.Unmarshal(result, &openaiRequest))
	require.Len(t, openaiRequest.Tools, 1)
	assert.True(t, openaiRequest.Tools[0].Function.Strict)
}

func TestOpenAIFileDataBecomesClaudeDocumentBlock(t *testing.T) {
	provider := newStandardClaudeProvider()
	request := &chatCompletionRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 8192,
		Messages: []chatMessage{{
			Role: roleUser,
			Content: []any{map[string]any{
				"type": "file",
				"file": map[string]any{
					"file_data": "data:application/pdf;base64,JVBERi0xLjQK",
					"filename":  "invoice.pdf",
				},
			}},
		}},
	}

	claudeReq := provider.buildClaudeTextGenRequest(request)

	require.Len(t, claudeReq.Messages, 1)
	blocks := claudeReq.Messages[0].Content.GetArrayValue()
	require.Len(t, blocks, 1)
	assert.Equal(t, "document", blocks[0].Type)
	require.NotNil(t, blocks[0].Source)
	assert.Equal(t, "base64", blocks[0].Source.Type)
	assert.Equal(t, "application/pdf", blocks[0].Source.MediaType)
	assert.Equal(t, "JVBERi0xLjQK", blocks[0].Source.Data)
}

func TestOpenAIFileIdBecomesClaudeFileSource(t *testing.T) {
	provider := newStandardClaudeProvider()
	request := &chatCompletionRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 8192,
		Messages: []chatMessage{{
			Role: roleUser,
			Content: []any{map[string]any{
				"type": "file",
				"file": map[string]any{"file_id": "file-abc123"},
			}},
		}},
	}

	claudeReq := provider.buildClaudeTextGenRequest(request)

	require.Len(t, claudeReq.Messages, 1)
	blocks := claudeReq.Messages[0].Content.GetArrayValue()
	require.Len(t, blocks, 1)
	assert.Equal(t, "document", blocks[0].Type)
	require.NotNil(t, blocks[0].Source)
	assert.Equal(t, "file", blocks[0].Source.Type)
	assert.Equal(t, "file-abc123", blocks[0].Source.FileId)
	assert.Empty(t, blocks[0].Source.Url, "a file id must not be declared as a url source")

	sourceBody, err := json.Marshal(blocks[0].Source)
	require.NoError(t, err)
	assert.NotContains(t, string(sourceBody), `"url"`)
}

func TestOpenAIInputAudioDoesNotEmptyClaudeContent(t *testing.T) {
	provider := newStandardClaudeProvider()
	request := &chatCompletionRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 8192,
		Messages: []chatMessage{{
			Role: roleUser,
			Content: []any{map[string]any{
				"type": "input_audio",
				"input_audio": map[string]any{
					"data":   "UklGRg==",
					"format": "wav",
				},
			}},
		}},
	}

	claudeReq := provider.buildClaudeTextGenRequest(request)

	require.Len(t, claudeReq.Messages, 1)
	content := claudeReq.Messages[0].Content
	assert.True(t, content.IsString, "Claude rejects an empty content array")
	assert.Equal(t, "", content.StringValue)

	body, err := json.Marshal(claudeReq)
	require.NoError(t, err)
	assert.NotContains(t, string(body), `"content":[]`)
}

func TestDeveloperRoleIsOnlyConvertedForTargetsThatRejectIt(t *testing.T) {
	assert.True(t, isDeveloperRoleSupported(providerTypeOpenAI), "the OpenAI target accepts the developer role as-is")
	assert.True(t, isDeveloperRoleSupported(providerTypeAzure))
	assert.False(t, isDeveloperRoleSupported(providerTypeClaude))

	converted, err := convertDeveloperRoleToSystem([]byte(`{"model":"gpt-4o","messages":[{"role":"developer","content":"Always answer in Chinese."}]}`))
	require.NoError(t, err)
	assert.Contains(t, string(converted), `"role":"system"`)
}

func TestSystemOnlyClaudeRequestSerializesEmptyMessagesArray(t *testing.T) {
	provider := newStandardClaudeProvider()
	request := &chatCompletionRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 8192,
		// This is what the bridge produces for a request whose only turn was a
		// `developer` message: the role is downgraded to `system` on the way in.
		Messages: []chatMessage{{Role: roleSystem, Content: "Always answer in Chinese."}},
	}

	claudeReq := provider.buildClaudeTextGenRequest(request)

	require.NotNil(t, claudeReq.Messages, "messages must never be null on the wire")
	assert.Empty(t, claudeReq.Messages)
	require.NotNil(t, claudeReq.System)

	body, err := json.Marshal(claudeReq)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"messages":[]`)
	assert.NotContains(t, string(body), `"messages":null`)
}

func TestAllowedToolsToolChoiceNarrowsClaudeTools(t *testing.T) {
	provider := newStandardClaudeProvider()
	request := &chatCompletionRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 8192,
		Messages:  []chatMessage{{Role: roleUser, Content: "Help me with my account."}},
		Tools: []tool{
			{Type: "function", Function: function{Name: "web_search", Parameters: map[string]interface{}{"type": "object"}}},
			{Type: "function", Function: function{Name: "delete_account", Parameters: map[string]interface{}{"type": "object"}}},
		},
		ToolChoice: map[string]interface{}{
			"type": "allowed_tools",
			"allowed_tools": map[string]interface{}{
				"mode": "required",
				"tools": []interface{}{
					map[string]interface{}{
						"type":     "function",
						"function": map[string]interface{}{"name": "web_search"},
					},
				},
			},
		},
	}

	claudeReq := provider.buildClaudeTextGenRequest(request)

	require.Len(t, claudeReq.Tools, 1)
	assert.Equal(t, "web_search", claudeReq.Tools[0].Name)
	require.NotNil(t, claudeReq.ToolChoice)
	assert.Equal(t, "any", claudeReq.ToolChoice.Type)
	assert.Empty(t, claudeReq.ToolChoice.Name)
}

func TestGeminiStrictToolsUseValidatedCallingMode(t *testing.T) {
	provider := &geminiProvider{}
	request := &chatCompletionRequest{
		Model:     "gemini-2.0-flash",
		MaxTokens: 8192,
		Messages:  []chatMessage{{Role: roleUser, Content: "Book a flight to Lisbon."}},
		Tools: []tool{{
			Type: "function",
			Function: function{
				Name:        "book_flight",
				Description: "Books a flight.",
				Parameters:  map[string]interface{}{"type": "object"},
				Strict:      true,
			},
		}},
	}

	geminiReq := provider.buildGeminiChatRequest(request)

	require.NotNil(t, geminiReq.ToolConfig)
	require.NotNil(t, geminiReq.ToolConfig.FunctionCallingConfig)
	assert.Equal(t, geminiFunctionCallingModeValidated, geminiReq.ToolConfig.FunctionCallingConfig.Mode)

	body, err := json.Marshal(geminiReq)
	require.NoError(t, err)
	// Google's schema rejects unknown members, so `strict` is translated into the
	// calling mode instead of being forwarded inside the function declaration.
	assert.NotContains(t, string(body), `"strict"`)
}

func TestGeminiNamedToolChoiceKeepsToolConfig(t *testing.T) {
	provider := &geminiProvider{}
	parallelToolCalls := false
	request := &chatCompletionRequest{
		Model:             "gemini-2.0-flash",
		MaxTokens:         8192,
		Messages:          []chatMessage{{Role: roleUser, Content: "Search for hammerhead sharks."}},
		Tools:             []tool{{Type: "function", Function: function{Name: "web_search", Parameters: map[string]interface{}{"type": "object"}}}},
		ToolChoice:        map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "web_search"}},
		ParallelToolCalls: &parallelToolCalls,
	}

	geminiReq := provider.buildGeminiChatRequest(request)

	require.NotNil(t, geminiReq.ToolConfig, "a named tool choice must not vanish from the Gemini request")
	require.NotNil(t, geminiReq.ToolConfig.FunctionCallingConfig)
	assert.Equal(t, geminiFunctionCallingModeAny, geminiReq.ToolConfig.FunctionCallingConfig.Mode)
	assert.Equal(t, []string{"web_search"}, geminiReq.ToolConfig.FunctionCallingConfig.AllowedFunctionNames)
}

func TestGeminiFilePartsBecomeInlineData(t *testing.T) {
	provider := &geminiProvider{}
	request := &chatCompletionRequest{
		Model:     "gemini-2.0-flash",
		MaxTokens: 8192,
		Messages: []chatMessage{{
			Role: roleUser,
			Content: []any{map[string]any{
				"type": "file",
				"file": map[string]any{"file_data": "data:application/pdf;base64,JVBERi0xLjQK"},
			}},
		}},
	}

	geminiReq := provider.buildGeminiChatRequest(request)

	require.Len(t, geminiReq.Contents, 1)
	require.Len(t, geminiReq.Contents[0].Parts, 1)
	require.NotNil(t, geminiReq.Contents[0].Parts[0].InlineData)
	assert.Equal(t, "application/pdf", geminiReq.Contents[0].Parts[0].InlineData.MimeType)
	assert.Equal(t, "JVBERi0xLjQK", geminiReq.Contents[0].Parts[0].InlineData.Data)
}

func TestGeminiFileIdPartIsNotForwardedAsEmptyParts(t *testing.T) {
	provider := &geminiProvider{}
	request := &chatCompletionRequest{
		Model:     "gemini-2.0-flash",
		MaxTokens: 8192,
		Messages: []chatMessage{{
			Role: roleUser,
			Content: []any{map[string]any{
				"type": "file",
				"file": map[string]any{"file_id": "file-abc123"},
			}},
		}},
	}

	geminiReq := provider.buildGeminiChatRequest(request)

	// Gemini rejects an empty parts array, so the untranslatable turn is reported
	// and skipped instead of being sent as {"role":"user","parts":[]}.
	assert.Empty(t, geminiReq.Contents)
	body, err := json.Marshal(geminiReq)
	require.NoError(t, err)
	assert.NotContains(t, string(body), `"parts":[]`)
}

func TestClaudeDocumentBlockBecomesOpenAIFilePart(t *testing.T) {
	converter := &ClaudeToOpenAIConverter{}
	claudeRequest := `{
		"model": "claude-sonnet-4-5-20250929",
		"max_tokens": 1024,
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "Summarize this invoice."},
				{"type": "document", "source": {"type": "base64", "media_type": "application/pdf", "data": "JVBERi0xLjQK"}}
			]
		}]
	}`

	result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
	require.NoError(t, err)

	var openaiRequest chatCompletionRequest
	require.NoError(t, json.Unmarshal(result, &openaiRequest))
	require.Len(t, openaiRequest.Messages, 1)

	contents, ok := openaiRequest.Messages[0].Content.([]interface{})
	require.True(t, ok, "the document must be forwarded as a content part")
	require.Len(t, contents, 2)
	firstPart, ok := contents[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, contentTypeText, firstPart["type"])
	filePart, ok := contents[1].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, contentTypeFile, filePart["type"])
	file, ok := filePart["file"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "data:application/pdf;base64,JVBERi0xLjQK", file["file_data"])
}

func TestClaudeDocumentUrlKeepsOpenAIContentKey(t *testing.T) {
	converter := &ClaudeToOpenAIConverter{}
	claudeRequest := `{
		"model": "claude-sonnet-4-5-20250929",
		"max_tokens": 1024,
		"messages": [{
			"role": "user",
			"content": [{"type": "document", "source": {"type": "url", "url": "https://example.com/invoice.pdf"}}]
		}]
	}`

	result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(result, &raw))
	messages, ok := raw["messages"].([]interface{})
	require.True(t, ok)
	require.Len(t, messages, 1)
	message, ok := messages[0].(map[string]interface{})
	require.True(t, ok)
	require.Contains(t, message, "content", "OpenAI rejects messages without a content key")
	assert.Equal(t, "", message["content"])
}

func TestParseContentReadsEveryFileField(t *testing.T) {
	message := &chatMessage{
		Role: roleUser,
		Content: []any{
			map[string]any{
				"type": "file",
				"file": map[string]any{
					"file_data": "data:application/pdf;base64,JVBERi0xLjQK",
					"file_name": "invoice.pdf",
				},
			},
			map[string]any{
				"type": "file",
				"file": map[string]any{"file_id": "file-abc123"},
			},
		},
	}

	parts := message.ParseContent()

	require.Len(t, parts, 2)
	require.NotNil(t, parts[0].File)
	assert.Equal(t, "data:application/pdf;base64,JVBERi0xLjQK", parts[0].File.FileData)
	assert.Equal(t, "invoice.pdf", parts[0].File.FileName)
	assert.Empty(t, parts[0].File.FileId)
	require.NotNil(t, parts[1].File)
	assert.Equal(t, "file-abc123", parts[1].File.FileId)
}

func TestAllowedToolNamesFromToolChoice(t *testing.T) {
	t.Run("full_function_reference", func(t *testing.T) {
		request := &chatCompletionRequest{
			ToolChoice: map[string]interface{}{
				"type": "allowed_tools",
				"allowed_tools": map[string]interface{}{
					"mode": "auto",
					"tools": []interface{}{
						map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "web_search"}},
						map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "book_flight"}},
					},
				},
			},
		}

		assert.Equal(t, []string{"web_search", "book_flight"}, request.getAllowedToolNames())
		assert.Equal(t, "allowed_tools", request.getToolChoiceType())
	})

	t.Run("shorthand_name", func(t *testing.T) {
		request := &chatCompletionRequest{
			ToolChoice: map[string]interface{}{
				"type": "allowed_tools",
				"allowed_tools": map[string]interface{}{
					"tools": []interface{}{map[string]interface{}{"name": "web_search"}},
				},
			},
		}

		assert.Equal(t, []string{"web_search"}, request.getAllowedToolNames())
	})

	t.Run("other_shapes", func(t *testing.T) {
		assert.Nil(t, (&chatCompletionRequest{ToolChoice: "auto"}).getAllowedToolNames())
		assert.Nil(t, (&chatCompletionRequest{ToolChoice: map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "web_search"}}}).getAllowedToolNames())
		assert.Nil(t, (&chatCompletionRequest{}).getAllowedToolNames())
	})
}

func TestSplitDataURL(t *testing.T) {
	mediaType, data, ok := splitDataURL("data:application/pdf;base64,JVBERi0xLjQK")
	require.True(t, ok)
	assert.Equal(t, "application/pdf", mediaType)
	assert.Equal(t, "JVBERi0xLjQK", data)

	_, _, ok = splitDataURL("JVBERi0xLjQK")
	assert.False(t, ok)

	_, _, ok = splitDataURL("data:application/pdf")
	assert.False(t, ok)
}

func TestMediaTypeFromFileName(t *testing.T) {
	assert.Equal(t, "application/pdf", mediaTypeFromFileName("invoice.PDF"))
	assert.Equal(t, "image/png", mediaTypeFromFileName("chart.png"))
	assert.Empty(t, mediaTypeFromFileName("archive"))
	assert.Empty(t, mediaTypeFromFileName("archive.zzz"))
	assert.Empty(t, mediaTypeFromFileName(""))
}

func TestFilePartMediaType(t *testing.T) {
	mediaType, data, isInline := filePartMediaType(&chatMessageContentFile{
		FileData: "data:text/plain;base64,aGVsbG8=",
		FileName: "ignored.pdf",
	})
	require.True(t, isInline)
	assert.Equal(t, "text/plain", mediaType)
	assert.Equal(t, "aGVsbG8=", data)

	mediaType, data, isInline = filePartMediaType(&chatMessageContentFile{FileData: "aGVsbG8=", FileName: "note.txt"})
	require.True(t, isInline)
	assert.Equal(t, "text/plain", mediaType)
	assert.Equal(t, "aGVsbG8=", data)

	_, _, isInline = filePartMediaType(&chatMessageContentFile{FileId: "file-abc123"})
	assert.False(t, isInline)

	_, _, isInline = filePartMediaType(nil)
	assert.False(t, isInline)
}
