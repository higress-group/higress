package vectordb

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/rag/config"
	"github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/rag/schema"
)

const (
	qdrantDefaultHost = "localhost"
	qdrantDefaultPort = 6334
	qdrantDefaultTopK = 10
)

type qdrantProviderInitializer struct{}

func (q *qdrantProviderInitializer) InitConfig(cfg *config.VectorDBConfig) error {
	if cfg.Provider != PROVIDER_TYPE_QDRANT {
		return fmt.Errorf("provider type mismatch: expected %s, got %s", PROVIDER_TYPE_QDRANT, cfg.Provider)
	}
	if cfg.Host == "" {
		cfg.Host = qdrantDefaultHost
	}
	if cfg.Port == 0 {
		cfg.Port = qdrantDefaultPort
	}
	if cfg.Collection == "" {
		cfg.Collection = schema.DEFAULT_DOCUMENT_COLLECTION
	}
	return nil
}

func (q *qdrantProviderInitializer) ValidateConfig(cfg *config.VectorDBConfig) error {
	if cfg.Host == "" {
		return fmt.Errorf("qdrant host is required")
	}
	if cfg.Port <= 0 {
		return fmt.Errorf("qdrant port must be positive")
	}
	if cfg.Collection == "" {
		return fmt.Errorf("qdrant collection is required")
	}
	return nil
}

func (q *qdrantProviderInitializer) CreateProvider(cfg *config.VectorDBConfig, dim int) (VectorStoreProvider, error) {
	if err := q.InitConfig(cfg); err != nil {
		return nil, err
	}
	if err := q.ValidateConfig(cfg); err != nil {
		return nil, err
	}
	return NewQdrantProvider(cfg, dim)
}

type QdrantProvider struct {
	client     *qdrant.Client
	collection string
	mapper     VectorDBMapper
}

func NewQdrantProvider(cfg *config.VectorDBConfig, dimensions int) (VectorStoreProvider, error) {
	if dimensions <= 0 {
		return nil, fmt.Errorf("qdrant vector dimension must be positive")
	}
	mapper, err := NewDefaultVectorDBMapper(PROVIDER_TYPE_QDRANT, cfg.Mapping)
	if err != nil {
		return nil, fmt.Errorf("failed to create default vector db mapper: %w", err)
	}
	client, err := qdrant.NewClient(qdrantDialConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to create qdrant client: %w", err)
	}
	provider := &QdrantProvider{client: client, collection: cfg.Collection, mapper: mapper}
	if err := provider.CreateCollection(context.Background(), dimensions); err != nil {
		_ = client.Close()
		return nil, err
	}
	return provider, nil
}

func qdrantDialConfig(cfg *config.VectorDBConfig) *qdrant.Config {
	host := strings.TrimSpace(cfg.Host)
	port := cfg.Port
	useTLS := false
	if u, err := url.Parse(host); err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
		useTLS = u.Scheme == "https"
		host = u.Host
	}
	if h, p, err := net.SplitHostPort(host); err == nil {
		host = h
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	if port == 0 {
		port = qdrantDefaultPort
	}
	return &qdrant.Config{
		Host:                   host,
		Port:                   port,
		APIKey:                 cfg.Password,
		UseTLS:                 useTLS,
		SkipCompatibilityCheck: true,
	}
}

func (q *QdrantProvider) CreateCollection(ctx context.Context, dim int) error {
	if dim <= 0 {
		return fmt.Errorf("qdrant vector dimension must be positive")
	}
	exists, err := q.client.CollectionExists(ctx, q.collection)
	if err != nil {
		return fmt.Errorf("failed to check %s collection existence: %w", q.collection, err)
	}
	if exists {
		info, err := q.client.GetCollectionInfo(ctx, q.collection)
		if err != nil {
			return fmt.Errorf("failed to inspect %s collection: %w", q.collection, err)
		}
		existingDim, err := qdrantCollectionDim(info)
		if err != nil {
			return fmt.Errorf("failed to inspect %s collection: %w", q.collection, err)
		}
		if existingDim != dim {
			return fmt.Errorf("collection %s exists with dimension %d, expected %d", q.collection, existingDim, dim)
		}
		return nil
	}
	if err := q.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: q.collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(dim),
			Distance: q.metricType(),
		}),
		HnswConfig: q.hnswConfig(),
	}); err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}
	return nil
}

func (q *QdrantProvider) DropCollection(ctx context.Context) error {
	exists, err := q.client.CollectionExists(ctx, q.collection)
	if err != nil {
		return fmt.Errorf("failed to check %s collection existence: %w", q.collection, err)
	}
	if !exists {
		return fmt.Errorf("collection %s does not exist", q.collection)
	}
	if err := q.client.DeleteCollection(ctx, q.collection); err != nil {
		return fmt.Errorf("failed to drop collection: %w", err)
	}
	return nil
}

