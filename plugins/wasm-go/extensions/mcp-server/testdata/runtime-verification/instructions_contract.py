"""Independent case and request identities for the focused instructions run."""
import hashlib
import io
import json
import re
from collections import Counter
from http.client import parse_headers

MODERN = "2026-07-28"


def request(port, rpc_id, mode="normal", raw=False):
    return {"port": port, "rpcId": rpc_id, "mode": mode, "raw": raw,
            "accessPath": "/mcp" if port in (14001, 14002) else
                          "/instructions?api_key=<redacted>" if port == 14004 else "/instructions",
            "accessRequestId": "rv-instructions-disconnect" if raw else f"rv-{port}-{rpc_id}"}


BASELINE_REQUESTS = {
    "default-off-14001": (request(14001, "default-14001"),),
    "default-off-14002": (request(14002, "default-14002"),),
}
CANDIDATE_REQUESTS = {
    **BASELINE_REQUESTS,
    **{"field-" + mode: (request(14000, f"instructions-{mode}-14000", mode),)
       for mode in ("normal", "sse", "whitespace", "limit", "missing", "empty", "number", "object", "null", "oversize")},
    **{"failure-" + mode: (request(14000, f"instructions-{mode}-14000", mode),)
       for mode in ("http401", "http403", "http429", "http500", "rpc-error", "invalid")},
    "auth-anonymous": (request(14000, "instructions-auth-14000"),),
    "auth-fixed-header": (request(14003, "instructions-auth-14003"),),
    "auth-query": (request(14004, "instructions-auth-14004"),),
    "auth-passthrough": (request(14005, "instructions-auth-14005"),),
    "auth-public-discover-protected-tools": (request(14006, "instructions-auth-14006"),),
    "incomplete-preserves-existing-path": (request(14000, "instructions-incomplete-14000", "incomplete"),),
    # One client submission may have two configured backend attempts.
    "main-route-retry": (request(14007, "instructions-retry-14007", "retry"),),
    "main-route-timeout": (request(14008, "instructions-slow-14008", "slow"),),
    "later-filter-denial": (request(14009, "instructions-normal-14009"),),
    "current-request-no-cache-isolation": (
        request(14000, "instructions-fresh-14000", "fresh"), request(14000, "fresh-second", "fresh"),
        request(14000, "tenant-a", "identity"), request(14000, "tenant-b", "identity")),
    "disconnect-late-callback": (
        request(14000, "disconnect", "slow", raw=True), request(14000, "after-disconnect")),
}


def case_requests(variant):
    return {"baseline": BASELINE_REQUESTS, "candidate": CANDIDATE_REQUESTS}[variant]


def check_raw_disconnect(exchange, expected):
    raw = exchange["rawRequest"].encode("utf-8")
    assert hashlib.sha256(raw).hexdigest() == exchange["rawRequestSha256"], "raw disconnect hash differs"
    head, delimiter, body = raw.partition(b"\r\n\r\n")
    assert delimiter and head.startswith(b"POST /mcp HTTP/1.1\r\n"), "raw disconnect HTTP request absent"
    headers = parse_headers(io.BytesIO(head.split(b"\r\n", 1)[1] + b"\r\n\r\n"))
    assert int(headers["Content-Length"]) == len(body), "raw disconnect body framing differs"
    assert headers["X-Request-ID"] == expected["accessRequestId"], "raw disconnect request ID differs"
    assert headers["MCP-Protocol-Version"] == MODERN and headers["Mcp-Method"] == "server/discover", "raw disconnect protocol differs"
    assert headers["X-Instructions-Mode"] == expected["mode"], "raw disconnect fixture mode differs"
    rpc = json.loads(body)
    assert rpc["jsonrpc"] == "2.0" and rpc["id"] == expected["rpcId"] and rpc["method"] == "server/discover", "raw disconnect RPC differs"
    assert rpc["params"]["_meta"] == {"io.modelcontextprotocol/protocolVersion": MODERN,
                                      "io.modelcontextprotocol/clientInfo": {"name": "runtime-verifier", "version": "1.0.0"},
                                      "io.modelcontextprotocol/clientCapabilities": {}}, "raw disconnect metadata differs"
    assert exchange["response"] == {"status": "client_disconnected"}, "raw disconnect claimed an unobserved response"


