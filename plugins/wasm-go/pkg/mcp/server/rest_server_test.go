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
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tidwall/sjson"
)

func TestConvertArgToString(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "string value",
			input:    "test string",
			expected: "test string",
		},
		{
			name:     "boolean true",
			input:    true,
			expected: "true",
		},
		{
			name:     "boolean false",
			input:    false,
			expected: "false",
		},
		{
			name:     "integer",
			input:    42,
			expected: "42",
		},
		{
			name:     "float",
			input:    3.14,
			expected: "3.14",
		},
		{
			name:     "map",
			input:    map[string]interface{}{"key": "value"},
			expected: `{"key":"value"}`,
		},
		{
			name:     "array",
			input:    []interface{}{1, 2, 3},
			expected: "[1,2,3]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertArgToString(tt.input)
			if result != tt.expected {
				t.Errorf("convertArgToString(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestResponseTemplatePrependAppend(t *testing.T) {
	// Test response template with PrependBody and AppendBody
	sampleResponse := `{"result": "success", "data": {"name": "Test", "value": 42}}`

	tests := []struct {
		name        string
		template    RestToolResponseTemplate
		expected    []string
		notExpected []string
	}{
		{
			name: "with body template only",
			template: RestToolResponseTemplate{
				Body: "# Result\n- Name: {{.data.name}}\n- Value: {{.data.value}}",
			},
			expected: []string{
				"# Result",
				"- Name: Test",
				"- Value: 42",
			},
			notExpected: []string{
				"Field Descriptions:",
				"End of Response",
				`{"result": "success"`,
			},
		},
		{
			name: "with prepend only",
			template: RestToolResponseTemplate{
				PrependBody: "# Field Descriptions:\n- result: Operation result\n- data: Response data\n\n",
			},
			expected: []string{
				"# Field Descriptions:",
				"- result: Operation result",
				"- data: Response data",
				`{"result": "success"`,
				`"name": "Test"`,
			},
		},
		{
			name: "with append only",
			template: RestToolResponseTemplate{
				AppendBody: "\n\n*End of Response*",
			},
			expected: []string{
				`{"result": "success"`,
				`"name": "Test"`,
				"*End of Response*",
			},
		},
		{
			name: "with both prepend and append",
			template: RestToolResponseTemplate{
				PrependBody: "# API Response:\n\n",
				AppendBody:  "\n\n*This is raw JSON data with field 'name' = Test and 'value' = 42*",
			},
			expected: []string{
				"# API Response:",
				`{"result": "success"`,
				`"name": "Test"`,
				"*This is raw JSON data with field 'name' = Test and 'value' = 42*",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a tool with the test template
			// For tests with only prepend/append (no body), add a RequestTemplate.URL
			// to avoid direct response mode validation
			tool := RestTool{
				ResponseTemplate: tt.template,
			}
			if tt.template.Body == "" && (tt.template.PrependBody != "" || tt.template.AppendBody != "") {
				tool.RequestTemplate.URL = "http://example.com/api"
			}

			// Parse templates
			err := tool.parseTemplates()
			if err != nil {
				t.Fatalf("Failed to parse templates: %v", err)
			}

			// Simulate response processing
			var result string
			responseBody := []byte(sampleResponse)

			// Case 1: Full response template is provided
			if tool.parsedResponseTemplate != nil {
				templateResult, err := executeTemplate(tool.parsedResponseTemplate, responseBody)
				if err != nil {
					t.Fatalf("Failed to execute response template: %v", err)
				}
				result = templateResult
			} else {
				// Case 2: No template, but prepend/append might be used
				rawResponse := string(responseBody)

				// Apply prepend/append if specified
				if tool.ResponseTemplate.PrependBody != "" || tool.ResponseTemplate.AppendBody != "" {
					result = tool.ResponseTemplate.PrependBody + rawResponse + tool.ResponseTemplate.AppendBody
				} else {
					// Case 3: No template and no prepend/append, just use raw response
					result = rawResponse
				}
			}

			// Check that the result contains expected substrings
			for _, substr := range tt.expected {
				if !strings.Contains(result, substr) {
					t.Errorf("Expected substring not found: %s", substr)
				}
			}

			// Check that the result does not contain unexpected substrings
			for _, substr := range tt.notExpected {
				if strings.Contains(result, substr) {
					t.Errorf("Unexpected substring found: %s", substr)
				}
			}
		})
	}
}

func TestHasContentType(t *testing.T) {
	tests := []struct {
		name            string
		headers         [][2]string
		contentTypeStr  string
		expectedOutcome bool
	}{
		{
			name: "exact match",
			headers: [][2]string{
				{"Content-Type", "application/json"},
			},
			contentTypeStr:  "application/json",
			expectedOutcome: true,
		},
		{
			name: "case insensitive match",
			headers: [][2]string{
				{"content-type", "application/JSON"},
			},
			contentTypeStr:  "application/json",
			expectedOutcome: true,
		},
		{
			name: "substring match",
			headers: [][2]string{
				{"Content-Type", "application/json; charset=utf-8"},
			},
			contentTypeStr:  "application/json",
			expectedOutcome: true,
		},
		{
			name: "no match",
			headers: [][2]string{
				{"Content-Type", "text/plain"},
			},
			contentTypeStr:  "application/json",
			expectedOutcome: false,
		},
		{
			name: "header not present",
			headers: [][2]string{
				{"Accept", "application/json"},
			},
			contentTypeStr:  "application/json",
			expectedOutcome: false,
		},
		{
			name:            "empty headers",
			headers:         [][2]string{},
			contentTypeStr:  "application/json",
			expectedOutcome: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasContentType(tt.headers, tt.contentTypeStr)
			if result != tt.expectedOutcome {
				t.Errorf("hasContentType(%v, %v) = %v, want %v", tt.headers, tt.contentTypeStr, result, tt.expectedOutcome)
			}
		})
	}
}

func TestRestToolValidation(t *testing.T) {
	tests := []struct {
		name          string
		tool          RestTool
		expectedError bool
	}{
		{
			name: "valid tool with no args options",
			tool: RestTool{
				RequestTemplate: RestToolRequestTemplate{
					URL:    "https://example.com",
					Method: "GET",
				},
			},
			expectedError: false,
		},
		{
			name: "valid tool with argsToJsonBody",
			tool: RestTool{
				RequestTemplate: RestToolRequestTemplate{
					URL:            "https://example.com",
					Method:         "POST",
					ArgsToJsonBody: true,
				},
			},
			expectedError: false,
		},
		{
			name: "valid tool with argsToUrlParam",
			tool: RestTool{
				RequestTemplate: RestToolRequestTemplate{
					URL:            "https://example.com",
					Method:         "GET",
					ArgsToUrlParam: true,
				},
			},
			expectedError: false,
		},
		{
			name: "valid tool with argsToFormBody",
			tool: RestTool{
				RequestTemplate: RestToolRequestTemplate{
					URL:            "https://example.com",
					Method:         "POST",
					ArgsToFormBody: true,
				},
			},
			expectedError: false,
		},
		{
			name: "invalid tool with multiple args options",
			tool: RestTool{
				RequestTemplate: RestToolRequestTemplate{
					URL:            "https://example.com",
					Method:         "POST",
					ArgsToJsonBody: true,
					ArgsToFormBody: true,
				},
			},
			expectedError: true,
		},
		{
			name: "invalid tool with all args options",
			tool: RestTool{
				RequestTemplate: RestToolRequestTemplate{
					URL:            "https://example.com",
					Method:         "POST",
					ArgsToJsonBody: true,
					ArgsToUrlParam: true,
					ArgsToFormBody: true,
				},
			},
			expectedError: true,
		},
		{
			name: "invalid tool with both Body and PrependBody",
			tool: RestTool{
				RequestTemplate: RestToolRequestTemplate{
					URL:    "https://example.com",
					Method: "GET",
				},
				ResponseTemplate: RestToolResponseTemplate{
					Body:        "# Result\n{{.data}}",
					PrependBody: "# Field Descriptions:\n",
				},
			},
			expectedError: true,
		},
		{
			name: "invalid tool with both Body and AppendBody",
			tool: RestTool{
				RequestTemplate: RestToolRequestTemplate{
					URL:    "https://example.com",
					Method: "GET",
				},
				ResponseTemplate: RestToolResponseTemplate{
					Body:       "# Result\n{{.data}}",
					AppendBody: "\n*End of response*",
				},
			},
			expectedError: true,
		},
		{
			name: "invalid tool with Body, PrependBody, and AppendBody",
			tool: RestTool{
				RequestTemplate: RestToolRequestTemplate{
					URL:    "https://example.com",
					Method: "GET",
				},
				ResponseTemplate: RestToolResponseTemplate{
					Body:        "# Result\n{{.data}}",
					PrependBody: "# Field Descriptions:\n",
					AppendBody:  "\n*End of response*",
				},
			},
			expectedError: true,
		},
		{
			name: "valid tool with PrependBody and AppendBody but no Body",
			tool: RestTool{
				RequestTemplate: RestToolRequestTemplate{
					URL:    "https://example.com",
					Method: "GET",
				},
				ResponseTemplate: RestToolResponseTemplate{
					PrependBody: "# Field Descriptions:\n",
					AppendBody:  "\n*End of response*",
				},
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tool.parseTemplates()
			if (err != nil) != tt.expectedError {
				t.Errorf("parseTemplates() error = %v, expectedError %v", err, tt.expectedError)
			}
		})
	}
}

func TestInputSchemaWithComplexTypes(t *testing.T) {
	// Create a tool with array and object type arguments
	tool := RestMCPTool{
		toolConfig: RestTool{
			Args: []RestToolArg{
				{
					Name:        "stringArg",
					Description: "A string argument",
					Type:        "string",
				},
				{
					Name:        "arrayArg",
					Description: "An array argument",
					Type:        "array",
					Items: map[string]interface{}{
						"type": "string",
					},
				},
				{
					Name:        "objectArg",
					Description: "An object argument",
					Type:        "object",
					Properties: map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Name property",
						},
						"age": map[string]interface{}{
							"type":        "integer",
							"description": "Age property",
						},
					},
				},
				{
					Name:        "arrayOfObjects",
					Description: "An array of objects",
					Type:        "array",
					Items: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"id": map[string]interface{}{
								"type": "string",
							},
							"value": map[string]interface{}{
								"type": "number",
							},
						},
					},
				},
			},
		},
	}

	schema := tool.InputSchema()

	// Check schema structure
	if schema["type"] != "object" {
		t.Errorf("Expected schema type to be 'object', got %v", schema["type"])
	}

	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected properties to be a map, got %T", schema["properties"])
	}

	// Check individual property types
	checkProperty := func(name, expectedType string) {
		prop, ok := properties[name].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected property %s to be a map, got %T", name, properties[name])
		}
		if prop["type"] != expectedType {
			t.Errorf("Expected property %s type to be '%s', got %v", name, expectedType, prop["type"])
		}
	}

	checkProperty("stringArg", "string")
	checkProperty("arrayArg", "array")
	checkProperty("objectArg", "object")
	checkProperty("arrayOfObjects", "array")

	// Check array items
	arrayArg, _ := properties["arrayArg"].(map[string]interface{})
	if arrayArg["items"] == nil {
		t.Errorf("Expected arrayArg to have items property")
	}

	// Check object properties
	objectArg, _ := properties["objectArg"].(map[string]interface{})
	if objectArg["properties"] == nil {
		t.Errorf("Expected objectArg to have properties property")
	}

	// Check array of objects
	arrayOfObjects, _ := properties["arrayOfObjects"].(map[string]interface{})
	items, ok := arrayOfObjects["items"].(map[string]interface{})
	if !ok || items["type"] != "object" {
		t.Errorf("Expected arrayOfObjects items to be of type object")
	}
}

