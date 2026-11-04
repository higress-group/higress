package vector

import (
	"errors"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	PROVIDER_TYPE_DASH_VECTOR = "dashvector"
	PROVIDER_TYPE_CHROMA      = "chroma"
	PROVIDER_TYPE_ES          = "elasticsearch"
	PROVIDER_TYPE_WEAVIATE    = "weaviate"
	PROVIDER_TYPE_PINECONE    = "pinecone"
	PROVIDER_TYPE_QDRANT      = "qdrant"
	PROVIDER_TYPE_MILVUS      = "milvus"

	ConsumerContextKey = "aiCacheAuthenticatedConsumer"
	TenantField        = "consumer"
)

// ConsumerFromContext returns the consumer recorded after consumer
// authentication. Ai-cache must run after that authentication stage.
func ConsumerFromContext(ctx wrapper.HttpContext) string {
	if ctx == nil {
		return ""
	}
	consumer, _ := ctx.GetContext(ConsumerContextKey).(string)
	return consumer
}

type providerInitializer interface {
	ValidateConfig(ProviderConfig) error
	CreateProvider(ProviderConfig) (Provider, error)
}

var (
	ErrConsumerRequired = errors.New("authenticated consumer is required")

	providerInitializers = map[string]providerInitializer{
		PROVIDER_TYPE_DASH_VECTOR: &dashVectorProviderInitializer{},
		PROVIDER_TYPE_CHROMA:      &chromaProviderInitializer{},
		PROVIDER_TYPE_ES:          &esProviderInitializer{},
		PROVIDER_TYPE_WEAVIATE:    &weaviateProviderInitializer{},
		PROVIDER_TYPE_PINECONE:    &pineconeProviderInitializer{},
		PROVIDER_TYPE_QDRANT:      &qdrantProviderInitializer{},
		PROVIDER_TYPE_MILVUS:      &milvusProviderInitializer{},
	}
)

// QueryResult 定义通用的查询结果的结构体
type QueryResult struct {
	Text      string    // 相似的文本
	Embedding []float64 // 相似文本的向量
	Score     float64   // 文本的向量相似度或距离等度量
	Answer    string    // 相似文本对应的LLM生成的回答
	Consumer  string    // 上传时记录的认证 consumer
}

type Provider interface {
	GetProviderType() string
}

type EmbeddingQuerier interface {
	QueryEmbedding(
		emb []float64,
		ctx wrapper.HttpContext,
		log log.Log,
		callback func(results []QueryResult, ctx wrapper.HttpContext, log log.Log, err error)) error
}

type EmbeddingUploader interface {
	UploadEmbedding(
		queryString string,
		queryEmb []float64,
		ctx wrapper.HttpContext,
		log log.Log,
		callback func(ctx wrapper.HttpContext, log log.Log, err error)) error
}

type AnswerAndEmbeddingUploader interface {
	UploadAnswerAndEmbedding(
		queryString string,
		queryEmb []float64,
		answer string,
		ctx wrapper.HttpContext,
		log log.Log,
		callback func(ctx wrapper.HttpContext, log log.Log, err error)) error
}

type StringQuerier interface {
	QueryString(
		queryString string,
		ctx wrapper.HttpContext,
		log log.Log,
		callback func(results []QueryResult, ctx wrapper.HttpContext, log log.Log, err error)) error
}

