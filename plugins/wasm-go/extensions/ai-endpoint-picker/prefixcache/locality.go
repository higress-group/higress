package prefixcache

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"

	"github.com/cespare/xxhash/v2"
)

const (
	MinBlockSizeTokens = 64
	MaxPrefixTokens    = 131072
	DefaultCapacity    = 31250
)

type Locality struct {
	Model     string
	CacheSalt string
	Prompts   [][]uint32
}

type requestEnvelope struct {
	Model     string          `json:"model"`
	CacheSalt string          `json:"cache_salt"`
	Messages  json.RawMessage `json:"messages"`
	Prompt    json.RawMessage `json:"prompt"`
	Tools     json.RawMessage `json:"tools"`
}

type chatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Extract returns an unavailable locality without an error for request shapes
// whose non-text content cannot be represented safely by the estimate tokenizer.
func Extract(body []byte) (*Locality, bool, error) {
	var request requestEnvelope
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, false, err
	}
	if request.Model == "" {
		return nil, false, fmt.Errorf("model is required")
	}
	locality := &Locality{Model: request.Model, CacheSalt: request.CacheSalt}
	switch {
	case request.Messages != nil:
		prompts, supported, err := extractChat(request.Messages, request.Tools)
		if err != nil || !supported {
			return nil, supported, err
		}
		locality.Prompts = prompts
	case request.Prompt != nil:
		prompts, supported, err := extractCompletion(request.Prompt)
		if err != nil || !supported {
			return nil, supported, err
		}
		locality.Prompts = prompts
	default:
		return nil, false, nil
	}
	return locality, true, nil
}

func extractChat(rawMessages, rawTools json.RawMessage) ([][]uint32, bool, error) {
	var messages []chatMessage
	if err := json.Unmarshal(rawMessages, &messages); err != nil {
		return nil, false, err
	}
	var text bytes.Buffer
	if len(rawTools) > 0 && string(rawTools) != "null" {
		var tools []any
		if err := json.Unmarshal(rawTools, &tools); err != nil {
			return nil, false, err
		}
		if len(tools) > 0 {
			canonical, err := json.Marshal(tools)
			if err != nil {
				return nil, false, err
			}
			text.Write(canonical)
		}
	}
	for _, message := range messages {
		text.WriteString(message.Role)
		content, supported, err := textContent(message.Content)
		if err != nil || !supported {
			return nil, supported, err
		}
		text.WriteString(content)
	}
	return [][]uint32{estimateTokens(text.Bytes())}, true, nil
}

func textContent(raw json.RawMessage) (string, bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", true, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, true, nil
	}
	var parts []contentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", false, nil
	}
	var combined bytes.Buffer
	for _, part := range parts {
		if part.Type != "text" {
			return "", false, nil
		}
		combined.WriteString(part.Text)
	}
	return combined.String(), true, nil
}

func extractCompletion(raw json.RawMessage) ([][]uint32, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var prompt any
	if err := decoder.Decode(&prompt); err != nil {
		return nil, false, err
	}
	switch value := prompt.(type) {
	case string:
		return [][]uint32{estimateTokens([]byte(value))}, true, nil
	case []any:
		if len(value) == 0 {
			return [][]uint32{{}}, true, nil
		}
		if _, ok := value[0].(string); ok {
			prompts := make([][]uint32, 0, len(value))
			for _, item := range value {
				text, ok := item.(string)
				if !ok {
					return nil, false, nil
				}
				prompts = append(prompts, estimateTokens([]byte(text)))
			}
			return prompts, true, nil
		}
		tokens := make([]uint32, 0, len(value))
		for _, item := range value {
			number, ok := item.(json.Number)
			if !ok {
				return nil, false, nil
			}
			parsed, err := number.Int64()
			if err != nil || parsed < 0 || parsed > math.MaxUint32 {
				return nil, false, nil
			}
			tokens = append(tokens, uint32(parsed))
		}
		return [][]uint32{tokens}, true, nil
	default:
		return nil, false, nil
	}
}

func estimateTokens(text []byte) []uint32 {
	tokens := make([]uint32, (len(text)+3)/4)
	for index := range tokens {
		var packed [4]byte
		copy(packed[:], text[index*4:min((index+1)*4, len(text))])
		tokens[index] = binary.LittleEndian.Uint32(packed[:])
	}
	return tokens
}

func EffectiveBlockSize(reported int) int {
	if reported < MinBlockSizeTokens {
		return MinBlockSizeTokens
	}
	return reported
}

func (locality *Locality) BlockChains(reportedBlockSize int) [][]uint64 {
	blockSize := EffectiveBlockSize(reportedBlockSize)
	maxBlocks := MaxPrefixTokens / blockSize
	result := make([][]uint64, 0, len(locality.Prompts))
	for _, prompt := range locality.Prompts {
		if len(prompt) > MaxPrefixTokens {
			prompt = prompt[:MaxPrefixTokens]
		}
		chain := make([]uint64, 0, min((len(prompt)+blockSize-1)/blockSize, maxBlocks))
		seed := xxhash.New()
		_, _ = seed.WriteString(locality.Model)
		_, _ = seed.WriteString(locality.CacheSalt)
		previous := seed.Sum64()
		for start := 0; start < len(prompt) && len(chain) < maxBlocks; start += blockSize {
			end := min(start+blockSize, len(prompt))
			content := make([]byte, (end-start)*4)
			for index, token := range prompt[start:end] {
				binary.LittleEndian.PutUint32(content[index*4:], token)
			}
			blockID := xxhash.Sum64(content)
			var chained [16]byte
			binary.LittleEndian.PutUint64(chained[:8], blockID)
			binary.LittleEndian.PutUint64(chained[8:], previous)
			previous = xxhash.Sum64(chained[:])
			chain = append(chain, previous)
		}
		result = append(result, chain)
	}
	return result
}
