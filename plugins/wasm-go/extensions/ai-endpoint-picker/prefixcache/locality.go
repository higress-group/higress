package prefixcache

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/cespare/xxhash/v2"
)

const (
	MaxSegmentTokens = 1024
	MaxPrefixTokens  = 131072
	DefaultCapacity  = 31250
	DefaultBlockSize = 16
	maxSegmentBytes  = MaxSegmentTokens * 4
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

type jsonField struct {
	key    string
	rawKey []byte
	value  []byte
}

type messageContentKind byte

const (
	messageContentAbsent messageContentKind = iota
	messageContentNull
	messageContentString
	messageContentStructuredText
)

// Extract parses prompt-bearing fields as spans into body. Large strings are
// unescaped directly into bounded hash buffers instead of materializing a full
// decoded string or token slice.
func Extract(body []byte) (*Locality, bool, error) {
	fields, err := parseObjectFields(body)
	if err != nil {
		return nil, false, err
	}
	modelRaw := fieldValue(fields, "model")
	if modelRaw == nil {
		return nil, false, fmt.Errorf("model is required")
	}
	modelLength, err := decodedStringLength(modelRaw)
	if err != nil || modelLength == 0 {
		return nil, false, fmt.Errorf("model is required")
	}
	cacheSaltRaw := fieldValue(fields, "cache_salt")
	if bytes.Equal(bytes.TrimSpace(cacheSaltRaw), []byte("null")) {
		cacheSaltRaw = nil
	}
	seed, err := namespaceHash(modelRaw, modelLength, cacheSaltRaw)
	if err != nil {
		return nil, false, err
	}
	if messages := fieldValue(fields, "messages"); messages != nil {
		chains, supported, err := extractChat(messages, fieldValue(fields, "tools"), seed)
		if err != nil || !supported {
			return nil, supported, err
		}
		return &Locality{Chains: chains}, true, nil
	}
	if prompt := fieldValue(fields, "prompt"); prompt != nil {
		chains, supported, err := extractCompletion(prompt, seed)
		if err != nil || !supported {
			return nil, supported, err
		}
		return &Locality{Chains: chains}, true, nil
	}
	return nil, false, nil
}

func extractChat(rawMessages, rawTools []byte, seed uint64) ([][]Block, bool, error) {
	chain := make([]Block, 0)
	previous, totalTokens := seed, 0
	if len(rawTools) > 0 && !bytes.Equal(bytes.TrimSpace(rawTools), []byte("null")) {
		hasValues, err := arrayHasValues(rawTools)
		if err != nil {
			return nil, false, err
		}
		if hasValues {
			builder := newSegmentBuilder(&chain, segmentTools, 0, &previous, &totalTokens)
			if err := writeCanonicalJSON(rawTools, builder); err != nil && !errors.Is(err, errSemanticCap) {
				return nil, false, err
			}
			builder.finish()
		}
	}
	err := forEachArrayValue(rawMessages, func(rawMessage []byte) error {
		if totalTokens >= MaxPrefixTokens {
			return errSemanticCap
		}
		fields, err := parseObjectFields(rawMessage)
		if err != nil {
			return err
		}
		if hasTopLevelMultimodal(fields) {
			return errUnsupportedContent
		}
		content, contentKind, supported, err := validateMessageContent(fieldValue(fields, "content"))
		if err != nil {
			return err
		}
		if !supported {
			return errUnsupportedContent
		}
		metadata := fieldsWithout(fields, "content")
		metadataLimit := (MaxPrefixTokens - totalTokens) * 4
		domainHasher := xxhash.New()
		_, _ = domainHasher.Write([]byte{byte(contentKind)})
		domainWriter := &boundedWriter{target: domainHasher, remaining: metadataLimit}
		if err := writeCanonicalObject(metadata, domainWriter); err != nil && !errors.Is(err, errSemanticCap) {
			return err
		}
		builder := newSegmentBuilder(&chain, segmentMessage, domainHasher.Sum64(), &previous, &totalTokens)
		metadataWriter := &boundedWriter{target: builder, remaining: domainWriter.written}
		if err := writeCanonicalObject(metadata, metadataWriter); err != nil && !errors.Is(err, errSemanticCap) {
			return err
		}
		if content != nil {
			if isJSONString(content) {
				if err := writeDecodedJSONString(content, builder); err != nil {
					return err
				}
			} else if err := writeCanonicalJSON(content, builder); err != nil {
				return err
			}
		}
		builder.finish()
		return nil
	})
	if errors.Is(err, errUnsupportedContent) {
		return nil, false, nil
	}
	if err != nil && !errors.Is(err, errSemanticCap) {
		return nil, false, err
	}
	return [][]Block{chain}, true, nil
}

var errUnsupportedContent = errors.New("unsupported non-text content")
var errSemanticCap = errors.New("semantic prefix cap reached")

func hasTopLevelMultimodal(fields []jsonField) bool {
	for _, name := range []string{"audio", "input_audio", "image", "image_url", "video", "video_url"} {
		value := bytes.TrimSpace(fieldValue(fields, name))
		if len(value) > 0 && !bytes.Equal(value, []byte("null")) {
			return true
		}
	}
	return false
}

func validateMessageContent(raw []byte) ([]byte, messageContentKind, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, messageContentAbsent, true, nil
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return trimmed, messageContentNull, true, nil
	}
	if isJSONString(trimmed) {
		return trimmed, messageContentString, true, nil
	}
	if trimmed[0] != '[' {
		return nil, 0, false, nil
	}
	supported := true
	err := forEachArrayValue(trimmed, func(rawPart []byte) error {
		fields, err := parseObjectFields(rawPart)
		if err != nil {
			return err
		}
		partType, err := decodeSmallString(fieldValue(fields, "type"))
		if err != nil || partType != "text" || !isJSONString(fieldValue(fields, "text")) {
			supported = false
		}
		return nil
	})
	return trimmed, messageContentStructuredText, supported, err
}

