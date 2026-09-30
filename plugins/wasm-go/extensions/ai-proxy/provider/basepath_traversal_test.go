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

package provider

import (
	"path"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/pathutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The :path header carries the query string. Joining it whole onto basePath let
// dot segments inside a query value resolve as path segments.
func TestPrependBasePathQueryCarriedTraversalWasUnsafe(t *testing.T) {
	const basePath = "/api"
	const requestPath = "/v1/models?next=/../../../admin/keys"

	assert.Equal(t, "/admin/keys", path.Join(basePath, requestPath),
		"path.Join on the raw :path escapes basePath")

	got, err := (&ProviderConfig{basePath: basePath, basePathHandling: basePathHandlingPrepend}).prependBasePath(requestPath)
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/models?next=/../../../admin/keys", got)
}

func TestPrependBasePath(t *testing.T) {
	tests := []struct {
		name        string
		basePath    string
		requestPath string
		want        string
		wantErr     error
	}{
		// Clean paths keep their previous result.
		{name: "clean path", basePath: "/api", requestPath: "/v1/chat/completions", want: "/api/v1/chat/completions"},
		{name: "trailing slash basePath", basePath: "/api/", requestPath: "/v1/models", want: "/api/v1/models"},
		{name: "already prefixed", basePath: "/api", requestPath: "/api/v1/models", want: "/api/v1/models"},
		{name: "already prefixed with query", basePath: "/api", requestPath: "/api/v1/models?a=1", want: "/api/v1/models?a=1"},
		{name: "empty request path", basePath: "/api", requestPath: "", want: "/api"},

		// The query is re-appended verbatim and never joins the path.
		{name: "path with query", basePath: "/api", requestPath: "/v1/models?a=1", want: "/api/v1/models?a=1"},
		{name: "multiple query params", basePath: "/api", requestPath: "/v1/models?a=1&b=2", want: "/api/v1/models?a=1&b=2"},
		{name: "empty query keeps separator", basePath: "/api", requestPath: "/v1/models?", want: "/api/v1/models?"},
		{
			name:        "dot segments carried by query value",
			basePath:    "/api",
			requestPath: "/v1/models?next=/../../../admin/keys",
			want:        "/api/v1/models?next=/../../../admin/keys",
		},
		{
			name:        "percent encoded dots in query stay as-is",
			basePath:    "/api",
			requestPath: "/v1/models?next=%2E%2E%2F%2E%2E%2Fadmin",
			want:        "/api/v1/models?next=%2E%2E%2F%2E%2E%2Fadmin",
		},
		{
			// %2E%2E is a literal segment for path.Join, but the upstream decodes the
			// path before resolving dot segments, so it must be treated as one.
			name:        "percent encoded dot dot in path",
			basePath:    "/api",
			requestPath: "/%2E%2E/admin",
			wantErr:     pathutil.ErrPathTraversal,
		},
		{
			name:        "percent encoded dots that stay inside basePath",
			basePath:    "/api",
			requestPath: "/v1/models%2E%2Elist",
			want:        "/api/v1/models%2E%2Elist",
		},

		// A path that already starts with basePath is not exempt: it can climb back
		// out of basePath, and the early return used to skip every check.
		{
			name:        "already prefixed dot dot escapes basePath",
			basePath:    "/api",
			requestPath: "/api/../../admin",
			wantErr:     pathutil.ErrPathTraversal,
		},
		{
			name:        "already prefixed dot dot below a segment escapes basePath",
			basePath:    "/api",
			requestPath: "/api/x/../../admin",
			wantErr:     pathutil.ErrPathTraversal,
		},
		{
			name:        "already prefixed percent encoded dot dot escapes basePath",
			basePath:    "/api",
			requestPath: "/api/%2E%2E/%2E%2E/admin",
			wantErr:     pathutil.ErrPathTraversal,
		},
		{
			name:        "already prefixed escape carrying a query",
			basePath:    "/api",
			requestPath: "/api/../../admin?a=1",
			wantErr:     pathutil.ErrPathTraversal,
		},

		// url.PathUnescape fails atomically, so an undecodable escape must not let
		// the %2E%2E next to it ride through unchecked.
		{
			name:        "invalid escape before percent encoded dot dot",
			basePath:    "/api",
			requestPath: "/%zz%2E%2E/admin",
			wantErr:     pathutil.ErrInvalidEscape,
		},
		{
			name:        "already prefixed invalid escape",
			basePath:    "/api",
			requestPath: "/api/%zz%2E%2E/admin",
			wantErr:     pathutil.ErrInvalidEscape,
		},

		// A basePath without a root cannot decide containment.
		{name: "relative basePath", basePath: "api", requestPath: "/v1/models", wantErr: pathutil.ErrRelativePrefix},

		// path.Clean collapses this basePath to "/", which every absolute path is
		// inside of, so containment would pass for any request path.
		{name: "basePath climbing to root", basePath: "/api/..", requestPath: "/v1/models", wantErr: pathutil.ErrNonCanonicalPrefix},
		{name: "dot dot basePath", basePath: "/..", requestPath: "/../../etc/passwd", wantErr: pathutil.ErrNonCanonicalPrefix},

		// Envoy rewrites %5C but leaves a raw backslash byte alone, and a consumer
		// reading it as a separator would resolve ".." where the check saw one
		// segment. The query string is never resolved.
		{name: "raw backslash joined dot dot", basePath: "/api", requestPath: `/v1\x\..\..\admin`, wantErr: pathutil.ErrPathTraversal},
		{name: "encoded backslash joined dot dot", basePath: "/api", requestPath: "/v1/..%5C..%5Cadmin", wantErr: pathutil.ErrPathTraversal},
		{name: "backslash in the query stays verbatim", basePath: "/api", requestPath: `/v1/models?file=a\b`, want: `/api/v1/models?file=a\b`},

		// A root basePath contains every absolute path, so an undecodable escape has
		// no containment check to disable and the path is forwarded unchanged.
		{name: "root basePath forwards an undecodable escape", basePath: "/", requestPath: "/v1/%zz", want: "/v1/%zz"},
		{name: "non root basePath rejects the same escape", basePath: "/api", requestPath: "/v1/%zz", wantErr: pathutil.ErrInvalidEscape},

		// Dot segments resolving inside basePath behave as before.
		{name: "inner dot segment stays inside", basePath: "/api", requestPath: "/v1/../v2/models", want: "/api/v2/models"},

		// Escapes are rejected instead of being silently rewritten.
		{name: "dot dot escapes basePath", basePath: "/api", requestPath: "/v1/../../admin/keys", wantErr: pathutil.ErrPathTraversal},
		{name: "leading dot dot", basePath: "/api", requestPath: "/../admin/keys", wantErr: pathutil.ErrPathTraversal},
		{name: "escape with query", basePath: "/api", requestPath: "/../admin/keys?a=1", wantErr: pathutil.ErrPathTraversal},
		{name: "basePath sibling escape", basePath: "/api", requestPath: "/x/../../apiv2/models", wantErr: pathutil.ErrPathTraversal},
		{name: "deep basePath escape", basePath: "/gateway/ai/v1", requestPath: "/../../v2/models", wantErr: pathutil.ErrPathTraversal},
	}

	config := &ProviderConfig{basePathHandling: basePathHandlingPrepend}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.basePath = tt.basePath
			got, err := config.prependBasePath(tt.requestPath)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Clean targets without a query must join byte-identically to the previous
// path.Join(basePath, :path) behaviour.
func TestPrependBasePathMatchesPathJoinForCleanTargets(t *testing.T) {
	config := &ProviderConfig{basePath: "/api", basePathHandling: basePathHandlingPrepend}
	for _, requestPath := range []string{"/v1/models", "/v1/chat/completions", "/v1/a/b/c", "/"} {
		got, err := config.prependBasePath(requestPath)
		require.NoError(t, err)
		assert.Equal(t, path.Join(config.basePath, requestPath), got, "request path %q", requestPath)
	}
}
