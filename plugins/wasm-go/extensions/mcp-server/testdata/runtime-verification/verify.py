#!/usr/bin/env python3
"""Execute the MCP runtime matrix entirely through Envoy listeners."""

import hashlib
import json
import os
import socket
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

from typed_canonical import DESCRIPTOR_MISMATCH_EXIT, canonical_json_sha256, loads_typed


EVIDENCE = Path(os.environ.get("RUNTIME_EVIDENCE", "/evidence"))
RESULTS = []
EXCHANGES = []
CURRENT_CASE = None
MODERN = "2026-07-28"
LEGACY = ("2024-11-05", "2025-03-26", "2025-06-18")
GATEWAY_HOST = os.environ.get("RUNTIME_GATEWAY_HOST", "gateway")
GENERATION_TRANSITION = os.environ.get("RUNTIME_GENERATION_TRANSITION", "") == "1"
COMPATIBILITY_ORACLE = os.environ.get("RUNTIME_ORACLE", "") == "1"
CORPUS_REVISION = os.environ.get("RUNTIME_CORPUS_REVISION", "")
DESCRIPTOR_SELF_TEST = os.environ.get("RUNTIME_DESCRIPTOR_SELF_TEST", "") == "1"
LAST_TYPED_RESPONSE = None
CORPUS_PARTIAL_RECORDS = None
CORPUS_CURRENT_RECORD = None


def check(condition, message):
    if not condition:
        raise AssertionError(message)


class DescriptorMismatchError(Exception):
    pass


def check_descriptor_hash(observed, expected, message):
    if observed != expected:
        raise DescriptorMismatchError(f"{message}: expected {expected}, got {observed}")


def exchange(url, body=None, headers=None, method="POST"):
    global LAST_TYPED_RESPONSE
    data = None if body is None else json.dumps(body, separators=(",", ":")).encode()
    request = urllib.request.Request(url, data=data, method=method)
    for name, value in (headers or {}).items():
        request.add_header(name, value)
    try:
        response = urllib.request.urlopen(request, timeout=15)
        raw = response.read()
        status, response_headers = response.status, dict(response.headers.items())
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        status, response_headers = exc.code, dict(exc.headers.items())
    try:
        parsed = json.loads(raw) if raw else None
    except json.JSONDecodeError:
        parsed = {"raw": raw.decode("utf-8", "replace")[:500]}
    try:
        LAST_TYPED_RESPONSE = loads_typed(raw) if raw else None
    except (json.JSONDecodeError, ValueError):
        LAST_TYPED_RESPONSE = None
    request_headers = {name.lower(): value for name, value in (headers or {}).items()}
    access_request_id = request_headers.get("x-request-id")
    if access_request_id:
        response_header_lookup = {name.lower(): value for name, value in response_headers.items()}
        selected_response_headers = {}
        for name in ("content-type", "www-authenticate", "mcp-protocol-version", "x-request-id"):
            if name in response_header_lookup:
                selected_response_headers[name] = response_header_lookup[name]
        selected_response_headers["mcp-session-id-present"] = "mcp-session-id" in response_header_lookup
        canonical_body = json.dumps(parsed, sort_keys=True, separators=(",", ":")).encode()
        request_body = body if isinstance(body, dict) else {}
        request_params = request_body.get("params") or {}
        EXCHANGES.append({
            "case": CURRENT_CASE,
            "accessRequestId": access_request_id,
            "request": {
                "listenerPort": urllib.parse.urlparse(url).port,
                "httpMethod": method,
                "path": urllib.parse.urlparse(url).path,
                "rpcMethod": request_body.get("method"),
                "rpcId": request_body.get("id"),
                "toolName": request_params.get("name"),
                "protocolVersion": request_headers.get("mcp-protocol-version"),
                "mcpMethod": request_headers.get("mcp-method"),
                "mcpName": request_headers.get("mcp-name"),
                "origin": request_headers.get("origin"),
                "authorizationPresent": "authorization" in request_headers,
                "cookiePresent": "cookie" in request_headers,
                "sessionPresent": "mcp-session-id" in request_headers,
                "lastEventIDPresent": "last-event-id" in request_headers,
                "futureParamPresent": "mcp-param-future" in request_headers,
            },
            "response": {
                "status": status,
                "headers": selected_response_headers,
                "body": parsed,
                "bodySha256": hashlib.sha256(canonical_body).hexdigest(),
            },
        })
    return status, response_headers, parsed


def wait_http(url):
    deadline = time.time() + 45
    while time.time() < deadline:
        try:
            status, _, _ = exchange(url, method="GET")
            if status == 200:
                return
        except (OSError, urllib.error.URLError, socket.timeout):
            pass
        time.sleep(0.5)
    raise RuntimeError("gateway/backend did not become ready: " + url)


def modern_meta():
    return {
        "io.modelcontextprotocol/protocolVersion": MODERN,
        "io.modelcontextprotocol/clientInfo": {"name": "runtime-verifier", "version": "1.0.0"},
        "io.modelcontextprotocol/clientCapabilities": {},
    }


def modern_rpc(port, rpc_method, rpc_id, name=None, arguments=None, extra_headers=None,
               origin="http://mcp.runtime.test", host="mcp.runtime.test"):
    params = {"_meta": modern_meta()}
    if name is not None:
        params["name"] = name
    if arguments is not None:
        params["arguments"] = arguments
    body = {"jsonrpc": "2.0", "id": rpc_id, "method": rpc_method, "params": params}
    headers = {
        "Host": host, "Origin": origin,
        "Content-Type": "application/json", "Accept": "application/json, text/event-stream",
        "MCP-Protocol-Version": MODERN, "Mcp-Method": rpc_method,
        "X-Request-ID": f"rv-{port}-{rpc_id}",
    }
    if name is not None:
        headers["Mcp-Name"] = name
    headers.update(extra_headers or {})
    return exchange(f"http://{GATEWAY_HOST}:{port}/mcp", body, headers)


