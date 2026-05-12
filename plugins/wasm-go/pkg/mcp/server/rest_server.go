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
package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	_ "time/tzdata"

	template "github.com/higress-group/gjson_template"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/tidwall/sjson"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// RestMCPConfig represents the configuration for REST MCP server
type RestMCPConfig struct {
	SecuritySchemes           []SecurityScheme    `json:"securitySchemes,omitempty"`
	DefaultDownstreamSecurity SecurityRequirement `json:"defaultDownstreamSecurity,omitempty"` // Default client-to-gateway authentication for all tools
	DefaultUpstreamSecurity   SecurityRequirement `json:"defaultUpstreamSecurity,omitempty"`   // Default gateway-to-backend authentication for all tools
}

// RestToolArg represents an argument for a REST tool
type RestToolArg struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Type        string        `json:"type,omitempty"` // JSON Schema type: string, number, integer, boolean, array, object
	Required    bool          `json:"required,omitempty"`
	Default     interface{}   `json:"default,omitempty"`
	Enum        []interface{} `json:"enum,omitempty"`
	// For array type
	Items interface{} `json:"items,omitempty"`
	// For object type
	Properties interface{} `json:"properties,omitempty"`
	// Position specifies where the argument should be placed in the request
	// Valid values: query, path, header, cookie, body
	Position string `json:"position,omitempty"`
}

// RestToolHeader represents an HTTP header
type RestToolHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// RestToolRequestTemplate defines how to construct the HTTP request
type RestToolRequestTemplate struct {
	URL            string              `json:"url"`
	Method         string              `json:"method"`
	Headers        []RestToolHeader    `json:"headers"`
	Body           string              `json:"body"`
	ArgsToJsonBody bool                `json:"argsToJsonBody,omitempty"` // Use args as JSON body
	ArgsToUrlParam bool                `json:"argsToUrlParam,omitempty"` // Add args to URL parameters
	ArgsToFormBody bool                `json:"argsToFormBody,omitempty"` // Use args as form-urlencoded body
	Security       SecurityRequirement `json:"security,omitempty"`
}

// RestToolResponseTemplate defines how to transform the HTTP response
type RestToolResponseTemplate struct {
	Body        string `json:"body"`
	PrependBody string `json:"prependBody,omitempty"` // Text to insert before the response body
	AppendBody  string `json:"appendBody,omitempty"`  // Text to insert after the response body
}

// RestTool represents a REST API that can be called as an MCP tool
type RestTool struct {
	Name                  string                   `json:"name"`
	Description           string                   `json:"description"`
	Security              SecurityRequirement      `json:"security,omitempty"` // Tool-level security for MCP Client to MCP Server
	Args                  []RestToolArg            `json:"args"`
	OutputSchema          map[string]any           `json:"outputSchema,omitempty"` // Output schema for MCP Protocol Version 2025-06-18
	RequestTemplate       RestToolRequestTemplate  `json:"requestTemplate,omitempty"`
	ResponseTemplate      RestToolResponseTemplate `json:"responseTemplate"`
	ErrorResponseTemplate string                   `json:"errorResponseTemplate"`

	// Parsed templates (not from JSON)
	parsedURLTemplate           *template.Template
	parsedHeaderTemplates       map[string]*template.Template
	parsedBodyTemplate          *template.Template
	parsedResponseTemplate      *template.Template
	parsedErrorResponseTemplate *template.Template

	// Map of argument names to their positions
	argPositions map[string]string

	// Flag to indicate if this is a direct response tool (no HTTP request)
	isDirectResponseTool bool
}

// parseIP
func parseIP(source string, fromHeader bool) string {
	if fromHeader {
		source = strings.Split(source, ",")[0]
	}
	source = strings.Trim(source, " ")
	if strings.Contains(source, ".") {
		// parse ipv4
		return strings.Split(source, ":")[0]
	}
	//parse ipv6
	if strings.Contains(source, "]") {
		return strings.Split(source, "]")[0][1:]
	}
	return source
}

// templateFuncs returns the template functions map
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		// Get IP from socket
		"getSocketIP": func() string {
			bs, _ := proxywasm.GetProperty([]string{"source", "address"})
			if len(bs) > 0 {
				return parseIP(string(bs), false)
			}
			return ""
		},
		// Get IP from header, fallback to socket if not available
		"getRealIP": func() string {
			ipStr, _ := proxywasm.GetHttpRequestHeader("x-forwarded-for")
			if ipStr != "" {
				return parseIP(ipStr, true)
			}
			// Fallback to socket IP if header is not available
			bs, _ := proxywasm.GetProperty([]string{"source", "address"})
			if len(bs) > 0 {
				return parseIP(string(bs), false)
			}
			return ""
		},
	}
}

