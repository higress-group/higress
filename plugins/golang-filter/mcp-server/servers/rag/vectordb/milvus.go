package vectordb

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/rag/config"
	"github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/rag/schema"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

const (
	MILVUS_DUMMY_DIM     = 8
	MILVUS_PROVIDER_TYPE = "milvus"
)

// MilvusProviderInitializer initializes the Milvus vector store provider
type milvusProviderInitializer struct{}

// InitConfig initializes the configuration with default values if not set
func (m *milvusProviderInitializer) InitConfig(cfg *config.VectorDBConfig) error {
	if cfg.Provider != MILVUS_PROVIDER_TYPE {
		return fmt.Errorf("provider type mismatch: expected %s, got %s", MILVUS_PROVIDER_TYPE, cfg.Provider)
	}

	// Set default values
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 19530
	}
	if cfg.Database == "" {
		cfg.Database = "default"
	}

	if cfg.Collection == "" {
		cfg.Collection = schema.DEFAULT_DOCUMENT_COLLECTION
	}

	return nil
}

// ValidateConfig validates the configuration parameters
func (m *milvusProviderInitializer) ValidateConfig(cfg *config.VectorDBConfig) error {
	if cfg.Host == "" {
		return fmt.Errorf("milvus host is required")
	}
	if cfg.Port <= 0 {
		return fmt.Errorf("milvus port must be positive")
	}

	if cfg.Database == "" {
		return fmt.Errorf("milvus database is required")
	}

	if cfg.Collection == "" {
		return fmt.Errorf("milvus document collection is required")
	}
	return nil
}

// CreateProvider creates a new Milvus vector store provider instance
func (m *milvusProviderInitializer) CreateProvider(cfg *config.VectorDBConfig, dim int) (VectorStoreProvider, error) {
	if err := m.InitConfig(cfg); err != nil {
		return nil, err
	}
	if err := m.ValidateConfig(cfg); err != nil {
		return nil, err
	}
	provider, err := NewMilvusProvider(cfg, dim)
	return provider, err
}

// MilvusProvider implements the vector store provider interface for Milvus
type MilvusProvider struct {
	client     client.Client
	config     *config.VectorDBConfig
	collection string
	mapper     VectorDBMapper
	dimensions int
}

// NewMilvusProvider creates a new instance of MilvusProvider
func NewMilvusProvider(cfg *config.VectorDBConfig, dimensions int) (VectorStoreProvider, error) {
	// Create Milvus client
	connectParam := client.Config{
		Address: fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
	}
	connectParam.DBName = cfg.Database
	// Add authentication if credentials are provided
	if cfg.Username != "" && cfg.Password != "" {
		connectParam.Username = cfg.Username
		connectParam.Password = cfg.Password
	}

	milvusClient, err := client.NewClient(context.Background(), connectParam)
	if err != nil {
		return nil, fmt.Errorf("failed to create milvus client: %w", err)
	}

	mapper, err := NewDefaultVectorDBMapper(MILVUS_PROVIDER_TYPE, cfg.Mapping)
	if err != nil {
		return nil, fmt.Errorf("failed to create default vector db mapper: %w", err)
	}

	provider := &MilvusProvider{
		client:     milvusClient,
		config:     cfg,
		collection: cfg.Collection,
		mapper:     mapper,
		dimensions: dimensions,
	}
	ctx := context.Background()
	if err := provider.CreateCollection(ctx, dimensions); err != nil {
		return nil, err
	}
	return provider, nil
}

func (m *MilvusProvider) buildSchema() (*entity.Schema, error) {
	// Create Milvus collection Schema
	idField, _ := m.mapper.GetIDField()
	isIDAuto := idField.IsAutoID()
	schema := entity.NewSchema().
		WithName(m.collection).
		WithDescription("Knowledge document collection").
		WithAutoID(isIDAuto).
		WithDynamicFieldEnabled(false)
	// Add fields
	var fieldEntity *entity.Field
	fieldMappings, _ := m.mapper.GetFieldMappings()
	for _, field := range fieldMappings {
		fieldEntity = nil
		maxLength := field.MaxLength()
		switch field.StandardName {
		case "id":
			isIDAuto := field.IsAutoID()
			fieldEntity = entity.NewField().
				WithName(field.RawName).
				WithDataType(entity.FieldTypeVarChar).
				WithMaxLength(int64(maxLength)).
				WithIsPrimaryKey(true)
			if isIDAuto {
				fieldEntity.WithIsAutoID(true)
			}
			schema.WithField(fieldEntity)
		case "content":
			fieldEntity = entity.NewField().
				WithName(field.RawName).
				WithDataType(entity.FieldTypeVarChar).
				WithMaxLength(int64(maxLength))
			schema.WithField(fieldEntity)
		case "vector":
			fieldEntity = entity.NewField().
				WithName(field.RawName).
				WithDataType(entity.FieldTypeFloatVector).
				WithDim(int64(m.dimensions))
			schema.WithField(fieldEntity)
		case "metadata":
			fieldEntity = entity.NewField().
				WithName(field.RawName).
				WithDataType(entity.FieldTypeJSON)
			schema.WithField(fieldEntity)
		case "created_at":
			fieldEntity = entity.NewField().
				WithName(field.RawName).
				WithDataType(entity.FieldTypeInt64)
			schema.WithField(fieldEntity)
		}
	}
	return schema, nil
}

