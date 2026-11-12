# 算邻坊 AI 能力 MCP Server

算邻坊（SuanLinFang）是闲置 GPU 算力共享平台。本 MCP Server 封装其 REST API，提供语音识别、语音合成、文字识别与智能决策能力。

## 工具列表

| 工具 | 说明 |
|---|---|
| transcribe_audio | 语音识别（STT）单笔：公网音频 URL 转文字（≤3 分钟 / ≤20MB），支持热词纠错专有名词 |
| recognize_image_url | 文字识别（OCR）单图：公网图片 URL，standard/accurate 双档位，高精度档可自定义 prompt |
| recognize_image_base64 | 文字识别（OCR）单图：base64 直传识别 |
| synthesize_speech | 语音合成（TTS）单笔：≤1000 汉字，返回现签音频下载链接 |
| submit_speech_task / get_speech_task | 长文本语音合成异步任务（≤20 万字符）+ 轮询查询 |
| list_tts_voices | 列出全部可用音色（21 个） |
| estimate_tts_cost | 语音合成计费预估（不实际合成、不计费） |
| decide | 智能决策：类型化问题（单选/判断/等级打分）返回校准概率，不生成文本 |

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

### decide 的 questions 示例

```json
{
  "team": {"type": "choice", "criteria": {"refund": "退款", "logistics": "物流"}},
  "urgent": {"type": "noul"},
  "severity": {"type": "score", "criteria": ["轻微", "一般", "严重"]}
}
```
