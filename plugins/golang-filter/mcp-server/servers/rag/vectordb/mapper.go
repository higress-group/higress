package vectordb

import (
	"errors"
	"fmt"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/rag/config"
)

// Error definitions
var (
	ErrFieldNotFound        = errors.New("field not found")
	ErrInvalidFieldType     = errors.New("invalid field type")
	ErrInvalidIndexType     = errors.New("invalid index type")
	ErrInvalidMetricType    = errors.New("invalid metric type")
	ErrInvalidSearchParams  = errors.New("invalid search parameters")
	ErrCollectionNotFound   = errors.New("collection not found")
	ErrUnsupportedOperation = errors.New("unsupported operation")
)

// VectorDBMapper interface for vector database mapping
type VectorDBMapper interface {
	// ParseMapping parses the mapping configuration
	ParseMapping(provider string, cfg config.MappingConfig) error

	// GetIndexConfig returns the index configuration
	GetIndexConfig() (config.IndexConfig, error)

	// GetSearchConfig returns the search configuration
	GetSearchConfig() (config.SearchConfig, error)

	// Get all raw field names
	GetRawAllFieldNames() ([]string, error)

	// GetIDField returns the ID field mapping
	GetIDField() (*config.FieldMapping, error)

	// GetVectorField returns the vector field mapping
	GetVectorField() (*config.FieldMapping, error)

	// Get raw field name by standard field name
	GetRawField(standardFieldName string) (*config.FieldMapping, error)

	// Get field mapping by raw field name
	GetField(rawFieldName string) (*config.FieldMapping, error)

	// Get all field mappings
	GetFieldMappings() ([]config.FieldMapping, error)
}

// DefaultVectorDBMapper is the default implementation of VectorDBMapper interface
type DefaultVectorDBMapper struct {
	// Mapping configuration
	mappingConfig config.MappingConfig
	// Map from standard field name to field mapping
	standardFieldMap map[string]*config.FieldMapping
	// Map from raw field name to field mapping
	rawFieldMap map[string]*config.FieldMapping
}

// NewDefaultVectorDBMapper creates a new default vector database mapper
func NewDefaultVectorDBMapper(provider string, mappingConfig config.MappingConfig) (*DefaultVectorDBMapper, error) {
	mapper := &DefaultVectorDBMapper{
		standardFieldMap: make(map[string]*config.FieldMapping),
		rawFieldMap:      make(map[string]*config.FieldMapping),
	}
	if err := mapper.ParseMapping(provider, mappingConfig); err != nil {
		return nil, err
	}
	return mapper, nil
}

// ParseMapping parses the mapping configuration
func (m *DefaultVectorDBMapper) ParseMapping(provider string, cfg config.MappingConfig) error {
	m.mappingConfig = cfg
	// Clear existing mappings
	m.standardFieldMap = make(map[string]*config.FieldMapping)
	m.rawFieldMap = make(map[string]*config.FieldMapping)
	// fill default field mappings
	if len(cfg.Fields) == 0 {
		defaultFields := []config.FieldMapping{
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
			},
			{
				StandardName: "metadata",
				RawName:      "metadata",
			},
			{
				StandardName: "created_at",
				RawName:      "created_at",
			},
		}
		cfg.Fields = defaultFields
	}
	m.mappingConfig.Fields = cfg.Fields

	// Parse field mappings
	for i, field := range m.mappingConfig.Fields {
		fieldPtr := &m.mappingConfig.Fields[i]
		m.standardFieldMap[field.StandardName] = fieldPtr
		m.rawFieldMap[field.RawName] = fieldPtr
	}

	// Check fields, must include id, content, vector fields
	requiredFields := []string{"id", "content", "vector"}
	for _, fieldName := range requiredFields {
		if _, err := m.GetRawField(fieldName); err != nil {
			return fmt.Errorf("[vector db mapper] required field %s not found or not varchar type", fieldName)
		}
	}

	return nil
}

// GetIndexConfig gets the index configuration
func (m *DefaultVectorDBMapper) GetIndexConfig() (config.IndexConfig, error) {
	return m.mappingConfig.Index, nil
}

// GetSearchConfig gets the search configuration
func (m *DefaultVectorDBMapper) GetSearchConfig() (config.SearchConfig, error) {
	return m.mappingConfig.Search, nil
}

// GetRawAllFieldNames gets all raw field names
func (m *DefaultVectorDBMapper) GetRawAllFieldNames() ([]string, error) {
	fieldNames := make([]string, 0, len(m.rawFieldMap))
	for name := range m.rawFieldMap {
		fieldNames = append(fieldNames, name)
	}
	return fieldNames, nil
}

// GetIDField gets the ID field
func (m *DefaultVectorDBMapper) GetIDField() (*config.FieldMapping, error) {
	return m.GetRawField("id")
}

// GetVectorField gets the vector field
func (m *DefaultVectorDBMapper) GetVectorField() (*config.FieldMapping, error) {
	return m.GetRawField("vector")
}

// GetRawField gets the raw field mapping by standard field name
func (m *DefaultVectorDBMapper) GetRawField(standardFieldName string) (*config.FieldMapping, error) {
	field, exists := m.standardFieldMap[standardFieldName]
	if !exists {
		return nil, fmt.Errorf("%w: standard field %s not found", ErrFieldNotFound, standardFieldName)
	}
	return field, nil
}

// GetField gets the field mapping by raw field name
func (m *DefaultVectorDBMapper) GetField(rawFieldName string) (*config.FieldMapping, error) {
	field, exists := m.rawFieldMap[rawFieldName]
	if !exists {
		return nil, fmt.Errorf("%w: raw field %s not found", ErrFieldNotFound, rawFieldName)
	}
	return field, nil
}

// GetFieldMappings gets all field mappings
func (m *DefaultVectorDBMapper) GetFieldMappings() ([]config.FieldMapping, error) {
	return m.mappingConfig.Fields, nil
}
