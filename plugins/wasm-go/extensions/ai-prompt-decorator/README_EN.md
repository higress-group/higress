---
title: AI Prompts
keywords: [ AI Gateway, AI Prompts ]
description: AI Prompts plugin configuration reference
---
## Function Description
The AI Prompts plugin allows inserting prompts before and after the LLM request, and rewriting the `content` text of every message in the final request via literal or regular-expression replacement. Typical use cases include rewriting brand/product names, normalizing wording across clients, or redacting placeholders such as API keys.

## Execution Properties
Plugin execution phase: `Default Phase`  
Plugin execution priority: `450`

## Configuration Description
| Name      | Data Type               | Requirement | Default Value | Description                                                                 |
|-----------|-------------------------|-------------|---------------|-----------------------------------------------------------------------------|
| `prepend` | array of message object | optional    | -             | Statements inserted before the initial input                                |
| `append`  | array of message object | optional    | -             | Statements inserted after the initial input                                 |
| `replace` | array of replace rule   | optional    | -             | Rules that rewrite the `content` of every message via literal/regex replace |

Message object configuration description:
| Name      | Data Type   | Requirement | Default Value | Description |
|-----------|-------------|-------------|---------------|-------------|
| `role`    | string      | required    | -             | Role        |
| `content` | string      | required    | -             | Message     |

Replace rule configuration description:
| Name          | Data Type | Requirement | Default Value | Description                                                                                |
|---------------|-----------|-------------|---------------|--------------------------------------------------------------------------------------------|
| `pattern`     | string    | required    | -             | Text to match. Compiled as a Go RE2 regex when `regex` is true.                            |
| `replacement` | string    | required    | -             | Replacement text. Supports `$1`, `$2`, ... back-references when `regex` is true.            |
| `on_role`     | string    | optional    | -             | Apply only to messages whose `role` equals this value. Empty/missing means any role.        |
| `regex`       | bool      | optional    | false         | Whether to interpret `pattern` as a regular expression.                                    |

Notes:

- `replace` rules run against the **final** assembled `messages` array (`prepend` + original messages + `append`) in declaration order, so multiple rules compose predictably.
- A message is rewritten only when its `content` is a plain string. Multimodal `content` (arrays/objects, e.g. vision payloads) is left untouched to preserve the request structure.
- `pattern` must not be empty. If `regex: true` and the pattern fails to compile, plugin start-up fails fast instead of erroring at request time.

## Example
An example configuration is as follows:
```yaml
prepend:
- role: system
  content: "Please answer the questions in English."
append:
- role: user
  content: "After answering each question, try to ask a follow-up question."
```

Using the above configuration to initiate a request:
```bash
curl http://localhost/test \
-H "content-type: application/json" \
-d '{
  "model": "gpt-3.5-turbo",
  "messages": [
    {
      "role": "user",
      "content": "Who are you?"
    }
  ]
}
```

After processing through the plugin, the actual request will be:
```bash
curl http://localhost/test \
-H "content-type: application/json" \
-d '{
  "model": "gpt-3.5-turbo",
  "messages": [
    {
      "role": "system",
      "content": "Please answer the questions in English."
    },
    {
      "role": "user",
      "content": "Who are you?"
    },
    {
      "role": "user",
      "content": "After answering each question, try to ask a follow-up question."
    }
  ]
}
```

## Replacing message content (`replace`)

`replace` rewrites the `content` text of every message in the **final** request using literal or regular-expression substitutions. It is useful for:

- Rewriting brand/product names that downstream models or gateways flag (for example, normalizing "OpenClaw" to "agent");
- Centrally cleaning up system prompts without changing each client;
- Light-weight redaction of user input such as phone numbers or API keys.

Example configuration:

```yaml
replace:
- on_role: system
  pattern: "OpenClaw"
  replacement: "agent"
- pattern: "secret-\\d+"
  replacement: "[REDACTED]"
  regex: true
```

Using the above configuration to initiate a request:

```bash
curl http://localhost/test \
-H "content-type: application/json" \
-d '{
  "model": "gpt-3.5-turbo",
  "messages": [
    {"role": "system", "content": "You are running inside OpenClaw."},
    {"role": "user", "content": "Show OpenClaw secret-1234 to the user"}
  ]
}'
```

After processing through the plugin, the actual request will be:

```bash
curl http://localhost/test \
-H "content-type: application/json" \
-d '{
  "model": "gpt-3.5-turbo",
  "messages": [
    {"role": "system", "content": "You are running inside agent."},
    {"role": "user", "content": "Show OpenClaw [REDACTED] to the user"}
  ]
}'
```

Notes:

- The first rule is gated on `on_role: system`, so the `OpenClaw` mention inside the user message is left as-is.
- The second rule has no `on_role`, so it applies to messages of any role and rewrites `secret-1234` to `[REDACTED]`.

## Based on the geo-ip plugin's capabilities, extend AI Prompt Decorator plugin to carry user geographic location information.
If you need to include user geographic location information before and after the LLM's requests, please ensure both the geo-ip plugin and the AI Prompt Decorator plugin are enabled. Moreover, in the same request processing phase, the geo-ip plugin's priority must be higher than that of the AI Prompt Decorator plugin. First, the geo-ip plugin will calculate the user's geographic location information based on the user's IP, and then pass it to subsequent plugins via request attributes. For instance, in the default phase, the geo-ip plugin's priority configuration is 1000, while the ai-prompt-decorator plugin's priority configuration is 500.

Example configuration for the geo-ip plugin:
```yaml
ipProtocal: "ipv4"
```

An example configuration for the AI Prompt Decorator plugin is as follows:
```yaml
prepend:
- role: system
  content: "The user's current geographic location is, country: ${geo-country}, province: ${geo-province}, city: ${geo-city}."
append:
- role: user
  content: "After answering each question, try to ask a follow-up question."
```

Using the above configuration to initiate a request:
```bash
curl http://localhost/test \
-H "content-type: application/json" \
-H "x-forwarded-for: 87.254.207.100,4.5.6.7" \
-d '{
  "model": "gpt-3.5-turbo",
  "messages": [
    {
      "role": "user",
      "content": "How is the weather today?"
    }
  ]
}'
```

After processing through the plugin, the actual request will be:
```bash
curl http://localhost/test \
-H "content-type: application/json" \
-H "x-forwarded-for: 87.254.207.100,4.5.6.7" \
-d '{
  "model": "gpt-3.5-turbo",
  "messages": [
    {
      "role": "system",
      "content": "The user's current geographic location is, country: China, province: Beijing, city: Beijing."
    },
    {
      "role": "user",
      "content": "How is the weather today?"
    },
    {
      "role": "user",
      "content": "After answering each question, try to ask a follow-up question."
    }
  ]
}'
```
