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

package agent

import (
	"strings"
	"testing"

	"github.com/higress-group/openapi-to-mcpserver/pkg/models"
)

// leakedOpenAPIMCPDefaultCredential is the fixed value `hgctl mcp add --type openapi`
// previously injected into the first generated security scheme.
const leakedOpenAPIMCPDefaultCredential = "b5b9752c7ad2cb9c6b19fb5fd6a23be8852eca9c"

func requiredAPIKeyMCPConfig() *models.MCPConfig {
	return &models.MCPConfig{
		Server: models.ServerConfig{
			Name: "pets",
			SecuritySchemes: []models.SecurityScheme{{
				ID:   "ApiKeyAuth",
				Type: "apiKey",
				In:   "header",
				Name: "X-API-Key",
			}},
		},
		Tools: []models.Tool{{
			Name: "listPets",
			RequestTemplate: models.RequestTemplate{
				URL:    "https://api.example.com/pets",
				Method: "GET",
				Security: &models.ToolSecurityRequirement{
					ID: "ApiKeyAuth",
				},
			},
		}},
	}
}

func TestPrepareOpenAPIMCPConfigDoesNotInjectDefaultCredential(t *testing.T) {
	config := requiredAPIKeyMCPConfig()

	prepareOpenAPIMCPConfig(config)

	if got := config.Server.SecuritySchemes[0].DefaultCredential; got != "" {
		t.Fatalf("DefaultCredential = %q, want empty", got)
	}

	published := convertMCPConfigToStr(config)
	if strings.Contains(published, leakedOpenAPIMCPDefaultCredential) {
		t.Fatalf("published OpenAPI MCP config contains the fixed DefaultCredential:\n%s", published)
	}
	if strings.Contains(published, "defaultCredential") {
		t.Fatalf("published OpenAPI MCP config must omit defaultCredential, got:\n%s", published)
	}
}

func TestPrepareOpenAPIMCPConfigPreservesExplicitCredential(t *testing.T) {
	config := requiredAPIKeyMCPConfig()
	config.Server.SecuritySchemes[0].DefaultCredential = "user-supplied-key"

	prepareOpenAPIMCPConfig(config)

	if got := config.Server.SecuritySchemes[0].DefaultCredential; got != "user-supplied-key" {
		t.Fatalf("DefaultCredential = %q, want %q", got, "user-supplied-key")
	}
}

func TestPrepareOpenAPIMCPConfigMissingCredentialPath(t *testing.T) {
	config := requiredAPIKeyMCPConfig()

	prepareOpenAPIMCPConfig(config)

	scheme := config.Server.SecuritySchemes[0]
	sec := config.Tools[0].RequestTemplate.Security
	if sec == nil || sec.ID == "" {
		t.Fatal("generated tool must require the converted security scheme")
	}
	if sec.Passthrough {
		t.Fatal("generated tool security must not enable passthrough by default")
	}
	if scheme.DefaultCredential != "" {
		t.Fatalf("required-but-missing credential was filled with %q", scheme.DefaultCredential)
	}

	// ApplySecurity uses DefaultCredential when the request has no passthrough
	// or per-tool credential. Leaving it empty is the missing-credential path:
	// the helper must not send an implicit secret to upstream.
	published := convertMCPConfigToStr(config)
	if strings.Contains(published, leakedOpenAPIMCPDefaultCredential) {
		t.Fatal("missing-credential path still publishes the fixed DefaultCredential")
	}
}

func TestPrepareOpenAPIMCPConfigEmptySchemes(t *testing.T) {
	config := &models.MCPConfig{
		Server: models.ServerConfig{Name: "open"},
	}

	prepareOpenAPIMCPConfig(config)

	if len(config.Server.SecuritySchemes) != 0 {
		t.Fatalf("unexpected security schemes: %#v", config.Server.SecuritySchemes)
	}
}

func TestParseOpenapiSpecDoesNotInjectDefaultCredential(t *testing.T) {
	h := &MCPAddHandler{arg: MCPAddArg{
		name: "pets",
		spec: "testdata/openapi-apikey.yaml",
		typ:  OPENAPI,
	}}

	config := h.parseOpenapiSpec()
	if config == nil {
		t.Fatal("parseOpenapiSpec returned nil")
	}
	if len(config.Server.SecuritySchemes) == 0 {
		t.Fatal("expected converted security schemes")
	}

	for _, scheme := range config.Server.SecuritySchemes {
		if scheme.DefaultCredential != "" {
			t.Fatalf("scheme %s DefaultCredential = %q, want empty", scheme.ID, scheme.DefaultCredential)
		}
	}

	published := convertMCPConfigToStr(config)
	if strings.Contains(published, leakedOpenAPIMCPDefaultCredential) {
		t.Fatalf("published OpenAPI MCP config contains the fixed DefaultCredential:\n%s", published)
	}
	if strings.Contains(published, "defaultCredential") {
		t.Fatalf("published OpenAPI MCP config must omit defaultCredential, got:\n%s", published)
	}

	foundRequiredScheme := false
	for _, tool := range config.Tools {
		if tool.RequestTemplate.Security != nil && tool.RequestTemplate.Security.ID != "" {
			foundRequiredScheme = true
			if tool.RequestTemplate.Security.Passthrough {
				t.Fatalf("tool %s enabled passthrough by default", tool.Name)
			}
		}
	}
	if !foundRequiredScheme {
		t.Fatal("expected a tool that requires the converted security scheme")
	}
}
