---
title: AI Content Security
keywords: [higress, AI, security]
description: Alibaba Cloud content security
---


## Introduction
Integrate with Aliyun content security service for detections of input and output of LLMs, ensuring that application content is legal and compliant.

## Runtime Properties

Plugin Phase: `CUSTOM`
Plugin Priority: `300`

## Configuration
| Name | Type | Requirement | Default | Description |
| ------------ | ------------ | ------------ | ------------ | ------------ |
| `serviceName` | string | requried | - | service name |
| `servicePort` | string | requried | - | service port |
| `serviceHost` | string | requried | - | Host of Aliyun content security service endpoint |
| `accessKey` | string | requried | - | Aliyun accesskey |
| `secretKey` | string | requried | - | Aliyun secretkey |
| `action` | string | requried | - | Aliyun ai guardrails business interface |
| `checkRequest` | bool | optional | false | check if the input is legal |
| `checkResponse` | bool | optional | false | check if the output is legal |
| `requestCheckService` | string | optional | llm_query_moderation | Aliyun yundun service name for input check |
| `responseCheckService` | string | optional | llm_response_moderation | Aliyun yundun service name for output check |
| `requestContentJsonPath` | string | optional | `messages.@reverse.0.content` | Specify the jsonpath of the content to be detected in the request body |
| `responseContentJsonPath` | string | optional | `choices.0.message.content` | Specify the jsonpath of the content to be detected in the response body |
| `responseStreamContentJsonPath` | string | optional | `choices.0.delta.content` | Specify the jsonpath of the content to be detected in the streaming response body |
| `responseContentFallbackJsonPaths` | array | optional | [`choices.0.message.content`, `content.#(type=="text")#.text`] | Fallback paths tried in order when `responseContentJsonPath` extracts empty content; entries equal to the primary path are skipped automatically; set to `[]` to disable fallback explicitly |
| `responseStreamContentFallbackJsonPaths` | array | optional | [`choices.0.delta.content`, `delta.text`] | Streaming fallback paths tried in order when `responseStreamContentJsonPath` extracts empty content; entries equal to the primary path are skipped automatically; set to `[]` to disable fallback explicitly |
| `denyCode` | int | optional | 200 | Response status code when the specified content is illegal |
| `denyMessage` | string | optional | Drainage/non-streaming response in openai format, the answer content is the suggested answer from Alibaba Cloud content security | Response content when the specified content is illegal |
| `protocol` | string | optional | openai | protocol format, `openai` or `original` |
| `contentModerationLevelBar` | string | optional | max | contentModeration risk level threshold, `max`, `high`, `medium` or `low` |
| `promptAttackLevelBar` | string | optional | max | promptAttack risk level threshold， `max`, `high`, `medium` or `low` |
| `sensitiveDataLevelBar` | string | optional | S4 | sensitiveData risk level threshold,  `S4`, `S3`, `S2` or `S1` |
| `customLabelLevelBar` | string | optional | max | Custom label detection risk level threshold, value can be max, high, medium, or low |
| `riskAction` | string | optional | block | Risk action, value can be `block` or `mask`. `block` means blocking requests based on risk level thresholds, `mask` means replacing sensitive fields with desensitized content when API returns mask suggestion. Note: masking only works with MultiModalGuard mode |
| `timeout` | int | optional | 2000 | timeout for lvwang service |
| `bufferLimit` | int | optional | 1000 | Limit the length of each text when calling the lvwang service |
| `consumerRequestCheckService` | map | optional | - | Specify specific request detection services for different consumers |
| `consumerResponseCheckService` | map | optional | - | Specify specific response detection services for different consumers |
| `consumerRiskLevel` | map | optional | - | Specify interception risk levels for different consumers in different dimensions |

Risk level explanations for each detection dimension:

- For content moderation and prompt attack detection (contentModeration, promptAttack):
    - `max`: Detect request/response content but do not block
    - `high`: Block when risk level is `high`
    - `medium`: Block when risk level >= `medium`
    - `low`: Block when risk level >= `low`

