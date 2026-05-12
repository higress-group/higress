---
title: AI Cache
keywords: [higress,ai cache]
description: AI Cache Plugin Configuration Reference
---

## Function Description

LLM result caching plugin. The default configuration can be directly used for OpenAI protocol result caching, and supports caching of both streaming and non-streaming responses.

**Tips**

When carrying the request header `x-higress-skip-ai-cache: on`, the current request will not use content from the cache but will be directly forwarded to the backend service. Additionally, the response content from this request will not be cached.

## Runtime Properties

Plugin Execution Phase: `Authentication Phase`
Plugin Execution Priority: `10`

## Configuration Description

The configuration is divided into 3 parts: Vector Database (vector), Text Embedding Service (embedding), and Cache Service (cache). It also provides fine-grained LLM request/response extraction parameter configurations.

This plugin supports both vector database-based semantic caching and string matching-based caching methods. If both vector database and cache database are configured, the cache database is used first, and the vector database capability is used when cache misses occur.

*Note*: Vector database (vector) and cache database (cache) cannot both be empty, otherwise this plugin cannot provide caching services.

| Name | Type | Requirement | Default | Description |
| --- | --- | --- | --- | --- |
| vector | object | optional | - | Vector storage service configuration, see Vector Database Service section below |
| embedding | object | optional | - | Text embedding service configuration, see Text Embedding Service section below |
| cache | object | optional | - | Cache service configuration, see Cache Service section below |
| cacheKeyStrategy | string | optional | "lastQuestion" | Strategy for generating cache key from historical questions. Options: "lastQuestion" (use last question), "allQuestions" (concatenate all questions), or "disabled" (disable caching) |
| enableSemanticCache | bool | optional | false | Whether to enable semantic caching. If disabled, string matching is used to find cache, requiring cache service configuration. Automatically enabled when a vector provider is configured |

Depending on whether semantic caching is needed, you can configure component combinations as follows:
1. `cache`: Enable string matching cache only
2. `vector (+ embedding)`: Enable semantic caching. If `vector` does not provide string representation service, you need to configure `embedding` service separately
3. `vector (+ embedding) + cache`: Enable semantic caching and use cache service to store LLM responses for acceleration

If you do not configure a related component, you can ignore the `required` fields of that component.

## Vector Database Service (vector)

| Name | Type | Requirement | Default | Description |
| --- | --- | --- | --- | --- |
| vector.type | string | required | - | Vector storage service provider type, e.g., dashvector, chroma, elasticsearch, weaviate, pinecone, qdrant, milvus |
| vector.serviceName | string | required | - | Vector storage service name |
| vector.serviceHost | string | optional | - | Vector storage service domain. Required for some providers (e.g., dashvector, pinecone) |
| vector.servicePort | int64 | optional | 443 | Vector storage service port |
| vector.apiKey | string | optional | - | Vector storage service API Key |
| vector.topK | int | optional | 1 | Return TopK results |
| vector.timeout | uint32 | optional | 10000 | Timeout for requesting vector storage service, in milliseconds. Default is 10000 (10 seconds) |
| vector.collectionID | string | optional | - | Vector storage service Collection ID |
| vector.threshold | float64 | optional | 1000 | Vector similarity measurement threshold |
| vector.thresholdRelation | string | optional | "lt" | Similarity measurement comparison method. Similarity measurement methods include `Cosine`, `DotProduct`, `Euclidean`, etc. The first two have higher similarity with larger values, while the latter has higher similarity with smaller values. Use `gt` for `Cosine` and `DotProduct`, and `lt` for `Euclidean`. All options include `lt` (less than), `lte` (less than or equal to), `gt` (greater than), `gte` (greater than or equal to) |
| vector.esUsername | string | optional | - | ElasticSearch username, only for elasticsearch type |
| vector.esPassword | string | optional | - | ElasticSearch password, only for elasticsearch type |

## Text Embedding Service (embedding)

| Name | Type | Requirement | Default | Description |
| --- | --- | --- | --- | --- |
| embedding.type | string | required | - | Text embedding service type, e.g., dashscope, openai, azure, cohere, ollama, huggingface, textin, xfyun |
| embedding.serviceName | string | required | - | Text embedding service name |
| embedding.serviceHost | string | optional | - | Text embedding service domain |
| embedding.servicePort | int64 | optional | 443 | Text embedding service port. Default varies by provider; ollama defaults to 11434 |
| embedding.timeout | uint32 | optional | 10000 | Timeout for requesting text embedding service, in milliseconds. Default is 10000 (10 seconds) |
| embedding.model | string | optional | - | Model name for text embedding service |
| embedding.apiKey | string | optional | - | API Key for text embedding service |

## Cache Service (cache)

