package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVertexParallelToolsResponse(t *testing.T) {
	v := &vertexProvider{}
	body, err := v.TransformResponseBody(newMockMultipartHttpContext(), ApiNameChatCompletion, []byte(`{
		"responseId":"response-parallel",
		"candidates":[
			{"index":0,"finishReason":"STOP","content":{"role":"model","parts":[
				{"text":"Checking. "},
				{"functionCall":{"name":"weather","args":{"city":"Hangzhou"}},"thoughtSignature":"signature-weather"},
				{"functionCall":{"name":"time","args":{"zone":"Asia/Shanghai"}},"thoughtSignature":"signature-time"},
				{"text":"Please wait."}
			]}},
			{"index":1,"finishReason":"STOP","content":{"role":"model","parts":[
				{"functionCall":{"name":"weather","args":{"city":"Shanghai"}}}
			]}}
		],
		"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":20,"totalTokenCount":30}
	}`))
	require.NoError(t, err)
	var response chatCompletionResponse
	require.NoError(t, json.Unmarshal(body, &response))
	require.Len(t, response.Choices, 2)
	choice := response.Choices[0]
	assert.Equal(t, "Checking. Please wait.", choice.Message.Content)
	require.Len(t, choice.Message.ToolCalls, 2)
	first, second := choice.Message.ToolCalls[0], choice.Message.ToolCalls[1]
	assert.Equal(t, "weather", first.Function.Name)
	assert.JSONEq(t, `{"city":"Hangzhou"}`, first.Function.Arguments)
	assert.Equal(t, "time", second.Function.Name)
	assert.JSONEq(t, `{"zone":"Asia/Shanghai"}`, second.Function.Arguments)
	assert.NotEmpty(t, first.Id)
	assert.NotEqual(t, first.Id, second.Id)
	assert.Equal(t, "signature-weather", first.getThoughtSignature())
	assert.Equal(t, "signature-time", second.getThoughtSignature())
	assert.Equal(t, "signature-time", getNestedString(second.ExtraContent, "google", "thought_signature"))
	require.NotNil(t, choice.FinishReason)
	assert.Equal(t, finishReasonToolCall, *choice.FinishReason)
	assert.Equal(t, 1, response.Choices[1].Index)
	require.Len(t, response.Choices[1].Message.ToolCalls, 1)
	assert.NotEqual(t, first.Id, response.Choices[1].Message.ToolCalls[0].Id)
	assert.Equal(t, 30, response.Usage.TotalTokens)
}

func TestVertexParallelToolsResponseClosesThinkingBeforeCall(t *testing.T) {
	v := &vertexProvider{}
	body, err := v.TransformResponseBody(newMockMultipartHttpContext(), ApiNameChatCompletion, []byte(`{
		"candidates":[{"index":0,"finishReason":"STOP","content":{"parts":[
			{"thought":true,"text":"reason"},
			{"functionCall":{"name":"weather","args":{}}},
			{"functionCall":{"name":"time","args":{}}}
		]}}]
	}`))
	require.NoError(t, err)
	var response chatCompletionResponse
	require.NoError(t, json.Unmarshal(body, &response))
	require.Len(t, response.Choices, 1)
	assert.Equal(t, reasoningStartTag+"reason"+reasoningEndTag, response.Choices[0].Message.Content)
	require.Len(t, response.Choices[0].Message.ToolCalls, 2)
}

func TestVertexParallelToolsRequestSkipsEmptyText(t *testing.T) {
	for _, content := range []any{"", []any{map[string]any{"type": "text", "text": ""}}} {
		v := &vertexProvider{}
		converted, err := v.buildVertexChatRequest(&chatCompletionRequest{Messages: []chatMessage{
			{Role: roleAssistant, Content: content, ToolCalls: []toolCall{
				{Id: "first", Function: functionCall{Name: "weather", Arguments: `{"city":"Hangzhou"}`}},
				{Id: "second", Function: functionCall{Name: "time", Arguments: `{"zone":"Asia/Shanghai"}`}},
			}},
		}})
		require.NoError(t, err)
		require.Len(t, converted.Contents, 1)
		body, err := json.Marshal(converted.Contents[0])
		require.NoError(t, err)
		assert.JSONEq(t, `{"role":"model","parts":[{"functionCall":{"name":"weather","args":{"city":"Hangzhou"}}},{"functionCall":{"name":"time","args":{"zone":"Asia/Shanghai"}}}]}`, string(body))
		assert.NotContains(t, string(body), "{}", "empty text must not produce an empty native part")
	}
}

