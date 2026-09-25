---
title: AI 搜索增强
keywords: [higress,ai search]
description: higress 支持通过集成搜索引擎（Google/Bing/Arxiv/Elasticsearch等）的实时结果，增强DeepSeek-R1等模型等回答准确性和时效性
---

## 功能说明

`ai-search`插件通过集成搜索引擎（Google/Bing/Arxiv/Elasticsearch等）的实时结果，增强AI模型的回答准确性和时效性。插件会自动将搜索结果注入到提示模板中，并根据配置决定是否在最终回答中添加引用来源。

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`460`

## 配置字段

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|------|----------|----------|--------|------|
| defaultEnable | bool | 选填 | true | 插件功能默认是否开启。设置为false时，仅当请求中包含web_search_options字段时才启用插件功能 |
| needReference | bool | 选填 | false | 是否在回答中添加引用来源 |
| referenceFormat | string | 选填 | `"**References:**\n%s"` | 引用内容格式，必须包含%s占位符 |
| referenceLocation | string | 选填 | "head" | 引用位置："head"在回答开头，"tail"在回答结尾 |
| defaultLang | string | 选填 | - | 默认搜索语言代码（如zh-CN/en-US） |
| promptTemplate | string | 选填 | 内置模板 | 提示模板，必须包含`{search_results}`和`{question}`占位符 |
| searchFrom | array of object | 必填 | - | 参考下面搜索引擎配置，至少配置一个引擎 |
| searchRewrite | object | 选填 | - | 搜索重写配置，用于使用LLM服务优化搜索查询 |

## 搜索重写说明

搜索重写功能使用LLM服务对用户的原始查询进行分析和优化，可以：
1. 识别用户问题是否需要查询搜索引擎，如果不需要，不会执行搜索增强相关逻辑
2. 将用户的自然语言查询转换为更适合搜索引擎的关键词组合
3. 对于Arxiv论文搜索，自动识别相关的论文类别并添加类别限定
4. 对于私有知识库搜索，将长查询拆分成多个精准的关键词组合

强烈建议在使用Arxiv或Elasticsearch引擎时启用此功能。对于Arxiv搜索，它能准确识别论文所属领域并优化英文关键词；对于私有知识库搜索，它能提供更精准的关键词匹配，显著提升搜索效果。

## 搜索重写配置

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|------|----------|----------|--------|------|
| llmServiceName | string | 必填 | - | LLM服务名称 |
| llmServicePort | number | 必填 | - | LLM服务端口 |
| llmApiKey | string | 选填 | - | LLM服务API密钥 |
| llmUrl | string | 必填 | - | LLM服务API地址 |
| llmModelName | string | 必填 | - | LLM模型名称 |
| timeoutMillisecond | number | 选填 | 30000 | API调用超时时间（毫秒） |
| maxCount | number | 选填 | 3 | 搜索重写生成的最大查询次数 |

## 搜索引擎通用配置

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|------|----------|----------|--------|------|
| type | string | 必填 | - | 引擎类型（google/bing/arxiv/elasticsearch/quark） |
| serviceName | string | 必填 | - | 后端服务名称 |
| servicePort | number | 必填 | - | 后端服务端口 |
| apiKey | string | 必填 | - | 搜索引擎API密钥/Aliyun AccessKey |
| count | number | 选填 | 10 | 单次搜索返回结果数量 |
| start | number | 选填 | 0 | 搜索结果偏移量（从第start+1条结果开始返回） |
| timeoutMillisecond | number | 选填 | 5000 | API调用超时时间（毫秒） |
| optionArgs | map | 选填 | - | 搜索引擎特定参数（key-value格式） |

## Google 特定配置

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|------|----------|----------|--------|------|
| cx | string | 必填 | - | Google自定义搜索引擎ID，用于指定搜索范围 |

