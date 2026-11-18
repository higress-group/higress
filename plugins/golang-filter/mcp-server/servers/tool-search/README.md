# Tool Search MCP Server

这是一个基于 Higress Golang Filter 实现的 MCP Server，用于提供工具语义搜索功能。当前实现仅支持向量语义搜索，后端可以是 Milvus 或 Qdrant，不包含全文检索或混合搜索。

## 功能特性

- **向量语义搜索**：使用 OpenAI 兼容的 Embedding API 将用户查询转换为向量，并在 Milvus 或 Qdrant 中进行相似度检索
- **工具元数据支持**：从数据库中读取完整的工具定义（JSON 格式），并动态拼接工具名称
- **全量工具列表**：支持获取数据库中所有可用工具
- **可配置 Embedding 模型**：支持自定义模型、维度及 API 端点（如 DashScope）
- **向量库**：Milvus 或 Qdrant，二选一即可

## 数据库要求

集合需预先创建，字段如下：

| 字段名          | 类型                | 说明                      |
|--------------|-------------------|-------------------------|
| `id`         | VarChar(64)           | 文档唯一 ID                 |
| `content`    | VarChar(64)           | 工具描述文本                  |
| `metadata`   | JSON              | 完整的工具定义（必须包含 `name` 字段） |
| `vector`     | FloatVector(1024) | embedding 向量            |
| `metadata`   | Int64             | 创建时间                    |


## 配置参数

### 根级配置

| 参数         | 类型   | 必填 | 默认值                                              | 说明 |
|--------------|--------|------|-----------------------------------------------------|------|
| `vector`     | object | 是   | -                                                   | 向量数据库配置（见下文） |
| `embedding`  | object | 是   | -                                                   | Embedding API 配置（见下文） |
| `description`| string | 否   | `"Tool search server for semantic similarity search"` | MCP Server 描述信息 |

### Vector 配置（`vector` 对象）

| 参数        | 类型   | 必填 | 默认值             | 说明 |
|-------------|--------|------|--------------------|------|
| `type`      | string | 是   | -                  | `milvus` 或 `qdrant` |
| `host`      | string | 是   | -                  | 服务地址（如 `localhost`） |
| `port`      | int    | 是   | -                  | Milvus 默认 `19530`，Qdrant gRPC 默认 `6334` |
| `database`  | string | 否   | `"default"`        | Milvus 数据库名。Qdrant 忽略 |
| `tableName` | string | 否   | `"apig_mcp_tools"` | 集合名 |
| `username`  | string | 否   | -                  | Milvus 用户名。Qdrant 忽略 |
| `password`  | string | 否   | -                  | Milvus 密码，或 Qdrant API key |

### Embedding 配置（`embedding` 对象）

| 参数         | 类型   | 必填 | 默认值                                                    | 说明 |
|--------------|--------|------|-----------------------------------------------------------|------|
| `apiKey`     | string | 是   | -                                                         | Embedding 服务的 API Key |
| `baseURL`    | string | 否   | `https://dashscope.aliyuncs.com/compatible-mode/v1`       | OpenAI 兼容 API 的 Base URL |
| `model`      | string | 否   | `text-embedding-v4`                                       | 使用的 Embedding 模型 |
| `dimensions` | int    | 否   | `1024`                                                    | 向量维度 |

## 配置示例


Tool Search MCP Server 也可以作为 Higress 的一个模块进行配置。以下是一个在 Higress ConfigMap 中配置 Tool Search 的示例：

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: higress-config
  namespace: higress-system
data:
  higress: |
    mcpServer:
      enable: true
      sse_path_suffix: "/sse"
      redis:
        address: "<Redis IP>:6379"
        username: ""
        password: ""
        db: 0
      match_list:
      - path_rewrite_prefix: ""
        upstream_type: ""
        enable_path_rewrite: false
        match_rule_domain: "*"
        match_rule_path: "/mcp-servers/tool-search"
        match_rule_type: "prefix"
      servers:
      - path: "/mcp-servers/tool-search"
        name: "tool-search"
        type: "tool-search"
        config:
          vector:
            type: "milvus"
            host: "localhost"
            port: 19530
            database: "default"
            tableName: "apig_mcp_tools"
            username: "root"
            password: "Milvus"
            maxTools: 1000
          embedding:
            apiKey: "your-dashscope-api-key"
            baseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"
            model: "text-embedding-v4"
            dimensions: 1024
          description: "Higress 工具语义搜索服务"
```

Qdrant 把 `vector` 改成：

```yaml
          vector:
            type: "qdrant"
            host: "localhost"
            port: 6334
            tableName: "apig_mcp_tools"
            password: ""
```

Qdrant 点 ID 必须是 UUID 或无符号整数。`database` 和 `username` 无效。

## 工具搜索接口

Tool Search MCP Server 提供以下 MCP 工具：

### x_higress_tool_search

基于语义相似度搜索最相关的工具。

**输入参数**:

| 参数名  | 类型   | 必填 | 说明 |
|---------|--------|------|------|
| `query` | string | 是   | 查询语句，用于与工具描述进行语义相似度比较 |
| `topK`  | int    | 否   | 指定需要选择的工具数量，默认选择前10个工具 |

**输出格式**:

```
{
  "tools": [
    {
      "name": "server_name___tool_name",
      "title": "Tool Title", 
      "description": "Tool description",
      "inputSchema": {...},
      "outputSchema": {...}
    }
  ]
}
```


## 搜索实现

通过向量相似度进行搜索，索引配置如下
- 使用 HNSW 索引算法进行向量索引
- 默认参数：M=8, efConstruction=64
- 相似度度量方式：内积（IP）