func (q *QdrantProvider) AddDoc(ctx context.Context, docs []schema.Document) error {
	if len(docs) == 0 {
		return nil
	}
	points := make([]*qdrant.PointStruct, 0, len(docs))
	for _, doc := range docs {
		point, err := q.docToPoint(doc)
		if err != nil {
			return err
		}
		points = append(points, point)
	}
	if _, err := q.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: q.collection,
		Points:         points,
		Wait:           qdrant.PtrOf(true),
	}); err != nil {
		return fmt.Errorf("failed to insert documents: %w", err)
	}
	return nil
}

func (q *QdrantProvider) DeleteDoc(ctx context.Context, id string) error {
	return q.DeleteDocs(ctx, []string{id})
}

func (q *QdrantProvider) UpdateDoc(ctx context.Context, docs []schema.Document) error {
	return q.AddDoc(ctx, docs)
}

func (q *QdrantProvider) SearchDocs(ctx context.Context, vector []float32, options *schema.SearchOptions) ([]schema.SearchResult, error) {
	topK := qdrantDefaultTopK
	threshold := 0.0
	if options != nil {
		if options.TopK > 0 {
			topK = options.TopK
		}
		threshold = options.Threshold
	}
	req := &qdrant.QueryPoints{
		CollectionName: q.collection,
		Query:          qdrant.NewQueryDense(vector),
		Limit:          qdrant.PtrOf(uint64(topK)),
		WithPayload:    qdrant.NewWithPayload(true),
		Params:         q.searchParams(),
	}
	if threshold > 0 {
		req.ScoreThreshold = qdrant.PtrOf(float32(threshold))
	}
	scored, err := q.client.Query(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to search documents: %w", err)
	}
	results := make([]schema.SearchResult, 0, len(scored))
	for _, point := range scored {
		results = append(results, schema.SearchResult{
			Document: q.pointToDoc(point.GetId(), point.GetPayload()),
			Score:    float64(point.GetScore()),
		})
	}
	return results, nil
}

func (q *QdrantProvider) DeleteDocs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	pointIDs := make([]*qdrant.PointId, 0, len(ids))
	for _, id := range ids {
		pointID, err := qdrantPointID(id)
		if err != nil {
			return fmt.Errorf("failed to delete documents: %w", err)
		}
		pointIDs = append(pointIDs, pointID)
	}
	if _, err := q.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: q.collection,
		Points:         qdrant.NewPointsSelector(pointIDs...),
		Wait:           qdrant.PtrOf(true),
	}); err != nil {
		return fmt.Errorf("failed to delete documents: %w", err)
	}
	return nil
}

func (q *QdrantProvider) ListDocs(ctx context.Context, limit int) ([]schema.Document, error) {
	if limit <= 0 {
		limit = qdrantDefaultTopK
	}
	docs := make([]schema.Document, 0, limit)
	req := &qdrant.ScrollPoints{
		CollectionName: q.collection,
		WithPayload:    qdrant.NewWithPayload(true),
	}
	for len(docs) < limit {
		req.Limit = qdrant.PtrOf(uint32(limit - len(docs)))
		points, next, err := q.client.ScrollAndOffset(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("failed to query documents: %w", err)
		}
		if len(points) == 0 {
			break
		}
		for _, point := range points {
			docs = append(docs, q.pointToDoc(point.GetId(), point.GetPayload()))
			if len(docs) == limit {
				break
			}
		}
		if next == nil {
			break
		}
		req.Offset = next
	}
	return docs, nil
}

func (q *QdrantProvider) GetProviderType() string {
	return PROVIDER_TYPE_QDRANT
}

func (q *QdrantProvider) Close() error {
	if q.client == nil {
		return nil
	}
	return q.client.Close()
}

func (q *QdrantProvider) docToPoint(doc schema.Document) (*qdrant.PointStruct, error) {
	id := doc.ID
	if id == "" {
		idField, err := q.mapper.GetIDField()
		if err != nil {
			return nil, err
		}
		if !idField.IsAutoID() {
			return nil, fmt.Errorf("document id is required")
		}
		id = uuid.New().String()
	}
	pointID, err := qdrantPointID(id)
	if err != nil {
		return nil, err
	}
	if len(doc.Vector) == 0 {
		return nil, fmt.Errorf("document vector is required")
	}
	payload := map[string]any{}
	q.setPayload(payload, "content", doc.Content)
	metadata := doc.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	q.setPayload(payload, "metadata", metadata)
	q.setPayload(payload, "created_at", doc.CreatedAt.UnixMilli())
	valueMap, err := qdrant.TryValueMap(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode payload: %w", err)
	}
	return &qdrant.PointStruct{
		Id:      pointID,
		Vectors: qdrant.NewVectorsDense(doc.Vector),
		Payload: valueMap,
	}, nil
}

