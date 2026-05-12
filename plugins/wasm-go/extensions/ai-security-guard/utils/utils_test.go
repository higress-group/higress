package utils

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestReplaceJsonFieldTextContent(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		jsonPath   string
		newContent string
		wantCheck  func(t *testing.T, result []byte)
	}{
		{
			name:       "string content replaced directly",
			body:       `{"messages":[{"role":"user","content":"我的电话是13800138000"}]}`,
			jsonPath:   "messages.0.content",
			newContent: "我的电话是1**********",
			wantCheck: func(t *testing.T, result []byte) {
				got := gjson.GetBytes(result, "messages.0.content").String()
				if got != "我的电话是1**********" {
					t.Errorf("content = %q, want %q", got, "我的电话是1**********")
				}
			},
		},
		{
			name:       "array content preserves image_url items",
			body:       `{"messages":[{"role":"user","content":[{"type":"text","text":"我的电话是13800138000"},{"type":"image_url","image_url":{"url":"https://example.com/img.png"}}]}]}`,
			jsonPath:   "messages.0.content",
			newContent: "我的电话是1**********",
			wantCheck: func(t *testing.T, result []byte) {
				content := gjson.GetBytes(result, "messages.0.content")
				if !content.IsArray() {
					t.Fatal("content should remain an array")
				}
				items := content.Array()
				if len(items) != 2 {
					t.Fatalf("expected 2 items, got %d", len(items))
				}
				// text item updated
				if items[0].Get("type").String() != "text" {
					t.Error("first item type should be text")
				}
				if items[0].Get("text").String() != "我的电话是1**********" {
					t.Errorf("text = %q, want %q", items[0].Get("text").String(), "我的电话是1**********")
				}
				// image_url item preserved
				if items[1].Get("type").String() != "image_url" {
					t.Error("second item type should be image_url")
				}
				if items[1].Get("image_url.url").String() != "https://example.com/img.png" {
					t.Error("image_url should be preserved")
				}
			},
		},
		{
			name:       "array content with multiple text items",
			body:       `{"messages":[{"role":"user","content":[{"type":"text","text":"你好"},{"type":"text","text":"我的电话是13800138000"}]}]}`,
			jsonPath:   "messages.0.content",
			newContent: "你好我的电话是1**********",
			wantCheck: func(t *testing.T, result []byte) {
				content := gjson.GetBytes(result, "messages.0.content")
				if !content.IsArray() {
					t.Fatal("content should remain an array")
				}
				items := content.Array()
				if len(items) != 2 {
					t.Fatalf("expected 2 items, got %d", len(items))
				}
				// Both items should still be text type
				combined := items[0].Get("text").String() + items[1].Get("text").String()
				if combined != "你好我的电话是1**********" {
					t.Errorf("combined text = %q, want %q", combined, "你好我的电话是1**********")
				}
			},
		},
		{
			name:       "array content with only image items returns body unchanged",
			body:       `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}},{"type":"image_url","image_url":{"url":"https://example.com/b.png"}}]}]}`,
			jsonPath:   "messages.0.content",
			newContent: "masked",
			wantCheck: func(t *testing.T, result []byte) {
				content := gjson.GetBytes(result, "messages.0.content")
				items := content.Array()
				if len(items) != 2 {
					t.Fatalf("expected 2 items, got %d", len(items))
				}
				for _, item := range items {
					if item.Get("type").String() != "image_url" {
						t.Error("all items should remain image_url")
					}
				}
			},
		},
		{
			name:       "array content text before and after image",
			body:       `{"messages":[{"role":"user","content":[{"type":"text","text":"前缀"},{"type":"image_url","image_url":{"url":"https://img.com/1.png"}},{"type":"text","text":"后缀包含手机号13800138000"}]}]}`,
			jsonPath:   "messages.0.content",
			newContent: "前缀后缀包含手机号1**********",
			wantCheck: func(t *testing.T, result []byte) {
				content := gjson.GetBytes(result, "messages.0.content")
				items := content.Array()
				if len(items) != 3 {
					t.Fatalf("expected 3 items, got %d", len(items))
				}
				if items[0].Get("type").String() != "text" {
					t.Error("item 0 should be text")
				}
				if items[1].Get("type").String() != "image_url" {
					t.Error("item 1 should be image_url")
				}
				if items[1].Get("image_url.url").String() != "https://img.com/1.png" {
					t.Error("image_url should be preserved")
				}
				if items[2].Get("type").String() != "text" {
					t.Error("item 2 should be text")
				}
				combined := items[0].Get("text").String() + items[2].Get("text").String()
				if combined != "前缀后缀包含手机号1**********" {
					t.Errorf("combined text = %q, want %q", combined, "前缀后缀包含手机号1**********")
				}
			},
		},
		{
			name:       "resolveJsonPath with @reverse",
			body:       `{"messages":[{"role":"system","content":"sys"},{"role":"user","content":"我的电话是13800138000"}]}`,
			jsonPath:   "messages.@reverse.0.content",
			newContent: "我的电话是1**********",
			wantCheck: func(t *testing.T, result []byte) {
				// @reverse.0 should resolve to the last message (index 1)
				got := gjson.GetBytes(result, "messages.1.content").String()
				if got != "我的电话是1**********" {
					t.Errorf("content = %q, want %q", got, "我的电话是1**********")
				}
				// system message should be untouched
				sys := gjson.GetBytes(result, "messages.0.content").String()
				if sys != "sys" {
					t.Errorf("system content = %q, want %q", sys, "sys")
				}
			},
		},
		{
			name:       "multiple text items with CJK characters split at rune boundary",
			body:       `{"messages":[{"role":"user","content":[{"type":"text","text":"a"},{"type":"text","text":"bbbbbbbbb"}]}]}`,
			jsonPath:   "messages.0.content",
			newContent: "你好12345678",
			wantCheck: func(t *testing.T, result []byte) {
				content := gjson.GetBytes(result, "messages.0.content")
				items := content.Array()
				if len(items) != 2 {
					t.Fatalf("expected 2 items, got %d", len(items))
				}
				// Each segment must be valid UTF-8 with no truncated characters
				for i, item := range items {
					txt := item.Get("text").String()
					for _, r := range txt {
						if r == '\uFFFD' {
							t.Errorf("item %d contains replacement char U+FFFD, text=%q", i, txt)
						}
					}
				}
				combined := items[0].Get("text").String() + items[1].Get("text").String()
				if combined != "你好12345678" {
					t.Errorf("combined text = %q, want %q", combined, "你好12345678")
				}
			},
		},
		{
			name:       "multiple empty text items with non-empty newContent no panic",
			body:       `{"messages":[{"role":"user","content":[{"type":"text","text":""},{"type":"text","text":""},{"type":"image_url","image_url":{"url":"https://img.com/1.png"}}]}]}`,
			jsonPath:   "messages.0.content",
			newContent: "脱敏后的内容abc",
			wantCheck: func(t *testing.T, result []byte) {
				content := gjson.GetBytes(result, "messages.0.content")
				items := content.Array()
				if len(items) != 3 {
					t.Fatalf("expected 3 items, got %d", len(items))
				}
				// image_url item preserved
				if items[2].Get("type").String() != "image_url" {
					t.Error("item 2 should be image_url")
				}
				// All newContent must be distributed across the two text items
				combined := items[0].Get("text").String() + items[1].Get("text").String()
				if combined != "脱敏后的内容abc" {
					t.Errorf("combined text = %q, want %q", combined, "脱敏后的内容abc")
				}
			},
		},
		{
			name:       "resolveJsonPath with @reverse and array content",
			body:       `{"messages":[{"role":"system","content":"sys"},{"role":"user","content":[{"type":"text","text":"敏感内容"},{"type":"image_url","image_url":{"url":"https://img.com/x.png"}}]}]}`,
			jsonPath:   "messages.@reverse.0.content",
			newContent: "脱敏内容",
			wantCheck: func(t *testing.T, result []byte) {
				content := gjson.GetBytes(result, "messages.1.content")
				if !content.IsArray() {
					t.Fatal("content should remain an array")
				}
				items := content.Array()
				if len(items) != 2 {
					t.Fatalf("expected 2 items, got %d", len(items))
				}
				if items[0].Get("text").String() != "脱敏内容" {
					t.Errorf("text = %q, want %q", items[0].Get("text").String(), "脱敏内容")
				}
				if items[1].Get("image_url.url").String() != "https://img.com/x.png" {
					t.Error("image_url should be preserved")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ReplaceJsonFieldTextContent([]byte(tt.body), tt.jsonPath, tt.newContent)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Verify result is valid JSON
			if !gjson.ValidBytes(result) {
				t.Fatal("result is not valid JSON")
			}
			tt.wantCheck(t, result)
		})
	}
}

