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

package annotations

import "testing"

func TestNeedRegexMatch(t *testing.T) {
	testCases := []struct {
		input     *Ingress
		inputPath string
		expect    bool
	}{
		{
			input:  &Ingress{},
			expect: false,
		},
		{
			input: &Ingress{
				Rewrite: &RewriteConfig{},
			},
			expect: false,
		},
		{
			input: &Ingress{
				Rewrite: &RewriteConfig{
					UseRegex: true,
				},
			},
			expect: true,
		},
		{
			input: &Ingress{
				Rewrite: &RewriteConfig{
					UseRegex: false,
				},
			},
			expect: false,
		},
		{
			input: &Ingress{
				Rewrite: &RewriteConfig{
					UseRegex:      false,
					RewriteTarget: "/$1",
				},
			},
			expect: true,
		},
		{
			input: &Ingress{
				Rewrite: &RewriteConfig{
					UseRegex:      false,
					RewriteTarget: "/",
				},
			},
			inputPath: "/.*",
			expect:    true,
		},
		{
			input: &Ingress{
				Rewrite: &RewriteConfig{
					UseRegex: false,
				},
			},
			inputPath: "/.",
			expect:    false,
		},
		{
			input: &Ingress{
				Rewrite: &RewriteConfig{
					UseRegex:      false,
					RewriteTarget: "/",
				},
			},
			inputPath: "/",
			expect:    false,
		},
	}

	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			if testCase.input.NeedRegexMatch(testCase.inputPath) != testCase.expect {
				t.Fatalf("Should be %t, but actual is %t", testCase.expect, !testCase.expect)
			}
		})
	}
}

func TestIsCanary(t *testing.T) {
	testCases := []struct {
		input  *Ingress
		expect bool
	}{
		{
			input:  &Ingress{},
			expect: false,
		},
		{
			input: &Ingress{
				Canary: &CanaryConfig{},
			},
			expect: false,
		},
		{
			input: &Ingress{
				Canary: &CanaryConfig{
					Enabled: true,
				},
			},
			expect: true,
		},
	}

	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			if testCase.input.IsCanary() != testCase.expect {
				t.Fatalf("Should be %t, but actual is %t", testCase.expect, testCase.input.IsCanary())
			}
		})
	}
}

func TestCanaryKind(t *testing.T) {
	testCases := []struct {
		input    *Ingress
		byHeader bool
		byWeight bool
	}{
		{
			input:    &Ingress{},
			byHeader: false,
			byWeight: false,
		},
		{
			input: &Ingress{
				Canary: &CanaryConfig{},
			},
			byHeader: false,
			byWeight: false,
		},
		{
			input: &Ingress{
				Canary: &CanaryConfig{
					Enabled: true,
				},
			},
			byHeader: false,
			byWeight: true,
		},
		{
			input: &Ingress{
				Canary: &CanaryConfig{
					Enabled: true,
					Header:  "test",
				},
			},
			byHeader: true,
			byWeight: false,
		},
		{
			input: &Ingress{
				Canary: &CanaryConfig{
					Enabled: true,
					Cookie:  "test",
				},
			},
			byHeader: true,
			byWeight: false,
		},
		{
			input: &Ingress{
				Canary: &CanaryConfig{
					Enabled: true,
					Weight:  2,
				},
			},
			byHeader: false,
			byWeight: true,
		},
	}

	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			byHeader, byWeight := testCase.input.CanaryKind()
			if byHeader != testCase.byHeader {
				t.Fatalf("Should be %t, but actual is %t", testCase.byHeader, byHeader)
			}

			if byWeight != testCase.byWeight {
				t.Fatalf("Should be %t, but actual is %t", testCase.byWeight, byWeight)
			}
		})
	}
}

func TestNeedTrafficPolicy(t *testing.T) {
	config1 := &Ingress{}
	if config1.NeedTrafficPolicy() {
		t.Fatal("should be false")
	}

	config2 := &Ingress{
		UpstreamTLS: &UpstreamTLSConfig{
			BackendProtocol: defaultBackendProtocol,
		},
	}
	if !config2.NeedTrafficPolicy() {
		t.Fatal("should be true")
	}

	config3 := &Ingress{
		Destination: &DestinationConfig{
			Protocols: map[string]string{
				"secure.example.com:443": "HTTPS",
			},
		},
	}
	if !config3.NeedTrafficPolicy() {
		t.Fatal("should be true")
	}
}