- For sensitive data detection (sensitiveData):
    - `S4`: Detect request/response content but do not block
    - `S3`: Block when risk level is `S3`
    - `S2`: Block when risk level >= `S2`
    - `S1`: Block when risk level >= `S1`

- For custom label detection (customLabel):
    - `max`: Detect request/response content but do not block
    - `high`: Block when custom label detection result risk level is `high`
    - Note: The Alibaba Cloud API only returns `high` and `none` for the customLabel dimension, unlike other dimensions which have four levels. Set to `high` to block on detection hit, set to `max` to not block. `medium` and `low` are kept for configuration compatibility but will not be returned by the API.

- For risk action (riskAction):
    - `block`: Block requests based on risk level thresholds for each dimension
    - `mask`: Replace sensitive fields with desensitized content when API returns `Suggestion=mask`, still block when `Suggestion=block`
    - Note: Masking only works with MultiModalGuard mode (action configured as MultiModalGuard), other modes do not support masking

### Deny Response Body

When content is blocked, the plugin (`MultiModalGuard` action) returns the following structured JSON object. The location in the response depends on the protocol:

```json
{
  "blockedDetails": [
    {
      "Type": "contentModeration",
      "Level": "high",
      "Suggestion": "block"
    }
  ],
  "requestId": "AAAAAA-BBBB-CCCC-DDDD-EEEEEEE****",
  "guardCode": 200
}
```

Field descriptions:

| Field | Type | Description |
| --- | --- | --- |
| `blockedDetails` | array | Details of the triggered blocking dimensions. Synthesised from top-level risk signals when the security service returns no detail entries. |
| `blockedDetails[].Type` | string | Risk type: `contentModeration` / `promptAttack` / `sensitiveData` / `maliciousUrl` / `modelHallucination` |
| `blockedDetails[].Level` | string | Risk level: `high` / `medium` / `low` etc. |
| `blockedDetails[].Suggestion` | string | Action recommended by the security service, usually `block` |
| `requestId` | string | Request ID from the security service, for tracing |
| `guardCode` | int | Business code returned by the security service (not an HTTP status code; `200` indicates a successful check that detected a risk) |

How the body is embedded per protocol:

- **`text_generation` (OpenAI non-streaming)**: `choices[0].message.content` carries the human-readable deny text (`denyMessage`, defaults to `Sorry, I cannot answer your question.` when unconfigured); the structure above is placed at `choices[0].x_higress` as an embedded object (not a JSON string)
- **`text_generation` (OpenAI streaming SSE)**: the first frame's `delta.content` carries the human-readable deny text; the structure above is attached only to the last chunk at `choices[0].x_higress` as an embedded object, followed by `data: [DONE]`
- **`text_generation` (`protocol=original`)**: returned directly as the JSON response body (no OpenAI wrapper, no `x_higress`)
- **`image_generation`**: returned directly as the JSON response body (HTTP 403)
- **`mcp` (JSON-RPC)**: serialised as a JSON string and placed in `error.message`
- **`mcp` (SSE)**: same, returned via SSE event

## Examples of configuration
### Check if the input is legal

```yaml
serviceName: safecheck.dns
servicePort: 443
serviceHost: "green-cip.cn-shanghai.aliyuncs.com"
accessKey: "XXXXXXXXX"
secretKey: "XXXXXXXXXXXXXXX"
checkRequest: true
```

### Check if both the input and output are legal

```yaml
serviceName: safecheck.dns
servicePort: 443
serviceHost: green-cip.cn-shanghai.aliyuncs.com
accessKey: "XXXXXXXXX"
secretKey: "XXXXXXXXXXXXXXX"
checkRequest: true
checkResponse: true
```

### Configure response fallback extraction paths

When primary extraction paths are empty, you can configure ordered fallback paths to support multiple response formats:

```yaml
serviceName: safecheck.dns
servicePort: 443
serviceHost: green-cip.cn-shanghai.aliyuncs.com
accessKey: "XXXXXXXXX"
secretKey: "XXXXXXXXXXXXXXX"
checkResponse: true
responseContentJsonPath: "choices.0.message.content"
responseStreamContentJsonPath: "choices.0.delta.content"
responseContentFallbackJsonPaths:
  - "output.text"
  - 'content.#(type=="text")#.text'
responseStreamContentFallbackJsonPaths:
  - "payload.delta"
  - "delta.text"
```

