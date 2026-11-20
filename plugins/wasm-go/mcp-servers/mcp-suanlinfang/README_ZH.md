# 算邻坊 AI 能力 MCP Server

算邻坊（SuanLinFang）是闲置 GPU 算力共享平台。本 MCP Server 封装其 REST API，提供文生图、语音识别与语音合成能力。

## 工具列表

| 工具 | 说明 |
|---|---|
| list_image_models | 列出文生图当前在线模型与实时牌价（按张计价） |
| submit_image_task | 文生图任务提交（1024×1024 PNG，按张计费，失败不计费）。提交后立即返回 taskNo；model 留空默认 flux2-klein |
| get_image_task | 查询生图任务状态：queued/running 进行中；success 返回现签下载链接（1 小时内有效）；failed 不扣费 |
| transcribe_audio | 语音识别（STT）单笔：公网音频 URL 转文字（≤3 分钟 / ≤20MB），支持热词纠错专有名词 |
| synthesize_speech | 语音合成（TTS）单笔：≤1000 汉字，返回现签音频下载链接 |
| submit_speech_task / get_speech_task | 长文本语音合成异步任务（≤20 万字符）+ 轮询查询 |
| list_tts_voices | 列出全部可用音色（21 个） |

官网：[https://suanlinfang.cn](https://suanlinfang.cn)

## 使用教程

### 获取 API-KEY

1. 注册 [https://suanlinfang.cn](https://suanlinfang.cn)（新用户注册即送 ¥10 体验金）
2. 在「API Key 管理」页面创建 key（nk- 前缀）：[https://suanlinfang.cn/consume-web/api-keys.html](https://suanlinfang.cn/consume-web/api-keys.html)

### 生成 SSE URL

在 MCP Server 界面，登录后输入 API-KEY，生成 URL。

### 配置 MCP Client

在用户的 MCP Client 界面，将生成的 SSE URL 添加到 MCP Server 列表中。

```json
"mcpServers": {
    "suanlinfang": {
      "url": "https://mcp.higress.ai/mcp-suanlinfang/{generate_key}",
    }
}
```

### 生图任务示例

用一段 prompt（如「黄昏湖畔小镇的水彩画」）调用 `submit_image_task`，再用 `get_image_task` 每 5~10 秒轮询，`status` 变为 `success` 后在 1 小时内下载 `resultUrl`。
