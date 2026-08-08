package prefixcache

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"

	"github.com/cespare/xxhash/v2"
)

const (
	MaxSegmentTokens = 1024
	MaxPrefixTokens  = 131072
	DefaultCapacity  = 31250
	DefaultBlockSize = 16
)

type segmentKind byte

const (
	segmentTools segmentKind = iota + 1
	segmentMessage
	segmentCompletionText
	segmentCompletionTokens
)

type Block struct {
	Hash            uint64
	EstimatedTokens int
}

type Locality struct {
	Chains [][]Block
}

type requestEnvelope struct {
	Model     string          `json:"model"`
	CacheSalt string          `json:"cache_salt"`
	Messages  json.RawMessage `json:"messages"`
	Prompt    json.RawMessage `json:"prompt"`
	Tools     json.RawMessage `json:"tools"`
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
	seed := namespaceHash(request.Model, request.CacheSalt)
	var chains [][]Block
	var supported bool
	var err error
	switch {
	case request.Messages != nil:
		chains, supported, err = extractChat(request.Messages, request.Tools, seed)
	case request.Prompt != nil:
		chains, supported, err = extractCompletion(request.Prompt, seed)
	default:
		return nil, false, nil
	}
	if err != nil || !supported {
		return nil, supported, err
	}
	return &Locality{Chains: chains}, true, nil
}

func extractChat(rawMessages, rawTools json.RawMessage, seed uint64) ([][]Block, bool, error) {
	chain := make([]Block, 0)
	previous := seed
	totalTokens := 0
	if len(rawTools) > 0 && string(rawTools) != "null" {
		canonical, nonEmpty, err := canonicalArray(rawTools)
		if err != nil {
			return nil, false, err
		}
		if nonEmpty {
			appendTextSegments(&chain, segmentTools, canonical, &previous, &totalTokens)
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(rawMessages))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('[') {
		return nil, false, fmt.Errorf("messages must be an array")
	}
	for decoder.More() {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, false, err
		}
		message, supported, err := canonicalMessage(raw)
		if err != nil || !supported {
			return nil, supported, err
		}
		appendTextSegments(&chain, segmentMessage, message, &previous, &totalTokens)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, false, err
	}
	return [][]Block{chain}, true, nil
}

func canonicalArray(raw json.RawMessage) ([]byte, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value []any
	if err := decoder.Decode(&value); err != nil {
		return nil, false, err
	}
	canonical, err := json.Marshal(value)
	return canonical, len(value) > 0, err
}

func canonicalMessage(raw json.RawMessage) ([]byte, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var message map[string]any
	if err := decoder.Decode(&message); err != nil {
		return nil, false, err
	}
	if !textOnlyContent(message["content"]) {
		return nil, false, nil
	}
	canonical, err := json.Marshal(message)
	return canonical, true, err
}