func TestArgsToUrlParamAndFormBody(t *testing.T) {
	// Test argsToUrlParam
	t.Run("argsToUrlParam", func(t *testing.T) {
		args := map[string]interface{}{
			"string": "value",
			"int":    42,
			"bool":   true,
			"array":  []interface{}{1, 2, 3},
			"object": map[string]interface{}{"key": "value"},
		}

		// Parse URL and add parameters
		baseURL := "https://example.com/api"
		parsedURL, _ := url.Parse(baseURL)
		query := parsedURL.Query()

		for key, value := range args {
			query.Set(key, convertArgToString(value))
		}

		parsedURL.RawQuery = query.Encode()
		result := parsedURL.String()

		// Verify each parameter is in the URL
		for key, value := range args {
			strValue := convertArgToString(value)
			encodedValue := url.QueryEscape(strValue)
			paramStr := key + "=" + encodedValue

			if !strings.Contains(result, paramStr) {
				t.Errorf("URL parameter missing: %s", paramStr)
			}
		}
	})

	// Test argsToFormBody
	t.Run("argsToFormBody", func(t *testing.T) {
		args := map[string]interface{}{
			"string": "value",
			"int":    42,
			"bool":   true,
			"array":  []interface{}{1, 2, 3},
			"object": map[string]interface{}{"key": "value"},
		}

		// Create form values
		formValues := url.Values{}
		for key, value := range args {
			formValues.Set(key, convertArgToString(value))
		}

		formBody := formValues.Encode()

		// Verify each parameter is in the form body
		for key, value := range args {
			strValue := convertArgToString(value)
			encodedValue := url.QueryEscape(strValue)
			paramStr := key + "=" + encodedValue

			if !strings.Contains(formBody, paramStr) {
				t.Errorf("Form body missing parameter: %s", paramStr)
			}
		}
	})
}

