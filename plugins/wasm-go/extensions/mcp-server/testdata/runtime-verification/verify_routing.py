#!/usr/bin/env python3
"""Machine assertions. Affected is expected red only for named compatibility assertions."""
import concurrent.futures
import hashlib
import http.client
import json
import os
import socket
import time
from pathlib import Path

OUT = Path("/evidence")
VARIANT = os.environ["ROUTING_VARIANT"]
COMPAT = VARIANT != "affected"
RESULTS = []
HTTP = []
META = {"io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": {}}
PROFILES = [(18080, "legacy", False, 2)]
if VARIANT != "oracle": PROFILES += [(18081, "modern", True, 0), (18082, "auto-modern", True, 1), (18084, "auto-legacy", True, 3)]


def check(name, condition, details=None):
    RESULTS.append({"assertion": name, "pass": bool(condition), "details": details})


def ledger(key):
    with open(OUT / "ledger.jsonl") as source: return [row for row in map(json.loads, source) if row["key"] == key]


def request(port, name, modern, method="tools/call", scenario="normal", extra=None, path="/entry/mcp", disconnect=False, **args):
    key = name
    headers = {"content-type": "application/json", "accept": "application/json,text/event-stream", "baggage": key,
               "cookie": "synthetic=compat", "x-tenant": "synthetic-tenant", "authorization": "Bearer synthetic-removed",
               "x-case-id": key, "x-hash-key": args.pop("hash_key", key)}
    params = {"name": "echo", "arguments": {"scenario": scenario, **args}} if method == "tools/call" else {}
    if modern:
        params["_meta"] = META
        headers.update({"MCP-Protocol-Version": "2026-07-28", "Mcp-Method": method})
        if method == "tools/call": headers["Mcp-Name"] = "echo"; headers["Mcp-Param-Test"] = "synthetic-param"
    headers.update(extra or {})
    payload = json.dumps({"jsonrpc": "2.0", "id": name, "method": method, "params": params}).encode()
    conn = http.client.HTTPConnection("gateway", port, timeout=22)
    begin = time.monotonic()
    conn.request("POST", path, body=payload, headers=headers)
    if disconnect:
        time.sleep(.01); conn.close(); return None
    response = conn.getresponse()
    body = response.read()
    record = {"name": name, "port": port, "status": response.status, "headers": list(response.getheaders()), "body": body.decode(errors="replace"),
              "body_sha256": hashlib.sha256(body).hexdigest(), "elapsed": round(time.monotonic() - begin, 3)}
    HTTP.append(record); conn.close()
    try: record["json"] = json.loads(body)
    except ValueError: record["json"] = {}
    return record


