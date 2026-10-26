#!/bin/bash
# Generate WASM plugin scaffold for nginx snippet migration

set -e

if [ "$#" -lt 1 ]; then
    echo "Usage: $0 <plugin-name> [output-dir]"
    echo ""
    echo "Example: $0 custom-headers ./plugins"
    exit 1
fi

PLUGIN_NAME="$1"
OUTPUT_DIR="${2:-.}"
PLUGIN_DIR="${OUTPUT_DIR}/${PLUGIN_NAME}"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}Generating WASM plugin scaffold: ${PLUGIN_NAME}${NC}"

# Create directory
mkdir -p "$PLUGIN_DIR"

# Generate go.mod
cat > "${PLUGIN_DIR}/go.mod" << EOF
module ${PLUGIN_NAME}

go 1.24

require (
	github.com/higress-group/proxy-wasm-go-sdk v1.0.1-0.20241230091623-edc7227eb588
	github.com/higress-group/wasm-go v1.0.1-0.20250107151137-19a0ab53cfec
	github.com/tidwall/gjson v1.18.0
)
EOF

# Generate main.go
cat > "${PLUGIN_DIR}/main.go" << 'EOF'
package main

import (
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"PLUGIN_NAME_PLACEHOLDER",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessResponseBody(onHttpResponseBody),
	)
}

// PluginConfig holds the plugin configuration
type PluginConfig struct {
	// TODO: Add configuration fields
	// Example:
	// HeaderName  string
	// HeaderValue string
	Enabled bool
}

// parseConfig parses the plugin configuration from YAML (converted to JSON)
func parseConfig(json gjson.Result, config *PluginConfig) error {
	// TODO: Parse configuration
	// Example:
	// config.HeaderName = json.Get("headerName").String()
	// config.HeaderValue = json.Get("headerValue").String()
	config.Enabled = json.Get("enabled").Bool()
	
	proxywasm.LogInfof("Plugin config loaded: enabled=%v", config.Enabled)
	return nil
}

// onHttpRequestHeaders is called when request headers are received
func onHttpRequestHeaders(ctx wrapper.HttpContext, config PluginConfig) types.Action {
	if !config.Enabled {
		return types.HeaderContinue
	}

	// TODO: Implement request header processing
	// Example: Add custom header
	// proxywasm.AddHttpRequestHeader(config.HeaderName, config.HeaderValue)
	
	// Example: Check path and block
	// path := ctx.Path()
	// if strings.Contains(path, "/blocked") {
	//     proxywasm.SendHttpResponse(403, nil, []byte("Forbidden"), -1)
	//     return types.HeaderStopAllIterationAndWatermark
	// }

	return types.HeaderContinue
}

// onHttpRequestBody is called when request body is received
// Remove this function from init() if not needed
func onHttpRequestBody(ctx wrapper.HttpContext, config PluginConfig, body []byte) types.Action {
	if !config.Enabled {
		return types.BodyContinue
	}

	// TODO: Implement request body processing
	// Example: Log body size
	// proxywasm.LogInfof("Request body size: %d", len(body))

	return types.BodyContinue
}

// onHttpResponseHeaders is called when response headers are received
func onHttpResponseHeaders(ctx wrapper.HttpContext, config PluginConfig) types.Action {
	if !config.Enabled {
		return types.HeaderContinue
	}

	// TODO: Implement response header processing
	// Example: Add security headers
	// proxywasm.AddHttpResponseHeader("X-Content-Type-Options", "nosniff")
	// proxywasm.AddHttpResponseHeader("X-Frame-Options", "DENY")

	return types.HeaderContinue
}

// onHttpResponseBody is called when response body is received
// Remove this function from init() if not needed
func onHttpResponseBody(ctx wrapper.HttpContext, config PluginConfig, body []byte) types.Action {
	if !config.Enabled {
		return types.BodyContinue
	}

	// TODO: Implement response body processing
	// Example: Modify response body
	// newBody := strings.Replace(string(body), "old", "new", -1)
	// proxywasm.ReplaceHttpResponseBody([]byte(newBody))

	return types.BodyContinue
}
EOF

# Replace plugin name placeholder
sed -i "s/PLUGIN_NAME_PLACEHOLDER/${PLUGIN_NAME}/g" "${PLUGIN_DIR}/main.go"

# Generate Dockerfile
cat > "${PLUGIN_DIR}/Dockerfile" << 'EOF'
FROM scratch
COPY main.wasm /plugin.wasm
EOF

# Generate build script
cat > "${PLUGIN_DIR}/build.sh" << 'EOF'
#!/bin/bash
set -e

echo "Downloading dependencies..."
go mod tidy

echo "Building WASM plugin..."
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o main.wasm ./

echo "Build complete: main.wasm"
ls -lh main.wasm
EOF
chmod +x "${PLUGIN_DIR}/build.sh"

# Generate WasmPlugin manifest
cat > "${PLUGIN_DIR}/wasmplugin.yaml" << EOF
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: ${PLUGIN_NAME}
  namespace: higress-system
spec:
  # TODO: Replace with your registry
  url: oci://YOUR_REGISTRY/${PLUGIN_NAME}:v1
  phase: UNSPECIFIED_PHASE
  priority: 100
  defaultConfig:
    enabled: true
    # TODO: Add your configuration
  # Optional: Apply to specific routes/domains
  # matchRules:
  # - domain:
  #   - "*.example.com"
  #   config:
  #     enabled: true
EOF

# Generate README
cat > "${PLUGIN_DIR}/README.md" << EOF
# ${PLUGIN_NAME}

A Higress WASM plugin migrated from nginx configuration.

## Build

\`\`\`bash
./build.sh
\`\`\`

## Push to Registry

\`\`\`bash
# Set your registry
REGISTRY=your-registry.com/higress-plugins

# Build Docker image
docker build -t \${REGISTRY}/${PLUGIN_NAME}:v1 .

# Push
docker push \${REGISTRY}/${PLUGIN_NAME}:v1
\`\`\`

## Deploy

1. Update \`wasmplugin.yaml\` with your registry URL
2. Apply to cluster:
   \`\`\`bash
   kubectl apply -f wasmplugin.yaml
   \`\`\`

## Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| enabled | bool | true | Enable/disable plugin |

## TODO

- [ ] Implement plugin logic in main.go
- [ ] Add configuration fields
- [ ] Test locally
- [ ] Push to registry
- [ ] Deploy to cluster
EOF

echo -e "\n${GREEN}✓ Plugin scaffold generated at: ${PLUGIN_DIR}${NC}"
echo ""
echo "Files created:"
echo "  - ${PLUGIN_DIR}/main.go        (plugin source)"
echo "  - ${PLUGIN_DIR}/go.mod         (Go module)"
echo "  - ${PLUGIN_DIR}/Dockerfile     (OCI image)"
echo "  - ${PLUGIN_DIR}/build.sh       (build script)"
echo "  - ${PLUGIN_DIR}/wasmplugin.yaml (K8s manifest)"
echo "  - ${PLUGIN_DIR}/README.md      (documentation)"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo "1. cd ${PLUGIN_DIR}"
echo "2. Edit main.go to implement your logic"
echo "3. Run: ./build.sh"
echo "4. Push image to your registry"
echo "5. Update wasmplugin.yaml with registry URL"
echo "6. Deploy: kubectl apply -f wasmplugin.yaml"