func TestRestToolConfig(t *testing.T) {
	// Example REST tool configuration
	configJSON := `
{
  "server": {
    "name": "rest-amap-server",
    "config": {
      "apiKey": "xxxxx"
    }
  },
  "tools": [
    {
      "name": "maps-geo",
      "description": "将详细的结构化地址转换为经纬度坐标。支持对地标性名胜景区、建筑物名称解析为经纬度坐标",
      "args": [
        {
          "name": "address",
          "description": "待解析的结构化地址信息",
          "type": "string",
          "required": true
        },
        {
          "name": "city",
          "description": "指定查询的城市",
          "required": false
        },
        {
          "name": "output",
          "description": "输出格式",
          "type": "string",
          "enum": ["json", "xml"],
          "default": "json"
        },
        {
          "name": "options",
          "description": "高级选项",
          "type": "object",
          "properties": {
            "extensions": {
              "type": "string",
              "enum": ["base", "all"]
            },
            "batch": {
              "type": "boolean"
            }
          }
        },
        {
          "name": "batch_addresses",
          "description": "批量地址",
          "type": "array",
          "items": {
            "type": "string"
          }
        }
      ],
      "requestTemplate": {
        "url": "https://restapi.amap.com/v3/geocode/geo?key={{.config.apiKey}}&address={{.args.address}}&city={{.args.city}}&output={{.args.output}}&source=ts_mcp",
        "method": "GET",
        "headers": [
          {
            "key": "Content-Type",
            "value": "application/json"
          }
        ]
      },
      "responseTemplate": {
        "body": "# 地理编码信息\n{{- range $index, $geo := .Geocodes }}\n## 地点 {{add $index 1}}\n\n- **国家**: {{ $geo.Country }}\n- **省份**: {{ $geo.Province }}\n- **城市**: {{ $geo.City }}\n- **城市代码**: {{ $geo.Citycode }}\n- **区/县**: {{ $geo.District }}\n- **街道**: {{ $geo.Street }}\n- **门牌号**: {{ $geo.Number }}\n- **行政编码**: {{ $geo.Adcode }}\n- **坐标**: {{ $geo.Location }}\n- **级别**: {{ $geo.Level }}\n{{- end }}"
      }
    }
  ]
}
`

	// Parse the config to verify it's valid JSON
	var configData map[string]interface{}
	err := json.Unmarshal([]byte(configJSON), &configData)
	if err != nil {
		t.Fatalf("Invalid JSON config: %v", err)
	}

	// Example tool configuration
	tool := RestTool{
		Name:        "maps-geo",
		Description: "将详细的结构化地址转换为经纬度坐标。支持对地标性名胜景区、建筑物名称解析为经纬度坐标",
		Args: []RestToolArg{
			{
				Name:        "address",
				Description: "待解析的结构化地址信息",
				Type:        "string",
				Required:    true,
			},
			{
				Name:        "city",
				Description: "指定查询的城市",
				Required:    false,
			},
			{
				Name:        "output",
				Description: "输出格式",
				Type:        "string",
				Enum:        []interface{}{"json", "xml"},
				Default:     "json",
			},
			{
				Name:        "options",
				Description: "高级选项",
				Type:        "object",
				Properties: map[string]interface{}{
					"extensions": map[string]interface{}{
						"type": "string",
						"enum": []interface{}{"base", "all"},
					},
					"batch": map[string]interface{}{
						"type": "boolean",
					},
				},
			},
			{
				Name:        "batch_addresses",
				Description: "批量地址",
				Type:        "array",
				Items: map[string]interface{}{
					"type": "string",
				},
			},
		},
		RequestTemplate: RestToolRequestTemplate{
			URL:    "https://restapi.amap.com/v3/geocode/geo?key={{.config.apiKey}}&address={{.args.address}}&city={{.args.city}}&output={{.args.output}}&source=ts_mcp",
			Method: "GET",
			Headers: []RestToolHeader{
				{
					Key:   "Content-Type",
					Value: "application/json",
				},
			},
		},
		ResponseTemplate: RestToolResponseTemplate{
			Body: `# 地理编码信息
{{- range $index, $geo := .Geocodes }}
## 地点 {{add $index 1}}

- **国家**: {{ $geo.Country }}
- **省份**: {{ $geo.Province }}
- **城市**: {{ $geo.City }}
- **城市代码**: {{ $geo.Citycode }}
- **区/县**: {{ $geo.District }}
- **街道**: {{ $geo.Street }}
- **门牌号**: {{ $geo.Number }}
- **行政编码**: {{ $geo.Adcode }}
- **坐标**: {{ $geo.Location }}
- **级别**: {{ $geo.Level }}
{{- end }}`,
		},
	}

	// Parse templates
	err = tool.parseTemplates()
	if err != nil {
		t.Fatalf("Failed to parse templates: %v", err)
	}

	var templateData []byte
	templateData, _ = sjson.SetBytes(templateData, "config", map[string]interface{}{"apiKey": "test-api-key"})
	templateData, _ = sjson.SetBytes(templateData, "args", map[string]interface{}{
		"address": "北京市朝阳区阜通东大街6号",
		"city":    "北京",
		"output":  "json",
	})

	// Test URL template
	url, err := executeTemplate(tool.parsedURLTemplate, templateData)
	if err != nil {
		t.Fatalf("Failed to execute URL template: %v", err)
	}

	expectedURL := "https://restapi.amap.com/v3/geocode/geo?key=test-api-key&address=北京市朝阳区阜通东大街6号&city=北京&output=json&source=ts_mcp"
	if url != expectedURL {
		t.Errorf("URL template rendering failed. Expected: %s, Got: %s", expectedURL, url)
	}

	// Test InputSchema for complex types
	mcpTool := &RestMCPTool{
		toolConfig: tool,
	}

	schema := mcpTool.InputSchema()
	properties := schema["properties"].(map[string]interface{})

	// Check object type
	options, ok := properties["options"].(map[string]interface{})
	if !ok || options["type"] != "object" {
		t.Errorf("Expected options to be of type object")
	}

	// Check array type
	batchAddresses, ok := properties["batch_addresses"].(map[string]interface{})
	if !ok || batchAddresses["type"] != "array" {
		t.Errorf("Expected batch_addresses to be of type array")
	}

	// Test response template with sample data
	sampleResponse := `
		{"Geocodes": [
			{
				"Country":  "中国",
				"Province": "北京市",
				"City":     "北京市",
				"Citycode": "010",
				"District": "朝阳区",
				"Street":   "阜通东大街",
				"Number":   "6号",
				"Adcode":   "110105",
				"Location": "116.483038,39.990633",
				"Level":    "门牌号",
			}]}`

	result, err := executeTemplate(tool.parsedResponseTemplate, []byte(sampleResponse))
	if err != nil {
		t.Fatalf("Failed to execute response template: %v", err)
	}

	// Just check that the result contains expected substrings
	expectedSubstrings := []string{
		"# 地理编码信息",
		"## 地点 1",
		"**国家**: 中国",
		"**省份**: 北京市",
		"**坐标**: 116.483038,39.990633",
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(result, substr) {
			t.Errorf("Response template rendering failed. Expected substring not found: %s", substr)
		}
	}
}