func (m *MilvusProvider) GetMetricType(metricType string) entity.MetricType {
	switch strings.ToUpper(metricType) {
	case "L2":
		return entity.L2
	case "IP":
		return entity.IP
	case "COSINE":
		return entity.COSINE
	case "HAMMING":
		return entity.HAMMING
	case "JACCARD":
		return entity.JACCARD
	case "TANIMOTO":
		return entity.TANIMOTO
	case "SUBSTRUCTURE":
		return entity.SUBSTRUCTURE
	case "SUPERSTRUCTURE":
		return entity.SUPERSTRUCTURE
	default:
		return entity.IP
	}
}

func (m *MilvusProvider) buildVectorIndex() (entity.Index, error) {
	// Map index type
	indexConfig, _ := m.mapper.GetIndexConfig()
	searchConfig, _ := m.mapper.GetSearchConfig()
	// Map index parameters
	milvusIndexType := strings.ToUpper(indexConfig.IndexType)
	if milvusIndexType == "" {
		milvusIndexType = "HNSW"
	}
	metricType := m.GetMetricType(searchConfig.MetricType)
	switch milvusIndexType {
	case "FLAT":
		// FLAT index doesn't need additional parameters
		index, err := entity.NewIndexFlat(metricType)
		if err != nil {
			return nil, fmt.Errorf("failed to create FLAT index: %w", err)
		}
		return index, nil

	case "BIN_FLAT":
		// BIN_FLAT index doesn't need additional parameters
		nlist := 128
		if nlistVal, err := indexConfig.ParamsInt64("nlist"); err == nil {
			nlist = int(nlistVal)
		}
		index, err := entity.NewIndexBinFlat(metricType, nlist)
		if err != nil {
			return nil, fmt.Errorf("failed to create BIN_FLAT index: %w", err)
		}
		return index, nil

	case "IVF_FLAT":
		// Default parameters
		nlist := 128
		if nlistVal, err := indexConfig.ParamsInt64("nlist"); err == nil {
			nlist = int(nlistVal)
		}
		index, err := entity.NewIndexIvfFlat(metricType, nlist)
		if err != nil {
			return nil, fmt.Errorf("failed to create IVF_FLAT index: %w", err)
		}
		return index, nil

	case "BIN_IVF_FLAT":
		// Default parameters
		nlist := 128
		if nlistVal, err := indexConfig.ParamsInt64("nlist"); err == nil {
			nlist = int(nlistVal)
		}
		index, err := entity.NewIndexBinIvfFlat(metricType, nlist)
		if err != nil {
			return nil, fmt.Errorf("failed to create BIN_IVF_FLAT index: %w", err)
		}
		return index, nil

	case "IVF_SQ8":
		// Default parameters
		nlist := 128
		if nlistVal, err := indexConfig.ParamsInt64("nlist"); err == nil {
			nlist = int(nlistVal)
		}
		index, err := entity.NewIndexIvfSQ8(metricType, nlist)
		if err != nil {
			return nil, fmt.Errorf("failed to create IVF_SQ8 index: %w", err)
		}
		return index, nil

	case "IVF_PQ":
		// Default parameters
		nlist := 128
		m := 4
		nbits := 8

		if nlistVal, err := indexConfig.ParamsInt64("nlist"); err == nil {
			nlist = int(nlistVal)
		}
		if mVal, err := indexConfig.ParamsFloat64("m"); err == nil {
			m = int(mVal)
		}
		if nbitsVal, err := indexConfig.ParamsInt64("nbits"); err == nil {
			nbits = int(nbitsVal)
		}

		index, err := entity.NewIndexIvfPQ(metricType, nlist, m, nbits)
		if err != nil {
			return nil, fmt.Errorf("failed to create IVF_PQ index: %w", err)
		}
		return index, nil

	case "HNSW":
		// Default parameters
		m := 8
		efConstruction := 64
		if mVal, err := indexConfig.ParamsInt64("M"); err == nil {
			m = int(mVal)
		}
		if efConstructionVal, err := indexConfig.ParamsInt64("efConstruction"); err == nil {
			efConstruction = int(efConstructionVal)
		}
		index, err := entity.NewIndexHNSW(metricType, m, efConstruction)
		if err != nil {
			return nil, fmt.Errorf("failed to create HNSW index: %w", err)
		}
		return index, nil

	case "IVF_HNSW":
		// Default parameters
		nlist := 128
		m := 8
		efConstruction := 64

		if nlistVal, err := indexConfig.ParamsInt64("nlist"); err == nil {
			nlist = int(nlistVal)
		}
		if mVal, err := indexConfig.ParamsInt64("M"); err == nil {
			m = int(mVal)
		}

		if efConstructionVal, err := indexConfig.ParamsInt64("efConstruction"); err == nil {
			efConstruction = int(efConstructionVal)
		}

		index, err := entity.NewIndexIvfHNSW(metricType, nlist, m, efConstruction)
		if err != nil {
			return nil, fmt.Errorf("failed to create IVF_HNSW index: %w", err)
		}
		return index, nil

	case "DISKANN":
		// DISKANN index parameters
		index, err := entity.NewIndexDISKANN(metricType)
		if err != nil {
			return nil, fmt.Errorf("failed to create DISKANN index: %w", err)
		}
		return index, nil

	case "SCANN":
		// SCANN index parameters
		nlist := 128
		with_raw_data := false
		if nlistVal, err := indexConfig.ParamsInt64("nlist"); err == nil {
			nlist = int(nlistVal)
		}
		if with_raw_dataVal, err := indexConfig.ParamsBool("with_raw_data"); err == nil {
			with_raw_data = with_raw_dataVal
		}
		index, err := entity.NewIndexSCANN(metricType, nlist, with_raw_data)
		if err != nil {
			return nil, fmt.Errorf("failed to create SCANN index: %w", err)
		}
		return index, nil

	case "AUTOINDEX":
		// Auto index
		index, err := entity.NewIndexAUTOINDEX(metricType)
		if err != nil {
			return nil, fmt.Errorf("failed to create AUTOINDEX index: %w", err)
		}
		return index, nil

	default:
		return nil, fmt.Errorf("unsupported index type: %s", milvusIndexType)
	}
}

