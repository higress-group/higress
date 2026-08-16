import { emptyPluginConfigSchema } from "openclaw/plugin-sdk";

const DEFAULT_GATEWAY_URL = "http://localhost:8080";
const DEFAULT_CONSOLE_URL = "http://localhost:8001";

// Model-specific context window and max tokens configurations
const MODEL_CONFIG: Record<string, { contextWindow: number; maxTokens: number }> = {
  "gpt-5.4": { contextWindow: 1_000_000, maxTokens: 128_000 },
  "gpt-5.4-mini": { contextWindow: 400_000, maxTokens: 128_000 },
  "gpt-5.4-nano": { contextWindow: 400_000, maxTokens: 128_000 },
  "claude-opus-4-6": { contextWindow: 1_000_000, maxTokens: 128_000 },
  "claude-sonnet-4-6": { contextWindow: 1_000_000, maxTokens: 64_000 },
  "claude-haiku-4-5": { contextWindow: 200_000, maxTokens: 64_000 },
  "qwen3.5-plus": { contextWindow: 960_000, maxTokens: 64_000 },
  "deepseek-chat": { contextWindow: 256_000, maxTokens: 128_000 },
  "deepseek-reasoner": { contextWindow: 256_000, maxTokens: 128_000 },
  "kimi-k2.5": { contextWindow: 256_000, maxTokens: 128_000 },
  "glm-5": { contextWindow: 200_000, maxTokens: 128_000 },
  "MiniMax-M2.5": { contextWindow: 200_000, maxTokens: 128_000 },
};

// Default values for unknown models
const DEFAULT_CONTEXT_WINDOW = 200_000;
const DEFAULT_MAX_TOKENS = 128_000;

// Common models that Higress AI Gateway typically supports
const DEFAULT_MODEL_IDS = [
  // Auto-routing special model
  "higress/auto",
  // Commonly models
  "kimi-k2.5",
  "glm-5",
  "MiniMax-M2.5",
  "qwen3.5-plus",
  // Anthropic models
  "claude-opus-4-6",
  "claude-sonnet-4-6",
  "claude-haiku-4-5",
  // OpenAI models
  "gpt-5.4",
  "gpt-5.4-mini",
  "gpt-5.4-nano",
  // DeepSeek models
  "deepseek-chat",
  "deepseek-reasoner",  
] as const;

function normalizeBaseUrl(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return DEFAULT_GATEWAY_URL;
  let normalized = trimmed;
  while (normalized.endsWith("/")) normalized = normalized.slice(0, -1);
  if (!normalized.endsWith("/v1")) normalized = `${normalized}/v1`;
  return normalized;
}

function validateUrl(value: string): string | undefined {
  const normalized = normalizeBaseUrl(value);
  try {
    new URL(normalized);
  } catch {
    return "Enter a valid URL";
  }
  return undefined;
}

function parseModelIds(input: string): string[] {
  const parsed = input
    .split(/[\n,]/)
    .map((model) => model.trim())
    .filter(Boolean);
  return Array.from(new Set(parsed));
}

function buildModelDefinition(modelId: string) {
  const isAutoModel = modelId === "higress/auto";
  const config = MODEL_CONFIG[modelId] || { contextWindow: DEFAULT_CONTEXT_WINDOW, maxTokens: DEFAULT_MAX_TOKENS };

  return {
    id: modelId,
    name: isAutoModel ? "Higress Auto Router" : modelId,
    api: "openai-completions",
    reasoning: true,
    input: ["text", "image"],
    cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
    contextWindow: config.contextWindow,
    maxTokens: config.maxTokens,
  };
}

async function testGatewayConnection(gatewayUrl: string): Promise<boolean> {
  try {
    // gatewayUrl already ends with /v1 from normalizeBaseUrl()
    // Use chat/completions endpoint with empty body to test connection
    // Higress doesn't support /models endpoint
    const response = await fetch(`${gatewayUrl}/chat/completions`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({}),
      signal: AbortSignal.timeout(5000),
    });
    // Any response (including 400/401/422) means gateway is reachable
    return true;
  } catch {
    return false;
  }
}

async function fetchAvailableModels(consoleUrl: string): Promise<string[]> {
  try {
    // Try to get models from Higress Console API
    const response = await fetch(`${consoleUrl}/v1/ai/routes`, {
      method: "GET",
      headers: { "Content-Type": "application/json" },
      signal: AbortSignal.timeout(5000),
    });
    if (response.ok) {
      const data = (await response.json()) as { data?: { model?: string }[] };
      if (data.data && Array.isArray(data.data)) {
        return data.data
          .map((route: { model?: string }) => route.model)
          .filter((m): m is string => typeof m === "string");
      }
    }
  } catch {
    // Ignore errors, use defaults
  }
  return [];
}

