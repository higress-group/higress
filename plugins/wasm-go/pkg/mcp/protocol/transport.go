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

import "strings"

const (
	HeaderProtocolVersion = "MCP-Protocol-Version"
	HeaderMethod          = "Mcp-Method"
	HeaderName            = "Mcp-Name"

	// MaxBodyBytes covers the complete JSON-RPC body for both profiles.
	MaxBodyBytes       uint32 = 100 * 1024 * 1024
	ModernMaxBodyBytes        = MaxBodyBytes
	LegacyMaxBodyBytes        = MaxBodyBytes
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
// Accept field lines are combined in receive order as one list. Repeated
// protocol identity headers are rejected later rather than guessed.
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
	if transport.AmbiguousModernHeader {
		return HeaderMismatch()
	}
	if transport.Method == "GET" || transport.Method == "DELETE" {
		return MethodNotAllowed()
	}
	return nil
}

// ValidateOrigin retains source compatibility with existing protocol consumers.
// Deprecated: Origin follows the historical acceptance policy for both profiles.
func ValidateOrigin(Transport) *Error {
	return nil
}