def legacy_rpc(port, body, session=None, version=None, request_id=None, host="mcp.runtime.test"):
    headers = {"Host": host, "Content-Type": "application/json", "Accept": "application/json, text/event-stream"}
    stable_id = request_id or body.get("id") or body.get("method", "request").replace("/", "-")
    headers["X-Request-ID"] = f"rv-{port}-{stable_id}"
    if session:
        headers["Mcp-Session-Id"] = session
    if version:
        headers["MCP-Protocol-Version"] = version
    return exchange(f"http://{GATEWAY_HOST}:{port}/mcp", body, headers)


def wait_lds_version(phase, admin_port=9921):
    deadline = time.time() + 45
    expected = f'"version_info":"{phase}"'
    while time.time() < deadline:
        try:
            status, _, dump = exchange(f"http://{GATEWAY_HOST}:{admin_port}/config_dump", method="GET")
            if status == 200 and expected in json.dumps(dump, separators=(",", ":")):
                return
        except (OSError, urllib.error.URLError, socket.timeout):
            pass
        time.sleep(0.25)
    raise RuntimeError(f"Envoy did not publish LDS generation {phase}")


def apply_lds_generation(phase):
    source = EVIDENCE / f"lds-generation-{phase}.yaml"
    current = EVIDENCE / "lds-generation-current.yaml"
    temporary = EVIDENCE / f".lds-generation-{phase}.tmp"
    temporary.write_bytes(source.read_bytes())
    os.replace(temporary, current)
    wait_lds_version(phase)


def read_lds_rejected_count(admin_port=9981):
    status, _, stats = exchange(
        f"http://{GATEWAY_HOST}:{admin_port}/stats?filter=listener_manager.lds.update_rejected&format=json",
        method="GET",
    )
    check(status == 200 and isinstance(stats, dict), f"LDS rejection stats unavailable: {status} {stats}")
    for stat in stats.get("stats", []):
        if stat.get("name") == "listener_manager.lds.update_rejected":
            return int(stat.get("value", 0))
    return 0


def wait_lds_rejected_count(admin_port=9981, greater_than=None, timeout=45):
    deadline = time.time() + timeout
    successful_polls = 0
    last_error = None
    while time.time() < deadline:
        try:
            count = read_lds_rejected_count(admin_port)
            successful_polls += 1
            if greater_than is None or count > greater_than:
                return count
        except (OSError, urllib.error.URLError, socket.timeout, AssertionError) as exc:
            last_error = exc
        time.sleep(0.25)
    if successful_polls == 0:
        raise RuntimeError(f"LDS rejection checker remained unavailable: {last_error}")
    raise RuntimeError(f"Envoy did not report an LDS rejection above {greater_than}")


def apply_corpus_fixture(revision, fixture, expected_acceptance):
    source = EVIDENCE / f"lds-corpus-{revision}-{fixture}.yaml"
    current = EVIDENCE / f"lds-corpus-{revision}-current.yaml"
    temporary = EVIDENCE / f".lds-corpus-{revision}-{fixture}.tmp"
    rejected_before = wait_lds_rejected_count()
    temporary.write_bytes(source.read_bytes())
    os.replace(temporary, current)
    version = f"{revision}-{fixture}"
    if expected_acceptance:
        wait_lds_version(version, 9981)
        return True
    wait_lds_rejected_count(greater_than=rejected_before)
    return False


def generation_transition():
    wait_http("http://backend-primary:8080/healthz")
    wait_http(f"http://{GATEWAY_HOST}:9921/ready")
    affected = "getTransactionRecordListV2"
    records = []
    for phase in ("valid-before", "validation-unavailable", "valid-after"):
        if phase == "valid-before":
            wait_lds_version(phase)
        else:
            apply_lds_generation(phase)
        backend_reset()
        exchange_start = len(EXCHANGES)
        status, _, listed = modern_rpc(12008, "tools/list", f"generation-list-{phase}")
        check(status == 200, f"generation list failed: {status} {listed}")
        tools = result_contract(listed, ttl=True).get("tools") or []
        by_name = {tool.get("name"): tool for tool in tools}
        check(set(by_name) == {affected, "compat_health"}, f"generation tools changed: {tools}")
        business = by_name[affected]["inputSchema"]["properties"]["businessType"]
        arguments = {"transactionId": "transition", "businessType": ["SALE"]}
        status, _, called = modern_rpc(12008, "tools/call", f"generation-call-{phase}", affected, arguments)
        check(status == 200, f"generation call failed: {status} {called}")
        events = backend_state()["events"]
        if phase == "validation-unavailable":
            check(business.get("enum") == ["SALE", "REFUND"], f"degraded descriptor changed: {business}")
            check(called == {
                "jsonrpc": "2.0", "id": f"generation-call-{phase}",
                "error": {"code": -32603, "message": "tool input schema validation is unavailable",
                          "data": {"reason": "schema_validation_unavailable"}},
            }, f"degraded generation did not block exactly: {called}")
            check(events == [], f"degraded generation reached backend: {events}")
            state = "validation-unavailable"
        else:
            check("enum" not in business, f"valid descriptor unexpectedly degraded: {business}")
            result_contract(called)
            check(len(events) == 1 and events[0]["path"] == "/compat/transition", f"valid generation did not invoke exactly once: {events}")
            state = "validated"
        records.append({
            "phase": phase,
            "schemaState": state,
            "descriptor": by_name[affected],
            "response": called,
            "backendEvents": {"backend-primary": events},
            "clientExchanges": EXCHANGES[exchange_start:],
        })
        print(f"PASS generation-{phase} state={state} backend_calls={len(events)}", flush=True)
    detail = {"ldsVersions": [record["phase"] for record in records], "generations": records}
    (EVIDENCE / "generation-transition.json").write_text(json.dumps(detail, indent=2, sort_keys=True) + "\n")
    return 0


