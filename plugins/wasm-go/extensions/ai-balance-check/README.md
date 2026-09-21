# AI Balance Check Plugin

## Description

The AI Balance Check plugin is a Higress WASM plugin that validates user balance before allowing requests to proceed. It performs an asynchronous HTTP call to a balance check service and blocks the request if the balance check fails.

## Features

- **Asynchronous Balance Checking**: Uses non-blocking HTTP calls to check user balance
- **Block in Request Body Phase**: The async balance check is dispatched from `onHttpRequestBody` (not headers) so that `SendHttpResponse(402,...)` actually cancels the downstream request instead of racing with upstream forwarding
- **Configurable Timeout**: Customizable API timeout duration
- **Custom Failure Message**: Ability to configure custom error messages for balance failures
- **Memory Safety**: Proper cleanup and panic recovery to prevent memory leaks
- **Request Header Extraction**: Extracts user token from configurable request header

## Configuration

### Required Fields

- **apiUrl**: The URL of the balance check API service
  - Example: `https://balance-service.example.com/api/check`

- **serviceName**: The service name for DNS resolution (Higress cluster)
  - Example: `balance-service`

- **userTokenHeader**: The HTTP request header containing the user token
  - Example: `Authorization` or `X-User-Token`

### Optional Fields

- **apiTimeout**: Timeout in milliseconds for the balance check API call
  - Default: `5000` (5 seconds)
  - Example: `3000`

- **failMessage**: Custom message to return when balance check fails
  - Default: `Insufficient balance, please recharge`
  - Example: `Your account balance is too low. Please top up your account.`

- **skipPaths**: List of request URI path prefixes that should bypass the balance check. Useful for endpoints that must not be blocked even when the user's balance is insufficient (e.g. async task polling endpoints like image-generation task result lookups).
  - Default: `[]` (no paths skipped)
  - Matching rule: prefix match against the path (query string is stripped before matching)
  - Supports both array form and comma-separated string form
  - Example (array): `["/v1/images/tasks", "/v1/async/"]`
  - Example (string): `"/v1/images/tasks,/v1/async/"`

## Balance API Requirements

The balance check API must return a JSON response with the following format:

```json
{
  "success": true/false,
  "message": "Optional error message when success is false"
}
```

### Response Behavior

- **Success (`true`)**: The original request is allowed to proceed
- **Failure (`false`)**: The original request is blocked and a 402 Payment Required response is returned
- **HTTP Error**: If the balance check API returns a non-2xx status code, a corresponding error response is returned

### Example Balance Check API Endpoints

**GET /api/check**

Headers:
```
Authorization: Bearer <user_token>
```

Success Response:
```json
{
  "success": true
}
```

Failure Response:
```json
{
  "success": false,
  "message": "Insufficient balance"
}
```

## Higress Configuration Example

```yaml
apiVersion: gateway.higress.io/v1beta1
kind: GlobalPlugin
metadata:
  name: ai-balance-check
spec:
  plugins:
  - name: ai-balance-check
    priority: 10
    config:
      apiUrl: https://balance-service.example.com/api/check
      serviceName: balance-service
      userTokenHeader: Authorization
      apiTimeout: 5000
      failMessage: "Your account balance is too low. Please top up your account."
      skipPaths:
        - /v1/images/tasks
        - /v1/async/
```

## Build

### Prerequisites

- Go 1.24 or higher
- Docker (for building the container image)

### Build Steps

```bash
# Build WASM file
./build.sh

# Or use TinyGo (if installed)
./build.sh tinygo
```

The build script will:
1. Compile the Go code to WASM
2. Display build success information
3. Provide instructions for Docker image creation

### Docker Build

```bash
# Build Docker image
docker build -t ai-balance-check:1.0.0 .

# Or with version from VERSION file
VERSION=$(cat VERSION)
docker build -t ai-balance-check:$VERSION .
```

## Build and Push to Registry

```bash
# Use the build-and-push script
./build-and-push-plugin.sh

# Or manually push
docker tag ai-balance-check:1.0.0 your-registry/ai-balance-check:1.0.0
docker push your-registry/ai-balance-check:1.0.0
```

## Usage in Higress Console

1. Add the plugin to your Higress instance
2. Configure the plugin with your balance check API details
3. Apply the plugin to the desired domain/route

## HTTP Response Codes

- **402 Payment Required**: Returned when balance check fails or when user token is missing
- **4xx-5xx**: Returned when the balance check API itself returns an error status code
- **200 OK**: Returned when the original request is allowed to proceed (balance check passed)

## Example Responses

### Balance Check Failed (402)

```json
{
  "code": 402,
  "message": "Your account balance is too low. Please top up your account."
}
```

### Balance Check API Error (4xx/5xx)

```json
{
  "code": 503,
  "message": "Balance check API error"
}
```

### Missing User Token (402)

```json
{
  "code": 402,
  "message": "Insufficient balance, please recharge"
}
```

## Error Handling

The plugin implements comprehensive error handling:

1. **Panic Recovery**: All callbacks are wrapped with defer-recover to prevent crashes
2. **API Failure**: If the balance check API call fails, an error response is returned
3. **JSON Parse Error**: If the API response cannot be parsed, an error response is returned
4. **Missing Configuration**: If required configuration fields are missing, the plugin is disabled

## Memory Management

To prevent memory leaks:

1. Context values are cleaned up appropriately
2. Panic recovery ensures no goroutine leaks
3. HTTP client is reused (created once during configuration)
4. No circular dependencies or unbounded memory allocation

## Development

### File Structure

```
ai-balance-check/
├── main.go                 # Main plugin implementation
├── go.mod                  # Go module definition
├── go.sum                   # Go dependencies lock file
├── Dockerfile               # Docker image definition
├── build.sh                # Build script
├── build-and-push-plugin.sh  # Build and push script
├── VERSION                  # Plugin version
├── README.md                # This file
└── main.wasm                # Compiled WASM (generated)
```

### Testing

To test the plugin locally:

1. Build the WASM file
2. Start a local Higress instance
3. Apply the plugin configuration
4. Send test requests with various user token headers

## License

Apache License 2.0

## Support

For issues and questions, please refer to the Higress documentation and community support.
