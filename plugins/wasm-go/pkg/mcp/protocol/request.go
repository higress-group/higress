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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"unicode/utf8"
)

// Icon is the complete optional icon schema nested under clientInfo.
type Icon struct {
	Source   string   `json:"src"`
	MIMEType string   `json:"mimeType,omitempty"`
	Sizes    []string `json:"sizes,omitempty"`
	Theme    string   `json:"theme,omitempty"`
}

// Implementation identifies a modern MCP client when it is provided.
type Implementation struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	WebsiteURL  string `json:"websiteUrl,omitempty"`
	Icons       []Icon `json:"icons,omitempty"`
}

// Metadata holds protocol identity and optional, best-effort client views.
type Metadata struct {
	ProtocolVersion    Version
	ClientCapabilities ClientCapabilities
	ClientInfo         *Implementation
	Extensions         map[string]json.RawMessage
}

// Cancellation is scoped to one modern HTTP exchange. It has no protocol
// session identity and is safe to signal more than once.
type Cancellation struct {
	mu        sync.Mutex
	done      chan struct{}
	cancelled bool
	nextID    uint64
	cleanups  map[uint64]func()
}

func newCancellation() *Cancellation {
	return &Cancellation{
		done:     make(chan struct{}),
		cleanups: make(map[uint64]func()),
	}
}

func (c *Cancellation) Done() <-chan struct{} {
	return c.done
}

func (c *Cancellation) Cancel() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.cancelled {
		c.mu.Unlock()
		return
	}
	c.cancelled = true
	cleanups := make([]func(), 0, len(c.cleanups))
	for _, cleanup := range c.cleanups {
		cleanups = append(cleanups, cleanup)
	}
	clear(c.cleanups)
	close(c.done)
	c.mu.Unlock()
	for _, cleanup := range cleanups {
		cleanup()
	}
}

// OnCancel registers request-scoped cleanup and returns an idempotent
// unregister function. Registration after cancellation runs cleanup
// immediately so pending work cannot escape a concurrent disconnect.
func (c *Cancellation) OnCancel(cleanup func()) func() {
	if c == nil || cleanup == nil {
		return func() {}
	}
	c.mu.Lock()
	if c.cancelled {
		c.mu.Unlock()
		cleanup()
		return func() {}
	}
	c.nextID++
	id := c.nextID
	c.cleanups[id] = cleanup
	c.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			delete(c.cleanups, id)
			c.mu.Unlock()
		})
	}
}

// RequestContext is the stable pre-dispatch contract shared by modern
// handlers. It deliberately has no protocol session or resumability fields.
type RequestContext struct {
	Era          Era
	Version      Version
	Transport    Transport
	Envelope     Envelope
	Metadata     Metadata
	Cancellation *Cancellation
}

func (c *RequestContext) Cancel() {
	if c != nil {
		c.Cancellation.Cancel()
	}
}

func (c *RequestContext) OnCancel(cleanup func()) func() {
	if c == nil {
		return func() {}
	}
	return c.Cancellation.OnCancel(cleanup)
}

// SemanticResult is the profile-independent result boundary for successor
// handlers. Result shaping remains outside this PROCESS.
type SemanticResult struct {
	Value      any
	ResultType string
	Meta       map[string]any
}

// MethodPolicy is the successor-facing pre-dispatch contract. Required client
// capabilities are validated before the business handler is invoked.
type MethodPolicy struct {
	Available                  bool
	RequiredClientCapabilities ClientCapabilities
}

type MethodPolicyLookup func(method string) MethodPolicy

// PrepareRequest classifies a request exactly once. Both profiles share message
// framing and historical business field reads. Modern requests additionally
// validate protocol identity and the supported method/lifecycle boundary.
func PrepareRequest(transport Transport, body []byte, methodAvailable func(string) bool) (*RequestContext, *Error) {
	return PrepareRequestWithPolicy(transport, body, func(method string) MethodPolicy {
		return MethodPolicy{Available: methodAvailable != nil && methodAvailable(method)}
	})
}

