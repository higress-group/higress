// Copyright (c) 2025 Alibaba Group Holding Ltd.
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

package pathutil

import (
	"errors"
	"path"
	"testing"
)

func TestSplitTarget(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		wantPath   string
		wantSuffix string
	}{
		{name: "empty", target: "", wantPath: "", wantSuffix: ""},
		{name: "clean path", target: "/users", wantPath: "/users", wantSuffix: ""},
		{name: "path and query", target: "/users?id=1", wantPath: "/users", wantSuffix: "?id=1"},
		{name: "multiple query params", target: "/users?a=1&b=2", wantPath: "/users", wantSuffix: "?a=1&b=2"},
		{name: "empty query keeps separator", target: "/users?", wantPath: "/users", wantSuffix: "?"},
		{name: "dot segments stay in query", target: "/p?next=/../../../admin", wantPath: "/p", wantSuffix: "?next=/../../../admin"},
		{name: "second question mark belongs to query", target: "/p?a=1?b=2", wantPath: "/p", wantSuffix: "?a=1?b=2"},
		{name: "leading question mark", target: "?a=1", wantPath: "", wantSuffix: "?a=1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotSuffix := SplitTarget(tt.target)
			if gotPath != tt.wantPath || gotSuffix != tt.wantSuffix {
				t.Fatalf("SplitTarget(%q) = (%q, %q), want (%q, %q)", tt.target, gotPath, gotSuffix, tt.wantPath, tt.wantSuffix)
			}
			if gotPath+gotSuffix != tt.target {
				t.Fatalf("SplitTarget(%q) does not round-trip: %q + %q", tt.target, gotPath, gotSuffix)
			}
		})
	}
}

