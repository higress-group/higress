package prefixcache

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractChatAndCompletions(t *testing.T) {
	chat, supported, err := Extract([]byte(`{"model":"m","messages":[{"role":"user","content":"hello"}],"temperature":0.9}`))
	if err != nil || !supported {
		t.Fatalf("chat extraction failed: supported=%v err=%v", supported, err)
	}
	if got := len(chat.Prompts[0]); got != 3 {
		t.Fatalf("role and text should be estimated in four-byte tokens, got %d", got)
	}
	completion, supported, err := Extract([]byte(`{"model":"m","prompt":[1,2,3],"max_tokens":1}`))
	if err != nil || !supported {
		t.Fatalf("completion extraction failed: supported=%v err=%v", supported, err)
	}
	if got := completion.Prompts[0]; len(got) != 3 || got[2] != 3 {
		t.Fatalf("pre-tokenized prompt not preserved: %v", got)
	}
}

func TestOutputParametersDoNotAffectPrefix(t *testing.T) {
	first, _, _ := Extract([]byte(`{"model":"m","prompt":"same","temperature":0,"max_tokens":1}`))
	second, _, _ := Extract([]byte(`{"model":"m","prompt":"same","temperature":1,"max_tokens":999}`))
	if first.BlockChains(64)[0][0] != second.BlockChains(64)[0][0] {
		t.Fatal("output-generation parameters changed locality hash")
	}
}

func TestEstimateTokenizerPacksUTF8Bytes(t *testing.T) {
	locality, supported, err := Extract([]byte(`{"model":"m","prompt":"abcde"}`))
	if err != nil || !supported {
		t.Fatalf("extraction failed: supported=%v err=%v", supported, err)
	}
	if got, want := locality.Prompts[0][0], binary.LittleEndian.Uint32([]byte("abcd")); got != want {
		t.Fatalf("first token=%d want %d", got, want)
	}
	if got := locality.Prompts[0][1]; got != uint32('e') {
		t.Fatalf("zero-padded tail token=%d want %d", got, 'e')
	}
}

func TestUnsupportedMultimodalOnlyDisablesPrefix(t *testing.T) {
	_, supported, err := Extract([]byte(`{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image_url","image_url":{"url":"x"}}]}]}`))
	if err != nil || supported {
		t.Fatalf("multimodal request should be unavailable without error: supported=%v err=%v", supported, err)
	}
}

func TestBlockChainsUseNamespaceAndConfiguredBlockSize(t *testing.T) {
	text := strings.Repeat("a", 300)
	base, _, _ := Extract([]byte(`{"model":"m","cache_salt":"a","prompt":"` + text + `"}`))
	otherSalt, _, _ := Extract([]byte(`{"model":"m","cache_salt":"b","prompt":"` + text + `"}`))
	if got := len(base.BlockChains(16)[0]); got != 2 {
		t.Fatalf("block_size 16 should floor to 64 tokens, got %d blocks", got)
	}
	if got := len(base.BlockChains(128)[0]); got != 1 {
		t.Fatalf("block_size 128 should remain 128 tokens, got %d blocks", got)
	}
	if base.BlockChains(64)[0][0] == otherSalt.BlockChains(64)[0][0] {
		t.Fatal("cache_salt must change the prefix namespace")
	}
}

func TestBlockChainsCapPrefixTokens(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"model":  "m",
		"prompt": strings.Repeat("a", (MaxPrefixTokens+MinBlockSizeTokens)*4),
	})
	if err != nil {
		t.Fatal(err)
	}
	locality, supported, err := Extract(body)
	if err != nil || !supported {
		t.Fatalf("extract: supported=%v err=%v", supported, err)
	}
	if got, want := len(locality.BlockChains(MinBlockSizeTokens)[0]), MaxPrefixTokens/MinBlockSizeTokens; got != want {
		t.Fatalf("block count=%d want capped %d", got, want)
	}
}
