---
name: agent-session-monitor
description: Real-time agent conversation monitoring - monitors Higress access logs, aggregates conversations by session, tracks token usage. Supports web interface for viewing complete conversation history and costs. Use when users ask about current session token consumption, conversation history, or cost statistics.

---

## Overview

Real-time monitoring of Higress access logs, extracting ai_log JSON, grouping multi-turn conversations by session_id, and calculating token costs with visualization.

### Core Features

- **Real-time Log Monitoring**: Monitors Higress access log files, parses new ai_log entries in real-time
- **Log Rotation Support**: Full logrotate support, automatically tracks access.log.1~5 etc.
- **Incremental Parsing**: Inode-based tracking, processes only new content, no duplicates
- **Session Grouping**: Associates multi-turn conversations by session_id (each turn is a separate request)
- **Complete Conversation Tracking**: Records messages, question, answer, reasoning, tool_calls for each turn
- **Token Usage Tracking**: Distinguishes input/output/reasoning/cached tokens
- **Web Visualization**: Browser-based UI with overview and session drill-down
- **Real-time URL Generation**: Clawdbot can generate observation links based on current session ID
- **Background Processing**: Independent process, continuously parses access logs
- **State Persistence**: Maintains parsing progress and session data across runs

## Usage

### 1. Background Monitoring (Continuous)

```bash
# Parse Higress access logs (with log rotation support)
python3 main.py --log-path /var/log/proxy/access.log --output-dir ./sessions

# Filter by session key
python3 main.py --log-path /var/log/proxy/access.log --session-key <session-id>

# Scheduled task (incremental parsing every minute)
* * * * * python3 /path/to/main.py --log-path /var/log/proxy/access.log --output-dir /var/lib/sessions
```

### 2. Start Web UI (Recommended)

```bash
# Start web server
python3 scripts/webserver.py --data-dir ./sessions --port 8888

# Access in browser
open http://localhost:8888
```

Web UI features:
- 📊 Overview: View all session statistics and group by model
- 🔍 Session Details: Click session ID to drill down into complete conversation history
- 💬 Conversation Log: Display messages, question, answer, reasoning, tool_calls for each turn
- 💰 Cost Statistics: Real-time token usage and cost calculation
- 🔄 Auto Refresh: Updates every 30 seconds

### 3. Use in Clawdbot Conversations

When users ask about current session token consumption or conversation history:

1. Get current session_id (from runtime or context)
2. Generate web UI URL and return to user

Example response:

```
Your current session statistics:
- Session ID: agent:main:discord:channel:1465367993012981988
- View details: http://localhost:8888/session?id=agent:main:discord:channel:1465367993012981988

Click the link to see:
✅ Complete conversation history
✅ Token usage breakdown per turn
✅ Tool call records
✅ Cost statistics
```

### 4. CLI Queries (Optional)

```bash
# View specific session details
python3 scripts/cli.py show <session-id>

# List all sessions
python3 scripts/cli.py list --sort-by cost --limit 10

# Statistics by model
python3 scripts/cli.py stats-model

# Statistics by date (last 7 days)
python3 scripts/cli.py stats-date --days 7

# Export reports
python3 scripts/cli.py export finops-report.json
```

## Configuration

### main.py (Background Monitor)

| Parameter | Description | Required | Default |
|-----------|-------------|----------|---------|
| `--log-path` | Higress access log file path | Yes | /var/log/higress/access.log |
| `--output-dir` | Session data storage directory | No | ./sessions |
| `--session-key` | Monitor only specified session key | No | Monitor all sessions |
| `--state-file` | State file path (records read offsets) | No | <output-dir>/.state.json |
| `--refresh-interval` | Log refresh interval (seconds) | No | 1 |

### webserver.py (Web UI)

| Parameter | Description | Required | Default |
|-----------|-------------|----------|---------|
| `--data-dir` | Session data directory | No | ./sessions |
| `--port` | HTTP server port | No | 8888 |
| `--host` | HTTP server address | No | 0.0.0.0 |

## Output Examples

### 1. Real-time Monitor