| Name | Type | Requirement | Default | Description |
| --- | --- | --- | --- | --- |
| cache.type | string | required | - | Cache service type, e.g., redis |
| cache.serviceName | string | required | - | Cache service name |
| cache.serviceHost | string | optional | - | Cache service domain |
| cache.servicePort | int64 | optional | 6379 | Cache service port. If serviceName ends with .static, default is 80 |
| cache.username | string | optional | - | Cache service username |
| cache.password | string | optional | - | Cache service password |
| cache.timeout | uint32 | optional | 10000 | Cache service timeout, in milliseconds. Default is 10000 (10 seconds) |
| cache.cacheTTL | int | optional | 0 | Cache expiration time, in seconds. Default is 0 (never expire) |
| cache.cacheKeyPrefix | string | optional | "higress-ai-cache:" | Prefix for cache keys |
| cache.database | int | optional | 0 | Database ID to use, only for Redis. For example, configure as 1 for `SELECT 1` |

## Other Configurations

| Name | Type | Requirement | Default | Description |
| --- | --- | --- | --- | --- |
| cacheKeyFrom | string | optional | "messages.@reverse.0.content" | Extract string from request Body using [GJSON PATH](https://github.com/tidwall/gjson/blob/master/SYNTAX.md) syntax |
| cacheValueFrom | string | optional | "choices.0.message.content" | Extract string from response Body using [GJSON PATH](https://github.com/tidwall/gjson/blob/master/SYNTAX.md) syntax |
| cacheStreamValueFrom | string | optional | "choices.0.delta.content" | Extract string from streaming response Body using [GJSON PATH](https://github.com/tidwall/gjson/blob/master/SYNTAX.md) syntax |
| cacheToolCallsFrom | string | optional | "choices.0.delta.content.tool_calls" | Extract string from streaming response Body using [GJSON PATH](https://github.com/tidwall/gjson/blob/master/SYNTAX.md) syntax |
| responseTemplate | string | optional | `{"id":"from-cache","choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"finish_reason":"stop"}],"model":"from-cache","object":"chat.completion","usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}` | Template for returning HTTP response, with %s marking the part to be replaced by cache value |
| streamResponseTemplate | string | optional | `data:{"id":"from-cache","choices":[{"index":0,"delta":{"role":"assistant","content":"%s"},"finish_reason":"stop"}],"model":"from-cache","object":"chat.completion","usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}\n\ndata:[DONE]\n\n` | Template for returning streaming HTTP response, with %s marking the part to be replaced by cache value |

## Text Embedding Provider Specific Configurations

### Azure OpenAI

For Azure OpenAI, set `embedding.type` to `azure`. You need to first create an [Azure OpenAI account](https://portal.azure.com/#view/Microsoft_Azure_ProjectOxford/CognitiveServicesHub/~/overview), then select and deploy a model in [Azure AI Foundry](https://ai.azure.com/resource/deployments). Click on your deployed model to see the target URI and key in the endpoint section. Please enter the host from the URI in `embedding.serviceHost` and the key in `embedding.apiKey`.

A complete URI example is https://YOUR_RESOURCE_NAME.openai.azure.com/openai/deployments/YOUR_DEPLOYMENT_NAME/embeddings?api-version=2024-10-21. You need to enter `YOUR_RESOURCE_NAME.openai.azure.com` in `embedding.serviceHost`.

Specific configuration fields:

| Name | Data Type | Requirement | Default | Description |
| ---------------------- | -------- | -------- | ------ | ------- |
| `embedding.apiVersion` | string | required | - | API version, the api-version value from the obtained URI |

Note that you must specify `embedding.serviceHost`, such as `YOUR_RESOURCE_NAME.openai.azure.com`. The default model is `text-embedding-ada-002`. For other models, specify in `embedding.model`.

### Cohere

For Cohere, set `embedding.type` to `cohere`. There are no specific configuration fields. You need to create an [API Key](https://docs.cohere.com/reference/embed) and enter it in `embedding.apiKey`.

### OpenAI

For OpenAI, set `embedding.type` to `openai`. There are no specific configuration fields. You need to create an [API Key](https://platform.openai.com/settings/organization/api-keys) and enter it in `embedding.apiKey`. An API Key example is `sk-xxxxxxx`.

### Ollama

For Ollama, set `embedding.type` to `ollama`. There are no specific configuration fields.

### Hugging Face

For Hugging Face, set `embedding.type` to `huggingface`. There are no specific configuration fields. You need to create an [hf_token](https://huggingface.co/blog/getting-started-with-embeddings) and enter it in `embedding.apiKey`. An hf_token example is `hf_xxxxxxx`.

`embedding.model` defaults to `sentence-transformers/all-MiniLM-L6-v2`.

### DashScope

For DashScope, set `embedding.type` to `dashscope`. You need to create an [API Key](https://help.aliyun.com/document_detail/2712195.html) and enter it in `embedding.apiKey`.

`embedding.model` defaults to `text-embedding-v2`. Other models like `text-embedding-v1` can also be used.

### TextIn

For TextIn, set `embedding.type` to `textin`. You need to first obtain [`app-id` and `secret-code`](https://www.textin.com/document/acge_text_embedding).

Specific configuration fields:

| Name | Data Type | Requirement | Default | Description |
| ------------------------------- | -------- | -------- | ------ | ------------------ |
| `embedding.textinAppId` | string | required | - | Application ID, obtained app-id |
| `embedding.textinSecretCode` | string | required | - | Secret for calling API, obtained secret-code |
| `embedding.textinMatryoshkaDim` | int | required | - | Dimension of returned single vector |

### Xfyun (讯飞星火)

For Xfyun, set `embedding.type` to `xfyun`. You need to first create an [application](https://console.xfyun.cn/services/emb) to obtain `APPID`, `APISecret`, and `APIKey`, and enter `APIKey` in `embedding.apiKey`.

Specific configuration fields:

| Name | Data Type | Requirement | Default | Description |
| --------------------- | -------- | -------- | ------ | -------------------- |
| `embedding.appId` | string | required | - | Application ID, obtained APPID |
| `embedding.apiSecret` | string | required | - | Secret for calling API, obtained APISecret |

## Vector Database Provider Specific Configurations

### Chroma

For Chroma, set `vector.type` to `chroma`. There are no specific configuration fields. You need to create a Collection in advance and fill in the Collection ID in `vector.collectionID`. A Collection ID example is `52bbb8b3-724c-477b-a4ce-d5b578214612`.

### DashVector

For DashVector, set `vector.type` to `dashvector`. There are no specific configuration fields. You need to create a Collection in advance and fill in the `Collection Name` in `vector.collectionID`.

### ElasticSearch

For ElasticSearch, set `vector.type` to `elasticsearch`. You need to create an Index in advance and fill in the Index Name in `vector.collectionID`.

It currently relies on the [KNN](https://www.elastic.co/guide/en/elasticsearch/reference/current/knn-search.html) method. Please ensure your ES version supports `KNN`. It has been tested on version `8.16`.

Specific configuration fields:

| Name | Data Type | Requirement | Default | Description |
|-------------------|----------|----------|--------|-------------------------------------------------------------------------------|
| `vector.esUsername` | string | optional | - | ElasticSearch username |
| `vector.esPassword` | string | optional | - | ElasticSearch password |

`vector.esUsername` and `vector.esPassword` are used for Basic authentication. API Key authentication is also supported. When `vector.apiKey` is filled in, API Key authentication is enabled. For SaaS versions, you need to fill in the `encoded` value.

### Milvus

For Milvus, set `vector.type` to `milvus`. There are no specific configuration fields. You need to create a Collection in advance and fill in the Collection Name in `vector.collectionID`.

### Pinecone

For Pinecone, set `vector.type` to `pinecone`. There are no specific configuration fields. You need to create an Index in advance and fill in the Index access domain in `vector.serviceHost`.

The `Namespace` parameter in Pinecone is configured through the plugin's `vector.collectionID`. If `vector.collectionID` is not filled in, it defaults to the Default Namespace.

### Qdrant

For Qdrant, set `vector.type` to `qdrant`. There are no specific configuration fields. You need to create a Collection in advance and fill in the Collection Name in `vector.collectionID`.

### Weaviate

For Weaviate, set `vector.type` to `weaviate`. There are no specific configuration fields. You need to create a Collection in advance and fill in the Collection Name in `vector.collectionID`.

Note that Weaviate automatically capitalizes the first letter, so when filling in `collectionID`, the first letter should be capitalized.

If using SaaS, you need to fill in the `vector.serviceHost` parameter.

## Configuration Example

### Basic Configuration
```yaml
embedding:
  type: dashscope
  serviceName: my_dashscope.dns
  apiKey: [Your Key]

vector:
  type: dashvector
  serviceName: my_dashvector.dns
  collectionID: [Your Collection ID]
  serviceHost: [Your domain]
  apiKey: [Your key]

cache:
  type: redis
  serviceName: my_redis.dns
  servicePort: 6379
  timeout: 100
```

## Advanced Usage

The current default cache key is extracted based on the GJSON PATH expression: `messages.@reverse.0.content`, which means reversing the messages array and taking the content of the first item.

GJSON PATH supports conditional syntax. For example, to get the content of the last role as user as the key, you can write: `messages.@reverse.#(role=="user").content`;

If you want to concatenate all content with role as user into an array as the key, you can write: `messages.@reverse.#(role=="user")#.content`;

It also supports pipeline syntax. For example, to get the second-to-last role as user as the key, you can write: `messages.@reverse.#(role=="user")#.content|1`.

For more usage, please refer to the [official documentation](https://github.com/tidwall/gjson/blob/master/SYNTAX.md). You can use the [GJSON Playground](https://gjson.dev/) for syntax testing.

## FAQ

1. If the returned error is `error status returned by host: bad argument`, please check if `serviceName` correctly includes the service type suffix (.dns, etc.).
