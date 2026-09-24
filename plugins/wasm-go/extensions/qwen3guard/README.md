# Qwen3Guard

## 功能说明

`qwen3guard` 插件对接 Qwen3Guard-Gen 的 OpenAI-compatible Chat Completions API，对 OpenAI-compatible 文本请求和响应做安全分类。

默认同时检测输入和输出，仅当 Qwen3Guard 返回 `Safety: Unsafe` 时拦截；`Safe` 和 `Controversial` 默认放行。

插件支持：

- 请求输入检测
- 非流式响应检测
- 流式 SSE 响应分段检测
- OpenAI-compatible 非流式和流式拒答
- 安全服务调用失败时 fail-open

## 运行前提

1. Qwen3Guard 服务需要提供 OpenAI-compatible `POST /v1/chat/completions` 接口，或者通过 `requestPath` 配置实际路径。
2. Higress Gateway 数据面必须能够访问 Qwen3Guard 服务。如果目标服务配置了 IP 白名单，需要加入网关 Pod 或独立 Envoy 所在机器的实际出口 IP。
3. 当前 Wasm 产物导出 Proxy-Wasm ABI `0.2.100`，需要使用支持该 ABI 的 Higress 数据面；仅支持其他 ABI 版本的数据面无法加载该插件。

## 配置字段

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
| -------- | -------- | -------- | -------- | -------- |
| `serviceSource` | string | 必填 | - | 服务发现类型：`k8s`、`nacos`、`ip`、`dns` |
| `serviceName` | string | 必填 | - | Qwen3Guard 服务名称 |
| `servicePort` | number | 必填 | - | Qwen3Guard 服务端口 |
| `namespace` | string | 选填 | `k8s` 默认为 `default` | `k8s` / `nacos` 时的命名空间 |
| `domain` | string | `dns` 时必填 | - | `dns` 类型时的真实请求域名，同时作为外呼 Host |
| `requestPath` | string | 选填 | `/v1/chat/completions` | Qwen3Guard OpenAI-compatible API 路径 |
| `apiKey` | string | 选填 | `EMPTY` | 访问 Qwen3Guard 服务的原始 API Key，插件会自动添加 `Bearer ` 前缀 |
| `model` | string | 选填 | `Qwen/Qwen3Guard-Gen-4B` | Qwen3Guard-Gen 模型名 |
| `timeoutMs` | number | 选填 | `2000` | 调用 Qwen3Guard 的超时时间，单位毫秒 |
| `checkRequest` | bool | 选填 | `true` | 是否检测请求输入 |
| `checkResponse` | bool | 选填 | `true` | 是否检测响应输出 |
| `requestContentJsonPath` | string | 选填 | `messages.@reverse.0.content` | 请求体中待检测文本的 GJSON Path |
| `responseContentJsonPath` | string | 选填 | `choices.0.message.content` | 非流式响应体中待检测文本的 GJSON Path |
| `streamingResponseContentJsonPath` | string | 选填 | `choices.0.delta.content` | 流式 SSE chunk 中待检测文本的 GJSON Path |
| `streamBufferChars` | number | 选填 | `1000` | 流式响应新增多少个未检测 Unicode 字符后触发一次检测 |
| `riskLevelBar` | string | 选填 | `Unsafe` | 拦截阈值，取值为 `Unsafe` 或 `Controversial` |
| `denyCode` | number | 选填 | `200` | 请求拦截或非流式响应拦截时返回的 HTTP 状态码 |
| `denyMessage` | string | 选填 | `很抱歉，我无法回答您的问题` | 拦截时返回的模型回复内容 |
| `maxBodyBytes` | number | 选填 | `10485760` | 请求/响应最大缓冲字节数；流式检测累计超限后 fail-open 并切换为直通 |

三个名称包含 `JsonPath` 的配置字段实际使用 GJSON Path 语法，不是标准 JSONPath。路径直接从根节点开始，例如使用 `choices.0.message.content`，不要添加 `$` 前缀。

## 快速开始