```
🔍 Session Monitor - Active
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📊 Active Sessions: 3

┌──────────────────────────┬─────────┬──────────┬───────────┐
│ Session ID               │ Msgs    │ Input    │ Output    │
├──────────────────────────┼─────────┼──────────┼───────────┤
│ sess_abc123              │       5 │    1,250 │       800 │
│ sess_xyz789              │       3 │      890 │       650 │
│ sess_def456              │       8 │    2,100 │     1,200 │
└──────────────────────────┴─────────┴──────────┴───────────┘

📈 Token Statistics
  Total Input:   4240 tokens
  Total Output:  2650 tokens
  Total Cached:  0 tokens
  Total Cost:    $0.00127
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### 2. CLI Session Details

```bash
$ python3 scripts/cli.py show agent:main:discord:channel:1465367993012981988

======================================================================
📊 Session Detail: agent:main:discord:channel:1465367993012981988
======================================================================

🕐 Created:  2026-02-01T09:30:00+08:00
🕑 Updated:  2026-02-01T10:35:12+08:00
🤖 Model:    Qwen3-rerank
💬 Messages: 5

📈 Token Statistics:
   Input:           1,250 tokens
   Output:            800 tokens
   Reasoning:         150 tokens
   Total:           2,200 tokens

💰 Estimated Cost: $0.00126000 USD

📝 Conversation Rounds (5):
──────────────────────────────────────────────────────────────────────

  Round 1 @ 2026-02-01T09:30:15+08:00
    Tokens: 250 in → 160 out
    🔧 Tool calls: Yes
    Messages (2):
      [user] Check Beijing weather
    ❓ Question: Check Beijing weather
    ✅ Answer: Checking Beijing weather for you...
    🧠 Reasoning: User wants to know Beijing weather, I need to call weather API.
    🛠️  Tool Calls:
       - get_weather({"location":"Beijing"})
```

### 3. Statistics by Model

```bash
$ python3 scripts/cli.py stats-model

================================================================================
📊 Statistics by Model
================================================================================

Model                Sessions   Input           Output          Cost (USD)  
────────────────────────────────────────────────────────────────────────────
Qwen3-rerank         12         15,230          9,840           $  0.016800
DeepSeek-R1          5          8,450           6,200           $  0.010600
Qwen-Max             3          4,200           3,100           $  0.008300
GPT-4                2          2,100           1,800           $  0.017100
────────────────────────────────────────────────────────────────────────────
TOTAL                22         29,980          20,940          $  0.052800

================================================================================
```

### 4. Statistics by Date

```bash
$ python3 scripts/cli.py stats-date --days 7

================================================================================
📊 Statistics by Date (Last 7 days)
================================================================================

Date         Sessions   Input           Output          Cost (USD)   Models              
────────────────────────────────────────────────────────────────────────────
2026-01-26   3          2,100           1,450           $  0.0042   Qwen3-rerank
2026-01-27   5          4,850           3,200           $  0.0096   Qwen3-rerank, GPT-4
2026-01-28   4          3,600           2,800           $  0.0078   DeepSeek-R1, Qwen
────────────────────────────────────────────────────────────────────────────
TOTAL        22         29,980          20,940          $  0.0528

================================================================================
```

### 5. Web UI (Recommended)

Access `http://localhost:8888` to see:

**Home Page:**
- 📊 Total sessions, token consumption, cost cards
- 📋 Recent sessions list (clickable for details)
- 📈 Statistics by model table

**Session Detail Page:**
- 💬 Complete conversation log (messages, question, answer, reasoning, tool_calls per turn)
- 🔧 Tool call history
- 💰 Token usage breakdown and costs

**Features:**
- 🔄 Auto-refresh every 30 seconds
- 📱 Responsive design, mobile-friendly
- 🎨 Clean UI, easy to read

## Session Data Structure

Each session is stored as an independent JSON file with complete conversation history and token statistics:

