package config

import "fmt"

// Config represents the main configuration structure for the MCP server
type Config struct {
	RAG       RAGConfig       `json:"rag" yaml:"rag"`
	LLM       LLMConfig       `json:"llm" yaml:"llm"`
	Embedding EmbeddingConfig `json:"embedding" yaml:"embedding"`
	VectorDB  VectorDBConfig  `json:"vectordb" yaml:"vectordb"`
}

// RAGConfig contains basic configuration for the RAG system
type RAGConfig struct {
	Splitter  SplitterConfig `json:"splitter" yaml:"splitter"`
	Threshold float64        `json:"threshold,omitempty" yaml:"threshold,omitempty"`
	TopK      int            `json:"top_k,omitempty" yaml:"top_k,omitempty"`
}

// SplitterConfig defines document splitter configuration
type SplitterConfig struct {
	Provider     string `json:"provider" yaml:"provider"` // Available options: recursive, character, token
	ChunkSize    int    `json:"chunk_size,omitempty" yaml:"chunk_size,omitempty"`
	ChunkOverlap int    `json:"chunk_overlap,omitempty" yaml:"chunk_overlap,omitempty"`
}

// LLMConfig defines configuration for Large Language Models
type LLMConfig struct {
	Provider    string  `json:"provider" yaml:"provider"` // Available options: openai, dashscope, qwen
	APIKey      string  `json:"api_key,omitempty" yaml:"api_key"`
	BaseURL     string  `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	Model       string  `json:"model" yaml:"model"`
	Temperature float64 `json:"temperature,omitempty" yaml:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
}

// EmbeddingConfig defines configuration for embedding models
type EmbeddingConfig struct {
	Provider   string `json:"provider" yaml:"provider"` // Available options: openai, dashscope
	APIKey     string `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	BaseURL    string `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	Model      string `json:"model,omitempty" yaml:"model,omitempty"`
	Dimensions int    `json:"dimensions,omitempty" yaml:"dimension,omitempty"`
}

// VectorDBConfig defines configuration for vector databases
type VectorDBConfig struct {
	Provider   string        `json:"provider" yaml:"provider"` // Available options: milvus, qdrant, chroma
	Host       string        `json:"host,omitempty" yaml:"host,omitempty"`
	Port       int           `json:"port,omitempty" yaml:"port,omitempty"`
	Database   string        `json:"database,omitempty" yaml:"database,omitempty"`
	Collection string        `json:"collection,omitempty" yaml:"collection,omitempty"`
	Username   string        `json:"username,omitempty" yaml:"username,omitempty"`
	Password   string        `json:"password,omitempty" yaml:"password,omitempty"`
	Mapping    MappingConfig `json:"mapping,omitempty" yaml:"mapping,omitempty"`
}

// MappingConfig defines field mapping configuration for vector databases
type MappingConfig struct {
	Fields []FieldMapping `json:"fields,omitempty" yaml:"fields,omitempty"`
	Index  IndexConfig    `json:"index,omitempty" yaml:"index,omitempty"`
	Search SearchConfig   `json:"search,omitempty" yaml:"search,omitempty"`
}

// // CollectionMapping defines field mapping for collection
// type CollectionMapping struct {
// 	Fields []FieldMapping `json:"fields,omitempty" yaml:"fields,omitempty"`
// }

// FieldMapping defines mapping for a single field
type FieldMapping struct {
	StandardName string                 `json:"standard_name" yaml:"standard_name"`
	RawName      string                 `json:"raw_name" yaml:"raw_name"`
	Properties   map[string]interface{} `json:"properties,omitempty" yaml:"properties,omitempty"`
}

func (f FieldMapping) IsPrimaryKey() bool {
	return f.StandardName == "id"
}

func (f FieldMapping) IsAutoID() bool {
	if f.Properties == nil {
		return false
	}
	autoID, ok := f.Properties["auto_id"].(bool)
	if !ok {
		return false
	}
	return autoID
}

func (f FieldMapping) IsVectorField() bool {
	return f.StandardName == "vector"
}

func (f FieldMapping) MaxLength() int {
	if f.Properties == nil {
		return 0
	}
	// Configuration values come from JSON/structpb, where numbers are float64;
	// accept plain ints as well for in-process construction.
	if maxLength, ok := f.Properties["max_length"].(int); ok {
		return maxLength
	}
	if maxLength, ok := f.Properties["max_length"].(float64); ok {
		return int(maxLength)
	}
	return 256
}

// IndexConfig defines configuration for index parameters
type IndexConfig struct {
	// Index type, e.g., IVF_FLAT, IVF_SQ8, HNSW, etc.
	IndexType string `json:"index_type" yaml:"index_type"`
	// Index parameter configuration
	Params map[string]interface{} `json:"params" yaml:"params"`
}

func (i IndexConfig) ParamsString(key string) (string, error) {
	if mVal, ok := i.Params[key].(string); ok {
		return mVal, nil
	}
	return "", fmt.Errorf("params %s not found", key)
}

func (i IndexConfig) ParamsInt64(key string) (int64, error) {
	if mVal, ok := i.Params[key].(int64); ok {
		return mVal, nil
	}
	if mVal, ok := i.Params[key].(int); ok {
		return int64(mVal), nil
	}
	// JSON/structpb decode numbers as float64; this is the type the parameter
	// actually arrives as when the config is parsed from Envoy's TypedStruct.
	if mVal, ok := i.Params[key].(float64); ok {
		return int64(mVal), nil
	}
	return 0, fmt.Errorf("params %s not found", key)
}

func (i IndexConfig) ParamsFloat64(key string) (float64, error) {
	if mVal, ok := i.Params[key].(float64); ok {
		return mVal, nil
	}
	if mVal, ok := i.Params[key].(float32); ok {
		return float64(mVal), nil
	}
	return 0, fmt.Errorf("params %s not found", key)
}

func (i IndexConfig) ParamsBool(key string) (bool, error) {
	if mVal, ok := i.Params[key].(bool); ok {
		return mVal, nil
	}
	return false, fmt.Errorf("params %s not found", key)
}

// SearchConfig defines configuration for search parameters
type SearchConfig struct {
	// Metric type, e.g., L2, IP, etc.
	MetricType string `json:"metric_type,omitempty" yaml:"metric_type,omitempty"`
	// Search parameter configuration
	Params map[string]interface{} `json:"params" yaml:"params"`
}

func (i SearchConfig) ParamsString(key string) (string, error) {
	if mVal, ok := i.Params[key].(string); ok {
		return mVal, nil
	}
	return "", fmt.Errorf("params %s not found", key)
}

func (i SearchConfig) ParamsInt64(key string) (int64, error) {
	if mVal, ok := i.Params[key].(int64); ok {
		return mVal, nil
	}
	// JSON/structpb decode numbers as float64; this is the type the parameter
	// actually arrives as when the config is parsed from Envoy's TypedStruct.
	if mVal, ok := i.Params[key].(float64); ok {
		return int64(mVal), nil
	}
	return 0, fmt.Errorf("params %s not found", key)
}

func (i SearchConfig) ParamsFloat64(key string) (float64, error) {
	if mVal, ok := i.Params[key].(float64); ok {
		return mVal, nil
	}
	return 0, fmt.Errorf("params %s not found", key)
}

func (i SearchConfig) ParamsBool(key string) (bool, error) {
	if mVal, ok := i.Params[key].(bool); ok {
		return mVal, nil
	}
	return false, fmt.Errorf("params %s not found", key)
}
