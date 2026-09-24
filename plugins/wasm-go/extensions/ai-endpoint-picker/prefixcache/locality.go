package prefixcache

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/cespare/xxhash/v2"
)

const (
	MaxSegmentTokens       = 1024
	MaxPrefixTokens        = 131072
	DefaultMaxBlocks       = 32
	MaxBlocksLimit         = 128
	DefaultBlockSizeTokens = MaxSegmentTokens
	MaxTools               = 64
	// MaxToolIdentityBytes bounds the total type/name data hashed in identity mode.
	MaxToolIdentityBytes = 8192
	DefaultCapacity      = 31250
	DefaultBlockSize     = 16
	maxSegmentBytes      = MaxSegmentTokens * 4
	// Over-budget tool schemas fall back to non-prefix scheduling.
	maxCanonicalNodes = 16384
	maxJSONDepth      = 64
)

type ToolMode string

const (
	ToolModeNone     ToolMode = "none"
	ToolModeIdentity ToolMode = "identity"
	ToolModeFull     ToolMode = "full"
	DefaultToolMode           = ToolModeIdentity
)

type Options struct {
	ToolMode        ToolMode
	MaxBlocks       int
	BlockSizeTokens int
}

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
	_, locality, supported, err := InspectRequest(body)
	return locality, supported, err
}

// ExtractWithToolMode derives locality using the requested tools precision.
func ExtractWithToolMode(body []byte, toolMode ToolMode) (*Locality, bool, error) {
	_, locality, supported, err := InspectRequestWithToolMode(body, toolMode)
	return locality, supported, err
}

// ExtractWithOptions derives locality using explicit bounded extraction options.
func ExtractWithOptions(body []byte, options Options) (*Locality, bool, error) {
	_, locality, supported, err := InspectRequestWithOptions(body, options)
	return locality, supported, err
}

// InspectRequest validates the bounded request envelope, extracts the model,
// and derives approximate prefix locality. Requests beyond the canonical depth
// limit retain a safely extracted top-level model but disable prefix locality.
func InspectRequest(body []byte) (string, *Locality, bool, error) {
	return InspectRequestWithOptions(body, Options{})
}

// InspectRequestWithToolMode validates the request and controls how top-level
// tools contribute to the approximate prefix. Callers must validate toolMode.
func InspectRequestWithToolMode(body []byte, toolMode ToolMode) (string, *Locality, bool, error) {
	return InspectRequestWithOptions(body, Options{ToolMode: toolMode})
}

// InspectRequestWithOptions validates the request and applies a request-wide
// block budget across tools, messages, and every Completion prompt chain.
func InspectRequestWithOptions(body []byte, options Options) (string, *Locality, bool, error) {
	if options.ToolMode == "" {
		options.ToolMode = DefaultToolMode
	}
	if options.MaxBlocks == 0 {
		options.MaxBlocks = DefaultMaxBlocks
	}
	if options.BlockSizeTokens == 0 {
		options.BlockSizeTokens = DefaultBlockSizeTokens
	}
	if options.ToolMode != ToolModeNone && options.ToolMode != ToolModeIdentity && options.ToolMode != ToolModeFull {
		return "", nil, false, fmt.Errorf("unsupported tool mode %q", options.ToolMode)
	}
	if options.MaxBlocks < 1 || options.MaxBlocks > MaxBlocksLimit {
		return "", nil, false, fmt.Errorf("max blocks must be in [1,%d]", MaxBlocksLimit)
	}
	if options.BlockSizeTokens < 1 || options.BlockSizeTokens > MaxSegmentTokens {
		return "", nil, false, fmt.Errorf("block size tokens must be in [1,%d]", MaxSegmentTokens)
	}
	start := skipWhitespace(body, 0)
	if start >= len(body) || body[start] != '{' {
		return "", nil, false, fmt.Errorf("expected JSON object")
	}
	end, err := scanJSONValue(body, start)
	if errors.Is(err, errJSONDepthLimit) {
		model, modelErr := extractModelFromDeepEnvelope(body)
		if modelErr != nil {
			return "", nil, false, modelErr
		}
		return model, nil, false, nil
	}
	if err != nil || skipWhitespace(body, end) != len(body) || !json.Valid(body) {
		return "", nil, false, fmt.Errorf("invalid JSON request")
	}
	return inspectValidatedRequest(body, options)
}

