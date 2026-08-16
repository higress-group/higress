package main

import (
	"encoding/json"
	"fmt"
	"time"
)

type chatCompletionBody struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
	Usage   usageBody    `json:"usage"`
}

type chatChoice struct {
	Index        int          `json:"index"`
	Message      *messageBody `json:"message,omitempty"`
	Delta        interface{}  `json:"delta,omitempty"`
	Logprobs     interface{}  `json:"logprobs"`
	FinishReason interface{}  `json:"finish_reason"`
}

type messageBody struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type usageBody struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func buildChatDenyBody(message string) []byte {
	body := chatCompletionBody{
		ID:      newDenyID(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "from-security-guard",
		Choices: []chatChoice{
			{
				Index: 0,
				Message: &messageBody{
					Role:    "assistant",
					Content: message,
				},
				Logprobs:     nil,
				FinishReason: "stop",
			},
		},
		Usage: usageBody{},
	}
	data, _ := json.Marshal(body)
	return data
}

func buildStreamDenyBody(message string) []byte {
	id := newDenyID()
	created := time.Now().Unix()
	chunk := chatCompletionBody{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   "from-security-guard",
		Choices: []chatChoice{
			{
				Index: 0,
				Delta: map[string]string{
					"role":    "assistant",
					"content": message,
				},
				Logprobs:     nil,
				FinishReason: nil,
			},
		},
	}
	end := chatCompletionBody{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   "from-security-guard",
		Choices: []chatChoice{
			{
				Index:        0,
				Delta:        map[string]string{},
				Logprobs:     nil,
				FinishReason: "stop",
			},
		},
		Usage: usageBody{},
	}

	chunkData, _ := json.Marshal(chunk)
	endData, _ := json.Marshal(end)
	return []byte(fmt.Sprintf("data: %s\n\ndata: %s\n\ndata: [DONE]\n\n", chunkData, endData))
}

func newDenyID() string {
	return fmt.Sprintf("chatcmpl-qwen3guard-%d", time.Now().UnixNano())
}