// parseTemplates parses all templates in the tool configuration
func (t *RestTool) parseTemplates() error {
	var err error

	// Check if this is a direct response tool (no RequestTemplate)
	if t.RequestTemplate.URL == "" {
		t.isDirectResponseTool = true
	} else {
		// Validate args configuration - only one of the three options can be true
		argsOptionCount := 0
		if t.RequestTemplate.ArgsToJsonBody {
			argsOptionCount++
		}
		if t.RequestTemplate.ArgsToUrlParam {
			argsOptionCount++
		}
		if t.RequestTemplate.ArgsToFormBody {
			argsOptionCount++
		}
		if argsOptionCount > 1 {
			return fmt.Errorf("only one of argsToJsonBody, argsToUrlParam, or argsToFormBody can be set to true")
		}

		// Parse URL template
		t.parsedURLTemplate, err = template.New("url").Funcs(templateFuncs()).Parse(t.RequestTemplate.URL)
		if err != nil {
			return fmt.Errorf("error parsing URL template: %v", err)
		}

		// Parse header templates
		t.parsedHeaderTemplates = make(map[string]*template.Template)
		for i, header := range t.RequestTemplate.Headers {
			if header.Key == "" {
				log.Warnf("Skipping header with empty key at index %d", i)
				continue
			}

			tmplName := fmt.Sprintf("header_%d", i)
			t.parsedHeaderTemplates[header.Key], err = template.New(tmplName).Funcs(templateFuncs()).Parse(header.Value)
			if err != nil {
				return fmt.Errorf("error parsing header template for %s: %v", header.Key, err)
			}
		}

		// Parse body template if present
		if t.RequestTemplate.Body != "" {
			t.parsedBodyTemplate, err = template.New("body").Funcs(templateFuncs()).Parse(t.RequestTemplate.Body)
			if err != nil {
				return fmt.Errorf("error parsing body template: %v", err)
			}
		}
	}

	// Parse response template if present
	if t.ResponseTemplate.Body != "" {
		// Validate that PrependBody and AppendBody are not used with Body
		if t.ResponseTemplate.PrependBody != "" || t.ResponseTemplate.AppendBody != "" {
			return fmt.Errorf("PrependBody and AppendBody cannot be used when Body is specified")
		}

		t.parsedResponseTemplate, err = template.New("response").Funcs(templateFuncs()).Parse(t.ResponseTemplate.Body)
		if err != nil {
			return fmt.Errorf("error parsing response template: %v", err)
		}
	} else if t.isDirectResponseTool {
		return errors.New("direct response mode must set responseTemplate.body")
	}

	// Parse error response template if present
	if t.ErrorResponseTemplate != "" {
		t.parsedErrorResponseTemplate, err = template.New("errorResponse").Funcs(templateFuncs()).Parse(t.ErrorResponseTemplate)
		if err != nil {
			return fmt.Errorf("error parsing error response template: %v", err)
		}
	}

	// Initialize argument positions map
	t.argPositions = make(map[string]string)
	for _, arg := range t.Args {
		if arg.Position != "" {
			t.argPositions[arg.Name] = strings.ToLower(arg.Position)
		}
	}

	return nil
}