func inspectValidatedRequest(body []byte, options Options) (string, *Locality, bool, error) {
	fields, err := parseObjectFields(body)
	if err != nil {
		return "", nil, false, err
	}
	modelRaw := fieldValue(fields, "model")
	model, err := decodeSmallString(modelRaw)
	if err != nil || model == "" {
		return "", nil, false, fmt.Errorf("model is required")
	}
	modelLength := len(model)
	cacheSaltRaw := fieldValue(fields, "cache_salt")
	if bytes.Equal(bytes.TrimSpace(cacheSaltRaw), []byte("null")) {
		cacheSaltRaw = nil
	}
	seed, err := namespaceHash(modelRaw, modelLength, cacheSaltRaw)
	if err != nil {
		return "", nil, false, err
	}
	budget := &blockBudget{remaining: options.MaxBlocks, blockSizeTokens: options.BlockSizeTokens}
	if messages := fieldValue(fields, "messages"); messages != nil {
		chains, supported, err := extractChat(messages, fieldValue(fields, "tools"), seed, options.ToolMode, budget)
		if errors.Is(err, errCanonicalComplexity) || errors.Is(err, errJSONDepthLimit) {
			return model, nil, false, nil
		}
		if err != nil || !supported {
			return model, nil, supported, err
		}
		return model, &Locality{Chains: chains}, true, nil
	}
	if prompt := fieldValue(fields, "prompt"); prompt != nil {
		chains, supported, err := extractCompletion(prompt, seed, budget)
		if errors.Is(err, errCanonicalComplexity) || errors.Is(err, errJSONDepthLimit) {
			return model, nil, false, nil
		}
		if err != nil || !supported {
			return model, nil, supported, err
		}
		return model, &Locality{Chains: chains}, true, nil
	}
	return model, nil, false, nil
}