const higressPlugin = {
  id: "higress",
  name: "Higress AI Gateway",
  description: "Model provider plugin for Higress AI Gateway with auto-routing support",
  configSchema: emptyPluginConfigSchema(),
  register(api) {
    api.registerProvider({
      id: "higress",
      label: "Higress AI Gateway",
      docsPath: "/providers/models",
      aliases: ["higress-gateway", "higress-ai"],
      auth: [
        {
          id: "api-key",
          label: "API Key",
          hint: "Configure Higress AI Gateway endpoint with optional API key",
          kind: "custom",
          run: async (ctx) => {
            // Step 1: Get Gateway URL
            const gatewayUrlInput = await ctx.prompter.text({
              message: "Higress AI Gateway URL",
              initialValue: DEFAULT_GATEWAY_URL,
              validate: validateUrl,
            });
            const gatewayUrl = normalizeBaseUrl(gatewayUrlInput);

            // Step 2: Get Console URL (for auto-router configuration)
            const consoleUrlInput = await ctx.prompter.text({
              message: "Higress Console URL (for auto-router config)",
              initialValue: DEFAULT_CONSOLE_URL,
              validate: validateUrl,
            });
            const consoleUrl = normalizeBaseUrl(consoleUrlInput);

            // Step 3: Test connection (create a new spinner)
            const spin = ctx.prompter.progress("Testing gateway connection…");
            const isConnected = await testGatewayConnection(gatewayUrl);
            if (!isConnected) {
              spin.stop("Gateway connection failed");
              await ctx.prompter.note(
                [
                  "Could not connect to Higress AI Gateway.",
                  "Make sure the gateway is running and the URL is correct.",
                ].join("\n"),
                "Connection Warning",
              );
            } else {
              spin.stop("Gateway connected");
            }

            // Step 4: Get API Key (optional for local gateway)
            const apiKeyInput = await ctx.prompter.text({
              message: "API Key (leave empty if not required)",
              initialValue: "",
            }) || '';
            const apiKey = apiKeyInput.trim() || "higress-local";

            // Step 5: Fetch available models (create a new spinner)
            const spin2 = ctx.prompter.progress("Fetching available models…");
            const fetchedModels = await fetchAvailableModels(consoleUrl);
            const defaultModels = fetchedModels.length > 0
              ? ["higress/auto", ...fetchedModels]
              : DEFAULT_MODEL_IDS;
            spin2.stop();

            // Step 6: Let user customize model list
            const modelInput = await ctx.prompter.text({
              message: "Model IDs (comma-separated, higress/auto enables auto-routing)",
              initialValue: defaultModels.slice(0, 10).join(", "),
              validate: (value) =>
                parseModelIds(value).length > 0 ? undefined : "Enter at least one model id",
            });

            const modelIds = parseModelIds(modelInput);
            const hasAutoModel = modelIds.includes("higress/auto");

            // Always add higress/ provider prefix to create model reference
            const defaultModelId = hasAutoModel
              ? "higress/auto"
              : (modelIds[0] ?? "glm-5");
            const defaultModelRef = `higress/${defaultModelId}`;

            // Step 7: Configure default model for auto-routing
            let autoRoutingDefaultModel = "glm-5";
            if (hasAutoModel) {
              const autoRoutingModelInput = await ctx.prompter.text({
                message: "Default model for auto-routing (when no rule matches)",
                initialValue: "glm-5",
              });
              autoRoutingDefaultModel = autoRoutingModelInput.trim(); // FIX: Add trim() here
            }

            return {
              profiles: [
                {
                  profileId: `higress:${apiKey === "higress-local" ? "local" : "default"}`,
                  credential: {
                    type: "token",
                    provider: "higress",
                    token: apiKey,
                  },
                },
              ],
              configPatch: {
                models: {
                  providers: {
                    higress: {
                      // gatewayUrl already ends with /v1 from normalizeBaseUrl()
                      baseUrl: gatewayUrl,
                      apiKey: apiKey,
                      api: "openai-completions",
                      authHeader: apiKey !== "higress-local",
                      models: modelIds.map((modelId) => buildModelDefinition(modelId)),
                    },
                  },
                },
                agents: {
                  defaults: {
                    models: Object.fromEntries(
                      modelIds.map((modelId) => {
                        // Always add higress/ provider prefix to create model reference
                        const modelRef = `higress/${modelId}`;
                        return [modelRef, {}];
                      }),
                    ),
                  },
                },
                plugins: {
                  entries: {
                    "higress": {
                      enabled: true,
                      config: {
                        gatewayUrl,
                        consoleUrl,
                        autoRoutingDefaultModel,
                      },
                    },
                  },
                },
              },
              defaultModel: defaultModelRef,
              notes: [
                "Higress AI Gateway is now configured as a model provider.",
                hasAutoModel
                  ? `Auto-routing enabled: use model "higress/auto" to route based on message content.`
                  : "Add 'higress/auto' to models to enable auto-routing.",
                // gatewayUrl already ends with /v1 from normalizeBaseUrl()
                `Gateway endpoint: ${gatewayUrl}/chat/completions`,
                `Console: ${consoleUrl}`,
                "",
                "💡 Future Configuration Updates (No Restart Needed):",
                "   • Add New Providers: Add LLM providers (DeepSeek, OpenAI, Claude, etc.) dynamically.",
                "   • Update API Keys: Update existing provider keys without restart.",
                "   • Configure Auto-Routing: Ask OpenClaw to set up intelligent routing rules.",
                "   All changes hot-load via Higress — no gateway restart required!",
                "",
                "🎯 Recommended Skills (install via OpenClaw conversation):",
                "",
                "1. Auto-Routing Skill:",
                "   Configure automatic model routing based on message content",
                "   https://github.com/alibaba/higress/tree/main/.claude/skills/higress-auto-router",
                '   Say: "Install higress-auto-router skill"',
              ],
            };
          },
        },
      ],
    });
  },
};

export default higressPlugin;