```json
{
  "session_id": "agent:main:discord:channel:1465367993012981988",
  "created_at": "2026-02-01T10:30:00Z",
  "updated_at": "2026-02-01T10:35:12Z",
  "messages_count": 5,
  "total_input_tokens": 1250,
  "total_output_tokens": 800,
  "total_reasoning_tokens": 150,
  "total_cached_tokens": 0,
  "model": "Qwen3-rerank",
  "rounds": [
    {
      "round": 1,
      "timestamp": "2026-02-01T10:30:15Z",
      "input_tokens": 250,
      "output_tokens": 160,
      "reasoning_tokens": 0,
      "cached_tokens": 0,
      "model": "Qwen3-rerank",
      "has_tool_calls": true,
      "response_type": "normal",
      "messages": [
        {
          "role": "system",
          "content": "You are a helpful assistant..."
        },
        {
          "role": "user",
          "content": "Check Beijing weather"
        }
      ],
      "question": "Check Beijing weather",
      "answer": "Checking Beijing weather for you...",
      "reasoning": "User wants to know Beijing weather, need to call weather API.",
      "tool_calls": [
        {
          "index": 0,
          "id": "call_abc123",
          "type": "function",
          "function": {
            "name": "get_weather",
            "arguments": "{\"location\":\"Beijing\"}"
          }
        }
      ],
      "input_token_details": {"cached_tokens": 0},
      "output_token_details": {}
    }
  ]
}
```

### Field Descriptions

**Session Level:**
- `session_id`: Unique session identifier (from ai_log's session_id field)
- `created_at`: Session creation time
- `updated_at`: Last update time
- `messages_count`: Number of conversation turns
- `total_input_tokens`: Cumulative input tokens
- `total_output_tokens`: Cumulative output tokens
- `total_reasoning_tokens`: Cumulative reasoning tokens (DeepSeek, o1, etc.)
- `total_cached_tokens`: Cumulative cached tokens (prompt caching)
- `model`: Current model in use

**Round Level (rounds):**
- `round`: Turn number
- `timestamp`: Current turn timestamp
- `input_tokens`: Input tokens for this turn
- `output_tokens`: Output tokens for this turn
- `reasoning_tokens`: Reasoning tokens (o1, etc.)
- `cached_tokens`: Cached tokens (prompt caching)
- `model`: Model used for this turn
- `has_tool_calls`: Whether includes tool calls
- `response_type`: Response type (normal/error, etc.)
- `messages`: Complete conversation history (OpenAI messages format)
- `question`: User's question for this turn (last user message)
- `answer`: AI's answer for this turn
- `reasoning`: AI's thinking process (if model supports)
- `tool_calls`: Tool call list (if any)
- `input_token_details`: Complete input token details (JSON)
- `output_token_details`: Complete output token details (JSON)

## Log Format Requirements

Higress access logs must include ai_log field (JSON format). Example:

```json
{
  "__file_offset__": "1000",
  "timestamp": "2026-02-01T09:30:15Z",
  "ai_log": "{\"session_id\":\"sess_abc\",\"messages\":[...],\"question\":\"...\",\"answer\":\"...\",\"input_token\":250,\"output_token\":160,\"model\":\"Qwen3-rerank\"}"
}
```

Supported ai_log attributes:
- `session_id`: Session identifier (required)
- `messages`: Complete conversation history
- `question`: Question for current turn
- `answer`: AI answer
- `reasoning`: Thinking process (DeepSeek, o1, etc.)
- `reasoning_tokens`: Reasoning token count (from PR #3424)
- `cached_tokens`: Cached token count (from PR #3424)
- `tool_calls`: Tool call list
- `input_token`: Input token count
- `output_token`: Output token count
- `input_token_details`: Complete input token details (JSON)
- `output_token_details`: Complete output token details (JSON)
- `model`: Model name
- `response_type`: Response type

## Implementation

### Technology Stack

- **Log Parsing**: Direct JSON parsing, no regex needed
- **File Monitoring**: Polling-based (no watchdog dependency)
- **Session Management**: In-memory + disk hybrid storage
- **Token Calculation**: Model-specific pricing for GPT-4, Qwen, Claude, o1, etc.

### Privacy and Security

- ✅ Does not record conversation content in logs, only token statistics
- ✅ Session data stored locally, not uploaded to external services
- ✅ Supports log file path allowlist
- ✅ Session key access control

### Performance Optimization

- Incremental log parsing, avoids full scans
- In-memory session data with periodic persistence
- Optimized log file reading (offset tracking)
- Inode-based file identification (handles rotation efficiently)
