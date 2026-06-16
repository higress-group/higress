---
title: JWT Authentication
keywords: [higress,jwt auth]
description: JWT Authentication plugin configuration reference
---

## Function Description
`jwt-auth` plugin implements authentication and authorization based on JWT (JSON Web Tokens). It supports extracting JWT from HTTP request URL parameters, request headers, and Cookie fields, while verifying whether the Token has the necessary permissions to access the resource. 

The difference between this plugin and the JWT authentication in `Security Capability -> Authentication and Authorization` is that it additionally provides the capability of identifying the caller's identity, supporting different JWT credentials for different callers.

## Runtime Properties
Plugin execution phase: `Authentication Phase`
Plugin execution priority: `340`

## Configuration Fields
**Note:**
- In one rule, authentication configuration and authorization configuration cannot coexist.
- For requests authenticated through authentication and authorization, the request header will be augmented with an `X-Mse-Consumer` field to identify the caller's name.

### Authentication Configuration
| Name        | Data Type        | Requirements                                   | Default Value | Description                                                  |
| ----------- | --------------- | ---------------------------------------------- | ------------- | ----------------------------------------------------------- |
| `global_auth` | bool            | Optional (**instance-level configuration only**) | -             | Can only be configured at the instance level. If set to true, it will globally enable the authentication mechanism; if set to false, it will only apply to the configured domain names and routes. If not configured, it will only globally take effect when no domain names and routes are configured (to be compatible with old user habits). |
| `consumers` | array of object | Required                                       | -             | Configure service consumers for request authentication       |

The configuration fields for each item in `consumers` are as follows:
| Name                    | Data Type         | Requirements | Default Value                                      | Description                     |
| ----------------------- | ------------------ | ------------ | -------------------------------------------------- | ------------------------------- |
| `name`                  | string             | Required     | -                                                  | The name of the consumer        |
| `jwks`                  | string             | Required for wasm-cpp; required for wasm-go unless `remote_jwks` is set | - | JSON format string specified by https://www.rfc-editor.org/rfc/rfc7517, consisting of the public key (or symmetric key) used to verify the JWT signature. |
| `remote_jwks`           | object             | Optional in the wasm-go implementation | -                         | Remote JWKS service reference. The referenced service must already be configured or discovered by Higress, for example via McpBridge. |
| `jwks_cache_duration`   | number             | Optional in the wasm-go implementation | 600                       | Remote JWKS cache duration, in seconds. Maximum: 604800. |
| `jwks_fetch_timeout`    | number             | Optional in the wasm-go implementation | 1500                      | Remote JWKS fetch timeout, in milliseconds. Maximum: 10000. |
| `issuer`                | string             | Required     | -                                                  | The issuer of the JWT, must match the `iss` field in the payload. |
| `claims_to_headers`     | array of object    | Optional     | -                                                  | Extract the specified fields from the JWT payload and set them in the specified request headers to forward to the backend. |
| `from_headers`          | array of object    | Optional     | {"name":"Authorization","value_prefix":"Bearer "} | Extract JWT from the specified request headers. |
| `from_params`           | array of string    | Optional     | access_token                                       | Extract JWT from the specified URL parameters. |
| `from_cookies`          | array of string    | Optional     | -                                                  | Extract JWT from the specified cookies. |
| `clock_skew_seconds`    | number             | Optional     | 60                                               | The allowed clock skew when validating the `exp` and `iat` fields of the JWT, measured in seconds. |
| `keep_token`            | bool               | Optional     | true                                             | Whether to retain the JWT when forwarding to the backend. |

**Note:**
- The default values will only be used when `from_headers`, `from_params`, and `from_cookies` are not all configured.
- `remote_jwks`, `jwks_cache_duration`, and `jwks_fetch_timeout` apply only to the wasm-go implementation. The wasm-cpp implementation still requires `jwks`.
- In the wasm-go implementation, only one of `jwks` and `remote_jwks` can be configured.
- When `remote_jwks` is used, requests fail closed if the remote JWKS fetch fails, the response is invalid, refresh is throttled, or no matching `kid` can be found in a multi-key set. Tokens without `kid` are accepted only when the fetched JWKS contains exactly one key.
- Failed remote JWKS fetches are throttled per remote service reference for 30 seconds. Concurrent requests during the initial cold fetch, and requests during the post-failure throttle window, are denied locally. Expired cached JWKS are served only while a refresh is already in flight.
- Remote JWKS responses larger than 64 KiB are rejected.
- `jwks_cache_duration` must be at least 30 seconds so cache expiry does not undercut the remote JWKS failure backoff window.
- In the wasm-go implementation, empty inline `jwks` key sets are rejected during configuration parsing. When `keep_token` is `false`, tokens extracted from `from_params` are removed from the forwarded request path.

