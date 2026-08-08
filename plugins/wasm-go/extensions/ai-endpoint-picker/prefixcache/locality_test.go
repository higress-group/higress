package prefixcache

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func extractForTest(t *testing.T, body string) *Locality {
	t.Helper()
	locality, supported, err := Extract([]byte(body))
	if err != nil || !supported {
		t.Fatalf("extract failed: supported=%v err=%v", supported, err)
	}
	return locality
}

func TestChatUsesWholeCanonicalMessageSegments(t *testing.T) {
	base := extractForTest(t, `{"model":"m","messages":[{"role":"user","name":"alice","content":"hello","tool_calls":[{"id":"1","type":"function"}]}]}`)
	if got := len(base.Chains[0]); got != 1 {
		t.Fatalf("small complete message produced %d blocks, want 1", got)
	}
	reordered := extractForTest(t, `{"messages":[{"tool_calls":[{"type":"function","id":"1"}],"content":"hello","name":"alice","role":"user"}],"model":"m"}`)
	if base.Chains[0][0].Hash != reordered.Chains[0][0].Hash {
		t.Fatal("canonical message hash changed with JSON field order")
	}
	changedName := extractForTest(t, `{"model":"m","messages":[{"role":"user","name":"bob","content":"hello","tool_calls":[{"id":"1","type":"function"}]}]}`)
	if base.Chains[0][0].Hash == changedName.Chains[0][0].Hash {
		t.Fatal("prompt-relevant message fields must affect the hash")
	}
}

func TestChatToolsAreSeparateOrderedCanonicalSegment(t *testing.T) {
	first := extractForTest(t, `{"model":"m","tools":[{"type":"function","function":{"name":"lookup","parameters":{"b":2,"a":1}}}],"messages":[{"role":"user","content":"hello"}]}`)
	second := extractForTest(t, `{"messages":[{"content":"hello","role":"user"}],"tools":[{"function":{"parameters":{"a":1,"b":2},"name":"lookup"},"type":"function"}],"model":"m"}`)
	if len(first.Chains[0]) != 2 {
		t.Fatalf("tools plus message produced %d blocks, want 2", len(first.Chains[0]))
	}
	for index := range first.Chains[0] {
		if first.Chains[0][index].Hash != second.Chains[0][index].Hash {
			t.Fatalf("canonical block %d differs after field reorder", index)
		}
	}
}

func TestLongMessageSplitsAtBoundedSemanticSize(t *testing.T) {
	content := strings.Repeat("a", MaxSegmentTokens*4*2)
	body, _ := json.Marshal(map[string]any{
		"model":    "m",
		"messages": []any{map[string]any{"role": "user", "content": content}},
	})
	locality := extractForTest(t, string(body))
	if got := len(locality.Chains[0]); got != 3 {
		t.Fatalf("long canonical message produced %d blocks, want 3", got)
	}
	for _, block := range locality.Chains[0] {
		if block.EstimatedTokens <= 0 || block.EstimatedTokens > MaxSegmentTokens {
			t.Fatalf("unbounded segment length %d", block.EstimatedTokens)
		}
	}
}