func textOnlyContent(content any) bool {
	switch value := content.(type) {
	case nil, string:
		return true
	case []any:
		for _, rawPart := range value {
			part, ok := rawPart.(map[string]any)
			if !ok || part["type"] != "text" {
				return false
			}
			if _, ok := part["text"].(string); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func extractCompletion(raw json.RawMessage, seed uint64) ([][]Block, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return nil, false, err
	}
	if text, ok := token.(string); ok {
		return [][]Block{textChain(text, segmentCompletionText, seed)}, true, nil
	}
	if token != json.Delim('[') {
		return nil, false, fmt.Errorf("prompt must be a string or an array")
	}
	if !decoder.More() {
		_, err := decoder.Token()
		return nil, true, err
	}

	var first json.RawMessage
	if err := decoder.Decode(&first); err != nil {
		return nil, false, err
	}
	first = bytes.TrimSpace(first)
	if len(first) > 0 && first[0] == '"' {
		chains := make([][]Block, 0)
		var text string
		if err := json.Unmarshal(first, &text); err != nil {
			return nil, false, err
		}
		chains = append(chains, textChain(text, segmentCompletionText, seed))
		for decoder.More() {
			if err := decoder.Decode(&text); err != nil {
				return nil, false, fmt.Errorf("prompt contains mixed types: %w", err)
			}
			chains = append(chains, textChain(text, segmentCompletionText, seed))
		}
		if _, err := decoder.Token(); err != nil {
			return nil, false, err
		}
		return chains, true, nil
	}

	builder := newTokenChainBuilder(seed)
	var number json.Number
	if err := json.Unmarshal(first, &number); err != nil {
		return nil, false, fmt.Errorf("prompt contains mixed types: %w", err)
	}
	if err := builder.appendNumber(number); err != nil {
		return nil, false, err
	}
	for decoder.More() {
		if err := decoder.Decode(&number); err != nil {
			return nil, false, err
		}
		if err := builder.appendNumber(number); err != nil {
			return nil, false, err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, false, err
	}
	return [][]Block{builder.finish()}, true, nil
}

func textChain(text string, kind segmentKind, seed uint64) []Block {
	chain := make([]Block, 0, (min(len(text), MaxPrefixTokens*4)+MaxSegmentTokens*4-1)/(MaxSegmentTokens*4))
	previous, totalTokens := seed, 0
	appendStringSegments(&chain, kind, text, &previous, &totalTokens)
	return chain
}

func appendStringSegments(chain *[]Block, kind segmentKind, data string, previous *uint64, totalTokens *int) {
	remainingBytes := (MaxPrefixTokens - *totalTokens) * 4
	if remainingBytes <= 0 {
		return
	}
	data = data[:min(len(data), remainingBytes)]
	const maxSegmentBytes = MaxSegmentTokens * 4
	for start := 0; start < len(data); start += maxSegmentBytes {
		segment := data[start:min(start+maxSegmentBytes, len(data))]
		tokens := (len(segment) + 3) / 4
		*previous = chainedHash(kind, tokens, xxhash.Sum64String(segment), *previous)
		*chain = append(*chain, Block{Hash: *previous, EstimatedTokens: tokens})
		*totalTokens += tokens
	}
}

func appendTextSegments(chain *[]Block, kind segmentKind, data []byte, previous *uint64, totalTokens *int) {
	remainingBytes := (MaxPrefixTokens - *totalTokens) * 4
	if remainingBytes <= 0 {
		return
	}
	data = data[:min(len(data), remainingBytes)]
	const maxSegmentBytes = MaxSegmentTokens * 4
	for start := 0; start < len(data); start += maxSegmentBytes {
		segment := data[start:min(start+maxSegmentBytes, len(data))]
		tokens := (len(segment) + 3) / 4
		*previous = chainedHash(kind, tokens, xxhash.Sum64(segment), *previous)
		*chain = append(*chain, Block{Hash: *previous, EstimatedTokens: tokens})
		*totalTokens += tokens
	}
}

type tokenChainBuilder struct {
	chain    []Block
	buffer   [MaxSegmentTokens]uint32
	buffered int
	total    int
	previous uint64
}

func newTokenChainBuilder(seed uint64) *tokenChainBuilder {
	return &tokenChainBuilder{previous: seed}
}

func (builder *tokenChainBuilder) appendNumber(number json.Number) error {
	value, err := number.Int64()
	if err != nil || value < 0 || value > math.MaxUint32 {
		return fmt.Errorf("prompt token ID is invalid")
	}
	if builder.total >= MaxPrefixTokens {
		return nil
	}
	builder.buffer[builder.buffered] = uint32(value)
	builder.buffered++
	builder.total++
	if builder.buffered == len(builder.buffer) {
		builder.flush()
	}
	return nil
}

func (builder *tokenChainBuilder) finish() []Block {
	builder.flush()
	return builder.chain
}

func (builder *tokenChainBuilder) flush() {
	if builder.buffered == 0 {
		return
	}
	var encoded [MaxSegmentTokens * 4]byte
	for index, token := range builder.buffer[:builder.buffered] {
		binary.LittleEndian.PutUint32(encoded[index*4:], token)
	}
	contentHash := xxhash.Sum64(encoded[:builder.buffered*4])
	builder.previous = chainedHash(segmentCompletionTokens, builder.buffered, contentHash, builder.previous)
	builder.chain = append(builder.chain, Block{Hash: builder.previous, EstimatedTokens: builder.buffered})
	builder.buffered = 0
}

func namespaceHash(model, cacheSalt string) uint64 {
	hasher := xxhash.New()
	var length [8]byte
	binary.LittleEndian.PutUint64(length[:], uint64(len(model)))
	_, _ = hasher.Write(length[:])
	_, _ = io.WriteString(hasher, model)
	binary.LittleEndian.PutUint64(length[:], uint64(len(cacheSalt)))
	_, _ = hasher.Write(length[:])
	_, _ = io.WriteString(hasher, cacheSalt)
	return hasher.Sum64()
}

func chainedHash(kind segmentKind, tokens int, contentHash, previous uint64) uint64 {
	var encoded [25]byte
	encoded[0] = byte(kind)
	binary.LittleEndian.PutUint64(encoded[1:9], uint64(tokens))
	binary.LittleEndian.PutUint64(encoded[9:17], contentHash)
	binary.LittleEndian.PutUint64(encoded[17:25], previous)
	return xxhash.Sum64(encoded[:])
}
