---
title: Key Authentication
keywords: [higress,key auth]
description: Key Authentication Plugin Configuration Reference
---
## Function Description
The `key-auth` plugin implements authentication based on API Key, supporting the parsing of the API Key from HTTP request URL parameters or request headers, while also verifying whether the API Key has permission to access the resource.

## Runtime Properties
Plugin Execution Phase: `Authentication Phase`
Plugin Execution Priority: `310`

## Configuration Fields
**Note:**
- Authentication and authorization configurations cannot coexist within a single rule.
- For requests that are authenticated, a header field `X-Mse-Consumer` will be added to identify the caller's name.

### Authentication Configuration
| Name          | Data Type        | Requirements                                    | Default Value | Description                                                                                                                                                                            |
| ------------- | ---------------- | ----------------------------------------------- | ------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `global_auth` | bool             | Optional (**Instance-Level Configuration Only**) | -             | Can only be configured at the instance level; if set to true, the authentication mechanism takes effect globally; if set to false, it only applies to the configured hostnames and routes. If not configured, it will only take effect globally when no hostname and route configurations are present (to maintain compatibility with older user habits). |
| `consumers`   | array of object  | Required                                        | -             | Configures the service callers for request authentication.                                                                                                                                  |
| `keys`        | array of string  | Required                                        | -             | Source field names for the API Key, which can be URL parameters or HTTP request header names.                                                                                           |
| `in_query`    | bool             | At least one of `in_query` and `in_header` must be true | true          | When configured as true, the gateway will attempt to parse the API Key from URL parameters.                                                                                             |
| `in_header`   | bool             | At least one of `in_query` and `in_header` must be true | true          | When configured as true, the gateway will attempt to parse the API Key from HTTP request headers.                                                                                      |

The configuration field descriptions for each item in `consumers` are as follows:
| Name         | Data Type | Requirements | Default Value | Description                   |
| ------------ | --------- | ------------ | ------------- | ------------------------------ |
| `credential` | string    | Either `credential` or `credentials` must be configured | -             | Configures one access credential for this consumer. |
| `credentials` | array of string | Either `credential` or `credentials` must be configured | -             | Configures multiple access credentials for this consumer. Cannot be used together with `credential`. |
| `name`       | string    | Required     | -             | Configures the name for this consumer.     |

### Authorization Configuration (Optional)
| Name        | Data Type        | Requirements                                    | Default Value | Description                                                                                                                                                           |
| ----------- | ---------------- | ----------------------------------------------- | ------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `allow`     | array of string  | Optional (**Non-Instance Level Configuration**) | -             | Can only be configured on fine-grained rules such as routes or hostnames; specifies the allowed consumers for matching requests, allowing for fine-grained permission control. |

## Configuration Example
### Global Configuration for Authentication and Granular Route Authorization
The following configuration will enable Key Auth authentication and authorization for specific routes or hostnames in the gateway. The `credential` or `credentials` values must not repeat.

At the instance level, do the following plugin configuration:
```yaml
global_auth: false
consumers:
- credential: 2bda943c-ba2b-11ec-ba07-00163e1250b5
  name: consumer1
- credential: c8c8e9ca-558e-4a2d-bb62-e700dcc40e35
  name: consumer2
keys:
- apikey
- x-api-key
```

For routes route-a and route-b, do the following configuration:
```yaml
allow:
- consumer1
```

For the hostnames *.example.com and test.com, do the following configuration:
```yaml
allow:
- consumer2
```

**Note:**
The routes route-a and route-b specified in this example refer to the route names filled in when creating the gateway routes. When matched with these two routes, requests from the caller named consumer1 will be allowed while others will be denied.

The specified hostnames *.example.com and test.com are used to match the request's domain name. When a domain name is matched, callers named consumer2 will be allowed while others will be denied.

Based on this configuration, the following requests will be allowed:

Assuming the following request matches route-a:
**Setting API Key in URL Parameters**
```bash
curl  http://xxx.hello.com/test?apikey=2bda943c-ba2b-11ec-ba07-00163e1250b5
```

**Setting API Key in HTTP Request Headers**
```bash
curl  http://xxx.hello.com/test -H 'x-api-key: 2bda943c-ba2b-11ec-ba07-00163e1250b5'
```

After successful authentication and authorization, the request's header will have an added `X-Mse-Consumer` field with the value `consumer1`, to identify the name of the caller.

The following requests will be denied access:
**Request without an API Key returns 401**
```bash
curl  http://xxx.hello.com/test
```

**Request with an invalid API Key returns 401**
```bash
curl  http://xxx.hello.com/test?apikey=926d90ac-ba2e-11ec-ab68-00163e1250b5
```

**Caller matched with provided API Key has no access rights, returns 403**
```bash
# consumer2 is not in the allow list of route-a
curl  http://xxx.hello.com/test?apikey=c8c8e9ca-558e-4a2d-bb62-e700dcc40e35
```

### Enabling at the Instance Level
The following configuration will enable Basic Auth authentication at the instance level for the gateway, requiring all requests to pass authentication before accessing.

```yaml
global_auth: true
consumers:
- credential: 2bda943c-ba2b-11ec-ba07-00163e1250b5
  name: consumer1
- credential: c8c8e9ca-558e-4a2d-bb62-e700dcc40e35
  name: consumer2
keys:
- apikey
- x-api-key
```

## Related Error Codes
| HTTP Status Code | Error Message                                              | Reason Explanation                |
| ---------------- | ---------------------------------------------------------- | --------------------------------- |
| 401              | Request denied by Key Auth check. Multiple API keys found in request | Multiple API Keys provided in the request.      |
| 401              | Request denied by Key Auth check. No API key found in request | API Key not provided in the request.      |
| 401              | Request denied by Key Auth check. Invalid API key         | The current API Key is not authorized for access. |
| 403              | Request denied by Key Auth check. Unauthorized consumer   | The caller does not have access permissions.  |
