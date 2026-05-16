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
 - `deny_words` supports multi-word keywords with spaces (e.g., `hello world`, `credit card`). The plugin automatically uses substring matching for keywords containing spaces.
