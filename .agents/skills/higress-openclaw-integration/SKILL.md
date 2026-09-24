---
name: higress-openclaw-integration
description: "Deploy and configure Higress AI Gateway for OpenClaw integration. Use when: (1) User wants to deploy Higress AI Gateway, (2) User wants to configure OpenClaw to use more model providers, (3) User mentions 'higress', 'ai gateway', 'model gateway', 'AI网关', (4) User wants to set up model routing or auto-routing, (5) User needs to manage LLM provider API keys."
---

# Higress AI Gateway Integration

Deploy Higress AI Gateway and configure OpenClaw to use it as a unified model provider.

## Quick Start

### Step 1: Collect Information from User

**Ask the user for the following information upfront:**

1. **Which LLM provider(s) to use?** (at least one required)

   **Commonly Used Providers:**

   | Provider | Parameter | Notes |
   |----------|-----------|-------|
   | 智谱 / z.ai | `--zhipuai-key` | Models: glm-*, Code Plan mode enabled by default |
   | Claude Code | `--claude-code-key` | **Requires OAuth token from `claude setup-token`** |
   | Moonshot (Kimi) | `--moonshot-key` | Models: moonshot-*, kimi-* |
   | Minimax | `--minimax-key` | Models: abab-* |
   | 阿里云通义千问 (Dashscope) | `--dashscope-key` | Models: qwen* |
   | OpenAI | `--openai-key` | Models: gpt-*, o1-*, o3-* |
   | DeepSeek | `--deepseek-key` | Models: deepseep-* |
   | Grok | `--grok-key` | Models: grok-* |

   **Other Providers:**

   | Provider | Parameter | Notes |
   |----------|-----------|-------|
   | Claude | `--claude-key` | Models: claude-* |
   | Google Gemini | `--gemini-key` | Models: gemini-* |
   | OpenRouter | `--openrouter-key` | Supports all models (catch-all) |
   | Groq | `--groq-key` | Fast inference |
   | Doubao (豆包) | `--doubao-key` | Models: doubao-* |
   | Mistral | `--mistral-key` | Models: mistral-* |
   | Baichuan (百川) | `--baichuan-key` | Models: Baichuan* |
   | 01.AI (Yi) | `--yi-key` | Models: yi-* |
   | Stepfun (阶跃星辰) | `--stepfun-key` | Models: step-* |
   | Cohere | `--cohere-key` | Models: command* |
   | Fireworks AI | `--fireworks-key` | - |
   | Together AI | `--togetherai-key` | - |
   | GitHub Models | `--github-key` | - |
   
   **Cloud Providers (require additional config):**
   - Azure OpenAI: `--azure-key` (requires service URL)
   - AWS Bedrock: `--bedrock-key` (requires region and access key)
   - Google Vertex AI: `--vertex-key` (requires project ID and region)
   
   **Brand Name Display (z.ai / 智谱):**
   - If user communicates in Chinese: display as "智谱"
   - If user communicates in English: display as "z.ai"

2. **Enable auto-routing?** (recommended)
   - If yes: `--auto-routing --auto-routing-default-model <model-name>`
   - Auto-routing allows using `model="higress/auto"` to automatically route requests based on message content

3. **Custom ports?** (optional, defaults: HTTP=8080, HTTPS=8443, Console=8001)

### Step 2: Deploy Gateway

**Auto-detect region for z.ai / 智谱 domain configuration:**

When user selects z.ai / 智谱 provider, detect their region:

```bash
# Run region detection script (scripts/detect-region.sh relative to skill directory)
REGION=$(bash scripts/detect-region.sh)
# Output: "china" or "international"
```

**Based on detection result:**

- If `REGION="china"`: use default domain `open.bigmodel.cn`, no extra parameter needed
- If `REGION="international"`: automatically add `--zhipuai-domain api.z.ai` to deployment command

**After deployment (for international users):**
Notify user in English: "The z.ai endpoint domain has been set to api.z.ai. If you want to change it, let me know and I can update the configuration."

```bash
# Create installation directory
mkdir -p higress-install
cd higress-install

# Download script (if not exists)
curl -fsSL https://higress.ai/ai-gateway/install.sh -o get-ai-gateway.sh
chmod +x get-ai-gateway.sh

# Deploy with user's configuration
# For z.ai / 智谱: always include --zhipuai-code-plan-mode
# For non-China users: include --zhipuai-domain api.z.ai
./get-ai-gateway.sh start --non-interactive \
  --<provider>-key <api-key> \
  [--auto-routing --auto-routing-default-model <model>]
```

**z.ai / 智谱 Options:**
| Option | Description |
|--------|-------------|
| `--zhipuai-code-plan-mode` | Enable Code Plan mode (enabled by default) |
| `--zhipuai-domain <domain>` | Custom domain, default: `open.bigmodel.cn` (China), `api.z.ai` (international) |

**Example (China user):**
```bash
./get-ai-gateway.sh start --non-interactive \
  --zhipuai-key sk-xxx \
  --zhipuai-code-plan-mode \
  --auto-routing \
  --auto-routing-default-model glm-5
```

**Example (International user):**
```bash
./get-ai-gateway.sh start --non-interactive \
  --zhipuai-key sk-xxx \
  --zhipuai-domain api.z.ai \
  --zhipuai-code-plan-mode \
  --auto-routing \
  --auto-routing-default-model glm-5
```