// executeTemplate executes a parsed template with the given data
func executeTemplate(tmpl *template.Template, data []byte) (string, error) {
	if tmpl == nil {
		return "", errors.New("template is nil")
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// RestMCPServer implements Server interface for REST-to-MCP conversion
type RestMCPServer struct {
	name                      string
	base                      BaseMCPServer
	toolsConfig               map[string]RestTool // Store original tool configs for template rendering
	securitySchemes           map[string]SecurityScheme
	defaultDownstreamSecurity SecurityRequirement // Default client-to-gateway authentication
	defaultUpstreamSecurity   SecurityRequirement // Default gateway-to-backend authentication
	passthroughAuthHeader     bool                // If true, pass through Authorization header even without downstream security
}

// NewRestMCPServer creates a new REST-to-MCP server
func NewRestMCPServer(name string) *RestMCPServer {
	return &RestMCPServer{
		name:            name,
		base:            NewBaseMCPServer(),
		toolsConfig:     make(map[string]RestTool),
		securitySchemes: make(map[string]SecurityScheme), // Initialize the map
	}
}

// AddSecurityScheme adds a security scheme to the server's map
func (s *RestMCPServer) AddSecurityScheme(scheme SecurityScheme) {
	if s.securitySchemes == nil {
		s.securitySchemes = make(map[string]SecurityScheme)
	}
	s.securitySchemes[scheme.ID] = scheme
}

// GetSecurityScheme retrieves a security scheme by its ID from the map
func (s *RestMCPServer) GetSecurityScheme(id string) (SecurityScheme, bool) {
	scheme, ok := s.securitySchemes[id]
	return scheme, ok
}

// SetDefaultDownstreamSecurity sets the default downstream security configuration
func (s *RestMCPServer) SetDefaultDownstreamSecurity(security SecurityRequirement) {
	s.defaultDownstreamSecurity = security
}

// GetDefaultDownstreamSecurity gets the default downstream security configuration
func (s *RestMCPServer) GetDefaultDownstreamSecurity() SecurityRequirement {
	return s.defaultDownstreamSecurity
}

// SetDefaultUpstreamSecurity sets the default upstream security configuration
func (s *RestMCPServer) SetDefaultUpstreamSecurity(security SecurityRequirement) {
	s.defaultUpstreamSecurity = security
}

// GetDefaultUpstreamSecurity gets the default upstream security configuration
func (s *RestMCPServer) GetDefaultUpstreamSecurity() SecurityRequirement {
	return s.defaultUpstreamSecurity
}

// SetPassthroughAuthHeader sets the passthrough auth header flag
func (s *RestMCPServer) SetPassthroughAuthHeader(passthrough bool) {
	s.passthroughAuthHeader = passthrough
}

// GetPassthroughAuthHeader gets the passthrough auth header flag
func (s *RestMCPServer) GetPassthroughAuthHeader() bool {
	return s.passthroughAuthHeader
}

// AddMCPTool implements Server interface
func (s *RestMCPServer) AddMCPTool(name string, tool Tool) Server {
	s.base.AddMCPTool(name, tool)
	return s
}

// AddRestTool adds a REST tool configuration
func (s *RestMCPServer) AddRestTool(toolConfig RestTool) error {
	// Parse templates at configuration time
	if err := toolConfig.parseTemplates(); err != nil {
		return err
	}

	s.toolsConfig[toolConfig.Name] = toolConfig
	s.base.AddMCPTool(toolConfig.Name, &RestMCPTool{
		serverName: s.name,
		name:       toolConfig.Name,
		toolConfig: toolConfig,
	})

	return nil
}

// GetMCPTools implements Server interface
func (s *RestMCPServer) GetMCPTools() map[string]Tool {
	return s.base.GetMCPTools()
}

// SetConfig implements Server interface
func (s *RestMCPServer) SetConfig(config []byte) {
	s.base.SetConfig(config)
}

// GetConfig implements Server interface
func (s *RestMCPServer) GetConfig(v any) {
	s.base.GetConfig(v)
}

// Clone implements Server interface
func (s *RestMCPServer) Clone() Server {
	newServer := &RestMCPServer{
		name:            s.name,
		base:            s.base.CloneBase(),
		toolsConfig:     make(map[string]RestTool),
		securitySchemes: make(map[string]SecurityScheme), // Initialize the map
	}
	for k, v := range s.toolsConfig {
		newServer.toolsConfig[k] = v
	}
	// Deep copy securitySchemes
	if s.securitySchemes != nil {
		for k, v := range s.securitySchemes {
			newServer.securitySchemes[k] = v
		}
	}
	return newServer
}

// GetToolConfig returns the REST tool configuration for a given tool name
func (s *RestMCPServer) GetToolConfig(name string) (RestTool, bool) {
	config, ok := s.toolsConfig[name]
	return config, ok
}

// RestMCPTool implements Tool interface for REST-to-MCP
type RestMCPTool struct {
	serverName string
	name       string
	toolConfig RestTool
	arguments  map[string]interface{}
}

// Create implements Tool interface
func (t *RestMCPTool) Create(params []byte) Tool {
	newTool := &RestMCPTool{
		serverName: t.serverName,
		name:       t.name,
		toolConfig: t.toolConfig,
		arguments:  make(map[string]interface{}),
	}

	// Parse raw arguments
	var rawArgs map[string]interface{}
	if err := json.Unmarshal(params, &rawArgs); err != nil {
		log.Warnf("Failed to parse tool arguments: %v", err)
	}

	// Process arguments with type conversion
	for _, arg := range t.toolConfig.Args {
		// Check if argument was provided
		rawValue, exists := rawArgs[arg.Name]
		if !exists {
			// Apply default if available
			if arg.Default != nil {
				newTool.arguments[arg.Name] = arg.Default
			}
			continue
		}

		// Convert value based on type
		switch arg.Type {
		case "boolean":
			// Convert to boolean
			switch v := rawValue.(type) {
			case bool:
				newTool.arguments[arg.Name] = v
			case string:
				if v == "true" {
					newTool.arguments[arg.Name] = true
				} else if v == "false" {
					newTool.arguments[arg.Name] = false
				} else {
					newTool.arguments[arg.Name] = rawValue
				}
			default:
				newTool.arguments[arg.Name] = rawValue
			}
		case "integer":
			// Convert to integer
			switch v := rawValue.(type) {
			case float64:
				newTool.arguments[arg.Name] = int(v)
			case string:
				if intVal, err := json.Number(v).Int64(); err == nil {
					newTool.arguments[arg.Name] = int(intVal)
				} else {
					newTool.arguments[arg.Name] = rawValue
				}
			default:
				newTool.arguments[arg.Name] = rawValue
			}
		case "number":
			// Convert to number (float64)
			switch v := rawValue.(type) {
			case string:
				if floatVal, err := json.Number(v).Float64(); err == nil {
					newTool.arguments[arg.Name] = floatVal
				} else {
					newTool.arguments[arg.Name] = rawValue
				}
			default:
				newTool.arguments[arg.Name] = rawValue
			}
		default:
			// For string, array, object, or unspecified types, use as is
			newTool.arguments[arg.Name] = rawValue
		}
	}

	return newTool
}

// convertArgToString converts an argument value to a string representation
func convertArgToString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case bool, int, int64, float64:
		return fmt.Sprintf("%v", v)
	default:
		// For complex types, try to marshal to JSON
		if jsonBytes, err := json.Marshal(v); err == nil {
			return string(jsonBytes)
		}
		return fmt.Sprintf("%v", v)
	}
}