// TestRestServerDefaultSecurity tests the default security configuration for REST MCP server
func TestRestServerDefaultSecurity(t *testing.T) {
	server := NewRestMCPServer("test-rest-server")

	// Add security schemes
	defaultScheme := SecurityScheme{
		ID:                "DefaultAuth",
		Type:              "apiKey",
		In:                "header",
		Name:              "X-Default-Key",
		DefaultCredential: "default-key",
	}
	toolScheme := SecurityScheme{
		ID:                "ToolAuth",
		Type:              "apiKey",
		In:                "header",
		Name:              "X-Tool-Key",
		DefaultCredential: "tool-key",
	}
	server.AddSecurityScheme(defaultScheme)
	server.AddSecurityScheme(toolScheme)

	// Test setting default security directly on server
	server.SetDefaultDownstreamSecurity(SecurityRequirement{
		ID:          "DefaultAuth",
		Passthrough: false,
	})
	server.SetDefaultUpstreamSecurity(SecurityRequirement{
		ID: "DefaultAuth",
	})

	// Verify default security settings
	retrievedDownstream := server.GetDefaultDownstreamSecurity()
	assert.Equal(t, "DefaultAuth", retrievedDownstream.ID)
	assert.False(t, retrievedDownstream.Passthrough)

	retrievedUpstream := server.GetDefaultUpstreamSecurity()
	assert.Equal(t, "DefaultAuth", retrievedUpstream.ID)

	t.Logf("REST server default security configuration test completed successfully")
}

