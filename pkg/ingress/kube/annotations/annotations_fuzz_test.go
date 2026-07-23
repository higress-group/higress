// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

	networking "istio.io/api/networking/v1alpha3"
)

func FuzzRewriteAnnotation(f *testing.F) {
	f.Add("/$1", "/api", uint8(0))
	f.Add(`/${2}/$1`, `/users/(.*)`, uint8(2))
	f.Add("", "/", uint8(1))

	f.Fuzz(func(t *testing.T, target, path string, matchKind uint8) {
		if len(target) > 4096 || len(path) > 4096 {
			t.Skip()
		}

		config := &Ingress{}
		input := Annotations{
			buildNginxAnnotationKey(rewriteTarget): target,
			buildNginxAnnotationKey(useRegex):      "true",
		}
		if err := (rewrite{}).Parse(input, config, nil); err != nil {
			t.Fatalf("parse rewrite annotation: %v", err)
		}

		match := &networking.StringMatch{}
		switch matchKind % 3 {
		case 0:
			match.MatchType = &networking.StringMatch_Exact{Exact: path}
		case 1:
			match.MatchType = &networking.StringMatch_Prefix{Prefix: path}
		default:
			match.MatchType = &networking.StringMatch_Regex{Regex: path}
		}
		route := &networking.HTTPRoute{
			Match: []*networking.HTTPMatchRequest{{Uri: match}},
		}
		(rewrite{}).ApplyRoute(route, config)

		converted := convertToRE2(target)
		if convertToRE2(converted) != converted {
			t.Fatalf("rewrite conversion is not idempotent: %q", target)
		}
	})
}

func FuzzHeaderControlAnnotation(f *testing.F) {
	f.Add("x-request-id request-id")
	f.Add("\"x quoted\" 'value with spaces'")
	f.Add("x-first one\nx-second two")
	f.Add("")

	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 8192 {
			t.Skip()
		}

		config := &Ingress{}
		input := Annotations{
			buildHigressAnnotationKey(requestHeaderAdd):     value,
			buildHigressAnnotationKey(responseHeaderUpdate): value,
		}
		if err := (headerControl{}).Parse(input, config, nil); err != nil {
			t.Fatalf("parse header-control annotation: %v", err)
		}

		route := &networking.HTTPRoute{}
		(headerControl{}).ApplyRoute(route, config)
	})
}