// hasContentType checks if the headers contain a specific content type
func hasContentType(headers [][2]string, contentTypeSubstr string) bool {
	for _, header := range headers {
		if strings.EqualFold(header[0], "Content-Type") && strings.Contains(strings.ToLower(header[1]), contentTypeSubstr) {
			return true
		}
	}
	return false
}

// applySecurity applies the configured security scheme to the request with fallback to default upstream security.
// It modifies reqCtx.Headers and reqCtx.ParsedURL (specifically RawQuery) in place if necessary.
func (t *RestMCPTool) applySecurity(serverObj Server, reqCtx *AuthRequestContext) error {
	restServer, ok := serverObj.(*RestMCPServer)
	if !ok {
		return errors.New("server is not a RestMCPServer")
	}

	// Determine which upstream security to use: tool-level or server's default
	var upstreamSecurity SecurityRequirement
	if t.toolConfig.RequestTemplate.Security.ID != "" {
		// Use tool-level upstream security if configured
		upstreamSecurity = t.toolConfig.RequestTemplate.Security
		log.Debugf("Using tool-level upstream security for tool %s: %s", t.name, upstreamSecurity.ID)
	} else {
		// Fall back to server's default upstream security
		upstreamSecurity = restServer.GetDefaultUpstreamSecurity()
		if upstreamSecurity.ID != "" {
			log.Debugf("Using default upstream security for tool %s: %s", t.name, upstreamSecurity.ID)
		}
	}

	// Apply security using the determined configuration
	return ApplySecurity(upstreamSecurity, restServer, reqCtx)
}