## Arxiv 特定配置

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|------|----------|----------|--------|------|
| arxivCategory | string | 选填 | - | 搜索的论文[类别](https://arxiv.org/category_taxonomy)（如cs.AI, cs.CL等） |

## Elasticsearch 特定配置

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述                                 |
|------|----------|------|--------|------------------------------------|
| index | string | 必填   | - | 要搜索的 Elasticsearch 索引名称            |
| contentField | string | 必填   | - | 要查询的内容字段名称                         |
| semanticTextField | string | 必填   | - | 要查询的 embedding 字段名称                | 
| linkField | string | 选填   | - | 结果链接字段名称，当配置 `needReference` 时需要填写 |
| titleField | string | 选填   | - | 结果标题字段名称，当配置 `needReference` 时需要填写 |
| username | string | 选填   | - | Elasticsearch 用户名                  |
| password | string | 选填   | - | Elasticsearch 密码                   |

混合搜索中使用的 [Reciprocal Rank Fusion (RRF)](https://www.elastic.co/guide/en/elasticsearch/reference/8.17/rrf.html) 查询要求 Elasticsearch 的版本在 8.8 及以上。

目前文档向量化依赖于 Elasticsearch 的 Embedding 模型，该功能需要 Elasticsearch 企业版 License，或可使用 30 天的 Trial License。安装 Elasticsearch 内置 Embedding 模型的步骤可参考[该文档](https://www.elastic.co/docs/explore-analyze/machine-learning/nlp/ml-nlp-elser#alternative-download-deploy)；若需安装第三方 Embedding 模型，可参考[该文档](https://www.elastic.co/docs/explore-analyze/machine-learning/nlp/ml-nlp-text-emb-vector-search-example)。

有关 ai-search 插件集成 Elasticsearch 的完整教程，请参考：[使用 LangChain + Higress + Elasticsearch 构建 RAG 应用](https://cr7258.github.io/blogs/original/2025/15-rag-higress-es-langchain)。

## Quark 特定配置

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
|------|----------|----------|--------|------|
| contentMode | string | 选填 | "summary" | 内容模式："summary"使用摘要(snippet)，"full"使用正文(优先markdownText，为空则用mainText) |

## 配置示例

### 基础配置（单搜索引擎）

```yaml
needReference: true
searchFrom:
- type: google
  apiKey: "your-google-api-key"
  cx: "search-engine-id"
  serviceName: "google-svc.dns"
  servicePort: 443
  count: 5
  optionArgs:
    fileType: "pdf"
```

### Arxiv搜索配置

```yaml
searchFrom:
- type: arxiv
  serviceName: "arxiv-svc.dns" 
  servicePort: 443
  arxivCategory: "cs.AI"
  count: 10
```


### 夸克搜索配置

```yaml
searchFrom:
- type: quark
  serviceName: "quark-svc.dns" 
  servicePort: 443
  apiKey: "quark api key"
  contentMode: "full"  # 可选值："summary"(默认)或"full"
```

### 多搜索引擎配置

```yaml
defaultLang: "en-US"
promptTemplate: |
  # Search Results:
  {search_results}
  
  # Please answer this question: 
  {question}
searchFrom:
- type: google
  apiKey: "google-key"
  cx: "github-search-id"  # 专门搜索GitHub内容的搜索引擎ID
  serviceName: "google-svc.dns"
  servicePort: 443
- type: google
  apiKey: "google-key"
  cx: "news-search-id"    # 专门搜索Google News内容的搜索引擎ID 
  serviceName: "google-svc.dns"
  servicePort: 443
- type: bing
  apiKey: "bing-key"
  serviceName: "bing-svc.dns"
  servicePort: 443
  optionArgs:
    answerCount: "5"
```

### 并发查询配置

由于搜索引擎对单次查询返回结果数量有限制（如Google限制单次最多返回100条结果），可以通过以下方式获取更多结果：
1. 设置较小的count值（如10）
2. 通过start参数指定结果偏移量
3. 并发发起多个查询请求，每个请求的start值按count递增

例如，要获取30条结果，可以配置count=10，并发发起3个查询，每个查询的start值分别为0,10,20：

```yaml
searchFrom:
- type: google
  apiKey: "your-google-api-key"
  cx: "search-engine-id"
  serviceName: "google-svc.dns"
  servicePort: 443
  start: 0
  count: 10
- type: google
  apiKey: "your-google-api-key"
  cx: "search-engine-id"
  serviceName: "google-svc.dns"
  servicePort: 443
  start: 10
  count: 10
- type: google
  apiKey: "your-google-api-key"
  cx: "search-engine-id"
  serviceName: "google-svc.dns"
  servicePort: 443
  start: 20
  count: 10 
```

注意，过高的并发可能会导致限流，需要根据实际情况调整。

### Elasticsearch 配置（用于对接私有知识库）

```yaml
searchFrom:
- type: elasticsearch
  serviceName: "es-svc.static"
  index: "knowledge_base"
  contentField: "content"
  semanticTextField: "semantic_text"
  # username: "elastic"
  # password: "password"
```

### 自定义引用格式

```yaml
needReference: true
referenceFormat: "### 数据来源\n%s"
searchFrom:
- type: bing
  apiKey: "your-bing-key"
  serviceName: "search-service.dns"
  servicePort: 8080
```

### 自定义引用位置

```yaml
needReference: true
referenceLocation: "tail"  # 在回答结尾添加引用，而不是开头
searchFrom:
- type: bing
  apiKey: "your-bing-key"
  serviceName: "search-service.dns"
  servicePort: 8080
```

### 搜索重写配置

```yaml
searchFrom:
- type: google
  apiKey: "your-google-api-key"
  cx: "search-engine-id"
  serviceName: "google-svc.dns"
  servicePort: 443
searchRewrite:
  llmServiceName: "llm-svc.dns"
  llmServicePort: 443
  llmApiKey: "your-llm-api-key"
  llmUrl: "https://api.example.com/v1/chat/completions"
  llmModelName: "gpt-3.5-turbo"
  timeoutMillisecond: 15000
```

### 按需启用插件配置

配置插件仅在请求中包含`web_search_options`字段时才启用：

```yaml
defaultEnable: false
searchFrom:
- type: google
  apiKey: "your-google-api-key"
  cx: "search-engine-id"
  serviceName: "google-svc.dns"
  servicePort: 443
```

这种配置可以兼容OpenAI的搜索模型协议。当请求中包含`web_search_options`字段时，即使是空对象（`"web_search_options": {}`），插件也会被激活。

### 搜索上下文大小配置

通过在请求中的`web_search_options`字段中添加`search_context_size`参数，可以动态调整搜索查询次数：

```json
{
  "web_search_options": {
    "search_context_size": "medium"
  }
}
```

`search_context_size`支持三个级别：
- `low`: 生成1个搜索查询（适合简单问题）
- `medium`: 生成3个搜索查询（默认值）
- `high`: 生成5个搜索查询（适合复杂问题）

这个设置会覆盖配置中的`maxCount`值，允许客户端根据问题复杂度动态调整搜索深度。

## 注意事项

1. 提示词模版必须包含`{search_results}`和`{question}`占位符，可选使用`{cur_date}`插入当前日期（格式：2006年1月2日）
2. 默认模板包含搜索结果处理指引和回答规范，如无特殊需要可以直接用默认模板，否则请根据实际情况修改
3. 多个搜索引擎是并行查询，总超时时间 = 所有搜索引擎配置中最大timeoutMillisecond值 + 处理时间
4. Arxiv搜索不需要API密钥，但可以指定论文类别（arxivCategory）来缩小搜索范围
