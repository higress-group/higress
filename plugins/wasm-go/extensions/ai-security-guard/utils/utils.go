package utils

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
	"strings"
	"unicode/utf8"

	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func GenerateHexID(length int) (string, error) {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func GenerateRandomChatID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 29)
	for i := range b {
		b[i] = charset[mrand.Intn(len(charset))]
	}
	return "chatcmpl-" + string(b)
}

func ExtractMessageFromStreamingBody(data []byte, jsonPath string) string {
	chunks := bytes.Split(bytes.TrimSpace(wrapper.UnifySSEChunk(data)), []byte("\n\n"))
	strChunks := []string{}
	for _, chunk := range chunks {
		// Example: "choices":[{"index":0,"delta":{"role":"assistant","content":"%s"},"logprobs":null,"finish_reason":null}]
		strChunks = append(strChunks, gjson.GetBytes(chunk, jsonPath).String())
	}
	return strings.Join(strChunks, "")
}

func GetConsumer(ctx wrapper.HttpContext) string {
	return ctx.GetStringContext("consumer", "")
}

func ReplaceJsonFieldContent(body []byte, jsonPath string, newContent string) ([]byte, error) {
	return sjson.SetBytes(body, resolveJsonPath(body, jsonPath), newContent)
}

// ReplaceJsonFieldTextContent replaces text content at jsonPath, handling both
// string and array (multimodal) content formats. When the field is an array
// (e.g. OpenAI multimodal content with text + image_url items), only the text
// items are updated while image_url and other items are preserved.
func ReplaceJsonFieldTextContent(body []byte, jsonPath string, newContent string) ([]byte, error) {
	resolved := resolveJsonPath(body, jsonPath)
	fieldValue := gjson.GetBytes(body, resolved)
	if !fieldValue.IsArray() {
		// Simple string content — replace directly
		return sjson.SetBytes(body, resolved, newContent)
	}
	// Array content (multimodal): replace text items, preserve others
	result := body
	var err error
	remaining := newContent
	items := fieldValue.Array()
	// Collect original text lengths for proportional splitting
	type textEntry struct {
		index int
		text  string
	}
	var textEntries []textEntry
	totalTextLen := 0
	for i, item := range items {
		if item.Get("type").String() == "text" {
			t := item.Get("text").String()
			textEntries = append(textEntries, textEntry{index: i, text: t})
			totalTextLen += utf8.RuneCountInString(t)
		}
	}
	if len(textEntries) == 0 {
		// No text items found, nothing to replace
		return body, nil
	}
	// If there's only one text item, put all desensitized content there
	if len(textEntries) == 1 {
		itemPath := fmt.Sprintf("%s.%d.text", resolved, textEntries[0].index)
		return sjson.SetBytes(result, itemPath, newContent)
	}
	// Multiple text items: split desensitized content proportionally by original lengths
	for j, entry := range textEntries {
		var replacement string
		if j == len(textEntries)-1 {
			// Last text item gets all remaining content
			replacement = remaining
		} else {
			// Proportional split based on original text length (rune-aware)
			var proportion int
			if totalTextLen == 0 {
				// All original text items are empty; roughly even with remainder on later segments
				proportion = utf8.RuneCountInString(newContent) / len(textEntries)
			} else {
				proportion = utf8.RuneCountInString(entry.text) * utf8.RuneCountInString(newContent) / totalTextLen
			}
			runeCount := utf8.RuneCountInString(remaining)
			if proportion > runeCount {
				proportion = runeCount
			}
			// Convert rune count to byte offset to split at character boundary
			byteOffset := 0
			for i := 0; i < proportion; i++ {
				_, size := utf8.DecodeRuneInString(remaining[byteOffset:])
				byteOffset += size
			}
			replacement = remaining[:byteOffset]
			remaining = remaining[byteOffset:]
		}
		itemPath := fmt.Sprintf("%s.%d.text", resolved, entry.index)
		result, err = sjson.SetBytes(result, itemPath, replacement)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// resolveJsonPath converts gjson modifier paths (e.g. "messages.@reverse.0.content")
// into concrete index paths (e.g. "messages.2.content") that sjson can handle.
func resolveJsonPath(body []byte, jsonPath string) string {
	parts := strings.Split(jsonPath, ".")
	var resolved []string
	for i := 0; i < len(parts); i++ {
		if strings.HasPrefix(parts[i], "@reverse") && i+1 < len(parts) {
			// Get the array at the path resolved so far
			arrayPath := strings.Join(resolved, ".")
			arrayLen := int(gjson.GetBytes(body, arrayPath+".#").Int())
			// Next part should be the reversed index
			i++
			reversedIdx := 0
			fmt.Sscanf(parts[i], "%d", &reversedIdx)
			actualIdx := arrayLen - 1 - reversedIdx
			if actualIdx < 0 {
				actualIdx = 0
			}
			resolved = append(resolved, fmt.Sprintf("%d", actualIdx))
		} else {
			resolved = append(resolved, parts[i])
		}
	}
	return strings.Join(resolved, ".")
}
