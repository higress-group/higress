package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

const guardMaxTokens = 128

var (
	safetyPattern     = regexp.MustCompile(`(?mi)^Safety:\s*(Safe|Unsafe|Controversial)\b`)
	categoriesLine    = regexp.MustCompile(`(?mi)^Categories:\s*(.+)$`)
	refusalLine       = regexp.MustCompile(`(?mi)^Refusal:\s*(Yes|No)\b`)
	errEmptyChoices   = errors.New("qwen3guard: empty choices")
	errMissingSafety  = errors.New("qwen3guard: missing safety label")
	errEmptyModerated = errors.New("qwen3guard: empty content")
)

type qwenChatRequest struct {
	Model     string        `json:"model"`
	Messages  []qwenMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens"`
}

type qwenMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type qwenChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type guardResult struct {
	Safety     string
	Categories []string
	Refusal    string
	Raw        string
}

func buildPromptModerationBody(model string, prompt string) ([]byte, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, errEmptyModerated
	}
	return json.Marshal(qwenChatRequest{
		Model: model,
		Messages: []qwenMessage{
			{Role: "user", Content: prompt},
		},
		MaxTokens: guardMaxTokens,
	})
}

func buildResponseModerationBody(model string, prompt string, response string) ([]byte, error) {
	if strings.TrimSpace(response) == "" {
		return nil, errEmptyModerated
	}
	messages := make([]qwenMessage, 0, 2)
	if strings.TrimSpace(prompt) != "" {
		messages = append(messages, qwenMessage{Role: "user", Content: prompt})
	}
	messages = append(messages, qwenMessage{Role: "assistant", Content: response})
	return json.Marshal(qwenChatRequest{
		Model:     model,
		Messages:  messages,
		MaxTokens: guardMaxTokens,
	})
}

func buildGuardHeaders(apiKey string) [][2]string {
	headers := [][2]string{{"content-type", "application/json"}}
	if apiKey != "" {
		headers = append(headers, [2]string{"authorization", "Bearer " + apiKey})
	}
	return headers
}

func parseGuardHTTPResponse(statusCode int, responseBody []byte) (guardResult, error) {
	if statusCode != http.StatusOK {
		return guardResult{}, fmt.Errorf("qwen3guard returned status %d", statusCode)
	}
	var response qwenChatResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return guardResult{}, err
	}
	if len(response.Choices) == 0 {
		return guardResult{}, errEmptyChoices
	}
	return parseGuardResult(response.Choices[0].Message.Content)
}

func parseGuardResult(content string) (guardResult, error) {
	result := guardResult{Raw: content}
	safetyMatch := safetyPattern.FindStringSubmatch(content)
	if len(safetyMatch) != 2 {
		return result, errMissingSafety
	}
	result.Safety = safetyMatch[1]

	if categoriesMatch := categoriesLine.FindStringSubmatch(content); len(categoriesMatch) == 2 {
		for _, category := range strings.Split(categoriesMatch[1], ",") {
			category = strings.TrimSpace(category)
			if category != "" {
				result.Categories = append(result.Categories, category)
			}
		}
	}
	if refusalMatch := refusalLine.FindStringSubmatch(content); len(refusalMatch) == 2 {
		result.Refusal = refusalMatch[1]
	}
	return result, nil
}
