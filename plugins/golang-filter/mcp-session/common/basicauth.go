package common

import (
	"crypto/subtle"
	"encoding/base64"
	"strings"
)

// BasicAuthProvider is implemented by server configs that require HTTP Basic
// authentication. When a server exposes non-empty credentials, the MCP server
// filter enforces Basic auth on every request routed to that server.
type BasicAuthProvider interface {
	// GetBasicAuthCredentials returns the username and password required to
	// access the server. An empty username means authentication is disabled.
	GetBasicAuthCredentials() (username string, password string)
}

// CheckBasicAuth reports whether the given Authorization header value carries
// HTTP Basic credentials matching the expected username and password. The
// comparison is constant-time to avoid leaking credential length or content
// through timing. It always returns false when the expected username is empty.
func CheckBasicAuth(authHeader, expectedUsername, expectedPassword string) bool {
	if expectedUsername == "" {
		return false
	}

	const prefix = "Basic "
	if len(authHeader) < len(prefix) || !strings.EqualFold(authHeader[:len(prefix)], prefix) {
		return false
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(authHeader[len(prefix):]))
	if err != nil {
		return false
	}

	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return false
	}

	userMatch := subtle.ConstantTimeCompare([]byte(username), []byte(expectedUsername)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(password), []byte(expectedPassword)) == 1
	return userMatch && passMatch
}