The configuration fields for `remote_jwks` are as follows:
| Name           | Data Type | Requirements | Default Value | Description |
| -------------- | --------- | ------------ | ------------- | ----------- |
| `service_name` | string    | Required     | -             | The Higress service name used to build the outbound cluster. |
| `service_host` | string    | Required     | -             | Host or `:authority` header for the JWKS request, without a port. Use `service_port` for the port. |
| `service_port` | number    | Optional     | 443           | Service port for the JWKS request. |
| `path`         | string    | Required     | -             | JWKS request path, for example `/.well-known/jwks.json`. |

The configuration fields for each item in `from_headers` are as follows:
| Name            | Data Type        | Requirements | Default Value | Description                                     |
| --------------- | ---------------- | ------------ | ------------- | ----------------------------------------------- |
| `name`          | string           | Required     | -             | Extract JWT from the request header.           |
| `value_prefix`  | string           | Required     | -             | Remove the prefix from the request header value, with the remaining part serving as the JWT. |

The configuration fields for each item in `claims_to_headers` are as follows:
| Name            | Data Type        | Requirements | Default Value | Description                                   |
| --------------- | ---------------- | ------------ | ------------- | --------------------------------------------- |
| `claim`         | string           | Required     | -             | The specified field in the JWT payload, must be a string or unsigned integer type. |
| `header`        | string           | Required     | -             | The value of the field extracted from the payload is set to this request header and forwarded to the backend. |
| `override`      | bool             | Optional     | true          | If true, existing headers with the same name will be overridden; if false, they will be appended. |

### Authorization Configuration (Optional)
| Name        | Data Type        | Requirements                                   | Default Value | Description                                                                                           |
| ----------- | --------------- | ---------------------------------------------- | ------------- | ----------------------------------------------------------------------------------------------------- |
| `allow`     | array of string | Optional (**not instance-level configuration**) | -             | Can only be configured on fine-grained rules such as routes or domain names, allowing specified consumers to access matching requests for fine-grained permission control. |

## Configuration Examples
### Global Configuration for Authentication and Route-Level Authorization
Note: If a JWT matches multiple `jwks`, the first matching consumer will be applied according to the configuration order.

Configure the plugin at the instance level as follows:
```yaml
global_auth: false
consumers:
- name: consumer1
  issuer: abcd
  jwks: |
    {
      "keys": [
        {
          "kty": "oct",
          "kid": "123",
          "k": "hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew",
          "alg": "HS256"
        }
      ]
    }
- name: consumer2
  issuer: abc
  jwks: |
    {
      "keys": [
        {
          "kty": "RSA",
          "e": "AQAB",
          "use": "sig",
          "kid": "123",
          "alg": "RS256",
          "n": "i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw"
        }
      ]
    }
```

Configure the following for routes `route-a` and `route-b`:
```yaml
allow:
- consumer1
```

Configure the following for domain names `*.example.com` and `test.com`:
```yaml
allow:
- consumer2
```

**Explanation:**
The specified `route-a` and `route-b` refer to the routing names filled in when creating the gateway route. When these two routes are matched, the caller with the name consumer1 will be allowed access, while others will not be permitted.

The specified `*.example.com` and `test.com` are used to match the request domain names. When a matching domain name is found, the caller with the name consumer2 will be allowed access, while others will not. 

Based on this configuration, the following requests will be allowed:

Suppose the following request matches the route-a.
**Setting the JWT in URL parameters**
```bash
curl  'http://xxx.hello.com/test?access_token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxODY1NjczODE5fQ.-vBSV0bKeDwQcuS6eeSZN9dLTUnSnZVk8eVCXdooCQ4'
```

**Setting the JWT in HTTP request headers**
```bash
curl  http://xxx.hello.com/test -H 'Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxODY1NjczODE5fQ.-vBSV0bKeDwQcuS6eeSZN9dLTUnSnZVk8eVCXdooCQ4'
```

After successful authentication and authorization, the request's header will include an `X-Mse-Consumer` field, in this example with the value `consumer1`, to identify the caller's name.

The following requests will be denied:
**Request without JWT returns 401**
```bash
curl  http://xxx.hello.com/test
```

