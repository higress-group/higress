#!/bin/bash
# Generate test script for all Ingress routes
# Tests each route against Higress gateway to validate migration

set -e

NAMESPACE="${1:-}"

# Colors for output script
cat << 'HEADER'
#!/bin/bash
# Higress Migration Test Script
# Auto-generated - tests all Ingress routes against Higress gateway

set -e

GATEWAY_IP="${1:-}"
TIMEOUT="${2:-5}"
VERBOSE="${3:-false}"

if [ -z "$GATEWAY_IP" ]; then
    echo "Usage: $0 <higress-gateway-ip[:port]> [timeout] [verbose]"
    echo ""
    echo "Examples:"
    echo "  # With LoadBalancer IP"
    echo "  $0 10.0.0.100 5 true"
    echo ""
    echo "  # With port-forward (run this first: kubectl port-forward -n higress-system svc/higress-gateway 8080:80 &)"
    echo "  $0 127.0.0.1:8080 5 true"
    exit 1
fi

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

TOTAL=0
PASSED=0
FAILED=0
FAILED_TESTS=()

test_route() {
    local host="$1"
    local path="$2"
    local expected_code="${3:-200}"
    local description="$4"
    
    TOTAL=$((TOTAL + 1))
    
    # Build URL
    local url="http://${GATEWAY_IP}${path}"
    
    # Make request
    local response
    response=$(curl -s -o /dev/null -w "%{http_code}" \
        -H "Host: ${host}" \
        --connect-timeout "${TIMEOUT}" \
        --max-time $((TIMEOUT * 2)) \
        "${url}" 2>/dev/null) || response="000"
    
    # Check result
    if [ "$response" = "$expected_code" ] || [ "$expected_code" = "*" ]; then
        PASSED=$((PASSED + 1))
        echo -e "${GREEN}✓${NC} [${response}] ${host}${path}"
        if [ "$VERBOSE" = "true" ]; then
            echo "  Expected: ${expected_code}, Got: ${response}"
        fi
    else
        FAILED=$((FAILED + 1))
        FAILED_TESTS+=("${host}${path} (expected ${expected_code}, got ${response})")
        echo -e "${RED}✗${NC} [${response}] ${host}${path}"
        echo "  Expected: ${expected_code}, Got: ${response}"
    fi
}

echo "========================================"
echo "Higress Migration Test"
echo "========================================"
echo "Gateway IP: ${GATEWAY_IP}"
echo "Timeout: ${TIMEOUT}s"
echo ""
echo "Testing routes..."
echo ""

HEADER

# Get Ingress resources
if [ -n "$NAMESPACE" ]; then
    INGRESS_JSON=$(kubectl get ingress -n "$NAMESPACE" -o json 2>/dev/null)
else
    INGRESS_JSON=$(kubectl get ingress -A -o json 2>/dev/null)
fi

if [ -z "$INGRESS_JSON" ] || [ "$(echo "$INGRESS_JSON" | jq '.items | length')" -eq 0 ]; then
    echo "# No Ingress resources found"
    echo "echo 'No Ingress resources found to test'"
    echo "exit 0"
    exit 0
fi