func extractCompletion(raw []byte, seed uint64) ([][]Block, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if isJSONString(trimmed) {
		chain := make([]Block, 0)
		previous, totalTokens := seed, 0
		builder := newSegmentBuilder(&chain, segmentCompletionText, 0, &previous, &totalTokens)
		if err := writeDecodedJSONString(trimmed, builder); err != nil {
			return nil, false, err
		}
		builder.finish()
		return [][]Block{chain}, true, nil
	}
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, false, fmt.Errorf("prompt must be a string or an array")
	}
	chains := make([][]Block, 0)
	var tokenBuilder *tokenChainBuilder
	mode := byte(0)
	err := forEachArrayValue(trimmed, func(item []byte) error {
		item = bytes.TrimSpace(item)
		if mode == 0 {
			if isJSONString(item) {
				mode = 's'
			} else {
				mode = 'n'
				tokenBuilder = newTokenChainBuilder(seed)
			}
		}
		if mode == 's' {
			if !isJSONString(item) {
				return fmt.Errorf("prompt contains mixed types")
			}
			chain := make([]Block, 0)
			previous, totalTokens := seed, 0
			builder := newSegmentBuilder(&chain, segmentCompletionText, 0, &previous, &totalTokens)
			if err := writeDecodedJSONString(item, builder); err != nil {
				return err
			}
			builder.finish()
			chains = append(chains, chain)
			return nil
		}
		if isJSONString(item) {
			return fmt.Errorf("prompt contains mixed types")
		}
		if tokenBuilder.total >= MaxPrefixTokens {
			return errSemanticCap
		}
		return tokenBuilder.appendNumberBytes(item)
	})
	if err != nil && !errors.Is(err, errSemanticCap) {
		return nil, false, err
	}
	if tokenBuilder != nil {
		chains = append(chains, tokenBuilder.finish())
	}
	return chains, true, nil
}

type segmentBuilder struct {
	chain       *[]Block
	kind        segmentKind
	domain      uint64
	previous    *uint64
	totalTokens *int
	remaining   int
	buffer      [maxSegmentBytes]byte
	buffered    int
}