func TestVertexParallelToolsRequest(t *testing.T) {
	for _, sameName := range []bool{false, true} {
		name := "different_functions"
		secondName := "time"
		if sameName {
			name = "same_function_different_arguments"
			secondName = "weather"
		}
		t.Run(name, func(t *testing.T) {
			v := &vertexProvider{}
			request := &chatCompletionRequest{Messages: []chatMessage{
				{Role: roleUser, Content: "Compare these results."},
				{Role: roleAssistant, Content: "Checking.", ToolCalls: []toolCall{
					{Id: "call-first", Type: "function", ThoughtSignature: "first-signature", Function: functionCall{Name: "weather", Arguments: `{"city":"Hangzhou"}`}},
					{Id: "call-second", Type: "function", ExtraContent: map[string]interface{}{"google": map[string]interface{}{"thought_signature": "second-signature"}}, Function: functionCall{Name: secondName, Arguments: `{"city":"Shanghai"}`}},
				}},
				// Results can complete in a different order from their calls.
				{Role: roleTool, ToolCallId: "call-second", Content: "second-result"},
				{Role: roleTool, ToolCallId: "call-first", Content: "first-result"},
				{Role: roleAssistant, Content: "Both checked."},
				{Role: roleUser, Content: "Thanks."},
			}}
			converted, err := v.buildVertexChatRequest(request)
			require.NoError(t, err)
			require.Len(t, converted.Contents, 5)
			model := converted.Contents[1]
			assert.Equal(t, "model", model.Role)
			require.Len(t, model.Parts, 3)
			assert.Equal(t, "Checking.", model.Parts[0].Text)
			require.NotNil(t, model.Parts[1].FunctionCall)
			require.NotNil(t, model.Parts[2].FunctionCall)
			assert.Equal(t, "weather", model.Parts[1].FunctionCall.Name)
			assert.Equal(t, secondName, model.Parts[2].FunctionCall.Name)
			assert.Equal(t, "first-signature", model.Parts[1].ThoughtSignature)
			assert.Equal(t, "second-signature", model.Parts[2].ThoughtSignature)
			results := converted.Contents[2]
			assert.Equal(t, roleUser, results.Role)
			require.Len(t, results.Parts, 2)
			assert.Equal(t, "weather", results.Parts[0].FunctionResponse.Name)
			assert.Equal(t, "first-result", results.Parts[0].FunctionResponse.Response.Output)
			assert.Equal(t, secondName, results.Parts[1].FunctionResponse.Name)
			assert.Equal(t, "second-result", results.Parts[1].FunctionResponse.Response.Output)
			assert.Equal(t, "Both checked.", converted.Contents[3].Parts[0].Text)
			assert.Equal(t, "Thanks.", converted.Contents[4].Parts[0].Text)
		})
	}
}

func TestVertexParallelToolsResponsePreservesNonStopReason(t *testing.T) {
	v := &vertexProvider{}
	for _, reason := range []string{"MAX_TOKENS", "SAFETY"} {
		t.Run(reason, func(t *testing.T) {
			response := v.buildChatCompletionResponse(newMockMultipartHttpContext(), &vertexChatResponse{
				Candidates: []vertexChatCandidate{{
					FinishReason: reason,
					Content: vertexChatContent{Parts: []vertexPart{{
						FunctionCall: &vertexFunctionCall{Name: "weather", Args: map[string]interface{}{}},
					}}},
				}},
			})
			require.Len(t, response.Choices, 1)
			require.Len(t, response.Choices[0].Message.ToolCalls, 1)
			require.NotNil(t, response.Choices[0].FinishReason)
			assert.Equal(t, reason, *response.Choices[0].FinishReason)
		})
	}
}