// CreateCollection creates a new collection with the specified dimension
func (m *MilvusProvider) CreateCollection(ctx context.Context, dim int) error {
	// Check if collection exists
	document_exists, err := m.client.HasCollection(ctx, m.collection)
	if err != nil {
		return fmt.Errorf("failed to check %s collection existence: %w", m.collection, err)
	}

	if !document_exists {
		fmt.Printf("create collection %s\n", m.collection)
		// Create schema
		schema, err := m.buildSchema()
		if err != nil {
			return fmt.Errorf("failed to build schema: %w", err)
		}
		// Create collection
		err = m.client.CreateCollection(ctx, schema, entity.DefaultShardNumber)
		if err != nil {
			return fmt.Errorf("failed to create collection: %w", err)
		}
		// Create vector index
		vectorIndex, err := m.buildVectorIndex()
		vectorField, _ := m.mapper.GetVectorField()
		if err != nil {
			return fmt.Errorf("failed to create vector index: %w", err)
		}

		err = m.client.CreateIndex(ctx, m.collection, vectorField.RawName, vectorIndex, false, client.WithIndexName("vector_index"))
		if err != nil {
			return fmt.Errorf("failed to create vector index: %w", err)
		}
	}
	// Load collection
	err = m.client.LoadCollection(ctx, m.collection, false)
	if err != nil {
		return fmt.Errorf("failed to load document collection: %w", err)
	}
	return nil
}

// DropCollection removes the collection from the database
func (m *MilvusProvider) DropCollection(ctx context.Context) error {
	// Check if collection exists
	exists, err := m.client.HasCollection(ctx, m.collection)
	if err != nil {
		return fmt.Errorf("failed to check %s collection existence: %w", m.collection, err)
	}
	if !exists {
		return fmt.Errorf("collection %s does not exist", m.collection)
	}
	// Drop collection
	err = m.client.DropCollection(ctx, m.collection)
	if err != nil {
		return fmt.Errorf("failed to drop collection: %w", err)
	}
	return nil
}

