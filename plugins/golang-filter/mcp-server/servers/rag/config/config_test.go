// Copyright (c) 2022 Alibaba Group Holding Ltd.
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

package config

import "testing"

// The Params maps are populated from Envoy's TypedStruct (structpb.AsMap),
// where every JSON number arrives as float64. These tests pin that the int
// accessors accept that type, so user-configured index/search parameters are
// not silently discarded in favor of the hardcoded defaults.
func TestIndexConfigParamsInt64AcceptsFloat64(t *testing.T) {
	cfg := IndexConfig{Params: map[string]interface{}{
		"nlist":          float64(256), // as decoded from JSON/structpb
		"M":              float64(32),
		"efConstruction": float64(128),
		"nbits":          float64(16),
		"int64":          int64(7),
		"int":            int(9),
		"missing":        "not a number",
	}}

	for key, want := range map[string]int64{
		"nlist": 256, "M": 32, "efConstruction": 128, "nbits": 16,
		"int64": 7, "int": 9,
	} {
		got, err := cfg.ParamsInt64(key)
		if err != nil {
			t.Fatalf("ParamsInt64(%q) returned error: %v", key, err)
		}
		if got != want {
			t.Errorf("ParamsInt64(%q) = %d, want %d", key, got, want)
		}
	}

	if _, err := cfg.ParamsInt64("missing"); err == nil {
		t.Error("ParamsInt64(missing) = no error, want error")
	}
	if _, err := cfg.ParamsInt64("absent"); err == nil {
		t.Error("ParamsInt64(absent) = no error, want error")
	}
}

func TestSearchConfigParamsInt64AcceptsFloat64(t *testing.T) {
	cfg := SearchConfig{Params: map[string]interface{}{
		"reorder_k":   float64(64),
		"search_list": float64(200),
		"level":       float64(12),
		"int64":       int64(5),
		"bad":         "x",
	}}

	for key, want := range map[string]int64{
		"reorder_k": 64, "search_list": 200, "level": 12, "int64": 5,
	} {
		got, err := cfg.ParamsInt64(key)
		if err != nil {
			t.Fatalf("ParamsInt64(%q) returned error: %v", key, err)
		}
		if got != want {
			t.Errorf("ParamsInt64(%q) = %d, want %d", key, got, want)
		}
	}

	if _, err := cfg.ParamsInt64("bad"); err == nil {
		t.Error("ParamsInt64(bad) = no error, want error")
	}
}

func TestFieldMappingMaxLengthAcceptsFloat64(t *testing.T) {
	tests := []struct {
		name       string
		properties map[string]interface{}
		want       int
	}{
		{name: "float64 from JSON", properties: map[string]interface{}{"max_length": float64(512)}, want: 512},
		{name: "plain int", properties: map[string]interface{}{"max_length": int(64)}, want: 64},
		{name: "missing falls back to default", properties: map[string]interface{}{"auto_id": true}, want: 256},
		{name: "wrong type falls back to default", properties: map[string]interface{}{"max_length": "128"}, want: 256},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := FieldMapping{StandardName: "content", Properties: tt.properties}
			if got := f.MaxLength(); got != tt.want {
				t.Errorf("MaxLength() = %d, want %d", got, tt.want)
			}
		})
	}

	var nilProps FieldMapping
	if got := nilProps.MaxLength(); got != 0 {
		t.Errorf("MaxLength() with nil Properties = %d, want 0", got)
	}
}
