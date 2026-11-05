---
title: HMAC Authentication
keywords: [higress,hmac auth]
description: HMAC Authentication plugin configuration reference
---
## Function Description
The `hmac-auth` plugin implements the generation of tamper-proof signatures for HTTP requests based on the HMAC algorithm, and performs authentication and authorization based on the signature.

## Running Attributes
Plugin execution phase: `Authentication phase`
Plugin execution priority: `330`

## Configuration Fields
**Note:**  
- In a rule, authentication and authorization configurations cannot coexist.  
- For requests that pass authentication and authorization, the request header will be added with an `X-Mse-Consumer` field to identify the caller's name.
  The gateway **replaces** this header instead of appending to it: any client-supplied `X-Mse-Consumer` is removed
  before the consumer name from this authentication is set. Downstream therefore always reads the gateway's own
  assertion, so a caller cannot forge its identity by sending the header itself.

### Authentication Configuration
| Name          | Data Type        | Requirement         | Default Value | Description                                                                                                             |
| ------------- | ---------------- | ------------------- | ------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `global_auth` | bool             | Optional (**Instance level configuration only**) | -             | Can only be configured at the instance level. If set to true, it acts globally; if false, only applies to configured domains and routes. If not configured, it will apply globally only when there are no domain and route configurations (to accommodate old user habits). |
| `consumers`   | array of object  | Mandatory           | -             | Configures the callers of the service for request authentication.                                                     |
| `date_offset` | number           | Optional            | -             | Configures the maximum allowed client time offset, in seconds; parsed based on the request header `Date`; can be used to prevent request replay; no validation is performed if not configured. |

The configuration fields for each item in `consumers` are as follows:
| Name     | Data Type | Requirement | Default Value | Description                                |
| -------- | --------- | ----------- | ------------- | ------------------------------------------- |
| `key`    | string    | Mandatory   | -             | Configures the key extracted from the `x-ca-key` header of the request. |
| `secret` | string    | Mandatory   | -             | Configures the secret used to generate the signature.            |
| `name`   | string    | Mandatory   | -             | Configures the name of the consumer.                |

### Authorization Configuration (Optional)
| Name        | Data Type        | Requirement                                    | Default Value | Description                                                                                                                                                          |
| ----------- | ---------------- | --------------------------------------------- | ------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `allow`     | array of string  | Optional (**Non-instance level configuration**) | -             | Can only be configured on granular rules such as routes or domains. For requests that match the conditions, configure the allowed consumers to achieve fine-grained permission control. |

## Configuration Example
### Global Configuration Authentication and Route Granular Authorization
Configure the following plugin settings at the instance level. Note that the `key` field cannot be duplicated:
```yaml
global_auth: false
consumers:
- key: appKey-example-1
  secret: appSecret-example-1
  name: consumer-1
- key: appKey-example-2
  secret: appSecret-example-2
  name: consumer-2
```
For route-a and route-b, configure the plugin as follows:
```yaml
allow:
- consumer1
```
For the two domains *.example.com and test.com, configure as follows:
```yaml
allow:
- consumer2
```
If configured in the console, the specified route names route-a and route-b correspond to the route names filled in when creating the gateway routes. When matched to these two routes, access will be allowed for the caller named consumer1, while other callers will not be allowed access.

The specified *.example.com and test.com are used to match the domains of the requests. When a domain match is found, access will be allowed for the caller named consumer2, while other callers will not be allowed access.

### Gateway Instance Level Activation
The following configuration will enable HMAC Auth authentication at the gateway instance level, requiring all requests to undergo authentication before access.
```yaml
global_auth: true
consumers:
- key: appKey-example-1
  secret: appSecret-example-1
  name: consumer-1
- key: appKey-example-2
  secret: appSecret-example-2
  name: consumer-2
```

## Signature Mechanism Description
### Configuration Preparation
As mentioned above, configure the credentials required for generating and verifying signatures in the plugin settings.
- key: to be set in the request header `x-ca-key`.
- secret: used for generating request signatures.

### Client Signature Generation Process
#### Overview
The client generates a signature through three main steps:
1. Extract key data from the original request to create a string for signing.
2. Encrypt the key data signing string using the algorithm and the configured `secret` to obtain the signature.
3. Include all relevant headers for the signature into the original HTTP request to form the final HTTP request.