// AddDoc adds documents to the vector database
func (m *MilvusProvider) AddDoc(ctx context.Context, docs []schema.Document) error {
	if len(docs) == 0 {
		return nil
	}

	// Get field mappings
	fieldMappings, err := m.mapper.GetFieldMappings()
	if err != nil {
		return fmt.Errorf("failed to get field mappings: %w", err)
	}
	// Prepare data and columns
	columns := make([]entity.Column, 0, len(fieldMappings))
	// Create corresponding column data for each field
	for _, field := range fieldMappings {
		// Skip ID field if configured as auto ID
		if field.IsPrimaryKey() && field.IsAutoID() {
			continue
		}
		switch field.StandardName {
		case "id":
			// Handle string type fields
			values := make([]string, len(docs))
			for i, doc := range docs {
				values[i] = doc.ID
			}
			columns = append(columns, entity.NewColumnVarChar(field.RawName, values))
		case "content":
			values := make([]string, len(docs))
			for i, doc := range docs {
				values[i] = doc.Content
			}
			columns = append(columns, entity.NewColumnVarChar(field.RawName, values))

		case "vector":
			// Handle vector fields
			vectors := make([][]float32, len(docs))
			for i, doc := range docs {
				vectors[i] = doc.Vector
			}
			columns = append(columns, entity.NewColumnFloatVector(field.RawName, len(vectors[0]), vectors))
		case "metadata":
			// Handle JSON type fields (like metadata)
			values := make([][]byte, len(docs))
			for i, doc := range docs {
				// Serialize metadata
				metadataBytes, err := json.Marshal(doc.Metadata)
				if err != nil {
					return fmt.Errorf("failed to marshal metadata for doc %s: %w", doc.ID, err)
				}
				values[i] = metadataBytes
			}
			columns = append(columns, entity.NewColumnJSONBytes(field.RawName, values))
		case "created_at":
			// Handle integer type fields
			values := make([]int64, len(docs))
			for i, doc := range docs {
				values[i] = doc.CreatedAt.UnixMilli()
			}
			columns = append(columns, entity.NewColumnInt64(field.RawName, values))
		}
	}
	// Insert data
	_, err = m.client.Insert(ctx, m.collection, "", columns...)
	if err != nil {
		return fmt.Errorf("failed to insert documents: %w", err)
	}

	// Flush data
	err = m.client.Flush(ctx, m.collection, false)
	if err != nil {
		return fmt.Errorf("failed to flush collection: %w", err)
	}

	return nil
}

// DeleteDoc deletes a document by its ID
func (m *MilvusProvider) DeleteDoc(ctx context.Context, id string) error {
	// Get ID field
	idField, _ := m.mapper.GetIDField()
	// Build delete expression using the RawName of ID field
	expr := fmt.Sprintf(`%s == "%s"`, idField.RawName, id)

	// Delete data
	err := m.client.Delete(ctx, m.collection, "", expr)
	if err != nil {
		return fmt.Errorf("failed to delete documents for id %s: %w", id, err)
	}

	// Flush data
	err = m.client.Flush(ctx, m.collection, false)
	if err != nil {
		return fmt.Errorf("failed to flush collection after delete: %w", err)
	}

	return nil
}

// UpdateDoc updates documents by first deleting existing ones and then adding new ones
func (m *MilvusProvider) UpdateDoc(ctx context.Context, docs []schema.Document) error {
	// Delete existing documents
	ids := make([]string, len(docs))
	for i, doc := range docs {
		ids[i] = doc.ID
	}
	if err := m.DeleteDocs(ctx, ids); err != nil {
		return fmt.Errorf("failed to delete existing documents: %w", err)
	}
	// Add new documents
	if err := m.AddDoc(ctx, docs); err != nil {
		return fmt.Errorf("failed to add new documents: %w", err)
	}

	return nil
}

