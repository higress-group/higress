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

package services

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
)

const (
	a2aPluginName          = "a2a-protocol"
	canonicalAgentCardPath = "/.well-known/agent-card.json"
)

type a2aRoutePublication struct {
	name     string
	route    map[string]interface{}
	instance map[string]interface{}
}

type a2aPublication struct {
	service map[string]interface{}
	routes  []a2aRoutePublication
	plugin  map[string]interface{}
}

// PublishA2A creates the service and route before attaching the protocol
// plugin to that route. It does not create or persist Agent task state.
func PublishA2A(client *HigressClient, name, rawURL, externalBaseURL, pluginURL string) error {
	publication, err := buildA2APublication(name, rawURL, externalBaseURL, pluginURL)
	if err != nil {
		return err
	}
	if response, err := client.Post("/v1/service-sources", publication.service); err != nil {
		return fmt.Errorf("publish A2A service (%s): %w", boundedResponse(response), err)
	}
	for _, route := range publication.routes {
		if response, err := client.Post("/v1/routes", route.route); err != nil {
			return fmt.Errorf("publish A2A route %s (%s): %w", route.name, boundedResponse(response), err)
		}
	}
	if response, err := client.Post("/v1/wasm-plugins", publication.plugin); err != nil {
		if !errors.Is(err, errResourceAlreadyExists) {
			return fmt.Errorf("publish A2A WasmPlugin (%s): %w", boundedResponse(response), err)
		}
	}
	for _, route := range publication.routes {
		instancePath := fmt.Sprintf("/v1/routes/%s/plugin-instances/%s", url.PathEscape(route.name), a2aPluginName)
		if response, err := client.Put(instancePath, route.instance); err != nil {
			return fmt.Errorf("attach A2A WasmPlugin to %s (%s): %w", route.name, boundedResponse(response), err)
		}
	}
	return nil
}

