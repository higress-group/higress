// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package protocol

import (
	"mime"
	"net"
	"net/url"
	"strings"
)

const (
	HeaderProtocolVersion = "MCP-Protocol-Version"
	HeaderMethod          = "Mcp-Method"
	HeaderName            = "Mcp-Name"

	// ModernMaxBodyBytes bounds a modern single-message exchange without
	// changing the existing legacy buffer limit.
	ModernMaxBodyBytes uint32 = 1024 * 1024
	LegacyMaxBodyBytes uint32 = 100 * 1024 * 1024
)

// Transport contains only protocol-relevant request metadata. It intentionally
// excludes authorization, cookies, request IDs, and all runtime session state.
type Transport struct {
	Method                string
	Authority             string
	ContentType           string
	Accept                string
	Origin                string
	ProtocolVersion       string
	MCPMethod             string
	MCPName               string
	AmbiguousHeader       bool
	HasProtocolVersion    bool
	HasMCPMethod          bool
	HasMCPName            bool
	AmbiguousModernHeader bool
}

// NewTransport captures relevant headers without retaining credentials.
// Accept field lines are combined in receive order as one list; repeated
// singleton or identity headers are rejected later rather than guessed.
func NewTransport(method, authority string, headers [][2]string) Transport {
	transport := Transport{Method: method, Authority: authority}
	seen := make(map[string]bool)
	acceptValues := make([]string, 0, 1)
	for _, header := range headers {
		name := strings.ToLower(header[0])
		var target *string
		switch name {
		case "content-type":
			target = &transport.ContentType
		case "accept":
			acceptValues = append(acceptValues, strings.TrimSpace(header[1]))
			continue
		case "origin":
			target = &transport.Origin
		case strings.ToLower(HeaderProtocolVersion):
			target = &transport.ProtocolVersion
			transport.HasProtocolVersion = true
		case strings.ToLower(HeaderMethod):
			target = &transport.MCPMethod
			transport.HasMCPMethod = true
		case strings.ToLower(HeaderName):
			target = &transport.MCPName
			transport.HasMCPName = true
		default:
			continue
		}
		if seen[name] {
			transport.AmbiguousHeader = true
			if name == strings.ToLower(HeaderProtocolVersion) || name == strings.ToLower(HeaderMethod) || name == strings.ToLower(HeaderName) {
				transport.AmbiguousModernHeader = true
			}
			continue
		}
		seen[name] = true
		if name == strings.ToLower(HeaderName) {
			// Mcp-Name whitespace is identity data: only the Base64 sentinel may
			// carry leading or trailing whitespace in the decoded name.
			*target = header[1]
		} else {
			*target = strings.TrimSpace(header[1])
		}
	}
	transport.Accept = strings.Join(acceptValues, ", ")
	return transport
}

// HasModernIdentityHeaders reports whether a request carries any header that
// belongs exclusively to the modern profile. Incomplete modern identity must
// never fall back to legacy dispatch.
func HasModernIdentityHeaders(transport Transport) bool {
	return transport.HasMCPMethod || transport.HasMCPName ||
		transport.MCPMethod != "" || transport.MCPName != "" ||
		transport.AmbiguousModernHeader ||
		(transport.HasProtocolVersion && transport.ProtocolVersion == "") ||
		(transport.ProtocolVersion != "" && !IsLegacyVersion(Version(transport.ProtocolVersion)))
}

// ValidateModernTransport performs checks which do not depend on the JSON-RPC
// envelope and therefore can run before request-body processing.
func ValidateModernTransport(transport Transport) *Error {
	if protocolError := ValidateOrigin(transport); protocolError != nil {
		return protocolError
	}
	if transport.AmbiguousHeader {
		return HeaderMismatch()
	}
	if transport.Method != "POST" {
		return MethodNotAllowed()
	}
	mediaType, _, err := mime.ParseMediaType(transport.ContentType)
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return UnsupportedMediaType()
	}
	if !accepts(transport.Accept, "application/json") || !accepts(transport.Accept, "text/event-stream") {
		return NotAcceptable()
	}
	return nil
}