// TestRestServerSecurityFallback tests the fallback mechanism from tool-level to default security
func TestRestServerSecurityFallback(t *testing.T) {
	server := NewRestMCPServer("test-rest-server")

	// Add security schemes
	defaultScheme := SecurityScheme{
		ID:                "DefaultAuth",
		Type:              "apiKey",
		In:                "header",
		Name:              "X-Default-Key",
		DefaultCredential: "default-key",
	}
	toolScheme := SecurityScheme{
		ID:                "ToolAuth",
		Type:              "apiKey",
		In:                "header",
		Name:              "X-Tool-Key",
		DefaultCredential: "tool-key",
	}
	server.AddSecurityScheme(defaultScheme)
	server.AddSecurityScheme(toolScheme)

	// Test tool configuration with tool-level security (should use tool-level, not default)
	toolConfigWithSecurity := RestTool{
		Name:        "secure_tool",
		Description: "Tool with its own security",
		Security: SecurityRequirement{
			ID:          "ToolAuth",
			Passthrough: true,
		},
		RequestTemplate: RestToolRequestTemplate{
			URL:    "http://api.example.com/secure",
			Method: "GET",
			Security: SecurityRequirement{
				ID: "ToolAuth",
			},
		},
	}

	// Test tool configuration without tool-level security (should fallback to default)
	toolConfigWithoutSecurity := RestTool{
		Name:        "fallback_tool",
		Description: "Tool that falls back to default security",
		// No Security field configured, should use default
		RequestTemplate: RestToolRequestTemplate{
			URL:    "http://api.example.com/fallback",
			Method: "GET",
			// No Security field configured, should use default
		},
	}

	// Add tools to server
	err := server.AddRestTool(toolConfigWithSecurity)
	assert.NoError(t, err)
	err = server.AddRestTool(toolConfigWithoutSecurity)
	assert.NoError(t, err)

	// Verify tools were added
	tools := server.GetMCPTools()
	assert.Contains(t, tools, "secure_tool")
	assert.Contains(t, tools, "fallback_tool")

	t.Logf("REST server security fallback test completed successfully")
}

