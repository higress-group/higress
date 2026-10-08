#!/usr/bin/env python3
"""One deterministic configuration for all source variants; JSON is Envoy YAML."""
import json
from pathlib import Path

PROFILES = [(18080, "legacy", "http"), (18081, "modern", "http"), (18082, "auto", "http"), (18083, "legacy", "sse"), (18084, "auto-legacy", "http")]


def typed(kind, **values):
    return {"@type": "type.googleapis.com/envoy.extensions." + kind, **values}


def fixture():
    listeners = []
    for port, strategy, transport in PROFILES:
        name = strategy + "-" + transport
        server = {"name": "routing", "type": "mcp-proxy", "transport": transport, "mcpServerURL": "http://backend:8080/target", "protocolStrategy": "auto" if strategy == "auto-legacy" else strategy}
        if strategy == "auto-legacy": server["mcpServerURL"] = "http://backend:8080/legacy"
        # Omit strategy for the oracle-compatible default legacy configuration.
        if strategy == "legacy": del server["protocolStrategy"]
        routes = [{"match": {"prefix": "/target"}, "route": {"cluster": "wrong"}}]
        for prefix, timeout, retry in [("/entry/timeout", "2s", False), ("/entry/retry", "15s", True), ("/", "15s", False)]:
            route = {"cluster": "backend", "timeout": timeout, "hash_policy": [{"header": {"header_name": "x-hash-key"}}]}
            if transport == "http": route["regex_rewrite"] = {"pattern": {"google_re2": {}, "regex": "^/target$"}, "substitution": "/rewritten"}
            if retry: route["retry_policy"] = {"retry_on": "5xx", "num_retries": 1, "per_try_timeout": "7s"}
            routes.append({"match": {"prefix": prefix}, "route": route,
                           "request_headers_to_add": [{"header": {"key": "x-route", "value": "original"}, "append_action": "OVERWRITE_IF_EXISTS_OR_ADD"}]})
        if transport == "sse":
            for entry in routes[1:]: entry["route"]["cluster"] = "sse"
        config = {"stat_prefix": name, "route_config": {"name": name, "virtual_hosts": [{"name": "all", "domains": ["*"], "routes": routes}]},
                  "access_log": [{"name": "envoy.access_loggers.stdout", "typed_config": typed("access_loggers.stream.v3.StdoutAccessLog")}],
                  "http_filters": [
                      {"name": "envoy.filters.http.lua", "typed_config": typed("filters.http.lua.v3.Lua", inline_code='function envoy_on_request(h) h:headers():add("x-before", "present") end')},
                      {"name": "envoy.filters.http.wasm", "typed_config": typed("filters.http.wasm.v3.Wasm", config={"name": name, "root_id": name,
                       "vm_config": {"vm_id": name, "runtime": "envoy.wasm.runtime.v8", "code": {"local": {"filename": "/evidence/plugin.wasm"}}},
                       "configuration": {"@type": "type.googleapis.com/google.protobuf.StringValue", "value": json.dumps({"server": server})}})},
                      {"name": "envoy.filters.http.lua", "typed_config": typed("filters.http.lua.v3.Lua", inline_code='function envoy_on_request(h) h:headers():add("x-after", "present"); if h:headers():get("x-block") == "yes" then h:respond({[":status"]="403"}, "blocked by later filter") end end')},
                      {"name": "envoy.filters.http.router", "typed_config": typed("filters.http.router.v3.Router")}]}
        listeners.append({"name": name, "address": {"socket_address": {"address": "0.0.0.0", "port_value": port}}, "filter_chains": [{"filters": [{"name": "envoy.filters.network.http_connection_manager", "typed_config": typed("filters.network.http_connection_manager.v3.HttpConnectionManager", **config)}]}]})
    clusters = []
    for name, host in [("backend", "backend"), ("wrong", "wrong"), ("sse", "backend")]:
        clusters.append({"name": name, "type": "STRICT_DNS", "connect_timeout": "2s", "dns_lookup_family": "V4_ONLY", "lb_policy": "RING_HASH",
                         "load_assignment": {"cluster_name": name, "endpoints": [{"lb_endpoints": [{"endpoint": {"address": {"socket_address": {"address": host, "port_value": 8080}}}}]}]}})
    backend = clusters[0]["load_assignment"]["endpoints"][0]["lb_endpoints"]
    backend.append({"endpoint": {"address": {"socket_address": {"address": "backend-alt", "port_value": 8080}}}})
    return {"admin": {"address": {"socket_address": {"address": "0.0.0.0", "port_value": 9901}}}, "static_resources": {"listeners": listeners, "clusters": clusters}}


if __name__ == "__main__":
    import sys
    Path(sys.argv[1]).write_text(json.dumps(fixture(), indent=2) + "\n")