func (q *QdrantProvider) pointToDoc(id *qdrant.PointId, payload map[string]*qdrant.Value) schema.Document {
	doc := schema.Document{
		ID:       qdrantPointIDString(id),
		Metadata: map[string]any{},
	}
	if payload == nil {
		return doc
	}
	if name, ok := q.rawField("content"); ok {
		doc.Content = payload[name].GetStringValue()
	}
	if name, ok := q.rawField("metadata"); ok {
		if s := payload[name].GetStructValue(); s != nil {
			meta := make(map[string]any, len(s.Fields))
			for k, v := range s.Fields {
				meta[k] = qdrantScalar(v)
			}
			doc.Metadata = meta
		}
	}
	if name, ok := q.rawField("created_at"); ok {
		if v := payload[name]; v != nil {
			if _, isInt := v.Kind.(*qdrant.Value_IntegerValue); isInt {
				doc.CreatedAt = time.UnixMilli(v.GetIntegerValue())
			}
		}
	}
	return doc
}

func (q *QdrantProvider) setPayload(payload map[string]any, standard string, value any) {
	if name, ok := q.rawField(standard); ok {
		payload[name] = value
	}
}

func (q *QdrantProvider) rawField(standard string) (string, bool) {
	field, err := q.mapper.GetRawField(standard)
	if err != nil {
		return "", false
	}
	return field.RawName, true
}

func (q *QdrantProvider) metricType() qdrant.Distance {
	searchConfig, _ := q.mapper.GetSearchConfig()
	switch strings.ToUpper(searchConfig.MetricType) {
	case "L2", "EUCLID", "EUCLIDEAN":
		return qdrant.Distance_Euclid
	case "IP", "DOT", "DOTPRODUCT", "INNER_PRODUCT":
		return qdrant.Distance_Dot
	default:
		return qdrant.Distance_Cosine
	}
}

func (q *QdrantProvider) hnswConfig() *qdrant.HnswConfigDiff {
	indexConfig, _ := q.mapper.GetIndexConfig()
	if t := strings.ToUpper(indexConfig.IndexType); t != "" && t != "HNSW" {
		return nil
	}
	cfg := &qdrant.HnswConfigDiff{}
	if m, err := indexConfig.ParamsInt64("M"); err == nil {
		cfg.M = qdrant.PtrOf(uint64(m))
	}
	if ef, err := indexConfig.ParamsInt64("efConstruction"); err == nil {
		cfg.EfConstruct = qdrant.PtrOf(uint64(ef))
	}
	if cfg.M == nil && cfg.EfConstruct == nil {
		return nil
	}
	return cfg
}

func (q *QdrantProvider) searchParams() *qdrant.SearchParams {
	searchConfig, _ := q.mapper.GetSearchConfig()
	ef, err := searchConfig.ParamsInt64("ef")
	if err != nil {
		ef, err = searchConfig.ParamsInt64("hnsw_ef")
	}
	if err != nil {
		return nil
	}
	return &qdrant.SearchParams{HnswEf: qdrant.PtrOf(uint64(ef))}
}

func qdrantPointID(id string) (*qdrant.PointId, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("document id is required")
	}
	if _, err := uuid.Parse(id); err == nil {
		return qdrant.NewIDUUID(id), nil
	}
	if n, err := strconv.ParseUint(id, 10, 64); err == nil {
		return qdrant.NewIDNum(n), nil
	}
	return nil, fmt.Errorf("invalid qdrant point id %q: valid values are either an unsigned integer or a UUID", id)
}

func qdrantPointIDString(id *qdrant.PointId) string {
	if id == nil {
		return ""
	}
	if id.GetUuid() != "" {
		return id.GetUuid()
	}
	return strconv.FormatUint(id.GetNum(), 10)
}

func qdrantCollectionDim(info *qdrant.CollectionInfo) (int, error) {
	params := info.GetConfig().GetParams().GetVectorsConfig().GetParams()
	if params == nil || params.GetSize() == 0 {
		return 0, fmt.Errorf("collection vector size is missing")
	}
	return int(params.GetSize()), nil
}

func qdrantScalar(v *qdrant.Value) any {
	if v == nil {
		return nil
	}
	switch v.Kind.(type) {
	case *qdrant.Value_BoolValue:
		return v.GetBoolValue()
	case *qdrant.Value_IntegerValue:
		return v.GetIntegerValue()
	case *qdrant.Value_DoubleValue:
		return v.GetDoubleValue()
	case *qdrant.Value_StringValue:
		return v.GetStringValue()
	default:
		return nil
	}
}
