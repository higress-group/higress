# Doris 数据库 MCP 插件

本插件为 Higress 提供 Doris 数据库的远程查询能力，支持通过 MCP 协议安全、灵活地访问 Doris 数据库。

## 功能简介
- 通过统一 API 查询 Doris 表数据
- 支持自定义 SQL 查询
- 支持参数校验、权限控制、限流、日志审计
- 适合 AI Agent、数据分析、自动化运维等场景

## 快速开始
1. 配置 `config/config.yaml`，填写 Doris 数据库连接信息。
2. 在 Higress 控制台注册本插件。
3. 通过 MCP API 远程调用数据库。

## API 用法

### 1. 查询表数据
- **接口名**：`queryTable`
- **参数**：
  - `table`：表名（必填）
  - `fields`：字段列表（可选，默认全部）
  - `where`：条件（可选）
  - `limit`：每页数量（可选，默认10）
  - `offset`：偏移量（可选，默认0）
- **返回**：
  - `data`：数据列表
  - `total`：总数
- **示例**：
```json
{
  "action": "queryTable",
  "params": {
    "table": "user",
    "fields": ["id", "name", "email"],
    "limit": 10,
    "offset": 0
  }
}
```

### 2. 执行自定义 SQL
- **接口名**：`executeSQL`
- **参数**：
  - `sql`：SQL 语句（必填）
  - `args`：参数数组（可选）
- **返回**：
  - `data`：查询结果或执行状态
  - `error`：错误信息（如有）
- **示例**：
```json
{
  "action": "executeSQL",
  "params": {
    "sql": "SELECT * FROM user WHERE id = ?",
    "args": [123]
  }
}
```

## 常见问题
- 插件如何保证安全？
  - 支持参数校验、SQL 白名单、权限配置，防止 SQL 注入和越权访问。
- 如何扩展更多数据库？
  - 可参考本插件结构，替换驱动和查询逻辑即可。

## 联系与支持
如有问题请在 Higress 社区或 GitHub 提 Issue。 