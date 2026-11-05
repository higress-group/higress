// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiBuildChatCompletionResponseFunctionCalls(t *testing.T) {
	provider := &geminiProvider{}
	response := provider.buildChatCompletionResponse(newMockMultipartHttpContext(), &geminiChatResponse{
		Candidates: []geminiChatCandidate{
			{
				Index:        0,
				FinishReason: "STOP",
				Content: geminiChatContent{
					Parts: []geminiPart{
						{FunctionCall: &geminiFunctionCall{
							FunctionName: "get_weather",
							Arguments:    map[string]any{"city": "Hangzhou"},
						}},
						{FunctionCall: &geminiFunctionCall{
							FunctionName: "get_time",
							Arguments:    map[string]any{"timezone": "Asia/Shanghai"},
						}},
					},
				},
			},
		},
	})

	require.Len(t, response.Choices, 1)
	choice := response.Choices[0]
	require.NotNil(t, choice.Message)
	require.Len(t, choice.Message.ToolCalls, 2)
	assert.Equal(t, "get_weather", choice.Message.ToolCalls[0].Function.Name)
	assert.JSONEq(t, `{"city":"Hangzhou"}`, choice.Message.ToolCalls[0].Function.Arguments)
	assert.Equal(t, 0, choice.Message.ToolCalls[0].Index)
	assert.Equal(t, "get_time", choice.Message.ToolCalls[1].Function.Name)
	assert.JSONEq(t, `{"timezone":"Asia/Shanghai"}`, choice.Message.ToolCalls[1].Function.Arguments)
	assert.Equal(t, 1, choice.Message.ToolCalls[1].Index)
	assert.NotEmpty(t, choice.Message.ToolCalls[0].Id)
	assert.NotEmpty(t, choice.Message.ToolCalls[1].Id)
	assert.NotEqual(t, choice.Message.ToolCalls[0].Id, choice.Message.ToolCalls[1].Id)
	require.NotNil(t, choice.FinishReason)
	assert.Equal(t, finishReasonToolCall, *choice.FinishReason)
}

func TestGeminiBuildChatCompletionResponseMixedParts(t *testing.T) {
	provider := &geminiProvider{}
	response := provider.buildChatCompletionResponse(newMockMultipartHttpContext(), &geminiChatResponse{
		Candidates: []geminiChatCandidate{
			{
				Index:        0,
				FinishReason: "STOP",
				Content: geminiChatContent{
					Parts: []geminiPart{
						{Text: "Let me check. "},
						{FunctionCall: &geminiFunctionCall{
							FunctionName: "lookup",
							Arguments:    map[string]any{"id": float64(42)},
						}},
						{Text: "One moment."},
					},
				},
			},
		},
	})

	require.Len(t, response.Choices, 1)
	choice := response.Choices[0]
	require.NotNil(t, choice.Message)
	assert.Equal(t, "Let me check. One moment.", choice.Message.Content)
	require.Len(t, choice.Message.ToolCalls, 1)
	assert.Equal(t, "lookup", choice.Message.ToolCalls[0].Function.Name)
	assert.JSONEq(t, `{"id":42}`, choice.Message.ToolCalls[0].Function.Arguments)
}

func TestGeminiBuildChatCompletionResponseSkipsEmptyCandidateAndPreservesInlineData(t *testing.T) {
	provider := &geminiProvider{}
	response := provider.buildChatCompletionResponse(newMockMultipartHttpContext(), &geminiChatResponse{
		Candidates: []geminiChatCandidate{
			{
				Index:   0,
				Content: geminiChatContent{},
			},
			{
				Index:        1,
				FinishReason: "STOP",
				Content: geminiChatContent{
					Parts: []geminiPart{
						{InlineData: &geminiInlineData{
							MimeType: "text/plain",
							Data:     "inline-data",
						}},
					},
				},
			},
		},
	})

	require.Len(t, response.Choices, 1)
	assert.Equal(t, 1, response.Choices[0].Index)
	require.NotNil(t, response.Choices[0].Message)
	assert.Equal(t, "inline-data", response.Choices[0].Message.Content)
	assert.Empty(t, response.Choices[0].Message.ToolCalls)
}

func TestGeminiBuildToolCallsSkipsUnmarshalableArguments(t *testing.T) {
	provider := &geminiProvider{}
	candidate := &geminiChatCandidate{
		Content: geminiChatContent{
			Parts: []geminiPart{
				{FunctionCall: &geminiFunctionCall{
					FunctionName: "invalid",
					Arguments:    make(chan int),
				}},
				{FunctionCall: &geminiFunctionCall{
					FunctionName: "valid",
					Arguments:    map[string]any{"id": float64(42)},
				}},
			},
		},
	}

	calls := provider.buildToolCalls(candidate)

	require.Len(t, calls, 1)
	assert.Equal(t, 0, calls[0].Index)
	assert.Equal(t, "valid", calls[0].Function.Name)
	assert.JSONEq(t, `{"id":42}`, calls[0].Function.Arguments)
}

func TestGeminiTransformResponseBodyFunctionCall(t *testing.T) {
	provider := &geminiProvider{}
	body, err := provider.TransformResponseBody(newMockMultipartHttpContext(), ApiNameChatCompletion, []byte(`{
		"candidates":[{
			"content":{"role":"model","parts":[
				{"functionCall":{"name":"get_weather","args":{"city":"Hangzhou"}}},
				{"functionCall":{"name":"get_time","args":{"timezone":"Asia/Shanghai"}}}
			]},
			"finishReason":"STOP",
			"index":0
		}],
		"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}
	}`))
	require.NoError(t, err)

	var response chatCompletionResponse
	require.NoError(t, json.Unmarshal(body, &response))
	require.Len(t, response.Choices, 1)
	require.NotNil(t, response.Choices[0].Message)
	require.Len(t, response.Choices[0].Message.ToolCalls, 2)
	assert.Equal(t, "get_weather", response.Choices[0].Message.ToolCalls[0].Function.Name)
	assert.Equal(t, "get_time", response.Choices[0].Message.ToolCalls[1].Function.Name)
	require.NotNil(t, response.Choices[0].FinishReason)
	assert.Equal(t, finishReasonToolCall, *response.Choices[0].FinishReason)
	assert.Equal(t, 10, response.Usage.PromptTokens)
	assert.Equal(t, 5, response.Usage.CompletionTokens)
	assert.Equal(t, 15, response.Usage.TotalTokens)
}
