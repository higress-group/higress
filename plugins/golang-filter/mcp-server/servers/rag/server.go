package rag

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/rag/config"
	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/mark3labs/mcp-go/mcp"
)

const Version = "1.0.0"

type RAGConfig struct {
	config *config.Config
}

func init() {
	api.LogDebugf("RAG init")
	common.GlobalRegistry.RegisterServer("rag", &RAGConfig{
		config: &config.Config{
			RAG: config.RAGConfig{
				Splitter: config.SplitterConfig{
					Provider:     "recursive",
					ChunkSize:    500,
					ChunkOverlap: 50,
				},
				Threshold: 0.5,
				TopK:      10,
			},
			LLM: config.LLMConfig{
				Provider:    "",
				APIKey:      "",
				BaseURL:     "",
				Model:       "gpt-4o",
				Temperature: 0.5,
				MaxTokens:   2048,
			},
			Embedding: config.EmbeddingConfig{
				Provider:   "openai",
				APIKey:     "",
				BaseURL:    "",
				Model:      "text-embedding-ada-002",
				Dimensions: 1536,
			},
			VectorDB: config.VectorDBConfig{
				Provider:   "milvus",
				Host:       "localhost",
				Port:       19530,
				Database:   "default",
				Collection: "rag",
				Username:   "",
				Password:   "",
				Mapping: config.MappingConfig{
					Fields: []config.FieldMapping{
						{
							StandardName: "id",
							RawName:      "id",
							Properties: map[string]interface{}{
								"max_length": 256,
								"auto_id":    false,
							},
						},
						{
							StandardName: "content",
							RawName:      "content",
							Properties: map[string]interface{}{
								"max_length": 8192,
							},
						},
						{
							StandardName: "vector",
							RawName:      "vector",
							Properties:   make(map[string]interface{}),
						},
						{
							StandardName: "metadata",
							RawName:      "metadata",
							Properties:   make(map[string]interface{}),
						},
						{
							StandardName: "created_at",
							RawName:      "created_at",
							Properties:   make(map[string]interface{}),
						},
					},
					Index: config.IndexConfig{
						IndexType: "HNSW",
						Params:    map[string]interface{}{"M": 8, "efConstruction": 64},
					},
					Search: config.SearchConfig{
						MetricType: "IP",
						Params:     make(map[string]interface{}),
					},
				},
			},
		},
	})
}

func (c *RAGConfig) Clone() common.Server {
	if c.config == nil {
		return &RAGConfig{}
	}
	configBytes, err := json.Marshal(c.config)
	if err != nil {
		clonedConfig := *c.config
		return &RAGConfig{config: &clonedConfig}
	}
	var clonedConfig config.Config
	if err := json.Unmarshal(configBytes, &clonedConfig); err != nil {
		clonedConfig = *c.config
	}
	return &RAGConfig{config: &clonedConfig}
}

