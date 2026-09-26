# Content Security Mock Server

> 模拟阿里云内容安全（绿网）`TextModerationPlus` / `MultiModalGuard` 接口，专为 Higress [AI 安全防护插件](https://higress.cn/docs/latest/plugins/ai/ai-security-guard) e2e 测试设计。

监听地址：`:8090`。**不校验** ACS3-HMAC-SHA256 签名（仅检查 `Authorization` 头是否存在），便于 e2e 使用 mock AK/SK。

## 接口说明

### `GET /health`

健康检查，返回 `{"status":"ok"}`。

### `POST /`

模拟绿网文本审核接口。

**Query**

| 参数 | 说明 | 示例 |
|------|------|------|
| `Service` | 检测服务名 | `llm_query_moderation` / `llm_response_moderation` / `query_security_check` / `response_security_check` |

**Headers（与插件真实调用对齐，签名不校验）**

- `Authorization`：必填（缺失返回 HTTP 401）
- `x-acs-action`：如 `TextModerationPlus` / `MultiModalGuard`
- `x-acs-version`：如 `2022-03-02`
- `content-type`：`application/x-www-form-urlencoded`
- `User-Agent`：`CIPFrom/AIGateway`

**Body（form）**

| 字段 | 说明 |
|------|------|
| `ServiceParameters` | JSON 字符串，含 `content` / `sessionId` / `requestFrom` |

**响应**

始终返回 **HTTP 200**（绿网业务结果在 JSON 的 `Code` / `Data` 中），形状与 `ai-security-guard` 插件 `config.Response` 一致：

```json
{
  "Code": 200,
  "Message": "Success",
  "RequestId": "...",
  "Data": {
    "RiskLevel": "none|low|medium|high",
    "Suggestion": "pass|block|mask",
    "Advice": [{"Answer": "..."}],
    "Detail": [{"Type": "contentModeration|sensitiveData|...", "Level": "high|S3", "Suggestion": "block|mask|pass", "Result": []}]
  }
}
```

## 关键词风险规则

根据 `ServiceParameters.content` 模拟审核结果（block 优先于 mask）：

| 内容包含 | RiskLevel | Suggestion | Detail |
|----------|-----------|------------|--------|
| `BLOCK` / `违规` / `illegal`（大小写不敏感仅对 illegal） | `high` | `block` | `contentModeration` / `high`，含 Advice |
| `MASK` / `敏感` | `none` | `mask` | `sensitiveData` / `S3`，`Ext.Desensitization=[MASKED]` |
| 其他 | `none` | `pass` | 无 |

## 本地运行

```bash
go test ./...
go run .
# 或
docker build -t content-security-mock-server .
docker run --rm -p 8090:8090 content-security-mock-server
```

## 示例

```bash
# pass
curl -s -X POST 'http://127.0.0.1:8090/?Service=llm_query_moderation' \
  -H 'Authorization: ACS3-HMAC-SHA256 Credential=mock' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'ServiceParameters={"content":"hello","sessionId":"s1","requestFrom":"CIPFrom/AIGateway"}'

# block
curl -s -X POST 'http://127.0.0.1:8090/?Service=llm_query_moderation' \
  -H 'Authorization: ACS3-HMAC-SHA256 Credential=mock' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'ServiceParameters={"content":"BLOCK this","sessionId":"s1","requestFrom":"CIPFrom/AIGateway"}'

# mask
curl -s -X POST 'http://127.0.0.1:8090/?Service=llm_response_moderation' \
  -H 'Authorization: ACS3-HMAC-SHA256 Credential=mock' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'ServiceParameters={"content":"MASK phone","sessionId":"s1","requestFrom":"CIPFrom/AIGateway"}'
```