As shown in the figure below:
![](https://help-static-aliyun-doc.aliyuncs.com/assets/img/zh-CN/1745707061/p188113.png)

#### Signing String Extraction Process
The client needs to extract key data from the HTTP request, combine it into a signing string, which has the following format:
```text
HTTPMethod
Accept
Content-MD5
Content-Type
Date
Headers
PathAndParameters
```
The seven fields above constitute the entire signing string, separated by newline characters `\n`. If Headers is empty, no newline is needed; other fields should retain `\n` if empty. The signature is case-sensitive. Below are the extraction rules for each field:
- HTTPMethod: The HTTP method, all uppercase (e.g., POST).
- Accept: The value of the Accept header in the request, can be empty. It is recommended to explicitly set the Accept Header. When Accept is empty, some HTTP clients may set a default value of `*/*`, resulting in a signature verification failure.
- Content-MD5: The value of the Content-MD5 header in the request, can be empty. It is calculated only if there is a Body in the request and it is not in Form format. Here’s a reference calculation method for the Content-MD5 value in Java:
```java
String content-MD5 = Base64.encodeBase64(MD5(bodyStream.getBytes("UTF-8")));
```
- Content-Type: The value of the Content-Type header in the request, can be empty.
- Date: The value of the Date header in the request. If the `date_offset` configuration is not turned on, it can be empty; otherwise, it will be used for time offset verification.
- Headers: Users can select specific headers to participate in the signature. The rules for concatenating the signing header string are as follows:
    - The Keys of the headers participating in the signature calculation are concatenated after being sorted lexicographically, as follows:
    ```text
    HeaderKey1 + ":" + HeaderValue1 + "\n" +
    HeaderKey2 + ":" + HeaderValue2 + "\n" +
    ...
    HeaderKeyN + ":" + HeaderValueN + "\n"
    ```
    - If the Value of a certain header is empty, use HeaderKey + ":" + "\n" to participate in the signature, retaining the Key and the colon.
    - The collection of all participating header Keys is placed in the Header with the key X-Ca-Signature-Headers, separated by commas.
    - The following headers are not included in the header signature calculation: X-Ca-Signature, X-Ca-Signature-Headers, Accept, Content-MD5, Content-Type, Date.
- PathAndParameters: This field includes Path, Query, and all parameters in Form, specifically organized as follows:
```text
Path + "?" + Key1 + "=" + Value1 + "&" + Key2 + "=" + Value2 + ... "&" + KeyN + "=" + ValueN
```
Note:
1. The Key of Query and Form parameters should be sorted lexicographically before being concatenated as above.
2. If Query and Form parameters are empty, just use Path without adding `?`.
3. If the Value of parameters is empty, only the Key should be retained in the signature, the equal sign does not need to be added.
4. In the case of array parameters (parameters with the same key but different values), only the first Value should be used for signature calculation.

#### Signing String Extraction Example
Initial HTTP request:
```text
POST /http2test/test?param1=test HTTP/1.1
host:api.aliyun.com
accept:application/json; charset=utf-8
ca_version:1
content-type:application/x-www-form-urlencoded; charset=utf-8
x-ca-timestamp:1525872629832
date:Wed, 09 May 2018 13:30:29 GMT+00:00
user-agent:ALIYUN-ANDROID-DEMO
x-ca-nonce:c9f15cbf-f4ac-4a6c-b54d-f51abf4b5b44
content-length:33
username=xiaoming&password=123456789
```
The generated correct signing string is:
```text
POST
application/json; charset=utf-8
application/x-www-form-urlencoded; charset=utf-8
Wed, 09 May 2018 13:30:29 GMT+00:00
x-ca-key:203753385
x-ca-nonce:c9f15cbf-f4ac-4a6c-b54d-f51abf4b5b44
x-ca-signature-method:HmacSHA256
x-ca-timestamp:1525872629832
/http2test/test?param1=test&password=123456789&username=xiaoming
```
#### Signature Calculation Process
After the client assembles the key data extracted from the HTTP request into a signing string, it needs to encrypt the signing string and encode it to form the final signature. 

The specific encryption form is as follows, where `stringToSign` is the extracted signing string, `secret` is the one filled in the plugin configuration, and `sign` is the final generated signature:
```java
Mac hmacSha256 = Mac.getInstance("HmacSHA256");
byte[] secretBytes = secret.getBytes("UTF-8");
hmacSha256.init(new SecretKeySpec(secretBytes, 0, secretBytes.length, "HmacSHA256"));
byte[] result = hmacSha256.doFinal(stringToSign.getBytes("UTF-8"));
String sign = Base64.encodeBase64String(result);
```
To summarize, the `stringToSign` is decoded using UTF-8 to obtain a Byte array, then the encryption algorithm is applied to the Byte array, and finally, the Base64 algorithm is used for encoding, forming the final signature.

#### Adding the Signature Process
The client needs to include the following four headers in the HTTP request to transmit to the API gateway for signature verification:
- x-ca-key: The APP Key, mandatory.
- x-ca-signature-method: The signature algorithm, can be HmacSHA256 or HmacSHA1, optional, default is HmacSHA256.
- x-ca-signature-headers: The collection of all signature header Keys, separated by commas, optional.
- x-ca-signature: The signature, mandatory.

Below is an example of the entire HTTP request carrying the signature:
```text
POST /http2test/test?param1=test HTTP/1.1
host:api.aliyun.com
accept:application/json; charset=utf-8
ca_version:1
content-type:application/x-www-form-urlencoded; charset=utf-8
x-ca-timestamp:1525872629832
date:Wed, 09 May 2018 13:30:29 GMT+00:00
user-agent:ALIYUN-ANDROID-DEMO
x-ca-nonce:c9f15cbf-f4ac-4a6c-b54d-f51abf4b5b44
x-ca-key:203753385
x-ca-signature-method:HmacSHA256
x-ca-signature-headers:x-ca-timestamp,x-ca-key,x-ca-nonce,x-ca-signature-method
x-ca-signature:xfX+bZxY2yl7EB/qdoDy9v/uscw3Nnj1pgoU+Bm6xdM=
content-length:33
username=xiaoming&password=123456789
```

### Server-side Signature Verification Method
#### Overview
The server verifies the client signature through four main steps:
1. Extract key data from the received request to create a signing string.
2. Read the `key` from the received request and query the corresponding `secret`.
3. Encrypt the key data signing string using the algorithm and the `secret` to obtain the signature.
4. Read the client signature from the received request and compare the server-side signature with the client-side signature for consistency.

As shown in the figure below:
![](https://help-static-aliyun-doc.aliyuncs.com/assets/img/zh-CN/1745707061/p188116.png)

### Signature Troubleshooting Method
When the gateway signature verification fails, the server's signing string (StringToSign) will be returned in the HTTP Response header to the client, with the key: X-Ca-Error-Message. The user only needs to compare the locally computed signing string (StringToSign) with the signing string returned by the server to find the issue.

If the StringToSign from the server and the client are consistent, please check whether the APP Secret used for signature calculation is correct.

Since HTTP headers cannot express line breaks, the line breaks in the StringToSign have been replaced with `#`, as shown below:
```text
X-Ca-Error-Message:  Server StringToSign:`GET#application/json##application/json##X-Ca-Key:200000#X-Ca-Timestamp:1589458000000#/app/v1/config/keys?keys=TEST`
```

## Related Error Codes
| HTTP Status Code | Error Message         | Reasoning                                    |
| ---------------- | --------------------- | --------------------------------------------- |
| 401              | Invalid Key           | The request header did not provide x-ca-key, or x-ca-key is invalid.          |
| 401              | Empty Signature       | The request header did not provide the x-ca-signature signing string.          |
| 400              | Invalid Signature     | The x-ca-signature signing string in the request header does not match the signature calculated by the server. |
| 400              | Invalid Content-MD5   | The Content-MD5 header in the request is incorrect.                            |
| 400              | Invalid Date          | The time offset calculated based on the Date header in the request exceeds the configured date_offset. |
| 413              | Request Body Too Large| The request Body exceeds the maximum size of 32 MB.                           |
| 413              | Payload Too Large     | The request Body exceeds the global configured DownstreamConnectionBufferLimits. |
| 403              | Unauthorized Consumer  | The calling party does not have access permissions for the request.            |
