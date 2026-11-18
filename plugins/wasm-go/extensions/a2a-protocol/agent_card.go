// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const (
	canonicalAgentCardPath = "/.well-known/agent-card.json"
	legacyAgentCardPath    = "/.well-known/agent.json"
	defaultMaxCardBytes    = 256 << 10
	hardMaxCardBytes       = 1 << 20

	cardSignaturePreserve = "preserve"
	cardSignatureResign   = "resign"
)

var (
	errInvalidAgentCard = errors.New("invalid A2A Agent Card")
	errUnsafeEndpoint   = errors.New("unsafe A2A Agent Card endpoint")
)

type cardResult struct {
	body      []byte
	rewritten bool
}

func processAgentCard(body []byte, config pluginConfig, legacyPath bool, externalURL string) (cardResult, error) {
	if len(body) > config.AgentCard.MaxResponseBytes {
		return cardResult{}, fmt.Errorf("%w: response exceeds configured limit", errInvalidAgentCard)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var card map[string]json.RawMessage
	if err := decoder.Decode(&card); err != nil || card == nil {
		return cardResult{}, fmt.Errorf("%w: response must be a JSON object", errInvalidAgentCard)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return cardResult{}, fmt.Errorf("%w: trailing JSON data", errInvalidAgentCard)
	}
	signed, err := validateCardSignatures(card)
	if err != nil {
		return cardResult{}, err
	}
	if signed && config.AgentCard.rewrite && config.AgentCard.SignatureMode == cardSignatureResign {
		return cardResult{}, fmt.Errorf("%w: signature resigning is not configured", errInvalidAgentCard)
	}
	exposeOriginalEndpoints := !config.AgentCard.rewrite || signed
	legacy, err := detectLegacyCard(card, config.Legacy03.Enabled, legacyPath)
	if err != nil {
		return cardResult{}, err
	}
	if legacy {
		if err := validateAndRewriteLegacyCard(card, externalURL, config.AgentCard.rewrite, exposeOriginalEndpoints); err != nil {
			return cardResult{}, err
		}
	} else {
		if err := validateV1Capabilities(card, config.AgentCard.rewrite && !signed); err != nil {
			return cardResult{}, err
		}
		if err := validateAndRewriteV1Card(card, externalURL, config.AgentCard.rewrite, exposeOriginalEndpoints); err != nil {
			return cardResult{}, err
		}
	}
	if signed && config.AgentCard.rewrite {
		return cardResult{body: body}, nil
	}
	if !config.AgentCard.rewrite {
		return cardResult{body: body}, nil
	}
	rewritten, err := json.Marshal(card)
	if err != nil {
		return cardResult{}, fmt.Errorf("%w: encode rewritten response", errInvalidAgentCard)
	}
	return cardResult{body: rewritten, rewritten: true}, nil
}

func validateV1Capabilities(card map[string]json.RawMessage, canRewrite bool) error {
	raw, ok := card["capabilities"]
	if !ok {
		return nil
	}
	var capabilities map[string]json.RawMessage
	if json.Unmarshal(raw, &capabilities) != nil || capabilities == nil {
		return fmt.Errorf("%w: capabilities must be an object", errInvalidAgentCard)
	}
	if extended, ok := capabilities["extendedAgentCard"]; ok {
		var enabled bool
		if json.Unmarshal(extended, &enabled) != nil {
			return fmt.Errorf("%w: capabilities.extendedAgentCard must be boolean", errInvalidAgentCard)
		}
	}
	if _, obsolete := capabilities["stateTransitionHistory"]; obsolete {
		if !canRewrite {
			return fmt.Errorf("%w: capabilities.stateTransitionHistory is not part of A2A 1.0", errInvalidAgentCard)
		}
		delete(capabilities, "stateTransitionHistory")
		card["capabilities"], _ = json.Marshal(capabilities)
	}
	return nil
}

func detectLegacyCard(card map[string]json.RawMessage, legacyEnabled, legacyPath bool) (bool, error) {
	if legacyPath {
		if !legacyEnabled {
			return false, fmt.Errorf("%w: legacy Agent Cards are disabled", errInvalidAgentCard)
		}
		return true, nil
	}
	if _, ok := card["supportedInterfaces"]; ok {
		return false, nil
	}
	if _, ok := card["url"]; ok && legacyEnabled {
		return true, nil
	}
	return false, nil
}

func validateAndRewriteV1Card(card map[string]json.RawMessage, externalURL string, rewrite, exposeOriginalEndpoints bool) error {
	raw, ok := card["supportedInterfaces"]
	if !ok {
		return fmt.Errorf("%w: supportedInterfaces is required", errInvalidAgentCard)
	}
	var interfaces []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &interfaces); err != nil || len(interfaces) == 0 {
		return fmt.Errorf("%w: supportedInterfaces must be a non-empty array", errInvalidAgentCard)
	}
	jsonRPCInterfaces := make([]map[string]json.RawMessage, 0, len(interfaces))
	for i, iface := range interfaces {
		binding, err := requiredString(iface, "protocolBinding")
		if err != nil {
			return fmt.Errorf("%w: supportedInterfaces[%d].protocolBinding is required", errInvalidAgentCard, i)
		}
		protocolVersion, err := requiredString(iface, "protocolVersion")
		if err != nil {
			return fmt.Errorf("%w: supportedInterfaces[%d].protocolVersion is required", errInvalidAgentCard, i)
		}
		if !isJSONRPCBinding(binding) || protocolVersion != "1.0" {
			if exposeOriginalEndpoints || !rewrite {
				return fmt.Errorf("%w: preserved Card advertises an unsupported interface", errInvalidAgentCard)
			}
			continue
		}
		if rawTenant, ok := iface["tenant"]; ok {
			var tenant string
			if json.Unmarshal(rawTenant, &tenant) != nil || strings.TrimSpace(tenant) == "" {
				return fmt.Errorf("%w: supportedInterfaces[%d].tenant must be a non-empty string", errInvalidAgentCard, i)
			}
		}
		endpoint, err := requiredString(iface, "url")
		if err != nil {
			return fmt.Errorf("%w: supportedInterfaces[%d].url", errInvalidAgentCard, i)
		}
		if err := validateDeclaredEndpoint(endpoint, externalURL, exposeOriginalEndpoints); err != nil {
			return fmt.Errorf("supportedInterfaces[%d]: %w", i, err)
		}
		if rewrite {
			iface["url"], _ = json.Marshal(externalURL)
		}
		jsonRPCInterfaces = append(jsonRPCInterfaces, iface)
	}
	if len(jsonRPCInterfaces) == 0 {
		return fmt.Errorf("%w: a JSON-RPC 1.0 interface is required", errInvalidAgentCard)
	}
	if rewrite {
		card["supportedInterfaces"], _ = json.Marshal(jsonRPCInterfaces)
	}
	return nil
}