// Call implements Tool interface
func (t *RestMCPTool) Call(httpCtx HttpContext, server Server) error {
	ctx := httpCtx.(wrapper.HttpContext)

	// Get server instance for configuration access
	restServer, ok := server.(*RestMCPServer)
	if !ok {
		return fmt.Errorf("server is not a RestMCPServer")
	}

	// Handle tool-level or default downstream security: extract credential for passthrough if configured
	// toolConfig.Security represents client-to-gateway authentication, falls back to server's defaultDownstreamSecurity
	passthroughCredential := ""
	var downstreamSecurity SecurityRequirement
	if t.toolConfig.Security.ID != "" {
		// Use tool-level security if configured
		downstreamSecurity = t.toolConfig.Security
		log.Debugf("Using tool-level downstream security for tool %s: %s", t.name, downstreamSecurity.ID)
	} else {
		// Fall back to server's default downstream security
		downstreamSecurity = restServer.GetDefaultDownstreamSecurity()
		if downstreamSecurity.ID != "" {
			log.Debugf("Using default downstream security for tool %s: %s", t.name, downstreamSecurity.ID)
		}
	}

	if downstreamSecurity.ID != "" {
		clientScheme, schemeOk := restServer.GetSecurityScheme(downstreamSecurity.ID)
		if !schemeOk {
			log.Warnf("Downstream security scheme ID '%s' not found for tool %s.", downstreamSecurity.ID, t.name)
		} else {
			// Extract and remove the credential from the incoming request
			extractedCred, err := ExtractAndRemoveIncomingCredential(clientScheme)
			if err != nil {
				log.Warnf("Failed to extract/remove incoming credential for tool %s using scheme %s: %v", t.name, clientScheme.ID, err)
			} else if extractedCred == "" {
				log.Debugf("No incoming credential found for tool %s using scheme %s for extraction/removal.", t.name, clientScheme.ID)
			}

			// Only use passthrough if explicitly configured
			if downstreamSecurity.Passthrough && extractedCred != "" {
				passthroughCredential = extractedCred
				log.Debugf("Passthrough credential set for tool %s.", t.name)
			}
		}
	}

	var templateDataBytes []byte
	// Get server config for template data if needed (but don't use for default security)
	var serverConfig map[string]interface{}
	restServer.GetConfig(&serverConfig)
	templateDataBytes, _ = sjson.SetBytes(templateDataBytes, "config", serverConfig)
	templateDataBytes, _ = sjson.SetBytes(templateDataBytes, "args", t.arguments)

	// Check if this is a direct response tool (no HTTP request needed)
	if t.toolConfig.isDirectResponseTool {
		// Process response directly
		var result string

		// Render the response template with the arguments
		templateResult, err := executeTemplate(t.toolConfig.parsedResponseTemplate, templateDataBytes)
		if err != nil {
			return fmt.Errorf("error executing response template: %v", err)
		}
		result = templateResult

		// Check if tool has outputSchema and try to parse templateResult as structured content
		var structuredContent json.RawMessage
		if t.toolConfig.OutputSchema != nil && len(t.toolConfig.OutputSchema) > 0 {
			// For direct response tools, check if templateResult is valid JSON
			if json.Valid([]byte(result)) {
				structuredContent = json.RawMessage(result)
			}
		}

		// Send the result using structured content if available
		if structuredContent != nil {
			utils.SendMCPToolTextResultWithStructuredContent(ctx, result, structuredContent, fmt.Sprintf("mcp:tools/call:%s/%s:result", t.serverName, t.name))
		} else {
			utils.SendMCPToolTextResult(ctx, result, fmt.Sprintf("mcp:tools/call:%s/%s:result", t.serverName, t.name))
		}
		return nil
	}

	// Regular REST tool with HTTP request
	// Execute URL template
	urlStr, err := executeTemplate(t.toolConfig.parsedURLTemplate, templateDataBytes)
	if err != nil {
		return fmt.Errorf("error executing URL template: %v", err)
	}

	// Execute header templates from tool config
	headers := make([][2]string, 0, len(t.toolConfig.RequestTemplate.Headers))
	for i, header := range t.toolConfig.RequestTemplate.Headers {
		if header.Key == "" {
			log.Warnf("Skipping header with empty key at index %d", i)
			continue
		}
		tmpl, ok := t.toolConfig.parsedHeaderTemplates[header.Key]
		if !ok {
			return fmt.Errorf("header template not found for %s", header.Key)
		}
		value, err := executeTemplate(tmpl, templateDataBytes)
		if err != nil {
			return fmt.Errorf("error executing header template for %s: %v", header.Key, err)
		}
		headers = append(headers, [2]string{header.Key, value})
	}

	// Authorization or specific API key headers are handled by extractAndRemoveIncomingCredential if tool-level security is defined.
	// If no tool-level security is defined, this generic RemoveHttpRequestHeader("Authorization") acts as a fallback.
	// Unless passthroughAuthHeader is explicitly set to true.
	if t.toolConfig.Security.ID == "" {
		if !restServer.GetPassthroughAuthHeader() {
			proxywasm.RemoveHttpRequestHeader("Authorization") // Remove if not handled by specific scheme
		}
	}
	// General cleanup of Accept header from original client request.
	proxywasm.RemoveHttpRequestHeader("Accept")

	// After applySecurity, urlStr, headers, and parsedURL might have been modified.

	// Categorize arguments by position
	pathArgs := make(map[string]interface{})
	queryArgs := make(map[string]interface{})
	headerArgs := make(map[string]interface{})
	cookieArgs := make(map[string]interface{})
	bodyArgs := make(map[string]interface{})
	defaultArgs := make(map[string]interface{}) // Args without explicit position

	// Categorize arguments based on their position
	for name, value := range t.arguments {
		position, hasPosition := t.toolConfig.argPositions[name]
		if !hasPosition {
			defaultArgs[name] = value
			continue
		}

		switch position {
		case "path":
			pathArgs[name] = value
		case "query":
			queryArgs[name] = value
		case "header":
			headerArgs[name] = value
		case "cookie":
			cookieArgs[name] = value
		case "body":
			bodyArgs[name] = value
		default:
			// If position is invalid, treat as default
			defaultArgs[name] = value
		}
	}

	// Process path parameters
	for name, value := range pathArgs {
		placeholder := fmt.Sprintf("{%s}", name)
		// Path parameters are substituted directly into urlStr
		urlStr = strings.Replace(urlStr, placeholder, convertArgToString(value), -1)
	}

	// After path parameters are substituted, parse urlStr to create/update parsedURL.
	// This is the primary point where parsedURL is established before query manipulations.
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("error parsing URL after path param substitution: %v", err)
	}

	// Get existing query values
	query := parsedURL.Query()

	// Add query parameters from args
	for name, value := range queryArgs {
		query.Set(name, convertArgToString(value))
	}

	// Process URL parameters if argsToUrlParam is true (add defaultArgs to query)
	if t.toolConfig.RequestTemplate.ArgsToUrlParam {
		for name, value := range defaultArgs {
			query.Set(name, convertArgToString(value))
		}
	}

	// Update the URL with the new query string
	parsedURL.RawQuery = query.Encode()

	// Add header parameters from args
	for name, value := range headerArgs {
		headers = append(headers, [2]string{name, convertArgToString(value)})
	}

	// Add cookie parameters from args
	for name, value := range cookieArgs {
		cookie := fmt.Sprintf("%s=%s", name, convertArgToString(value))
		cookieHeaderFound := false
		for i, header := range headers {
			if strings.EqualFold(header[0], "Cookie") {
				headers[i][1] = header[1] + "; " + cookie
				cookieHeaderFound = true
				break
			}
		}
		if !cookieHeaderFound {
			headers = append(headers, [2]string{"Cookie", cookie})
		}
	}

	// Check for existing content types from tool config headers
	hasJsonContentType := hasContentType(headers, "application/json")
	hasFormContentType := hasContentType(headers, "application/x-www-form-urlencoded")

	// Prepare request body
	var requestBody []byte
	hasExplicitBody := t.toolConfig.parsedBodyTemplate != nil

	if hasExplicitBody {
		// If explicit body template is provided, use it
		body, err := executeTemplate(t.toolConfig.parsedBodyTemplate, templateDataBytes)
		if err != nil {
			return fmt.Errorf("error executing body template: %v", err)
		}
		requestBody = []byte(body)

		// Check if body is JSON and add content type if needed
		trimmedBody := bytes.TrimSpace(requestBody)
		if !hasJsonContentType && len(trimmedBody) > 0 &&
			((trimmedBody[0] == '{' && trimmedBody[len(trimmedBody)-1] == '}') ||
				(trimmedBody[0] == '[' && trimmedBody[len(trimmedBody)-1] == ']')) {
			// Try to parse as JSON to confirm
			var js interface{}
			if json.Unmarshal(trimmedBody, &js) == nil {
				headers = append(headers, [2]string{"Content-Type", "application/json; charset=utf-8"})
			}
		}
	} else if t.toolConfig.RequestTemplate.ArgsToJsonBody {
		// Combine body args and default args for JSON body
		combinedArgs := make(map[string]interface{})
		for k, v := range bodyArgs {
			combinedArgs[k] = v
		}
		for k, v := range defaultArgs {
			combinedArgs[k] = v
		}

		// Use args directly as JSON in the request body
		argsJson, err := json.Marshal(combinedArgs)
		if err != nil {
			return fmt.Errorf("error marshaling args to JSON: %v", err)
		}
		requestBody = argsJson

		// Add JSON content type if not already present
		if !hasJsonContentType {
			headers = append(headers, [2]string{"Content-Type", "application/json; charset=utf-8"})
		}
	} else if t.toolConfig.RequestTemplate.ArgsToFormBody {
		// Use args as form-urlencoded body
		formValues := url.Values{}
		for name, value := range bodyArgs {
			formValues.Set(name, convertArgToString(value))
		}
		for name, value := range defaultArgs {
			formValues.Set(name, convertArgToString(value))
		}

		requestBody = []byte(formValues.Encode())

		// Add form content type if not already present
		if !hasFormContentType {
			headers = append(headers, [2]string{"Content-Type", "application/x-www-form-urlencoded"})
		}
	} else if len(bodyArgs) > 0 {
		// If we have body args but no explicit body handling method,
		// check if there's already a form content type
		if hasFormContentType {
			// Format as form-urlencoded
			formValues := url.Values{}
			for name, value := range bodyArgs {
				formValues.Set(name, convertArgToString(value))
			}
			requestBody = []byte(formValues.Encode())
		} else {
			// Default to JSON
			argsJson, err := json.Marshal(bodyArgs)
			if err != nil {
				return fmt.Errorf("error marshaling body args to JSON: %v", err)
			}
			requestBody = argsJson

			// Add JSON content type if not already present
			if !hasJsonContentType {
				headers = append(headers, [2]string{"Content-Type", "application/json; charset=utf-8"})
			}
		}
	}

	// Ensure Accept header if not already set by tool config or args
	hasAcceptHeader := false
	for _, kv := range headers {
		if strings.EqualFold(kv[0], "accept") {
			hasAcceptHeader = true
			break
		}
	}
	if !hasAcceptHeader {
		headers = append(headers, [2]string{"Accept", "*/*"})
	}

	// Apply security scheme just before making the call, after all other modifications
	authReqCtx := AuthRequestContext{
		Method:                t.toolConfig.RequestTemplate.Method,
		Headers:               headers, // Pass the current headers slice
		ParsedURL:             parsedURL,
		RequestBody:           requestBody,
		PassthroughCredential: passthroughCredential,
	}
	if err := t.applySecurity(server, &authReqCtx); err != nil {
		// Log the error and continue, rather than failing the entire call.
		// The request will proceed without the intended security modifications if applySecurity failed.
		log.Errorf("Failed to apply security scheme for tool %s: %v. Request will proceed with potentially incomplete authentication.", t.name, err)
	}
	// After applySecurity, authReqCtx.Headers and authReqCtx.ParsedURL (RawQuery) might have been modified.
	// Update urlStr from the potentially modified ParsedURL.
	u := authReqCtx.ParsedURL
	encodedPath := u.EscapedPath()
	if u.Scheme != "" && u.Host != "" {
		urlStr = u.Scheme + "://" + u.Host + encodedPath
	} else {
		urlStr = "/" + strings.TrimPrefix(encodedPath, "/")
	}
	if u.RawQuery != "" {
		urlStr += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		urlStr += "#" + u.Fragment
	}
	// Make HTTP request using potentially modified headers from authReqCtx
	err = ctx.RouteCall(authReqCtx.Method, urlStr, authReqCtx.Headers, authReqCtx.RequestBody,
		func(statusCode int, responseHeaders [][2]string, responseBody []byte) {

			if statusCode >= 300 || statusCode < 200 {
				if t.toolConfig.parsedErrorResponseTemplate != nil {
					// Error response template is provided to customize the error response result.
					// Based on the responseBody, access the map-structured responseHeaders through _headers to reference their values within the errorResponseTemplate.
					// Usage examples in errorResponseTemplate:
					// - {{gjson "_headers.\\:status"}} -> Get HTTP status code
					// - {{gjson "_headers.x-ca-error-code"}} -> Get value of header key "x-ca-error-code"
					// - {{.data.value}} -> Access original responseBody content (e.g., JSON field "data.value")
					errorResponseTemplateDataBytes, _ := sjson.SetBytes(responseBody, "_headers", convertHeaders(responseHeaders))
					errorTemplateResult, err := executeTemplate(t.toolConfig.parsedErrorResponseTemplate, errorResponseTemplateDataBytes)
					if err != nil {
						utils.OnMCPToolCallError(ctx, fmt.Errorf("error executing error response template: %v", err))
						return
					}
					if errorTemplateResult != "" {
						utils.OnMCPToolCallError(ctx, fmt.Errorf("%s", errorTemplateResult))
						return
					}
				}
				utils.OnMCPToolCallError(ctx, fmt.Errorf("call failed, status: %d, response: %s", statusCode, responseBody))
				return
			}

			// Process response
			var result string

			headerMap := convertHeaders(responseHeaders)
			contentType := headerMap[strings.ToLower("Content-Type")]
			// Check if the response is an image
			if strings.HasPrefix(contentType, "image/") {
				// Handle image response by sending it as an MCP tool result
				utils.SendMCPToolImageResult(ctx, responseBody, contentType, fmt.Sprintf("mcp:tools/call:%s/%s:result", t.serverName, t.name))
				return
			}

			// Case 1: Full response template is provided
			if t.toolConfig.parsedResponseTemplate != nil {
				templateResult, err := executeTemplate(t.toolConfig.parsedResponseTemplate, responseBody)
				if err != nil {
					utils.OnMCPToolCallError(ctx, fmt.Errorf("error executing response template: %v", err))
					return
				}
				result = templateResult
			} else {
				// Case 2: No template, but prepend/append might be used
				rawResponse := string(responseBody)

				// Apply prepend/append if specified
				if t.toolConfig.ResponseTemplate.PrependBody != "" || t.toolConfig.ResponseTemplate.AppendBody != "" {
					result = t.toolConfig.ResponseTemplate.PrependBody + rawResponse + t.toolConfig.ResponseTemplate.AppendBody
				} else {
					// Case 3: No template and no prepend/append, just use raw response
					result = rawResponse
				}
			}
			if result == "" {
				result = "success"
			}

			// Check if tool has outputSchema and try to parse response as structured content
			var structuredContent json.RawMessage
			if t.toolConfig.OutputSchema != nil && len(t.toolConfig.OutputSchema) > 0 {
				// Try to parse response as JSON for structured content
				if json.Valid(responseBody) {
					structuredContent = json.RawMessage(responseBody)
				}
				// If not valid JSON, don't force structuredContent creation
				// Standard approach: use isError: true + error text (type: "text")
				// Only add structuredContent when there's a structured need for errors
			}

			// Send the result using structured content if available
			if structuredContent != nil {
				utils.SendMCPToolTextResultWithStructuredContent(ctx, result, structuredContent, fmt.Sprintf("mcp:tools/call:%s/%s:result", t.serverName, t.name))
			} else {
				utils.SendMCPToolTextResult(ctx, result, fmt.Sprintf("mcp:tools/call:%s/%s:result", t.serverName, t.name))
			}
		})
	if err != nil {
		utils.OnMCPToolCallError(ctx, errors.New("route failed"))
		log.Errorf("call api failed, err:%v", err)
	}
	return nil
}