func (m *MilvusProvider) buildSearchParam() (entity.SearchParam, error) {
	// Get index configuration
	indexConfig, err := m.mapper.GetIndexConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get index config: %w", err)
	}

	// Get search configuration
	searchConfig, err := m.mapper.GetSearchConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get search config: %w", err)
	}

	// Choose appropriate search parameters based on index type
	milvusIndexType := strings.ToUpper(indexConfig.IndexType)
	if milvusIndexType == "" {
		milvusIndexType = "HNSW" // Default to HNSW index
	}

	switch milvusIndexType {
	case "FLAT":
		// FLAT and BIN_FLAT indices don't need additional search parameters
		return entity.NewIndexFlatSearchParam()

	case "BIN_FLAT", "IVF_FLAT", "BIN_IVF_FLAT", "IVF_SQ8":
		// Search parameters for IVF series indices
		nprobe := 16 // Default value
		if nprobeVal, err := searchConfig.ParamsFloat64("nprobe"); err == nil {
			nprobe = int(nprobeVal)
		}
		return entity.NewIndexIvfFlatSearchParam(nprobe)

	case "IVF_PQ":
		// Search parameters for IVF_PQ index
		nprobe := 16 // Default value
		if nprobeVal, err := searchConfig.ParamsFloat64("nprobe"); err == nil {
			nprobe = int(nprobeVal)
		}
		return entity.NewIndexIvfPQSearchParam(nprobe)

	case "HNSW":
		// Search parameters for HNSW index
		efSearch := 16 // Default value
		if efSearchVal, err := searchConfig.ParamsFloat64("ef"); err == nil {
			efSearch = int(efSearchVal)
		}
		return entity.NewIndexHNSWSearchParam(efSearch)

	case "IVF_HNSW":
		// Search parameters for IVF_HNSW index
		nprobe := 16   // Default value
		efSearch := 64 // Default value
		if nprobeVal, err := searchConfig.ParamsFloat64("nprobe"); err == nil {
			nprobe = int(nprobeVal)
		}
		if efSearchVal, err := searchConfig.ParamsFloat64("ef"); err == nil {
			efSearch = int(efSearchVal)
		}
		return entity.NewIndexIvfHNSWSearchParam(nprobe, efSearch)

	case "SCANN":
		// Search parameters for SCANN index
		nprobe := 16 // Default value
		reorder_k := 64
		if nprobeVal, err := searchConfig.ParamsFloat64("nprobe"); err == nil {
			nprobe = int(nprobeVal)
		}
		if reorderKVal, err := searchConfig.ParamsInt64("reorder_k"); err == nil {
			reorder_k = int(reorderKVal)
		}
		return entity.NewIndexSCANNSearchParam(nprobe, reorder_k)

	case "DISKANN":
		// Search parameters for DISKANN index
		search_list := 100 // Default value
		if searchListVal, err := searchConfig.ParamsInt64("search_list"); err == nil {
			search_list = int(searchListVal)
		}
		return entity.NewIndexDISKANNSearchParam(search_list)

	case "AUTOINDEX":
		level := 8
		if levelVal, err := searchConfig.ParamsInt64("level"); err == nil {
			level = int(levelVal)
		}
		// Search parameters for AUTOINDEX index
		return entity.NewIndexAUTOINDEXSearchParam(level)
	default:
		// Default to using HNSW search parameters
		return entity.NewIndexHNSWSearchParam(16)
	}
}

// SearchDocs performs similarity search for documents
func (m *MilvusProvider) SearchDocs(ctx context.Context, vector []float32, options *schema.SearchOptions) ([]schema.SearchResult, error) {
	if options == nil {
		options = &schema.SearchOptions{TopK: 10}
	}

	// Build search parameters
	sp, err := m.buildSearchParam()
	if err != nil {
		return nil, fmt.Errorf("failed to build search param: %w", err)
	}

	outputFields, _ := m.mapper.GetRawAllFieldNames()
	vectorField, _ := m.mapper.GetVectorField()
	searchConfig, _ := m.mapper.GetSearchConfig()
	metricType := m.GetMetricType(searchConfig.MetricType)

	// Build filter expression
	expr := ""
	searchResults, err := m.client.Search(
		ctx,
		m.collection,
		[]string{},   // partition names
		expr,         // filter expression
		outputFields, // output fields
		[]entity.Vector{entity.FloatVector(vector)},
		vectorField.RawName, // anns_field
		metricType,          // metric_type
		options.TopK,
		sp,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to search documents: %w", err)
	}

	// Parse results
	var results []schema.SearchResult
	for _, result := range searchResults {
		for i := 0; i < result.ResultCount; i++ {
			id, _ := result.IDs.Get(i)
			score := result.Scores[i]
			// Get field data
			var content string
			var metadata map[string]interface{}
			for _, field := range result.Fields {
				fieldMapping, err := m.mapper.GetField(field.Name())
				if err != nil {
					continue
				}
				fieldName := strings.ToLower(fieldMapping.StandardName)
				switch fieldName {
				case "content":
					if contentCol, ok := field.(*entity.ColumnVarChar); ok {
						if contentVal, err := contentCol.Get(i); err == nil {
							if contentStr, ok := contentVal.(string); ok {
								content = contentStr
							}
						}
					}
				case "metadata":
					if metaCol, ok := field.(*entity.ColumnJSONBytes); ok {
						if metaVal, err := metaCol.Get(i); err == nil {
							if metaBytes, ok := metaVal.([]byte); ok {
								if err := json.Unmarshal(metaBytes, &metadata); err != nil {
									metadata = make(map[string]interface{})
								}
							}
						}
					}
				}
			}
			searchResult := schema.SearchResult{
				Document: schema.Document{
					ID:       fmt.Sprintf("%s", id),
					Content:  content,
					Metadata: metadata,
				},
				Score: float64(score),
			}
			results = append(results, searchResult)
		}
	}
	return results, nil
}