func TestVertexParallelToolsRequestMultipleRounds(t *testing.T) {
	v := &vertexProvider{}
	converted, err := v.buildVertexChatRequest(&chatCompletionRequest{Messages: []chatMessage{
		{Role: roleAssistant, ToolCalls: []toolCall{
			{Id: "first-weather", Function: functionCall{Name: "weather", Arguments: `{}`}},
			{Id: "first-time", Function: functionCall{Name: "time", Arguments: `{}`}},
		}},
		{Role: roleTool, ToolCallId: "first-time", Content: "first time"},
		{Role: roleTool, ToolCallId: "first-weather", Content: "first weather"},
		{Role: roleAssistant, ToolCalls: []toolCall{
			{Id: "second-time", Function: functionCall{Name: "time", Arguments: `{}`}},
			{Id: "second-weather", Function: functionCall{Name: "weather", Arguments: `{}`}},
		}},
		{Role: roleTool, ToolCallId: "second-weather", Content: "second weather"},
		{Role: roleTool, ToolCallId: "second-time", Content: "second time"},
	}})
	require.NoError(t, err)
	require.Len(t, converted.Contents, 4)
	require.Len(t, converted.Contents[1].Parts, 2)
	assert.Equal(t, "weather", converted.Contents[1].Parts[0].FunctionResponse.Name)
	assert.Equal(t, "first weather", converted.Contents[1].Parts[0].FunctionResponse.Response.Output)
	assert.Equal(t, "first time", converted.Contents[1].Parts[1].FunctionResponse.Response.Output)
	require.Len(t, converted.Contents[3].Parts, 2)
	assert.Equal(t, "time", converted.Contents[3].Parts[0].FunctionResponse.Name)
	assert.Equal(t, "second time", converted.Contents[3].Parts[0].FunctionResponse.Response.Output)
	assert.Equal(t, "second weather", converted.Contents[3].Parts[1].FunctionResponse.Response.Output)
}

func TestVertexParallelToolsRequestRejectsDuplicateOrStaleResults(t *testing.T) {
	call := func(id, name string) chatMessage {
		return chatMessage{Role: roleAssistant, ToolCalls: []toolCall{{Id: id, Function: functionCall{Name: name, Arguments: `{}`}}}}
	}
	result := func(id string) chatMessage {
		return chatMessage{Role: roleTool, ToolCallId: id, Content: "result"}
	}
	for _, test := range []struct {
		name     string
		messages []chatMessage
	}{
		{"duplicate_result", []chatMessage{call("first", "weather"), result("first"), result("first")}},
		{"stale_result_after_next_call", []chatMessage{call("first", "weather"), result("first"), call("second", "time"), result("first")}},
		{"stale_result_after_user_turn", []chatMessage{call("first", "weather"), result("first"), {Role: roleUser, Content: "next"}, result("first")}},
		{"stale_id_does_not_match_idless_call", []chatMessage{call("first", "weather"), result("first"), call("", "time"), result("first")}},
		{"duplicate_call_ids", []chatMessage{{Role: roleAssistant, ToolCalls: []toolCall{
			{Id: "same", Function: functionCall{Name: "weather", Arguments: `{}`}},
			{Id: "same", Function: functionCall{Name: "time", Arguments: `{}`}},
		}}, result("same")}},
		{"missing_parallel_result", []chatMessage{{Role: roleAssistant, ToolCalls: []toolCall{
			{Id: "first", Function: functionCall{Name: "weather", Arguments: `{}`}},
			{Id: "second", Function: functionCall{Name: "time", Arguments: `{}`}},
		}}, result("first")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := &vertexProvider{}
			_, err := v.buildVertexChatRequest(&chatCompletionRequest{Messages: test.messages})
			require.Error(t, err)
		})
	}
}

