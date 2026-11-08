# Speech AI MCP Server

MCP 服务器，提供发音评估、语音转文字和文字转语音功能，专为 AI 智能体设计。适用于语言学习、无障碍访问和语音应用场景。

## 功能特性

- **发音评估**：在音素、单词和句子级别对英语发音进行 0-100 分评分。17MB 模型，延迟 <300ms，准确度超过人类专家。
- **语音转文字（STT）**：将音频转录为文字，提供单词级时间戳和置信度分数。
- **文字转语音（TTS）**：使用 12 种英语语音（美式和英式口音）生成自然语音。在 TTS Arena 排名第一。

源码：[https://github.com/fasuizu-br/speech-ai-examples](https://github.com/fasuizu-br/speech-ai-examples)

官网：[https://brainiall.com](https://brainiall.com)

## 工具列表

| 工具 | 描述 |
|------|------|
| `assess_pronunciation` | 在音素、单词和句子级别评估英语发音（0-100分） |
| `transcribe_audio` | 将音频转录为文字，提供单词级时间戳 |
| `synthesize_speech` | 使用 12 种英语语音从文字生成语音 |
| `list_tts_voices` | 列出可用的 TTS 语音 |

# 使用指南

## 获取 API 密钥

1. 访问 [Azure Marketplace](https://azuremarketplace.microsoft.com) 搜索 "Speech AI"
2. 订阅计划（提供免费层级）
3. 订阅后将获得 API 密钥

或联系 fasuizu@brainiall.com 获取密钥。

## 生成 SSE URL

在 MCP Server 界面登录并输入 API 密钥生成 URL。

## 配置 MCP 客户端

将生成的 SSE URL 添加到 MCP 客户端配置中：

```json
"mcpServers": {
    "speech-ai": {
      "url": "https://mcp.higress.ai/mcp-speech-ai/{generate_key}"
    }
}
```

## 支持的音频格式

WAV, MP3, OGG, FLAC, WebM

## 定价

每次 API 调用 $0.02。通过 Azure Marketplace 提供免费层级。
