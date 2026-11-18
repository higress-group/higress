# log-request-response 插件

这个插件用于在 Higress 的访问日志中添加以下信息：

- HTTP 请求头（添加为 `%FILTER_STATE(wasm.log-request-headers:PLAIN)%`）
- POST、PUT、PATCH 请求的请求体内容（添加为 `%FILTER_STATE(wasm.log-request-body:PLAIN)%`）
- 响应头（添加为 `%FILTER_STATE(wasm.log-response-headers:PLAIN)%`）
- 响应体内容（添加为 `%FILTER_STATE(wasm.log-response-body:PLAIN)%`）

## 配置参数

在 Higress 控制台配置该插件时，使用以下结构化的 YAML 配置：

```yaml
# 状态码条件（可选，不配置时记录所有请求）
# 配置后，只有响应状态码满足条件时才会记录以下内容（请求头、请求体、响应头、响应体）
statusCode:
  # 比较操作符，支持 >=、>、=、==、!=、<、<=（默认：>=）
  operator: ">="
  # 目标状态码，取值范围 [100, 599]，例如 400 表示只在状态码 >= 400 时记录
  value: 400

# 请求相关配置
request:
  # 请求头配置
  headers:
    # 是否记录请求头（默认：false）
    enabled: true
  # 请求体配置
  body:
    # 是否记录请求体内容（默认：false）
    enabled: true
    # 最大记录长度限制，单位字节（默认：10KB）
    maxSize: 10240
    # 需要记录请求体的内容类型（默认包含常见的内容类型）
    contentTypes:
      - application/json
      - application/xml
      - application/x-www-form-urlencoded
      - text/plain

# 响应相关配置
response:
  # 响应头配置
  headers:
    # 是否记录响应头（默认：false）
    enabled: true
  # 响应体配置
  body:
    # 是否记录响应体内容（默认：false）
    enabled: true
    # 最大记录长度限制，单位字节（默认：10KB）
    maxSize: 10240
    # 需要记录响应体的内容类型（默认包含常见的内容类型）
    contentTypes:
      - application/json
      - application/xml
      - text/plain
      - text/html
```

## 工作原理

1. 请求处理时，插件会根据配置决定是否记录请求头和请求体
2. 只有当请求方法为 POST、PUT 或 PATCH，且内容类型在配置的 `request.body.contentTypes` 列表中时，才会记录请求体
3. 响应处理时，插件会根据配置决定是否记录响应头和响应体
4. 只有当响应的内容类型在配置的 `response.body.contentTypes` 列表中时，才会记录响应体
5. 所有记录的内容都会被限制在配置的 `maxSize` 指定的大小内
6. 插件对请求体和响应体都使用流式处理方式，不会阻止或修改原始内容传递
7. 记录的内容会被存储在 Envoy 的 Filter State 中，可以通过访问日志配置获取
8. 配置了 `statusCode` 条件时，由于 HTTP 状态码要等到响应阶段才可知，插件会改变记录时机：
   - 请求头和请求体会先暂存在当前请求的上下文中（请求体最多暂存 `maxSize` 字节），不会在请求阶段写入
   - 拿到响应状态码后判断是否命中条件：命中则把暂存的请求头、请求体连同响应头、响应体一起写入；未命中则全部丢弃，不产生任何日志
   - 网关自身生成的错误响应（如 404、429、503）也会进入响应处理流程，同样按条件判断后记录

## 编译方法

```bash
# 先整理依赖
go mod tidy

# 编译
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o ./main.wasm ./main.go
```

## 访问日志配置

要在 Higress 访问日志中显示插件添加的 Filter State 数据，需要修改 Higress 的访问日志配置。编辑 ConfigMap：

```bash
kubectl edit cm -n higress-system higress-config
```

在 `envoyAccessLogService.config.accessLog` 下的 `format` 字段中添加以下内容：

```json
{
  "request_headers": "%FILTER_STATE(wasm.log-request-headers:PLAIN)%",
  "request_body": "%FILTER_STATE(wasm.log-request-body:PLAIN)%",
  "response_headers": "%FILTER_STATE(wasm.log-response-headers:PLAIN)%",
  "response_body": "%FILTER_STATE(wasm.log-response-body:PLAIN)%"
}
```

