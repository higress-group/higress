package provider

import (
	"encoding/json"
	"fmt"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"
)

func decodeChatCompletionRequest(body []byte, request *chatCompletionRequest) error {
	if err := json.Unmarshal(body, request); err != nil {
		return fmt.Errorf("unable to unmarshal request: %v", err)
	}
	if request.Messages == nil || len(request.Messages) == 0 {
		return fmt.Errorf("no message found in the request body: %s", body)
	}
	return nil
}

func decodeEmbeddingsRequest(body []byte, request *embeddingsRequest) error {
	if err := json.Unmarshal(body, request); err != nil {
		return fmt.Errorf("unable to unmarshal request: %v", err)
	}
	return nil
}

func decodeImageGenerationRequest(body []byte, request *imageGenerationRequest) error {
	if err := json.Unmarshal(body, request); err != nil {
		return fmt.Errorf("unable to unmarshal request: %v", err)
	}
	return nil
}

func decodeImageEditRequest(body []byte, request *imageEditRequest) error {
	if err := json.Unmarshal(body, request); err != nil {
		return fmt.Errorf("unable to unmarshal request: %v", err)
	}
	return nil
}

func decodeImageVariationRequest(body []byte, request *imageVariationRequest) error {
	if err := json.Unmarshal(body, request); err != nil {
		return fmt.Errorf("unable to unmarshal request: %v", err)
	}
	return nil
}

func replaceJsonRequestBody(request interface{}) error {
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("unable to marshal request: %v", err)
	}
	log.Debugf("request body: %s", string(body))
	err = proxywasm.ReplaceHttpRequestBody(body)
	if err != nil {
		return fmt.Errorf("unable to replace the original request body: %v", err)
	}
	return err
}

func replaceRequestBody(body []byte) error {
	log.Debugf("request body: %s", string(body))
	err := proxywasm.ReplaceHttpRequestBody(body)
	if err != nil {
		return fmt.Errorf("unable to replace the original request body: %v", err)
	}
	return nil
}

func insertContextMessage(request *chatCompletionRequest, content string) {
	fileMessage := chatMessage{
		Role:    roleSystem,
		Content: content,
	}
	var firstNonSystemMessageIndex int
	for i, message := range request.Messages {
		if message.Role != roleSystem {
			firstNonSystemMessageIndex = i
			break
		}
	}
	if firstNonSystemMessageIndex == 0 {
		request.Messages = append([]chatMessage{fileMessage}, request.Messages...)
	} else {
		request.Messages = append(request.Messages[:firstNonSystemMessageIndex], append([]chatMessage{fileMessage}, request.Messages[firstNonSystemMessageIndex:]...)...)
	}
}

// cleanupContextMessages 根据配置的清理命令清理上下文消息
// 查找最后一个完全匹配任意 cleanupCommands 的 user 消息，将该消息及之前所有非 system 消息清理掉，只保留 system 消息
func cleanupContextMessages(body []byte, cleanupCommands []string) ([]byte, error) {
	if len(cleanupCommands) == 0 {
		return body, nil
	}

	request := &chatCompletionRequest{}
	if err := json.Unmarshal(body, request); err != nil {
		return body, fmt.Errorf("unable to unmarshal request for context cleanup: %v", err)
	}

	if len(request.Messages) == 0 {
		return body, nil
	}

	// 从后往前查找最后一个匹配任意清理命令的 user 消息
	cleanupIndex := -1
	for i := len(request.Messages) - 1; i >= 0; i-- {
		msg := request.Messages[i]
		if msg.Role == roleUser {
			content := msg.StringContent()
			for _, cmd := range cleanupCommands {
				if content == cmd {
					cleanupIndex = i
					break
				}
			}
			if cleanupIndex != -1 {
				break
			}
		}
	}

	// 没有找到匹配的清理命令
	if cleanupIndex == -1 {
		return body, nil
	}

	log.Debugf("[contextCleanup] found cleanup command at index %d, cleaning up messages", cleanupIndex)

	// 构建新的消息列表：
	// 1. 保留 cleanupIndex 之前的 system 消息（只保留 system，其他都清理）
	// 2. 删除 cleanupIndex 位置的清理命令消息
	// 3. 保留 cleanupIndex 之后的所有消息
	var newMessages []chatMessage

	// 处理 cleanupIndex 之前的消息，只保留 system
	for i := 0; i < cleanupIndex; i++ {
		msg := request.Messages[i]
		if msg.Role == roleSystem {
			newMessages = append(newMessages, msg)
		}
	}

	// 跳过 cleanupIndex 位置的消息（清理命令本身）
	// 保留 cleanupIndex 之后的所有消息
	for i := cleanupIndex + 1; i < len(request.Messages); i++ {
		newMessages = append(newMessages, request.Messages[i])
	}

	request.Messages = newMessages
	log.Debugf("[contextCleanup] messages after cleanup: %d", len(newMessages))

	return json.Marshal(request)
}

// mergeConsecutiveMessages merges consecutive messages of the same role (user or assistant).
// Many LLM providers require strict user↔assistant alternation and reject requests where
// two messages of the same role appear consecutively. When enabled, consecutive same-role
// messages have their content concatenated into a single message.
func mergeConsecutiveMessages(body []byte) ([]byte, error) {
	request := &chatCompletionRequest{}
	if err := json.Unmarshal(body, request); err != nil {
		return body, fmt.Errorf("unable to unmarshal request for message merging: %v", err)
	}
	if len(request.Messages) <= 1 {
		return body, nil
	}

	merged := false
	result := make([]chatMessage, 0, len(request.Messages))
	for _, msg := range request.Messages {
		if len(result) > 0 &&
			result[len(result)-1].Role == msg.Role &&
			(msg.Role == roleUser || msg.Role == roleAssistant) {
			last := &result[len(result)-1]
			if msg.Role == roleAssistant &&
				(len(last.ToolCalls) > 0 || len(msg.ToolCalls) > 0 || last.FunctionCall != nil || msg.FunctionCall != nil) {
				// Assistant tool-calling turns are structurally sensitive. Keep them split
				// rather than trying to merge content/reasoning and risking dropped calls.
				result = append(result, msg)
				continue
			}
			last.Content = mergeMessageContent(last.Content, msg.Content)
			last.ReasoningContent = mergeReasoningContent(last.ReasoningContent, msg.ReasoningContent)
			merged = true
			continue
		}
		result = append(result, msg)
	}

	if !merged {
		return body, nil
	}
	request.Messages = result
	return json.Marshal(request)
}

// mergeMessageContent concatenates two message content values.
// If both are plain strings they are joined with a blank line.
// Otherwise both are converted to content-block arrays and concatenated.
func mergeMessageContent(prev, curr any) any {
	prevStr, prevIsStr := prev.(string)
	currStr, currIsStr := curr.(string)
	if prevIsStr && currIsStr {
		return prevStr + "\n\n" + currStr
	}
	prevParts := (&chatMessage{Content: prev}).ParseContent()
	currParts := (&chatMessage{Content: curr}).ParseContent()
	return append(prevParts, currParts...)
}

func mergeReasoningContent(prev, curr string) string {
	switch {
	case prev == "":
		return curr
	case curr == "":
		return prev
	default:
		return prev + "\n\n" + curr
	}
}

func ReplaceResponseBody(body []byte) error {
	log.Debugf("response body: %s", string(body))
	err := proxywasm.ReplaceHttpResponseBody(body)
	if err != nil {
		return fmt.Errorf("unable to replace the original response body: %v", err)
	}
	return nil
}
