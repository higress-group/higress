"""Focused listener configuration, reusing the existing Envoy renderer."""
import json
from copy import deepcopy
from pathlib import Path

import generate_envoy as base

PORTS = {"registered": 15000, "restricted": 15001, "rest": 15002,
         "http-legacy": 15003, "http-modern": 15004, "auto-modern": 15005,
         "auto-legacy": 15006, "sse": 15007, "sse-path": 15008,
         "sse-rewrite": 15009, "sse-static-header": 15010,
         "sse-static-query": 15011, "sse-forward": 15012,
         "sse-passthrough": 15013, "sse-removed": 15014,
         "e2bdev": 15015, "firecrawl": 15016, "sse-path-rewrite": 15017, "composed": 15018, "sse-static-authorization": 15019, "registered-body": 15020}


def proxy(name):
    strategy = "auto" if name.startswith("auto-") else "modern" if name == "http-modern" else "legacy"
    path = "auto-modern" if name == "auto-modern" else "auto-legacy" if name == "auto-legacy" else "modern" if strategy == "modern" else "legacy"
    server = {"name": "compat-" + name, "type": "mcp-proxy", "transport": "http", "protocolStrategy": strategy,
              "mcpServerURL": "http://initial.virtual/unified/" + path, "timeout": 10000}
    if name.startswith("sse"):
        server["transport"] = "sse"
        server["mcpServerURL"] = "/unified/sse" if name in ("sse-path", "sse-path-rewrite") else "http://initial.virtual/unified/sse?initial=one"
    if name in ("sse-static-header", "sse-static-query", "sse-static-authorization", "sse-passthrough"):
        scheme = {"id": "up", "type": "apiKey", "in": "query" if name == "sse-static-query" else "header",
                  "name": "api_key" if name == "sse-static-query" else "X-Upstream-Key", "defaultCredential": "compat-static"}
        if name == "sse-static-authorization":
            scheme = {"id": "up", "type": "http", "scheme": "bearer", "defaultCredential": "compat-static"}
        if name == "sse-passthrough":
            scheme = {"id": "up", "type": "http", "scheme": "bearer"}
            server["defaultDownstreamSecurity"] = {"id": "down", "passthrough": True}
        server["securitySchemes"] = [scheme, {"id": "down", "type": "apiKey", "in": "header", "name": "X-Client-Key"}]
        server["defaultUpstreamSecurity"] = {"id": "up"}
    if name == "sse-forward":
        server["passthroughAuthHeader"] = True
    return {"server": server, "tools": [{"name": "echo", "inputSchema": {"type": "object"}}]}


def configs(out, variant):
    out = Path(out)
    registered = {"server": {"name": "runtime-schema-fixtures"}}
    rest = deepcopy(base.SCHEMA_COMPATIBILITY)
    rest["tools"][0]["requestTemplate"]["url"] = "http://backend-primary:8080/unified/rest/converted/{transactionId}?fixed=yes"
    rest["tools"][1]["requestTemplate"]["url"] = "http://backend-primary:8080/unified/rest/health"
    rest["tools"] += [{"name": "legacy-" + state, "legacyOnly": state == "true",
                        "outputSchema": {"type": "object"}, "responseTemplate": {"body": "visible"}}
                       for state in ("false", "true")]
    entries = []
    for name, port in PORTS.items():
        config = deepcopy(registered) if name in ("registered", "registered-body", "restricted", "composed") else deepcopy(rest) if name == "rest" else proxy(name)
        wasm = f"registered-{variant}.wasm" if name in ("registered", "registered-body", "restricted", "composed") else f"plain-{variant}.wasm"
        if name == "composed":
            config["_rules_"] = [{"_match_domain_": ["mcp.runtime.test"], "toolSet": {"name": "compat-composed", "serverTools": [{"serverName": "runtime-schema-fixtures", "tools": ["raw_summary", "numeric_comparison"]}]}}]
        if name == "restricted":
            config["allowTools"] = ["runtime_counters", "raw_summary"]
        if name in ("e2bdev", "firecrawl"):
            config = json.loads((out / f"builtin-{name}.json").read_text())
        if variant == "oracle" and name in ("http-modern", "auto-modern", "auto-legacy"):
            continue
        listener = base.listener_yaml(port, "unified-" + name, "compat-route", config, wasm)
        if name in ("sse-rewrite", "sse-path-rewrite"):
            listener = listener.replace("cluster: compat-route, timeout: 30s", "cluster: compat-route, timeout: 30s, host_rewrite_literal: fixed.virtual")
        entries.append(listener)
    # Existing auto assertions and cancellation/barrier fixtures retain their
    # own ports and strict-control expectations rather than a second model.
    if variant != "oracle":
        entries += [base.listener_yaml(*entry, wasm_file=f"plain-{variant}.wasm") for entry in base.LISTENERS if entry[0] >= 10009]
        previous = base.OUT
        try:
            base.OUT = out
            base.instructions_configs()
        finally:
            base.OUT = previous
        instructions = (out / ("envoy-instructions-baseline.yaml" if variant == "control" else "envoy-instructions-candidate.yaml")).read_text()
        entries.append(instructions.split("  listeners:\n", 1)[1].split("  clusters:\n", 1)[0])
    rendered = "admin:\n  address:\n    socket_address: {address: 0.0.0.0, port_value: 9901}\nstatic_resources:\n  listeners:\n" + "".join(entries)
    cluster = base.cluster_yaml("compat-route").replace("address: compat-route,", "address: backend-primary,")
    rendered += "  clusters:\n" + cluster + base.cluster_yaml("backend-primary") + base.cluster_yaml("backend-secondary")
    (out / "envoy.yaml").write_text(rendered)


def sse_get_auth_config(out):
    # One fixed listener/VM and filename keep affected/candidate configs identical.
    config = base.listener_yaml(PORTS["sse"], "sse-get-auth", "compat-route", proxy("sse"), "plugin.wasm")
    cluster = base.cluster_yaml("compat-route").replace("address: compat-route,", "address: backend-primary,")
    Path(out, "envoy.yaml").write_text("admin:\n  address:\n    socket_address: {address: 0.0.0.0, port_value: 9901}\nstatic_resources:\n  listeners:\n" + config + "  clusters:\n" + cluster)


if __name__ == "__main__":
    import sys
    if "--sse-get-auth-headers" in sys.argv[3:]:
        if sys.argv[2] not in ("affected", "candidate"): raise SystemExit("focused role must be affected or candidate")
        sse_get_auth_config(sys.argv[1])
    else:
        configs(sys.argv[1], sys.argv[2])
