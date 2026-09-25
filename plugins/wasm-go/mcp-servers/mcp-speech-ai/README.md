# Speech AI MCP Server

An MCP server that provides pronunciation assessment, speech-to-text, and text-to-speech capabilities for AI agents. Built for language learning, accessibility, and voice applications.

## Features

- **Pronunciation Assessment**: Score English pronunciation at phoneme, word, and sentence level (0-100). 17MB model, <300ms latency. Exceeds human expert accuracy.
- **Speech-to-Text (STT)**: Transcribe audio with word-level timestamps and confidence scores.
- **Text-to-Speech (TTS)**: Generate natural speech with 12 English voices (US + UK accents). Ranked #1 on TTS Arena.

Source: [https://github.com/fasuizu-br/speech-ai-examples](https://github.com/fasuizu-br/speech-ai-examples)

Website: [https://brainiall.com](https://brainiall.com)

## Tools

| Tool | Description |
|------|-------------|
| `assess_pronunciation` | Score English pronunciation at phoneme, word, and sentence levels (0-100) |
| `transcribe_audio` | Transcribe audio to text with word-level timestamps |
| `synthesize_speech` | Generate speech from text with 12 English voices |
| `list_tts_voices` | List available TTS voices |

# Usage Guide

## Get API Key

1. Visit [Azure Marketplace](https://azuremarketplace.microsoft.com) and search for "Speech AI"
2. Subscribe to a plan (Free tier available)
3. Your API key will be provided after subscription

Or contact fasuizu@brainiall.com for a key.

## Generate SSE URL

On the MCP Server interface, log in and enter the API key to generate the URL.

## Configure MCP Client

Add the generated SSE URL to your MCP client configuration:

```json
"mcpServers": {
    "speech-ai": {
      "url": "https://mcp.higress.ai/mcp-speech-ai/{generate_key}"
    }
}
```

## Example: Pronunciation Assessment

Send base64-encoded audio with the reference text to get detailed pronunciation scores:

- **Overall Score**: 0-100 calibrated score
- **Word Scores**: Individual word pronunciation quality
- **Phoneme Scores**: Granular phoneme-level feedback with IPA notation

## Supported Audio Formats

WAV, MP3, OGG, FLAC, WebM

## Pricing

$0.02 per API call. Free tier available via Azure Marketplace.
