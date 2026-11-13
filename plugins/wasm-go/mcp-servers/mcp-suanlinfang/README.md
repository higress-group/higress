# SuanLinFang AI Capabilities MCP Server

SuanLinFang (算邻坊) is a GPU compute-sharing platform. This MCP server wraps its REST APIs, providing speech-to-text, text-to-speech, OCR and typed decision capabilities.

## Tools

| Tool | Description |
|---|---|
| transcribe_audio | Speech-to-text for a short public audio URL (≤3 min, ≤20MB), with hot-word hints for proper nouns |
| recognize_image_url | OCR for a public image URL, standard/accurate tiers, customizable prompt on accurate tier |
| recognize_image_base64 | OCR for a base64-encoded image |
| synthesize_speech | Text-to-speech (≤1000 Chinese chars), returns a signed download URL |
| submit_speech_task / get_speech_task | Long-text TTS async task (≤200K chars) with polling |
| list_tts_voices | List all available TTS voices (21 voices) |
| estimate_tts_cost | Estimate TTS cost without synthesizing |
| decide | Typed decision: returns calibrated probabilities for choice / yes-no / ordinal-score questions |

Website: [https://suanlinfang.cn](https://suanlinfang.cn)

## Usage Guide

### Get API-KEY

1. Register at [https://suanlinfang.cn](https://suanlinfang.cn) (new users get ¥10 free trial credit)
2. Create an API key (nk- prefix) at the API Key management page: [https://suanlinfang.cn/consume-web/api-keys.html](https://suanlinfang.cn/consume-web/api-keys.html)

### Generate SSE URL

On the MCP Server interface, log in and enter the API-KEY to generate the URL.

### Configure MCP Client

On the user's MCP Client interface, add the generated SSE URL to the MCP Server list.

```json
"mcpServers": {
    "suanlinfang": {
      "url": "https://mcp.higress.ai/mcp-suanlinfang/{generate_key}",
    }
}
```

### decide questions example

```json
{
  "team": {"type": "choice", "criteria": {"refund": "退款", "logistics": "物流"}},
  "urgent": {"type": "noul"},
  "severity": {"type": "score", "criteria": ["轻微", "一般", "严重"]}
}
```
