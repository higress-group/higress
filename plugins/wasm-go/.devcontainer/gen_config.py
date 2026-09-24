import json
import sys


plugin_name = sys.argv[1]
plugin_root = sys.argv[2] if len(sys.argv) > 2 else "extensions"
plugin_dir = f"{plugin_root}/{plugin_name}"

with open(f"{plugin_dir}/config.json", "r") as f:
    plugin_config = json.load(f)

config = f'''static_resources:
  listeners:
  - address:
      socket_address:
        address: 0.0.0.0
        port_value: 8080
    filter_chains:
    - filters:
      - name: envoy.filters.network.http_connection_manager
        typed_config:
          '@type': type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          codec_type: AUTO
          stat_prefix: ingress_http
          route_config:
            name: test
            virtual_hosts:
            - name: direct_response_service
              domains: 
              - "*"
              routes:
              - match:
                  prefix: "/"
                direct_response:
                  status: 200
                  body:
                    inline_string: "hello world\\n"
              # - match:
              #     prefix: "/"
              #   route:
              #     cluster: service-backend
          http_filters:
          - name: {plugin_name}
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
                        filename: ./{plugin_dir}/main.wasm
                  configuration:
                    "@type": "type.googleapis.com/google.protobuf.StringValue"
                    value: '{json.dumps(plugin_config)}'
          - name: envoy.filters.http.router
            typed_config:
              '@type': type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  # clusters:
  # - name: service-backend
  #   connect_timeout: 600s
  #   type: STATIC
  #   lb_policy: ROUND_ROBIN
  #   load_assignment:
  #     cluster_name: service-backend
  #     endpoints:
  #       - lb_endpoints:
  #           - endpoint:
  #               address:
  #                 socket_address:
  #                   address: 127.0.0.1
  #                   port_value: 8000
'''

with open(f"{plugin_dir}/config.yaml", "w") as f:
    f.write(config)