func newSegmentBuilder(chain *[]Block, kind segmentKind, domain uint64, previous *uint64, totalTokens *int) *segmentBuilder {
	return &segmentBuilder{
		chain: chain, kind: kind, domain: domain, previous: previous, totalTokens: totalTokens,
		remaining: (MaxPrefixTokens - *totalTokens) * 4,
	}
}

func (builder *segmentBuilder) Write(data []byte) (int, error) {
	original := len(data)
	if builder.remaining <= 0 {
		return original, nil
	}
	data = data[:min(len(data), builder.remaining)]
	builder.remaining -= len(data)
	for len(data) > 0 {
		copied := copy(builder.buffer[builder.buffered:], data)
		builder.buffered += copied
		data = data[copied:]
		if builder.buffered == len(builder.buffer) {
			builder.flush()
		}
	}
	return original, nil
}

func (builder *segmentBuilder) inputLimit() int { return builder.remaining }

func (builder *segmentBuilder) semanticCapReached() bool { return builder.remaining <= 0 }

func (builder *segmentBuilder) finish() { builder.flush() }

func (builder *segmentBuilder) flush() {
	if builder.buffered == 0 {
		return
	}
	tokens := (builder.buffered + 3) / 4
	*builder.previous = chainedHash(builder.kind, tokens, xxhash.Sum64(builder.buffer[:builder.buffered]), builder.domain, *builder.previous)
	*builder.chain = append(*builder.chain, Block{Hash: *builder.previous, EstimatedTokens: tokens})
	*builder.totalTokens += tokens
	builder.buffered = 0
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

func (builder *tokenChainBuilder) appendNumberBytes(raw []byte) error {
	if builder.total >= MaxPrefixTokens {
		return errSemanticCap
	}
	var number json.Number
	if err := json.Unmarshal(bytes.TrimSpace(raw), &number); err != nil {
		return fmt.Errorf("prompt contains mixed types: %w", err)
	}
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
	builder.previous = chainedHash(segmentCompletionTokens, builder.buffered, contentHash, 0, builder.previous)
	builder.chain = append(builder.chain, Block{Hash: builder.previous, EstimatedTokens: builder.buffered})
	builder.buffered = 0
}

type boundedWriter struct {
	target    io.Writer
	remaining int
	written   int
}

type countingWriter struct{ written int }

func (writer *countingWriter) Write(data []byte) (int, error) {
	writer.written += len(data)
	return len(data), nil
}

func (writer *boundedWriter) Write(data []byte) (int, error) {
	original := len(data)
	if writer.remaining <= 0 {
		return original, nil
	}
	data = data[:min(len(data), writer.remaining)]
	n, err := writer.target.Write(data)
	writer.remaining -= n
	writer.written += n
	if err != nil {
		return 0, err
	}
	return original, nil
}

func (writer *boundedWriter) semanticCapReached() bool { return writer.remaining <= 0 }

func writeCanonicalJSON(raw []byte, writer io.Writer) error {
	if semanticCapReached(writer) {
		return errSemanticCap
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return fmt.Errorf("empty JSON value")
	}
	switch raw[0] {
	case '{':
		fields, err := parseObjectFields(raw)
		if err != nil {
			return err
		}
		return writeCanonicalObject(fields, writer)
	case '[':
		if _, err := writer.Write([]byte{'['}); err != nil {
			return err
		}
		first := true
		err := forEachArrayValue(raw, func(value []byte) error {
			if semanticCapReached(writer) {
				return errSemanticCap
			}
			if !first {
				if _, err := writer.Write([]byte{','}); err != nil {
					return err
				}
			}
			first = false
			return writeCanonicalJSON(value, writer)
		})
		if err != nil {
			return err
		}
		_, err = writer.Write([]byte{']'})
		return err
	case '"':
		return writeCanonicalJSONString(raw, writer)
	default:
		_, err := writer.Write(raw)
		return err
	}
}

func writeCanonicalObject(fields []jsonField, writer io.Writer) error {
	if semanticCapReached(writer) {
		return errSemanticCap
	}
	fields = append([]jsonField(nil), fields...)
	sort.Slice(fields, func(left, right int) bool { return fields[left].key < fields[right].key })
	if _, err := writer.Write([]byte{'{'}); err != nil {
		return err
	}
	for index, field := range fields {
		if semanticCapReached(writer) {
			return errSemanticCap
		}
		if index > 0 {
			if _, err := writer.Write([]byte{','}); err != nil {
				return err
			}
		}
		if err := writeCanonicalJSONString(field.rawKey, writer); err != nil {
			return err
		}
		if _, err := writer.Write([]byte{':'}); err != nil {
			return err
		}
		if err := writeCanonicalJSON(field.value, writer); err != nil {
			return err
		}
	}
	_, err := writer.Write([]byte{'}'})
	return err
}

func writeCanonicalJSONString(raw []byte, writer io.Writer) error {
	if semanticCapReached(writer) {
		return errSemanticCap
	}
	if _, err := writer.Write([]byte{'"'}); err != nil {
		return err
	}
	encoder := &canonicalStringWriter{target: writer}
	if err := writeDecodedJSONString(raw, encoder); err != nil {
		return err
	}
	_, err := writer.Write([]byte{'"'})
	return err
}

type canonicalStringWriter struct{ target io.Writer }

func (writer *canonicalStringWriter) semanticCapReached() bool {
	return semanticCapReached(writer.target)
}

func (writer *canonicalStringWriter) Write(data []byte) (int, error) {
	original := len(data)
	for len(data) > 0 {
		if writer.semanticCapReached() {
			return original - len(data), errSemanticCap
		}
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 {
			r = utf8.RuneError
		}
		data = data[size:]
		var encoded [12]byte
		var output []byte
		switch r {
		case '\\', '"':
			encoded[0], encoded[1] = '\\', byte(r)
			output = encoded[:2]
		case '\b':
			output = []byte(`\b`)
		case '\f':
			output = []byte(`\f`)
		case '\n':
			output = []byte(`\n`)
		case '\r':
			output = []byte(`\r`)
		case '\t':
			output = []byte(`\t`)
		default:
			if r < 0x20 || r == '<' || r == '>' || r == '&' || r == '\u2028' || r == '\u2029' {
				copy(encoded[:], `\u0000`)
				const hex = "0123456789abcdef"
				encoded[2] = hex[(r>>12)&0xf]
				encoded[3] = hex[(r>>8)&0xf]
				encoded[4] = hex[(r>>4)&0xf]
				encoded[5] = hex[r&0xf]
				output = encoded[:6]
			} else {
				size = utf8.EncodeRune(encoded[:], r)
				output = encoded[:size]
			}
		}
		if _, err := writer.target.Write(output); err != nil {
			return 0, err
		}
	}
	return original, nil
}

func semanticCapReached(writer io.Writer) bool {
	limiter, ok := writer.(interface{ semanticCapReached() bool })
	return ok && limiter.semanticCapReached()
}

func writeDecodedJSONString(raw []byte, writer io.Writer) error {
	raw = bytes.TrimSpace(raw)
	if !isJSONString(raw) {
		return fmt.Errorf("expected JSON string")
	}
	data := raw[1 : len(raw)-1]
	for len(data) > 0 {
		search := data
		if limiter, ok := writer.(interface{ inputLimit() int }); ok {
			limit := limiter.inputLimit()
			if limit <= 0 {
				return nil
			}
			search = data[:min(len(data), limit)]
		}
		escape := bytes.IndexByte(search, '\\')
		if escape < 0 {
			if _, err := writer.Write(search); err != nil {
				return err
			}
			data = data[len(search):]
			continue
		}
		if escape > 0 {
			if _, err := writer.Write(data[:escape]); err != nil {
				return err
			}
		}
		data = data[escape+1:]
		if len(data) == 0 {
			return fmt.Errorf("invalid string escape")
		}
		var decoded [4]byte
		var output []byte
		switch data[0] {
		case '"', '\\', '/':
			decoded[0], output = data[0], decoded[:1]
			data = data[1:]
		case 'b', 'f', 'n', 'r', 't':
			values := map[byte]byte{'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t'}
			decoded[0], output = values[data[0]], decoded[:1]
			data = data[1:]
		case 'u':
			r, consumed, err := decodeUnicodeEscape(data)
			if err != nil {
				return err
			}
			size := utf8.EncodeRune(decoded[:], r)
			output = decoded[:size]
			data = data[consumed:]
		default:
			return fmt.Errorf("invalid string escape")
		}
		if _, err := writer.Write(output); err != nil {
			return err
		}
	}
	return nil
}

func decodeUnicodeEscape(data []byte) (rune, int, error) {
	if len(data) < 5 || data[0] != 'u' {
		return 0, 0, fmt.Errorf("invalid unicode escape")
	}
	first, ok := parseHex4(data[1:5])
	if !ok {
		return 0, 0, fmt.Errorf("invalid unicode escape")
	}
	if first < 0xd800 || first > 0xdfff {
		return rune(first), 5, nil
	}
	if first > 0xdbff || len(data) < 11 || data[5] != '\\' || data[6] != 'u' {
		return utf8.RuneError, 5, nil
	}
	second, ok := parseHex4(data[7:11])
	if !ok || second < 0xdc00 || second > 0xdfff {
		return utf8.RuneError, 5, nil
	}
	return utf16.DecodeRune(rune(first), rune(second)), 11, nil
}

func parseHex4(data []byte) (uint16, bool) {
	var value uint16
	for _, digit := range data {
		value <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			value += uint16(digit - '0')
		case digit >= 'a' && digit <= 'f':
			value += uint16(digit-'a') + 10
		case digit >= 'A' && digit <= 'F':
			value += uint16(digit-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func parseObjectFields(raw []byte) ([]jsonField, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '{' {
		return nil, fmt.Errorf("expected JSON object")
	}
	position := 1
	fields := make([]jsonField, 0, 8)
	for {
		position = skipWhitespace(raw, position)
		if position >= len(raw) {
			return nil, fmt.Errorf("unterminated JSON object")
		}
		if raw[position] == '}' {
			return fields, nil
		}
		keyStart := position
		keyEnd, err := scanJSONString(raw, keyStart)
		if err != nil {
			return nil, err
		}
		key, err := decodeSmallString(raw[position:keyEnd])
		if err != nil {
			return nil, err
		}
		position = skipWhitespace(raw, keyEnd)
		if position >= len(raw) || raw[position] != ':' {
			return nil, fmt.Errorf("missing JSON object colon")
		}
		position = skipWhitespace(raw, position+1)
		valueEnd, err := scanJSONValue(raw, position)
		if err != nil {
			return nil, err
		}
		fields = append(fields, jsonField{key: key, rawKey: raw[keyStart:keyEnd], value: raw[position:valueEnd]})
		position = skipWhitespace(raw, valueEnd)
		if position < len(raw) && raw[position] == ',' {
			position++
			continue
		}
		if position < len(raw) && raw[position] == '}' {
			return fields, nil
		}
		return nil, fmt.Errorf("invalid JSON object separator")
	}
}

func forEachArrayValue(raw []byte, visit func([]byte) error) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '[' {
		return fmt.Errorf("expected JSON array")
	}
	position := 1
	for {
		position = skipWhitespace(raw, position)
		if position >= len(raw) {
			return fmt.Errorf("unterminated JSON array")
		}
		if raw[position] == ']' {
			return nil
		}
		end, err := scanJSONValue(raw, position)
		if err != nil {
			return err
		}
		if err := visit(raw[position:end]); err != nil {
			return err
		}
		position = skipWhitespace(raw, end)
		if position < len(raw) && raw[position] == ',' {
			position++
			continue
		}
		if position < len(raw) && raw[position] == ']' {
			return nil
		}
		return fmt.Errorf("invalid JSON array separator")
	}
}

func arrayHasValues(raw []byte) (bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '[' {
		return false, fmt.Errorf("expected JSON array")
	}
	position := skipWhitespace(raw, 1)
	if position >= len(raw) {
		return false, fmt.Errorf("unterminated JSON array")
	}
	return raw[position] != ']', nil
}

func scanJSONValue(raw []byte, start int) (int, error) {
	if start >= len(raw) {
		return 0, fmt.Errorf("missing JSON value")
	}
	if raw[start] == '"' {
		return scanJSONString(raw, start)
	}
	if raw[start] != '{' && raw[start] != '[' {
		end := start
		for end < len(raw) && !isValueDelimiter(raw[end]) {
			end++
		}
		if end == start {
			return 0, fmt.Errorf("invalid JSON value")
		}
		return end, nil
	}
	stack := []byte{raw[start]}
	for position := start + 1; position < len(raw); position++ {
		switch raw[position] {
		case '"':
			end, err := scanJSONString(raw, position)
			if err != nil {
				return 0, err
			}
			position = end - 1
		case '{', '[':
			stack = append(stack, raw[position])
		case '}', ']':
			expected := byte('{')
			if raw[position] == ']' {
				expected = '['
			}
			if len(stack) == 0 || stack[len(stack)-1] != expected {
				return 0, fmt.Errorf("mismatched JSON delimiter")
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return position + 1, nil
			}
		}
	}
	return 0, fmt.Errorf("unterminated JSON value")
}

func isValueDelimiter(value byte) bool {
	switch value {
	case ',', ']', '}', ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func scanJSONString(raw []byte, start int) (int, error) {
	if start >= len(raw) || raw[start] != '"' {
		return 0, fmt.Errorf("expected JSON string")
	}
	for position := start + 1; position < len(raw); position++ {
		switch raw[position] {
		case '\\':
			position++
		case '"':
			return position + 1, nil
		}
	}
	return 0, fmt.Errorf("unterminated JSON string")
}

func skipWhitespace(raw []byte, position int) int {
	for position < len(raw) {
		switch raw[position] {
		case ' ', '\t', '\r', '\n':
			position++
		default:
			return position
		}
	}
	return position
}

func decodeSmallString(raw []byte) (string, error) {
	var value string
	if err := json.Unmarshal(bytes.TrimSpace(raw), &value); err != nil {
		return "", err
	}
	return value, nil
}

func isJSONString(raw []byte) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"'
}

func fieldValue(fields []jsonField, name string) []byte {
	for _, field := range fields {
		if field.key == name {
			return field.value
		}
	}
	return nil
}

func fieldsWithout(fields []jsonField, name string) []jsonField {
	result := make([]jsonField, 0, len(fields))
	for _, field := range fields {
		if field.key != name {
			result = append(result, field)
		}
	}
	return result
}

func decodedStringLength(raw []byte) (int, error) {
	counter := &countingWriter{}
	if err := writeDecodedJSONString(raw, counter); err != nil {
		return 0, err
	}
	return counter.written, nil
}

func namespaceHash(modelRaw []byte, modelLength int, cacheSaltRaw []byte) (uint64, error) {
	hasher := xxhash.New()
	var length [8]byte
	binary.LittleEndian.PutUint64(length[:], uint64(modelLength))
	_, _ = hasher.Write(length[:])
	if err := writeDecodedJSONString(modelRaw, hasher); err != nil {
		return 0, err
	}
	cacheSaltLength := 0
	if cacheSaltRaw != nil {
		var err error
		cacheSaltLength, err = decodedStringLength(cacheSaltRaw)
		if err != nil {
			return 0, err
		}
	}
	binary.LittleEndian.PutUint64(length[:], uint64(cacheSaltLength))
	_, _ = hasher.Write(length[:])
	if cacheSaltRaw != nil {
		if err := writeDecodedJSONString(cacheSaltRaw, hasher); err != nil {
			return 0, err
		}
	}
	return hasher.Sum64(), nil
}

func chainedHash(kind segmentKind, tokens int, contentHash, domain, previous uint64) uint64 {
	var encoded [33]byte
	encoded[0] = byte(kind)
	binary.LittleEndian.PutUint64(encoded[1:9], uint64(tokens))
	binary.LittleEndian.PutUint64(encoded[9:17], contentHash)
	binary.LittleEndian.PutUint64(encoded[17:25], domain)
	binary.LittleEndian.PutUint64(encoded[25:33], previous)
	return xxhash.Sum64(encoded[:])
}
