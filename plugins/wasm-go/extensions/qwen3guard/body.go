package main

import (
	"strings"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

func extractJSONText(body []byte, path string) (string, bool) {
	result := gjson.GetBytes(body, path)
	if !result.Exists() {
		return "", false
	}
	return result.String(), true
}

func splitCompleteSSE(data string, endStream bool) (string, string) {
	if endStream {
		return data, ""
	}
	lfEnd := strings.LastIndex(data, "\n\n")
	crlfEnd := strings.LastIndex(data, "\r\n\r\n")
	switch {
	case lfEnd == -1 && crlfEnd == -1:
		return "", data
	case crlfEnd > lfEnd:
		return data[:crlfEnd+4], data[crlfEnd+4:]
	default:
		return data[:lfEnd+2], data[lfEnd+2:]
	}
}

func extractSSEDataPayloads(data string) []string {
	normalized := strings.ReplaceAll(data, "\r\n", "\n")
	blocks := strings.Split(normalized, "\n\n")
	payloads := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		var lines []string
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data:") {
				lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if len(lines) > 0 {
			payloads = append(payloads, strings.Join(lines, "\n"))
		}
	}
	return payloads
}

func isDonePayload(payload string) bool {
	return strings.TrimSpace(payload) == "[DONE]"
}

func charCount(value string) int {
	return utf8.RuneCountInString(value)
}

func exceedsByteLimit(currentBytes int, additionalBytes int, limit uint32) bool {
	return uint64(currentBytes)+uint64(additionalBytes) > uint64(limit)
}
