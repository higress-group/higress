---
title: Basic Authentication
keywords: [higress,basic auth]
description: Basic authentication plugin configuration reference
---
## Function Description
The `basic-auth` plugin implements authentication and authorization based on the HTTP Basic Auth standard.

## Operation Attributes
Plugin execution stage: `Authentication Phase`  
Plugin execution priority: `320`

## Configuration Fields
**Note:**
- In one rule, authentication configurations and authorization configurations cannot coexist.
- For requests that pass authentication, the request header will include an `X-Mse-Consumer` field to identify the caller's name.

### Authentication Configuration
| Name          | Data Type        | Requirements                   | Default Value | Description                                                                                                                                                                            |
| ------------- | ---------------- | ------------------------------- | -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `global_auth` | bool             | Optional (**instance-level only**) | -              | Can only be configured at the instance level. If set to true, the authentication mechanism will take effect globally; if set to false, it will only take effect for the configured domains and routes. If not configured, it will only take effect globally when there are no domain and route configurations (compatible with old user habits). |
| `consumers`   | array of object  | Required                        | -              | Configures the service callers for request authentication.                                                                                                                                           |

Each configuration field in `consumers` is described as follows:
| Name         | Data Type | Requirements | Default Value | Description                     |
| ------------ | --------- | ------------ | ------------- | ------------------------------- |
| `credential` | string    | Required     | -             | Configures the access credentials for this consumer. |
| `name`       | string    | Required     | -             | Configures the name of this consumer.     |

### Authorization Configuration (Optional)
| Name             | Data Type        | Requirements                                          | Default Value | Description                                               |
| ---------------- | ---------------- | ---------------------------------------------------- | -------------- | -------------------------------------------------------- |
| `allow`          | array of string  | Required                                             | -              | Configures the consumer names allowed to access for matching requests. |

## Configuration Example
### Global Authentication and Route Granularity Authorization
The following configuration will enable Basic Auth authentication and authorization for specific routes or domains of the gateway. Note that the username and password in the credential information are separated by ":", and the `credential` field cannot be duplicated.

Make the following plugin configuration at the instance level:
```yaml
consumers:
- credential: 'admin:123456'
  name: consumer1
- credential: 'guest:abc'
  name: consumer2
global_auth: false
```

For routes `route-a` and `route-b`, configure as follows:
```yaml
allow:
- consumer1
```

For the domains `*.example.com` and `test.com`, configure as follows:
```yaml
allow:
- consumer2
```

If configured in the console, the specified `route-a` and `route-b` refer to the route names filled in when creating the routes in the console. When matching these two routes, callers with the name `consumer1` will be allowed access, while other callers will not.

The specified `*.example.com` and `test.com` are used to match the request domain. When a match is found, callers with the name `consumer2` will be allowed access, while other callers will not.

Based on this configuration, the following requests may be allowed access:
**Request with specified username and password**
```bash
# Assuming the following request matches the route-a route
# Using curl's -u parameter to specify
curl -u admin:123456  http://xxx.hello.com/test
# Or directly specify the Authorization request header with the username and password encoded in base64
curl -H 'Authorization: Basic YWRtaW46MTIzNDU2'  http://xxx.hello.com/test
```

After successful authentication, the request header will have an added `X-Mse-Consumer` field, which in this case is `consumer1` to identify the caller's name.

The following requests will be denied access:
**Request without username and password, returns 401**
```bash
curl  http://xxx.hello.com/test
```

**Request with incorrect username and password, returns 401**
```bash
curl -u admin:abc  http://xxx.hello.com/test
```

**Caller matched by username and password has no access, returns 403**
```bash
# consumer2 is not in the allow list for route-a
curl -u guest:abc  http://xxx.hello.com/test
```

## Related Error Codes
| HTTP Status Code | Error Message                                                                         | Reason Description               |
| ---------------- | ------------------------------------------------------------------------------------- | -------------------------------- |
| 401              | Request denied by Basic Auth check. No Basic Authentication information found.      | Request did not provide credentials.         |
| 401              | Request denied by Basic Auth check. Invalid username and/or password.               | Request credentials are invalid.           |
| 403              | Request denied by Basic Auth check. Unauthorized consumer.                          | The caller making the request does not have access. |