func validateAndRewriteLegacyCard(card map[string]json.RawMessage, externalURL string, rewrite, exposeOriginalEndpoints bool) error {
	endpoint, err := requiredString(card, "url")
	if err != nil {
		return fmt.Errorf("%w: url is required", errInvalidAgentCard)
	}
	if err := validateDeclaredEndpoint(endpoint, externalURL, exposeOriginalEndpoints); err != nil {
		return err
	}
	if raw, ok := card["preferredTransport"]; ok {
		var transport string
		if json.Unmarshal(raw, &transport) != nil || !isJSONRPCBinding(transport) {
			return fmt.Errorf("%w: unsupported preferredTransport", errInvalidAgentCard)
		}
	}
	if raw, ok := card["additionalInterfaces"]; ok {
		var interfaces []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &interfaces); err != nil {
			return fmt.Errorf("%w: additionalInterfaces must be an array", errInvalidAgentCard)
		}
		jsonRPCInterfaces := make([]map[string]json.RawMessage, 0, len(interfaces))
		for i, iface := range interfaces {
			transport, err := requiredString(iface, "transport")
			if err != nil {
				return fmt.Errorf("%w: additionalInterfaces[%d].transport is required", errInvalidAgentCard, i)
			}
			if !isJSONRPCBinding(transport) {
				if exposeOriginalEndpoints || !rewrite {
					return fmt.Errorf("%w: preserved Card advertises an unsupported interface", errInvalidAgentCard)
				}
				continue
			}
			endpoint, err := requiredString(iface, "url")
			if err != nil {
				return fmt.Errorf("%w: additionalInterfaces[%d].url is required", errInvalidAgentCard, i)
			}
			if err := validateDeclaredEndpoint(endpoint, externalURL, exposeOriginalEndpoints); err != nil {
				return fmt.Errorf("additionalInterfaces[%d]: %w", i, err)
			}
			if rewrite {
				iface["url"], _ = json.Marshal(externalURL)
			}
			jsonRPCInterfaces = append(jsonRPCInterfaces, iface)
		}
		if rewrite {
			card["additionalInterfaces"], _ = json.Marshal(jsonRPCInterfaces)
		}
	}
	if rewrite {
		card["url"], _ = json.Marshal(externalURL)
	}
	return nil
}