func (c *RAGConfig) ParseConfig(cfg map[string]any) error {
	api.LogDebugf("RAG start to parse config: %+v", cfg)
	// Parse RAG con
	api.LogDebugf("RAG parse rag config")
	if ragConfig, ok := cfg["rag"].(map[string]any); ok {
		if splitter, exists := ragConfig["splitter"].(map[string]any); exists {
			if splitterType, exists := splitter["provider"].(string); exists {
				c.config.RAG.Splitter.Provider = splitterType
			}
			if chunkSize, exists := splitter["chunk_size"].(float64); exists {
				c.config.RAG.Splitter.ChunkSize = int(chunkSize)
			}
			if chunkOverlap, exists := splitter["chunk_overlap"].(float64); exists {
				c.config.RAG.Splitter.ChunkOverlap = int(chunkOverlap)
			}
		}
		if threshold, exists := ragConfig["threshold"].(float64); exists {
			c.config.RAG.Threshold = threshold
		}
		if topK, exists := ragConfig["top_k"].(float64); exists {
			c.config.RAG.TopK = int(topK)
		}
	}

	// Parse Embedding configuration
	api.LogDebugf("RAG parse embedding config")
	if embeddingConfig, ok := cfg["embedding"].(map[string]any); ok {
		if provider, exists := embeddingConfig["provider"].(string); exists {
			c.config.Embedding.Provider = provider
		} else {
			return errors.New("missing embedding provider")
		}

		if apiKey, exists := embeddingConfig["api_key"].(string); exists {
			c.config.Embedding.APIKey = apiKey
		}
		if baseURL, exists := embeddingConfig["base_url"].(string); exists {
			c.config.Embedding.BaseURL = baseURL
		}
		if model, exists := embeddingConfig["model"].(string); exists {
			c.config.Embedding.Model = model
		}
		if dimensions, exists := embeddingConfig["dimensions"].(float64); exists {
			c.config.Embedding.Dimensions = int(dimensions)
		}
	}

	// Parse llm configuration
	api.LogDebugf("RAG parse llm config")
	if llmConfig, ok := cfg["llm"].(map[string]any); ok {
		if provider, exists := llmConfig["provider"].(string); exists {
			c.config.LLM.Provider = provider
		}
		if apiKey, exists := llmConfig["api_key"].(string); exists {
			c.config.LLM.APIKey = apiKey
		}
		if baseURL, exists := llmConfig["base_url"].(string); exists {
			c.config.LLM.BaseURL = baseURL
		}
		if model, exists := llmConfig["model"].(string); exists {
			c.config.LLM.Model = model
		}
		if temperature, exists := llmConfig["temperature"].(float64); exists {
			c.config.LLM.Temperature = temperature
		}
		if maxTokens, exists := llmConfig["max_tokens"].(float64); exists {
			c.config.LLM.MaxTokens = int(maxTokens)
		}
	}

	// Parse VectorDB configuration
	api.LogDebugf("RAG parse vectordb config")
	if vectordbConfig, ok := cfg["vectordb"].(map[string]any); ok {
		if provider, exists := vectordbConfig["provider"].(string); exists {
			c.config.VectorDB.Provider = provider
		} else {
			return errors.New("missing vectordb provider")
		}
		if host, exists := vectordbConfig["host"].(string); exists {
			c.config.VectorDB.Host = host
		}
		if port, exists := vectordbConfig["port"].(float64); exists {
			c.config.VectorDB.Port = int(port)
		}
		if dbName, exists := vectordbConfig["database"].(string); exists {
			c.config.VectorDB.Database = dbName
		}
		if collection, exists := vectordbConfig["collection"].(string); exists {
			c.config.VectorDB.Collection = collection
		}
		if username, exists := vectordbConfig["username"].(string); exists {
			c.config.VectorDB.Username = username
		}
		if password, exists := vectordbConfig["password"].(string); exists {
			c.config.VectorDB.Password = password
		}

		// Parse mapping here
		if mapping, exists := vectordbConfig["mapping"].(map[string]any); exists {
			// Parse field mappings
			if fields, ok := mapping["fields"].([]any); ok {
				c.config.VectorDB.Mapping.Fields = []config.FieldMapping{}
				for _, field := range fields {
					if fieldMap, ok := field.(map[string]any); ok {
						fieldMapping := config.FieldMapping{
							Properties: make(map[string]interface{}),
						}
						if standardName, ok := fieldMap["standard_name"].(string); ok {
							fieldMapping.StandardName = standardName
						}

						if rawName, ok := fieldMap["raw_name"].(string); ok {
							fieldMapping.RawName = rawName
						}
						// Parse properties
						if properties, ok := fieldMap["properties"].(map[string]any); ok {
							for key, value := range properties {
								fieldMapping.Properties[key] = value
							}
						}
						c.config.VectorDB.Mapping.Fields = append(c.config.VectorDB.Mapping.Fields, fieldMapping)
					}
				}
			}

			// Parse index configuration
			if index, ok := mapping["index"].(map[string]any); ok {
				if indexType, ok := index["index_type"].(string); ok {
					c.config.VectorDB.Mapping.Index.IndexType = indexType
				}

				// Parse index parameters
				if params, ok := index["params"].(map[string]any); ok {
					c.config.VectorDB.Mapping.Index.Params = params
				}
			}

			// Parse search configuration
			if search, ok := mapping["search"].(map[string]any); ok {
				if metricType, ok := search["metric_type"].(string); ok {
					c.config.VectorDB.Mapping.Search.MetricType = metricType
				}
				// Parse search parameters
				if params, ok := search["params"].(map[string]any); ok {
					c.config.VectorDB.Mapping.Search.Params = params
				}
			}
		}
	}

	api.LogDebugf("RAG parse config successful with config:%+v", c.config)
	return nil
}

func (c *RAGConfig) NewServer(serverName string) (*common.MCPServer, error) {
	api.LogDebugf("RAG NewServer: %s", serverName)
	mcpServer := common.NewMCPServer(
		serverName,
		Version,
		common.WithInstructions("This is a RAG (Retrieval-Augmented Generation) server for knowledge management and intelligent Q&A"),
	)

	// Initialize RAG client with configuration
	api.LogDebugf("RAG NewRAGClient: %+v", c.config)
	ragClient, err := NewRAGClient(c.config)
	if err != nil {
		return nil, fmt.Errorf("create rag client failed, err: %w", err)
	}

	api.LogDebugf("RAG start add tool")
	// Knowledge Base Management Tools
	mcpServer.AddTool(
		mcp.NewToolWithRawSchema("create-chunks-from-text", "Process and segment input text into semantic chunks for knowledge base ingestion", GetCreateChunkFromTextSchema()),
		HandleCreateChunkFromText(ragClient),
	)

	// Chunk Management Tools
	mcpServer.AddTool(
		mcp.NewToolWithRawSchema("list-chunks", "Retrieve and display all knowledge chunks in the database", GetListChunksSchema()),
		HandleListChunks(ragClient),
	)
	mcpServer.AddTool(
		mcp.NewToolWithRawSchema("delete-chunk", "Remove a specific knowledge chunk from the database using its unique identifier", GetDeleteChunkSchema()),
		HandleDeleteChunk(ragClient),
	)

	// Semantic Search Tool
	mcpServer.AddTool(
		mcp.NewToolWithRawSchema("search-chunks", "Perform semantic search across knowledge chunks using natural language query", GetSearchSchema()),
		HandleSearch(ragClient),
	)

	// Intelligent Q&A Tool
	mcpServer.AddTool(
		mcp.NewToolWithRawSchema("chat", "Answer user questions by retrieving relevant knowledge from the database and generating responses using RAG-enhanced LLM", GetChatSchema()),
		HandleChat(ragClient),
	)
	api.LogDebugf("RAG NewServer successful")
	return mcpServer, nil
}
