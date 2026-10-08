<!--
Copyright (c) 2025 Alibaba Group Holding Ltd.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

# Content Security Mock Server

> 模拟阿里云内容安全（绿网）`TextModerationPlus` / `MultiModalGuard` 接口，专为 Higress [AI 安全防护插件](https://higress.cn/docs/latest/plugins/ai/ai-security-guard) e2e 测试设计。

监听地址：`:8090`。**不校验** ACS3-HMAC-SHA256 签名（仅检查 `Authorization` 头是否存在），便于 e2e 使用 mock AK/SK。

模拟的 API 版本为 `2022-03-02`（即插件请求中的 `x-acs-version`）。响应字段按插件 `config.Response` 的解析结构实现；若阿里云 API 或插件解析结构升级，需同步更新本 mock。

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
| `MASK` / `敏感` | `none` | `mask` | `sensitiveData` / `S4`，`Ext.Desensitization=[MASKED]`，`Ext.SensitiveData` |
| 其他 | `none` | `pass` | 无 |

### 异常场景

异常关键词优先于上面的风险判定（`THROTTLE` > `ERROR` > `TIMEOUT`）：

| 内容包含 | 响应 | 插件行为（TextModerationPlus / MultiModalGuard，请求与响应阶段一致） |
|----------|------|----------------------------------------------------------------------|
| `ERROR` | HTTP 200，`{"Code":500,"Message":"mock business error","RequestId":"..."}` | `Code != 200`，fail-open：放行原始请求/响应 |
| `THROTTLE` | HTTP 429，`{"Code":"Throttling.User","Message":"Request was denied due to user flow control.","RequestId":"..."}` | HTTP 状态码非 200，fail-open：放行 |
| `TIMEOUT` | 延迟 3s 后返回正常判定结果 | 超过插件默认 `timeout`（2000ms），回调收到 502，fail-open：放行 |

插件侧对应逻辑见 `lvwang/*/text/openai.go` 与 `lvwang/common/text/openai.go` 中 `statusCode != 200 || Code != 200` 分支；若测试配置了大于 3000ms 的 `timeout`，`TIMEOUT` 将不会触发超时。

## 插件配置要求

mock 只返回阿里云文档列出的等级（`high` / `none`；敏感等级 `S4` 为文本护栏文档中的最高等级），不会返回 `max`（`max` 只是插件配置项中“只检测不拦截”的阈值）。结合插件默认配置（`SetDefaultValues`）与 `EvaluateRisk` 的判定逻辑：

| action | 期望结果 | 默认配置下 | 需要的插件配置 |
|--------|----------|------------|----------------|
| `TextModerationPlus` | block | 拦截（`riskLevelBar` 默认 `high`） | 无 |
| `TextModerationPlus` | mask | 不支持（脱敏仅适用于 MultiModalGuard） | — |
| `MultiModalGuard` | block | 放行（`contentModerationLevelBar` 默认 `max`，即只检测不拦截） | `contentModerationLevelBar: high` |
| `MultiModalGuard` | mask | 拦截（`riskAction` 默认 `block`） | `riskAction: mask`（`sensitiveDataLevelBar` 默认 `S4` 即可） |

MultiModalGuard e2e 测试配置示例（仅列出与判定相关的字段）：

```yaml
action: MultiModalGuard
checkRequest: true
contentModerationLevelBar: high   # BLOCK / 违规 / illegal → 拦截
riskAction: mask                  # MASK / 敏感 → 脱敏
```

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
