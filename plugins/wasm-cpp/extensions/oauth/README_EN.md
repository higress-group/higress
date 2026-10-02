---
title: OAuth2 Authentication
keywords: [higress,oauth2]
description: OAuth2 authentication plugin configuration reference
---
## Function Description
`OAuth2` plugin implements the capability of issuing OAuth2 Access Tokens based on JWT (JSON Web Tokens), complying with the [RFC9068](https://datatracker.ietf.org/doc/html/rfc9068) specification.

## Runtime Properties
Plugin execution phase: `Authentication Phase`
Plugin execution priority: `350`

## Configuration Fields
### Authentication Configuration
| Name                 | Data Type        | Requirement                                 | Default Value    | Description                                                                                                                                                                       |
| -------------------- | ---------------- | ------------------------------------------- | ---------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `consumers`          | array of object  | Required                                    | -                | Configures the callers of the service for request authentication                                                                                                                |
| `issuer`             | string           | Optional                                    | Higress-Gateway  | Used to fill the issuer in the JWT                                                                                                                                              |
| `auth_path`          | string           | Optional                                    | /oauth2/token    | Specifies the path suffix for issuing Tokens. When configured at the routing level, ensure it matches the corresponding route first. When using API management, create an interface with the same path. |
| `global_credentials` | bool             | Optional                                    | true             | Allows any route to issue credentials for access under the condition of authentication through the consumer.                                                                    |
| `auth_header_name`   | string           | Optional                                    | Authorization    | Specifies which request header to retrieve the JWT from                                                                                                                                                     |
| `token_ttl`          | number           | Optional                                    | 7200             | The time duration in seconds for which the token is valid after issuance.                                                                                                      |
| `clock_skew_seconds` | number           | Optional                                    | 60               | Allowed clock skew when verifying the exp and iat fields of the JWT, in seconds.                                                                                               |
| `keep_token`         | bool             | Optional                                    | true             | Indicates whether to keep the JWT when forwarding to the backend.                                                                                                              |
| `global_auth`        | bool  | Optional (**Instance-level configuration only**) | -                | Can only be configured at the instance level. If set to true, the global authentication mechanism takes effect; if false, the authentication mechanism only takes effect for configured domains and routes; if not configured, global effect occurs only when there are no domain and route configurations (compatible with legacy user habits). |

The configuration fields for each item in `consumers` are as follows:
| Name                    | Data Type         | Requirement | Default Value                                     | Description                        |
| ----------------------- | ------------------| ----------- | ------------------------------------------------- | ---------------------------------- |
| `name`                  | string            | Required    | -                                               | Configures the name of the consumer.  |
| `client_id`             | string            | Required    | -                                               | OAuth2 client id                  |
| `client_secret`         | string            | Required    | -                                               | OAuth2 client secret              |

**Note:**
- For routes with this configuration enabled, if the path suffix matches `auth_path`, the route will not forward to the original target service but will be used to generate a Token.
- If `global_credentials` is disabled, please ensure that the routes enabling this plugin do not precisely match routes. If there is another prefix-matching route, it may lead to unexpected behavior.
- For requests authenticated and authorized, the request header will have an `X-Mse-Consumer` field added to identify the caller's name.
  The gateway **replaces** this header instead of appending to it: any client-supplied `X-Mse-Consumer` is removed
  before the consumer name from this authentication is set. Downstream therefore always reads the gateway's own
  assertion, so a caller cannot forge its identity by sending the header itself.

### Authorization Configuration (Optional)
| Name        | Data Type        | Requirement                                    | Default Value | Description                                                                                                                                                         |
| ----------- | ---------------- | ---------------------------------------------- | -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `allow`     | array of string  | Optional (**Non-instance-level configuration**) | -              | Can only be configured on fine-grained rules such as routes or domains, allowing specified consumers to access requests that meet the matching conditions for fine-grained permission control. |

**Note:**
- Authentication and authorization configurations cannot coexist in one rule.

## Configuration Example
### Route Granularity Configuration Authentication
For the two routes `route-a` and `route-b`, do the following plugin configuration:
```yaml
consumers:
- name: consumer1
  client_id: 12345678-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  client_secret: abcdefgh-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```
At this time, although using the same configuration, the credentials issued under `route-a` cannot be used to access `route-b`, and vice versa.

If you want the same configuration to share credential access permissions, you can configure as follows:
```yaml
global_credentials: true
consumers:
- name: consumer1
  client_id: 12345678-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  client_secret: abcdefgh-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

### Global Configuration Authentication, Route Granularity Authorization
The following configuration will enable Jwt Auth for specific routes or domains on the gateway. Note that if a JWT matches multiple `jwks`, it will hit the first matching `consumer` in the order of configuration.

At the instance level, do the following plugin configuration:
```yaml
global_auth: false
consumers:
- name: consumer1
  client_id: 12345678-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  client_secret: abcdefgh-xxxx-xxxx-xxxx-xxxxxxxxxxxx
- name: consumer2
  client_id: 87654321-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  client_secret: hgfedcba-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

For the routes `route-a` and `route-b`, do the following plugin configuration:
```yaml
allow:
- consumer1
```

For the domains `*.example.com` and `test.com`, do the following plugin configuration:
```yaml
allow:
- consumer2
```

In this example, route names `route-a` and `route-b` refer to the route names filled in when creating the gateway route. When these two routes are matched, it will allow access for the caller with `name` as `consumer1`, and other callers will not be allowed to access.

In this example, the domains `*.example.com` and `test.com` are used to match request domains. When a matching domain is found, it will allow access for the caller with `name` as `consumer2`, while other callers will not be allowed to access.

### Enable at Gateway Instance Level
The following configuration will enable OAuth2 authentication at the gateway instance level, requiring all requests to be authenticated before access.
```yaml
global_auth: true
consumers:
- name: consumer1
  client_id: 12345678-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  client_secret: abcdefgh-xxxx-xxxx-xxxx-xxxxxxxxxxxx
- name: consumer2
  client_id: 87654321-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  client_secret: hgfedcba-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

# Request Example
## Using Client Credential Authorization Mode
### Get AccessToken
```bash
# Get via GET method (recommended)
curl 'http://test.com/oauth2/token?grant_type=client_credentials&client_id=12345678-xxxx-xxxx-xxxx-xxxxxxxxxxxx&client_secret=abcdefgh-xxxx-xxxx-xxxx-xxxxxxxxxxxx'

# Get via POST method (requires matching a route with a real target service first, or the gateway will not read the request Body)
curl 'http://test.com/oauth2/token' -H 'content-type: application/x-www-form-urlencoded' -d 'grant_type=client_credentials&client_id=12345678-xxxx-xxxx-xxxx-xxxxxxxxxxxx&client_secret=abcdefgh-xxxx-xxxx-xxxx-xxxxxxxxxxxx'

# Simply get the access_token field from the response:
{
  "token_type": "bearer",
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uXC9hdCtqd3QifQ.eyJhdWQiOiJkZWZhdWx0IiwiY2xpZW50X2lkIjoiMTIzNDU2NzgteHh4eC14eHh4LXh4eHgteHh4eHh4eHh4eHh4IiwiZXhwIjoxNjg3OTUxNDYzLCJpYXQiOjE2ODc5NDQyNjMsImlzcyI6IkhpZ3Jlc3MtR2F0ZXdheSIsImp0aSI6IjEwOTU5ZDFiLThkNjEtNGRlYy1iZWE3LTk0ODEwMzc1YjYzYyIsInN1YiI6ImNvbnN1bWVyMSJ9.NkT_rG3DcV9543vBQgneVqoGfIhVeOuUBwLJJ4Wycb0",
  "expires_in": 7200
}
```

### AccessToken Request
```bash
curl 'http://test.com' -H 'Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uXC9hdCtqd3QifQ.eyJhdWQiOiJkZWZhdWx0IiwiY2xpZW50X2lkIjoiMTIzNDU2NzgteHh4eC14eHh4LXh4eHgteHh4eHh4eHh4eHh4IiwiZXhwIjoxNjg3OTUxNDYzLCJpYXQiOjE2ODc5NDQyNjMsImlzcyI6IkhpZ3Jlc3MtR2F0ZXdheSIsImp0aSI6IjEwOTU5ZDFiLThkNjEtNGRlYy1iZWE3LTk0ODEwMzc1YjYzYyIsInN1YiI6ImNvbnN1bWVyMSJ9.NkT_rG3DcV9543vBQgneVqoGfIhVeOuUBwLJJ4Wycb0'
```

# Common Error Code Description
| HTTP Status Code | Error Message         | Explanation                                                                  |
| ---------------- | ----------------------| --------------------------------------------------------------------------- |
| 401              | Invalid Jwt token      | JWT not provided in request header, or JWT format is incorrect, or expired, etc. |
| 403              | Access Denied          | No permission to access the current route.                                  |