// ---------------------------------------------------------------------------
// parseIP
// ---------------------------------------------------------------------------

func TestParseIP(t *testing.T) {
	cases := []struct {
		name       string
		source     string
		fromHeader bool
		want       string
	}{
		{"ipv4 only", "10.0.0.1", false, "10.0.0.1"},
		{"ipv4 with port", "10.0.0.1:8080", false, "10.0.0.1"},
		{"ipv4 with leading whitespace", " 10.0.0.1:80", false, "10.0.0.1"},
		{"ipv4 X-Forwarded-For first hop", "10.0.0.1, 10.0.0.2, 10.0.0.3", true, "10.0.0.1"},
		{"ipv4 X-Forwarded-For with spaces", " 10.0.0.1 , 10.0.0.2 ", true, "10.0.0.1"},
		{"ipv6 bracketed with port", "[2001:db8::1]:443", false, "2001:db8::1"},
		{"ipv6 bracketed no port", "[2001:db8::1]", false, "2001:db8::1"},
		{"ipv6 bare passes through", "2001:db8::1", false, "2001:db8::1"},
		{"empty string", "", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseIP(c.source, c.fromHeader)
			assert.Equal(t, c.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// parseTemplates — fill remaining error branches
// ---------------------------------------------------------------------------

func TestParseTemplates_DirectResponseMissingBody(t *testing.T) {
	// No RequestTemplate.URL → direct-response mode. ResponseTemplate.Body must be set.
	tool := RestTool{}
	err := tool.parseTemplates()
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "direct response mode")
	}
}

func TestParseTemplates_DirectResponseWithBodyOk(t *testing.T) {
	tool := RestTool{
		ResponseTemplate: RestToolResponseTemplate{Body: "{{.}}"},
	}
	assert.NoError(t, tool.parseTemplates())
	assert.True(t, tool.isDirectResponseTool)
}

func TestParseTemplates_URLTemplateParseError(t *testing.T) {
	tool := RestTool{
		RequestTemplate: RestToolRequestTemplate{
			URL:    "http://x/{{ .unclosed ", // missing closing braces
			Method: "GET",
		},
	}
	err := tool.parseTemplates()
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "URL template")
	}
}

func TestParseTemplates_HeaderTemplateParseError(t *testing.T) {
	tool := RestTool{
		RequestTemplate: RestToolRequestTemplate{
			URL:    "http://x",
			Method: "GET",
			Headers: []RestToolHeader{
				{Key: "X-Bad", Value: "{{ .unclosed "},
			},
		},
	}
	err := tool.parseTemplates()
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "header template")
	}
}

func TestParseTemplates_BodyTemplateParseError(t *testing.T) {
	tool := RestTool{
		RequestTemplate: RestToolRequestTemplate{
			URL:    "http://x",
			Method: "POST",
			Body:   "{{ .unclosed ",
		},
	}
	err := tool.parseTemplates()
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "body template")
	}
}

func TestParseTemplates_ResponseTemplateParseError(t *testing.T) {
	tool := RestTool{
		RequestTemplate: RestToolRequestTemplate{URL: "http://x", Method: "GET"},
		ResponseTemplate: RestToolResponseTemplate{
			Body: "{{ .unclosed ",
		},
	}
	err := tool.parseTemplates()
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "response template")
	}
}

func TestParseTemplates_ErrorResponseTemplateParseError(t *testing.T) {
	tool := RestTool{
		RequestTemplate:       RestToolRequestTemplate{URL: "http://x", Method: "GET"},
		ErrorResponseTemplate: "{{ .unclosed ",
	}
	err := tool.parseTemplates()
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "error response template")
	}
}