type ProviderConfig struct {
	// @Title zh-CN 向量存储服务提供者类型
	// @Description zh-CN 向量存储服务提供者类型，例如 dashvector、chroma
	typ string
	// @Title zh-CN 向量存储服务名称
	// @Description zh-CN 向量存储服务名称
	serviceName string
	// @Title zh-CN 向量存储服务域名
	// @Description zh-CN 向量存储服务域名
	serviceHost string
	// @Title zh-CN 向量存储服务端口
	// @Description zh-CN 向量存储服务端口
	servicePort int64
	// @Title zh-CN 向量存储服务 API Key
	// @Description zh-CN 向量存储服务 API Key
	apiKey string
	// @Title zh-CN 返回TopK结果
	// @Description zh-CN 返回TopK结果，默认为 1
	topK int
	// @Title zh-CN 请求超时
	// @Description zh-CN 请求向量存储服务的超时时间，单位为毫秒。默认值是10000，即10秒
	timeout uint32
	// @Title zh-CN 向量存储服务 Collection ID
	// @Description zh-CN 向量存储服务的 Collection ID
	collectionID string
	// @Title zh-CN 相似度度量阈值
	// @Description zh-CN 启用语义缓存时必须显式配置，无数值默认值。
	Threshold    float64
	thresholdSet bool
	// @Title zh-CN 相似度度量比较方式
	// @Description zh-CN 启用语义缓存时必须显式配置。
	// 相似度度量方式有 Cosine, DotProduct, Euclidean 等，前两者值越大相似度越高，后者值越小相似度越高。
	// 所以需要允许自定义比较方式，对于 Cosine 和 DotProduct 选择 gt，对于 Euclidean 则选择 lt。
	// 所有条件包括 lt (less than，小于)、lte (less than or equal to，小等于)、gt (greater than，大于)、gte (greater than or equal to，大等于)
	ThresholdRelation    string
	thresholdRelationSet bool

	// ES 配置
	// @Title zh-CN ES 用户名
	// @Description zh-CN ES 用户名
	esUsername string
	// @Title zh-CN ES 密码
	// @Description zh-CN ES 密码
	esPassword string
}

func (c *ProviderConfig) GetProviderType() string {
	return c.typ
}

func (c *ProviderConfig) HasThreshold() bool {
	return c.thresholdSet
}

func (c *ProviderConfig) HasThresholdRelation() bool {
	return c.thresholdRelationSet
}

func (c *ProviderConfig) FromJson(json gjson.Result) {
	c.typ = json.Get("type").String()
	c.serviceName = json.Get("serviceName").String()
	c.serviceHost = json.Get("serviceHost").String()
	c.servicePort = int64(json.Get("servicePort").Int())
	if c.servicePort == 0 {
		c.servicePort = 443
	}
	c.apiKey = json.Get("apiKey").String()
	c.collectionID = json.Get("collectionID").String()
	c.topK = int(json.Get("topK").Int())
	if c.topK == 0 {
		c.topK = 1
	}
	c.timeout = uint32(json.Get("timeout").Int())
	if c.timeout == 0 {
		c.timeout = 10000
	}
	threshold := json.Get("threshold")
	c.thresholdSet = threshold.Exists() && threshold.Type != gjson.Null
	c.Threshold = threshold.Float()
	relation := json.Get("thresholdRelation")
	c.thresholdRelationSet = relation.Exists() && relation.String() != ""
	c.ThresholdRelation = relation.String()

	// ES
	c.esUsername = json.Get("esUsername").String()
	c.esPassword = json.Get("esPassword").String()
}

func (c *ProviderConfig) Validate() error {
	if c.typ == "" {
		return errors.New("vector database service is required")
	}
	initializer, has := providerInitializers[c.typ]
	if !has {
		return errors.New("unknown vector database service provider type: " + c.typ)
	}
	if c.ThresholdRelation != "" && !isRelationValid(c.ThresholdRelation) {
		return errors.New("invalid thresholdRelation: " + c.ThresholdRelation)
	}
	if err := initializer.ValidateConfig(*c); err != nil {
		return err
	}
	return nil
}

func CreateProvider(pc ProviderConfig) (Provider, error) {
	initializer, has := providerInitializers[pc.typ]
	if !has {
		return nil, errors.New("unknown provider type: " + pc.typ)
	}
	return initializer.CreateProvider(pc)
}

func isRelationValid(relation string) bool {
	for _, r := range []string{"lt", "lte", "gt", "gte"} {
		if r == relation {
			return true
		}
	}
	return false
}
