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

package tools

import "testing"

func TestParseIPMalformedValueDoesNotPanic(t *testing.T) {
	// x-forwarded-for is read as a fallback when no socket IP is available,
	// so the value is client controlled.
	tests := []struct {
		name       string
		source     string
		fromHeader bool
		want       string
	}{
		{name: "closing bracket only (from header)", source: "]", fromHeader: true, want: ""},
		{name: "closing bracket with port (from header)", source: "]:80", fromHeader: true, want: ""},
		{name: "bracketed ipv6", source: "[::1]", fromHeader: false, want: "::1"},
		{name: "bracketed ipv6 with port", source: "[fe80::14d5:8aff:fed9:2114]:123", fromHeader: false, want: "fe80::14d5:8aff:fed9:2114"},
		{name: "ipv4", source: "127.0.0.1", fromHeader: false, want: "127.0.0.1"},
		{name: "ipv4 X-Forwarded-For first hop", source: "10.0.0.1, 10.0.0.2", fromHeader: true, want: "10.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseIP(tt.source, tt.fromHeader); got != tt.want {
				t.Fatalf("parseIP(%q, %v) = %q, want %q", tt.source, tt.fromHeader, got, tt.want)
			}
		})
	}
}
