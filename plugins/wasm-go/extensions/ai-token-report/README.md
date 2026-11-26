# AI Token Report Plugin

## 概述

AI Token Report 插件用于将 AI 服务的 token 使用情况上报到指定的报告 API。该插件独立于 ai-statistics 插件，专注于 token 使用数据的收集和上报。

## 功能特性

- ✅ **Token 使用统计**：自动从 AI 服务响应中提取 token 使用量
- ✅ **流式和非流式响应支持**：支持 SSE 流式和普通 HTTP 响应
- ✅ **异步上报**：非阻塞式上报，不影响主流程性能
- ✅ **配置化**：支持配置上报 API 地址、服务名、超时时间
- ✅ **错误处理**：内置 panic recovery，确保上报失败不会影响主流程
- ✅ **ID 生成**：优先使用有效 Envoy request ID（非空且不为 `-`），否则生成唯一 ID

## 配置说明

### 必需配置

| 配置项 | 类型 | 说明 | 默认值 |
|---------|------|------|---------|
| `reportApiUrl` | string | Token 上报 API 的完整 URL | - |

### 可选配置

| 配置项 | 类型 | 说明 | 默认值 |
|---------|------|------|---------|
| `reportServiceName` | string | 服务名，用于 DNS 解析 | `default` |
| `reportTimeout` | integer | 上报超时时间（毫秒） | `5000` |
| `source.headerName` | string | 请求头回调读取的普通来源 Header（合法 HTTP header token） | `x-wujie-source` |
| `source.allowedValues` | string[] | 最多 16 个精确来源值；不识别任意请求值 | `[]` |
| `source.fallbackValue` | string | 缺失或未命中的来源值 | `unknown` |

来源配置语义与 ai-route-monitor 一致，允许值和 fallback 匹配 `[A-Za-z0-9_-]{1,64}`。在 `defaultConfig` 或 `matchRules[].config` 配置；各规则独立解析，请填写完整的上报和 source 配置。不配置 source 的旧配置继续有效，现有上报增加 `productSource:"unknown"`。通用插件不内置任何产品允许值。

来源在请求头回调时保存于当前请求上下文，响应结束时写入原有上报 JSON，后续 path/host/source Header 改写不会改变快照；异步回调不再读取来源。当前 ai-proxy 请求处理未删除该来源头；实际部署仍需核对更早的自定义过滤器是否剥离它，插件无法恢复到达自身之前已被移除的头。

这只补计费来源字段，不增加上报次数，不改变 token 提取、金额、幂等 ID、发送时机、超时/重试或旧业务规则。模型请求成功率继续独立使用 ai-route-monitor，不能按有无用量报告判定成功。

### 配置示例

```json
{
  "reportApiUrl": "https://example.com/model/usage/report"
}
```

### 完整配置示例

```json
{
  "reportApiUrl": "https://example.com/model/usage/report",
  "reportServiceName": "wujieai",
  "reportTimeout": 5000,
  "source": {
    "headerName": "x-wujie-source",
    "allowedValues": ["wujieai"],
    "fallbackValue": "unknown"
  }
}
```

## 上报数据格式

### 请求格式

```json
{
  "requestId": "abc1234561710987654",
  "productSource": "wujieai",
  "model": "gpt-4",
  "userKey": "user_123",
  "inputToken": 1000,
  "outputToken": 500,
  "totalToken": 1500,
  "duration": 12500,
  "timestamp": 1710987654321
}
```

### 字段说明

| 字段名 | 类型 | 说明 |
|---------|------|------|
| `requestId` | string | 请求唯一 ID，优先使用有效 `request-id` header；header 被移除时使用 `wasm.requestId`；未提供时使用有效 Envoy `x-request-id`；均无效时格式为 `{random8位}{timestamp毫秒}` |
| `productSource` | string | 请求来源快照；新增 camelCase 字段，未命中为配置 fallback（默认 unknown），不按注册来源推断 |
| `model` | string | 模型名称（从请求体中提取） |
| `userKey` | string | 用户 Key（从 `X-Original-Api-Key` header 获取） |
| `inputToken` | integer | 输入 token 数 |
| `outputToken` | integer | 输出 token 数 |
| `totalToken` | integer | 总 token 数 |
| `duration` | integer | 请求耗时（毫秒） |
| `timestamp` | integer | 时间戳（毫秒） |

## 使用场景

### 1. 流式响应（SSE）

```
请求 → 获取响应 → 提取 token → 上报
     ↓
第一个 chunk → 记录首次 token 时间
     ↓
中间 chunks → 累积 token
     ↓
最后一个 chunk → 上报总 token 使用
```

### 2. 非流式响应

```
请求 → 获取响应 → 提取 token → 上报
```

## 构建和部署

### 构建插件

```bash
# 使用 Go 1.24 原生编译器构建（推荐）
./build.sh

# 或使用 TinyGo 构建
./build.sh tinygo
```