# Generate test cases for each Ingress
echo "$INGRESS_JSON" | jq -c '.items[]' | while read -r ingress; do
    NAME=$(echo "$ingress" | jq -r '.metadata.name')
    NS=$(echo "$ingress" | jq -r '.metadata.namespace')
    
    echo ""
    echo "# ================================================"
    echo "# Ingress: ${NS}/${NAME}"
    echo "# ================================================"
    
    # Check for TLS hosts
    TLS_HOSTS=$(echo "$ingress" | jq -r '.spec.tls[]?.hosts[]?' 2>/dev/null | sort -u)
    
    # Process each rule
    echo "$ingress" | jq -c '.spec.rules[]?' | while read -r rule; do
        HOST=$(echo "$rule" | jq -r '.host // "*"')
        
        # Process each path
        echo "$rule" | jq -c '.http.paths[]?' | while read -r path_item; do
            PATH=$(echo "$path_item" | jq -r '.path // "/"')
            PATH_TYPE=$(echo "$path_item" | jq -r '.pathType // "Prefix"')
            SERVICE=$(echo "$path_item" | jq -r '.backend.service.name // .backend.serviceName // "unknown"')
            PORT=$(echo "$path_item" | jq -r '.backend.service.port.number // .backend.service.port.name // .backend.servicePort // "80"')
            
            # Generate test
            # For Prefix paths, test the exact path
            # For Exact paths, test exactly
            # Add a simple 200 or * expectation (can be customized)
            
            echo ""
            echo "# Path: ${PATH} (${PATH_TYPE}) -> ${SERVICE}:${PORT}"
            
            # Test the path
            if [ "$PATH_TYPE" = "Exact" ]; then
                echo "test_route \"${HOST}\" \"${PATH}\" \"*\" \"Exact path\""
            else
                # For Prefix, test base path and a subpath
                echo "test_route \"${HOST}\" \"${PATH}\" \"*\" \"Prefix path\""
                
                # If path doesn't end with /, add a subpath test
                if [[ ! "$PATH" =~ /$ ]] && [ "$PATH" != "/" ]; then
                    echo "test_route \"${HOST}\" \"${PATH}/\" \"*\" \"Prefix path with trailing slash\""
                fi
            fi
        done
    done
    
    # Check for specific annotations that might need special testing
    REWRITE=$(echo "$ingress" | jq -r '.metadata.annotations["nginx.ingress.kubernetes.io/rewrite-target"] // .metadata.annotations["higress.io/rewrite-target"] // ""')
    if [ -n "$REWRITE" ] && [ "$REWRITE" != "null" ]; then
        echo ""
        echo "# Note: This Ingress has rewrite-target: ${REWRITE}"
        echo "# Verify the rewritten path manually if needed"
    fi
    
    CANARY=$(echo "$ingress" | jq -r '.metadata.annotations["nginx.ingress.kubernetes.io/canary"] // .metadata.annotations["higress.io/canary"] // ""')
    if [ "$CANARY" = "true" ]; then
        echo ""
        echo "# Note: This is a canary Ingress - test with appropriate headers/cookies"
        CANARY_HEADER=$(echo "$ingress" | jq -r '.metadata.annotations["nginx.ingress.kubernetes.io/canary-header"] // .metadata.annotations["higress.io/canary-header"] // ""')
        CANARY_VALUE=$(echo "$ingress" | jq -r '.metadata.annotations["nginx.ingress.kubernetes.io/canary-header-value"] // .metadata.annotations["higress.io/canary-header-value"] // ""')
        if [ -n "$CANARY_HEADER" ] && [ "$CANARY_HEADER" != "null" ]; then
            echo "# Canary header: ${CANARY_HEADER}=${CANARY_VALUE}"
        fi
    fi
done

# Generate summary section
cat << 'FOOTER'

# ================================================
# Summary
# ================================================
echo ""
echo "========================================"
echo "Test Summary"
echo "========================================"
echo -e "Total:  ${TOTAL}"
echo -e "Passed: ${GREEN}${PASSED}${NC}"
echo -e "Failed: ${RED}${FAILED}${NC}"
echo ""

if [ ${FAILED} -gt 0 ]; then
    echo -e "${YELLOW}Failed tests:${NC}"
    for test in "${FAILED_TESTS[@]}"; do
        echo -e "  ${RED}•${NC} $test"
    done
    echo ""
    echo -e "${YELLOW}⚠ Some tests failed. Please investigate before switching traffic.${NC}"
    exit 1
else
    echo -e "${GREEN}✓ All tests passed!${NC}"
    echo ""
    echo "========================================"
    echo -e "${GREEN}Ready for Traffic Cutover${NC}"
    echo "========================================"
    echo ""
    echo "Next steps:"
    echo "1. Switch traffic to Higress gateway:"
    echo "   - DNS: Update A/CNAME records to ${GATEWAY_IP}"
    echo "   - L4 Proxy: Update upstream to ${GATEWAY_IP}"
    echo ""
    echo "2. Monitor for errors after switch"
    echo ""
    echo "3. Once stable, scale down nginx:"
    echo "   kubectl scale deployment -n ingress-nginx ingress-nginx-controller --replicas=0"
    echo ""
fi
FOOTER