func TestLongMessageMetadataChangesInvalidateFirstSegment(t *testing.T) {
	content := strings.Repeat("a", MaxSegmentTokens*4*3)
	tests := []struct {
		name  string
		first map[string]any
		other map[string]any
	}{
		{
			name:  "role",
			first: map[string]any{"role": "user", "content": content},
			other: map[string]any{"role": "system", "content": content},
		},
		{
			name:  "name",
			first: map[string]any{"role": "user", "name": "alice", "content": content},
			other: map[string]any{"role": "user", "name": "bob", "content": content},
		},
		{
			name: "tool_calls",
			first: map[string]any{
				"role": "assistant", "content": content,
				"tool_calls": []any{map[string]any{"id": "one", "type": "function"}},
			},
			other: map[string]any{
				"role": "assistant", "content": content,
				"tool_calls": []any{map[string]any{"id": "two", "type": "function"}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			firstBody, _ := json.Marshal(map[string]any{"model": "m", "messages": []any{test.first}})
			otherBody, _ := json.Marshal(map[string]any{"model": "m", "messages": []any{test.other}})
			first := extractForTest(t, string(firstBody))
			other := extractForTest(t, string(otherBody))
			if len(first.Chains[0]) < 3 || len(first.Chains[0]) != len(other.Chains[0]) {
				t.Fatalf("unexpected long-message chains: %d vs %d", len(first.Chains[0]), len(other.Chains[0]))
			}
			if first.Chains[0][0].Hash == other.Chains[0][0].Hash {
				t.Fatal("message framing metadata did not isolate the first segment")
			}
			index := NewIndex(DefaultCapacity)
			index.Record("a", first.Chains, DefaultBlockSize)
			if score := index.Score("a", other.Chains); score != 0 {
				t.Fatalf("metadata change retained a false prefix score %v", score)
			}
		})
	}
}

func TestLongMessageContentSuffixRetainsEarlierChunks(t *testing.T) {
	prefix := strings.Repeat("a", MaxSegmentTokens*4*2)
	firstBody, _ := json.Marshal(map[string]any{
		"model": "m", "messages": []any{map[string]any{"role": "user", "name": "same", "content": prefix + "first suffix"}},
	})
	otherBody, _ := json.Marshal(map[string]any{
		"model": "m", "messages": []any{map[string]any{"name": "same", "content": prefix + "other suffix", "role": "user"}},
	})
	first := extractForTest(t, string(firstBody))
	other := extractForTest(t, string(otherBody))
	if len(first.Chains[0]) != 3 || len(other.Chains[0]) != 3 {
		t.Fatalf("unexpected suffix chains: %d vs %d", len(first.Chains[0]), len(other.Chains[0]))
	}
	for block := 0; block < 2; block++ {
		if first.Chains[0][block].Hash != other.Chains[0][block].Hash {
			t.Fatalf("unchanged content prefix block %d did not match", block)
		}
	}
	if first.Chains[0][2].Hash == other.Chains[0][2].Hash {
		t.Fatal("changed content suffix retained the final block")
	}
}

func TestCompletionsTextTokenIDsAndMultiplePrompts(t *testing.T) {
	text := extractForTest(t, `{"model":"m","prompt":"hello"}`)
	if len(text.Chains) != 1 || len(text.Chains[0]) != 1 || text.Chains[0][0].EstimatedTokens != 2 {
		t.Fatalf("text completion locality=%+v", text.Chains)
	}
	tokens := extractForTest(t, `{"model":"m","prompt":[1,2,3]}`)
	if len(tokens.Chains) != 1 || tokens.Chains[0][0].EstimatedTokens != 3 {
		t.Fatalf("token completion locality=%+v", tokens.Chains)
	}
	multiple := extractForTest(t, `{"model":"m","prompt":["first","second"]}`)
	if len(multiple.Chains) != 2 || len(multiple.Chains[0]) != 1 || len(multiple.Chains[1]) != 1 {
		t.Fatalf("multiple prompts did not create independent chains: %+v", multiple.Chains)
	}
}

func TestNamespaceSegmentKindAndOutputIsolation(t *testing.T) {
	base := extractForTest(t, `{"model":"m","cache_salt":"a","prompt":"same","temperature":0}`)
	outputChanged := extractForTest(t, `{"model":"m","cache_salt":"a","prompt":"same","temperature":1,"max_tokens":999}`)
	otherModel := extractForTest(t, `{"model":"other","cache_salt":"a","prompt":"same"}`)
	otherSalt := extractForTest(t, `{"model":"m","cache_salt":"b","prompt":"same"}`)
	ambiguousA := extractForTest(t, `{"model":"ab","cache_salt":"c","prompt":"same"}`)
	ambiguousB := extractForTest(t, `{"model":"a","cache_salt":"bc","prompt":"same"}`)
	chat := extractForTest(t, `{"model":"m","cache_salt":"a","messages":[{"role":"user","content":"same"}]}`)
	baseHash := base.Chains[0][0].Hash
	if baseHash != outputChanged.Chains[0][0].Hash {
		t.Fatal("output-generation fields affected prefix hash")
	}
	if baseHash == otherModel.Chains[0][0].Hash || baseHash == otherSalt.Chains[0][0].Hash || baseHash == chat.Chains[0][0].Hash {
		t.Fatal("namespace or segment kind did not isolate prefix hash")
	}
	if ambiguousA.Chains[0][0].Hash == ambiguousB.Chains[0][0].Hash {
		t.Fatal("model and cache_salt namespace components were not length-delimited")
	}
}

func TestEscapedStringsAndKeysCanonicalizeDeterministically(t *testing.T) {
	plain := extractForTest(t, `{"model":"m","messages":[{"role":"user","content":"hello 世界"}]}`)
	escaped := extractForTest(t, `{"model":"m","messages":[{"r\u006fle":"user","content":"\u0068ello \u4e16\u754c"}]}`)
	if plain.Chains[0][0].Hash != escaped.Chains[0][0].Hash {
		t.Fatal("equivalent JSON string/key escapes produced different semantic hashes")
	}
}

func TestMessageContentRepresentationIsDomainSeparated(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
	}{
		{
			name:  "absent versus empty string",
			left:  `{"model":"m","messages":[{"role":"user"}]}`,
			right: `{"model":"m","messages":[{"role":"user","content":""}]}`,
		},
		{
			name:  "null versus null string",
			left:  `{"model":"m","messages":[{"role":"user","content":null}]}`,
			right: `{"model":"m","messages":[{"role":"user","content":"null"}]}`,
		},
		{
			name:  "structured versus matching plain string",
			left:  `{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`,
			right: `{"model":"m","messages":[{"role":"user","content":"[{\"text\":\"hello\",\"type\":\"text\"}]"}]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			left := extractForTest(t, test.left)
			right := extractForTest(t, test.right)
			if left.Chains[0][0].Hash == right.Chains[0][0].Hash {
				t.Fatal("different content representations produced the same first hash")
			}
		})
	}

	ordered := extractForTest(t, `{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	reordered := extractForTest(t, `{"messages":[{"content":[{"text":"hello","type":"text"}],"role":"user"}],"model":"m"}`)
	if ordered.Chains[0][0].Hash != reordered.Chains[0][0].Hash {
		t.Fatal("structured content hash changed with JSON field order")
	}
}

func TestPrefixTokenCapDoesNotCreateTinyBlocks(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"model":  "m",
		"prompt": strings.Repeat("a", (MaxPrefixTokens+MaxSegmentTokens)*4),
	})
	locality := extractForTest(t, string(body))
	if got, want := len(locality.Chains[0]), MaxPrefixTokens/MaxSegmentTokens; got != want {
		t.Fatalf("block count=%d want %d", got, want)
	}
	total := 0
	for _, block := range locality.Chains[0] {
		total += block.EstimatedTokens
	}
	if total != MaxPrefixTokens {
		t.Fatalf("estimated tokens=%d want capped %d", total, MaxPrefixTokens)
	}
}

func TestTokenIDPromptIsStreamedAndCapped(t *testing.T) {
	var body strings.Builder
	body.WriteString(`{"model":"m","prompt":[`)
	for token := 0; token < MaxPrefixTokens+MaxSegmentTokens; token++ {
		if token > 0 {
			body.WriteByte(',')
		}
		body.WriteString(strconv.Itoa(token))
	}
	body.WriteString(`]}`)
	locality := extractForTest(t, body.String())
	if got, want := len(locality.Chains[0]), MaxPrefixTokens/MaxSegmentTokens; got != want {
		t.Fatalf("token-ID block count=%d want %d", got, want)
	}
	if got := totalTokens(locality.Chains); got != MaxPrefixTokens {
		t.Fatalf("token-ID count=%d want capped %d", got, MaxPrefixTokens)
	}
}

func TestMultiMegabyteTextPromptsRemainAllocationBounded(t *testing.T) {
	content := strings.Repeat("a", 4<<20)
	tests := []struct {
		name string
		body []byte
	}{
		{name: "completion", body: mustJSON(t, map[string]any{"model": "m", "prompt": content})},
		{name: "chat", body: mustJSON(t, map[string]any{
			"model": "m", "messages": []any{map[string]any{"role": "user", "content": content}},
		})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			locality, supported, err := Extract(test.body)
			if err != nil || !supported {
				t.Fatalf("extract: supported=%v err=%v", supported, err)
			}
			if got := len(locality.Chains[0]); got != MaxPrefixTokens/MaxSegmentTokens {
				t.Fatalf("semantic segment count=%d want %d", got, MaxPrefixTokens/MaxSegmentTokens)
			}
			if got := totalTokens(locality.Chains); got != MaxPrefixTokens {
				t.Fatalf("token count=%d want %d", got, MaxPrefixTokens)
			}
			if allocations := testing.AllocsPerRun(3, func() {
				_, _, _ = Extract(test.body)
			}); allocations > 100 {
				t.Fatalf("allocations=%v want bounded <=100", allocations)
			}
		})
	}
}