def result_contract(response, ttl=False):
    check(isinstance(response, dict) and "result" in response, f"missing result: {response}")
    result = response["result"]
    check(result.get("resultType") == "complete", f"resultType is not complete: {result}")
    info = (result.get("_meta") or {}).get("io.modelcontextprotocol/serverInfo")
    check(isinstance(info, dict) and info.get("name") and info.get("version"), f"missing serverInfo: {result}")
    if ttl:
        check(result.get("ttlMs") == 0 and result.get("cacheScope") == "private", f"bad cache wire fields: {result}")
    forbidden = json.dumps(result)
    for token in ("2025-11-25", "listChanged", "subscriptions", "requestState", "multi-round"):
        check(token not in forbidden, f"forbidden advertised capability {token}: {result}")
    return result


def evidence_snapshot():
    snapshots = {}
    for host in ("backend-primary", "backend-secondary"):
        try:
            snapshots[host] = backend_state(host)["events"]
        except Exception as exc:
            snapshots[host] = {"snapshotError": str(exc)}
    return snapshots


def write_corpus_partial():
    if CORPUS_PARTIAL_RECORDS is None or not CORPUS_REVISION:
        return
    result = {"revision": CORPUS_REVISION, "fixtures": CORPUS_PARTIAL_RECORDS}
    (EVIDENCE / f"corpus-{CORPUS_REVISION}.json").write_text(
        json.dumps(result, indent=2, sort_keys=True) + "\n",
    )


def record_corpus_failure(exc):
    if CORPUS_CURRENT_RECORD is None:
        return
    CORPUS_CURRENT_RECORD["diagnosticError"] = str(exc)
    CORPUS_CURRENT_RECORD["backendEvents"] = evidence_snapshot()
    write_corpus_partial()


def record(name, callback):
    global CURRENT_CASE
    start = len(EXCHANGES)
    CURRENT_CASE = name
    try:
        detail = callback() or {}
        detail["clientExchanges"] = EXCHANGES[start:]
        RESULTS.append({"case": name, "status": "PASS", "detail": detail})
        print(f"PASS {name}", flush=True)
    except Exception as exc:
        RESULTS.append({"case": name, "status": "FAIL", "detail": {"error": str(exc), "backendEvents": evidence_snapshot(), "clientExchanges": EXCHANGES[start:]}})
        print(f"FAIL {name}: {exc}", flush=True)
    finally:
        CURRENT_CASE = None


def backend_reset(host="backend-primary"):
    status, _, _ = exchange(f"http://{host}:8080/__reset", {}, {"Content-Type": "application/json"})
    check(status == 200, "backend reset failed")


def backend_state(host="backend-primary"):
    status, _, body = exchange(f"http://{host}:8080/__state", method="GET")
    check(status == 200, "backend state failed")
    return body


def direct_mode(port, tool_name, arguments, expected_path):
    def case():
        backend_reset()
        status, _, discover = modern_rpc(port, "server/discover", f"discover-{port}")
        check(status == 200, f"discover status {status}: {discover}")
        discovered = result_contract(discover, ttl=True)
        check(discovered.get("capabilities") == {"tools": {}}, f"capabilities are not tools-only: {discovered}")
        check(discovered.get("supportedVersions") == ["2024-11-05", "2025-03-26", "2025-06-18", "2026-07-28"], f"supportedVersions are not exact: {discovered}")
        status, _, listed = modern_rpc(port, "tools/list", f"list-{port}")
        check(status == 200, f"list status {status}: {listed}")
        tools = result_contract(listed, ttl=True).get("tools") or []
        check(tool_name in [tool.get("name") for tool in tools], f"tool absent: {tools}")
        status, _, called = modern_rpc(port, "tools/call", f"call-{port}", tool_name, arguments)
        check(status == 200, f"call status {status}: {called}")
        result_contract(called)
        events = backend_state()["events"]
        matching = [event for event in events if event["path"] == expected_path]
        check(len(matching) == 1, f"expected exactly one real backend call: {events}")
        return {"backendCalls": len(events), "toolCount": len(tools), "backendEvents": {"backend-primary": events}}
    return case


def rest_rejections():
    backend_reset()
    status, _, response = modern_rpc(10001, "tools/call", "invalid-rest", "get_weather", {})
    check(status == 200 and response["result"].get("isError") is True, f"invalid args not a tool error: {status} {response}")
    check("invalid arguments for tool" in json.dumps(response), f"missing argument validation detail: {response}")
    check(len(backend_state()["events"]) == 0, "invalid REST args reached backend")
    status, _, response = modern_rpc(10001, "tools/list", "hostile-origin", origin="https://hostile.invalid")
    check(status == 403, f"hostile Origin status {status}: {response}")
    check(len(backend_state()["events"]) == 0, "hostile Origin reached backend")
    return {"invalidArgsStatus": 200, "toolExecutionError": True, "hostileOriginStatus": 403, "backendCalls": 0, "backendEvents": {"backend-primary": []}}


def composed_boundary():
    backend_reset()
    status, _, listed = modern_rpc(10002, "tools/list", "composed-list")
    check(status == 200, f"composed list status {status}: {listed}")
    tools = result_contract(listed, ttl=True).get("tools") or []
    check([tool.get("name") for tool in tools] == ["amap-tools___maps_weather"], f"unexpected composed tools: {tools}")
    status, _, called = modern_rpc(10002, "tools/call", "composed-call", "amap-tools___maps_weather", {"city": "Hangzhou"})
    check(status in (400, 404, 405), f"standalone composed call must be rejected: {status} {called}")
    check((called.get("error") or {}).get("code") == -32601, f"wrong boundary error: {called}")
    check(len(backend_state()["events"]) == 0, "rejected composed call reached backend")
    return {"list": "supported", "publishedTool": "amap-tools___maps_weather", "call": "requires mcp-router", "callStatus": status, "backendEvents": {"backend-primary": []}}