func requiredString(object map[string]json.RawMessage, key string) (string, error) {
	raw, ok := object[key]
	if !ok {
		return "", errInvalidAgentCard
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
		return "", errInvalidAgentCard
	}
	return value, nil
}

func isJSONRPCBinding(binding string) bool {
	normalized := strings.NewReplacer("-", "", "+", "", "_", "", " ", "").Replace(strings.ToUpper(strings.TrimSpace(binding)))
	return normalized == "JSONRPC"
}

func validateDeclaredEndpoint(raw, externalURL string, exposed bool) error {
	if err := validateEndpointURL(raw, exposed); err != nil {
		return err
	}
	if exposed && !sameEndpoint(raw, externalURL) {
		return fmt.Errorf("%w: preserved endpoint is not the configured external endpoint", errUnsafeEndpoint)
	}
	return nil
}

func sameEndpoint(raw, configured string) bool {
	normalize := func(value string) string {
		parsed, err := url.Parse(value)
		if err != nil {
			return ""
		}
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.Host = strings.ToLower(parsed.Host)
		return strings.TrimRight(parsed.String(), "/")
	}
	return normalize(raw) != "" && normalize(raw) == normalize(configured)
}

func validateHTTPSURL(raw string) error {
	return validateEndpointURL(raw, true)
}

func validateEndpointURL(raw string, requirePublicHTTPS bool) error {
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" || (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return fmt.Errorf("%w: endpoint must be an absolute HTTP or HTTPS URL", errUnsafeEndpoint)
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("%w: credentials and fragments are forbidden", errUnsafeEndpoint)
	}
	if err := validatePort(parsed.Port()); err != nil {
		return err
	}
	if !requirePublicHTTPS {
		return nil
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return fmt.Errorf("%w: exposed endpoint must use HTTPS", errUnsafeEndpoint)
	}
	return validatePublicHost(parsed.Hostname())
}

func validatePublicHost(host string) error {
	normalized := strings.TrimSuffix(strings.ToLower(host), ".")
	if normalized == "localhost" || strings.HasSuffix(normalized, ".localhost") {
		return fmt.Errorf("%w: loopback host is forbidden", errUnsafeEndpoint)
	}
	if isNonCanonicalIPv4Literal(normalized) {
		return fmt.Errorf("%w: non-canonical IP literals are forbidden", errUnsafeEndpoint)
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() || ip.IsMulticast() {
			return fmt.Errorf("%w: loopback, link-local, private, multicast, and unspecified addresses are forbidden", errUnsafeEndpoint)
		}
	}
	return nil
}

func isNonCanonicalIPv4Literal(host string) bool {
	if strings.Contains(host, ":") || net.ParseIP(host) != nil {
		return false
	}
	parts := strings.Split(host, ".")
	if len(parts) > 4 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		if _, err := strconv.ParseUint(part, 0, 64); err != nil {
			if _, err = strconv.ParseUint(part, 10, 64); err != nil {
				return false
			}
		}
	}
	return true
}

func validatePort(port string) error {
	if port == "" {
		return nil
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("%w: invalid endpoint port", errUnsafeEndpoint)
	}
	return nil
}

type cardSignature struct {
	Protected string `json:"protected"`
	Signature string `json:"signature"`
}

func validateCardSignatures(card map[string]json.RawMessage) (bool, error) {
	raw, ok := card["signatures"]
	if !ok {
		return false, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false, fmt.Errorf("%w: signatures must be an array", errInvalidAgentCard)
	}
	var signatures []cardSignature
	if err := json.Unmarshal(raw, &signatures); err != nil {
		return false, fmt.Errorf("%w: signatures must be an array", errInvalidAgentCard)
	}
	for i, signature := range signatures {
		if strings.TrimSpace(signature.Protected) == "" || strings.TrimSpace(signature.Signature) == "" {
			return false, fmt.Errorf("%w: signatures[%d] requires protected and signature", errInvalidAgentCard, i)
		}
	}
	return len(signatures) > 0, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing interface{}
	if err := decoder.Decode(&trailing); err == nil {
		return errors.New("trailing JSON value")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func deriveExternalAgentURL(config pluginConfig) (string, error) {
	if config.Agent.ExternalBaseURL == "" {
		return "", fmt.Errorf("agent.externalBaseURL is required for Agent Card publication")
	}
	if err := validateHTTPSURL(config.Agent.ExternalBaseURL); err != nil {
		return "", fmt.Errorf("configured agent.externalBaseURL: %w", err)
	}
	return strings.TrimRight(config.Agent.ExternalBaseURL, "/"), nil
}

func rewrittenETag(body []byte) string {
	digest := sha256.Sum256(body)
	return `"` + hex.EncodeToString(digest[:]) + `"`
}