### 构建 Docker 镜像

```bash
docker build -t ai-token-report:1.0.0 .
```

### 推送到镜像仓库

```bash
docker tag ai-token-report:1.0.0 your-registry/ai-token-report:1.0.0
docker push your-registry/ai-token-report:1.0.0
```

### 在 Higress 中配置

1. 创建或更新 WasmPlugin 资源：

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: ai-token-report
spec:
  defaultConfig:
    disable: false
    reportApiUrl: "https://your-api.com/model/usage/report"
    reportServiceName: "wujieai"
    reportTimeout: 5000
  url: oci://your-registry/ai-token-report:1.0.0:latest
  phase: DONE
```

2. 应用到路由：

```yaml
apiVersion: networking.k8s.io/v1beta1
kind: Ingress
metadata:
  name: ai-ingress
spec:
  rules:
  - host: ai.example.com
    http:
      paths:
      - path: /v1/chat/completions
        backend:
          service:
            name: ai-service
  extensions:
  - name: ai-token-report
    config:
      disable: false
```

## 日志和调试

### 日志级别

插件会输出以下日志级别：

- **DEBUG**: 详细的处理流程
- **INFO**: 关键操作（如配置加载、上报成功）
- **WARN**: 警告信息（如配置缺失）
- **ERROR**: 错误信息（如上报失败）

### 关键日志示例

```
[INFO] ai-token-report: token usage report configured
[INFO]  reportApiUrl: https://example.com/model/usage/report
[INFO]  DnsCluster.ServiceName (服务名): wujieai
[DEBUG] ai-token-report: extracted model from request body: gpt-4
[DEBUG] ai-token-report: extracted token usage (streaming): model=gpt-4, input=1000, output=500, total=1500
[INFO] Reporting token usage to https://example.com/model/usage/report: requestId=abc1234561710987654
[DEBUG] Token usage report callback: status=200, requestId=abc1234561710987654
[INFO] Token usage report initiated successfully for requestId: abc1234561710987654
```

## 性能优化

- ✅ **异步上报**：上报请求不阻塞主流程
- ✅ **非阻塞调用**：使用 `reportClient.Post()` 非阻塞方式
- ✅ **超时控制**：默认 5 秒超时，避免长时间等待
- ✅ **错误隔离**：panic recovery 确保上报失败不影响主流程

## 注意事项

1. **兼容性**：
   - 适用于 OpenAI、Claude、Gemini 等主流 AI 服务
   - 支持流式和非流式响应

2. **性能影响**：
   - 上报是异步的，对主流程性能影响极小
   - 建议设置合理的超时时间（5 秒）

3. **配置要求**：
   - 必须配置 `reportApiUrl`
   - 确保 API 地址可访问

4. **监控建议**：
   - 监控上报成功率
   - 监控上报响应时间
   - 监控 API 可用性

5. **安全考虑**：
   - 使用 HTTPS 保护上报数据
   - 确保 userKey 不会泄露到日志中

## 故障排查

### 问题 1：上报失败

**症状**：日志显示 "Failed to dispatch token usage report call"

**可能原因**：
- API 地址配置错误
- 网络连接问题
- API 服务不可用

**解决方案**：
1. 检查 `reportApiUrl` 配置是否正确
2. 检查网络连接
3. 检查 API 服务状态

### 问题 2：Token 数据缺失

**症状**：日志显示 "Model not found in context"

**可能原因**：
- AI 服务响应格式不匹配
- Token 信息未正确提取

**解决方案**：
1. 检查 AI 服务响应格式
2. 检查 `tokenusage.GetTokenUsage` 是否正常工作

### 问题 3：内存持续增长

**症状**：插件内存持续增长

**可能原因**：
- Context 数据未正确清理
- Token 数据在 context 中累积

**解决方案**：
1. 检查是否有内存泄漏
2. 考虑配置 VM 重建阈值
3. 定期重启 Pod

## 版本历史

| 版本 | 日期 | 说明 |
|------|------|------|
| v1.0.0 | 2024-03-23 | 初始版本，支持 token 使用统计和上报 |

## 技术支持

如有问题，请提交 Issue 到 Higress 项目：
- GitHub: https://github.com/alibaba/higress
- 官方文档: https://higress.io/

## 团队用量上报

在迁移后的聊天和实际出图路由设置 `requireTeamId: true`，并将 `teamReportService`
设为已在 Higress 注册的 Open Platform 内部服务（例如 `onepiece-open-platform-internal.dns`，端口 8080）。
插件从同一请求的 `wasm.teamId` 读取已准入的内部团队 ID，向 `/internal/model/usage/team-report` 上报。
缺少团队归属或 HTTP 响应失败时跳过上报。Banana 使用出图路由配置的价格模型，成功返回图片且没有 token 时也会产生用量事件。
