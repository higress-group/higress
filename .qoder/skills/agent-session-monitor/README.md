# Agent Session Monitor

Real-time agent conversation monitoring for Clawdbot, designed to monitor Higress access logs and track token usage across multi-turn conversations.

## Features

- 🔍 **Complete Conversation Tracking**: Records messages, question, answer, reasoning, tool_calls for each turn
- 💰 **Token Usage Statistics**: Distinguishes input/output/reasoning/cached tokens, calculates costs in real-time
- 🌐 **Web Visualization**: Browser-based UI with overview and drill-down into session details
- 🔗 **Real-time URL Generation**: Clawdbot can generate observation links based on current session ID
- 🔄 **Log Rotation Support**: Automatically handles rotated log files (access.log, access.log.1, etc.)
- 📊 **FinOps Reporting**: Export usage data in JSON/CSV formats

## Quick Start

### 1. Run Demo

```bash
cd example
bash demo.sh
```

### 2. Start Web UI

```bash
# Parse logs
python3 main.py --log-path /var/log/higress/access.log --output-dir ./sessions

# Start web server
python3 scripts/webserver.py --data-dir ./sessions --port 8888

# Access in browser
open http://localhost:8888
```

### 3. Use in Clawdbot

When users ask "How many tokens did this conversation use?", you can respond with:

```
Your current session statistics:
- Session ID: agent:main:discord:channel:1465367993012981988
- View details: http://localhost:8888/session?id=agent:main:discord:channel:1465367993012981988

Click to see:
✅ Complete conversation history
✅ Token usage breakdown per turn
✅ Tool call records
✅ Cost statistics
```

## Files

- `main.py`: Background monitor, parses Higress access logs
- `scripts/webserver.py`: Web server, provides browser-based UI
- `scripts/cli.py`: Command-line tools for queries and exports
- `example/`: Demo examples and test data

## Dependencies

- Python 3.8+
- No external dependencies (uses only standard library)

## Documentation

- `SKILL.md`: Main skill documentation
- `QUICKSTART.md`: Quick start guide

## License

MIT