func TestCappedSemanticSuffixesDoNotScaleAllocations(t *testing.T) {
	tests := []struct {
		name     string
		capped   []byte
		extended []byte
	}{
		{
			name:     "token IDs",
			capped:   tokenIDBody(MaxPrefixTokens),
			extended: tokenIDBody(MaxPrefixTokens + 1_000_000),
		},
		{
			name:     "messages",
			capped:   repeatedChatBody(140, 0),
			extended: repeatedChatBody(140, 100_000),
		},
		{
			name:     "tools",
			capped:   repeatedToolsBody(140, 0),
			extended: repeatedToolsBody(140, 100_000),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, body := range [][]byte{test.capped, test.extended} {
				locality, supported, err := Extract(body)
				if err != nil || !supported {
					t.Fatalf("extract: supported=%v err=%v", supported, err)
				}
				if got := totalTokens(locality.Chains); got != MaxPrefixTokens {
					t.Fatalf("token count=%d want capped %d", got, MaxPrefixTokens)
				}
			}
			cappedAllocs := testing.AllocsPerRun(3, func() { _, _, _ = Extract(test.capped) })
			extendedAllocs := testing.AllocsPerRun(3, func() { _, _, _ = Extract(test.extended) })
			t.Logf("allocations capped=%v extended=%v", cappedAllocs, extendedAllocs)
			if extendedAllocs > cappedAllocs+8 {
				t.Fatalf("allocations scaled after cap: capped=%v extended=%v", cappedAllocs, extendedAllocs)
			}
		})
	}
}