def legacy_direct_versions():
    for index, version in enumerate(LEGACY):
        init = {"jsonrpc": "2.0", "id": f"init-{index}", "method": "initialize", "params": {"protocolVersion": version, "capabilities": {}, "clientInfo": {"name": "runtime-verifier", "version": "1"}}}
        status, headers, response = legacy_rpc(10001, init)
        check(status == 200 and response["result"]["protocolVersion"] == version, f"legacy initialize failed: {status} {response}")
        initialized = {"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}}
        status, _, _ = legacy_rpc(10001, initialized, version=version, request_id=f"initialized-{index}")
        check(status in (200, 202), f"initialized failed: {status}")
        status, _, listed = legacy_rpc(10001, {"jsonrpc": "2.0", "id": f"list-{index}", "method": "tools/list", "params": {}}, version=version)
        check(status == 200 and listed.get("result", {}).get("tools"), f"legacy list failed: {status} {listed}")
        status, _, called = legacy_rpc(10001, {"jsonrpc": "2.0", "id": f"call-{index}", "method": "tools/call", "params": {"name": "get_weather", "arguments": {"city": "Hangzhou"}}}, version=version)
        check(status == 200 and "result" in called, f"legacy call failed: {status} {called}")
        text = json.dumps([response, listed, called])
        for forbidden in ("resultType", "ttlMs", "cacheScope", "io.modelcontextprotocol/serverInfo"):
            check(forbidden not in text, f"modern field leaked to legacy {version}: {forbidden}")
    events = backend_state()["events"]
    check(len(events) == 3 and all(event["path"] == "/rest/weather" for event in events), f"legacy REST calls did not reach real backend exactly three times: {events}")
    return {"versions": list(LEGACY), "flow": "initialize/initialized/list/call", "backendCalls": 3, "backendEvents": {"backend-primary": events}}


def schema_compatibility():
    backend_reset()
    affected = "getTransactionRecordListV2"
    valid = "compat_health"

    status, _, listed = modern_rpc(10008, "tools/list", "compat-modern-list")
    check(status == 200, f"modern compatibility list failed: {status} {listed}")
    tools = result_contract(listed, ttl=True).get("tools") or []
    by_name = {tool.get("name"): tool for tool in tools}
    check(set(by_name) == {affected, valid}, f"compatibility tools are not exact: {tools}")
    business = by_name[affected]["inputSchema"]["properties"]["businessType"]
    check(business == {"description": "Business types", "enum": ["SALE", "REFUND"], "items": {"type": "string"}, "type": "array"},
          f"affected descriptor changed: {business}")

    def call_valid(rpc_id):
        status, _, response = modern_rpc(10008, "tools/call", rpc_id, valid, {})
        check(status == 200, f"valid compatibility tool failed: {status} {response}")
        result_contract(response)

    call_valid("compat-valid-before")
    status, _, blocked = modern_rpc(10008, "tools/call", "compat-blocked", affected, {"businessType": ["SALE"]})
    check(status == 200, f"degraded modern call status is not 200: {status} {blocked}")
    check(blocked == {
        "jsonrpc": "2.0", "id": "compat-blocked",
        "error": {"code": -32603, "message": "tool input schema validation is unavailable",
                  "data": {"reason": "schema_validation_unavailable"}},
    }, f"degraded modern error envelope changed: {blocked}")
    events_after_block = backend_state()["events"]
    check(len(events_after_block) == 1 and events_after_block[0]["path"] == "/compat/health",
          f"degraded modern call reached backend: {events_after_block}")
    call_valid("compat-valid-after")

    expected_arguments = {
        "transactionId": "tx-42", "page": "7", "X-Compat-Flag": "true",
        "businessType": ["SALE", "REFUND"], "payload": {"amount": 12},
    }
    for index, version in enumerate(LEGACY):
        status, _, legacy_list = legacy_rpc(
            10008, {"jsonrpc": "2.0", "id": f"compat-list-{index}", "method": "tools/list", "params": {}},
            version=version,
        )
        check(status == 200, f"legacy compatibility list failed for {version}: {status} {legacy_list}")
        legacy_tools = {tool.get("name"): tool for tool in legacy_list.get("result", {}).get("tools", [])}
        check(set(legacy_tools) == {affected, valid}, f"legacy compatibility descriptors absent for {version}: {legacy_tools}")
        check(legacy_tools[affected]["inputSchema"]["properties"]["businessType"]["enum"] == ["SALE", "REFUND"],
              f"legacy compatibility enum changed for {version}: {legacy_tools[affected]}")
        status, _, called = legacy_rpc(
            10008,
            {"jsonrpc": "2.0", "id": f"compat-call-{index}", "method": "tools/call",
             "params": {"name": affected, "arguments": expected_arguments}},
            version=version,
        )
        check(status == 200 and "result" in called, f"legacy compatibility call failed for {version}: {status} {called}")

    events = backend_state()["events"]
    valid_events = [event for event in events if event["path"] == "/compat/health"]
    legacy_events = [event for event in events if event["path"] == "/compat/tx-42"]
    check(len(valid_events) == 2 and len(legacy_events) == len(LEGACY), f"compatibility backend counts changed: {events}")
    for event in legacy_events:
        observed = event.get("compatibilityRequest") or {}
        check(event["httpMethod"] == "POST", f"legacy method changed: {event}")
        check(observed.get("query") == {"fixed": ["yes"], "page": ["7"]}, f"legacy query changed: {event}")
        check(observed.get("flag") == "true", f"legacy header conversion changed: {event}")
        check(observed.get("jsonBody") == {"businessType": ["SALE", "REFUND"], "payload": {"amount": 12}},
              f"legacy JSON body changed: {event}")
    return {
        "modernDescriptorPreserved": True,
        "modernBlockedError": "schema_validation_unavailable",
        "modernBlockedBackendCalls": 0,
        "validCallsBeforeAndAfter": 2,
        "legacyVersions": list(LEGACY),
        "legacyBackendCalls": len(legacy_events),
        "backendEvents": {"backend-primary": events},
    }


def compatibility_oracle():
    wait_http("http://backend-primary:8080/healthz")
    wait_http(f"http://{GATEWAY_HOST}:9941/ready")
    backend_reset()
    affected = "getTransactionRecordListV2"
    version = LEGACY[-1]
    status, _, listed = legacy_rpc(
        13018,
        {"jsonrpc": "2.0", "id": "oracle-list", "method": "tools/list", "params": {}},
        version=version,
    )
    check(status == 200, f"v2.0.0 oracle list failed: {status} {listed}")
    tools = {tool.get("name"): tool for tool in listed.get("result", {}).get("tools", [])}
    check(set(tools) == {affected, "compat_health"}, f"v2.0.0 oracle tools changed: {tools}")
    check(tools[affected]["inputSchema"]["properties"]["businessType"]["enum"] == ["SALE", "REFUND"],
          f"v2.0.0 oracle descriptor changed: {tools[affected]}")
    arguments = {
        "transactionId": "tx-42", "page": "7", "X-Compat-Flag": "true",
        "businessType": ["SALE", "REFUND"], "payload": {"amount": 12},
    }
    status, _, called = legacy_rpc(
        13018,
        {"jsonrpc": "2.0", "id": "oracle-call", "method": "tools/call",
         "params": {"name": affected, "arguments": arguments}},
        version=version,
    )
    check(status == 200 and "result" in called, f"v2.0.0 oracle call failed: {status} {called}")
    events = backend_state()["events"]
    check(len(events) == 1 and events[0]["path"] == "/compat/tx-42", f"v2.0.0 oracle backend count changed: {events}")
    observed = events[0].get("compatibilityRequest") or {}
    check(events[0]["httpMethod"] == "POST", f"v2.0.0 oracle method changed: {events[0]}")
    check(observed.get("query") == {"fixed": ["yes"], "page": ["7"]}, f"v2.0.0 oracle query changed: {events[0]}")
    check(observed.get("flag") == "true", f"v2.0.0 oracle header conversion changed: {events[0]}")
    check(observed.get("jsonBody") == {"businessType": ["SALE", "REFUND"], "payload": {"amount": 12}},
          f"v2.0.0 oracle JSON body changed: {events[0]}")
    result = {
        "configurationAccepted": True,
        "descriptorPreserved": True,
        "legacyVersion": version,
        "legacyRESTMapping": True,
        "backendEvents": {"backend-primary": events},
        "clientExchanges": EXCHANGES,
    }
    (EVIDENCE / "oracle-verification.json").write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
    (EVIDENCE / "backend-oracle-state.json").write_text(json.dumps({"events": events}, indent=2, sort_keys=True) + "\n")
    return 0


def corpus_verification():
    global CORPUS_PARTIAL_RECORDS, CORPUS_CURRENT_RECORD
    records = []
    CORPUS_PARTIAL_RECORDS = records
    write_corpus_partial()
    wait_http("http://backend-primary:8080/healthz")
    wait_http(f"http://{GATEWAY_HOST}:9981/ready")
    manifest = json.loads((EVIDENCE / "corpus-manifest.json").read_text())
    for fixture in manifest["fixtures"]:
        slug = fixture["fixture"]
        expected = fixture["expectedAcceptance"][CORPUS_REVISION]
        record = {
            "fixture": slug,
            "expectedAcceptance": expected,
            "actualAcceptance": None,
            "tool": fixture["tool"],
            "legacyMappingExpected": fixture["legacyMapping"],
            "modernList": False,
            "modernDescriptorSha256": None,
            "modernCallBlocked": False,
            "legacyList": False,
            "legacyDescriptorSha256": None,
            "legacyRESTMapping": False,
            "validSiblingCallable": False,
            "validSiblingResultContract": False,
            "validSiblingBackendEvents": [],
            "globalList": False,
            "backendEvents": {"backend-primary": []},
        }
        CORPUS_CURRENT_RECORD = record
        records.append(record)
        write_corpus_partial()
        actual = apply_corpus_fixture(CORPUS_REVISION, slug, expected)
        record["actualAcceptance"] = actual
        write_corpus_partial()
        check(actual == expected, f"{CORPUS_REVISION}/{slug} acceptance mismatch")
        if not actual:
            CORPUS_CURRENT_RECORD = None
            continue

        backend_reset()
        port = fixture["port"]
        tool_name = fixture["tool"]
        if CORPUS_REVISION == "candidate":
            status, _, listed = modern_rpc(port, "tools/list", f"corpus-{slug}-modern-list")
            check(status == 200, f"candidate modern list failed for {slug}: {status} {listed}")
            tools = result_contract(listed, ttl=True).get("tools") or []
            typed_tools = (LAST_TYPED_RESPONSE.get("result") or {}).get("tools") or []
            tools_by_name = {tool.get("name"): tool for tool in tools}
            typed_tools_by_name = {tool.get("name"): tool for tool in typed_tools}
            check(tool_name in tools_by_name, f"candidate descriptor missing for {slug}: {tools}")
            check(tool_name in typed_tools_by_name, f"candidate typed descriptor missing for {slug}")
            modern_descriptor_hash = canonical_json_sha256(typed_tools_by_name[tool_name].get("inputSchema"))
            check_descriptor_hash(
                modern_descriptor_hash, fixture["expectedInputSchemaSha256"],
                f"candidate modern descriptor changed for {slug}",
            )
            record["modernDescriptorSha256"] = modern_descriptor_hash
            if slug == "mixed-valid-invalid":
                check("valid_sibling" in {tool.get("name") for tool in tools}, f"valid sibling missing for {slug}: {tools}")
                status, _, valid_called = modern_rpc(port, "tools/call", f"corpus-{slug}-valid", "valid_sibling", {})
                check(status == 200, f"valid sibling call failed for {slug}: {status} {valid_called}")
                valid_result = result_contract(valid_called)
                check(valid_result.get("isError") is False,
                      f"valid sibling returned an MCP error result for {slug}: {valid_result}")
                content = valid_result.get("content")
                check(isinstance(content, list) and len(content) > 0,
                      f"valid sibling returned no successful content for {slug}: {valid_result}")
                sibling_events = backend_state()["events"]
                check(
                    len(sibling_events) == 1
                    and sibling_events[0]["httpMethod"] == "GET"
                    and sibling_events[0]["path"] == "/corpus/valid",
                    f"valid sibling did not make exactly one successful backend call for {slug}: {sibling_events}",
                )
                record["validSiblingCallable"] = True
                record["validSiblingResultContract"] = True
                record["validSiblingBackendEvents"] = sibling_events
                write_corpus_partial()
                backend_reset()
            record["modernList"] = True
            write_corpus_partial()
            status, _, blocked = modern_rpc(
                port, "tools/call", f"corpus-{slug}-modern-call", tool_name,
                {"recordId": "record-42", "page": 7, "X-Corpus-Flag": True, "payload": {"amount": 12}},
            )
            check(status == 200 and (blocked.get("error") or {}).get("code") == -32603,
                  f"candidate modern call was not blocked for {slug}: {status} {blocked}")
            check((blocked.get("error") or {}).get("data", {}).get("reason") == "schema_validation_unavailable",
                  f"candidate modern reason changed for {slug}: {blocked}")
            check(backend_state()["events"] == [], f"candidate modern blocked call reached upstream for {slug}")
            record["modernCallBlocked"] = True
            write_corpus_partial()

        status, _, listed = legacy_rpc(
            port, {"jsonrpc": "2.0", "id": f"corpus-{slug}-legacy-list", "method": "tools/list", "params": {}},
            version=LEGACY[-1],
        )
        check(status == 200, f"{CORPUS_REVISION} legacy list failed for {slug}: {status} {listed}")
        legacy_tools = {tool.get("name"): tool for tool in listed.get("result", {}).get("tools", [])}
        typed_legacy_tools = {
            tool.get("name"): tool for tool in LAST_TYPED_RESPONSE.get("result", {}).get("tools", [])
        }
        check(tool_name in legacy_tools, f"{CORPUS_REVISION} legacy descriptor missing for {slug}: {listed}")
        check(tool_name in typed_legacy_tools, f"{CORPUS_REVISION} typed legacy descriptor missing for {slug}")
        legacy_descriptor_hash = canonical_json_sha256(typed_legacy_tools[tool_name].get("inputSchema"))
        check_descriptor_hash(
            legacy_descriptor_hash, fixture["expectedInputSchemaSha256"],
            f"{CORPUS_REVISION} legacy descriptor changed for {slug}",
        )
        record["legacyDescriptorSha256"] = legacy_descriptor_hash
        record["legacyList"] = True
        write_corpus_partial()

        if slug == "rule-level":
            if CORPUS_REVISION == "candidate":
                status, _, global_listed = modern_rpc(
                    port, "tools/list", f"corpus-{slug}-global-modern-list",
                    origin="http://global.runtime.test", host="global.runtime.test",
                )
                check(status == 200, f"candidate global rule list failed: {status} {global_listed}")
                global_tools = result_contract(global_listed, ttl=True).get("tools") or []
            else:
                status, _, global_listed = legacy_rpc(
                    port,
                    {"jsonrpc": "2.0", "id": f"corpus-{slug}-global-legacy-list", "method": "tools/list", "params": {}},
                    version=LEGACY[-1], host="global.runtime.test",
                )
                global_tools = global_listed.get("result", {}).get("tools", [])
            check(status == 200 and {tool.get("name") for tool in global_tools} == {"global_valid"},
                  f"{CORPUS_REVISION} global rule fallback changed: {status} {global_listed}")
            record["globalList"] = True
            write_corpus_partial()

        if fixture["legacyMapping"]:
            arguments = {
                "recordId": "record-42", "page": 7, "X-Corpus-Flag": True,
                "payload": {"amount": 12},
            }
            status, _, called = legacy_rpc(
                port,
                {"jsonrpc": "2.0", "id": f"corpus-{slug}-legacy-call", "method": "tools/call",
                 "params": {"name": tool_name, "arguments": arguments}},
                version=LEGACY[-1],
            )
            check(status == 200 and "result" in called,
                  f"{CORPUS_REVISION} legacy call failed for {slug}: {status} {called}")
            events = backend_state()["events"]
            check(len(events) == 1 and events[0]["path"] == f"/corpus/{slug}/record-42",
                  f"{CORPUS_REVISION} legacy path/count changed for {slug}: {events}")
            observed = events[0].get("compatibilityRequest") or {}
            check(events[0]["httpMethod"] == "POST", f"legacy method changed for {slug}: {events[0]}")
            check(observed.get("query") == {"fixed": ["yes"], "page": ["7"]},
                  f"legacy query changed for {slug}: {events[0]}")
            check(observed.get("flag") == "true", f"legacy header changed for {slug}: {events[0]}")
            check(observed.get("jsonBody") == {"payload": {"amount": 12}},
                  f"legacy body changed for {slug}: {events[0]}")
            record["legacyRESTMapping"] = True
            record["backendEvents"] = {"backend-primary": events}
        CORPUS_CURRENT_RECORD = None
        write_corpus_partial()
    CORPUS_CURRENT_RECORD = None
    write_corpus_partial()
    return 0


def descriptor_self_test():
    manifest = json.loads((EVIDENCE / "corpus-manifest.json").read_text())
    fixture_name = os.environ["RUNTIME_DESCRIPTOR_FIXTURE"]
    actual_path = Path(os.environ["RUNTIME_DESCRIPTOR_ACTUAL"])
    actual = loads_typed(actual_path.read_bytes())
    fixture = next(item for item in manifest["fixtures"] if item["fixture"] == fixture_name)
    observed = canonical_json_sha256(actual)
    check_descriptor_hash(
        observed, fixture["expectedInputSchemaSha256"], f"descriptor self-test mismatch for {fixture_name}",
    )
    return 0


def modern_proxy():
    backend_reset()
    sensitive = {
        "Cookie": "downstream-cookie", "Mcp-Session-Id": "downstream-session",
        "Last-Event-ID": "downstream-event",
        "x-unrelated-credential": "unrelated", "Authorization": "Bearer downstream-not-policy",
        "x-envoy-allow-mcp-tools": "proxy_echo",
    }
    status, _, listed = modern_rpc(10003, "tools/list", "proxy-modern-list", extra_headers=sensitive)
    check(status == 200, f"modern proxy list failed: {status} {listed}")
    result_contract(listed, ttl=True)
    call_headers = dict(sensitive)
    call_headers["Mcp-Param-Future"] = "opaque-current-call"
    status, _, called = modern_rpc(10003, "tools/call", "proxy-modern-call", "proxy_echo", {"value": "modern"}, call_headers)
    check(status == 200, f"modern proxy call failed: {status} {called}")
    result_contract(called)
    events = backend_state()["events"]
    check([event["rpcMethod"] for event in events] == ["tools/list", "tools/call"], f"modern proxy probed/retried: {events}")
    for event in events:
        check(event["protocolVersion"] == MODERN and event["mcpMethod"] == event["rpcMethod"], f"modern metadata absent: {event}")
        check(not any(event[key] for key in ("cookiePresent", "sessionPresent", "lastEventIDPresent", "internalRoutePresent", "unrelatedCredentialPresent", "authorizationPresent")), f"sensitive header leaked: {event}")
    check(events[0]["futureParam"] is None and events[1]["futureParam"] == "opaque-current-call", f"Mcp-Param scope wrong: {events}")
    return {"upstreamSequence": [event["rpcMethod"] for event in events], "singleStatelessRPC": True, "backendEvents": {"backend-primary": events}}


def modern_to_legacy():
    backend_reset()
    for rpc_method, rpc_id, name, arguments in (
        ("tools/list", "bridge-list", None, None),
        ("tools/call", "bridge-call", "proxy_echo", {"value": "bridge"}),
    ):
        status, _, response = modern_rpc(10004, rpc_method, rpc_id, name, arguments, {"Mcp-Param-Future": "must-not-cross-era"})
        check(status == 200, f"modern-to-legacy failed: {status} {response}")
        result_contract(response, ttl=rpc_method == "tools/list")
    events = backend_state()["events"]
    methods = [event["rpcMethod"] for event in events]
    check(methods == ["initialize", "notifications/initialized", "tools/list", "initialize", "notifications/initialized", "tools/call"], f"legacy exchange not isolated: {events}")
    for event in events:
        check(event["futureParam"] is None, f"modern Mcp-Param crossed legacy boundary: {event}")
        check(not any(event[key] for key in ("cookiePresent", "lastEventIDPresent", "internalRoutePresent", "unrelatedCredentialPresent", "authorizationPresent")), f"sensitive header leaked: {event}")
    return {"upstreamSequence": methods, "isolatedHandshakes": 2, "backendEvents": {"backend-primary": events}}


def default_legacy_proxy():
    backend_reset()
    for index, version in enumerate(LEGACY):
        init = {"jsonrpc": "2.0", "id": f"default-init-{index}", "method": "initialize", "params": {"protocolVersion": version, "capabilities": {}, "clientInfo": {"name": "runtime-verifier", "version": "1"}}}
        status, headers, response = legacy_rpc(10005, init)
        check(status == 200 and response.get("result", {}).get("protocolVersion") == version, f"default legacy init failed: {status} {response}")
        status, _, _ = legacy_rpc(10005, {"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}}, version=version, request_id=f"default-initialized-{index}")
        check(status in (200, 202), f"default legacy initialized failed: {status}")
        status, _, listed = legacy_rpc(10005, {"jsonrpc": "2.0", "id": f"default-list-{index}", "method": "tools/list", "params": {}}, version=version)
        check(status == 200 and listed.get("result", {}).get("tools"), f"default legacy list failed: {status} {listed}")
        status, _, called = legacy_rpc(10005, {"jsonrpc": "2.0", "id": f"default-call-{index}", "method": "tools/call", "params": {"name": "proxy_echo", "arguments": {"value": version}}}, version=version)
        check(status == 200 and "result" in called, f"default legacy call failed: {status} {called}")
    events = backend_state()["events"]
    expected_methods = []
    for _ in LEGACY:
        expected_methods.extend(["initialize", "notifications/initialized", "tools/list"])
        expected_methods.extend(["initialize", "notifications/initialized", "tools/call"])
    check(len(events) == 18, f"default legacy exchanges emitted unexpected extra requests: {events}")
    check(all(event["path"] == "/legacy" for event in events), f"default legacy forwarded original downstream request: {events}")
    check([event["rpcMethod"] for event in events] == expected_methods, f"default legacy sequence is not exact: {events}")
    return {"versions": list(LEGACY), "upstreamInitializeCount": 6, "backendEvents": {"backend-primary": events}}


def unsupported_legacy_to_modern():
    backend_reset()
    init = {"jsonrpc": "2.0", "id": "unsupported", "method": "initialize", "params": {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "runtime-verifier", "version": "1"}}}
    status, _, response = legacy_rpc(10003, init)
    check(status == 200, f"legacy initialize compatibility failed before unsupported bridge: {status} {response}")
    status, _, _ = legacy_rpc(10003, {"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}}, version="2025-06-18", request_id="unsupported-initialized")
    check(status in (200, 202), f"legacy initialized failed: {status}")
    status, _, response = legacy_rpc(10003, {"jsonrpc": "2.0", "id": "unsupported-list", "method": "tools/list", "params": {}}, version="2025-06-18")
    check(status >= 400 or "error" in (response or {}), f"legacy-to-modern list unexpectedly bridged: {status} {response}")
    check(len(backend_state()["events"]) == 0, "unsupported path probed/fell back/retried upstream")
    return {"status": status, "upstreamCalls": 0, "autoDetection": False, "backendEvents": {"backend-primary": []}}


def auth_and_isolation():
    backend_reset()
    status, headers, response = modern_rpc(10006, "tools/call", "auth-ok", "proxy_echo", {"value": "authorized"})
    check(status == 200, f"explicit auth failed: {status} {response}")
    events = backend_state()["events"]
    check(len(events) == 1 and events[0]["authorizationMatchesExplicitPolicy"], f"explicit Authorization policy not applied: {events}")

    for mode, expected in (("auth401", 401), ("auth403", 403)):
        status, response_headers, _ = modern_rpc(10006, "tools/call", mode, "proxy_echo", {"value": mode}, {"Mcp-Param-Test-Mode": mode})
        check(status == expected, f"upstream {expected} not preserved: {status}")
        www = next((value for key, value in response_headers.items() if key.lower() == "www-authenticate"), "")
        check(www.startswith("Bearer"), f"WWW-Authenticate not preserved: {response_headers}")

    backend_reset("backend-secondary")
    status, _, response = modern_rpc(10007, "tools/call", "after-failure", "proxy_echo", {"value": "secondary"})
    check(status == 200, f"secondary origin failed after primary RPC errors: {status} {response}")
    secondary = backend_state("backend-secondary")["events"]
    check(len(secondary) == 1 and secondary[0]["origin"] == "secondary", f"secondary origin evidence wrong: {secondary}")
    check(not any(secondary[0][key] for key in ("authorizationPresent", "cookiePresent", "sessionPresent", "lastEventIDPresent", "internalRoutePresent", "unrelatedCredentialPresent")), f"state/header leaked to other origin: {secondary}")
    return {"explicitAuth": True, "preservedStatuses": [401, 403], "postFailureOrigin": "secondary", "backendEvents": {"backend-primary": backend_state()["events"], "backend-secondary": secondary}}


def main():
    if DESCRIPTOR_SELF_TEST:
        return descriptor_self_test()
    if GENERATION_TRANSITION:
        return generation_transition()
    if COMPATIBILITY_ORACLE:
        return compatibility_oracle()
    if CORPUS_REVISION:
        return corpus_verification()
    wait_http("http://backend-primary:8080/healthz")
    wait_http(f"http://{GATEWAY_HOST}:9901/ready")
    cases = [
        ("registered-modern-discover-list-call", direct_mode(10000, "maps_weather", {"city": "Hangzhou"}, "/v3/weather/weatherInfo")),
        ("rest-modern-discover-list-call", direct_mode(10001, "get_weather", {"city": "Hangzhou"}, "/rest/weather")),
        ("rest-invalid-args-and-hostile-origin", rest_rejections),
        ("composed-list-and-router-call-boundary", composed_boundary),
        ("rest-three-legacy-versions", legacy_direct_versions),
        ("rest-schema-compatibility-modern-block-and-legacy-pass-through", schema_compatibility),
        ("proxy-modern-stateless-and-header-isolation", modern_proxy),
        ("proxy-modern-to-legacy-isolated-handshakes", modern_to_legacy),
        ("proxy-default-is-legacy-for-three-versions", default_legacy_proxy),
        ("proxy-legacy-to-modern-is-explicitly-unsupported", unsupported_legacy_to_modern),
        ("proxy-auth-errors-and-cross-origin-isolation", auth_and_isolation),
    ]
    for name, callback in cases:
        record(name, callback)
    primary = backend_state()
    secondary = backend_state("backend-secondary")
    (EVIDENCE / "backend-primary-final.json").write_text(json.dumps(primary, indent=2, sort_keys=True))
    (EVIDENCE / "backend-secondary-final.json").write_text(json.dumps(secondary, indent=2, sort_keys=True))
    (EVIDENCE / "client-exchanges.json").write_text(json.dumps({"exchanges": EXCHANGES}, indent=2, sort_keys=True))
    (EVIDENCE / "matrix.json").write_text(json.dumps({"cases": RESULTS}, indent=2, sort_keys=True))
    failed = [result for result in RESULTS if result["status"] != "PASS"]
    print(f"SUMMARY pass={len(RESULTS) - len(failed)} fail={len(failed)}", flush=True)
    return 1 if failed else 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except DescriptorMismatchError as exc:
        record_corpus_failure(exc)
        print(f"descriptor mismatch: {exc}", file=sys.stderr)
        sys.exit(DESCRIPTOR_MISMATCH_EXIT)
    except Exception as exc:
        record_corpus_failure(exc)
        raise