def focused_evidence(snapshot, variant, log):
    contract = case_requests(variant)
    cases = snapshot.get("cases", [])
    exchanges = snapshot.get("exchanges", [])
    complete = (snapshot.get("expectedCases") == list(contract) and
                Counter(row.get("case") for row in cases) == Counter(contract.keys()) and
                all(row.get("status") == "PASS" for row in cases))
    expected_ids = [item["accessRequestId"] for items in contract.values() for item in items]
    errors = []
    try:
        assert Counter(exchange["accessRequestId"] for exchange in exchanges) == Counter(expected_ids), "global exchange IDs incomplete or duplicated"
        for name, expected_requests in contract.items():
            actual = [exchange for exchange in exchanges if exchange.get("case") == name]
            expected_by_id = {item["accessRequestId"]: item for item in expected_requests}
            assert Counter(item["accessRequestId"] for item in actual) == Counter(expected_by_id.keys()), f"{name}: exchange coverage differs"
            rows = [row for row in cases if row.get("case") == name]
            assert len(rows) == 1, f"{name}: case absent or duplicated"
            recorded = rows[0]["detail"]["clientExchanges"]
            assert Counter(json.dumps(item, sort_keys=True) for item in recorded) == Counter(json.dumps(item, sort_keys=True) for item in actual), f"{name}: case/global ledger differs"
            for exchange in actual:
                expected = expected_by_id[exchange["accessRequestId"]]
                observed = exchange["request"]
                fields = {"listenerPort": expected["port"], "rpcId": expected["rpcId"], "rpcMethod": "server/discover",
                          "mcpMethod": "server/discover", "protocolVersion": MODERN, "httpMethod": "POST", "path": "/mcp",
                          "instructionsMode": expected["mode"]}
                assert all(observed.get(key) == value for key, value in fields.items()), f"{name}: request metadata differs"
                if expected["raw"]:
                    assert exchange.get("transport") == "raw-disconnect", "raw disconnect transport absent"
                    check_raw_disconnect(exchange, expected)
                else:
                    assert exchange.get("transport", "http") == "http" and type(exchange["response"]["status"]) is int, f"{name}: HTTP response evidence absent"
    except (AssertionError, KeyError, TypeError, ValueError) as exc:
        errors.append(str(exc))
    accesses = re.findall(r"access request_id=(\S+) listener=(\S+) method=(\S+) path=(\S+) code=(\d+) flags=(\S+)", log)
    access_ok = (len(accesses) == sum("access request_id=" in line for line in log.splitlines()) and
                 Counter(item[0] for item in accesses) == Counter(expected_ids))
    by_id = {item["accessRequestId"]: item for items in contract.values() for item in items}
    observed_by_id = {item.get("accessRequestId"): item for item in exchanges}
    for request_id, listener, method, path, code, flags in accesses:
        expected = by_id.get(request_id)
        observed = observed_by_id.get(request_id, {})
        if expected is None:
            access_ok = False
            continue
        expected_code = "0" if expected["raw"] else str(observed.get("response", {}).get("status"))
        access_ok = access_ok and (listener == f"instructions-{expected['port']}" and method == "POST" and
                                   path == expected["accessPath"] and code == expected_code and
                                   (not expected["raw"] or "DC" in flags.split(",")))
    case_ledgers = {name: {"expectedIds": [item["accessRequestId"] for item in items],
                           "expectedCount": len(items),
                           "exchangeIds": [item.get("accessRequestId") for item in exchanges if item.get("case") == name],
                           "accessIds": [item[0] for item in accesses if item[0] in {entry["accessRequestId"] for entry in items}]}
                    for name, items in contract.items()}
    return {"cases": len(cases), "complete": complete, "expectedExchangeCount": len(expected_ids),
            "exchangeCount": len(exchanges), "exchangeLedgerComplete": not errors,
            "accessRecordCount": len(accesses), "accessLedgerComplete": access_ok, "caseLedgers": case_ledgers, "errors": errors}
