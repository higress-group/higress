"""Generate an isolated Envoy bootstrap from built repo plugins and tag overlay."""
import json
import os
from pathlib import Path

root = Path(__file__).resolve().parent
source = {"headerName": "x-wujie-source", "allowedValues": ["wujieai"], "fallbackValue": "unknown"}


def wasm(name, configuration, instance=None):
    return {"name": "envoy.filters.http.wasm", "typed_config": {
        "@type": "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm",
        "config": {"name": instance or name, "root_id": instance or name,
                   "configuration": {"@type": "type.googleapis.com/google.protobuf.StringValue", "value": json.dumps(configuration)},
                   "vm_config": {"vm_id": instance or name, "runtime": "envoy.wasm.runtime.v8", "code": {"local": {"filename": f"/validation/{name}.wasm"}}}}}}


def cluster(name):
    return {"name": name, "connect_timeout": "1s", "type": "STRICT_DNS", "dns_lookup_family": "V4_ONLY",
            "load_assignment": {"cluster_name": name, "endpoints": [{"lb_endpoints": [{"endpoint": {"address": {"socket_address": {"address": "ope9626-higress-mock", "port_value": 8081}}}}]}]}}


filters = [
    wasm("request-block", {"block_headers": ["x-test-early-reject"]}, "early-reject"),
    wasm("ai-route-monitor", {"legacyMetrics": True, "terminalMetrics": os.environ.get("OPE9626_TERMINAL_METRICS", "1") == "1", "source": source, "models": {"allowedValues": ["gpt-4"]}, "asyncSubmissionPaths": ["/v1/tasks"]}),
    wasm("request-block", {"block_headers": ["x-test-reject"]}, "observable-reject"),
    wasm("ai-proxy", {"provider": {"type": "deepseek", "apiTokens": ["synthetic-provider-key"]}}),
    wasm("model-mapper", {"enableOnPathSuffix": ["/completions", "/responses", "/tasks"]}),
    wasm("ai-token-report", {"reportApiUrl": "http://fixture/model/usage/report", "reportServiceName": "billing", "source": source}),
    {"name": "envoy.filters.http.lua", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua", "inline_code": 'function envoy_on_request(h) h:headers():replace("x-wujie-source", "rewritten-after-snapshot") end'}},
    {"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}},
]
config = json.loads((root / "bootstrap-tags.json").read_text())
config.update({
    "admin": {"address": {"socket_address": {"address": "0.0.0.0", "port_value": 9901}}},
    "static_resources": {"clusters": [cluster("model"), cluster("outbound|80||billing.dns")], "listeners": [{
        "name": "fixture", "address": {"socket_address": {"address": "0.0.0.0", "port_value": 8080}},
        "filter_chains": [{"filters": [{"name": "envoy.filters.network.http_connection_manager", "typed_config": {
            "@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
            "stat_prefix": "fixture", "stream_idle_timeout": "3s", "route_config": {"name": "fixture", "virtual_hosts": [{"name": "fixture", "domains": ["*"], "routes": [{"name": "disconnect-fixture", "match": {"path": "/v1/disconnect/chat/completions"}, "route": {"cluster": "model", "timeout": "3s"}}, {"name": "generation", "match": {"prefix": "/"}, "route": {"cluster": "model", "timeout": "0.3s", "retry_policy": {"retry_on": "5xx", "num_retries": 1}}}]}]},
            "http_filters": filters}}]}]}]}})
(root / "envoy.json").write_text(json.dumps(config, indent=2))

# The relabel rules are exported verbatim from the repository PodMonitor.
(root / "prometheus.json").write_text(json.dumps({"global": {"scrape_interval": "1s"}, "scrape_configs": [{"job_name": "higress-fixture", "metrics_path": "/stats/prometheus", "static_configs": [{"targets": ["ope9626-higress-gateway:9901"], "labels": {"pod": "gateway-fixture", "namespace": "fixture"}}], "metric_relabel_configs": json.loads((root / "metric-relabelings.json").read_text())}]}, indent=2))