完整的访问日志配置可能会像这样（添加到现有配置中）：

```yaml
mesh:
  accessLogFile: "/dev/stdout"
  accessLogFormat: |
    {
      "authority": "%REQ(:AUTHORITY)%",
      "bytes_received": "%BYTES_RECEIVED%",
      "bytes_sent": "%BYTES_SENT%",
      "downstream_local_address": "%DOWNSTREAM_LOCAL_ADDRESS%",
      "downstream_remote_address": "%DOWNSTREAM_REMOTE_ADDRESS%",
      "duration": "%DURATION%",
      "method": "%REQ(:METHOD)%",
      "path": "%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%",
      "protocol": "%PROTOCOL%",
      "request_id": "%REQ(X-REQUEST-ID)%",
      "requested_server_name": "%REQUESTED_SERVER_NAME%",
      "response_code": "%RESPONSE_CODE%",
      "response_flags": "%RESPONSE_FLAGS%",
      "route_name": "%ROUTE_NAME%",
      "start_time": "%START_TIME%",
      "trace_id": "%REQ(X-B3-TRACEID)%",
      "upstream_cluster": "%UPSTREAM_CLUSTER%",
      "upstream_host": "%UPSTREAM_HOST%",
      "upstream_local_address": "%UPSTREAM_LOCAL_ADDRESS%",
      "upstream_service_time": "%RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)%",
      "upstream_transport_failure_reason": "%UPSTREAM_TRANSPORT_FAILURE_REASON%",
      "user_agent": "%REQ(USER-AGENT)%",
      "x_forwarded_for": "%REQ(X-FORWARDED-FOR)%",
      "request_headers": "%FILTER_STATE(wasm.log-request-headers:PLAIN)%",
      "request_body": "%FILTER_STATE(wasm.log-request-body:PLAIN)%",
      "response_headers": "%FILTER_STATE(wasm.log-response-headers:PLAIN)%",
      "response_body": "%FILTER_STATE(wasm.log-response-body:PLAIN)%"
    }
```

## 日志输出示例

配置完成后，Higress 的访问日志中将包含这些额外的字段（取决于您的配置启用了哪些选项）：

```json
{
  "authority": "example.com",
  "method": "POST",
  "path": "/api/users",
  "response_code": 200,
  "request_headers": "{\"host\":\"example.com\",\"path\":\"/api/users\",\"method\":\"POST\",\"content-type\":\"application/json\"}",
  "request_body": "{\"name\":\"测试用户\",\"email\":\"test@example.com\"}",
  "response_headers": "{\"content-type\":\"application/json\",\"status\":\"200\"}",
  "response_body": "{\"id\":123,\"status\":\"success\"}"
}
```

## 注意事项

1. 所有日志记录选项默认都是关闭的（false），需要明确启用才会记录相应内容
2. 对于大型请求体或响应体，可以通过 `request.body.maxSize` 和 `response.body.maxSize` 参数限制记录的长度，以避免日志过大
3. 插件使用流式处理方式处理请求体和响应体，不会对原始内容产生任何影响
4. 只有指定内容类型的 POST、PUT、PATCH 请求才会记录请求体内容
5. 只有指定内容类型的响应才会记录响应体内容
6. 请确保合理配置该插件，避免记录敏感信息到日志中
7. `statusCode.value` 必须是 [100, 599] 范围内的 HTTP 状态码，`statusCode.operator` 必须是 `>=`、`>`、`=`、`==`、`!=`、`<`、`<=` 之一，配置非法值会导致插件加载失败
8. 配置了 `statusCode` 条件后请了解以下行为变化：
   - 如果请求最终没有得到响应（如连接被重置），暂存的请求头和请求体不会写入日志
   - HTTP/2 早响应（响应先于请求体传输完成到达）场景下，响应阶段会先写入已收到的请求体内容，请求体流结束时再以完整内容覆盖写入
   - 延迟暂存会让请求侧内容在内存中多保留一段时间（请求体受 `maxSize` 限制，默认 10KB；请求头以原始头形式暂存，JSON 序列化推迟到命中条件后才执行，未命中的请求不会产生序列化开销），对内存的影响可以忽略