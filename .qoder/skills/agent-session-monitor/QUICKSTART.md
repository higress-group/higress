# Agent Session Monitor - Quick Start

实时Agent对话观测程序，用于监控Higress访问日志，追踪多轮对话的token开销和模型使用情况。

## 快速开始

### 1. 运行Demo

```bash
cd example
bash demo.sh
```

这将：
- 解析示例日志文件
- 列出所有session
- 显示session详细信息（包括完整的messages、question、answer、reasoning、tool_calls）
- 按模型和日期统计token开销
- 导出FinOps报表

### 2. 启动Web界面（推荐）

```bash
# 先解析日志生成session数据
python3 main.py --log-path /var/log/higress/access.log --output-dir ./sessions

# 启动Web服务器
python3 scripts/webserver.py --data-dir ./sessions --port 8888

# 浏览器访问
open http://localhost:8888
```

Web界面功能：
- 📊 总览所有session，按模型分组统计
- 🔍 点击session ID下钻查看完整对话
- 💬 查看每轮的messages、question、answer、reasoning、tool_calls
- 💰 实时计算token开销和成本
- 🔄 每30秒自动刷新

### 3. 在Clawdbot对话中使用

当用户询问当前会话token消耗时，生成观测链接：

```
你的当前会话ID: agent:main:discord:channel:1465367993012981988

查看详情：http://localhost:8888/session?id=agent:main:discord:channel:1465367993012981988

点击可以看到：
✅ 完整对话历史（每轮messages）
✅ Token消耗明细
✅ 工具调用记录
✅ 成本统计
```

### 4. 使用CLI查询（可选）

```bash
# 查看session详细信息
python3 scripts/cli.py show <session-id>

# 列出所有session
python3 scripts/cli.py list

# 按模型统计
python3 scripts/cli.py stats-model

# 导出报表
python3 scripts/cli.py export finops-report.json
```

## 核心功能

✅ **完整对话追踪**：记录每轮对话的完整messages、question、answer、reasoning、tool_calls  
✅ **Token开销统计**：区分input/output/reasoning/cached token，实时计算成本  
✅ **Session聚合**：按session_id关联多轮对话  
✅ **Web可视化界面**：浏览器访问，总览+下钻查看session详情  
✅ **实时URL生成**：Clawdbot可根据当前会话ID生成观测链接  
✅ **FinOps报表**：导出JSON/CSV格式的成本分析报告  

## 日志格式要求

Higress访问日志需要包含ai_log字段（JSON格式），示例：

```json
{
  "__file_offset__": "1000",
  "timestamp": "2026-02-01T09:30:15Z",
  "ai_log": "{\"session_id\":\"sess_abc\",\"messages\":[...],\"question\":\"...\",\"answer\":\"...\",\"input_token\":250,\"output_token\":160,\"model\":\"Qwen3-rerank\"}"
}
```

ai_log字段支持的属性：
- `session_id`: 会话标识（必需）
- `messages`: 完整对话历史
- `question`: 当前轮次问题
- `answer`: AI回答
- `reasoning`: 思考过程（DeepSeek等模型）
- `tool_calls`: 工具调用列表
- `input_token`: 输入token数
- `output_token`: 输出token数
- `model`: 模型名称
- `response_type`: 响应类型

## 输出目录结构

```
sessions/
├── agent:main:discord:1465367993012981988.json
└── agent:test:discord:9999999999999999999.json
```

每个session文件包含：
- 基本信息（创建时间、更新时间、模型）
- Token统计（总输入、总输出、总reasoning、总cached）
- 对话轮次列表（每轮的完整messages、question、answer、reasoning、tool_calls）

## 常见问题

**Q: 如何在Higress中配置session_id header？**  
A: 在ai-statistics插件中配置`session_id_header`，或使用默认header（x-openclaw-session-key、x-clawdbot-session-key等）。详见PR #3420。

**Q: 支持哪些模型的pricing？**  
A: 目前支持Qwen、DeepSeek、GPT-4、Claude等主流模型。可以在main.py的TOKEN_PRICING字典中添加新模型。

**Q: 如何实时监控日志文件变化？**  
A: 直接运行main.py即可，程序使用定时轮询机制（每秒自动检查一次），无需安装额外依赖。

**Q: CLI查询速度慢？**  
A: 大量session时，可以使用`--limit`限制结果数量，或按条件过滤（如`--sort-by cost`只查看成本最高的session）。

## 下一步

- 集成到Higress FinOps Dashboard
- 支持更多模型的pricing
- 添加趋势预测和异常检测
- 支持多数据源聚合分析