func buildA2APublication(name, rawURL, externalBaseURL, pluginURL string) (*a2aPublication, error) {
	if name == "" {
		return nil, fmt.Errorf("A2A agent name is required")
	}
	upstream, err := url.Parse(rawURL)
	if err != nil || upstream.Hostname() == "" || (upstream.Scheme != "http" && upstream.Scheme != "https") {
		return nil, fmt.Errorf("invalid A2A agent URL %q", rawURL)
	}
	external, err := url.Parse(externalBaseURL)
	if err != nil || externalBaseURL == "" || !external.IsAbs() || external.Hostname() == "" || !strings.EqualFold(external.Scheme, "https") || external.User != nil || external.Fragment != "" || validateA2AExternalPort(external.Port()) != nil || !isPublicA2AHost(external.Hostname()) {
		return nil, fmt.Errorf("a public HTTPS external A2A base URL is required")
	}
	pluginRepository, pluginVersion, err := parseA2APluginURL(pluginURL)
	if err != nil {
		return nil, err
	}
	port := upstream.Port()
	if port == "" {
		if upstream.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	serviceType := "dns"
	serviceDomain := upstream.Hostname()
	servicePort := interface{}(port)
	if net.ParseIP(upstream.Hostname()) != nil {
		serviceType = "static"
		serviceDomain = net.JoinHostPort(upstream.Hostname(), port)
		servicePort = 80
	}
	serviceName := "agent-" + name
	targetService := serviceName + "." + serviceType
	routeName := name + "-route"
	discoveryRouteName := name + "-discovery-route"
	matchPath := path.Clean("/" + strings.TrimPrefix(upstream.EscapedPath(), "/"))
	if matchPath == "." {
		matchPath = "/"
	}
	externalPath := path.Clean("/" + strings.TrimPrefix(external.EscapedPath(), "/"))
	if externalPath != matchPath {
		return nil, fmt.Errorf("external A2A URL path %q must match route path %q", externalPath, matchPath)
	}
	service := map[string]interface{}{
		"domain": serviceDomain, "type": serviceType, "port": servicePort,
		"name": serviceName, "proxyName": "", "domainForEdit": serviceDomain, "protocol": upstream.Scheme,
	}
	route := map[string]interface{}{
		"name":       routeName,
		"path":       map[string]interface{}{"matchType": "PRE", "matchValue": matchPath, "caseSensitive": true},
		"authConfig": map[string]interface{}{"enabled": false},
		"services":   []map[string]interface{}{{"name": targetService}},
	}
	discoveryRoute := map[string]interface{}{
		"name":       discoveryRouteName,
		"path":       map[string]interface{}{"matchType": "EQUAL", "matchValue": canonicalAgentCardPath, "caseSensitive": true},
		"authConfig": map[string]interface{}{"enabled": false},
		"services":   []map[string]interface{}{{"name": targetService}},
	}
	plugin := map[string]interface{}{
		"name": a2aPluginName, "pluginVersion": "1.0.0", "category": "protocol",
		"title": "A2A Protocol", "description": "Bounded A2A 1.0 JSON-RPC and SSE metadata extraction",
		"builtIn":         false,
		"imageRepository": pluginRepository,
		"imageVersion":    pluginVersion, "imagePullPolicy": "IfNotPresent", "phase": "AUTHN", "priority": 300,
	}
	configurations := map[string]interface{}{
		"protocolVersion": "1.0", "mode": "enforce",
		"legacy03":      map[string]interface{}{"enabled": false},
		"agent":         map[string]interface{}{"id": name, "externalBaseURL": strings.TrimRight(externalBaseURL, "/")},
		"jsonrpc":       map[string]interface{}{"maxRequestBytes": 4194304, "maxSSEEventBytes": 262144},
		"authorization": map[string]interface{}{"exposeInternalHeaders": true},
	}
	instance := func(target string) map[string]interface{} {
		return map[string]interface{}{
			"targets":        map[string]interface{}{"ROUTE": target},
			"pluginName":     a2aPluginName,
			"pluginVersion":  "1.0.0",
			"enabled":        true,
			"internal":       false,
			"configurations": configurations,
		}
	}
	return &a2aPublication{
		service: service,
		plugin:  plugin,
		routes: []a2aRoutePublication{
			{name: routeName, route: route, instance: instance(routeName)},
			{name: discoveryRouteName, route: discoveryRoute, instance: instance(discoveryRouteName)},
		},
	}, nil
}

func parseA2APluginURL(raw string) (string, string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "oci" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", fmt.Errorf("a2a-plugin-url must be an operator-approved OCI image URL with an explicit tag")
	}
	withoutScheme := strings.TrimPrefix(raw, "oci://")
	lastSlash := strings.LastIndexByte(withoutScheme, '/')
	tagSeparator := strings.LastIndexByte(withoutScheme, ':')
	if tagSeparator <= lastSlash || tagSeparator == len(withoutScheme)-1 || strings.Contains(withoutScheme, "@") {
		return "", "", fmt.Errorf("a2a-plugin-url must be an operator-approved OCI image URL with an explicit tag")
	}
	repository := "oci://" + withoutScheme[:tagSeparator]
	version := withoutScheme[tagSeparator+1:]
	if strings.ContainsAny(version, "/?#") {
		return "", "", fmt.Errorf("a2a-plugin-url must be an operator-approved OCI image URL with an explicit tag")
	}
	return repository, version, nil
}

func validateA2AExternalPort(port string) error {
	if port == "" {
		return nil
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("invalid external A2A port")
	}
	return nil
}

func isPublicA2AHost(host string) bool {
	normalized := strings.TrimSuffix(strings.ToLower(host), ".")
	if normalized == "localhost" || strings.HasSuffix(normalized, ".localhost") || isNonCanonicalA2AIPv4Literal(normalized) {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsPrivate() && !ip.IsMulticast()
	}
	return true
}

func isNonCanonicalA2AIPv4Literal(host string) bool {
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

func boundedResponse(response []byte) string {
	const max = 512
	if len(response) > max {
		response = response[:max]
	}
	return strings.TrimSpace(string(response))
}
