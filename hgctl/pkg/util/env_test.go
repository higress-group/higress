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

package util

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		name string
		v1   string
		v2   string
		want int
	}{
		{"equal", "3.12.0", "3.12.0", 0},
		{"equal with padding", "3.12", "3.12.0", 0},
		{"newer patch", "3.12.1", "3.12", 1},
		{"newer minor", "3.13", "3.12", 1},
		{"older minor", "3.9", "3.12", -1},
		{"leading text is ignored", "v3.12", "3.12", 0},
		// The doc comment says "3.12.0b1" is read as "3.12.0"; the old regex
		// produced [3 12 0 1] and reported the pre-release as newer.
		{"pre-release is not a component", "3.12.0b1", "3.12.0", 0},
		{"pre-release vs older minor", "3.12.0b1", "3.11", 1},
		{"no digits", "", "3.12", -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CompareVersions(c.v1, c.v2); got != c.want {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.v1, c.v2, got, c.want)
			}
		})
	}
}
