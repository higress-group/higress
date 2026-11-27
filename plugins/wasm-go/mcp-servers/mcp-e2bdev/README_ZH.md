# E2BDev MCP Server

基于 E2B API 的 MCP 服务器实现，提供沙盒生命周期管理功能。本服务器仅提供 `create_sandbox` 和 `kill_sandbox` 两个工具，不支持代码执行。

## 使用教程

### 获取 API-KEY
1. 注册E2B账号 [注册入口](https://e2b.dev/auth/sign-up)，每位新用户有$100的免费额度。
2. 在DashBoard中生成 API Key [生成 API Key](https://e2b.dev/dashboard?tab=keys)

### 配置 MCP Client

在用户的 MCP Client 界面，添加 E2BDev MCP Server 配置。

```json
"mcpServers": {
    "e2bdev": {
      "url": "https://mcp.higress.ai/mcp-e2bdev/{generate_key}",
    }
}
```

### 工具使用

本服务器仅提供以下沙盒生命周期工具：

- **create_sandbox**: 创建E2B沙盒环境
  - 参数:
    - timeout: 沙盒超时时间（秒），超时后沙盒将被终止
  - 返回: 沙盒ID

- **kill_sandbox**: 终止沙盒环境
  - 参数:
    - sandbox_id: 要终止的沙盒ID
  - 返回: 终止结果