### Step 3: Install OpenClaw Plugin

Install the Higress provider plugin for OpenClaw:

```bash
# Copy plugin files (PLUGIN_SRC is relative to skill directory: scripts/plugin)
PLUGIN_SRC="scripts/plugin"
PLUGIN_DEST="$HOME/.openclaw/extensions/higress"

mkdir -p "$PLUGIN_DEST"
cp -r "$PLUGIN_SRC"/* "$PLUGIN_DEST/"
```

**Tell user to run the following commands manually in their terminal (interactive commands, cannot be executed by AI agent):**

```bash
# Step 1: Enable the plugin
openclaw plugins enable higress

# Step 2: Configure provider (interactive - will prompt for Gateway URL, API Key, models, etc.)
openclaw models auth login --provider higress --set-default

# Step 3: Restart OpenClaw gateway to apply changes
openclaw gateway restart
```

The `openclaw models auth login` command will interactively prompt for:
1. Gateway URL (default: `http://localhost:8080`)
2. Console URL (default: `http://localhost:8001`)
3. API Key (optional for local deployments)
4. Model list (auto-detected or manually specified)
5. Auto-routing default model (if using `higress/auto`)

After configuration and restart, Higress models are available in OpenClaw with `higress/` prefix (e.g., `higress/glm-5`, `higress/auto`).

**Future Configuration Updates (No Restart Needed)**

After the initial setup, you can manage your configuration through conversation with OpenClaw:

- **Add New Providers**: Add new LLM providers (e.g., DeepSeek, OpenAI, Claude) and their models dynamically.
- **Update API Keys**: Update existing provider API keys without service restart.
- **Configure Auto-routing**: If you've set up multiple models, ask OpenClaw to configure auto-routing rules. Requests will be intelligently routed based on your message content, using the most suitable model automatically.

All configuration changes are hot-loaded through Higress — no `openclaw gateway restart` required. Iterate on your model provider setup dynamically without service interruption!

## Post-Deployment Management

### Add/Update API Keys (Hot-reload)

```bash
./get-ai-gateway.sh config add --provider <provider> --key <api-key>
./get-ai-gateway.sh config list
./get-ai-gateway.sh config remove --provider <provider>
```

Provider aliases: `dashscope`/`qwen`, `moonshot`/`kimi`, `zhipuai`/`zhipu`

### Update z.ai Domain (Hot-reload)

If user wants to change the z.ai domain after deployment:

```bash
# Update domain configuration
./get-ai-gateway.sh config add --provider zhipuai --extra-config "zhipuDomain=api.z.ai"
# Or revert to China endpoint
./get-ai-gateway.sh config add --provider zhipuai --extra-config "zhipuDomain=open.bigmodel.cn"
```

### Add Routing Rules (for auto-routing)

```bash
# Add rule: route to specific model when message starts with trigger
./get-ai-gateway.sh route add --model <model> --trigger "keyword1|keyword2"

# Examples
./get-ai-gateway.sh route add --model glm-4-flash --trigger "quick|fast"
./get-ai-gateway.sh route add --model claude-opus-4 --trigger "think|complex"
./get-ai-gateway.sh route add --model deepseek-coder --trigger "code|debug"

# List/remove rules
./get-ai-gateway.sh route list
./get-ai-gateway.sh route remove --rule-id 0
```

### Stop/Delete Gateway

```bash
./get-ai-gateway.sh stop
./get-ai-gateway.sh delete
```

## Endpoints

| Endpoint | URL |
|----------|-----|
| Chat Completions | http://localhost:8080/v1/chat/completions |
| Console | http://localhost:8001 |
| Logs | `./higress-install/logs/access.log` |

## Testing

```bash
# Test with specific model
curl 'http://localhost:8080/v1/chat/completions' \
  -H 'Content-Type: application/json' \
  -d '{"model": "<model-name>", "messages": [{"role": "user", "content": "Hello"}]}'

# Test auto-routing (if enabled)
curl 'http://localhost:8080/v1/chat/completions' \
  -H 'Content-Type: application/json' \
  -d '{"model": "higress/auto", "messages": [{"role": "user", "content": "What is AI?"}]}'
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Container fails to start | Check `docker logs higress-ai-gateway` |
| Port already in use | Use `--http-port`, `--console-port` to change ports |
| API key error | Run `./get-ai-gateway.sh config list` to verify keys |
| Auto-routing not working | Ensure `--auto-routing` was set during deployment |
| Slow image download | Script auto-selects nearest registry based on timezone |

## Important Notes

1. **Claude Code Mode**: Requires OAuth token from `claude setup-token` command, not a regular API key
2. **z.ai Code Plan Mode**: Enabled by default, uses `/api/coding/paas/v4/chat/completions` endpoint, optimized for coding tasks
3. **z.ai Domain Selection**:
   - China users: `open.bigmodel.cn` (default)
   - International users: `api.z.ai` (auto-detected based on timezone)
   - Users can update domain anytime after deployment
4. **Auto-routing**: Must be enabled during initial deployment (`--auto-routing`); routing rules can be added later
5. **OpenClaw Integration**: The `openclaw models auth login` and `openclaw gateway restart` commands are **interactive** and must be run by the user manually in their terminal
6. **Hot-reload**: API key changes take effect immediately; no container restart needed
