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
	"net/url"
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

// Metadata is required on every modern request. ClientInfo is optional.
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

// PrepareRequest classifies a request exactly once. Legacy exchanges are
// passed through unchanged. Modern exchanges complete all transport,
// envelope, identity, lifecycle, and method validation before being returned.
func PrepareRequest(transport Transport, body []byte, methodAvailable func(string) bool) (*RequestContext, *Error) {
	return PrepareRequestWithPolicy(transport, body, func(method string) MethodPolicy {
		return MethodPolicy{Available: methodAvailable != nil && methodAvailable(method)}
	})
}

func PrepareRequestWithPolicy(transport Transport, body []byte, policyLookup MethodPolicyLookup) (*RequestContext, *Error) {
	if protocolError := ValidateOrigin(transport); protocolError != nil {
		return nil, protocolError
	}
	era, classificationError := classify(transport, body)
	if classificationError != nil {
		return nil, classificationError
	}
	if era == EraLegacy {
		return &RequestContext{
			Era:       EraLegacy,
			Version:   Version(transport.ProtocolVersion),
			Transport: transport,
			Envelope:  Envelope{Raw: bytes.Clone(body)},
		}, nil
	}
	if len(body) > int(ModernMaxBodyBytes) {
		return nil, RequestTooLarge()
	}
	if transport.ProtocolVersion != "" &&
		!IsModernVersion(Version(transport.ProtocolVersion)) &&
		!IsLegacyVersion(Version(transport.ProtocolVersion)) {
		return nil, UnsupportedVersion(Version(transport.ProtocolVersion), SupportedVersions())
	}
	if protocolError := ValidateModernTransport(transport); protocolError != nil {
		return nil, protocolError
	}
	if !IsModernVersion(Version(transport.ProtocolVersion)) || transport.MCPMethod == "" {
		return nil, HeaderMismatch()
	}
	envelope, protocolError := decodeEnvelope(body)
	if protocolError != nil {
		return nil, protocolError
	}
	request := &RequestContext{
		Era:       EraModern,
		Version:   Version20260728,
		Transport: transport,
		Envelope:  envelope,
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
	if len(params) == 0 {
		return Metadata{}, HeaderMismatch()
	}
	var decoded struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	if err := json.Unmarshal(params, &decoded); err != nil || decoded.Meta == nil {
		return Metadata{}, HeaderMismatch()
	}
	var version string
	if err := json.Unmarshal(decoded.Meta[MetaProtocolVersion], &version); err != nil || version == "" {
		return Metadata{}, HeaderMismatch()
	}
	capabilitiesRaw, ok := decoded.Meta[MetaClientCapabilities]
	if !ok || !isJSONObject(capabilitiesRaw) {
		return Metadata{}, InvalidParams("modern MCP clientCapabilities metadata is required")
	}
	var capabilities ClientCapabilities
	if err := json.Unmarshal(capabilitiesRaw, &capabilities); err != nil {
		return Metadata{}, InvalidParams("modern MCP clientCapabilities metadata is invalid")
	}
	metadata := Metadata{
		ProtocolVersion:    Version(version),
		ClientCapabilities: capabilities,
		Extensions:         make(map[string]json.RawMessage),
	}
	for name, value := range decoded.Meta {
		if name != MetaProtocolVersion && name != MetaClientCapabilities && name != MetaClientInfo {
			metadata.Extensions[name] = bytes.Clone(value)
		}
	}
	if clientInfoRaw, ok := decoded.Meta[MetaClientInfo]; ok {
		if !validImplementationShape(clientInfoRaw) {
			return Metadata{}, InvalidParams("modern MCP clientInfo metadata is invalid")
		}
		var clientInfo Implementation
		decoder := json.NewDecoder(bytes.NewReader(clientInfoRaw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&clientInfo); err != nil || !validImplementation(clientInfo) {
			return Metadata{}, InvalidParams("modern MCP clientInfo metadata is invalid")
		}
		metadata.ClientInfo = &clientInfo
	}
	return metadata, nil
}

func validImplementationShape(raw json.RawMessage) bool {
	members, err := decodeJSONObject(raw)
	if err != nil || !validRequiredString(members, "name") || !validRequiredString(members, "version") {
		return false
	}
	for _, name := range []string{"title", "description", "websiteUrl"} {
		if value, ok := members[name]; ok && !validJSONString(value) {
			return false
		}
	}
	iconsRaw, hasIcons := members["icons"]
	if !hasIcons {
		return true
	}
	var icons []json.RawMessage
	if isJSONNull(iconsRaw) || json.Unmarshal(iconsRaw, &icons) != nil || icons == nil {
		return false
	}
	for _, iconRaw := range icons {
		icon, iconErr := decodeJSONObject(iconRaw)
		if iconErr != nil || !validRequiredString(icon, "src") {
			return false
		}
		for _, name := range []string{"mimeType", "theme"} {
			if value, ok := icon[name]; ok && !validJSONString(value) {
				return false
			}
		}
		if sizesRaw, ok := icon["sizes"]; ok {
			var sizes []json.RawMessage
			if isJSONNull(sizesRaw) || json.Unmarshal(sizesRaw, &sizes) != nil || sizes == nil {
				return false
			}
			for _, size := range sizes {
				if !validJSONString(size) {
					return false
				}
			}
		}
	}
	return true
}

func validRequiredString(members JSONObject, name string) bool {
	value, ok := members[name]
	return ok && validJSONString(value)
}

func validJSONString(raw json.RawMessage) bool {
	if isJSONNull(raw) {
		return false
	}
	var value string
	return json.Unmarshal(raw, &value) == nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func validImplementation(clientInfo Implementation) bool {
	if clientInfo.Name == "" || clientInfo.Version == "" || len(clientInfo.Name) > 1024 || len(clientInfo.Version) > 1024 {
		return false
	}
	if clientInfo.WebsiteURL != "" {
		websiteURL, err := url.Parse(clientInfo.WebsiteURL)
		if err != nil || websiteURL.User != nil || websiteURL.Host == "" || (websiteURL.Scheme != "http" && websiteURL.Scheme != "https") {
			return false
		}
	}
	for _, icon := range clientInfo.Icons {
		if icon.Source == "" || len(icon.Source) > 4096 {
			return false
		}
		source, err := url.Parse(icon.Source)
		if err != nil || !source.IsAbs() {
			return false
		}
		if icon.Theme != "" && icon.Theme != "light" && icon.Theme != "dark" {
			return false
		}
		for _, size := range icon.Sizes {
			if size == "" {
				return false
			}
		}
	}
	return true
}

const (
	nameHeaderPrefix  = "=?base64?"
	nameHeaderSuffix  = "?="
	maxEncodedNameLen = 8192
	maxDecodedNameLen = 4096
)

func validateNameHeader(transport Transport, envelope Envelope) *Error {
	hasNameHeader := transport.HasMCPName || transport.MCPName != ""
	if envelope.Method != "tools/call" {
		if hasNameHeader {
			return HeaderMismatch()
		}
		return nil
	}
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(envelope.Params, &params); err != nil || params.Name == "" || len(params.Name) > maxDecodedNameLen {
		return InvalidParams("tools/call params.name is required")
	}
	decodedName, ok := decodeNameHeader(transport.MCPName, hasNameHeader)
	if !ok || decodedName != params.Name {
		return HeaderMismatch()
	}
	return nil
}

func decodeNameHeader(value string, present bool) (string, bool) {
	if !present || value == "" || len(value) > maxEncodedNameLen {
		return "", false
	}
	if strings.HasPrefix(value, nameHeaderPrefix) || strings.HasSuffix(value, nameHeaderSuffix) {
		if !strings.HasPrefix(value, nameHeaderPrefix) || !strings.HasSuffix(value, nameHeaderSuffix) {
			return "", false
		}
		payload := strings.TrimSuffix(strings.TrimPrefix(value, nameHeaderPrefix), nameHeaderSuffix)
		if payload == "" || len(payload) > base64.StdEncoding.EncodedLen(maxDecodedNameLen) || strings.ContainsAny(payload, "\r\n") {
			return "", false
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(payload)
		if err != nil || len(decoded) == 0 || len(decoded) > maxDecodedNameLen || !utf8.Valid(decoded) || base64.StdEncoding.EncodeToString(decoded) != payload {
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
