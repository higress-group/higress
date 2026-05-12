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

package validator

import (
	"encoding/json"
	"fmt"
)

// ExampleUsage demonstrates how to use the ValidateConfig function
func ExampleUsage() {
	// Example 1: REST server configuration
	restServerConfig := `{
		"server": {
			"name": "weather-api",
			"config": {
				"apiKey": "your-api-key"
			}
		},
		"tools": [
			{
				"name": "get_weather",
				"description": "Get current weather for a city",
				"args": [
					{
						"name": "city",
						"description": "City name",
						"type": "string",
						"required": true
					},
					{
						"name": "units",
						"description": "Temperature units",
						"type": "string",
						"enum": ["celsius", "fahrenheit"],
						"default": "celsius"
					}
				],
				"requestTemplate": {
					"url": "https://api.weather.com/v1/current?city={{.args.city}}&units={{.args.units}}",
					"method": "GET",
					"headers": [
						{
							"key": "Authorization",
							"value": "Bearer {{.config.apiKey}}"
						}
					]
				},
				"responseTemplate": {
					"body": "Current weather in {{.args.city}}: {{.temperature}}°{{.args.units}}"
				}
			}
		],
		"allowTools": ["get_weather"]
	}`

	result, err := ValidateConfig(restServerConfig)
	if err != nil {
		fmt.Printf("Error validating REST server config: %v\n", err)
		return
	}

	fmt.Printf("REST Server Config Validation:\n")
	fmt.Printf("  Valid: %t\n", result.IsValid)
	fmt.Printf("  Server Name: %s\n", result.ServerName)
	fmt.Printf("  Is Composed: %t\n", result.IsComposed)
	if result.Error != nil {
		fmt.Printf("  Error: %v\n", result.Error)
	}
	fmt.Println()

	// Example 2: ToolSet configuration
	toolSetConfig := `{
		"toolSet": {
			"name": "ai-assistant-tools",
			"serverTools": [
				{
					"serverName": "weather-api",
					"tools": ["get_weather", "get_forecast"]
				},
				{
					"serverName": "search-api",
					"tools": ["web_search", "image_search"]
				}
			]
		},
		"allowTools": ["weather-api/get_weather", "search-api/web_search"]
	}`

	result, err = ValidateConfig(toolSetConfig)
	if err != nil {
		fmt.Printf("Error validating toolSet config: %v\n", err)
		return
	}

	fmt.Printf("ToolSet Config Validation:\n")
	fmt.Printf("  Valid: %t\n", result.IsValid)
	fmt.Printf("  Server Name: %s\n", result.ServerName)
	fmt.Printf("  Is Composed: %t\n", result.IsComposed)
	if result.Error != nil {
		fmt.Printf("  Error: %v\n", result.Error)
	}
	fmt.Println()

	// Example 3: Pre-registered Go-based server (validation skipped)
	goServerConfig := `{
		"server": {
			"name": "custom-go-server",
			"config": {
				"database_url": "postgres://localhost:5432/mydb",
				"max_connections": 10
			}
		},
		"allowTools": ["query_database", "update_record"]
	}`

	result, err = ValidateConfig(goServerConfig)
	if err != nil {
		fmt.Printf("Error validating Go server config: %v\n", err)
		return
	}

	fmt.Printf("Go Server Config Validation (skipped):\n")
	fmt.Printf("  Valid: %t\n", result.IsValid)
	fmt.Printf("  Server Name: %s\n", result.ServerName)
	fmt.Printf("  Is Composed: %t\n", result.IsComposed)
	if result.Error != nil {
		fmt.Printf("  Error: %v\n", result.Error)
	}
	fmt.Println()

	// Example 4: Invalid configuration
	invalidConfig := `{
		"server": {
			"config": {}
		}
	}`

	result, err = ValidateConfig(invalidConfig)
	if err != nil {
		fmt.Printf("Error validating invalid config: %v\n", err)
		return
	}

	fmt.Printf("Invalid Config Validation:\n")
	fmt.Printf("  Valid: %t\n", result.IsValid)
	if result.Error != nil {
		fmt.Printf("  Error: %v\n", result.Error)
	}
}

// ValidateConfigFromBytes validates configuration from byte array
func ValidateConfigFromBytes(configBytes []byte) (*ValidationResult, error) {
	return ValidateConfig(string(configBytes))
}

// ValidateConfigFromMap validates configuration from a map
func ValidateConfigFromMap(configMap map[string]interface{}) (*ValidationResult, error) {
	configBytes, err := json.Marshal(configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config map: %v", err)
	}
	return ValidateConfig(string(configBytes))
}

// ExampleYAMLUsage demonstrates how to use the ValidateConfigYAML function
func ExampleYAMLUsage() {
	// Example YAML configuration for REST server
	yamlConfig := `
server:
  name: weather-api-yaml
  config:
    apiKey: your-api-key
tools:
  - name: get_weather
    description: Get current weather for a city
    args:
      - name: city
        description: City name
        type: string
        required: true
      - name: units
        description: Temperature units
        type: string
        enum: ["celsius", "fahrenheit"]
        default: celsius
    requestTemplate:
      url: "https://api.weather.com/v1/current?city={{.args.city}}&units={{.args.units}}"
      method: GET
      headers:
        - key: Authorization
          value: "Bearer {{.config.apiKey}}"
    responseTemplate:
      body: "Current weather in {{.args.city}}: {{.temperature}}°{{.args.units}}"
allowTools: ["get_weather"]
`

	result, err := ValidateConfigYAML(yamlConfig)
	if err != nil {
		fmt.Printf("Error validating YAML config: %v\n", err)
		return
	}

	fmt.Printf("YAML Config Validation:\n")
	fmt.Printf("  Valid: %t\n", result.IsValid)
	fmt.Printf("  Server Name: %s\n", result.ServerName)
	fmt.Printf("  Is Composed: %t\n", result.IsComposed)
	if result.Error != nil {
		fmt.Printf("  Error: %v\n", result.Error)
	}
	fmt.Println()

	// Example YAML configuration for ToolSet
	yamlToolSetConfig := `
toolSet:
  name: ai-assistant-tools-yaml
  serverTools:
    - serverName: weather-api
      tools: ["get_weather", "get_forecast"]
    - serverName: search-api
      tools: ["web_search", "image_search"]
allowTools: ["weather-api/get_weather", "search-api/web_search"]
`

	result, err = ValidateConfigYAML(yamlToolSetConfig)
	if err != nil {
		fmt.Printf("Error validating YAML toolSet config: %v\n", err)
		return
	}

	fmt.Printf("YAML ToolSet Config Validation:\n")
	fmt.Printf("  Valid: %t\n", result.IsValid)
	fmt.Printf("  Server Name: %s\n", result.ServerName)
	fmt.Printf("  Is Composed: %t\n", result.IsComposed)
	if result.Error != nil {
		fmt.Printf("  Error: %v\n", result.Error)
	}
}
