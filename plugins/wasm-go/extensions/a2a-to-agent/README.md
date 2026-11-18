# A2A to Agent API

`a2a-to-agent` 是独立的协议转换插件：在 Higress 路由上暴露 **A2A 1.0 JSON-RPC Server**，把文本消息转换为后端 Agent API，把执行进度与结果转换为 A2A Task、Artifact 和 Status Update。它不依赖 `a2a-protocol`，也不要求后端原生支持 A2A。原生 API 可用另一条不挂载本插件的路由访问。

## 支持范围

| provider | 后端 | SendMessage | SendStreamingMessage | contextId 续会话 |
|---|---|---|---|---|
| `dify` | Dify Chat / Agent `/v1/chat-messages` | 支持 | 支持 | 支持 |
| `bailian` | 百炼应用，包含 Agent 2.0 `/api/v1/apps/{appId}/completion` | 支持 | 支持 | 支持 |
| `bailian-managed` | 百炼 Managed Agents `/api/v1/agentstudio` sessions/events API | 拒绝 | 支持 | 首版拒绝 |
| `coze` | 扣子 Agent `/v3/chat` | 支持 | 支持 | 支持 |
| `qoder` | Qoder Cloud Agent sessions/events API | 拒绝 | 支持 | 首版拒绝 |
| `claude-managed` | Claude Managed Agents (CMA) sessions/events API | 拒绝 | 支持 | 首版拒绝 |

Dify、百炼应用和 Coze 后端始终使用 SSE；`SendMessage` 在网关有界聚合，因此也适用于不支持原生 blocking 的 Dify Agent 和异步 Coze Chat。Cloud Agent 创建 session 后先建立 SSE，再提交用户消息，避免 tail-only SSE 漏掉快速输出。CMA 使用 Managed Agents API，**不是** Claude Messages API。百炼 `bailian` 与 `bailian-managed` 是两种独立协议；后者使用 `input` 中的 `role:user/type:message`，不同于 Qoder/CMA 的 `events/user.message`，不能互换 preset。

仅支持 `ROLE_USER` 和文本 Part。文件、结构化输入、任务查询/取消/订阅恢复、推送通知、`returnImmediately:true`、外部工具结果提交、通用自定义 schema 均不在首版范围；这些请求会明确拒绝。Cloud Agent 每次请求创建独立 session，不接受传入 contextId；响应中的 contextId 仅用于关联。可靠的跨副本同会话并发协调、重连和历史补偿需要后续实现。

## 配置

将插件挂在专用 A2A 路由上。该路由还需覆盖 `/.well-known/agent-card.json`，或给 discovery 路由复用相同配置。其他 POST 路径只要匹配此插件都会按 A2A 请求处理；Agent Card 的 URL 必须填写实际外部路由。

```yaml
provider: dify
apiKey: <Dify application API key>
apiPath: /v1/chat-messages
consumerHeader: x-agent-consumer
contextSecret: <at-least-32-random-bytes>
inputs: {}
agentCard:
  name: My Agent
  description: Answers questions using the configured agent
  url: https://agent.example.com/a2a
  version: 1.0.0
  skills:
  - id: chat
    name: Chat
    description: Ask the agent a question
    tags: [chat]
```

| 配置 | 说明 |
|---|---|
| `provider` | 上表中的精确标识，必填 |
| `apiKey` | 后端凭证，必填；由网关管理，不接受客户端覆盖 |
| `agentId` | 百炼 app ID / Coze bot ID / Cloud Agent ID；对应 provider 必填 |
| `apiPath` | Dify、百炼应用和 Coze 的单 POST 完整路径，默认见上表 |
| `inputs` | Dify 固定应用输入，默认 `{}`；不会透传客户端任意 metadata |
| `consumerHeader` | 可信认证插件注入的稳定用户 ID 请求头，必填 |
| `contextSecret` | 至少 32 字节的随机 HMAC 密钥，必填；各副本配置一致 |
| `contextTTLSeconds` | 签名 contextId 有效期，默认 86400 秒 |
| `maxBodyBytes` | 请求体、单 SSE frame、最终文本及 blocking 响应上限，默认 1048576，范围 1024–16777216；单流累计上游数据另限制为此值的 16 倍 |
| `agentCard` | 至少包含 `name`、`url`；可配置 description、version、skills、securitySchemes/securityRequirements；插件生成 1.0 JSONRPC interface、streaming capability 与文本输入输出模式 |
| `upstreamAuthority` | 可选，重写主路由上游 authority；Cloud 必填，需与路由指向同一服务 |
| `upstreamCluster` | Cloud 必填，Envoy 已配置的目标集群名，供 session/message 子请求使用 |
| `upstreamScheme` | Cloud 子请求 scheme，默认 `https`；本地 demo 可为 `http` |
| `apiBasePath` | Cloud 路径前缀；Qoder 默认 `/api/v1/cloud`，CMA 默认 `/v1`，百炼 Managed 默认 `/api/v1/agentstudio` |
| `sessionConfig` | Cloud 固定 session 创建参数，必须提供 `environment_id`；`agent` 由 `agentId` 覆盖 |

`consumerHeader` 必须由前置认证插件先删除客户端同名请求头，再注入经过认证的身份。直接信任公网客户端自报该请求头不能提供租户隔离。HMAC contextId 绑定 provider、应用、API endpoint、API key 与消费者，并有过期时间；裸后端 session ID、伪造/跨消费者/跨应用 token 都会被拒绝。Token 是签名而非加密，不能用来隐藏后端 session ID。API key 或签名密钥轮换会使旧 contextId 失效。

Qoder 配置示例（主路由的 upstream 也必须指向相同主机）：

