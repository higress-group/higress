#!/bin/bash
# Analyze nginx Ingress resources and identify migration requirements

set -e

NAMESPACE="${1:-}"
OUTPUT_FORMAT="${2:-text}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Supported nginx annotations that map to Higress
SUPPORTED_ANNOTATIONS=(
    "rewrite-target"
    "use-regex"
    "ssl-redirect"
    "force-ssl-redirect"
    "backend-protocol"
    "proxy-body-size"
    "enable-cors"
    "cors-allow-origin"
    "cors-allow-methods"
    "cors-allow-headers"
    "cors-expose-headers"
    "cors-allow-credentials"
    "cors-max-age"
    "proxy-connect-timeout"
    "proxy-send-timeout"
    "proxy-read-timeout"
    "proxy-next-upstream-tries"
    "canary"
    "canary-weight"
    "canary-header"
    "canary-header-value"
    "canary-header-pattern"
    "canary-by-cookie"
    "auth-type"
    "auth-secret"
    "auth-realm"
    "load-balance"
    "upstream-hash-by"
    "whitelist-source-range"
    "denylist-source-range"
    "permanent-redirect"
    "temporal-redirect"
    "permanent-redirect-code"
    "proxy-set-headers"
    "proxy-hide-headers"
    "proxy-pass-headers"
    "proxy-ssl-secret"
    "proxy-ssl-verify"
)

# Unsupported annotations requiring WASM plugins
UNSUPPORTED_ANNOTATIONS=(
    "server-snippet"
    "configuration-snippet"
    "stream-snippet"
    "lua-resty-waf"
    "lua-resty-waf-score-threshold"
    "enable-modsecurity"
    "modsecurity-snippet"
    "limit-rps"
    "limit-connections"
    "limit-rate"
    "limit-rate-after"
    "client-body-buffer-size"
    "proxy-buffering"
    "proxy-buffers-number"
    "proxy-buffer-size"
    "custom-http-errors"
    "default-backend"
)

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Nginx to Higress Migration Analysis${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Check for ingress-nginx
echo -e "${YELLOW}Checking for ingress-nginx...${NC}"
if kubectl get pods -A 2>/dev/null | grep -q ingress-nginx; then
    echo -e "${GREEN}✓ ingress-nginx found${NC}"
    kubectl get pods -A | grep ingress-nginx | head -5
else
    echo -e "${RED}✗ ingress-nginx not found${NC}"
fi
echo ""

# Check IngressClass
echo -e "${YELLOW}IngressClass resources:${NC}"
kubectl get ingressclass 2>/dev/null || echo "No IngressClass resources found"
echo ""

# Get Ingress resources
if [ -n "$NAMESPACE" ]; then
    INGRESS_LIST=$(kubectl get ingress -n "$NAMESPACE" -o json 2>/dev/null)
else
    INGRESS_LIST=$(kubectl get ingress -A -o json 2>/dev/null)
fi

if [ -z "$INGRESS_LIST" ] || [ "$(echo "$INGRESS_LIST" | jq '.items | length')" -eq 0 ]; then
    echo -e "${RED}No Ingress resources found${NC}"
    exit 0
fi

TOTAL_INGRESS=$(echo "$INGRESS_LIST" | jq '.items | length')
echo -e "${YELLOW}Found ${TOTAL_INGRESS} Ingress resources${NC}"
echo ""

# Analyze each Ingress
COMPATIBLE_COUNT=0
NEEDS_PLUGIN_COUNT=0
UNSUPPORTED_FOUND=()

echo "$INGRESS_LIST" | jq -c '.items[]' | while read -r ingress; do
    NAME=$(echo "$ingress" | jq -r '.metadata.name')
    NS=$(echo "$ingress" | jq -r '.metadata.namespace')
    INGRESS_CLASS=$(echo "$ingress" | jq -r '.spec.ingressClassName // .metadata.annotations["kubernetes.io/ingress.class"] // "unknown"')
    
    # Skip non-nginx ingresses
    if [[ "$INGRESS_CLASS" != "nginx" && "$INGRESS_CLASS" != "unknown" ]]; then
        continue
    fi
    
    echo -e "${BLUE}-------------------------------------------${NC}"
    echo -e "${BLUE}Ingress: ${NS}/${NAME}${NC}"
    echo -e "IngressClass: ${INGRESS_CLASS}"
    
    # Get annotations
    ANNOTATIONS=$(echo "$ingress" | jq -r '.metadata.annotations // {}')
    
    HAS_UNSUPPORTED=false
    SUPPORTED_LIST=()
    UNSUPPORTED_LIST=()
    
    # Check each annotation
    echo "$ANNOTATIONS" | jq -r 'keys[]' | while read -r key; do
        # Extract annotation name (remove prefix)
        ANNO_NAME=$(echo "$key" | sed 's/nginx.ingress.kubernetes.io\///' | sed 's/higress.io\///')
        
        if [[ "$key" == nginx.ingress.kubernetes.io/* ]]; then
            # Check if supported
            IS_SUPPORTED=false
            for supported in "${SUPPORTED_ANNOTATIONS[@]}"; do
                if [[ "$ANNO_NAME" == "$supported" ]]; then
                    IS_SUPPORTED=true
                    break
                fi
            done
            
            # Check if explicitly unsupported
            for unsupported in "${UNSUPPORTED_ANNOTATIONS[@]}"; do
                if [[ "$ANNO_NAME" == "$unsupported" ]]; then
                    IS_SUPPORTED=false
                    HAS_UNSUPPORTED=true
                    VALUE=$(echo "$ANNOTATIONS" | jq -r --arg k "$key" '.[$k]')
                    echo -e "  ${RED}✗ $ANNO_NAME${NC} (requires WASM plugin)"
                    if [[ "$ANNO_NAME" == *"snippet"* ]]; then
                        echo -e "    Value preview: $(echo "$VALUE" | head -1)"
                    fi
                    break
                fi
            done
            
            if [ "$IS_SUPPORTED" = true ]; then
                echo -e "  ${GREEN}✓ $ANNO_NAME${NC}"
            fi
        fi
    done
    
    if [ "$HAS_UNSUPPORTED" = true ]; then
        echo -e "\n  ${YELLOW}Status: Requires WASM plugin for full compatibility${NC}"
    else
        echo -e "\n  ${GREEN}Status: Fully compatible${NC}"
    fi
    echo ""
done

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Summary${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "Total Ingress resources: ${TOTAL_INGRESS}"
echo ""
echo -e "${GREEN}✓ No Ingress modification needed!${NC}"
echo "  Higress natively supports nginx.ingress.kubernetes.io/* annotations."
echo ""
echo -e "${YELLOW}Next Steps:${NC}"
echo "1. Install Higress with the SAME ingressClass as nginx"
echo "   (set global.enableStatus=false to disable Ingress status updates)"
echo "2. For snippets/Lua: check Higress built-in plugins first, then generate custom WASM if needed"
echo "3. Generate and run migration test script"
echo "4. Switch traffic via DNS or L4 proxy after tests pass"
echo "5. After stable period, uninstall nginx and enable status updates (global.enableStatus=true)"