// DeleteDocs deletes multiple documents by their IDs
func (m *MilvusProvider) DeleteDocs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	// Build delete expression
	// Milvus expects string values to be quoted within the expression, otherwise the parser will
	// treat the hyphen inside UUID as a minus operator and raise a parse error.
	quotedIDs := make([]string, len(ids))
	for i, id := range ids {
		quotedIDs[i] = fmt.Sprintf("\"%s\"", id)
	}

	idField, _ := m.mapper.GetIDField()
	expr := fmt.Sprintf("%s in [%s]", idField.RawName, strings.Join(quotedIDs, ","))

	// Delete data
	err := m.client.Delete(ctx, m.collection, "", expr)
	if err != nil {
		return fmt.Errorf("failed to delete documents: %w", err)
	}
	// Flush data
	err = m.client.Flush(ctx, m.collection, false)
	if err != nil {
		return fmt.Errorf("failed to flush collection after delete: %w", err)
	}

	return nil
}

// ListDocs retrieves all documents with optional limit
func (m *MilvusProvider) ListDocs(ctx context.Context, limit int) ([]schema.Document, error) {
	// Build query expression
	expr := ""
	// Query all relevant documents
	outputFields, _ := m.mapper.GetRawAllFieldNames()
	queryResult, err := m.client.Query(
		ctx,
		m.collection,
		[]string{}, // partitions
		expr,       // filter condition
		outputFields,
		client.WithOffset(0), client.WithLimit(int64(limit)),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to query documents: %w", err)
	}

	if len(queryResult) == 0 {
		return []schema.Document{}, nil
	}

	rowCount := queryResult[0].Len()
	documents := make([]schema.Document, 0, rowCount)

	// Parse query results
	for i := 0; i < rowCount; i++ {
		var (
			id        string
			content   string
			metadata  map[string]interface{}
			createdAt int64
		)

		for _, col := range queryResult {
			fieldMapping, err := m.mapper.GetField(col.Name())
			if err != nil {
				continue
			}
			fieldName := strings.ToLower(fieldMapping.StandardName)
			switch fieldName {
			case "id":
				if typedCol, ok := col.(*entity.ColumnVarChar); ok {
					if v, err := typedCol.Get(i); err == nil {
						if s, ok := v.(string); ok {
							id = s
						}
					}
				}
			case "content":
				if typedCol, ok := col.(*entity.ColumnVarChar); ok {
					if v, err := typedCol.Get(i); err == nil {
						if s, ok := v.(string); ok {
							content = s
						}
					}
				}
			case "metadata":
				if typedCol, ok := col.(*entity.ColumnJSONBytes); ok {
					if v, err := typedCol.Get(i); err == nil {
						if bytes, ok := v.([]byte); ok {
							_ = json.Unmarshal(bytes, &metadata)
						}
					}
				}
			case "created_at":
				if typedCol, ok := col.(*entity.ColumnInt64); ok {
					if v, err := typedCol.Get(i); err == nil {
						if ts, ok := v.(int64); ok {
							createdAt = ts
						}
					}
				}
			}
		}

		doc := schema.Document{
			ID:        id,
			Content:   content,
			Metadata:  metadata,
			CreatedAt: time.UnixMilli(createdAt),
		}
		documents = append(documents, doc)
	}
	return documents, nil
}

// GetProviderType returns the provider type identifier
func (m *MilvusProvider) GetProviderType() string {
	return MILVUS_PROVIDER_TYPE
}

// Close closes the connection to the Milvus server
func (m *MilvusProvider) Close() error {
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}
