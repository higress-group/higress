---
title: AI Data Masking
keywords: [higress, ai data masking]
description: AI Data Masking Plugin Configuration Reference
---
## Function Description
  Interception and replacement of sensitive words in requests/responses
![image](https://img.alicdn.com/imgextra/i4/O1CN0156Wtko1T9JO0RiWow_!!6000000002339-0-tps-1314-638.jpg)

### Data Handling Scope
  - openai protocol: Request/response conversation content
  - jsonpath: Only process specified fields
  - raw: Entire request/response body

### Sensitive Word Interception
  - Directly intercept sensitive words in the data handling scope and return preset error messages
  - Supports system's built-in sensitive word library and custom sensitive words

### Sensitive Word Replacement
  - Replace sensitive words in request data with masked strings before passing to back-end services. Ensures that sensitive data does not leave the domain
  - Some masked data can be restored after being returned by the back-end service
  - Custom rules support standard regular expressions and grok rules, and replacement strings support variable substitution

## Execution Properties
Plugin Execution Phase: `Authentication Phase`  
Plugin Execution Priority: `991`

## Configuration Fields
| Name                   | Data Type       | Default Value | Description                          |
| ---------------------- | ---------------- | -------------- | ------------------------------------ |
|  deny_openai           | bool             | true           |  Intercept openai protocol          |
|  deny_jsonpath         | string           |   []           |  Intercept specified jsonpath       |
|  deny_raw              | bool             | false          |  Intercept raw body                 |
|  system_deny           | bool             | false          |  Enable built-in interception rules  |
|  deny_code             | int              | 200            |  HTTP status code when intercepted   |
|  deny_message          | string           | Sensitive words found in the question or answer have been blocked | AI returned message when intercepted |
|  deny_raw_message      | string           | {"errmsg":"Sensitive words found in the question or answer have been blocked"} | Content returned when not openai intercepted |
|  deny_content_type     | string           | application/json | Content type header returned when not openai intercepted |
|  deny_words            | array of string  | []             | Custom sensitive word list           |
|  replace_roles         | array            |   -            | Custom sensitive word regex replacement |
|  replace_roles.regex   | string           |   -            | Rule regex (built-in GROK rule)    |
|  replace_roles.type    | [replace, hash]  |   -            | Replacement type                     |
|  replace_roles.restore  | bool             | false          | Whether to restore                   |
|  replace_roles.value    | string          |   -            | Replacement value (supports regex variables) |

## Tool definitions and arguments (opt in)

`tool_arguments.check`, `tool_arguments.restore`, and `tool_definitions.check` default to `false` and do not imply each other. Existing configurations keep the original OpenAI, JSONPath, and raw behavior. A rule's `restore: true` does not authorize tool argument restoration.

| Field | Type | Default | Purpose |
| --- | --- | --- | --- |
| `tool_arguments.check` | bool | false | Deny/mask request arguments; deny response arguments |
| `tool_arguments.restore` | bool | false | Restore response arguments through a separate allowlist |
| `tool_arguments.restore_allowlist` | array | [] | `{tool, paths}` entries; required when tool restore is enabled |
| `tool_definitions.check` | bool | false | Deny matching tools/functions definitions; no new definition masking |
| `tool_arguments.limits.max_call_bytes` | positive integer | 1048576 | Per-call input and rewritten arguments, in UTF-8 bytes after outer JSON decoding |
| `tool_arguments.limits.max_buffer_bytes` | positive integer | 4194304 | Retained response parameters: input, rewritten/queued arguments, name and id |
| `tool_arguments.limits.max_calls` | positive integer | 128 | Registered calls requiring processing, including completed calls |

These values are development candidates pending capacity verification before release. The buffer budget is not an RSS cap: parsing trees, protocol encoding, legacy text windows, and HTTP buffering have additional costs. Request history uses only the per-call limit, not response byte/count quotas. Disabled features do not impose these limits on legacy traffic.

The checks use existing `deny_words`, `system_deny`, and `replace_roles`. Arguments include modern `tool_calls[].function.arguments` and legacy `function_call.arguments` in request history, JSON responses, and SSE deltas, independently of `deny_openai`. Definition-only checks do not take over responses. Definitions include string leaves and dynamic schema names under properties, patternProperties, $defs, definitions and related dependency fields.

```yaml
deny_openai: false
tool_arguments:
  check: true
  restore: true
  restore_allowlist:
    - tool: lookup_customer
      paths: ["/email", "/contacts/0/email"]
  limits:
    max_call_bytes: 1048576
    max_buffer_bytes: 4194304
    max_calls: 128
tool_definitions:
  check: false
replace_roles:
  - regex: "%{EMAILADDRESS}"
    type: hash
    restore: true
```

Tool names are case sensitive. Paths are raw RFC 6901 JSON Pointers (`~0` for `~`, `~1` for `/`); an empty pointer selects only a root string. Only exact string values are restored. Wildcards, subtree grants, URI fragments, array append `-`, and leading-zero array indices are unsupported. Invalid escapes, duplicate tools/paths and an empty required allowlist reject configuration. Missing or non-string targets are not restored.

Inner JSON string literals are decoded before inspection. Keys are checked but never masked. Only changed values are encoded again; unchanged key order, whitespace, escapes, duplicate keys and numeric spelling remain intact. Duplicate keys that make an allowlisted target ambiguous reject processing. Missing, null and empty outer arguments mean no parameters. Other arguments requiring processing must be strings containing valid JSON.

SSE arguments are buffered per choice/call until a non-null `finish_reason` (including an empty string), `[DONE]`, or HTTP end. Metadata is forwarded promptly; the complete delta-concatenated tool name authorizes restoration only at completion. Pending text precedes arguments, which precede termination. Full arguments therefore arrive later. Choice indices must be native nonnegative i64 integers; tool indices may also be decimal strings representable as u64. A missing tool index requires an existing id association; array position is not an identity. Legacy calls have separate identities.

Only the delta contract is supported. Cumulative fragments are not guessed or deduplicated, and cannot always be detected reliably; confirm the upstream protocol before enabling this feature. Known non-allowlisted calls in restore-only mode retain their parameters without new restoration quotas. Calls still eligible for restoration fail on quota overflow.

Malformed JSON/SSE, ambiguous identities/restoration, or quota overflow fail the enabled path. Requests use the existing refusal body and `deny_code` (200 by default); JSON responses are replaced; SSE pending output is discarded and subsequent callbacks remain rejected. Already emitted metadata/text cannot be withdrawn. Legal SSE comments, heartbeat/control fields and upstream errors are retained without fabricating successful termination.

New argument masking and legacy rewriting each run the rule table once in their owned scope. Legacy deny retains its original scope. Managed response arguments are restored only under the new switch/allowlist; legacy global restoration cannot cross their boundaries. A legacy rewrite that spans a managed boundary is rejected.

Tool restoration requires actual same-request mappings from rules with `restore: true`. Tool-generated mappings do not expand ordinary content restoration. Conflicting originals are not arbitrarily selected, and restoration is not recursive. Prefer hashes or unique placeholders over static values such as `***`. An allowlist cannot prevent a model or prompt injection from placing secrets into an allowed parameter; the tool executor must still validate authorization, purpose and destinations.

## Configuration Example
```yaml
system_deny: true
deny_openai: true
deny_jsonpath:
  - "$.messages[*].content"
deny_raw: true
deny_code: 200
deny_message: "Sensitive words found in the question or answer have been blocked"
deny_raw_message: "{\"errmsg\":\"Sensitive words found in the question or answer have been blocked\"}"
deny_content_type: "application/json"
deny_words:
  - "Custom sensitive word 1"
  - "Custom sensitive word 2"
replace_roles:
  - regex: "%{MOBILE}"
    type: "replace"
    value: "****"
    # Mobile number  13800138000 -> ****
  - regex: "%{EMAILLOCALPART}@%{HOSTNAME:domain}"
    type: "replace"
    restore: true
    value: "****@$domain"
    # Email  admin@gmail.com -> ****@gmail.com
  - regex: "%{IP}"
    type: "replace"
    restore: true
    value: "***.***.***.***"
    # IP 192.168.0.1 -> ***.***.***.***
  - regex: "%{IDCARD}"
    type: "replace"
    value: "****"
    # ID card number 110000000000000000 -> ****
  - regex: "sk-[0-9a-zA-Z]*"
    restore: true
    type: "hash"
    # hash sk-12345 -> 9cb495455da32f41567dab1d07f1973d
    # The hashed value is provided to the large model, and the hash value will be restored to the original value from the data returned by the large model
```

## Sensitive Word Replacement Example
### User Request Content
  Please change `curl http://172.20.5.14/api/openai/v1/chat/completions -H "Authorization: sk-12345" -H "Auth: test@gmail.com"` to POST method

### Processed Request Large Model Content
  `curl http://***.***.***.***/api/openai/v1/chat/completions -H "Authorization: 48a7e98a91d93896d8dac522c5853948" -H "Auth: ****@gmail.com"` change to POST method

### Large Model Returned Content
  You want to convert a `curl` GET request to a POST request, and this request is sending data to a specific API. Below is the modified `curl` command to send as POST:
```sh
curl -X POST \
     -H "Authorization: 48a7e98a91d93896d8dac522c5853948" \
     -H "Auth: ****@gmail.com" \
     -H "Content-Type: application/json" \
     -d '{"key":"value"}' \
     http://***.***.***.***/api/openai/v1/chat/completions
```
Here are the following modifications made:
- `-X POST` sets the request method to POST.
- `-H "Content-Type: application/json"` sets the `Content-Type` in the request header to `application/json`, which is typically used to inform the server that the data you are sending is in JSON format.
- `-d '{"key":"value"}'` sets the data to be sent, where `'{"key":"value"}'` is a simple example of a JSON object. You need to replace it with the actual data you want to send.

Please note that you need to replace `"key":"value"` with the actual data content you want to send. If your API accepts a different data structure or requires specific fields, please adjust this part according to your actual situation.

### Processed Return to User Content
  You want to convert a `curl` GET request to a POST request, and this request is sending data to a specific API. Below is the modified `curl` command to send as POST:
```sh
curl -X POST \
     -H "Authorization: sk-12345" \
     -H "Auth: test@gmail.com" \
     -H "Content-Type: application/json" \
     -d '{"key":"value"}' \
     http://172.20.5.14/api/openai/v1/chat/completions
```
Here are the following modifications made:
- `-X POST` sets the request method to POST.
- `-H "Content-Type: application/json"` sets the `Content-Type` in the request header to `application/json`, which is typically used to inform the server that the data you are sending is in JSON format.
- `-d '{"key":"value"}'` sets the data to be sent, where `'{"key":"value"}'` is a simple example of a JSON object. You need to replace it with the actual data you want to send.

Please note that you need to replace `"key":"value"` with the actual data content you want to send. If your API accepts a different data structure or requires specific fields, please adjust this part according to your actual situation.

## Related Notes
 - In streaming mode, if the masked words are split across multiple chunks, restoration may not be possible
 - In streaming mode, if sensitive words are split across multiple chunks, there may be cases where part of the sensitive word is returned to the user
 - Grok built-in rule list: https://help.aliyun.com/zh/sls/user-guide/grok-patterns
 - Built-in sensitive word library data source: https://github.com/houbb/sensitive-word-data/tree/main/src/main/resources
 - Since the sensitive word list is matched after tokenizing the text, please set `deny_words` to single words. In the case of multiple words in English, such as `hello world`, the match may not be successful.