func TestSafeJoin(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		target  string
		want    string
		wantErr error
	}{
		// Clean targets must stay byte-identical to path.Join(prefix, target).
		{name: "clean path", prefix: "/auth", target: "/users", want: "/auth/users"},
		{name: "clean path with trailing slash prefix", prefix: "/auth/", target: "/users", want: "/auth/users"},
		{name: "empty target", prefix: "/auth", target: "", want: "/auth"},
		{name: "root target", prefix: "/auth", target: "/", want: "/auth"},
		{name: "root prefix", prefix: "/", target: "/users", want: "/users"},
		{name: "nested prefix", prefix: "/api/v1", target: "/chat", want: "/api/v1/chat"},

		// Query strings are re-appended verbatim and never join the path.
		{name: "path and query", prefix: "/auth", target: "/users?id=1", want: "/auth/users?id=1"},
		{name: "multiple query params", prefix: "/auth", target: "/users?a=1&b=2", want: "/auth/users?a=1&b=2"},
		{name: "empty query", prefix: "/auth", target: "/users?", want: "/auth/users?"},
		{
			name:   "dot segments carried by query value",
			prefix: "/auth",
			target: "/path?next=/../../../admin/keys",
			want:   "/auth/path?next=/../../../admin/keys",
		},
		{
			name:   "query value that would escape if joined",
			prefix: "/auth",
			target: "/check?redirect=../../../../etc/passwd",
			want:   "/auth/check?redirect=../../../../etc/passwd",
		},
		{
			// %2E%2E inside the query is not a path segment, so it survives verbatim.
			name:   "percent encoded dots in query",
			prefix: "/auth",
			target: "/check?next=%2E%2E%2F%2E%2E%2Fadmin",
			want:   "/auth/check?next=%2E%2E%2F%2E%2E%2Fadmin",
		},
		{
			// %2E%2E is a literal segment for path.Join, but consumers of the joined
			// path decode it before resolving dot segments, so it must be treated as one.
			name:    "percent encoded dot dot in path",
			prefix:  "/auth",
			target:  "/%2E%2E/admin",
			wantErr: ErrPathTraversal,
		},
		{name: "lowercase percent encoded dot dot in path", prefix: "/auth", target: "/%2e%2e/admin", wantErr: ErrPathTraversal},
		{name: "encoded slashes hiding dot dot", prefix: "/auth", target: "/a%2F..%2F..%2Fadmin", wantErr: ErrPathTraversal},
		{
			name:   "percent encoded dots that stay inside the prefix",
			prefix: "/auth",
			target: "/users/%2E%2Eprofile",
			want:   "/auth/users/%2E%2Eprofile",
		},

		// url.PathUnescape fails atomically, so an undecodable escape anywhere in the
		// path portion must not fall back to checking the raw target only: the
		// %2E%2E riding along with it would otherwise pass.
		{name: "invalid escape before encoded dot dot", prefix: "/gateway", target: "/%zz%2E%2E/admin", wantErr: ErrInvalidEscape},
		{name: "invalid escape alone", prefix: "/auth", target: "/users/%zz", wantErr: ErrInvalidEscape},
		{name: "truncated escape", prefix: "/auth", target: "/users/%2", wantErr: ErrInvalidEscape},
		{name: "bare percent", prefix: "/auth", target: "/users/100%", wantErr: ErrInvalidEscape},
		{name: "invalid escape inside the query is not decoded", prefix: "/auth", target: "/users?q=%zz", want: "/auth/users?q=%zz"},

		// A relative prefix has no root to escape from, so containment cannot be
		// decided; every one of these must fail closed instead of allowing all.
		{name: "empty prefix", prefix: "", target: "/users", wantErr: ErrRelativePrefix},
		{name: "dot prefix", prefix: ".", target: "/users", wantErr: ErrRelativePrefix},
		{name: "dot slash prefix", prefix: "./", target: "/users", wantErr: ErrRelativePrefix},
		{name: "relative prefix", prefix: "auth", target: "/users", wantErr: ErrRelativePrefix},
		{name: "dot prefix with query only target", prefix: ".", target: "?a=1", wantErr: ErrRelativePrefix},
		{name: "empty prefix cannot smuggle an escape", prefix: "", target: "/../../etc", wantErr: ErrRelativePrefix},

		// Documented limit of a single decode level: %252E%252E decodes once to
		// %2E%2E, a literal segment, which Envoy's path normalization is expected to
		// resolve before a target reaches a plugin.
		{name: "double encoded dot dot is not decoded twice", prefix: "/auth", target: "/%252E%252E/admin", want: "/auth/%252E%252E/admin"},

		// A backslash is not a separator for path.Clean, but Envoy only rewrites
		// %5C and leaves a raw backslash byte alone, so a consumer that reads '\'
		// as a separator would resolve ".." where the check saw one segment.
		{name: "raw backslash joined dot dot", prefix: "/auth", target: `/x\..\..\admin`, wantErr: ErrPathTraversal},
		{name: "raw backslash without dot segments", prefix: "/auth", target: `/users\a`, wantErr: ErrPathTraversal},
		{name: "encoded backslash joined dot dot", prefix: "/auth", target: "/..%5C..%5Cadmin", wantErr: ErrPathTraversal},
		{name: "raw backslash in the query stays verbatim", prefix: "/auth", target: `/users?file=a\b`, want: `/auth/users?file=a\b`},
		{
			name:   "dot segments and backslash in the query stay verbatim",
			prefix: "/auth",
			target: `/users?next=/x\..\..\admin`,
			want:   `/auth/users?next=/x\..\..\admin`,
		},

		// Each of these prefixes collapses to "/", which every absolute path is
		// inside of, so containment would pass for any target while the operator
		// meant a narrower location.
		{name: "dot dot prefix", prefix: "/..", target: "/../../etc/passwd", wantErr: ErrNonCanonicalPrefix},
		{name: "prefix climbing to root", prefix: "/auth/..", target: "/../../etc/passwd", wantErr: ErrNonCanonicalPrefix},
		{name: "prefix climbing past root", prefix: "/a/../..", target: "/../../etc/passwd", wantErr: ErrNonCanonicalPrefix},
		{name: "prefix with an inner dot segment", prefix: "/auth/v1/../v2", target: "/models", wantErr: ErrNonCanonicalPrefix},
		{name: "non canonical prefix rejects a clean target too", prefix: "/auth/..", target: "/users", wantErr: ErrNonCanonicalPrefix},

		// An undecodable escape is only worth rejecting when there is a decoded
		// check for it to disable; a root prefix contains every absolute path.
		{name: "root prefix forwards an undecodable escape", prefix: "/", target: "/v1/%zz", want: "/v1/%zz"},
		{name: "root prefix forwards an escape hiding dot dot", prefix: "/", target: "/%zz%2E%2E/admin", want: "/%zz%2E%2E/admin"},
		{name: "non root prefix rejects the same escape", prefix: "/auth", target: "/v1/%zz", wantErr: ErrInvalidEscape},

		// Dot segments that resolve inside the prefix are preserved as before.
		{name: "inner dot segment stays inside", prefix: "/auth", target: "/a/../b", want: "/auth/b"},
		{name: "single dot segment", prefix: "/auth", target: "/a/./b", want: "/auth/a/b"},

		// Escapes must be rejected rather than silently rewritten.
		{name: "dot dot escapes prefix", prefix: "/auth", target: "/a/../../admin/keys", wantErr: ErrPathTraversal},
		{name: "dot dot escapes deep prefix", prefix: "/api/v1", target: "/../../v2/models", wantErr: ErrPathTraversal},
		{name: "leading dot dot", prefix: "/auth", target: "/../admin", wantErr: ErrPathTraversal},
		{name: "prefix sibling escape", prefix: "/auth", target: "/x/../../authz", wantErr: ErrPathTraversal},
		{name: "escape with query", prefix: "/auth", target: "/a/../../admin?x=1", wantErr: ErrPathTraversal},
		{name: "escape from root prefix is impossible", prefix: "/", target: "/../../etc", want: "/etc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeJoin(tt.prefix, tt.target)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("SafeJoin(%q, %q) error = %v, want %v", tt.prefix, tt.target, err, tt.wantErr)
				}
				if got != "" {
					t.Fatalf("SafeJoin(%q, %q) = %q on error, want empty", tt.prefix, tt.target, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("SafeJoin(%q, %q) unexpected error: %v", tt.prefix, tt.target, err)
			}
			if got != tt.want {
				t.Fatalf("SafeJoin(%q, %q) = %q, want %q", tt.prefix, tt.target, got, tt.want)
			}
		})
	}
}

