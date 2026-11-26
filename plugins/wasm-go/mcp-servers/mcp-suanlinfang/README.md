# SuanLinFang AI Capabilities MCP Server

SuanLinFang (算邻坊) is a GPU compute-sharing platform. This MCP server wraps its REST APIs, providing image generation, speech-to-text and text-to-speech capabilities.

## Tools

| Tool | Description |
|---|---|
| list_image_models | List currently available image models with live per-image pricing |
| submit_image_task | Submit a text-to-image task (1024×1024 PNG, billed per image, failures not billed). Returns taskNo immediately; default model flux2-klein when omitted |
| get_image_task | Poll an image task: queued/running in progress; success returns a presigned download URL (valid 1 hour); failed not billed |
| transcribe_audio | Speech-to-text for a short public audio URL (≤3 min, ≤20MB), with hot-word hints for proper nouns |
| synthesize_speech | Text-to-speech (≤1000 Chinese chars), returns a signed download URL |
| submit_speech_task / get_speech_task | Long-text TTS async task (≤200K chars) with polling |
| list_tts_voices | List all available TTS voices (21 voices) |

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

### Image task example

Call `submit_image_task` with a prompt (e.g. "a watercolor painting of a lakeside town at dusk"), then poll with `get_image_task` every 5-10 seconds until `status` is `success` and download the `resultUrl` within 1 hour.