**Caller matching from the provided JWT has no access permission, returning 403**
```bash
# consumer1 is not in the allow list for *.example.com
curl  'http://xxx.example.com/test' -H 'Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxODY1NjczODE5fQ.-vBSV0bKeDwQcuS6eeSZN9dLTUnSnZVk8eVCXdooCQ4'
```

### Using Remote JWKS (wasm-go implementation only)

If the signing keys are maintained by an identity provider, the wasm-go implementation can fetch and cache the remote JWKS from a configured service reference. The service must already be configured or discovered by Higress.

```yaml
global_auth: false
consumers:
- name: remote-consumer
  issuer: https://issuer.example.com
  remote_jwks:
    service_name: issuer.example.com.dns
    service_host: issuer.example.com
    service_port: 443
    path: /.well-known/jwks.json
  jwks_cache_duration: 600
  jwks_fetch_timeout: 1500
```

When remote JWKS is configured, make sure the referenced service is reachable from the gateway data plane. Remote JWKS responses larger than 64 KiB are treated as invalid responses.

#### Enable at Gateway Instance Level
The following configuration will enable JWT Auth authentication at the instance level, requiring all requests to be authenticated before accessing.
```yaml
global_auth: true
consumers:
- name: consumer1
  issuer: abcd
  jwks: |
    {
      "keys": [
        {
          "kty": "oct",
          "kid": "123",
          "k": "hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew",
          "alg": "HS256"
        }
      ]
    }
- name: consumer2
  issuer: abc
  jwks: |
    {
      "keys": [
        {
          "kty": "RSA",
          "e": "AQAB",
          "use": "sig",
          "kid": "123",
          "alg": "RS256",
          "n": "i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw"
        }
      ]
    }
```

## Common Error Codes
| HTTP Status Code | Error Message                | Reason Description                                                           |
| ---------------- | ----------------------------- | ---------------------------------------------------------------------------- |
| 401              | Jwt missing                   | The request header did not provide a JWT                                   |
| 401              | Jwt expired                   | The JWT has expired                                                         |
| 401              | Jwt verification fails        | JWT payload verification failed, such as mismatched `iss`                 |
| 403              | Access Denied                 | No permission to access the current route                                   |

## Detailed Description
### 1. Token-based Authentication
#### 1.1 Introduction
Many open APIs need to identify the requester's identity and determine whether the requested resource can be returned. A token is a mechanism used for identity verification. With this mechanism, applications do not need to retain user authentication information or session information on the server, allowing for stateless, distributed web application authorization, facilitating application scaling.