def run():
    for _ in range(120):
        try:
            conn = http.client.HTTPConnection("gateway", 9901, timeout=2); conn.request("GET", "/ready"); ready = conn.getresponse(); ready.read(); conn.close()
            if ready.status == 200: break
        except OSError: pass
        time.sleep(.5)
    else: raise RuntimeError("Envoy did not become ready")
    for port, profile, modern, controls in PROFILES:
        for method in ("tools/list", "tools/call"):
            key = profile + "-" + method.replace("/", "-")
            response = request(port, key, modern, method)
            rows = ledger(key); business = [r for r in rows if r["operation"] == method]
            check(key + ":V1-sequence", len(rows) == controls + 1 and len(business) == 1, rows)
            check(key + ":V8-id-result", response["json"].get("id") == key and "result" in response["json"], response)
            for row in rows:
                h = {k.lower(): v for k, v in row["headers"]}
                check(key + ":V3-headers:" + row["operation"], (h.get("cookie") == "synthetic=compat" and h.get("x-tenant") == "synthetic-tenant" and h.get("x-before") == "present") == COMPAT, h)
                check(key + ":V3-auth-removed:" + row["operation"], "authorization" not in h)
                if row["operation"] in ("initialize", "notifications/initialized", "server/discover"):
                    check(key + ":V3-control-no-params:" + row["operation"], "mcp-param-test" not in h and "x-after" not in h)
            if business:
                h = {k.lower(): v for k, v in business[0]["headers"]}
                check(key + ":V2-later-filter", (h.get("x-after") == "present") == COMPAT, h)
                check(key + ":V4-route-header", (h.get("x-route") == "original") == COMPAT, h)
                check(key + ":V4-fixed-route", business[0]["backend"] != "wrong")
                if profile != "auto-legacy": check(key + ":V4-path-rewrite", (business[0]["path"] == "/rewritten") == COMPAT, business[0]["path"])
        key = profile + "-blocked"
        response = request(port, key, modern, extra={"x-block": "yes"})
        rows = ledger(key)
        check(key + ":V2-rejection", (response["status"] == 403 and not any(r["operation"] == "tools/call" for r in rows)) == COMPAT, response)
        for scenario, path, success in [("slow", "/entry/mcp", COMPAT), ("timeout", "/entry/timeout", not COMPAT), ("retry", "/entry/retry", COMPAT), ("retry", "/entry/mcp", False)]:
            key = profile + "-" + scenario + "-" + path.rsplit("/", 1)[-1]
            response = request(port, key, modern, scenario=scenario, path=path)
            check(key + ":V5-V6-result", ("result" in response["json"]) == success, response)
            attempts = [r for r in ledger(key) if r["operation"] == "tools/call"]
            check(key + ":V6-attempts", len(attempts) == (2 if scenario == "retry" and path.endswith("retry") and COMPAT else 1), attempts)
        for scenario in (() if VARIANT == "oracle" else ("fragmented", "sse-body", "invalid", "status-401", "status-403", "status-429", "status-500")):
            for empty in ([False, True] if scenario == "status-401" else [False]):
                key = profile + "-" + scenario + str(empty)
                response = request(port, key, modern, scenario=scenario, empty=empty)
                expect_success = scenario in ("fragmented", "sse-body")
                check(key + ":V8-envelope", response["json"].get("id") == key and ("result" in response["json"]) == expect_success, response)
                if scenario in ("status-401", "status-403"): check(key + ":V8-auth-status", response["status"] == int(scenario.split("-")[1]), response)
                check(key + ":V8-no-upstream-cookie", not any(k.lower() == "set-cookie" for k, _ in response["headers"]))
        for i in range(20): request(port, profile + "-disconnect-" + str(i), modern, scenario="disconnect", disconnect=True)
        time.sleep(.4)
        for i in range(20):
            rows = ledger(profile + "-disconnect-" + str(i))
            check(profile + ":V7-disconnect:" + str(i), sum(r["operation"] == "tools/call" for r in rows) <= 1)
    if COMPAT:
        identities = {}
        for key in range(12):
            for repeat in range(2):
                name = f"hash-{key}-{repeat}"
                request(18080, name, False, hash_key=f"stable-{key}")
                business = [r for r in ledger(name) if r["operation"] == "tools/call"]
                identities.setdefault(key, []).append(business[0]["backend"])
        check("V4-hash-stability", all(len(set(v)) == 1 for v in identities.values()) and len(set(v[0] for v in identities.values())) > 1, identities)
    # SSE uses a single backend channel; its ordinary headers and POST transport
    # are separately covered by the same ledger rather than routing HTTP calls.
    response = request(18083, "sse-compat", False)
    rows = ledger("sse-compat")
    check("V9-sse-result", "result" in response["json"], response)
    check("V9-sse-sequence", [r["operation"] for r in rows] == ["sse/get", "initialize", "notifications/initialized", "tools/call"], rows)
    check("V9-sse-headers", all((dict((k.lower(), v) for k, v in r["headers"]).get("cookie") == "synthetic=compat") == COMPAT for r in rows), rows)


if __name__ == "__main__":
    try: run()
    except Exception as error: check("runner-exception", False, repr(error))
    finally:
        (OUT / "http.json").write_text(json.dumps(HTTP, indent=2) + "\n")
        (OUT / "assertions.json").write_text(json.dumps(RESULTS, indent=2) + "\n")
    failed = [r for r in RESULTS if not r["pass"]]
    print(json.dumps({"variant": VARIANT, "assertions": len(RESULTS), "failed": [r["assertion"] for r in failed]}))
    raise SystemExit(bool(failed))
