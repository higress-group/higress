---
name: nginx-to-higress-migration
description: "Migrate from ingress-nginx to Higress in Kubernetes environments. Use when (1) analyzing existing ingress-nginx setup (2) reading nginx Ingress resources and ConfigMaps (3) installing Higress via helm with proper ingressClass (4) identifying unsupported nginx annotations (5) generating WASM plugins for nginx snippets/advanced features (6) building and deploying custom plugins to image registry. Supports full migration workflow with compatibility analysis and plugin generation."
---

# Nginx to Higress Migration

Automate migration from ingress-nginx to Higress in Kubernetes environments.

## ⚠️ Critical Limitation: Snippet Annotations NOT Supported

> **Before you begin:** Higress does **NOT** support the following nginx annotations:
> - `nginx.ingress.kubernetes.io/server-snippet`
> - `nginx.ingress.kubernetes.io/configuration-snippet`
> - `nginx.ingress.kubernetes.io/http-snippet`
>
> These annotations will be **silently ignored**, causing functionality loss!
>
> **Pre-migration check (REQUIRED):**
> ```bash
> kubectl get ingress -A -o yaml | grep -E "snippet" | wc -l
> ```
> If count > 0, you MUST plan WASM plugin replacements before migration.
> See [Phase 6](#phase-6-use-built-in-plugins-or-create-custom-wasm-plugin-if-needed) for alternatives.

## Prerequisites

- kubectl configured with cluster access
- helm 3.x installed
- Go 1.24+ (for WASM plugin compilation)
- Docker (for plugin image push)

## Pre-Migration Checklist

### Before Starting

- [ ] Backup all Ingress resources
  ```bash
  kubectl get ingress -A -o yaml > ingress-backup.yaml
  ```
- [ ] Identify snippet usage (see warning above)
- [ ] List all nginx annotations in use
  ```bash
  kubectl get ingress -A -o yaml | grep "nginx.ingress.kubernetes.io" | sort | uniq -c
  ```
- [ ] Verify Higress compatibility for each annotation (see [annotation-mapping.md](references/annotation-mapping.md))
- [ ] Plan WASM plugins for unsupported features
- [ ] Prepare test environment (Kind/Minikube for testing recommended)

### During Migration

- [ ] Install Higress in parallel with nginx
- [ ] Verify all pods running in higress-system namespace
- [ ] Run test script against Higress gateway
- [ ] Compare responses between nginx and Higress
- [ ] Deploy any required WASM plugins
- [ ] Configure monitoring/alerting

### After Migration

- [ ] All routes verified working
- [ ] Custom functionality (snippet replacements) tested
- [ ] Monitoring dashboards configured
- [ ] Team trained on Higress operations
- [ ] Documentation updated
- [ ] Rollback procedure tested

## Migration Workflow

### Phase 1: Discovery

```bash
# Check for ingress-nginx installation
kubectl get pods -A | grep ingress-nginx
kubectl get ingressclass

# List all Ingress resources using nginx class
kubectl get ingress -A -o json | jq '.items[] | select(.spec.ingressClassName=="nginx" or .metadata.annotations["kubernetes.io/ingress.class"]=="nginx")'

# Get nginx ConfigMap
kubectl get configmap -n ingress-nginx ingress-nginx-controller -o yaml
```

### Phase 2: Compatibility Analysis

Run the analysis script to identify unsupported features:

```bash
./scripts/analyze-ingress.sh [namespace]
```

**Key point: No Ingress modification needed!**

Higress natively supports `nginx.ingress.kubernetes.io/*` annotations - your existing Ingress resources work as-is.

See [references/annotation-mapping.md](references/annotation-mapping.md) for the complete list of supported annotations.

**Unsupported annotations** (require built-in plugin or custom WASM plugin):
- `nginx.ingress.kubernetes.io/server-snippet`
- `nginx.ingress.kubernetes.io/configuration-snippet`
- `nginx.ingress.kubernetes.io/lua-resty-waf*`
- Complex Lua logic in snippets

For these, check [references/builtin-plugins.md](references/builtin-plugins.md) first - Higress may already have a plugin!

### Phase 3: Higress Installation (Parallel with nginx)

Higress natively supports `nginx.ingress.kubernetes.io/*` annotations. Install Higress **alongside** nginx for safe parallel testing.

```bash
# 1. Get current nginx ingressClass name
INGRESS_CLASS=$(kubectl get ingressclass -o jsonpath='{.items[?(@.spec.controller=="k8s.io/ingress-nginx")].metadata.name}')
echo "Current nginx ingressClass: $INGRESS_CLASS"

# 2. Detect timezone and select nearest registry
# China/Asia: higress-registry.cn-hangzhou.cr.aliyuncs.com (default)
# North America: higress-registry.us-west-1.cr.aliyuncs.com
# Southeast Asia: higress-registry.ap-southeast-7.cr.aliyuncs.com
TZ_OFFSET=$(date +%z)
case "$TZ_OFFSET" in
  -1*|-0*) REGISTRY="higress-registry.us-west-1.cr.aliyuncs.com" ;;      # Americas
  +07*|+08*|+09*) REGISTRY="higress-registry.cn-hangzhou.cr.aliyuncs.com" ;; # Asia
  +05*|+06*) REGISTRY="higress-registry.ap-southeast-7.cr.aliyuncs.com" ;;   # Southeast Asia
  *) REGISTRY="higress-registry.cn-hangzhou.cr.aliyuncs.com" ;;          # Default
esac
echo "Using registry: $REGISTRY"

# 3. Add Higress repo
helm repo add higress https://higress.io/helm-charts
helm repo update

# 4. Install Higress with parallel-safe settings
# Note: Override ALL component hubs to use the selected registry
helm install higress higress/higress \
  -n higress-system --create-namespace \
  --set global.ingressClass=${INGRESS_CLASS:-nginx} \
  --set global.hub=${REGISTRY}/higress \
  --set global.enableStatus=false \
  --set higress-core.controller.hub=${REGISTRY}/higress \
  --set higress-core.gateway.hub=${REGISTRY}/higress \
  --set higress-core.pilot.hub=${REGISTRY}/higress \
  --set higress-core.pluginServer.hub=${REGISTRY}/higress \
  --set higress-core.gateway.replicas=2
```

Key helm values:
- `global.ingressClass`: Use the **same** class as ingress-nginx
- `global.hub`: Image registry (auto-selected by timezone)
- `global.enableStatus=false`: **Disable Ingress status updates** to avoid conflicts with nginx (reduces API server pressure)
- Override all component hubs to ensure consistent registry usage
- Both nginx and Higress will watch the same Ingress resources
- Higress automatically recognizes `nginx.ingress.kubernetes.io/*` annotations
- Traffic still flows through nginx until you switch the entry point

⚠️ **Note**: After nginx is uninstalled, you can enable status updates:
```bash
helm upgrade higress higress/higress -n higress-system \
  --reuse-values \
  --set global.enableStatus=true
```

#### Kind/Local Environment Setup

In Kind or local Kubernetes clusters, the LoadBalancer service will stay in `PENDING` state. Use one of these methods:

**Option 1: Port Forward (Recommended for testing)**
```bash
# Forward Higress gateway to local port
kubectl port-forward -n higress-system svc/higress-gateway 8080:80 8443:443 &

# Test with Host header
curl -H "Host: example.com" http://localhost:8080/
```

**Option 2: NodePort**
```bash
# Patch service to NodePort
kubectl patch svc -n higress-system higress-gateway \
  -p '{"spec":{"type":"NodePort"}}'

# Get assigned port
NODE_PORT=$(kubectl get svc -n higress-system higress-gateway \
  -o jsonpath='{.spec.ports[?(@.port==80)].nodePort}')

# Test (use docker container IP for Kind)
curl -H "Host: example.com" http://localhost:${NODE_PORT}/
```

**Option 3: Kind with Port Mapping (Requires cluster recreation)**
```yaml
# kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30080
    hostPort: 80
  - containerPort: 30443
    hostPort: 443
```

### Phase 4: Generate and Run Test Script

After Higress is running, generate a test script covering all Ingress routes:

```bash
# Generate test script
./scripts/generate-migration-test.sh > migration-test.sh
chmod +x migration-test.sh

# Get Higress gateway address
# Option A: If LoadBalancer is supported
HIGRESS_IP=$(kubectl get svc -n higress-system higress-gateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# Option B: If LoadBalancer is NOT supported, use port-forward
kubectl port-forward -n higress-system svc/higress-gateway 8080:80 &
HIGRESS_IP="127.0.0.1:8080"

# Run tests
./migration-test.sh ${HIGRESS_IP}
```

The test script will:
- Extract all hosts and paths from Ingress resources
- Test each route against Higress gateway
- Verify response codes and basic functionality
- Report any failures for investigation

### Phase 5: Traffic Cutover (User Action Required)

⚠️ **Only proceed after all tests pass!**

Choose your cutover method based on infrastructure:

**Option A: DNS Switch**
```bash
# Update DNS records to point to Higress gateway IP
# Example: example.com A record -> ${HIGRESS_IP}
```

**Option B: Layer 4 Proxy/Load Balancer Switch**
```bash
# Update upstream in your L4 proxy (e.g., F5, HAProxy, cloud LB)
# From: nginx-ingress-controller service IP
# To: higress-gateway service IP
```

**Option C: Kubernetes Service Switch** (if using external traffic via Service)
```bash
# Update your external-facing Service selector or endpoints
```

### Phase 6: Use Built-in Plugins or Create Custom WASM Plugin (If Needed)

Before writing custom plugins, check if Higress has a built-in plugin that meets your needs!

#### Built-in Plugins (Recommended First)

Higress provides many built-in plugins. Check [references/builtin-plugins.md](references/builtin-plugins.md) for the full list.

Common replacements for nginx features:
| nginx feature | Higress built-in plugin |
|---------------|------------------------|
| Basic Auth snippet | `basic-auth` |
| IP restriction | `ip-restriction` |
| Rate limiting | `key-rate-limit`, `cluster-key-rate-limit` |
| WAF/ModSecurity | `waf` |
| Request validation | `request-validation` |
| Bot detection | `bot-detect` |
| JWT auth | `jwt-auth` |
| CORS headers | `cors` |
| Custom response | `custom-response` |
| Request/Response transform | `transformer` |

#### Common Snippet Replacements

| nginx snippet pattern | Higress solution |
|----------------------|------------------|
| Custom health endpoint (`location /health`) | WASM plugin: custom-location |
| Add response headers | WASM plugin: custom-response-headers |
| Request validation/blocking | WASM plugin with `OnHttpRequestHeaders` |
| Lua rate limiting | `key-rate-limit` plugin |

#### Custom WASM Plugin (If No Built-in Matches)

When nginx snippets or Lua logic has no built-in equivalent:

1. **Analyze snippet** - Extract nginx directives/Lua code
2. **Generate Go WASM code** - Use higress-wasm-go-plugin skill
3. **Build plugin**:
```bash
cd plugin-dir
go mod tidy
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o main.wasm ./
```

4. **Push to registry**:

If you don't have an image registry, install Harbor:
```bash
./scripts/install-harbor.sh
# Follow the prompts to install Harbor in your cluster
```

If you have your own registry:
```bash
# Build OCI image
docker build -t <registry>/higress-plugin-<name>:v1 .
docker push <registry>/higress-plugin-<name>:v1
```

5. **Deploy plugin**:
```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: custom-plugin
  namespace: higress-system
spec:
  url: oci://<registry>/higress-plugin-<name>:v1
  phase: UNSPECIFIED_PHASE
  priority: 100
```

See [references/plugin-deployment.md](references/plugin-deployment.md) for detailed plugin deployment.

## Common Snippet Conversions

### Header Manipulation
```nginx
# nginx snippet
more_set_headers "X-Custom: value";
```
→ Use `headerControl` annotation or generate plugin with `proxywasm.AddHttpResponseHeader()`.

### Request Validation
```nginx
# nginx snippet
if ($request_uri ~* "pattern") { return 403; }
```
→ Generate WASM plugin with request header/path check.

### Rate Limiting with Custom Logic
```nginx
# nginx snippet with Lua
access_by_lua_block { ... }
```
→ Generate WASM plugin implementing the logic.

See [references/snippet-patterns.md](references/snippet-patterns.md) for common patterns.

## Validation

Before traffic switch, use the generated test script:

```bash
# Generate test script
./scripts/generate-migration-test.sh > migration-test.sh
chmod +x migration-test.sh

# Get Higress gateway IP
HIGRESS_IP=$(kubectl get svc -n higress-system higress-gateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# Run all tests
./migration-test.sh ${HIGRESS_IP}
```

The test script will:
- Test every host/path combination from all Ingress resources
- Report pass/fail for each route
- Provide a summary and next steps

**Only proceed with traffic cutover after all tests pass!**

## Troubleshooting

### Common Issues

#### Q1: Ingress created but routes return 404
**Symptoms:** Ingress shows Ready, but curl returns 404

**Check:**
1. Verify IngressClass matches Higress config
   ```bash
   kubectl get ingress <name> -o yaml | grep ingressClassName
   ```
2. Check controller logs
   ```bash
   kubectl logs -n higress-system -l app=higress-controller --tail=100
   ```
3. Verify backend service is reachable
   ```bash
   kubectl run test --rm -it --image=curlimages/curl -- \
     curl http://<service>.<namespace>.svc
   ```

#### Q2: rewrite-target not working
**Symptoms:** Path not being rewritten, backend receives original path

**Solution:** Ensure `use-regex: "true"` is also set:
```yaml
annotations:
  nginx.ingress.kubernetes.io/rewrite-target: /$2
  nginx.ingress.kubernetes.io/use-regex: "true"
```

#### Q3: Snippet annotations silently ignored
**Symptoms:** nginx snippet features not working after migration

**Cause:** Higress does not support snippet annotations (by design, for security)

**Solution:** 
- Check [references/builtin-plugins.md](references/builtin-plugins.md) for built-in alternatives
- Create custom WASM plugin (see Phase 6)

#### Q4: TLS certificate issues
**Symptoms:** HTTPS not working or certificate errors

**Check:**
1. Verify Secret exists and is type `kubernetes.io/tls`
   ```bash
   kubectl get secret <secret-name> -o yaml
   ```
2. Check TLS configuration in Ingress
   ```bash
   kubectl get ingress <name> -o jsonpath='{.spec.tls}'
   ```

### Useful Debug Commands

```bash
# View Higress controller logs
kubectl logs -n higress-system -l app=higress-controller -c higress-core

# View gateway access logs
kubectl logs -n higress-system -l app=higress-gateway | grep "GET\|POST"

# Check Envoy config dump
kubectl exec -n higress-system deploy/higress-gateway -c istio-proxy -- \
  curl -s localhost:15000/config_dump | jq '.configs[2].dynamic_listeners'

# View gateway stats
kubectl exec -n higress-system deploy/higress-gateway -c istio-proxy -- \
  curl -s localhost:15000/stats | grep http
```

## Rollback

Since nginx keeps running during migration, rollback is simply switching traffic back:

```bash
# If traffic was switched via DNS:
# - Revert DNS records to nginx gateway IP

# If traffic was switched via L4 proxy:
# - Revert upstream to nginx service IP

# Nginx is still running, no action needed on k8s side
```

## Post-Migration Cleanup

**Only after traffic has been fully migrated and stable:**

```bash
# 1. Monitor Higress for a period (recommended: 24-48h)

# 2. Backup nginx resources
kubectl get all -n ingress-nginx -o yaml > ingress-nginx-backup.yaml

# 3. Scale down nginx (keep for emergency rollback)
kubectl scale deployment -n ingress-nginx ingress-nginx-controller --replicas=0

# 4. (Optional) After extended stable period, remove nginx
kubectl delete namespace ingress-nginx
```
