---
name: higress-auto-router
description: "Configure automatic model routing using the get-ai-gateway.sh CLI tool for Higress AI Gateway. Use when: (1) User wants to configure automatic model routing, (2) User mentions 'route to', 'switch model', 'use model when', 'auto routing', (3) User describes scenarios that should trigger specific models, (4) User wants to add, list, or remove routing rules."
---

# Higress Auto Router

Configure automatic model routing using the get-ai-gateway.sh CLI tool for intelligent model selection based on message content triggers.

## Prerequisites

- Higress AI Gateway running (container name: `higress-ai-gateway`)
- get-ai-gateway.sh script downloaded

## CLI Commands

### Add a Routing Rule

```bash
./get-ai-gateway.sh route add --model <model-name> --trigger "<trigger-phrases>"
```

**Options:**
- `--model MODEL` (required): Target model to route to
- `--trigger PHRASE`: Trigger phrase(s), separated by `|` (e.g., `"深入思考|deep thinking"`)
- `--pattern REGEX`: Custom regex pattern (alternative to `--trigger`)

**Examples:**

```bash
# Route complex reasoning to Claude
./get-ai-gateway.sh route add \
  --model claude-opus-4.5 \
  --trigger "深入思考|deep thinking"

# Route coding tasks to Qwen Coder
./get-ai-gateway.sh route add \
  --model qwen-coder \
  --trigger "写代码|code:|coding:"

# Route creative writing
./get-ai-gateway.sh route add \
  --model gpt-4o \
  --trigger "创意写作|creative:"

# Use custom regex pattern
./get-ai-gateway.sh route add \
  --model deepseek-chat \
  --pattern "(?i)^(数学题|math:)"
```

### List Routing Rules

```bash
./get-ai-gateway.sh route list
```

Output:
```
Default model: qwen-turbo

ID   Pattern                                  Model               
----------------------------------------------------------------------
0    (?i)^(深入思考|deep thinking)             claude-opus-4.5     
1    (?i)^(写代码|code:|coding:)               qwen-coder          
```

### Remove a Routing Rule

```bash
./get-ai-gateway.sh route remove --rule-id <id>
```

**Example:**
```bash
# Remove rule with ID 0
./get-ai-gateway.sh route remove --rule-id 0
```

## Common Trigger Mappings

| Scenario | Suggested Triggers | Recommended Model |
|----------|-------------------|-------------------|
| Complex reasoning | `深入思考\|deep thinking` | claude-opus-4.5, o1 |
| Coding tasks | `写代码\|code:\|coding:` | qwen-coder, deepseek-coder |
| Creative writing | `创意写作\|creative:` | gpt-4o, claude-sonnet |
| Translation | `翻译:\|translate:` | gpt-4o, qwen-max |
| Math problems | `数学题\|math:` | deepseek-r1, o1-mini |
| Quick answers | `快速回答\|quick:` | qwen-turbo, gpt-4o-mini |

## Usage Flow

1. **User Request:** "我希望在解决困难问题时路由到claude-opus-4.5"

2. **Execute CLI:**
   ```bash
   ./get-ai-gateway.sh route add \
     --model claude-opus-4.5 \
     --trigger "深入思考|deep thinking"
   ```

3. **Response to User:**
   ```
   ✅ 自动路由配置完成！
   
   触发方式：以 "深入思考" 或 "deep thinking" 开头
   目标模型：claude-opus-4.5
   
   使用示例：
   - 深入思考 这道算法题应该怎么解？
   - deep thinking What's the best architecture?
   
   提示：确保请求中 model 参数为 'higress/auto'
   ```

## How Auto-Routing Works

1. User sends request with `model: "higress/auto"`
2. Higress checks message content against routing rules
3. If a trigger pattern matches, routes to the specified model
4. If no match, uses the default model (e.g., `qwen-turbo`)

## Configuration File

Rules are stored in the container at:
```
/data/wasmplugins/model-router.internal.yaml
```

The CLI tool automatically:
- Edits the configuration file
- Triggers hot-reload (no container restart needed)
- Validates YAML syntax

## Error Handling

- **Container not running:** Start with `./get-ai-gateway.sh start`
- **Rule ID not found:** Use `route list` to see valid IDs
- **Invalid model:** Check configured providers in Higress Console
