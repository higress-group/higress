// Copyright (c) 2024 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"encoding/json"
	"testing"
)

func TestBuildBedrockImageGenerationRequestSize(t *testing.T) {
	b := &bedrockProvider{}
	cases := []struct {
		size  string
		wantW int
		wantH int
	}{
		{"1024x1024", 1024, 1024},
		{"512x768", 512, 768},
		{"", 1024, 1024},
		{"1024", 1024, 1024},
		{"abc", 1024, 1024},
		// A partially malformed size used to send the malformed half as 0.
		{"1024xabc", 1024, 1024},
		{"abcx1024", 1024, 1024},
		{"0x100", 1024, 100},
	}
	for _, c := range cases {
		out, err := b.buildBedrockImageGenerationRequest(
			&imageGenerationRequest{Prompt: "p", Size: c.size}, nil)
		if err != nil {
			t.Fatalf("size %q: unexpected error: %v", c.size, err)
		}
		var got bedrockImageGenerationRequest
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("size %q: cannot decode request: %v", c.size, err)
		}
		if got.ImageGenerationConfig == nil {
			t.Fatalf("size %q: no image generation config", c.size)
		}
		if got.ImageGenerationConfig.Width != c.wantW || got.ImageGenerationConfig.Height != c.wantH {
			t.Errorf("size %q: got %dx%d, want %dx%d", c.size,
				got.ImageGenerationConfig.Width, got.ImageGenerationConfig.Height, c.wantW, c.wantH)
		}
	}
}
