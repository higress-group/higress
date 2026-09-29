# Higress AI Gateway Plugin (Clawdbot)

Clawdbot model provider plugin for Higress AI Gateway with auto-routing support.

## What is this?

This is a TypeScript-based provider plugin that enables Clawdbot to use Higress AI Gateway as a model provider. It provides:

- **Auto-routing support**: Use `higress/auto` to intelligently route requests based on message content
- **Dynamic model discovery**: Auto-detect available models from Higress Console
- **Smart URL handling**: Automatic URL normalization and validation
- **Flexible authentication**: Support for both local and remote gateway deployments

## Files

- **index.ts**: Main plugin implementation
- **package.json**: NPM package metadata and Clawdbot extension declaration
- **clawdbot.plugin.json**: Plugin manifest for Clawdbot

## Installation

This plugin is automatically installed when you use the `higress-clawdbot-integration` skill. See the parent SKILL.md for complete installation instructions.

### Manual Installation

If you need to install manually:

```bash
# Copy plugin files
mkdir -p "$HOME/.clawdbot/extensions/higress-ai-gateway"
cp -r ./* "$HOME/.clawdbot/extensions/higress-ai-gateway/"

# Configure provider
clawdbot models auth login --provider higress
```

## Usage

After installation, configure Higress as a model provider:

```bash
clawdbot models auth login --provider higress
```

The plugin will prompt for:
1. Gateway URL (default: http://localhost:8080)
2. Console URL (default: http://localhost:8001)
3. API Key (optional for local deployments)
4. Model list (auto-detected or manually specified)
5. Auto-routing default model (if using higress/auto)

## Auto-routing

To use auto-routing, include `higress/auto` in your model list during configuration. Then use it in your conversations:

```bash
# Use auto-routing
clawdbot chat --model higress/auto "深入思考 这个问题应该怎么解决?"

# The gateway will automatically route to the appropriate model based on:
# - Message content triggers (configured via higress-auto-router skill)
# - Fallback to default model if no rule matches
```

## Related Resources

- **Parent Skill**: [higress-clawdbot-integration](../SKILL.md)
- **Auto-routing Configuration**: [higress-auto-router](../../higress-auto-router/SKILL.md)
- **Session Monitoring**: [agent-session-monitor](../../agent-session-monitor/SKILL.md)
- **Higress AI Gateway**: https://github.com/higress-group/higress-standalone

## Compatibility

- **Clawdbot**: v2.0.0+
- **Higress AI Gateway**: All versions

## License

Apache-2.0
