package tool_search

import (
	"errors"
	"fmt"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	Version = "1.0.0"

	// 默认配置值
	defaultTableName  = "apig_mcp_tools"
	defaultBaseURL    = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	defaultModel      = "text-embedding-v4"
	defaultDimensions = 1024
	// 写死最大工具数量为1000，仅用于单测
	fixedMaxTools = 1000
)

func init() {
	common.GlobalRegistry.RegisterServer("tool-search", &ToolSearchConfig{})
}

type VectorConfig struct {
	Type         string  `json:"type"`
	VectorWeight float64 `json:"vectorWeight"`
	TableName    string  `json:"tableName"`
	Host         string  `json:"host"`
	Port         int     `json:"port"`
	Database     string  `json:"database"`
	Username     string  `json:"username"`
	Password     string  `json:"password"`
}

type EmbeddingConfig struct {
	APIKey     string `json:"apiKey"`
	BaseURL    string `json:"baseURL"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
}

type ToolSearchConfig struct {
	Vector      VectorConfig    `json:"vector"`
	Embedding   EmbeddingConfig `json:"embedding"`
	description string
}

func (c *ToolSearchConfig) Clone() common.Server {
	cloned := *c
	return &cloned
}

func (c *ToolSearchConfig) ParseConfig(config map[string]any) error {
	// Parse vector configuration
	vectorConfig, ok := config["vector"].(map[string]any)
	if !ok {
		return errors.New("missing vector configuration")
	}

	if err := c.parseVectorConfig(vectorConfig); err != nil {
		return fmt.Errorf("failed to parse vector config: %w", err)
	}

	// Parse embedding configuration
	embeddingConfig, ok := config["embedding"].(map[string]any)
	if !ok {
		return errors.New("missing embedding configuration")
	}

	if err := c.parseEmbeddingConfig(embeddingConfig); err != nil {
		return fmt.Errorf("failed to parse embedding config: %w", err)
	}

	// Optional description
	if description, ok := config["description"].(string); ok {
		c.description = description
	} else {
		c.description = "Tool search server for semantic similarity search"
	}

	api.LogDebugf("ToolSearchConfig ParseConfig: %+v", config)
	return nil
}

func (c *ToolSearchConfig) parseVectorConfig(config map[string]any) error {
	if vectorType, ok := config["type"].(string); ok {
		c.Vector.Type = vectorType
	} else {
		return errors.New("missing vector.type")
	}

	if c.Vector.Type != "milvus" {
		return fmt.Errorf("unsupported vector.type: %s, only 'milvus' is supported", c.Vector.Type)
	}

	if host, ok := config["host"].(string); ok {
		c.Vector.Host = host
	} else {
		return errors.New("missing vector.host")
	}

	if port, ok := config["port"].(float64); ok {
		c.Vector.Port = int(port)
	} else if port, ok := config["port"].(int); ok {
		c.Vector.Port = port
	} else {
		return errors.New("missing vector.port")
	}

	if database, ok := config["database"].(string); ok {
		c.Vector.Database = database
	} else {
		c.Vector.Database = "default" // 默认数据库
	}

	if tableName, ok := config["tableName"].(string); ok {
		c.Vector.TableName = tableName
	} else {
		c.Vector.TableName = defaultTableName
	}

	if username, ok := config["username"].(string); ok {
		c.Vector.Username = username
	}

	if password, ok := config["password"].(string); ok {
		c.Vector.Password = password
	}

	// 移除maxTools的解析逻辑

	return nil
}

func (c *ToolSearchConfig) parseEmbeddingConfig(config map[string]any) error {
	// Parse API key (required)
	if apiKey, ok := config["apiKey"].(string); ok {
		c.Embedding.APIKey = apiKey
	} else {
		return errors.New("missing embedding.apiKey")
	}

	// Parse optional fields with defaults
	if baseURL, ok := config["baseURL"].(string); ok {
		c.Embedding.BaseURL = baseURL
	} else {
		c.Embedding.BaseURL = defaultBaseURL
	}

	if model, ok := config["model"].(string); ok {
		c.Embedding.Model = model
	} else {
		c.Embedding.Model = defaultModel
	}

	if dimensions, ok := config["dimensions"].(float64); ok {
		c.Embedding.Dimensions = int(dimensions)
	} else if dimensions, ok := config["dimensions"].(int); ok {
		c.Embedding.Dimensions = dimensions
	} else {
		c.Embedding.Dimensions = defaultDimensions
	}

	return nil
}

func (c *ToolSearchConfig) NewServer(serverName string) (*common.MCPServer, error) {
	mcpServer := common.NewMCPServer(
		serverName,
		Version,
		common.WithInstructions(c.description),
	)

	// Create embedding client
	embeddingClient := NewEmbeddingClient(c.Embedding.APIKey, c.Embedding.BaseURL, c.Embedding.Model, c.Embedding.Dimensions)

	// Create search service，使用写死的fixedMaxTools值
	searchService := NewSearchService(
		c.Vector.Host,
		c.Vector.Port,
		c.Vector.Database,
		c.Vector.Username,
		c.Vector.Password,
		c.Vector.TableName,
		embeddingClient,
		c.Embedding.Dimensions,
		fixedMaxTools, // 使用写死的值
	)

	// Add tool search tool
	mcpServer.AddTool(
		mcp.NewToolWithRawSchema("x_higress_tool_search", "Higress MCP Tools Searcher", GetToolSearchSchema()),
		HandleToolSearch(searchService),
	)

	return mcpServer, nil
}