func TestVertexParallelToolsRequestRejectsAmbiguousResults(t *testing.T) {
	for _, test := range []struct {
		name     string
		calls    []toolCall
		resultID string
	}{
		{"unknown_id", []toolCall{{Id: "known", Function: functionCall{Name: "weather", Arguments: `{}`}}}, "unknown"},
		{"missing_id_with_parallel_calls", []toolCall{{Id: "first", Function: functionCall{Name: "weather", Arguments: `{}`}}, {Id: "second", Function: functionCall{Name: "time", Arguments: `{}`}}}, ""},
		{"orphan_result", nil, "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := &vertexProvider{}
			_, err := v.buildVertexChatRequest(&chatCompletionRequest{Messages: []chatMessage{
				{Role: roleAssistant, ToolCalls: test.calls},
				{Role: roleTool, ToolCallId: test.resultID, Content: "result"},
			}})
			require.Error(t, err, "must not silently attribute a result to an unrelated call")
		})
	}
}

func TestVertexParallelToolsLegacySingleCallWithoutID(t *testing.T) {
	v := &vertexProvider{}
	converted, err := v.buildVertexChatRequest(&chatCompletionRequest{Messages: []chatMessage{
		{Role: roleAssistant, ToolCalls: []toolCall{{Function: functionCall{Name: "weather", Arguments: `{}`}}}},
		{Role: roleTool, Content: "sunny"},
	}})
	require.NoError(t, err)
	require.Len(t, converted.Contents, 2)
	require.Len(t, converted.Contents[1].Parts, 1)
	assert.Equal(t, "weather", converted.Contents[1].Parts[0].FunctionResponse.Name)
	assert.Equal(t, "sunny", converted.Contents[1].Parts[0].FunctionResponse.Response.Output)
}

func TestVertexParallelToolsStreamAcrossEvents(t *testing.T) {
	v := &vertexProvider{}
	ctx := newMockMultipartHttpContext()
	var first vertexChatResponse
	require.NoError(t, json.Unmarshal([]byte(`{"responseId":"stream-id","candidates":[
		{"index":0,"content":{"role":"model","parts":[
			{"text":"Checking. "},
			{"functionCall":{"name":"weather","args":{"city":"Hangzhou"}},"thoughtSignature":"sig-first"},
			{"functionCall":{"name":"time","args":{"zone":"Asia/Shanghai"}}}
		]}},
		{"index":1,"content":{"role":"model","parts":[{"functionCall":{"name":"other","args":{}}}]}}
	]}`), &first))
	chunk := v.buildChatCompletionStreamResponse(ctx, &first)
	require.Len(t, chunk.Choices, 2)
	require.Len(t, chunk.Choices[0].Delta.ToolCalls, 2)
	assert.Equal(t, "Checking. ", chunk.Choices[0].Delta.Content)
	firstCall, secondCall := chunk.Choices[0].Delta.ToolCalls[0], chunk.Choices[0].Delta.ToolCalls[1]
	assert.Equal(t, 0, firstCall.Index)
	assert.Equal(t, 1, secondCall.Index)
	assert.NotEqual(t, firstCall.Id, secondCall.Id)
	assert.Equal(t, "sig-first", firstCall.getThoughtSignature())
	assert.Empty(t, secondCall.getThoughtSignature(), "signature belongs only to its original part")
	assert.Nil(t, chunk.Choices[0].FinishReason)
	assert.Equal(t, 1, chunk.Choices[1].Index)
	require.Len(t, chunk.Choices[1].Delta.ToolCalls, 1)
	assert.Equal(t, 0, chunk.Choices[1].Delta.ToolCalls[0].Index)
	assert.NotEqual(t, firstCall.Id, chunk.Choices[1].Delta.ToolCalls[0].Id)

	// A complete functionCall in a later SSE event is another call, even when
	// its name and arguments equal a preceding call. Native parts have no index.
	var next vertexChatResponse
	require.NoError(t, json.Unmarshal([]byte(`{"responseId":"stream-id","candidates":[{"index":0,"content":{"role":"model","parts":[{"functionCall":{"name":"weather","args":{"city":"Hangzhou"}}}]}}]}`), &next))
	chunk = v.buildChatCompletionStreamResponse(ctx, &next)
	require.Len(t, chunk.Choices, 1)
	require.Len(t, chunk.Choices[0].Delta.ToolCalls, 1)
	assert.Equal(t, 2, chunk.Choices[0].Delta.ToolCalls[0].Index)
	assert.NotEqual(t, firstCall.Id, chunk.Choices[0].Delta.ToolCalls[0].Id)

	var terminal vertexChatResponse
	require.NoError(t, json.Unmarshal([]byte(`{"responseId":"stream-id","candidates":[{"index":0,"finishReason":"STOP"},{"index":1,"finishReason":"STOP"}],"usageMetadata":{"totalTokenCount":30}}`), &terminal))
	chunk = v.buildChatCompletionStreamResponse(ctx, &terminal)
	require.Len(t, chunk.Choices, 2)
	for _, choice := range chunk.Choices {
		require.NotNil(t, choice.FinishReason)
		assert.Equal(t, finishReasonToolCall, *choice.FinishReason)
		assert.Empty(t, choice.Delta.ToolCalls)
	}
	assert.Equal(t, 30, chunk.Usage.TotalTokens)
}