func TestParseTemplates_HeaderWithEmptyKeySkipped(t *testing.T) {
	tool := RestTool{
		RequestTemplate: RestToolRequestTemplate{
			URL:    "http://x",
			Method: "GET",
			Headers: []RestToolHeader{
				{Key: "", Value: "ignored"},
				{Key: "X-Real", Value: "real"},
			},
		},
	}
	assert.NoError(t, tool.parseTemplates())
	_, hasReal := tool.parsedHeaderTemplates["X-Real"]
	assert.True(t, hasReal)
	_, hasEmpty := tool.parsedHeaderTemplates[""]
	assert.False(t, hasEmpty)
}

func TestParseTemplates_PopulatesArgPositions(t *testing.T) {
	tool := RestTool{
		RequestTemplate: RestToolRequestTemplate{URL: "http://x", Method: "GET"},
		ResponseTemplate: RestToolResponseTemplate{Body: "{{.}}"},
		Args: []RestToolArg{
			{Name: "q", Position: "QUERY"},  // lower-cased in argPositions
			{Name: "h", Position: "Header"},
			{Name: "noPos"},                   // no position → not stored
		},
	}
	require := assert.New(t)
	require.NoError(tool.parseTemplates())
	require.Equal("query", tool.argPositions["q"])
	require.Equal("header", tool.argPositions["h"])
	_, ok := tool.argPositions["noPos"]
	require.False(ok)
}

// ---------------------------------------------------------------------------
// executeTemplate — nil + execution error
// ---------------------------------------------------------------------------

func TestExecuteTemplate_NilReturnsError(t *testing.T) {
	_, err := executeTemplate(nil, []byte(`{}`))
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "nil")
	}
}

// ---------------------------------------------------------------------------
// RestMCPServer accessors — GetSecurityScheme, GetPassthroughAuthHeader,
// AddMCPTool, GetConfig, GetToolConfig
// ---------------------------------------------------------------------------

func TestRestServer_AddMCPTool_DelegatesToBase(t *testing.T) {
	s := NewRestMCPServer("rest")
	tool := &stubTool{desc: "x"}
	ret := s.AddMCPTool("plain", tool)
	assert.Same(t, s, ret)
	got, ok := s.GetMCPTools()["plain"]
	assert.True(t, ok)
	assert.Same(t, tool, got)
}

func TestRestServer_GetSecurityScheme_HitAndMiss(t *testing.T) {
	s := NewRestMCPServer("rest")
	scheme := SecurityScheme{ID: "K", Type: "apiKey", In: "header", Name: "X-K"}
	s.AddSecurityScheme(scheme)

	got, ok := s.GetSecurityScheme("K")
	assert.True(t, ok)
	assert.Equal(t, "K", got.ID)

	_, ok = s.GetSecurityScheme("missing")
	assert.False(t, ok)
}

func TestRestServer_PassthroughAuthHeader(t *testing.T) {
	s := NewRestMCPServer("rest")
	assert.False(t, s.GetPassthroughAuthHeader())
	s.SetPassthroughAuthHeader(true)
	assert.True(t, s.GetPassthroughAuthHeader())
}

