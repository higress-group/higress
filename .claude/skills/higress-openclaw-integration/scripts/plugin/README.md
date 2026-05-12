# Higress AI Gateway Plugin

OpenClaw model provider plugin for Higress AI Gateway with auto-routing support.

## What is this?

This is a TypeScript-based provider plugin that enables OpenClaw to use Higress AI Gateway as a model provider. It provides:

- **Auto-routing support**: Use `higress/auto` to intelligently route requests based on message content
- **Dynamic model discovery**: Auto-detect available models from Higress Console
- **Smart URL handling**: Automatic URL normalization and validation
- **Flexible authentication**: Support for both local and remote gateway deployments

## Files

- **index.ts**: Main plugin implementation
- **package.json**: NPM package metadata and OpenClaw extension declaration
- **openclaw.plugin.json**: Plugin manifest for OpenClaw

## Installation

This plugin is automatically installed when you use the `higress-openclaw-integration` skill. See parent SKILL.md for complete installation instructions.

### Manual Installation

If you need to install manually:

```bash
# Copy plugin files
mkdir -p "$HOME/.openclaw/extensions/higress"
cp -r ./* "$HOME/.openclaw/extensions/higress/"

# Configure provider
openclaw plugins enable higress
openclaw models auth login --provider higress
```

## Usage

After installation, configure Higress as a model provider:

```bash
openclaw models auth login --provider higress
```

The plugin will prompt for:
1. Gateway URL (default: http://localhost:8080)
2. Console URL (default: http://localhost:8001)
3. API Key (optional for local deployments)
4. Model list (auto-detected or manually specified)
5. Auto-routing default model (if using higress/auto)


## Related Resources

- **Parent Skill**: [higress-openclaw-integration](../SKILL.md)
- **Auto-routing Configuration**: [higress-auto-router](../../higress-auto-router/SKILL.md)

## License

Apache-2.0