```yaml
provider: qoder
apiKey: <PAT-or-SAT>
agentId: agent_123
sessionConfig:
  environment_id: env_123
upstreamCluster: outbound|443||api.qoder.com
upstreamAuthority: api.qoder.com
upstreamScheme: https
apiBasePath: /api/v1/cloud
consumerHeader: x-agent-consumer
contextSecret: <at-least-32-random-bytes>
agentCard:
  name: Qoder Agent
  description: Coding agent; SendStreamingMessage only
  url: https://agent.example.com/a2a
```

中国地域可以使用 `api.qoder.com.cn`。CMA 设置 `provider: claude-managed`、`upstreamAuthority: api.anthropic.com`、相应 Envoy cluster、`apiBasePath: /v1`；插件会设置 `x-api-key`、`anthropic-version: 2023-06-01` 和 `anthropic-beta: managed-agents-2026-04-01`。`sessionConfig` 仅允许管理员配置；它不是通用的请求 schema。

百炼 Managed 设置 `provider: bailian-managed`，使用对应业务空间的 API Key、Agent ID 和 `sessionConfig.environment_id`，`upstreamAuthority` 与 `upstreamCluster` 指向该空间的实际 Managed API Endpoint；`apiBasePath` 默认 `/api/v1/agentstudio`。主路由和 session/message 子请求必须使用同一服务来源与 TLS 设置。插件使用 Bearer 鉴权，订阅 `event_deltas[]=message`。这里不使用百炼 completion 应用 ID，也不设置 `x-dashscope-sse`。

## 调用与结果

```json
{
  "jsonrpc": "2.0",
  "id": "request-1",
  "method": "SendStreamingMessage",
  "params": {
    "message": {
      "messageId": "message-1",
      "role": "ROLE_USER",
      "parts": [{"text": "Explain this project"}]
    }
  }
}
```

流输出使用 A2A 1.0 的 `result.task`、`result.artifactUpdate` 和 `result.statusUpdate` oneof，不使用旧版 `kind`、`final`、`message/send`。文本增量为同一 answer artifact 的 append，快照修正为 replace。Dify thoughts、百炼 thoughts、Coze 非答案事件、Cloud 工具和执行状态作为 statusUpdate.metadata 的 provider/event/data 暴露；Cloud 的 `agent.thinking` 仅表示执行进度，不捏造思考内容。Cloud delta preview 会与最终 buffered message 按 event ID 核对，补齐缺失尾部。

Dify `message_end`、百炼 `finish_reason:stop`、Coze `conversation.chat.completed`、Qoder/CMA 根 session 的 `session.status_idle` + `stop_reason.type:end_turn` 才表示完成。百炼 Managed 从 `session_status` 事件的 `content[].data` 读取 `session_status:idle` 与 `stop_reason.type:end_turn`；`requires_action` 表示等待外部操作，`retries_exhausted` 或会话终止表示失败。子线程 idle 不结束任务。外部操作需求映射 `TASK_STATE_INPUT_REQUIRED`；异常结束、非法事件、超限、没有终态的 EOF 映射 `TASK_STATE_FAILED`。Qoder/CMA 可恢复 `session.error` 和百炼 Managed 的运行期 `error` 作为进度暴露，等待后续终态。百炼 Managed 的 `tool_call`、`tool_call_output`、MCP、模型请求等事件保留为 provider metadata，用户消息回显不会输出为答案；根据新会话用户回显识别根线程，过滤显式其他线程和 session。预览省略线程信息时以最终快照核对，若快照属于子线程会撤回相应预览文本。

Cloud 上游 SSE 是长连接。插件在 A2A 终态使用 Higress `InjectEncodedDataToFilterChain(..., true)` 扩展结束下游响应流，无须等待厂商 EOF；因此需要支持此扩展的 Higress gateway。关闭 HTTP 流不等于取消厂商任务；本插件不宣称支持取消或清理云 session。网关路由应设置适合 Agent 执行时长的超时。

非 Cloud 的下一条消息可以把返回的 contextId 放在 `params.message.contextId`，继续后端会话；不要把 taskId 带入新消息。不同请求生成不同 A2A taskId。失败前已产生的文本不被改写成成功结果。

## 构建与验证

```sh
go test -count=1 ./...
go vet ./...
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o /tmp/a2a-to-agent.wasm .
```

`VERSION` 为 `0.1.0-alpha`，可进入现有 wasm-go batch builder。WASM 文件不提交。单元测试覆盖请求转换、租户 token 隔离、SSE 任意分块、CRLF、多行、错误与截断、末尾成功条件、工具等待、Cloud 子线程/其他 session 过滤以及 preview/full-message 合并。协议 fixture 的集群验证和真实厂商凭证联调是不同证据：没有厂商凭证的测试不能声称已完成厂商线上验证。

协议参考：[A2A 1.0](https://a2a-protocol.org/v1.0.0/specification/)、[Dify Chat API](https://docs.dify.ai/api-reference/chat/send-chat-message)、[百炼应用](https://help.aliyun.com/zh/model-studio/agent-and-workflow-application-api-reference)、[百炼 Managed SSE](https://help.aliyun.com/zh/model-studio/event-sse-stream)、[百炼 Managed Session](https://help.aliyun.com/zh/model-studio/session-create)、[Coze 官方 SDK](https://github.com/coze-dev/coze-py/blob/main/cozepy/chat/__init__.py)、[Qoder sessions](https://docs.qoder.com/cloud-agents/api/sessions/create)、[Qoder SSE tail 行为](https://docs.qoder.com/cloud-agents/sse-initial-connection-behavior-change)、[Claude Managed Agents](https://platform.claude.com/docs/en/managed-agents/reference)。