To enforce strict mode (no fallback), configure both fields as empty arrays:

```yaml
responseContentFallbackJsonPaths: []
responseStreamContentFallbackJsonPaths: []
```

## Observability
### Metric
ai-security-guard plugin provides following metrics:
- `ai_sec_request_deny`: count of requests denied at request phase
- `ai_sec_response_deny`: count of requests denied at response phase

### Trace
ai-security-guard plugin provides following span attributes:
- `ai_sec_risklabel`: risk type of this request
- `ai_sec_deny_phase`: denied phase of this request, value can be request/response

### AI Log
ai-security-guard writes each submission to the content security service into the AI access log, so gateway logs can be correlated with Alibaba Cloud content security requests:

| Field | Type | Description |
| --- | --- | --- |
| `safecheck_requests` | array | Submission event array. Each item is `{"requestId"?: string, "phase": string, "modality": string, "result": string}` |
| `safecheck_request_ids` | array | All valid content security `RequestId` values for the current gateway request, preserved in submission completion order without deduplication or truncation |
| `safecheck_request_id` | string | The latest valid content security `RequestId`, kept for consumers that only read a single value |
| `safecheck_status` | string | Legacy compatibility field reflecting the last status transition for this gateway request (see enum below) |
| `safecheck_request_rt` / `safecheck_response_rt` | int | Latency (ms) of the security check during the request / response phase |
| `safecheck_riskLabel` / `safecheck_riskWords` | string | Risk label and risk words when a risk is hit (taken from the first result returned by the security service) |

`safecheck_requests[].phase` is `request` or `response`; `modality` is `text`, `image`, or `mcp`; `result` describes **the processing outcome of that submission event itself** (not the gateway's final outbound action). Values:

| `result` value | Meaning |
| --- | --- |
| `pass` | The submission passed the check |
| `deny` | The submission hit a risk; the gateway returned a deny response |
| `mask` | The submission hit a risk with `Action=Mask`; the security service returned desensitized text and the request body was rewritten |
| `error` | The submission itself failed (HTTP non-200, business `Code` non-200, unmarshal failure, deny-response build failure, dispatch failure, etc.). When the failure occurs in the **streaming response callback** because building the deny response failed, the gateway fails open (injects buffered upstream content as-is); in that case `safecheck_status=build_fallback_pass` and the corresponding event has `result=error` to indicate the security submission did not complete |

The plugin writes `requestId`, `safecheck_request_ids`, and `safecheck_request_id` only when the security service response contains a JSON string `RequestId` and `strings.TrimSpace(RequestId) != ""`; missing, empty, whitespace-only, or non-string values do not produce empty placeholders.

Every submission attempt emits one `safecheck_requests` event, including HTTP non-200 responses, business failures, and failures to dispatch the security service call. These error paths are recorded as `result=error`. Use `safecheck_requests` for precise auditing across multiple submissions, streaming chunks, or multiple image checks.

`safecheck_status` enum (legacy field; overwritten on each status transition, so only the last transition's value is preserved when there are multiple submissions):

| `safecheck_status` value | Meaning |
| --- | --- |
| `request pass` | All request-phase submissions passed |
| `request mask` | A request-phase submission hit mask; the request body was rewritten with desensitized text |
| `reqeust deny` | A request-phase submission hit a risk; the gateway returned a deny response (note: typo `reqeust` is preserved for backward compatibility) |
| `request error` | A request-phase security submission itself failed (HTTP / unmarshal / dispatch / etc.); the gateway fails open |
| `response pass` | All response-phase submissions passed |
| `response deny` | A response-phase submission hit a risk; the gateway returned a deny response |
| `response error` | A response-phase security submission itself failed; the gateway fails open |
| `build_fallback_pass` | In the streaming response callback, building the deny response failed; the gateway fails open and injects the buffered upstream content as-is |