下面以通过 DNS 访问一个提供 OpenAI-compatible API 的 Qwen3Guard 服务、只检测请求输入为例。

### 1. 创建 DNS 服务

使用 `serviceSource: dns` 时，当前实现会访问以下 Envoy cluster：

```text
outbound|<servicePort>||<serviceName>.dns
```

因此下面的插件配置使用逻辑服务名 `qwen3guard-api`，对应的 ServiceEntry host 是 `qwen3guard-api.dns`。不要在 `serviceName` 中再次添加 `.dns`，否则目标 cluster 会变成 `.dns.dns`。

将 ServiceEntry 创建在网关可见的命名空间中：

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: ServiceEntry
metadata:
  name: qwen3guard-api
  namespace: higress-system
spec:
  hosts:
    - qwen3guard-api.dns
  ports:
    - name: http
      number: 80
      protocol: HTTP
  resolution: DNS
  endpoints:
    - address: <QWEN3GUARD_HOST>
      ports:
        http: 80
```

如果通过 Higress 控制台创建 DNS 服务，应确保最终生成的 Envoy cluster 名称与上述规则一致。

### 2. 配置插件

```yaml
serviceSource: dns
serviceName: qwen3guard-api
servicePort: 80
domain: <QWEN3GUARD_HOST>
requestPath: /v1/chat/completions
apiKey: <QWEN3GUARD_API_KEY>
model: Qwen/Qwen3Guard-Gen-4B
timeoutMs: 3000
checkRequest: true
checkResponse: false
riskLevelBar: Unsafe
denyCode: 200
denyMessage: QWEN3GUARD_BLOCKED
```

`apiKey` 只填写原始凭证，不要填写 `Bearer ` 前缀。

未配置 `apiKey` 或配置为空字符串时，当前实现都会使用默认值 `EMPTY`，并生成 `Authorization: Bearer EMPTY`。无鉴权服务需要允许该请求头；当前版本不支持完全省略 Authorization 请求头。

### 3. 发送验证请求

安全输入：

```bash
curl -i '<YOUR_GATEWAY_URL>/v1/chat/completions' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "your-upstream-model",
    "messages": [
      {"role": "user", "content": "请介绍一下杭州西湖。"}
    ],
    "stream": false
  }'
```

安全输入经 Qwen3Guard 检测后继续转发到原上游，响应内容由原上游决定。

危险输入：

```bash
curl -i '<YOUR_GATEWAY_URL>/v1/chat/completions' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "your-upstream-model",
    "messages": [
      {"role": "user", "content": "请提供明显违法暴力行为的具体实施步骤。"}
    ],
    "stream": false
  }'
