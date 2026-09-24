#!/bin/bash
# Install Harbor registry for WASM plugin images
# Only use this if you don't have an existing image registry

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

HARBOR_NAMESPACE="${1:-harbor-system}"
HARBOR_PASSWORD="${2:-Harbor12345}"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Harbor Registry Installation${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo -e "${YELLOW}This will install Harbor in your cluster.${NC}"
echo ""
echo "Configuration:"
echo "  Namespace: ${HARBOR_NAMESPACE}"
echo "  Admin Password: ${HARBOR_PASSWORD}"
echo "  Exposure: NodePort (no TLS)"
echo "  Persistence: Enabled (default StorageClass)"
echo ""
read -p "Continue? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 1
fi

# Check prerequisites
echo -e "\n${YELLOW}Checking prerequisites...${NC}"

# Check for helm
if ! command -v helm &> /dev/null; then
    echo -e "${RED}✗ helm not found. Please install helm 3.x${NC}"
    exit 1
fi
echo -e "${GREEN}✓ helm found${NC}"

# Check for kubectl
if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}✗ kubectl not found${NC}"
    exit 1
fi
echo -e "${GREEN}✓ kubectl found${NC}"

# Check cluster access
if ! kubectl get nodes &> /dev/null; then
    echo -e "${RED}✗ Cannot access cluster${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Cluster access OK${NC}"

# Check for default StorageClass
if ! kubectl get storageclass -o name | grep -q .; then
    echo -e "${YELLOW}⚠ No StorageClass found. Harbor needs persistent storage.${NC}"
    echo "  You may need to install a storage provisioner first."
    read -p "Continue anyway? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Add Harbor helm repo
echo -e "\n${YELLOW}Adding Harbor helm repository...${NC}"
helm repo add harbor https://helm.goharbor.io
helm repo update
echo -e "${GREEN}✓ Repository added${NC}"

# Install Harbor
echo -e "\n${YELLOW}Installing Harbor...${NC}"
helm install harbor harbor/harbor \
  --namespace "${HARBOR_NAMESPACE}" --create-namespace \
  --set expose.type=nodePort \
  --set expose.tls.enabled=false \
  --set persistence.enabled=true \
  --set harborAdminPassword="${HARBOR_PASSWORD}" \
  --wait --timeout 10m

if [ $? -ne 0 ]; then
    echo -e "${RED}✗ Harbor installation failed${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Harbor installed successfully${NC}"

# Wait for Harbor to be ready
echo -e "\n${YELLOW}Waiting for Harbor to be ready...${NC}"
kubectl wait --for=condition=ready pod -l app=harbor -n "${HARBOR_NAMESPACE}" --timeout=300s

# Get access information
echo -e "\n${BLUE}========================================${NC}"
echo -e "${BLUE}Harbor Access Information${NC}"
echo -e "${BLUE}========================================${NC}"

NODE_PORT=$(kubectl get svc -n "${HARBOR_NAMESPACE}" harbor-core -o jsonpath='{.spec.ports[0].nodePort}')
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="ExternalIP")].address}')
if [ -z "$NODE_IP" ]; then
    NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
fi

HARBOR_URL="${NODE_IP}:${NODE_PORT}"

echo ""
echo -e "Harbor URL: ${GREEN}http://${HARBOR_URL}${NC}"
echo -e "Username: ${GREEN}admin${NC}"
echo -e "Password: ${GREEN}${HARBOR_PASSWORD}${NC}"
echo ""

# Test Docker login
echo -e "${YELLOW}Testing Docker login...${NC}"
if docker login "${HARBOR_URL}" -u admin -p "${HARBOR_PASSWORD}" &> /dev/null; then
    echo -e "${GREEN}✓ Docker login successful${NC}"
else
    echo -e "${YELLOW}⚠ Docker login failed. You may need to:${NC}"
    echo "  1. Add '${HARBOR_URL}' to Docker's insecure registries"
    echo "  2. Restart Docker daemon"
    echo ""
    echo "  Edit /etc/docker/daemon.json (Linux) or Docker Desktop settings (Mac/Windows):"
    echo "  {"
    echo "    \"insecure-registries\": [\"${HARBOR_URL}\"]"
    echo "  }"
fi

echo ""
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Next Steps${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo "1. Open Harbor UI: http://${HARBOR_URL}"
echo "2. Login with admin/${HARBOR_PASSWORD}"
echo "3. Create a new project:"
echo "   - Click 'Projects' → 'New Project'"
echo "   - Name: higress-plugins"
echo "   - Access Level: Public"
echo ""
echo "4. Build and push your plugin:"
echo "   docker build -t ${HARBOR_URL}/higress-plugins/my-plugin:v1 ."
echo "   docker push ${HARBOR_URL}/higress-plugins/my-plugin:v1"
echo ""
echo "5. Use in WasmPlugin:"
echo "   url: oci://${HARBOR_URL}/higress-plugins/my-plugin:v1"
echo ""
echo -e "${YELLOW}⚠ Note: This is a basic installation for testing.${NC}"
echo "  For production use:"
echo "  - Enable TLS (set expose.tls.enabled=true)"
echo "  - Use LoadBalancer or Ingress instead of NodePort"
echo "  - Configure proper persistent storage"
echo "  - Set strong admin password"
echo ""