// TestResolveJsonPathEdgeCases covers edge cases in resolveJsonPath
func TestResolveJsonPathEdgeCases(t *testing.T) {
	// @reverse with index exceeding array length → actualIdx clamped to 0
	t.Run("@reverse out of bounds index clamps to 0", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
		// Array has 1 element (index 0). @reverse.5 → actualIdx = 0 - 5 = -5 → clamped to 0
		result, err := ReplaceJsonFieldTextContent(body, "messages.@reverse.5.content", "replaced")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !gjson.ValidBytes(result) {
			t.Fatal("result is not valid JSON")
		}
		got := gjson.GetBytes(result, "messages.0.content").String()
		if got != "replaced" {
			t.Errorf("content = %q, want %q", got, "replaced")
		}
	})

	// @reverse on empty array → actualIdx clamped to 0
	t.Run("@reverse on single-element array resolves correctly", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"user","content":"only one"}]}`)
		// Array has 1 element. @reverse.0 → actualIdx = 1 - 1 - 0 = 0
		result, err := ReplaceJsonFieldTextContent(body, "messages.@reverse.0.content", "updated")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := gjson.GetBytes(result, "messages.0.content").String()
		if got != "updated" {
			t.Errorf("content = %q, want %q", got, "updated")
		}
	})
}

// TestReplaceJsonFieldContent covers the simple ReplaceJsonFieldContent function
func TestReplaceJsonFieldContent(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"original"}]}`)
	result, err := ReplaceJsonFieldContent(body, "messages.0.content", "replaced")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := gjson.GetBytes(result, "messages.0.content").String()
	if got != "replaced" {
		t.Errorf("content = %q, want %q", got, "replaced")
	}
}