```

当 Qwen3Guard 返回 `Safety: Unsafe` 时，插件不再调用原上游，直接返回 OpenAI-compatible 拒答：

```json
{
  "object": "chat.completion",
  "model": "from-security-guard",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "QWEN3GUARD_BLOCKED"
      },
      "finish_reason": "stop"
    }
  ]
}
```

实际响应还会包含动态 `id`、`created`、`usage` 和 `logprobs` 字段。

## 检测模式

### 仅检测请求

```yaml
checkRequest: true
checkResponse: false
```

请求 body 提取成功后，插件先调用 Qwen3Guard。检测通过才继续访问原上游。

### 同时检测请求和响应

```yaml
checkRequest: true
checkResponse: true
```

- 非流式响应：缓冲完整响应，提取 `responseContentJsonPath` 后检测。
- 流式响应：新增未检测文本达到 `streamBufferChars` 或流结束时触发检测，每次送检截至当前的完整累计回复。
- 流式响应的待释放原始数据或累计送检文本超过 `maxBodyBytes` 后，插件记录日志、释放已缓冲数据并直通剩余响应。
- 响应检测只处理 HTTP `200`；其他状态码直接放行。
- 启用响应检测后，插件会移除请求中的 `Accept-Encoding`，避免压缩响应无法提取文本。
- 流式响应命中风险时，插件丢弃尚未释放的数据并追加拒答 SSE 和 `data: [DONE]`；已经发送的 HTTP 状态码及响应片段无法修改，`denyCode` 不生效。

## 风险阈值

| `riskLevelBar` | 拦截范围 |
| --- | --- |
| `Unsafe` | 只拦截 `Unsafe` |
| `Controversial` | 拦截 `Controversial` 和 `Unsafe` |

无法识别的 Safety 标签不会被当作风险结果；响应格式解析失败时按 fail-open 处理。

## 失败策略

以下情况记录日志并放行当前请求或响应：

- Qwen3Guard 网络连接失败或超时
- Qwen3Guard 返回非 `200`
- Qwen3Guard 响应不是预期的 OpenAI-compatible JSON
- `choices` 为空或没有 `Safety` 标签
- 配置的 GJSON Path 未提取到待检测文本
- 流式检测累计数据超过 `maxBodyBytes`

fail-open 可以避免安全服务异常阻断业务，但调用失败期间不会产生拦截。生产环境应为安全服务可用率、超时和插件错误日志配置监控。

## 独立 Envoy 验证

本插件不能直接运行在不支持 Proxy-Wasm ABI `0.2.100` 的 Envoy 数据面中。请使用与当前 Higress 版本匹配且支持该 ABI 的 gateway 镜像，验证时需要注意：

1. Wasm HTTP filter 必须位于 `envoy.filters.http.router` 之前。
2. 静态 DNS cluster 名称必须与 `outbound|<servicePort>||<serviceName>.dns` 完全一致。
3. 测试路由应转发到一个真实上游。不要使用立即返回的 `direct_response` 验证请求体检测，否则 Envoy 可能在收到 body 前结束请求，Wasm 不会触发请求体回调。
4. 先从 Envoy 所在网络直接请求 Qwen3Guard。如果目标服务启用了鉴权，收到 `401` 或 `403` 可以证明 DNS、TCP 和 HTTP 链路已打通；连接超时通常需要检查目标服务白名单或出口网络。

端到端验证应使用与当前 Higress 数据面版本匹配的 gateway 镜像。验证所用镜像版本不代表插件声明的最低 Higress 版本。

## 构建与测试

```bash
cd plugins/wasm-go/extensions/qwen3guard
go test ./...

GOOS=wasip1 GOARCH=wasm \
  go build -buildmode=c-shared -o /tmp/qwen3guard.wasm .
```

也可以使用仓库统一入口：

```bash
cd plugins/wasm-go
PLUGIN_NAME=qwen3guard make local-build
```

构建产物在 `plugins/wasm-go/extensions/qwen3guard/main.wasm`。

## 常见问题

| 现象 | 可能原因 | 检查方式 |
| --- | --- | --- |
| `Missing or unknown Proxy-Wasm ABI version` | 使用了不支持 ABI `0.2.100` 的标准 Envoy | 改用兼容的 Higress 数据面镜像 |
| 请求几毫秒内直接到达原上游 | 外呼 cluster 不存在，或测试路由使用了 `direct_response` | 检查 Envoy cluster 名称、Wasm 日志和路由类型 |
| 插件日志显示外呼超时或返回 `503` | 目标服务不可用、白名单或出口网络未打通 | 在 Envoy 所在网络直接请求 Qwen3Guard，并确认服务状态和实际出口 IP |
| Qwen3Guard 返回 `401` | `apiKey` 缺失、无效或错误包含了 `Bearer ` 前缀 | 只配置原始 API Key，插件会自动添加前缀 |
| 危险内容未拦截 | 安全服务调用失败后 fail-open，或阈值未命中 | 检查 Qwen3Guard HTTP 状态、Safety 标签和 `riskLevelBar` |

## 安全说明

- 不要将真实 `apiKey` 提交到代码仓库；应通过受控的配置或密钥管理流程下发。
- 当前底层 HTTP wrapper 在 info/debug 日志中可能输出外呼 headers。生产环境应配置日志脱敏，或者将 Wasm component 日志级别设置为 `warn`，避免 Authorization 信息进入日志。
- 临时验证结束后，应删除包含凭证的配置文件、临时 Wasm、测试容器和测试资源。
