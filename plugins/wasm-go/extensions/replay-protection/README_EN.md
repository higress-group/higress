---
title: Replay Attack Prevention
keywords: [higress, replay-protection]
description: Configuration reference for the replay attack prevention plugin
---

## Functional Description

The replay prevention plugin prevents request replay attacks by verifying the one-time random number in the request. Each request needs to carry a unique nonce value. The server will record and verify the uniqueness of this value, thus preventing requests from being maliciously replayed.

Specifically, it includes the following functions:

- **Mandatory or Optional Nonce Verification**: It can be configured to determine whether requests are required to carry a nonce value.
- **Nonce Uniqueness Verification Based on Redis**: The nonce value is stored and verified in Redis to ensure its uniqueness.
- **Configurable Nonce Validity Period**: It supports setting the validity period of the nonce, which will automatically expire after the period.
- **Nonce Format and Length Verification**: It supports verifying the format (Base64) and length of the nonce value.
- **Custom Error Response**: It supports configuring the status code and error message when a request is rejected.
- **Customizable Nonce Request Header**: The name of the request header carrying the nonce can be customized.

## Runtime Attributes

Plugin execution stage: `Authentication Stage`
Plugin execution priority: `800`

## Configuration Fields

| Name                | Data Type | Required | Default Value          | Description                              |
|----------------------|--------|------|-----------------|---------------------------------|
| `force_nonce`        | bool   | No   | true      | Whether requests are required to carry a nonce value.       |
| `nonce_header`       | string | No   | `X-Higress-Nonce`   | Specifies the name of the request header carrying the nonce value.       |
| `nonce_ttl`          | int    | No   | 900        | The validity period of the nonce, in seconds.    |
| `nonce_min_length`   | int    | No   | 8            | The minimum length of the nonce value.               |
| `nonce_max_length`   | int    | No   | 128        | The maximum length of the nonce value.               |
| `reject_code`        | int    | No   | 429        | The status code returned when a request is rejected.             |
| `reject_msg`         | string | No   | `Replay Attack Detected` | The error message returned when a request is rejected.           |
| `validate_base64`    | bool    | No   | false | Whether to verify the Base64 encoding format of the nonce. |
| `failure_mode` | string | No | `fail_closed` | Policy for Redis call failures, timeouts, and invalid responses: `fail_closed` returns a fixed 503; `fail_open` allows the request. Choose `fail_open` only when availability takes priority over temporary nonce deduplication. |
| `redis` | Object | Yes   | -              | Redis-related configuration |

Description of each configuration field in `redis`

| Name           | Data Type | Required | Default Value                 | Description|
| -------------- | -------- | ---- |---------------------| --------------------------------------- |
| `service_name` | string   | Yes   | -                   | The name of the Redis service, the complete FQDN name with the service type, such as my-redis.dns, redis.my-ns.svc.cluster.local. |
| `service_port` | int      | No   | 6379                | The port of the Redis service. |
| `username`     | string   | No   | -                   | The username of Redis. |
| `password`     | string   | No   | -                   | The password of Redis. |
| `timeout`      | int      | No   | 1000                | The connection timeout time of Redis, in milliseconds. |
| `database`     | int      | No   | 0                   | The ID of the database to be used. For example, if it is configured as 1, it corresponds to `SELECT 1`. |
| `key_prefix`   | string   | No   | `replay-protection` | The key prefix of Redis, used to distinguish different nonce keys. |

## Configuration Example

The following is a complete configuration example of the replay attack prevention plugin:

```yaml
force_nonce: true
nonce_header: "X-Higress-Nonce"    # Specifies the name of the nonce request header
nonce_ttl: 900                    # The validity period of the nonce, set to 900 seconds
nonce_min_length: 8               # The minimum length of the nonce
nonce_max_length: 128             # The maximum length of the nonce
validate_base64: true             # Whether to enable Base64 format verification
reject_code: 429                  # The HTTP status code returned when a request is rejected
reject_msg: "Replay Attack Detected"  # The error message content returned when a request is rejected
redis:
  service_name: redis.static       # The name of the Redis service
  service_port: 80                # The port used by the Redis service
  timeout: 1000                   # The timeout time of Redis operations (unit: milliseconds)
  key_prefix: "replay-protection" # The key prefix in Redis
```

## Usage Instructions

### Request Header Requirements

| Request Header Name       | Required         | Description                                       |
|-----------------|----------------|------------------------------------------|
| `X-Higress-Nonce`  | Determined by the `force_nonce` configuration | The randomly generated nonce value carried in the request, which needs to conform to the Base64 format. |

> **Note**: The name of the request header can be customized through the `nonce_header` configuration. The default value is `X-Higress-Nonce`.

### Usage Example

```bash
# Generate nonce
nonce=$(openssl rand -base64 32)

# Send request
curl -X POST 'https://api.example.com/path' \
  -H "X-Higress-Nonce: $nonce" \
  -d '{"key": "value"}'
```

## Return Results

```json
{
  "code": 429,
  "message": "Replay Attack Detected"
}
```

## Error Response Examples

| Error Scenario                 | Status Code | Error Message               |
|------------------------|-------|--------------------|
| Missing nonce request header         | 400 | `Missing Required Header` |
| Nonce length does not meet the requirements      | 400 | `Invalid Nonce` |
| Nonce format does not conform to Base64 | 400 | `Invalid Nonce` |
| Nonce has been used (replay attack) | 429 | `Replay Attack Detected` |
