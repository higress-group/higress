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

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRedirectParse(t *testing.T) {
	parser := redirect{}

	testCases := []struct {
		name   string
		input  Annotations
		expect *RedirectConfig
	}{
		{
			name:   "Don't contain any redirect keys",
			input:  Annotations{},
			expect: nil,
		},
		{
			name: "By appRoot",
			input: Annotations{
				buildHigressAnnotationKey(appRoot):          "/root",
				buildHigressAnnotationKey(sslRedirect):      "true",
				buildHigressAnnotationKey(forceSSLRedirect): "true",
			},
			expect: &RedirectConfig{
				AppRoot:       "/root",
				httpsRedirect: true,
				Code:          defaultPermanentRedirectCode,
			},
		},
		{
			name: "By temporalRedirect",
			input: Annotations{
				buildHigressAnnotationKey(temporalRedirect): "http://www.xxx.org",
			},
			expect: &RedirectConfig{
				URL:  "http://www.xxx.org",
				Code: defaultTemporalRedirectCode,
			},
		},
		{
			name: "By temporalRedirect with invalid url",
			input: Annotations{
				buildHigressAnnotationKey(temporalRedirect): "tcp://www.xxx.org",
			},
			expect: &RedirectConfig{
				Code: defaultPermanentRedirectCode,
			},
		},
		{
			name: "By permanentRedirect",
			input: Annotations{
				buildHigressAnnotationKey(permanentRedirect): "http://www.xxx.org",
			},
			expect: &RedirectConfig{
				URL:  "http://www.xxx.org",
				Code: defaultPermanentRedirectCode,
			},
		},
		{
			name: "sslRedirect is ignored on MCP server Ingress",
			input: Annotations{
				buildHigressAnnotationKey(sslRedirect):     "true",
				buildHigressAnnotationKey(enableMcpServer): "true",
			},
			expect: nil,
		},
		{
			name: "permanentRedirect is ignored on MCP server Ingress",
			input: Annotations{
				buildHigressAnnotationKey(permanentRedirect): "http://www.xxx.org",
				buildHigressAnnotationKey(enableMcpServer):   "true",
			},
			expect: nil,
		},
	}

	for _, tt := range testCases {
		t.Run("", func(t *testing.T) {
			config := &Ingress{}
			_ = parser.Parse(tt.input, config, nil)
			if diff := cmp.Diff(tt.expect, config.Redirect, cmp.AllowUnexported(RedirectConfig{})); diff != "" {
				t.Fatalf("TestRedirectParse() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
