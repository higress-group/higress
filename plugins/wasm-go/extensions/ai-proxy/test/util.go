package test

import (
	"encoding/json"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
)

// LegacyProviderPluginJSON builds the top-level plugin JSON with a single legacy "provider"
// object, matching historical wasm integration tests (see test/openai.go).
func LegacyProviderPluginJSON(provider map[string]interface{}) json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{"provider": provider})
	return json.RawMessage(data)
}

func RunMapRequestPathByCapabilityTests(t *testing.T) {
	testCases := []struct {
		name     string
		apiName  string
		origin   string
		mapping  map[string]string
		expected string
	}{
		{
			name:     "no mapping returns empty",
			apiName:  "openai/v1/chatcompletions",
			origin:   "/v1/chat/completions",
			mapping:  map[string]string{},
			expected: "",
		},
		{
			name:    "file placeholder is replaced",
			apiName: "openai/v1/retrievefile",
			origin:  "/openai/v1/files/file-abc",
			mapping: map[string]string{
				"openai/v1/retrievefile": "/v1/files/{file_id}",
			},
			expected: "/v1/files/file-abc",
		},
		{
			name:    "file content keeps query parameters",
			apiName: "openai/v1/retrievefilecontent",
			origin:  "/openai/v1/files/file-123/content?variant=thumbnail",
			mapping: map[string]string{
				"openai/v1/retrievefilecontent": "/v1/files/{file_id}/content",
			},
			expected: "/v1/files/file-123/content?variant=thumbnail",
		},
		{
			name:    "file content merges query string with mapped query",
			apiName: "openai/v1/retrievefilecontent",
			origin:  "/openai/v1/files/file-123/content?variant=thumbnail",
			mapping: map[string]string{
				"openai/v1/retrievefilecontent": "/v1/files/{file_id}/content?download=1",
			},
			expected: "/v1/files/file-123/content?download=1&variant=thumbnail",
		},
		{
			name:    "retrieve batch replaces batch id",
			apiName: "openai/v1/retrievebatch",
			origin:  "/openai/v1/batches/batch-001",
			mapping: map[string]string{
				"openai/v1/retrievebatch": "/v1/batches/{batch_id}",
			},
			expected: "/v1/batches/batch-001",
		},
		{
			name:    "cancel batch replaces batch id",
			apiName: "openai/v1/cancelbatch",
			origin:  "/openai/v1/batches/batch-002/cancel",
			mapping: map[string]string{
				"openai/v1/cancelbatch": "/v1/batches/{batch_id}/cancel",
			},
			expected: "/v1/batches/batch-002/cancel",
		},
		{
			name:    "video placeholder is replaced",
			apiName: "openai/v1/retrievevideo",
			origin:  "/openai/v1/videos/video-xyz",
			mapping: map[string]string{
				"openai/v1/retrievevideo": "/v1/videos/{video_id}",
			},
			expected: "/v1/videos/video-xyz",
		},
		{
			name:    "video placeholder is replaced in nested provider path",
			apiName: "openai/v1/retrievevideo",
			origin:  "/openai/v1/videos/video-xyz",
			mapping: map[string]string{
				"openai/v1/retrievevideo": "/v1/videos/text2video/{video_id}",
			},
			expected: "/v1/videos/text2video/video-xyz",
		},
		{
			name:    "video content placeholder with query",
			apiName: "openai/v1/retrievevideocontent",
			origin:  "/openai/v1/videos/video-xyz/content?variant=thumbnail",
			mapping: map[string]string{
				"openai/v1/retrievevideocontent": "/v1/videos/{video_id}/content",
			},
			expected: "/v1/videos/video-xyz/content?variant=thumbnail",
		},
		{
			name:    "video remix placeholder is replaced",
			apiName: "openai/v1/videoremix",
			origin:  "/openai/v1/videos/video-xyz/remix",
			mapping: map[string]string{
				"openai/v1/videoremix": "/v1/videos/{video_id}/remix",
			},
			expected: "/v1/videos/video-xyz/remix",
		},
		{
			name:    "non placeholder mapping returns mapped path directly",
			apiName: "openai/v1/videos",
			origin:  "/openai/v1/videos",
			mapping: map[string]string{
				"openai/v1/videos": "/v1/videos",
			},
			expected: "/v1/videos",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := util.MapRequestPathByCapability(tc.apiName, tc.origin, tc.mapping)
			if got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