// Description implements Tool interface
func (t *RestMCPTool) Description() string {
	return t.toolConfig.Description
}

// InputSchema implements Tool interface
func (t *RestMCPTool) InputSchema() map[string]any {
	// Convert tool args to JSON schema
	properties := make(map[string]interface{})
	required := []string{}

	for _, arg := range t.toolConfig.Args {
		argSchema := map[string]interface{}{
			"description": arg.Description,
		}

		// Set type (default to string if not specified)
		argType := arg.Type
		if argType == "" {
			argType = "string"
		}
		argSchema["type"] = argType

		// Add enum if specified
		if arg.Enum != nil && len(arg.Enum) > 0 {
			argSchema["enum"] = arg.Enum
		}

		// Add default if specified
		if arg.Default != nil {
			argSchema["default"] = arg.Default
		}

		// Add items for array type
		if argType == "array" && arg.Items != nil {
			argSchema["items"] = arg.Items
		}

		// Add properties for object type
		if argType == "object" && arg.Properties != nil {
			argSchema["properties"] = arg.Properties
		}

		properties[arg.Name] = argSchema

		// Add to required list if needed
		if arg.Required {
			required = append(required, arg.Name)
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}

	// Add required field only if there are required properties
	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// OutputSchema implements Tool interface (MCP Protocol Version 2025-06-18)
func (t *RestMCPTool) OutputSchema() map[string]any {
	return t.toolConfig.OutputSchema
}

func convertHeaders(responseHeaders [][2]string) map[string]string {
	headerMap := make(map[string]string)
	for _, h := range responseHeaders {
		if len(h) >= 2 {
			key := h[0]
			value := h[1]
			headerMap[key] = value
		}
	}
	return headerMap
}
