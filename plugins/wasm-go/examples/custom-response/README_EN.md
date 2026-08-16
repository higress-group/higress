---
title: Custom Response
keywords: [higress, custom response]
description: Custom response plugin configuration reference
---
## Function Description
The `custom-response` plugin supports the configuration of custom responses, including custom HTTP response status codes, HTTP response headers, and HTTP response bodies. It can be used for Mock responses or for providing custom responses based on specific status codes, such as implementing custom responses when triggering the gateway rate-limiting policy.

## Running Attributes
Plugin Execution Phase: `Authentication Phase`

Plugin Execution Priority: `910`

## Configuration Fields
### New version - Supports multiple returns
| Name                  | Data Type            | Requirements     | Default Value | Description         |
|---------------------|-----------------|----------|-----|------------|
| rules              | array of object           | Required | -   | rule array |

The configuration field description of `rules` is as follows：

| Name               | Data Type                 | Requirements | Default Value | Description                                                                                                                                                                             |
|--------------------|---------------------------|--------------|-----|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `status_code`      | number                    | Optional     | 200 | Custom HTTP response status code                                                                                                                                                        |
| `headers`          | array of string           | Optional     | -   | Custom HTTP response headers, keys and values separated by `=`                                                                                                                          |
| `body`             | string                    | Optional     | -   | Custom HTTP response body                                                                                                                                                               |
| `enable_on_status` | array of string or number | Optional     | -   | Match the original status code to generate a custom response. You can fill in the exact value such as :`200`,`404`, etc., you can also fuzzy match such as: `2xx` to match the status code between 200-299, `20x` to match the status code between 200-209, x represents any digit. If enable_on_status is not specified, the original status code is not determined and the first rule with ENABLE_ON_status left blank is used as the default rule |

#### Fuzzy matching rule
* Length is 3
* At least one digit
* At least one x(case insensitive)

| rule | Matching content                                                                                     |
|------|------------------------------------------------------------------------------------------|
| 40x  | 400-409; If the first two digits are 40                                                                      |
| 1x4  | 104,114,124,134,144,154,164,174,184,194；The first and third positions are 1 and 4 respectively                              |
| x23  | 023,123,223,323,423,523,623,723,823,923；The second and third positions are 23                                  |  
| 4xx  | 400-499；The first digit is 4                                                                         |
| x4x  | 040-049,140-149,240-249,340-349,440-449,540-549,640-649,740-749,840-849,940-949；The second digit is 4 |
| xx4  | When the mantissa is 4                                                                                 |

Matching priority: Exact Match > Fuzzy Match > Default configuration (the first enable_on_status parameter is null)

## Old version - Only one return is supported
| Name | Data Type | Requirements | Default Value | Description                                                                                        |
| -------- | -------- | -------- | -------- |----------------------------------------------------------------------------------------------------|
|  `status_code`    |  number     |  Optional      |   200  | Custom HTTP response status code                                                                   |
|  `headers`     |  array of string      |  Optional     |   -  | Custom HTTP response headers, keys and values separated by `=`                                     |
|  `body`      |  string    |  Optional     |   -   | Custom HTTP response body                                                                          |
|  `enable_on_status`   |  array of number    |   Optional     |  -  | Match original status codes to generate custom responses; if not specified, the original status code is not checked |


## Configuration Example

### Different status codes for different response scenarios

```yaml
rules:
  - body: '{"hello":"world 200"}'
    enable_on_status:
      - '200'
      - '201'
    headers:
      - key1=value1
      - key2=value2
    status_code: 200
  - body: '{"hello":"world 404"}'
    enable_on_status:
      - '404'
    headers:
      - key1=value1
      - key2=value2
    status_code: 200
```

According to this configuration 200 201 requests will return a custom response as follows：

```text
HTTP/1.1 200 OK
Content-Type: application/json
key1: value1
key2: value2
Content-Length: 21

{"hello":"world 200"}
```
According to this configuration 404 requests will return a custom response as follows：

```text
HTTP/1.1 200 OK
Content-Type: application/json
key1: value1
key2: value2
Content-Length: 21

{"hello":"world 400"}
```
With this configuration, 404 response will return the following custom response:
```text
HTTP/1.1 200 OK
Content-Type: application/json
key1: value1
key2: value2
Content-Length: 21

{"hello":"world 404"}
```

### Fuzzy matching scene

```yaml
rules:
  - body: '{"hello":"world 200"}'
    enable_on_status:
      - 200
    headers:
      - key1=value1
      - key2=value2
    status_code: 200
  - body: '{"hello":"world 40x"}'
    enable_on_status:
      - '40x'
    headers:
      - key1=value1
      - key2=value2
    status_code: 200
```

According to this configuration, the status 200 will return a custom reply as follows：

```text
HTTP/1.1 200 OK
Content-Type: application/json
key1: value1
key2: value2
Content-Length: 21

{"hello":"world 200"}
```
According to this configuration, the status code between 401-409 will return a custom reply as follows：

```text
HTTP/1.1 200 OK
Content-Type: application/json
key1: value1
key2: value2
Content-Length: 21

{"hello":"world 40x"}
```

### Mock Response Scenario
```yaml
enable_on_status:
  - 200
status_code: 200
headers:
  - Content-Type=application/json
  - Hello=World
body: "{\"hello\":\"world\"}"
```
With this configuration, 200/201 response will return the following custom response:
```text
HTTP/1.1 200 OK
Content-Type: application/json
key1: value1
key2: value2
Content-Length: 21

{"hello":"world"}
```

### Custom Response on Rate Limiting
```yaml
enable_on_status:
- 429
status_code: 302
headers:
- Location=https://example.com
```
When the gateway rate limiting is triggered, it generally returns the `429` status code, and the request will return the following custom response:
```text
HTTP/1.1 302 Found
Location: https://example.com
```
This achieves the goal of redirecting users who have been rate-limited to another page based on the browser's 302 redirect mechanism, which could be a static page on a CDN.

If you wish to return other responses normally when rate limiting is triggered, just refer to the Mock response scenario to configure the relevant fields accordingly.