#### 1.2 Process Description
![](https://help-static-aliyun-doc.aliyuncs.com/assets/img/zh-CN/2336348951/p135822.png)  
The above image shows the entire business process sequence diagram for gateway authentication using JWT. Below, we will detail the steps indicated in the diagram:

1. The client initiates an authentication request to the API gateway, generally carrying the terminal user's username and password.
2. The gateway forwards the request directly to the backend service.
3. The backend service reads the verification information in the request (such as username and password) for validation. Upon successful verification, it generates a standard token using a private key and returns it to the gateway.
4. The gateway returns a response containing the token to the client, who must cache this token locally.
5. The client sends a business request to the API gateway, carrying the token in the request.
6. The gateway validates the token using the user's set public key. Upon successful validation, it forwards the request to the backend service.
7. The backend service processes the business and responds.
8. The gateway returns the business response to the client.

Throughout this process, the gateway utilizes the token authentication mechanism, enabling the user to authorize their API using their user system. Next, we will introduce the structured token used by the gateway for token authentication: JSON Web Token (JWT).

#### 1.3 JWT
##### 1.3.1 Introduction
JSON Web Token (JWT) is an open standard (RFC7519) that defines a compact and self-contained way for securely transmitting information between parties as a JSON object. JWT can be used as a stand-alone authentication token, capable of containing user identity, user roles, permissions, and other information, aiding in resource retrieval from resource servers and adding any additional claims required by business logic, particularly suitable for login scenarios for distributed sites.

##### 1.3.2 JWT Structure
`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYWRtaW4iOnRydWV9.TJVA95OrM7E2cBab30RMHrHDcEfxjoYZgeFONFh7HgQ`  
As shown above, JWT is a string composed of three parts:  
- Header  
- Payload  
- Signature  

**Header**  
The header of the JWT carries two pieces of information:
- Token type, which is JWT  
- Signing algorithm  

The supported signing algorithms by the gateway are as follows:  
```text
ES256, ES384, ES512,  
HS256, HS384, HS512,  
RS256, RS384, RS512,  
PS256, PS384, PS512,  
EdDSA
```  

The complete header looks like the following JSON:
```js
{
  'typ': 'JWT',
  'alg': 'HS256'
}
```  
Then the header is Base64 encoded (this encoding is symmetrically decodable), forming the first part.  
`eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9`  

**Payload**  
The payload is where valid information is stored. Its details are defined as follows:  
```text
iss: Token issuer. Indicates who created the token, this claim is a string.  
sub: Subject Identifier, an identifier provided by the issuer for its end users, unique within the issuer's scope, up to 255 ASCII characters, case sensitive.  
aud: Audience(s), the audience of the token, an array of strings that are case-sensitive.  
exp: Expiration time, a timestamp of the token's expiration. Tokens expired beyond this time will be invalid. This claim is an integer, representing the number of seconds since January 1, 1970.  
iat: Issuance time of the token, this claim is an integer, representing the number of seconds since January 1, 1970.  
jti: Unique identifier for the token, the value of this claim must be unique for each token created by the token issuer to prevent collisions. It is typically a cryptographically random value. This value adds a random entropy component to the structured token that is not accessible to an attacker, helping prevent token guess and replay attacks.
```  
Custom fields necessary for the user system can also be added, for example, the following example adds a nickname `name`:
```js
{
  "sub": "1234567890",
  "name": "John Doe"
}
```  
Then encode it in Base64 to obtain the second part of the JWT:  
`JTdCJTBBJTIwJTIwJTIyc3ViJTIyJTNBJTIwJTIyMTIzNDU2Nzg5MCUyMiUyQyUwQSUyMCUyMCUyMm5hbWUlMjIlM0ElMjAlMjJKb2huJTIwRG9lJTIyJTBBJTdE`  

**Signature**  
This part requires the Base64-encoded Header and Base64-encoded Payload to be connected by a period, and then encrypted using the signing method declared in the Header (where `$secret` represents the user's private key) to form the third part of the JWT.  
```js
var encodedString = base64UrlEncode(header) + '.' + base64UrlEncode(payload);
var signature = HMACSHA256(encodedString, '$secret');
```  
Connecting these three parts with a period creates a complete string that forms the JWT example at the beginning of section 1.3.2.  

##### 1.3.3 Validity Period
The gateway will validate the `exp` field in the token. Once this field expires, the gateway will consider this token invalid and directly reject the request. The expiration time must be set.  

##### 1.3.4 Characteristics of JWT
1. JWT is not encrypted by default; do not write secret data into JWT.  
2. JWT can be used for both authentication and information exchange. Effectively using JWT can reduce the number of queries to the database on the server.  
3. The biggest drawback of JWT is that since the server does not maintain session state, it cannot revoke a token during use or change the permissions of said token. In other words, once a JWT is issued, it will remain valid until expiration, unless the server implements additional logic.  
4. JWT itself contains authentication information, and once leaked, anyone can gain all permissions of that token. To minimize theft, the validity period of JWT should be set to be relatively short. For some critical permissions, users should be re-authenticated.  
5. To reduce theft, JWT should not be transmitted in plain text over HTTP but should use HTTPS for transmission.

### 2. How User Systems Apply the JWT Plugin to Protect APIs
#### 2.1 Generating a Pair of JWKs (JSON Web Keys)
**Method 1: Online Generation:**  
Users can generate the private and public keys used for token generation and verification at this site https://mkjwk.org. The private key is used by the authorization service to issue JWTs, and the public key is configured into the JWT plugin for the gateway to verify requests. Pay attention to the jwks format configuration used by the gateway. In the image below, the Public Key should be placed into the keys structure, for example: `{"keys":[{"kty":"RSA","e":"AQAB",...}]}`  
<img src="https://help-static-aliyun-doc.aliyuncs.com/assets/img/zh-CN/2336348951/p135823.png" style="zoom:50%" />  

**Method 2: Local Generation:**  
This article demonstrates using Java; users of other languages can find related tools to generate key pairs. Create a new Maven project and include the following dependency:  
```xml
<dependency>
     <groupId>org.bitbucket.b_c</groupId>
     <artifactId>jose4j</artifactId>
     <version>0.7.0</version>
</dependency>
```  
Use the following code to generate a pair of RSA keys:  
```java
RsaJsonWebKey rsaJsonWebKey = RsaJwkGenerator.generateJwk(2048);
final String publicKeyString = rsaJsonWebKey.toJson(JsonWebKey.OutputControlLevel.PUBLIC_ONLY);
final String privateKeyString = rsaJsonWebKey.toJson(JsonWebKey.OutputControlLevel.INCLUDE_PRIVATE);
```  

#### 2.2 Using the Private Key in JWK to Implement the Token Issuance Authentication Service
You will need to use the Keypair JSON string (the first inside the three boxes) generated online in section 2.1 or the locally generated privateKeyString JSON string as the private key to issue tokens for authorizing trusted users to access protected APIs. The specific implementation can refer to the example below. The form of issuing tokens to customers can be determined by the user based on the specific business scenario; it can be deployed in the production environment, configured to be a normal API that visitors can access through username and password, or tokens can be generated locally and directly copied for specified users to use.  
```java
import java.security.PrivateKey;
import org.jose4j.json.JsonUtil;
import org.jose4j.jwk.RsaJsonWebKey;
import org.jose4j.jwk.RsaJwkGenerator;
import org.jose4j.jws.AlgorithmIdentifiers;
import org.jose4j.jws.JsonWebSignature;
import org.jose4j.jwt.JwtClaims;
import org.jose4j.jwt.NumericDate;
import org.jose4j.lang.JoseException;

public class GenerateJwtDemo {
    public static void main(String[] args) throws JoseException  {
        String keyId = "uniq_key";
        // Use the Keypair generated in section 2.1
        String privateKeyJson = "{\n"
            + "  \"kty\": \"RSA\",\n"
            + "  \"d\": \"O9MJSOgcjjiVMNJ4jmBAh0mRHF_TlaVva70Imghtlgwxl8BLfcf1S8ueN1PD7xV6Cnq8YenSKsfiNOhC6yZ_fjW1syn5raWfj68eR7cjHWjLOvKjwVY33GBPNOvspNhVAFzeqfWneRTBbga53Agb6jjN0SUcZdJgnelzz5JNdOGaLzhacjH6YPJKpbuzCQYPkWtoZHDqWTzCSb4mJ3n0NRTsWy7Pm8LwG_Fd3pACl7JIY38IanPQDLoighFfo-Lriv5z3IdlhwbPnx0tk9sBwQBTRdZ8JkqqYkxUiB06phwr7mAnKEpQJ6HvhZBQ1cCnYZ_nIlrX9-I7qomrlE1UoQ\",\n"
            + "  \"e\": \"AQAB\",\n"
            + "  \"alg\": \"RS256\",\n"
            + "  \"n\": \"vCuB8MgwPZfziMSytEbBoOEwxsG7XI3MaVMoocziP4SjzU4IuWuE_DodbOHQwb_thUru57_Efe--sfATHEa0Odv5ny3QbByqsvjyeHk6ZE4mSAV9BsHYa6GWAgEZtnDceeeDc0y76utXK2XHhC1Pysi2KG8KAzqDa099Yh7s31AyoueoMnrYTmWfEyDsQL_OAIiwgXakkS5U8QyXmWicCwXntDzkIMh8MjfPskesyli0XQD1AmCXVV3h2Opm1Amx0ggSOOiINUR5YRD6mKo49_cN-nrJWjtwSouqDdxHYP-4c7epuTcdS6kQHiQERBd1ejdpAxV4c0t0FHF7MOy9kw\"\n"
            + "}";
        JwtClaims claims = new JwtClaims();
        claims.setGeneratedJwtId();
        claims.setIssuedAtToNow();
        // Expiration time must be set
        NumericDate date = NumericDate.now();
        date.addSeconds(120*60);
        claims.setExpirationTime(date);
        claims.setNotBeforeMinutesInThePast(1);
        claims.setSubject("YOUR_SUBJECT");
        claims.setAudience("YOUR_AUDIENCE");
        // Add custom parameters, all values should be String type
        claims.setClaim("userId", "1213234");
        claims.setClaim("email", "userEmail@youapp.com");
        JsonWebSignature jws = new JsonWebSignature();
        jws.setAlgorithmHeaderValue(AlgorithmIdentifiers.RSA_USING_SHA256);
        jws.setKeyIdHeaderValue(keyId);
        jws.setPayload(claims.toJson());
        PrivateKey privateKey = new RsaJsonWebKey(JsonUtil.parseJson(privateKeyJson)).getPrivateKey();
        jws.setKey(privateKey);
        String jwtResult = jws.getCompactSerialization();
        System.out.println("Generate Json Web token , result is " + jwtResult);
    }
}
```