func extractChat(rawMessages, rawTools []byte, seed uint64, toolMode ToolMode, budget *blockBudget) ([][]Block, bool, error) {
	chain := make([]Block, 0)
	previous, totalTokens := seed, 0
	messageCanonicalNodes := maxCanonicalNodes
	if toolMode != ToolModeNone && len(rawTools) > 0 && !bytes.Equal(bytes.TrimSpace(rawTools), []byte("null")) {
		hasValues, err := arrayHasValues(rawTools)
		if err != nil {
			return nil, false, err
		}
		if hasValues {
			builder := newSegmentBuilder(&chain, segmentTools, 0, &previous, &totalTokens, budget)
			switch toolMode {
			case ToolModeIdentity:
				if err := writeToolIdentities(rawTools, builder); err != nil && !errors.Is(err, errSemanticCap) {
					return nil, false, err
				}
			case ToolModeFull:
				writer := &canonicalBudgetWriter{target: builder, remaining: maxCanonicalNodes}
				if err := writeCanonicalJSON(rawTools, writer); errors.Is(err, errCanonicalComplexity) || errors.Is(err, errJSONDepthLimit) {
					return nil, false, nil
				} else if err != nil && !errors.Is(err, errSemanticCap) {
					return nil, false, err
				}
			default:
				return nil, false, fmt.Errorf("unsupported tool mode %q", toolMode)
			}
			builder.finish()
		}
	}
	err := forEachArrayValueWhile(rawMessages, func() bool {
		return totalTokens < MaxPrefixTokens && !budget.exhausted()
	}, func(rawMessage []byte) error {
		if messageCanonicalNodes <= 1 {
			return errCanonicalComplexity
		}
		fields, err := parseObjectFieldsLimited(rawMessage, messageCanonicalNodes-1)
		if err != nil {
			return err
		}
		messageCanonicalNodes -= len(fields) + 1
		if hasTopLevelMultimodal(fields) {
			return errUnsupportedContent
		}
		content, contentKind, supported := classifyMessageContent(fieldValue(fields, "content"))
		if !supported {
			return errUnsupportedContent
		}
		metadata := fieldsWithout(fields, "content")
		metadataLimit := min((MaxPrefixTokens-totalTokens)*4, budget.remainingBytes(0))
		domainHasher := xxhash.New()
		_, _ = domainHasher.Write([]byte{byte(contentKind)})
		domainWriter := &boundedWriter{target: domainHasher, remaining: metadataLimit}
		domainCanonical := &canonicalBudgetWriter{target: domainWriter, remaining: messageCanonicalNodes}
		if err := writeCanonicalObject(metadata, domainCanonical); err != nil && !errors.Is(err, errSemanticCap) {
			return err
		}
		metadataNodes := messageCanonicalNodes - domainCanonical.remaining
		messageCanonicalNodes = domainCanonical.remaining
		builder := newSegmentBuilder(&chain, segmentMessage, domainHasher.Sum64(), &previous, &totalTokens, budget)
		metadataWriter := &boundedWriter{target: builder, remaining: domainWriter.written}
		metadataCanonical := &canonicalBudgetWriter{target: metadataWriter, remaining: metadataNodes}
		if err := writeCanonicalObject(metadata, metadataCanonical); err != nil {
			if errors.Is(err, errSemanticCap) {
				builder.finish()
			}
			return err
		}
		contentCanonical := &canonicalBudgetWriter{target: builder, remaining: messageCanonicalNodes}
		if err := writeMessageContent(content, contentKind, contentCanonical); err != nil {
			if errors.Is(err, errSemanticCap) {
				builder.finish()
			}
			return err
		}
		messageCanonicalNodes = contentCanonical.remaining
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

func writeToolIdentities(rawTools []byte, writer io.Writer) error {
	tools, identityBytes := 0, 0
	err := forEachArrayValueWhile(rawTools, func() bool {
		return !semanticCapReached(writer)
	}, func(rawTool []byte) error {
		if tools >= MaxTools || MaxToolIdentityBytes-identityBytes < 9 {
			return errToolIdentityCap
		}
		typeRaw, functionRaw, err := toolIdentityFields(rawTool)
		if err != nil {
			return err
		}
		nameRaw, err := functionNameField(functionRaw)
		if err != nil {
			return err
		}
		remaining := MaxToolIdentityBytes - identityBytes - 9
		typeLength, err := optionalDecodedStringLength(typeRaw, remaining)
		if err != nil {
			if errors.Is(err, errToolIdentityCap) {
				return errToolIdentityCap
			}
			return err
		}
		nameLength, err := optionalDecodedStringLength(nameRaw, remaining-typeLength)
		if err != nil {
			if errors.Is(err, errToolIdentityCap) {
				return errToolIdentityCap
			}
			return err
		}
		encodedLength := 9 + typeLength + nameLength
		if encodedLength > MaxToolIdentityBytes-identityBytes {
			return errToolIdentityCap
		}
		var framing [9]byte
		if len(typeRaw) > 0 {
			framing[0] |= 1
		}
		if len(nameRaw) > 0 {
			framing[0] |= 2
		}
		binary.LittleEndian.PutUint32(framing[1:5], uint32(typeLength))
		binary.LittleEndian.PutUint32(framing[5:9], uint32(nameLength))
		if _, err := writer.Write(framing[:]); err != nil {
			return err
		}
		if len(typeRaw) > 0 {
			if err := writeDecodedJSONString(typeRaw, writer); err != nil {
				return err
			}
		}
		if len(nameRaw) > 0 {
			if err := writeDecodedJSONString(nameRaw, writer); err != nil {
				return err
			}
		}
		tools++
		identityBytes += encodedLength
		return nil
	})
	if errors.Is(err, errToolIdentityCap) {
		return nil
	}
	return err
}

func toolIdentityFields(rawTool []byte) ([]byte, []byte, error) {
	typeRaw, err := findObjectField(rawTool, "type")
	if err != nil {
		return nil, nil, err
	}
	functionRaw, err := findObjectField(rawTool, "function")
	if err != nil {
		return nil, nil, err
	}
	return normalizeOptionalString(typeRaw), bytes.TrimSpace(functionRaw), nil
}

func functionNameField(rawFunction []byte) ([]byte, error) {
	if len(rawFunction) == 0 || bytes.Equal(rawFunction, []byte("null")) {
		return nil, nil
	}
	nameRaw, err := findObjectField(rawFunction, "name")
	if err != nil {
		return nil, err
	}
	return normalizeOptionalString(nameRaw), nil
}

func normalizeOptionalString(raw []byte) []byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	return raw
}

func optionalDecodedStringLength(raw []byte, limit int) (int, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	if !isJSONString(raw) {
		return 0, fmt.Errorf("tool identity field must be a string")
	}
	counter := &identityLengthWriter{remaining: limit}
	if err := writeDecodedJSONString(raw, counter); err != nil {
		return 0, err
	}
	return counter.written, nil
}

type identityLengthWriter struct {
	remaining int
	written   int
}

func (writer *identityLengthWriter) Write(data []byte) (int, error) {
	if len(data) > writer.remaining {
		return 0, errToolIdentityCap
	}
	writer.remaining -= len(data)
	writer.written += len(data)
	return len(data), nil
}

// One extra byte lets writeDecodedJSONString detect rather than silently stop
// at the identity budget.
func (writer *identityLengthWriter) inputLimit() int { return writer.remaining + 1 }

var errUnsupportedContent = errors.New("unsupported non-text content")
var errSemanticCap = errors.New("semantic prefix cap reached")
var errCanonicalComplexity = errors.New("canonical JSON complexity limit exceeded")
var errJSONDepthLimit = errors.New("JSON nesting depth limit exceeded")
var errToolIdentityCap = errors.New("tool identity cap reached")

func hasTopLevelMultimodal(fields []jsonField) bool {
	for _, name := range []string{"audio", "input_audio", "image", "image_url", "video", "video_url"} {
		value := bytes.TrimSpace(fieldValue(fields, name))
		if len(value) > 0 && !bytes.Equal(value, []byte("null")) {
			return true
		}
	}
	return false
}

func classifyMessageContent(raw []byte) ([]byte, messageContentKind, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, messageContentAbsent, true
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return trimmed, messageContentNull, true
	}
	if isJSONString(trimmed) {
		return trimmed, messageContentString, true
	}
	if trimmed[0] != '[' {
		return nil, 0, false
	}
	return trimmed, messageContentStructuredText, true
}

func writeMessageContent(content []byte, kind messageContentKind, writer io.Writer) error {
	switch kind {
	case messageContentAbsent:
		return nil
	case messageContentString:
		return writeDecodedJSONString(content, writer)
	case messageContentNull:
		_, err := writer.Write(content)
		return err
	case messageContentStructuredText:
		return writeStructuredTextContent(content, writer)
	default:
		return errUnsupportedContent
	}
}

func writeStructuredTextContent(raw []byte, writer io.Writer) error {
	if !consumeCanonicalNodes(writer, 1) {
		return errCanonicalComplexity
	}
	if _, err := writer.Write([]byte{'['}); err != nil {
		return err
	}
	first := true
	err := forEachArrayValueWhile(raw, func() bool {
		return !semanticCapReached(writer)
	}, func(rawPart []byte) error {
		if !first {
			if _, err := writer.Write([]byte{','}); err != nil {
				return err
			}
			if semanticCapReached(writer) {
				return errSemanticCap
			}
		}
		first = false
		remaining := canonicalNodesRemaining(writer)
		if remaining == 0 {
			return errCanonicalComplexity
		}
		maxFields := remaining - 1
		if remaining < 0 {
			maxFields = -1
		}
		fields, err := parseObjectFieldsLimited(rawPart, maxFields)
		if err != nil {
			return err
		}
		if !consumeCanonicalNodes(writer, len(fields)+1) {
			return errCanonicalComplexity
		}
		partType, err := decodeSmallString(fieldValue(fields, "type"))
		if err != nil || partType != "text" || !isJSONString(fieldValue(fields, "text")) {
			return errUnsupportedContent
		}
		return writeCanonicalObject(fields, writer)
	})
	if err != nil {
		return err
	}
	_, err = writer.Write([]byte{']'})
	return err
}

func extractCompletion(raw []byte, seed uint64, budget *blockBudget) ([][]Block, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if isJSONString(trimmed) {
		chain := make([]Block, 0)
		previous, totalTokens := seed, 0
		builder := newSegmentBuilder(&chain, segmentCompletionText, 0, &previous, &totalTokens, budget)
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
	err := forEachArrayValueWhile(trimmed, func() bool {
		return !budget.exhausted()
	}, func(item []byte) error {
		item = bytes.TrimSpace(item)
		if mode == 0 {
			switch {
			case isJSONString(item):
				mode = 's'
			case len(item) > 0 && item[0] == '[':
				mode = 'b'
			default:
				mode = 'n'
				tokenBuilder = newTokenChainBuilder(seed, budget)
			}
		}
		if mode == 's' {
			if !isJSONString(item) {
				return fmt.Errorf("prompt contains mixed types")
			}
			chain := make([]Block, 0)
			previous, totalTokens := seed, 0
			builder := newSegmentBuilder(&chain, segmentCompletionText, 0, &previous, &totalTokens, budget)
			if err := writeDecodedJSONString(item, builder); err != nil {
				return err
			}
			builder.finish()
			chains = append(chains, chain)
			return nil
		}
		if mode == 'b' {
			if len(item) == 0 || item[0] != '[' {
				return fmt.Errorf("prompt contains mixed types")
			}
			return validateTokenIDArray(item)
		}
		if isJSONString(item) || (len(item) > 0 && item[0] == '[') {
			return fmt.Errorf("prompt contains mixed types")
		}
		if tokenBuilder.total >= MaxPrefixTokens || budget.exhausted() {
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
	if mode == 'b' {
		return nil, false, nil
	}
	return chains, true, nil
}

func validateTokenIDArray(raw []byte) error {
	return forEachArrayValue(raw, func(item []byte) error {
		if _, ok := parseUint32(item); !ok {
			return fmt.Errorf("prompt token ID is invalid")
		}
		return nil
	})
}

type segmentBuilder struct {
	chain       *[]Block
	kind        segmentKind
	domain      uint64
	previous    *uint64
	totalTokens *int
	budget      *blockBudget
	remaining   int
	buffer      [maxSegmentBytes]byte
	buffered    int
}

func newSegmentBuilder(chain *[]Block, kind segmentKind, domain uint64, previous *uint64, totalTokens *int, budget *blockBudget) *segmentBuilder {
	return &segmentBuilder{
		chain: chain, kind: kind, domain: domain, previous: previous, totalTokens: totalTokens,
		budget: budget, remaining: (MaxPrefixTokens - *totalTokens) * 4,
	}
}

func (builder *segmentBuilder) Write(data []byte) (int, error) {
	original := len(data)
	limit := builder.inputLimit()
	if limit <= 0 {
		return original, nil
	}
	data = data[:min(len(data), limit)]
	builder.remaining -= len(data)
	for len(data) > 0 {
		blockSizeBytes := builder.budget.blockSizeBytes()
		copied := copy(builder.buffer[builder.buffered:blockSizeBytes], data)
		builder.buffered += copied
		data = data[copied:]
		if builder.buffered == blockSizeBytes {
			builder.flush()
			if builder.budget.exhausted() {
				break
			}
		}
	}
	return original, nil
}

func (builder *segmentBuilder) inputLimit() int {
	return min(builder.remaining, builder.budget.remainingBytes(builder.buffered))
}

func (builder *segmentBuilder) semanticCapReached() bool {
	return builder.remaining <= 0 || builder.budget.exhausted()
}

func (builder *segmentBuilder) finish() { builder.flush() }

func (builder *segmentBuilder) flush() {
	if builder.buffered == 0 || builder.budget.exhausted() {
		builder.buffered = 0
		return
	}
	tokens := (builder.buffered + 3) / 4
	*builder.previous = chainedHash(builder.kind, tokens, xxhash.Sum64(builder.buffer[:builder.buffered]), builder.domain, *builder.previous)
	*builder.chain = append(*builder.chain, Block{Hash: *builder.previous, EstimatedTokens: tokens})
	*builder.totalTokens += tokens
	builder.budget.consume()
	builder.buffered = 0
}

type tokenChainBuilder struct {
	chain    []Block
	buffer   [MaxSegmentTokens]uint32
	buffered int
	total    int
	previous uint64
	budget   *blockBudget
}

func newTokenChainBuilder(seed uint64, budget *blockBudget) *tokenChainBuilder {
	return &tokenChainBuilder{previous: seed, budget: budget}
}

func (builder *tokenChainBuilder) appendNumberBytes(raw []byte) error {
	if builder.total >= MaxPrefixTokens || builder.budget.exhausted() {
		return errSemanticCap
	}
	value, ok := parseUint32(raw)
	if !ok {
		return fmt.Errorf("prompt token ID is invalid")
	}
	builder.buffer[builder.buffered] = value
	builder.buffered++
	builder.total++
	if builder.buffered == builder.budget.blockTokens() {
		builder.flush()
	}
	return nil
}

func parseUint32(raw []byte) (uint32, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || (len(raw) > 1 && raw[0] == '0') {
		return 0, false
	}
	var value uint32
	for _, digit := range raw {
		if digit < '0' || digit > '9' {
			return 0, false
		}
		next := uint32(digit - '0')
		if value > (^uint32(0)-next)/10 {
			return 0, false
		}
		value = value*10 + next
	}
	return value, true
}

func (builder *tokenChainBuilder) finish() []Block {
	builder.flush()
	return builder.chain
}

func (builder *tokenChainBuilder) flush() {
	if builder.buffered == 0 || builder.budget.exhausted() {
		builder.buffered = 0
		return
	}
	var encoded [MaxSegmentTokens * 4]byte
	for index, token := range builder.buffer[:builder.buffered] {
		binary.LittleEndian.PutUint32(encoded[index*4:], token)
	}
	contentHash := xxhash.Sum64(encoded[:builder.buffered*4])
	builder.previous = chainedHash(segmentCompletionTokens, builder.buffered, contentHash, 0, builder.previous)
	builder.chain = append(builder.chain, Block{Hash: builder.previous, EstimatedTokens: builder.buffered})
	builder.budget.consume()
	builder.buffered = 0
}

type blockBudget struct {
	remaining       int
	blockSizeTokens int
}

func (budget *blockBudget) exhausted() bool { return budget == nil || budget.remaining <= 0 }

func (budget *blockBudget) consume() {
	if budget.remaining > 0 {
		budget.remaining--
	}
}

func (budget *blockBudget) remainingBytes(buffered int) int {
	if budget.exhausted() {
		return 0
	}
	return budget.remaining*budget.blockSizeBytes() - buffered
}

func (budget *blockBudget) blockTokens() int {
	if budget == nil || budget.blockSizeTokens <= 0 {
		return DefaultBlockSizeTokens
	}
	return budget.blockSizeTokens
}

func (budget *blockBudget) blockSizeBytes() int { return budget.blockTokens() * 4 }

type boundedWriter struct {
	target    io.Writer
	remaining int
	written   int
}

type canonicalBudgetWriter struct {
	target    io.Writer
	remaining int
}

func (writer *canonicalBudgetWriter) Write(data []byte) (int, error) {
	return writer.target.Write(data)
}

func (writer *canonicalBudgetWriter) semanticCapReached() bool {
	return semanticCapReached(writer.target)
}

func (writer *canonicalBudgetWriter) inputLimit() int { return writerInputLimit(writer.target) }

func (writer *canonicalBudgetWriter) consumeCanonicalNodes(count int) bool {
	if count > writer.remaining {
		return false
	}
	writer.remaining -= count
	return true
}

func (writer *canonicalBudgetWriter) canonicalNodesRemaining() int { return writer.remaining }

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

func (writer *boundedWriter) inputLimit() int { return writer.remaining }

func writeCanonicalJSON(raw []byte, writer io.Writer) error {
	return writeCanonicalJSONAtDepth(raw, writer, 1)
}

func writeCanonicalJSONAtDepth(raw []byte, writer io.Writer, depth int) error {
	if semanticCapReached(writer) {
		return errSemanticCap
	}
	if !consumeCanonicalNodes(writer, 1) {
		return errCanonicalComplexity
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return fmt.Errorf("empty JSON value")
	}
	switch raw[0] {
	case '{':
		if depth > maxJSONDepth {
			return errJSONDepthLimit
		}
		fields, err := parseObjectFieldsLimited(raw, canonicalNodesRemaining(writer))
		if err != nil {
			return err
		}
		if !consumeCanonicalNodes(writer, len(fields)) {
			return errCanonicalComplexity
		}
		return writeCanonicalObjectAtDepth(fields, writer, depth)
	case '[':
		if depth > maxJSONDepth {
			return errJSONDepthLimit
		}
		if _, err := writer.Write([]byte{'['}); err != nil {
			return err
		}
		first := true
		err := forEachArrayValueWhile(raw, func() bool {
			return !semanticCapReached(writer)
		}, func(value []byte) error {
			if !first {
				if _, err := writer.Write([]byte{','}); err != nil {
					return err
				}
			}
			first = false
			return writeCanonicalJSONAtDepth(value, writer, depth+1)
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
	return writeCanonicalObjectAtDepth(fields, writer, 1)
}

func writeCanonicalObjectAtDepth(fields []jsonField, writer io.Writer, depth int) error {
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
		if err := writeCanonicalJSONAtDepth(field.value, writer, depth+1); err != nil {
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

func (writer *canonicalStringWriter) inputLimit() int { return writerInputLimit(writer.target) }

func (writer *canonicalStringWriter) Write(data []byte) (int, error) {
	original := len(data)
	for len(data) > 0 {
		if writer.semanticCapReached() {
			return original - len(data), errSemanticCap
		}
		run := 0
		for run < len(data) {
			r, size := utf8.DecodeRune(data[run:])
			if canonicalRuneNeedsEscape(r, size) {
				break
			}
			run += size
		}
		if run > 0 {
			if _, err := writer.target.Write(data[:run]); err != nil {
				return original - len(data), err
			}
			data = data[run:]
			continue
		}
		r, size := utf8.DecodeRune(data)
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

func canonicalRuneNeedsEscape(r rune, size int) bool {
	return (r == utf8.RuneError && size == 1) || r == '\\' || r == '"' || r < 0x20 ||
		r == '<' || r == '>' || r == '&' || r == '\u2028' || r == '\u2029'
}

func semanticCapReached(writer io.Writer) bool {
	limiter, ok := writer.(interface{ semanticCapReached() bool })
	return ok && limiter.semanticCapReached()
}

func consumeCanonicalNodes(writer io.Writer, count int) bool {
	limiter, ok := writer.(interface{ consumeCanonicalNodes(int) bool })
	return !ok || limiter.consumeCanonicalNodes(count)
}

func canonicalNodesRemaining(writer io.Writer) int {
	limiter, ok := writer.(interface{ canonicalNodesRemaining() int })
	if !ok {
		return -1
	}
	return limiter.canonicalNodesRemaining()
}

func writerInputLimit(writer io.Writer) int {
	limiter, ok := writer.(interface{ inputLimit() int })
	if !ok {
		return int(^uint(0) >> 1)
	}
	return limiter.inputLimit()
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
	return parseObjectFieldsLimited(raw, -1)
}

// findObjectField scans only as far as the requested field and returns a span
// into raw. In particular, identity-mode tool parsing does not materialize or
// canonicalize description and schema subtrees that follow type/name.
func findObjectField(raw []byte, name string) ([]byte, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '{' {
		return nil, fmt.Errorf("expected JSON object")
	}
	position := 1
	for {
		position = skipWhitespace(raw, position)
		if position >= len(raw) {
			return nil, fmt.Errorf("unterminated JSON object")
		}
		if raw[position] == '}' {
			return nil, nil
		}
		keyEnd, err := scanJSONString(raw, position)
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
		if key == name {
			return raw[position:valueEnd], nil
		}
		position = skipWhitespace(raw, valueEnd)
		if position < len(raw) && raw[position] == ',' {
			position++
			continue
		}
		if position < len(raw) && raw[position] == '}' {
			return nil, nil
		}
		return nil, fmt.Errorf("invalid JSON object separator")
	}
}

func parseObjectFieldsLimited(raw []byte, maxFields int) ([]jsonField, error) {
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
		if maxFields >= 0 && len(fields) >= maxFields {
			return nil, errCanonicalComplexity
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
	return forEachArrayValueWhile(raw, nil, visit)
}

func forEachArrayValueWhile(raw []byte, shouldContinue func() bool, visit func([]byte) error) error {
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
		if shouldContinue != nil && !shouldContinue() {
			return errSemanticCap
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
	var stack [maxJSONDepth]byte
	stack[0] = raw[start]
	depth := 1
	for position := start + 1; position < len(raw); position++ {
		switch raw[position] {
		case '"':
			end, err := scanJSONString(raw, position)
			if err != nil {
				return 0, err
			}
			position = end - 1
		case '{', '[':
			if depth == len(stack) {
				return 0, errJSONDepthLimit
			}
			stack[depth] = raw[position]
			depth++
		case '}', ']':
			expected := byte('{')
			if raw[position] == ']' {
				expected = '['
			}
			if depth == 0 || stack[depth-1] != expected {
				return 0, fmt.Errorf("mismatched JSON delimiter")
			}
			depth--
			if depth == 0 {
				return position + 1, nil
			}
		}
	}
	return 0, fmt.Errorf("unterminated JSON value")
}

func extractModelFromDeepEnvelope(raw []byte) (string, error) {
	position := skipWhitespace(raw, 0)
	if position >= len(raw) || raw[position] != '{' {
		return "", fmt.Errorf("expected JSON object")
	}
	position++
	model := ""
	for {
		position = skipWhitespace(raw, position)
		if position >= len(raw) {
			return "", fmt.Errorf("unterminated JSON object")
		}
		if raw[position] == '}' {
			position = skipWhitespace(raw, position+1)
			if position != len(raw) || model == "" {
				return "", fmt.Errorf("model is required")
			}
			return model, nil
		}
		keyEnd, err := scanJSONString(raw, position)
		if err != nil {
			return "", err
		}
		key, err := decodeSmallString(raw[position:keyEnd])
		if err != nil {
			return "", err
		}
		position = skipWhitespace(raw, keyEnd)
		if position >= len(raw) || raw[position] != ':' {
			return "", fmt.Errorf("missing JSON object colon")
		}
		position = skipWhitespace(raw, position+1)
		valueEnd, err := scanJSONValueUnbounded(raw, position)
		if err != nil {
			return "", err
		}
		if key == "model" {
			model, err = decodeSmallString(raw[position:valueEnd])
			if err != nil || model == "" {
				return "", fmt.Errorf("model is required")
			}
		}
		position = skipWhitespace(raw, valueEnd)
		if position < len(raw) && raw[position] == ',' {
			position++
			continue
		}
		if position < len(raw) && raw[position] == '}' {
			continue
		}
		return "", fmt.Errorf("invalid JSON object separator")
	}
}

// scanJSONValueUnbounded is used only after the strict preflight has already
// classified the request as over-depth. It keeps an integer nesting counter so
// locating later top-level fields remains linear and allocation-free.
func scanJSONValueUnbounded(raw []byte, start int) (int, error) {
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
	depth := 1
	for position := start + 1; position < len(raw); position++ {
		switch raw[position] {
		case '"':
			end, err := scanJSONString(raw, position)
			if err != nil {
				return 0, err
			}
			position = end - 1
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
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
