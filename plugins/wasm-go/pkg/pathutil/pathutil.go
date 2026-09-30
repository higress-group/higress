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

// Package pathutil combines an operator-configured path prefix with a request
// target received from a client without letting the target escape the prefix.
//
// Percent-decoding is single-level, so a double-encoded target such as
// "%252E%252E" is decoded once to "%2E%2E" and is deliberately not treated as
// ".."; Envoy's path normalization is expected to resolve it before a target
// reaches a plugin.
//
// A backslash is not a separator here, and one is not normalized away upstream
// either: path_with_escaped_slash_action only rewrites %2F and %5C, so a raw
// backslash byte arrives unchanged. Because a consumer that does read '\' as a
// separator would resolve somewhere other than the validated path, a literal
// backslash in the path portion is rejected, as is the %5C form once decoded.
// The query string is passed through untouched.
package pathutil

import (
	"errors"
	"net/url"
	"path"
	"strings"
)

var (
	// ErrPathTraversal reports that the request target resolves outside of the
	// configured prefix, or carries a literal backslash in its path portion.
	ErrPathTraversal = errors.New("request path escapes the configured prefix")

	// ErrInvalidEscape reports that a percent-escape in the path portion of the
	// request target cannot be decoded. Consumers of the path decode
	// percent-escapes before resolving dot segments, and url.PathUnescape fails
	// atomically, so an undecodable escape would otherwise disable the decoded
	// check for the whole target; it is rejected instead of being checked as
	// written only.
	//
	// A root prefix contains every absolute path, so there is nothing for a
	// decoded dot segment to escape into and no decoded check to disable: no
	// escape is reported for prefix "/". An escape inside the query string is
	// never decoded and never reported either.
	ErrInvalidEscape = errors.New("request path contains an undecodable percent-escape")

	// ErrRelativePrefix reports that the configured prefix is not absolute.
	// Containment is only decidable against a rooted path: a relative prefix such
	// as "." resolves to itself for every target and would silently disable the
	// check.
	ErrRelativePrefix = errors.New("path prefix must be absolute")

	// ErrNonCanonicalPrefix reports that the configured prefix is not the path it
	// resolves to. path.Clean collapses a prefix such as "/auth/.." to "/", which
	// every absolute path is inside of, so the containment check would pass for
	// any target while the operator meant a narrower location. A single trailing
	// slash names the same directory and is accepted.
	ErrNonCanonicalPrefix = errors.New("path prefix must be canonical")
)

// SplitTarget splits a raw request target at the first '?'. querySuffix keeps
// the separator so that pathPart+querySuffix reproduces target verbatim, and is
// empty when target carries no query.
func SplitTarget(target string) (pathPart string, querySuffix string) {
	if i := strings.IndexByte(target, '?'); i >= 0 {
		return target[:i], target[i:]
	}
	return target, ""
}

// ValidateWithin reports whether target, read as a complete request path rather
// than a suffix to be appended, resolves to prefix or to a location below it.
//
// prefix must be absolute and canonical. target may carry a query string; only
// its path portion is resolved, because a query value is never read as path
// segments. That path portion is checked both as written and percent-decoded:
// whoever consumes it decodes percent-escapes before resolving dot segments, so
// "%2E%2E" travels as ".." even though path.Clean sees a literal segment. The
// decoded check is skipped for a root prefix, which no absolute path can escape.
func ValidateWithin(prefix, target string) error {
	if err := validatePrefix(prefix); err != nil {
		return err
	}
	pathPart, _ := SplitTarget(target)
	// path.Clean reads a backslash as an ordinary byte, so "\.." stays one segment
	// here while a consumer that treats '\' as a separator resolves it as "..".
	// Envoy's escaped-slash handling covers %5C only, never a raw backslash byte.
	if strings.ContainsRune(pathPart, '\\') {
		return ErrPathTraversal
	}
	if !WithinPrefix(pathPart, prefix) {
		return ErrPathTraversal
	}
	if isRootPrefix(prefix) {
		return nil
	}
	decoded, err := url.PathUnescape(pathPart)
	if err != nil {
		return ErrInvalidEscape
	}
	if strings.ContainsRune(decoded, '\\') || !WithinPrefix(decoded, prefix) {
		return ErrPathTraversal
	}
	return nil
}

// SafeJoin joins prefix with the path portion of target and re-appends the
// query verbatim, so dot segments inside a query value can never be read as
// path segments. For a target without a query and without dot segments the
// result is byte-identical to path.Join(prefix, target).
//
// The joined path must resolve to prefix or below it, and prefix must be
// absolute and canonical: one that path.Clean rewrites, such as "/auth/..",
// would be checked against a location the operator never configured. path.Join
// cleans its result, so a target like "/a/../../b" would silently become a
// sibling of prefix; SafeJoin reports ErrPathTraversal instead of returning the
// rewritten path, letting callers fail closed rather than forward a request that
// the authorization server and the upstream would interpret differently.
func SafeJoin(prefix, target string) (string, error) {
	pathPart, querySuffix := SplitTarget(target)
	joined := path.Join(prefix, pathPart)
	if err := ValidateWithin(prefix, joined); err != nil {
		return "", err
	}
	return joined + querySuffix, nil
}

// WithinPrefix reports whether p resolves to prefix itself or to a location
// below it. Dot segments in both arguments are resolved first, and prefix is
// expected to be absolute: a relative one contains nothing.
func WithinPrefix(p, prefix string) bool {
	p = path.Clean(p)
	switch clean := path.Clean(prefix); clean {
	case "/":
		return strings.HasPrefix(p, "/")
	default:
		return p == clean || strings.HasPrefix(p, clean+"/")
	}
}

// validatePrefix checks that containment can be decided against prefix: it must
// name a rooted location, and it must be the location it names. A prefix that
// path.Clean rewrites, such as "/auth/..", would be compared against a different
// path than the operator configured. A single trailing slash is allowed because
// it names the same directory.
func validatePrefix(prefix string) error {
	if !strings.HasPrefix(prefix, "/") {
		return ErrRelativePrefix
	}
	trimmed := strings.TrimSuffix(prefix, "/")
	if trimmed == "" || path.Clean(trimmed) == trimmed {
		return nil
	}
	return ErrNonCanonicalPrefix
}

// isRootPrefix reports whether prefix resolves to "/", the one prefix that
// contains every absolute path.
func isRootPrefix(prefix string) bool {
	return path.Clean(prefix) == "/"
}