// ValidateOrigin is intentionally independent of protocol-era classification.
// A hostile Origin is rejected before version disclosure or body parsing.
func ValidateOrigin(transport Transport) *Error {
	if !trustedOrigin(transport.Origin, transport.Authority) {
		return UntrustedOrigin()
	}
	return nil
}

func accepts(value, required string) bool {
	for item := range strings.SplitSeq(value, ",") {
		item = strings.TrimSpace(item)
		mediaType, parameters, err := mime.ParseMediaType(item)
		if err != nil {
			continue
		}
		if quality, ok := parameters["q"]; ok {
			if hasQuotedQualityParameter(item) || !positiveQValue(quality) {
				continue
			}
		}
		if strings.EqualFold(mediaType, required) {
			return true
		}
	}
	return false
}

// positiveQValue implements the RFC 9110 qvalue grammar and additionally
// requires a non-zero value for a media range to be acceptable.
func positiveQValue(value string) bool {
	if value == "1" || value == "1." {
		return true
	}
	if strings.HasPrefix(value, "1.") {
		fraction := value[2:]
		if len(fraction) == 0 || len(fraction) > 3 {
			return false
		}
		for i := range fraction {
			if fraction[i] != '0' {
				return false
			}
		}
		return true
	}
	if value == "0" || value == "0." {
		return false
	}
	if !strings.HasPrefix(value, "0.") {
		return false
	}
	fraction := value[2:]
	if len(fraction) == 0 || len(fraction) > 3 {
		return false
	}
	positive := false
	for i := range fraction {
		if fraction[i] < '0' || fraction[i] > '9' {
			return false
		}
		positive = positive || fraction[i] != '0'
	}
	return positive
}

func hasQuotedQualityParameter(item string) bool {
	for i := 0; i < len(item); {
		if item[i] == '"' {
			i = skipQuotedHeaderValue(item, i)
			continue
		}
		if item[i] != ';' {
			i++
			continue
		}
		i++
		for i < len(item) && (item[i] == ' ' || item[i] == '\t') {
			i++
		}
		nameStart := i
		for i < len(item) && item[i] != '=' && item[i] != ';' && item[i] != ' ' && item[i] != '\t' {
			i++
		}
		name := item[nameStart:i]
		for i < len(item) && (item[i] == ' ' || item[i] == '\t') {
			i++
		}
		if i >= len(item) || item[i] != '=' {
			continue
		}
		i++
		for i < len(item) && (item[i] == ' ' || item[i] == '\t') {
			i++
		}
		if strings.EqualFold(name, "q") {
			return i < len(item) && item[i] == '"'
		}
	}
	return false
}

func skipQuotedHeaderValue(value string, quote int) int {
	for i := quote + 1; i < len(value); i++ {
		switch value[i] {
		case '\\':
			i++
		case '"':
			return i + 1
		}
	}
	return len(value)
}

func trustedOrigin(origin, authority string) bool {
	if origin == "" {
		return true
	}
	if strings.ContainsAny(origin, ", ") || authority == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	originHost, originPort, ok := normalizedHostPort(parsed.Host, parsed.Scheme)
	if !ok {
		return false
	}
	authorityHost, authorityPort, ok := normalizedHostPort(authority, parsed.Scheme)
	return ok && strings.EqualFold(originHost, authorityHost) && originPort == authorityPort
}

func normalizedHostPort(value, scheme string) (string, string, bool) {
	host := value
	port := ""
	if parsedHost, parsedPort, err := net.SplitHostPort(value); err == nil {
		host, port = parsedHost, parsedPort
	} else if strings.Count(value, ":") > 1 && !strings.HasPrefix(value, "[") {
		return "", "", false
	}
	if host == "" {
		return "", "", false
	}
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return strings.Trim(host, "[]"), port, true
}