func TestRestServer_GetToolConfig(t *testing.T) {
	s := NewRestMCPServer("rest")
	require.NoError(t, s.AddRestTool(RestTool{
		Name:             "t",
		ResponseTemplate: RestToolResponseTemplate{Body: "{{.}}"},
	}))
	cfg, ok := s.GetToolConfig("t")
	assert.True(t, ok)
	assert.Equal(t, "t", cfg.Name)

	_, ok = s.GetToolConfig("missing")
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// RestMCPServer.Clone — independence
// ---------------------------------------------------------------------------

func TestRestServer_Clone_Independence(t *testing.T) {
	orig := NewRestMCPServer("rest")
	orig.SetPassthroughAuthHeader(true)
	orig.SetConfig([]byte(`{"v":1}`))
	orig.AddSecurityScheme(SecurityScheme{ID: "K", Type: "apiKey", In: "header", Name: "X"})
	require.NoError(t, orig.AddRestTool(RestTool{
		Name:             "t",
		ResponseTemplate: RestToolResponseTemplate{Body: "{{.}}"},
	}))

	clonedI := orig.Clone()
	require.NotNil(t, clonedI)
	cloned, ok := clonedI.(*RestMCPServer)
	require.True(t, ok)

	// Mutate the original: cloned must not see the change.
	orig.AddSecurityScheme(SecurityScheme{ID: "K2", Type: "apiKey", In: "header", Name: "Y"})
	_, hasK2 := cloned.GetSecurityScheme("K2")
	assert.False(t, hasK2, "cloned server must not see security scheme added to original after Clone")

	// Tools map was deep-copied at Clone time.
	_, hasT := cloned.GetToolConfig("t")
	assert.True(t, hasT)
}

// ---------------------------------------------------------------------------
// RestMCPTool.Create — type coercion matrix
// ---------------------------------------------------------------------------

func newRestToolForCreate(t *testing.T) *RestMCPTool {
	t.Helper()
	tool := RestTool{
		Name: "t",
		Args: []RestToolArg{
			{Name: "b", Type: "boolean"},
			{Name: "i", Type: "integer"},
			{Name: "n", Type: "number"},
			{Name: "s", Type: "string"},
			{Name: "d", Type: "integer", Default: 7},
		},
		ResponseTemplate: RestToolResponseTemplate{Body: "{{.}}"},
	}
	require.NoError(t, tool.parseTemplates())
	return &RestMCPTool{
		serverName: "rest",
		name:       "t",
		toolConfig: tool,
	}
}

func TestRestMCPTool_Create_BooleanCoercion(t *testing.T) {
	tool := newRestToolForCreate(t)
	// Boolean from native true, native false, string "true", string "false",
	// string with garbage (passthrough), and other types (passthrough).
	cases := []struct {
		raw  any
		want any
	}{
		{true, true},
		{false, false},
		{"true", true},
		{"false", false},
		{"yes", "yes"},
		// JSON unmarshal turns any number into float64; non-bool/non-string
		// hits the default arm and is stored verbatim.
		{42, float64(42)},
	}
	for _, c := range cases {
		body, err := json.Marshal(map[string]any{"b": c.raw})
		require.NoError(t, err)
		created := tool.Create(body).(*RestMCPTool)
		assert.Equal(t, c.want, created.arguments["b"], "raw=%v", c.raw)
	}
}

func TestRestMCPTool_Create_IntegerCoercion(t *testing.T) {
	tool := newRestToolForCreate(t)
	cases := []struct {
		raw  any
		want any
	}{
		{float64(10), 10},
		{"42", 42},
		{"not-int", "not-int"},
		{true, true},
	}
	for _, c := range cases {
		body, err := json.Marshal(map[string]any{"i": c.raw})
		require.NoError(t, err)
		created := tool.Create(body).(*RestMCPTool)
		assert.Equal(t, c.want, created.arguments["i"], "raw=%v", c.raw)
	}
}

func TestRestMCPTool_Create_NumberCoercion(t *testing.T) {
	tool := newRestToolForCreate(t)
	cases := []struct {
		raw  any
		want any
	}{
		{"3.14", 3.14},
		{"abc", "abc"},
		{float64(2.5), 2.5}, // default: passthrough
	}
	for _, c := range cases {
		body, err := json.Marshal(map[string]any{"n": c.raw})
		require.NoError(t, err)
		created := tool.Create(body).(*RestMCPTool)
		assert.Equal(t, c.want, created.arguments["n"], "raw=%v", c.raw)
	}
}

func TestRestMCPTool_Create_DefaultApplied(t *testing.T) {
	tool := newRestToolForCreate(t)
	body := []byte(`{}`)
	created := tool.Create(body).(*RestMCPTool)
	assert.Equal(t, 7, created.arguments["d"])
	// Args without defaults are not present when omitted.
	_, hasI := created.arguments["i"]
	assert.False(t, hasI)
}

func TestRestMCPTool_Create_StringPassthrough(t *testing.T) {
	tool := newRestToolForCreate(t)
	body, _ := json.Marshal(map[string]any{"s": "hello"})
	created := tool.Create(body).(*RestMCPTool)
	assert.Equal(t, "hello", created.arguments["s"])
}

func TestRestMCPTool_Create_MalformedJSONStillProducesTool(t *testing.T) {
	tool := newRestToolForCreate(t)
	// Bad JSON is logged + ignored; defaults still applied.
	created := tool.Create([]byte("{not json")).(*RestMCPTool)
	assert.Equal(t, 7, created.arguments["d"], "default still applied when params unparseable")
}

// ---------------------------------------------------------------------------
// hasContentType — case + charset suffix
// ---------------------------------------------------------------------------

func TestHasContentType_CaseAndCharsetSuffix(t *testing.T) {
	headers := [][2]string{
		{"content-type", "Application/JSON; charset=utf-8"},
	}
	assert.True(t, hasContentType(headers, "application/json"))
	assert.True(t, hasContentType(headers, "json"))
	assert.False(t, hasContentType(headers, "xml"))

	emptyHeaders := [][2]string{}
	assert.False(t, hasContentType(emptyHeaders, "application/json"))
}

// pull in `require` for newer tests above without disturbing existing imports.
var _ = url.Parse
var _ = sjson.Set
var _ = strings.TrimSpace