func PrepareRequestWithPolicy(transport Transport, body []byte, policyLookup MethodPolicyLookup) (*RequestContext, *Error) {
	if protocolError := ValidateOrigin(transport); protocolError != nil {
		return nil, protocolError
	}
	if len(body) > int(MaxBodyBytes) {
		return nil, RequestTooLarge()
	}
	era, classificationError := classify(transport, body)
	if classificationError != nil {
		return nil, classificationError
	}
	envelope, envelopeError := decodeEnvelope(body, era == EraModern)
	request := &RequestContext{
		Era: era, Version: Version(transport.ProtocolVersion), Transport: transport, Envelope: envelope,
	}
	if envelopeError != nil {
		if envelope.Raw == nil {
			return nil, envelopeError
		}
		return request, envelopeError
	}
	if era == EraLegacy {
		return request, nil
	}
	request.Version = Version20260728
	if transport.ProtocolVersion != "" &&
		!IsModernVersion(Version(transport.ProtocolVersion)) &&
		!IsLegacyVersion(Version(transport.ProtocolVersion)) {
		return request, UnsupportedVersion(Version(transport.ProtocolVersion), SupportedVersions())
	}
	if protocolError := ValidateModernTransport(transport); protocolError != nil {
		return request, protocolError
	}
	if !IsModernVersion(Version(transport.ProtocolVersion)) || transport.MCPMethod == "" {
		return request, HeaderMismatch()
	}
	metadata, protocolError := decodeMetadata(envelope.Params)
	if protocolError != nil {
		return request, protocolError
	}
	request.Metadata = metadata
	if !IsModernVersion(metadata.ProtocolVersion) {
		return request, UnsupportedVersion(metadata.ProtocolVersion, SupportedVersions())
	}
	if transport.ProtocolVersion == "" || transport.ProtocolVersion != string(metadata.ProtocolVersion) {
		return request, HeaderMismatch()
	}
	if transport.MCPMethod == "" || transport.MCPMethod != envelope.Method {
		return request, HeaderMismatch()
	}
	if protocolError := validateNameHeader(transport, envelope); protocolError != nil {
		return request, protocolError
	}
	if !isCurrentModernMethod(envelope.Method) || policyLookup == nil {
		return request, MethodNotFound()
	}
	policy := policyLookup(envelope.Method)
	if !policy.Available {
		return request, MethodNotFound()
	}
	missingCapabilities := missingClientCapabilities(metadata.ClientCapabilities, policy.RequiredClientCapabilities)
	if !missingCapabilities.isEmpty() {
		return request, MissingRequiredClientCapability(missingCapabilities)
	}
	request.Cancellation = newCancellation()
	return request, nil
}

func classify(transport Transport, body []byte) (Era, *Error) {
	if HasModernIdentityHeaders(transport) {
		return EraModern, nil
	}
	switch classifyRequestBody(body) {
	case bodyClassificationModern:
		return EraModern, nil
	case bodyClassificationLegacy:
		return EraLegacy, nil
	default:
		return EraLegacy, ParseError()
	}
}

func decodeMetadata(params json.RawMessage) (Metadata, *Error) {
	if !isJSONObject(params) {
		return Metadata{}, HeaderMismatch()
	}
	metaRaw := businessObjectMembers(params)["_meta"]
	if !isJSONObject(metaRaw) {
		return Metadata{}, HeaderMismatch()
	}
	members := businessObjectMembers(metaRaw)
	var version string
	if err := json.Unmarshal(members[MetaProtocolVersion], &version); err != nil || version == "" {
		return Metadata{}, HeaderMismatch()
	}
	metadata := Metadata{ProtocolVersion: Version(version), Extensions: make(map[string]json.RawMessage)}
	// Non-identity metadata is descriptive input. Populate compatibility views
	// when possible; only control-flow decoders require their strict schemas.
	_ = json.Unmarshal(members[MetaClientCapabilities], &metadata.ClientCapabilities)
	var clientInfo Implementation
	if raw, ok := members[MetaClientInfo]; ok && isJSONObject(raw) && json.Unmarshal(raw, &clientInfo) == nil {
		metadata.ClientInfo = &clientInfo
	}
	for name, value := range members {
		if name != MetaProtocolVersion && name != MetaClientCapabilities && name != MetaClientInfo {
			metadata.Extensions[name] = bytes.Clone(value)
		}
	}
	return metadata, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

const (
	nameHeaderPrefix = "=?base64?"
	nameHeaderSuffix = "?="
)

func validateNameHeader(transport Transport, envelope Envelope) *Error {
	hasNameHeader := transport.HasMCPName || transport.MCPName != ""
	if envelope.Method != "tools/call" {
		if hasNameHeader {
			return HeaderMismatch()
		}
		return nil
	}
	params := businessObjectMembers(envelope.Params)
	nameRaw := params["name"]
	var name string
	if json.Unmarshal(nameRaw, &name) != nil || name == "" {
		return InvalidParams("tools/call params.name is required")
	}
	decodedName, ok := decodeNameHeader(transport.MCPName, hasNameHeader)
	if !ok || decodedName != name {
		return HeaderMismatch()
	}
	return nil
}

func decodeNameHeader(value string, present bool) (string, bool) {
	if !present || value == "" {
		return "", false
	}
	if strings.HasPrefix(value, nameHeaderPrefix) || strings.HasSuffix(value, nameHeaderSuffix) {
		if !strings.HasPrefix(value, nameHeaderPrefix) || !strings.HasSuffix(value, nameHeaderSuffix) {
			return "", false
		}
		payload := strings.TrimSuffix(strings.TrimPrefix(value, nameHeaderPrefix), nameHeaderSuffix)
		if payload == "" || strings.ContainsAny(payload, "\r\n") {
			return "", false
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(payload)
		if err != nil || len(decoded) == 0 || !utf8.Valid(decoded) || base64.StdEncoding.EncodeToString(decoded) != payload {
			return "", false
		}
		return string(decoded), true
	}
	if value[0] == ' ' || value[0] == '\t' || value[len(value)-1] == ' ' || value[len(value)-1] == '\t' {
		return "", false
	}
	for i := 0; i < len(value); i++ {
		if value[i] == ' ' || value[i] == '\t' {
			continue
		}
		if value[i] < 0x21 || value[i] > 0x7e {
			return "", false
		}
	}
	return value, true
}

func isCurrentModernMethod(method string) bool {
	switch method {
	case "server/discover", "tools/list", "tools/call":
		return true
	default:
		return false
	}
}