// TestSafeJoinMatchesPathJoinForCleanTargets pins the compatibility promise:
// targets without a query and without dot segments join exactly as before.
func TestSafeJoinMatchesPathJoinForCleanTargets(t *testing.T) {
	targets := []string{"/users", "/users/123", "/a/b/c", "", "/", "/chat/completions"}
	for _, prefix := range []string{"/", "/auth", "/api/v1", "/auth/"} {
		for _, target := range targets {
			want := path.Join(prefix, target)
			got, err := SafeJoin(prefix, target)
			if err != nil {
				t.Fatalf("SafeJoin(%q, %q) unexpected error: %v", prefix, target, err)
			}
			if got != want {
				t.Fatalf("SafeJoin(%q, %q) = %q, want path.Join result %q", prefix, target, got, want)
			}
		}
	}
}

// TestValidateWithin covers targets that already carry the prefix, the shape
// ValidateWithin exists for: callers that skip the join must not skip the check.
func TestValidateWithin(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		target  string
		wantErr error
	}{
		{name: "path below prefix", prefix: "/api", target: "/api/v1/models"},
		{name: "prefix itself", prefix: "/api", target: "/api"},
		{name: "prefix with trailing slash", prefix: "/api/", target: "/api/v1/models"},
		{name: "query is not resolved", prefix: "/api", target: "/api/v1/models?next=/../../../admin"},
		{name: "root prefix accepts any absolute path", prefix: "/", target: "/../../etc"},

		// The dot segments climb out of the prefix even though it is a literal prefix.
		{name: "already prefixed escape", prefix: "/api", target: "/api/../../admin", wantErr: ErrPathTraversal},
		{name: "already prefixed escape below a segment", prefix: "/api", target: "/api/x/../../admin", wantErr: ErrPathTraversal},
		{name: "already prefixed encoded escape", prefix: "/api", target: "/api/%2E%2E/%2E%2E/admin", wantErr: ErrPathTraversal},
		{name: "already prefixed encoded slash escape", prefix: "/api", target: "/api/a%2F..%2F..%2Fadmin", wantErr: ErrPathTraversal},
		{name: "sibling of prefix", prefix: "/api", target: "/apiv2/models", wantErr: ErrPathTraversal},
		{name: "invalid escape", prefix: "/api", target: "/api/%zz%2E%2E/admin", wantErr: ErrInvalidEscape},

		// Containment is undecidable without a root.
		{name: "empty prefix", prefix: "", target: "/api/v1/models", wantErr: ErrRelativePrefix},
		{name: "dot prefix", prefix: ".", target: "/api/v1/models", wantErr: ErrRelativePrefix},
		{name: "relative prefix", prefix: "api", target: "/api/v1/models", wantErr: ErrRelativePrefix},

		// Documented single-decode limit; a backslash is rejected instead, because
		// Envoy leaves a raw one untouched.
		{name: "double encoded dot dot", prefix: "/api", target: "/api/%252E%252E/admin"},
		{name: "encoded backslash joined dot dot", prefix: "/api", target: "/api/..%5C..%5Cadmin", wantErr: ErrPathTraversal},
		{name: "raw backslash joined dot dot", prefix: "/api", target: `/api/x\..\..\admin`, wantErr: ErrPathTraversal},
		{name: "raw backslash without dot segments", prefix: "/api", target: `/api/users\a`, wantErr: ErrPathTraversal},
		{name: "backslash in the query is not resolved", prefix: "/api", target: `/api/users?next=/x\..\..\admin`},

		// A prefix that resolves elsewhere than written cannot bound anything.
		{name: "dot dot prefix", prefix: "/..", target: "/../../etc/passwd", wantErr: ErrNonCanonicalPrefix},
		{name: "prefix climbing to root", prefix: "/api/..", target: "/../../etc/passwd", wantErr: ErrNonCanonicalPrefix},
		{name: "prefix climbing past root", prefix: "/a/../..", target: "/../../etc/passwd", wantErr: ErrNonCanonicalPrefix},

		// The decoded check exists to protect containment; a root prefix has none.
		{name: "root prefix accepts an undecodable escape", prefix: "/", target: "/v1/%zz"},
		{name: "non root prefix rejects an undecodable escape", prefix: "/api", target: "/api/v1/%zz", wantErr: ErrInvalidEscape},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWithin(tt.prefix, tt.target)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ValidateWithin(%q, %q) error = %v, want %v", tt.prefix, tt.target, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateWithin(%q, %q) unexpected error: %v", tt.prefix, tt.target, err)
			}
		})
	}
}

func TestWithinPrefix(t *testing.T) {
	tests := []struct {
		path   string
		prefix string
		want   bool
	}{
		{path: "/auth", prefix: "/auth", want: true},
		{path: "/auth/users", prefix: "/auth", want: true},
		{path: "/authx", prefix: "/auth", want: false},
		{path: "/admin", prefix: "/auth", want: false},
		{path: "/", prefix: "/auth", want: false},
		{path: "/anything", prefix: "/", want: true},
		{path: "anything", prefix: "/", want: false},
		{path: "/a/b", prefix: "/a/../a", want: true},
		// Dot segments in the candidate are resolved before the containment test.
		{path: "/auth/../admin", prefix: "/auth", want: false},
		{path: "/auth/users/../keys", prefix: "/auth", want: true},
		// A prefix without a root contains nothing.
		{path: "/anything", prefix: "", want: false},
		{path: "/anything", prefix: ".", want: false},
		{path: "anything", prefix: "any", want: false},
	}

	for _, tt := range tests {
		if got := WithinPrefix(tt.path, tt.prefix); got != tt.want {
			t.Errorf("WithinPrefix(%q, %q) = %v, want %v", tt.path, tt.prefix, got, tt.want)
		}
	}
}