func TestVertexParallelToolsStreamClosesThinkingBeforeCall(t *testing.T) {
	v := &vertexProvider{}
	ctx := newMockMultipartHttpContext()
	for _, test := range []struct {
		body    string
		content string
		calls   int
	}{
		{`{"candidates":[{"index":0,"content":{"parts":[{"thought":true,"text":"reason"}]}}]}`, reasoningStartTag + "reason", 0},
		{`{"candidates":[{"index":0,"content":{"parts":[{"functionCall":{"name":"weather","args":{}}},{"functionCall":{"name":"time","args":{}}}]}}]}`, reasoningEndTag, 2},
		{`{"candidates":[{"index":0,"content":{"parts":[{"text":"Checking."}]}}]}`, "Checking.", 0},
	} {
		var native vertexChatResponse
		require.NoError(t, json.Unmarshal([]byte(test.body), &native))
		chunk := v.buildChatCompletionStreamResponse(ctx, &native)
		require.Len(t, chunk.Choices, 1)
		assert.Equal(t, test.content, chunk.Choices[0].Delta.Content)
		assert.Len(t, chunk.Choices[0].Delta.ToolCalls, test.calls)
	}
}

func TestVertexParallelToolsStreamByteFragments(t *testing.T) {
	v := &vertexProvider{}
	ctx := newMockMultipartHttpContext()
	sse := "data: " + `{"responseId":"stream-id","candidates":[{"index":0,"content":{"role":"model","parts":[{"functionCall":{"name":"weather","args":{"city":"Hangzhou"}}},{"functionCall":{"name":"time","args":{}}}]}}]}` + "\n\n" +
		"data: " + `{"responseId":"stream-id","candidates":[{"index":0,"finishReason":"STOP"}]}` + "\n\n"
	var output strings.Builder
	for i := range []byte(sse) {
		chunk, err := v.OnStreamingResponseBody(ctx, ApiNameChatCompletion, []byte{sse[i]}, false)
		require.NoError(t, err)
		output.Write(chunk)
	}
	chunk, err := v.OnStreamingResponseBody(ctx, ApiNameChatCompletion, nil, true)
	require.NoError(t, err)
	output.Write(chunk)
	assert.Equal(t, 1, strings.Count(output.String(), "data: [DONE]"))
	var calls []toolCall
	var finalReason string
	for _, event := range strings.Split(output.String(), "\n\n") {
		data := strings.TrimPrefix(event, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}
		var response chatCompletionResponse
		require.NoError(t, json.Unmarshal([]byte(data), &response))
		for _, choice := range response.Choices {
			calls = append(calls, choice.Delta.ToolCalls...)
			if choice.FinishReason != nil {
				finalReason = *choice.FinishReason
			}
		}
	}
	require.Len(t, calls, 2)
	assert.Equal(t, []int{0, 1}, []int{calls[0].Index, calls[1].Index})
	assert.NotEqual(t, calls[0].Id, calls[1].Id)
	assert.Equal(t, finishReasonToolCall, finalReason)
}
