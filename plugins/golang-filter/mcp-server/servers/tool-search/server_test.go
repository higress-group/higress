package tool_search

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/mark3labs/mcp-go/mcp"
)

// Mock implementation of CommonCAPI for testing
type mockCommonCAPI struct {
	logs []string
}

func (m *mockCommonCAPI) Log(level api.LogType, message string) {
	fmt.Printf("[%s] %s\n", level, message)
	m.logs = append(m.logs, message)
}

func (m *mockCommonCAPI) LogLevel() api.LogType {
	return api.Debug
}

// TestServer is used for local functional testing
func TestServer(t *testing.T) {
	// Setup mock API for logging
	mockAPI := &mockCommonCAPI{}
	api.SetCommonCAPI(mockAPI)

	// Load configuration from environment variables or use defaults
	config := map[string]any{
		"vector": map[string]any{
			"type":         "milvus",
			"vectorWeight": 0.6,
			"tableName":    getEnvOrDefault("TEST_TABLE_NAME", "apig_mcp_tools"),
			"host":         getEnvOrDefault("TEST_MILVUS_HOST", "localhost"),
			"port":         getEnvOrDefaultInt("TEST_MILVUS_PORT", 19530),
			"database":     getEnvOrDefault("TEST_MILVUS_DATABASE", "default"),
			"username":     getEnvOrDefault("TEST_MILVUS_USERNAME", "root"),
			"password":     getEnvOrDefault("TEST_MILVUS_PASSWORD", "Milvus"),
			"maxTools":     getEnvOrDefaultInt("TEST_MAX_TOOLS", 1000),
		},
		"embedding": map[string]any{
			"apiKey":     getEnvOrDefault("TEST_API_KEY", "your-dashscope-api-key"),
			"baseURL":    getEnvOrDefault("TEST_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
			"model":      getEnvOrDefault("TEST_MODEL", "text-embedding-v4"),
			"dimensions": 1024,
		},
		"description": "Test MCP Tools Search Server",
	}

	// Create configuration instance
	toolSearchConfig := &ToolSearchConfig{}
	if err := toolSearchConfig.ParseConfig(config); err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	// Create MCP Server
	_, err := toolSearchConfig.NewServer("test-tool-search")
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test database connection
	vectorConfig := config["vector"].(map[string]any)
	embeddingConfig := config["embedding"].(map[string]any)

	// Test GetAllTools
	t.Logf("\n=== Testing GetAllTools ===")
	embeddingClient := NewEmbeddingClient(
		embeddingConfig["apiKey"].(string),
		embeddingConfig["baseURL"].(string),
		embeddingConfig["model"].(string),
		embeddingConfig["dimensions"].(int),
	)

	searchService, err := NewSearchService(
		toolSearchConfig.Vector,
		embeddingClient,
		embeddingConfig["dimensions"].(int),
		getEnvOrDefaultInt("TEST_MAX_TOOLS", 1000),
	)
	if err != nil {
		t.Fatalf("Failed to create search service: %v", err)
	}

	allTools, err := searchService.GetAllTools()
	if err != nil {
		t.Logf("GetAllTools failed: %v", err)
	} else {
		t.Logf("Found %d tools:", len(allTools.Tools))
		for i, tool := range allTools.Tools {
			if i < 3 { // Show only first 3 tools
				toolJSON, _ := json.MarshalIndent(tool, "", "  ")
				t.Logf("Tool %d: %s", i+1, string(toolJSON))
			}
		}
		if len(allTools.Tools) > 3 {
			t.Logf("... and %d more tools", len(allTools.Tools)-3)
		}
	}

	// Test tool search with timing
	t.Logf("\n=== Testing Tool Search ===")
	testQueries := []string{
		"weather data",
		"database query",
		"file operations",
		"HTTP requests",
		"library documents",
	}

	for _, query := range testQueries {
		t.Logf("\n--- Testing query: '%s' ---", query)

		// Create MCP tool call request
		request := mcp.CallToolRequest{
			Params: struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments,omitempty"`
				Meta      *struct {
					ProgressToken mcp.ProgressToken `json:"progressToken,omitempty"`
				} `json:"_meta,omitempty"`
			}{
				Name: "x_higress_tool_search",
				Arguments: map[string]interface{}{
					"query": query,
					"topK":  3,
				},
			},
		}

		// Get tool handler
		handler := HandleToolSearch(searchService)

		// Execute search with timing
		start := time.Now()
		result, err := handler(context.Background(), request)
		duration := time.Since(start)

		if err != nil {
			t.Logf("Search failed: %v", err)
			continue
		}

		// Print results with timing information
		t.Logf("Search completed in %v", duration)
		if len(result.Content) > 0 {
			if textContent, ok := result.Content[0].(mcp.TextContent); ok {
				var toolsResult map[string]interface{}
				if err := json.Unmarshal([]byte(textContent.Text), &toolsResult); err == nil {
					toolsJSON, _ := json.MarshalIndent(toolsResult, "", "  ")
					t.Logf("Tools Result: %s", string(toolsJSON))
				} else {
					t.Logf("Text Content: %s", textContent.Text)
				}
			}
		}
	}

	// Test configuration validation
	t.Logf("\n=== Configuration Validation ===")
	t.Logf("Host: %s", vectorConfig["host"])
	t.Logf("Port: %d", vectorConfig["port"])
	t.Logf("Database: %s", vectorConfig["database"])
	t.Logf("Table Name: %s", vectorConfig["tableName"])
	t.Logf("Vector Weight: %f", vectorConfig["vectorWeight"])
	t.Logf("Text Weight: %f", 1.0-vectorConfig["vectorWeight"].(float64))
	t.Logf("Model: %s", embeddingConfig["model"])
	t.Logf("Dimensions: %d", embeddingConfig["dimensions"])
	t.Logf("API Base URL: %s", embeddingConfig["baseURL"])

	t.Logf("\n=== Test completed ===")
}

// Helper function to get environment variable or default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvOrDefaultInt(key string, defaultValue int) int {
	if valueStr := os.Getenv(key); valueStr != "" {
		if value, err := fmt.Sscanf(valueStr, "%d", &defaultValue); err == nil && value == 1 {
			return defaultValue
		}
	}
	return defaultValue
}
