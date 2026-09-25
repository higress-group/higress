# Local Testing with Docker Compose

## Prerequisites

- Docker installed
- Compiled `main.wasm` file

## Setup

Create these files in your plugin directory:

### docker-compose.yaml

```yaml
version: '3.7'
services:
  envoy:
    image: higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:v2.1.5
    entrypoint: /usr/local/bin/envoy
    command: -c /etc/envoy/envoy.yaml --component-log-level wasm:debug
    depends_on:
      - httpbin
    networks:
      - wasmtest
    ports:
      - "10000:10000"
    volumes:
      - ./envoy.yaml:/etc/envoy/envoy.yaml
      - ./main.wasm:/etc/envoy/main.wasm

  httpbin:
    image: kennethreitz/httpbin:latest
    networks:
      - wasmtest
    ports:
      - "12345:80"

networks:
  wasmtest: {}
```

### envoy.yaml

```yaml
admin:
  address:
    socket_address:
      protocol: TCP
      address: 0.0.0.0
      port_value: 9901

static_resources:
  listeners:
    - name: listener_0
      address:
        socket_address:
          protocol: TCP
          address: 0.0.0.0
          port_value: 10000
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                scheme_header_transformation:
                  scheme_to_overwrite: https
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: local_service
                      domains: ["*"]
                      routes:
                        - match:
                            prefix: "/"
                          route:
                            cluster: httpbin
                http_filters:
                  - name: wasmdemo
                    typed_config:
                      "@type": type.googleapis.com/udpa.type.v1.TypedStruct
                      type_url: type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm
                      value:
                        config:
                          name: wasmdemo
                          vm_config:
                            runtime: envoy.wasm.runtime.v8
                            code:
                              local:
                                filename: /etc/envoy/main.wasm
                          configuration:
                            "@type": "type.googleapis.com/google.protobuf.StringValue"
                            value: |
                              {
                                "mockEnable": false
                              }
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

  clusters:
    - name: httpbin
      connect_timeout: 30s
      type: LOGICAL_DNS
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: httpbin
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: httpbin
                      port_value: 80
```

## Running

```bash
# Start
docker compose up

# Test without gateway (baseline)
curl http://127.0.0.1:12345/get

# Test with gateway (plugin applied)
curl http://127.0.0.1:10000/get

# Stop
docker compose down
```

## Modifying Plugin Config

1. Edit the `configuration.value` section in `envoy.yaml`
2. Restart: `docker compose restart envoy`

## Viewing Logs

```bash
# Follow Envoy logs
docker compose logs -f envoy

# WASM debug logs (enabled by --component-log-level wasm:debug)
```

## Adding External Services

To test external HTTP/Redis calls, add services to docker-compose.yaml:

```yaml
services:
  # ... existing services ...
  
  redis:
    image: redis:7-alpine
    networks:
      - wasmtest
    ports:
      - "6379:6379"

  auth-service:
    image: your-auth-service:latest
    networks:
      - wasmtest
```

Then add clusters to envoy.yaml:

```yaml
clusters:
  # ... existing clusters ...
  
  - name: outbound|6379||redis.static
    connect_timeout: 5s
    type: LOGICAL_DNS
    dns_lookup_family: V4_ONLY
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: redis
      endpoints:
        - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: redis
                    port_value: 6379
```