func tokenIDBody(count int) []byte {
	var body strings.Builder
	body.Grow(count*2 + 30)
	body.WriteString(`{"model":"m","prompt":[`)
	for token := 0; token < count; token++ {
		if token > 0 {
			body.WriteByte(',')
		}
		body.WriteByte(byte('0' + token%10))
	}
	body.WriteString(`]}`)
	return []byte(body.String())
}

func repeatedChatBody(prefixMessages, suffixMessages int) []byte {
	const prefix = `{"role":"user","content":"`
	const suffix = `"}`
	const small = `{"role":"user","content":"x"}`
	var body strings.Builder
	body.Grow(prefixMessages*(len(prefix)+maxSegmentBytes+len(suffix)) + suffixMessages*(len(small)+1) + 40)
	body.WriteString(`{"model":"m","messages":[`)
	for message := 0; message < prefixMessages+suffixMessages; message++ {
		if message > 0 {
			body.WriteByte(',')
		}
		if message < prefixMessages {
			body.WriteString(prefix)
			body.WriteString(strings.Repeat("a", maxSegmentBytes))
			body.WriteString(suffix)
		} else {
			body.WriteString(small)
		}
	}
	body.WriteString(`]}`)
	return []byte(body.String())
}

func repeatedToolsBody(prefixTools, suffixTools int) []byte {
	const prefix = `{"type":"function","function":{"name":"f","description":"`
	const suffix = `"}}`
	const small = `{"type":"function","function":{"name":"f"}}`
	var body strings.Builder
	body.Grow(prefixTools*(len(prefix)+maxSegmentBytes+len(suffix)) + suffixTools*(len(small)+1) + 60)
	body.WriteString(`{"model":"m","tools":[`)
	for tool := 0; tool < prefixTools+suffixTools; tool++ {
		if tool > 0 {
			body.WriteByte(',')
		}
		if tool < prefixTools {
			body.WriteString(prefix)
			body.WriteString(strings.Repeat("a", maxSegmentBytes))
			body.WriteString(suffix)
		} else {
			body.WriteString(small)
		}
	}
	body.WriteString(`],"messages":[]}`)
	return []byte(body.String())
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestUnsupportedMultimodalOnlyDisablesPrefix(t *testing.T) {
	tests := []string{
		`{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image_url","image_url":{"url":"x"}}]}]}`,
		`{"model":"m","messages":[{"role":"assistant","content":null,"audio":{"id":"audio-1"}}]}`,
	}
	for _, body := range tests {
		_, supported, err := Extract([]byte(body))
		if err != nil || supported {
			t.Fatalf("multimodal request should be unavailable without error: supported=%v err=%v", supported, err)
		}
	}

	_, supported, err := Extract([]byte(`{"model":"m","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function"}]}]}`))
	if err != nil || !supported {
		t.Fatalf("normal tool_calls must remain supported: supported=%v err=%v", supported, err)
	}
}
