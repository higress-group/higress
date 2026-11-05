#!/usr/bin/env python3
"""Bounded fixture/oracle fault injection, also run by orchestration_self_test."""
import base64
import hashlib
import gzip
import copy
import http.client
import io
import json
import socket
import tempfile
import threading
from collections import Counter
from http.client import parse_headers
from pathlib import Path

import backend
import compat_backend
import compat_contract as c
import compat_evidence as evidence
import compat_fixture
import verify_auto
from verify_compat import Compatibility, capture_sse_auth_response, inspect_sse_get_auth
from instructions_self_test import valid_evidence


def require(value, message):
    if not value:
        raise AssertionError(message)


def rejected(callback):
    try:
        callback()
    except (AssertionError, ValueError, TypeError, UnicodeError):
        return
    raise AssertionError("strict peer accepted an invalid request")


def headers(extra=""):
    return parse_headers(io.BytesIO(("Content-Type: application/json\r\nAccept: application/json,text/event-stream\r\nMCP-Protocol-Version: 2026-07-28\r\nMcp-Method: tools/call\r\nMcp-Name: echo\r\n" + extra + "\r\n").encode()))


def outbound():
    raw = c.envelope(True, tool="echo", rpc_id='"strict"')
    return raw.replace(b'"io.modelcontextprotocol/clientCapabilities":{}', b'"io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"Higress-mcp-proxy","version":"1"}')


def strict_peer_checks():
    raw = outbound()
    c.strict_request(raw, headers(), True)
    for token in ("null", "-0", "1.50", "1E+03", "9223372036854775808"):
        rejected(lambda token=token: c.strict_request(raw.replace(b'"strict"', token.encode()), headers(), True))
    for extra in ("Mcp-Method: tools/call\r\n", "Content-Type: application/json\r\n", "Mcp-Name: echo\r\n"):
        rejected(lambda extra=extra: c.strict_request(raw, headers(extra), True))
    for carrier in (b'"arguments":{}', b'"_meta":{'):
        replacement = b'"arguments":{"x":1,"x":2}' if carrier.endswith(b'{}') else b'"_meta":{"vendor":{"x":1,"x":2},'
        rejected(lambda carrier=carrier, replacement=replacement: c.strict_request(raw.replace(carrier, replacement), headers(), True))
    rejected(lambda: c.strict_request(raw[:-2] + b',"unknown":{"x":1,"x":2}}}', headers(), True))
    rejected(lambda: c.strict_request(raw.replace(b'clientCapabilities":{}', b'clientCapabilities":{"sampling":{}}'), headers(), True))
    rejected(lambda: c.strict_request(raw + b'{}', headers(), True))
    rejected(lambda: c.strict_request(b'[' + raw + b']', headers(), True))
    name = "界" * 3000
    encoded = "=?base64?" + base64.b64encode(name.encode()).decode() + "?="
    h = headers(); h.replace_header("Mcp-Name", encoded)
    c.strict_request(raw.replace(b'"name":"echo"', ('"name":' + c.token(name)).encode()), h, True)
    rejected(lambda: c.strict_request(raw, h, True))
    # Both request and result numeric tokens avoid float conversion.
    require(c.unique_json(b'{"n":900719925474099312345,"e":1E+03}')["e"].lexeme == "1E+03", "numeric token rounded")



def sse_correlation_checks():
    # Exercise the real case verifier with recorded upstream/downstream values.
    # The old SSE call ID is 2; the candidate deliberately separates list/call.
    expected_arguments = c.unique_json(b'{"number":900719925474099312345,"decimal":1.50,"exponent":1E+03,"choice":2,"nested":{"x":2}}')
    from typed_canonical import canonical_json_sha256
    successful_projections = {}
    for variant, method, expected_id in (("oracle", "tools/list", "2"), ("oracle", "tools/call", "2"),
                                         ("candidate", "tools/list", "2"), ("candidate", "tools/call", "3")):
        for response_mode in ("success", "rpc-error"):
            case = {"kind": "id", "profile": "legacy", "route": "sse", "method": method,
                    "id_index": 0, "response": response_mode}
            verifier = Compatibility(variant, ".")
            def run(internal_id, client_id="client-id", initialize_id="1", result_change=None):
                response = {"jsonrpc": "2.0", "id": client_id}
                if response_mode == "success":
                    response["result"] = c.unique_json(b'{"tools":[{"name":"echo","inputSchema":{"type":"object"}}],"content":[],"value":2,"recipient":"initial.virtual"}')
                    response["result"]["number"] = c.JSONNumber("900719925474099300000" if variant == "oracle" else "900719925474099312345")
                    if result_change: result_change(response["result"])
                else:
                    response["error"] = {"code": c.JSONNumber("-32603"), "message": "backend tool call failed"}
                events = [{"kind": "control", "rpcMethod": "initialize", "rpcId": initialize_id,
                           "accepted": True, "initializeCapabilityKeys": []},
                          {"kind": "business", "rpcMethod": method, "rpcId": internal_id, "accepted": True, "authority": "initial.virtual",
                           "argumentsTypedSha256": canonical_json_sha256(expected_arguments)}]
                verifier.exchange = lambda *args, **kwargs: (200, {}, response)
                verifier.ledger = lambda: {"events": events, "attempts": {"transport": 1, "probe": 0, "control": 2, "business": 1}}
                return verifier.proxy_case(case)
            detail = run(expected_id)
            require(detail["ledger"]["events"][-1]["rpcId"] == expected_id, "upstream ID evidence was normalized away")
            for wrong in {"1", "2", "3"} - {expected_id}:
                rejected(lambda wrong=wrong: run(wrong))
            rejected(lambda: run(expected_id, client_id="wrong-client"))
            rejected(lambda: run(expected_id, initialize_id="2"))
            if response_mode == "success":
                if variant == "oracle": successful_projections[method] = detail["projection"]
                else: require(detail["projection"] == successful_projections[method], "approved precision normalization did not compare corresponding business results")
                wrapped = run(expected_id, result_change=lambda result: result.update(resultType="complete", ttlMs=c.JSONNumber("0"), cacheScope="private", _meta={"io.modelcontextprotocol/serverInfo": {"name": "gateway"}}))
                require(wrapped["projection"] == detail["projection"], "protocol envelope normalization changed business projection")
                rejected(lambda: run(expected_id, result_change=lambda result: result.update(value=c.JSONNumber("1"))))
                for field in ("tools", "content"):
                    rejected(lambda field=field: run(expected_id, result_change=lambda result: result.pop(field)))
                rejected(lambda: run(expected_id, result_change=lambda result: result.update(_meta={"vendor": {"retained": True}})))



def cleanup_failure_checks():
    # Drive main's failure/finally path without invoking Podman or a compiler.
    # Empty container inventory must not hide failed network/volume cleanup.
    import contextlib
    import subprocess
    import sys
    from unittest.mock import patch
    import run_compat
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        harness = root / "source-harness"; harness.mkdir()
        (harness / "registered_fixture.go.txt").write_text("shared fixture")
        for cleanup_status in (0, 1):
            output = root / str(cleanup_status)
            def fake_run(command, log, **options):
                if command[:3] == ["podman", "image", "inspect"]:
                    raise RuntimeError("fixture image inspection failure")
                status = cleanup_status if "down" in command else 0
                if str(command[-1]).endswith("finalize_evidence.py"):
                    (output / "compat-manifest.json").write_text(json.dumps({"passed": False, "checksums": {}}))
                    status = 1
                if options.get("target"): Path(options["target"]).write_text("")
                return subprocess.CompletedProcess(command, status, stdout="")
            with patch.object(run_compat, "HERE", harness), patch.object(run_compat, "source_is_clean", return_value=True), \
                 patch.object(run_compat.subprocess, "check_output", return_value="a" * 40), \
                 patch.object(run_compat, "recorded_run", side_effect=fake_run), \
                 patch.object(sys, "argv", ["run_compat.py", "--output", str(output)]), contextlib.redirect_stdout(io.StringIO()):
                require(run_compat.main() == 1, "injected runner failure was lost")
            inputs = json.loads((output / "compat-inputs.json").read_text())
            require(inputs["cleanupExitCode"] == cleanup_status and inputs["cleanupComplete"] == (cleanup_status == 0), "empty containers masked compose cleanup failure")



def counter_and_legacy_diagnostic_checks():
    verifier = Compatibility("candidate", ".")
    def counter_case(before, after):
        replies = iter(((200, {}, {"result": {}}), (200, {}, {"result": before}),
                        (400, {}, {"error": {"code": c.JSONNumber("-32700")}}), (200, {}, {"result": after})))
        verifier.exchanges.clear()
        def exchange(*args, **kwargs):
            verifier.exchanges.append({})
            return next(replies)
        verifier.exchange = exchange
        return verifier.input_case({"profile": "legacy", "shape": "invalid"})
    stable = {"generation": "one", "creates": c.JSONNumber("1"), "calls": c.JSONNumber("1")}
    require(counter_case(stable, stable)["zeroCreateCall"], "stable negative proof rejected")
    require(verifier.exchanges[1]["executionCounters"] == {"generation": "one", "creates": 1, "calls": 1}, "counter observations omitted")
    for after in ({**stable, "generation": "two"}, {**stable, "creates": c.JSONNumber("2")}, {**stable, "calls": c.JSONNumber("2")}):
        rejected(lambda after=after: counter_case(stable, after))
    rejected(lambda: counter_case({**stable, "creates": c.JSONNumber("0"), "calls": c.JSONNumber("0")}, stable))
    verifier = Compatibility("control", ".")
    message = 'tool "legacy-true" is unavailable in the modern profile because its input schema cannot be validated: configured as legacy-only'
    result = {"isError": True, "content": [{"type": "text", "text": message}]}
    def diagnostic(response, attempts=0):
        verifier.exchange = lambda *args, **kwargs: (200, {}, response)
        verifier.ledger = lambda: {"attempts": {"business": attempts}}
        return verifier.red_case({"red": "legacyOnly"})
    require(diagnostic({"result": result})["diagnosticRedReproduced"], "legacyOnly tool error result rejected")
    for wrong in ({"error": {"code": -32603}}, {"result": {**result, "isError": False}}, {"result": {"isError": True, "content": []}}):
        rejected(lambda wrong=wrong: diagnostic(wrong))
    rejected(lambda: diagnostic({"result": result}, attempts=1))



def sse_get_auth_focused_checks():
    import verify
    from unittest.mock import patch
    cases = c.sse_get_auth_cases()
    require(len(cases) == 12 and len({case["id"] for case in cases}) == 12, "focused registry missing/duplicated")
    compressed = gzip.compress(c.SSE_GET_AUTH_BODY, mtime=0)
    source_headers = [["Content-Type", "application/json"], ["Content-Length", str(len(compressed))], ["Content-Encoding", "gzip"],
                      ["Set-Cookie", "synthetic-cookie=private"], ["Mcp-Session-Id", "synthetic-session"], ["X-Upstream-Private", "synthetic-private"],
                      ["Retry-After", "7"], *[["WWW-Authenticate", value] for value in c.SSE_GET_AUTH_CHALLENGES]]
    fixture = {"headerPairs": source_headers, "rawBodyBase64": base64.b64encode(compressed).decode(), **c.body_record(compressed)}
    good_headers = [["Content-Type", "application/json"], *[["WWW-Authenticate", value] for value in c.SSE_GET_AUTH_CHALLENGES]]
    def observations(case, affected):
        raw = b"" if case["profile"] == "notification" else b'{"jsonrpc":"2.0","id":' + case["rpcId"].encode() + b',"error":{"code":-32603,"message":"upstream authorization failed"}}'
        headers = good_headers if not affected else [pair for pair in source_headers if pair[0] != "Content-Length" and pair != ["WWW-Authenticate", c.SSE_GET_AUTH_CHALLENGES[0]]]
        captured, decoded = capture_sse_auth_response(raw, headers)
        response = c.loads_typed(decoded) if decoded else None
        exchange = {"case": case["id"], "accessRequestId": "focused-" + case["id"], "httpStatus": case["status"], "rawResponse": captured}
        event = {"case": case["id"], "kind": "transport", "method": "GET", "path": "/unified/sse", "authority": "initial.virtual", "backendOrigin": "primary", "authFixture": {**fixture, "status": case["status"]}}
        return exchange, response, {"events": [event], "attempts": {"transport": 1, "probe": 0, "control": 0, "business": 0}}
    for case in cases:
        for affected in (False, True):
            exchange, response, ledger = observations(case, affected)
            invariant, isolation = inspect_sse_get_auth(case, exchange, response, ledger)
            require(not invariant and bool(isolation) == affected, "focused red/green misclassified")
            require(base64.b64decode(exchange["rawResponse"]["rawBodyBase64"]) == (b"" if case["profile"] == "notification" else base64.b64decode(observations(case, False)[0]["rawResponse"]["rawBodyBase64"])), "raw response bytes lost")
        exchange, response, ledger = observations(case, False)
        combined = copy.deepcopy(exchange)
        combined["rawResponse"]["headerPairs"] = [["Content-Type", "application/json"], ["WWW-Authenticate", ", ".join(c.SSE_GET_AUTH_CHALLENGES)]]
        require(inspect_sse_get_auth(case, combined, response, ledger) == ([], []), "legal challenge field combination rejected")
        for fault in ("status", "wrong-case", "extra", "fixture"):
            bad_exchange, bad_response, bad_ledger = observations(case, True)
            if fault == "status": bad_exchange["httpStatus"] = 500
            elif fault == "wrong-case": bad_ledger["events"][0]["case"] = "unassociated"
            elif fault == "extra": bad_ledger["events"].append({"case": "unknown", "kind": "business"})
            else: bad_ledger["events"][0]["authFixture"] = {}
            require(bool(inspect_sse_get_auth(case, bad_exchange, bad_response, bad_ledger)[0]), "invalid red accepted: " + fault)
    # Wrong identity, notification body and an unmodified upstream auth body
    # must not be accepted as the targeted header-only red reproduction.
    for case in cases:
        exchange, response, ledger = observations(case, True)
        body = b'{}' if case["profile"] == "notification" else b'{"jsonrpc":"2.0","id":"wrong","error":{"code":-32603,"message":"upstream authorization failed"}}'
        exchange["rawResponse"], _ = capture_sse_auth_response(body, exchange["rawResponse"]["headerPairs"])
        require(bool(inspect_sse_get_auth(case, exchange, None, ledger)[0]), "wrong identity/body counted as red")
    with tempfile.TemporaryDirectory() as directory:
        for variant in ("affected", "candidate"):
            verifier = Compatibility(variant, directory)
            state = {}
            def exchange(port, raw, modern, **options):
                case = next(case for case in cases if case["id"] == verifier.case)
                require(port == compat_fixture.PORTS["sse"] and options["capture_response"], "focused transport changed")
                require(c.loads_typed(raw)["method"] == case["method"] and modern == (case["profile"] != "legacy"), "focused request changed")
                observed, response, ledger = observations(case, variant == "affected")
                verifier.exchanges.append(observed); state["events"] = ledger["events"]
                return case["status"], {}, response
            verifier.exchange = exchange
            verifier.backend = lambda reset=False: {"events": []} if reset else state
            with patch.object(verify, "wait_http", return_value=None):
                require(verifier.run_sse_get_auth_headers() == (1 if variant == "affected" else 0), "focused exit semantics changed")
            snapshot = json.loads(Path(directory, f"sse-get-auth-{variant}.json").read_text())
            require(snapshot["complete"] and snapshot["passed"] == (variant == "candidate") and snapshot["validRedReproduction"] == (variant == "affected"), "focused summary misclassified")
            verifier.exchanges.pop()
            require(not verifier.write_sse_get_auth_headers(cases, None)["complete"], "missing observed exchange accepted")
    with tempfile.TemporaryDirectory() as directory:
        verifier = Compatibility("affected", directory)
        with patch.object(verify, "wait_http", side_effect=RuntimeError("fixture startup failure")):
            require(verifier.run_sse_get_auth_headers() == 1, "startup failure accepted")
        snapshot = json.loads(Path(directory, "sse-get-auth-affected.json").read_text())
        require(snapshot["startupError"] and not snapshot["complete"] and not snapshot["validRedReproduction"], "startup failure counted as red")
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        compat_fixture.sse_get_auth_config(root); first = (root / "envoy.yaml").read_bytes()
        compat_fixture.sse_get_auth_config(root); require(first == (root / "envoy.yaml").read_bytes() and b"/runtime/plugin.wasm" in first, "focused renderer depends on role")


def registry_and_body_checks():
    for role in ("oracle", "control", "candidate"):
        registry = c.registry(role)
        rows = [{"id": case["id"]} for case in registry]
        require(c.complete(registry, rows), "registry incomplete")
        require(not c.complete(registry, rows[:-1]), "missing case accepted")
        require(not c.complete(registry, rows + rows[:1]), "duplicate case accepted")
        require(not c.complete(registry + registry[:1], rows + rows[:1]), "duplicate expected registry accepted")
    for modern in (False, True):
        for location in ("arguments", "meta"):
            for size in (2048, 2049):
                raw, arguments = c.sized_body(modern, size, location)
                require(len(raw) == size and raw.decode("utf-8"), "full UTF-8 body accounting changed")
                json.loads(raw)
                require(len(arguments) < size, "arguments incorrectly used as total body")
    require(dict((name, size) for name, size, _ in c.SIZES)["exact-limit-meta"] == 104857600, "100 MiB full-body limit changed")


def backend_checks():
    original = socket.getfqdn
    socket.getfqdn = lambda _: "localhost"
    try:
        server = backend.ThreadingHTTPServer(("127.0.0.1", 0), backend.Handler)
    finally:
        socket.getfqdn = original
    thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
    compat_backend.reset()
    try:
        for raw, accepted in ((outbound(), True), (outbound().replace(b'"arguments":{}', b'"arguments":{"x":1,"x":2}'), False), (b'{"bad":"\xff"}', False)):
            connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=2)
            connection.putrequest("POST", "/unified/modern")
            for key, value in headers().items(): connection.putheader(key, value)
            connection.putheader("X-Compat-Case", "peer-self-test")
            connection.putheader("Content-Length", str(len(raw)))
            connection.endheaders(raw)
            response = connection.getresponse(); response.read(); connection.close()
            require(response.status == (200 if accepted else 400), "peer HTTP rejection/acceptance differs")
        events = compat_backend.state()["events"]
        require([row["accepted"] for row in events] == [True, False, False], "strict peer ledger lost a rejection")
        require(all(row["kind"] == "business" for row in events), "peer attempt phase lost")
        for method, params in (("initialize", {"protocolVersion": "2025-03-26", "capabilities": {}}), ("tools/list", {}),
                               ("tools/call", {"name": "echo", "arguments": {"value": "native", "fail": False}}),
                               ("tools/call", {"name": "echo", "arguments": {"value": "native", "fail": True}})):
            connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=2)
            peer_headers = {"Content-Type": "application/json", "X-Compat-Nov25": "1", "X-Compat-Case": "nov25-peer"}
            if method != "initialize": peer_headers["Mcp-Session-Id"] = "nov25-upstream-session"
            connection.request("POST", "/unified/legacy", c.nov25_request(method, params), peer_headers)
            response = connection.getresponse(); result = json.loads(response.read())["result"]; connection.close()
            require(response.status == 200, "November peer rejected a supported operation")
            if method == "initialize": require(response.getheader("Mcp-Session-Id") == "nov25-upstream-session", "November peer session missing")
            elif method == "tools/list": require(result["tools"] == [c.NOV25_TOOL], "November peer descriptor changed")
            else: require(result["isError"] is params["arguments"]["fail"] and result["content"][0]["text"] == ("synthetic execution failed" if params["arguments"]["fail"] else "echo:native"), "November peer tool result/error incorrect")
        require([row["sessionAlias"] for row in compat_backend.state()["events"] if row["case"] == "nov25-peer"] == [None, "nov25-upstream", "nov25-upstream", "nov25-upstream"], "November peer ledger session aliases changed")
        connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=2)
        connection.request("GET", "/unified/sse", headers={"X-Compat-Case": "gzip-self-test", "X-Compat-Fail-Stage": "get", "X-Compat-Response": "http401", "X-Compat-Auth-Fixture": "gzip-headers"})
        response = connection.getresponse(); raw = response.read(); pairs = response.getheaders(); connection.close()
        require(response.status == 401 and gzip.decompress(raw) == c.SSE_GET_AUTH_BODY, "backend gzip body is not real compressed JSON")
        captured, decoded = capture_sse_auth_response(raw, pairs)
        require(decoded == c.SSE_GET_AUTH_BODY and captured["decodeError"] is None, "content encoding decoder ignored actual gzip")
        event = compat_backend.state()["events"][-1]
        require(event["case"] == "gzip-self-test" and event["authFixture"]["rawBodyBase64"] == base64.b64encode(raw).decode(), "upstream gzip evidence differs")
    finally:
        server.shutdown(); server.server_close(); thread.join(timeout=2)


def fixture_checks():
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        for name in ("e2bdev", "firecrawl"): (root / f"builtin-{name}.json").write_text('{"server":{"name":"test"}}')
        compat_fixture.configs(root, "candidate")
        text = (root / "envoy.yaml").read_text()
        require("cluster: compat-route" in text and "address: backend-primary" in text, "original custom cluster lost")
        require("host_rewrite_literal: fixed.virtual" in text and "http://initial.virtual/unified/sse" in text, "SSE URL/Host rewrite coverage lost")
        require("registered-candidate.wasm" in text and "plain-candidate.wasm" in text, "fixture/plain paths collapsed")
        require(text.count("vm_id: unified-registered\n") == 1 and text.count("vm_id: unified-registered-body\n") == 1, "large bodies share the counter VM")


def nov25_observations(root):
    """Feed bounded wire/ledger observations through the actual focused assertions."""
    from unittest.mock import patch
    import verify
    snapshots = {}
    for role in ("nov25-baseline", "candidate"):
        verifier = Compatibility(role, root)
        backend_state = {"events": []}
        baseline = role == "nov25-baseline"
        versions = c.NOV25_OLD_VERSIONS if baseline else c.NOV25_VERSIONS
        def exchange(port, raw, modern, **options):
            request = json.loads(raw)
            method, params = request["method"], request["params"]
            case = next(row for row in c.nov25_cases() if row["id"] == verifier.case)
            kind = case["kind"]
            status, response = 200, {"jsonrpc": "2.0", "id": "nov25-client"}
            if kind in ("initialize-header", "initialize-body", "fallback", "modern-body-fallback", "old"):
                response["result"] = {"protocolVersion": "2025-06-18" if kind in ("fallback", "modern-body-fallback") or baseline and kind == "initialize-body" else params["protocolVersion"], "capabilities": {"tools": {}}}
            elif kind == "discovery":
                response["result"] = {"supportedVersions": versions, "capabilities": {"tools": {}}, "resultType": "complete", "ttlMs": c.JSONNumber("0"), "cacheScope": "private", "_meta": {"io.modelcontextprotocol/serverInfo": {"name": "compat-http-legacy", "version": "1"}}}
            elif kind == "tools-workflow":
                if method == "initialize": response["result"] = {"protocolVersion": c.NOV25}
                elif method == "notifications/initialized": status, response = 202, None
                else:
                    if method == "tools/list": response["result"] = {"tools": [c.NOV25_TOOL]}
                    else:
                        failed = params["arguments"]["fail"]
                        response["result"] = {"content": [{"type": "text", "text": "synthetic execution failed" if failed else "echo:nov25"}], "isError": failed}
                    if not baseline:
                        backend_state["events"] += [{"kind": "control" if operation != method else "business", "rpcMethod": operation, "accepted": True,
                            "protocolVersion": None, "initializeVersion": "2025-03-26" if operation == "initialize" else None,
                            "sessionAlias": None if operation == "initialize" else "nov25-upstream", "case": verifier.case}
                            for operation in ("initialize", "notifications/initialized", method)]
            rejection = kind not in ("initialize-header", "initialize-body", "fallback", "modern-body-fallback", "old", "discovery", "tools-workflow") or baseline and kind in ("initialize-header", "tools-workflow")
            if rejection:
                code = -32022 if kind in ("unknown-header", "modern-mismatched-meta") or baseline and kind in ("initialize-header", "tools-workflow", "duplicate-version", "handshake-modern-identity", "handshake-modern-meta") else -32020
                status, response = 400, {"jsonrpc": "2.0", "id": None, "error": {"code": c.JSONNumber(str(code)), "data": {"supported": versions}}}
            count = verifier.exchange_counts.get(verifier.case, 0) + 1
            verifier.exchange_counts[verifier.case] = count
            verifier.exchanges.append({"case": verifier.case, "accessRequestId": f"compat-nov25-{verifier.case}-{count}", "httpStatus": status, "port": port,
                "request": c.body_record(raw), "requestHeadersSha256": hashlib.sha256(json.dumps(options, sort_keys=True).encode()).hexdigest()})
            return status, {}, response
        verifier.exchange = exchange
        def backend(reset=False):
            if reset: backend_state["events"] = []
            return backend_state
        verifier.backend = backend
        with patch.object(verify, "wait_http", return_value=None):
            require(verifier.run_nov25() == 0, "valid November observations rejected: " + role)
        snapshot = json.loads((root / f"nov25-{role}.json").read_text())
        snapshots[role] = snapshot
        (root / f"gateway-nov25-{role}.log").write_text("plugin start successfully\n" + "\n".join(f"access request_id={row['accessRequestId']} listener=nov25 method=POST path=/mcp code={row['httpStatus']} flags=-" for row in snapshot["exchanges"]))
    return snapshots


def nov25_focused_checks():
    from unittest.mock import patch
    import verify
    require(len(c.nov25_cases()) == 17, "November registry changed unexpectedly")
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        snapshots = nov25_observations(root)
        for role in snapshots:
            require(len(snapshots[role]["exchanges"]) == 21, "November traffic incomplete")
        compat_fixture.nov25_config(root); first = (root / "envoy.yaml").read_bytes()
        compat_fixture.nov25_config(root)
        require(first == (root / "envoy.yaml").read_bytes() and b"/runtime/plugin.wasm" in first, "November configuration depends on source role")
        # A positive-looking RPC with the wrong descriptor, ID or tool-error
        # polarity must fail the actual candidate workflow assertion.
        for fault in ("descriptor", "id", "isError", "duplicate", "upstream-version", "session"):
            verifier = Compatibility("candidate", root)
            workflow = next(row for row in c.nov25_cases() if row["kind"] == "tools-workflow")
            verifier.case = workflow["id"]
            responses = [(200, {}, {"jsonrpc": "2.0", "id": "nov25-client", "result": {"protocolVersion": c.NOV25}}), (202, {}, None),
                         (200, {}, {"jsonrpc": "2.0", "id": "nov25-client", "result": {"tools": [copy.deepcopy(c.NOV25_TOOL)]}})]
            for fail in (False, True): responses.append((200, {}, {"jsonrpc": "2.0", "id": "nov25-client", "result": {"content": [{"type": "text", "text": "synthetic execution failed" if fail else "echo:nov25"}], "isError": fail}}))
            if fault == "descriptor": responses[2][2]["result"]["tools"][0]["inputSchema"] = {}
            elif fault == "id": responses[3][2]["id"] = "wrong"
            elif fault == "isError": responses[4][2]["result"]["isError"] = False
            iterator = iter(responses)
            def exchange(*args, **kwargs):
                verifier.exchanges.append({}); return next(iterator)
            verifier.exchange = exchange
            ledger = copy.deepcopy(snapshots["candidate"]["cases"][-1]["detail"]["ledger"])
            if fault == "duplicate":
                ledger["events"].append(copy.deepcopy(ledger["events"][-1])); ledger["attempts"]["business"] += 1
            elif fault == "upstream-version": ledger["events"][0]["initializeVersion"] = c.NOV25
            elif fault == "session": ledger["events"][1]["sessionAlias"] = "downstream"
            verifier.ledger = lambda: ledger
            rejected(lambda: verifier.nov25_case(workflow))


def concurrent_legacy_control(interleaved=False):
    methods = (verify_auto.PROBE, verify_auto.INIT, verify_auto.NOTIFY, verify_auto.CALL)
    order = [("alice", 0), ("bob", 0), ("alice", 1), ("bob", 1), ("alice", 2), ("bob", 2), ("alice", 3), ("bob", 3)]
    if interleaved:
        order = [("alice", 0), ("alice", 1), ("bob", 0), ("alice", 2), ("bob", 1), ("bob", 2), ("bob", 3), ("alice", 3)]
    events = [{"requestKey": key, "authAlias": key, "rpcMethod": methods[phase], "seq": index + 1,
               "sessionPresent": phase >= 2, "sessionMatchesRequest": phase >= 2} for index, (key, phase) in enumerate(order)]
    return {"case": "auto-concurrent-legacy-session-isolation", "status": "PASS", "detail": {
        "upstreamSequence": [event["rpcMethod"] for event in events], "backendEvents": {"backend-primary": events},
        "backendState": {"executions": {"alice": 1, "bob": 1}}}}


def concurrent_control_checks():
    original = concurrent_legacy_control()
    expected = {key: [verify_auto.PROBE, verify_auto.INIT, verify_auto.NOTIFY, verify_auto.CALL] for key in ("alice", "bob")}
    require(evidence.retained_control_sequence(original) == expected, "ordered request projection differs")
    require(evidence.retained_control_sequence(concurrent_legacy_control(True)) == expected, "legal concurrent interleaving rejected")
    for fault in ("missing", "extra", "per-request-order", "session-crossing", "session-missing", "auth-crossing",
                  "request-crossing", "execution-missing", "execution-extra", "early-business", "global-sequence", "event-sequence", "backend"):
        changed = copy.deepcopy(original)
        detail, events = changed["detail"], changed["detail"]["backendEvents"]["backend-primary"]
        if fault == "missing": events.pop()
        elif fault == "extra": events.append(copy.deepcopy(events[-1]))
        elif fault == "per-request-order": events[0], events[2] = events[2], events[0]
        elif fault == "session-crossing": events[4]["sessionMatchesRequest"] = False
        elif fault == "session-missing": events[6]["sessionPresent"] = False
        elif fault == "auth-crossing": events[4]["authAlias"] = "bob"
        elif fault == "request-crossing": events[4]["requestKey"] = "bob"
        elif fault == "execution-missing": detail["backendState"]["executions"].pop("bob")
        elif fault == "execution-extra": detail["backendState"]["executions"]["alice"] = 2
        elif fault == "early-business": events[5], events[6] = events[6], events[5]
        elif fault == "backend": detail["backendEvents"]["backend-secondary"] = []
        # Keep ordinary ledger metadata consistent while fault-injecting the
        # actual request/session/order condition, rather than a stale mirror.
        detail["upstreamSequence"] = [event["rpcMethod"] for event in events]
        for index, event in enumerate(events): event["seq"] = index + 1
        if fault == "global-sequence": detail["upstreamSequence"].reverse()
        elif fault == "event-sequence": events[-1]["seq"] = 1
        rejected(lambda: evidence.retained_control_sequence(changed))
    ordinary = {"case": "auto-success-legacy", "detail": {"upstreamSequence": expected["alice"]}}
    reordered = copy.deepcopy(ordinary); reordered["detail"]["upstreamSequence"][0:2] = list(reversed(expected["alice"][0:2]))
    require(evidence.retained_control_sequence(ordinary) != evidence.retained_control_sequence(reordered), "sequential case ordering was weakened")


def valid_runtime(root):
    def write(name, value): (root / name).write_text(json.dumps(value))
    fixture = root / "harness" / "registered_fixture.go.txt"; fixture.parent.mkdir(); fixture.write_text("same fixture")
    fixture_sha = hashlib.sha256(fixture.read_bytes()).hexdigest()
    inputs = {"candidate": "a" * 40, "businessOracle": c.ORACLE, "controlScopeBaseline": c.CONTROL,
              "sourceStillClean": True, "tokenOverridesUnset": True, "cleanupComplete": True,
              "nov25Baseline": c.NOV25_BASELINE, "nov25Configs": {},
              "registeredFixtureSha256": fixture_sha, "harness": {"registered_fixture.go.txt": fixture_sha},
              "images": {"gateway@sha256:" + "b" * 64: {}}, "versions": {}}
    for role, source in (("oracle", c.ORACLE), ("control", c.CONTROL), ("candidate", "a" * 40)):
        inputs["versions"][role] = {"source": source, "go": "go1.24.4", "moduleFiles": {"go.mod": "c" * 64}, "wasm": {}}
        for flavor in ("plain", "registered"):
            wasm = root / f"{flavor}-{role}.wasm"; wasm.write_bytes((role + flavor).encode())
            inputs["versions"][role]["wasm"][flavor] = hashlib.sha256(wasm.read_bytes()).hexdigest()
        registry, rows, exchanges, log = c.registry(role), [], [], ["plugin start successfully"]
        for case in registry:
            detail = {}
            if case["id"] in {"input/legacy/" + shape for shape in ("invalid", "concatenated", "batch")}:
                detail = {"basicMessageBoundaryException": True} if role == "oracle" else {"zeroCreateCall": True, "boundary": "basic-message"}
            rows.append({"id": case["id"], "acceptance": case["acceptance"], "status": "PASS", "detail": detail})
            for count in range(c.exchange_count(case)):
                request_id = "compat-" + hashlib.sha256(case["id"].encode()).hexdigest() + "-" + str(count)
                exchanges.append({"case": case["id"], "accessRequestId": request_id, "httpStatus": 200})
                log.append("access request_id=" + request_id + " listener=fixture method=POST path=/mcp code=200 flags=-")
        write(f"compat-{role}.json", {"expectedCases": registry, "cases": rows, "exchanges": exchanges})
        if role != "oracle":
            names = [name for name, _ in verify_auto.cases(None)]
            write(f"controls-{role}.json", {"expectedCases": names, "cases": [concurrent_legacy_control(role == "candidate") if name == "auto-concurrent-legacy-session-isolation" else {"case": name, "status": "PASS", "detail": {"upstreamSequence": []}} for name in names]})
            instructions_role = "baseline" if role == "control" else "candidate"
            instructions, access = valid_evidence(instructions_role)
            write(f"instructions-{instructions_role}.json", instructions)
            log.append(access)
        (root / f"gateway-{role}.log").write_text("\n".join(log))
    baseline_wasm = root / "plain-nov25-baseline.wasm"; baseline_wasm.write_bytes(b"November baseline")
    inputs["versions"]["nov25-baseline"] = {**inputs["versions"]["candidate"], "source": c.NOV25_BASELINE, "wasm": {"plain": hashlib.sha256(baseline_wasm.read_bytes()).hexdigest()}}
    nov25_observations(root)
    compat_fixture.nov25_config(root)
    for role in ("nov25-baseline", "candidate"):
        config = root / f"envoy-nov25-{role}.yaml"; config.write_bytes((root / "envoy.yaml").read_bytes())
        inputs["nov25Configs"][role] = hashlib.sha256(config.read_bytes()).hexdigest()
    write("compat-inputs.json", inputs)
    (root / "routing").mkdir(); write("routing/manifest.json", {"passed": True, "cleanup_complete": True})
    return inputs


def evidence_checks():
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory); inputs = valid_runtime(root)
        require(evidence.finalize(root) == 0, "complete focused synthetic evidence rejected")
        controls = root / "controls-candidate.json"; saved_controls = controls.read_text()
        changed = json.loads(saved_controls)
        row = next(row for row in changed["cases"] if row["case"] == "auto-concurrent-legacy-session-isolation")
        row["detail"]["backendEvents"]["backend-primary"][-1]["sessionMatchesRequest"] = False
        controls.write_text(json.dumps(changed)); require(evidence.finalize(root) == 1, "concurrent session crossing accepted by finalization")
        controls.write_text(saved_controls)
        focused = root / "nov25-candidate.json"; saved_focused = focused.read_text()
        for fault in ("missing", "duplicate", "request", "headers", "status"):
            snapshot = json.loads(saved_focused)
            if fault == "missing": snapshot["exchanges"].pop()
            elif fault == "duplicate": snapshot["exchanges"].append(snapshot["exchanges"][0])
            elif fault == "request": snapshot["exchanges"][0]["request"]["sha256"] = "0" * 64
            elif fault == "headers": snapshot["exchanges"][0]["requestHeadersSha256"] = "0" * 64
            else: snapshot["cases"][0]["status"] = "FAIL"
            focused.write_text(json.dumps(snapshot)); require(bool(evidence.nov25_errors(root, inputs)), "November invalid evidence accepted: " + fault)
        focused.write_text(saved_focused)
        path = root / "compat-candidate.json"; original = json.loads(path.read_text())
        for field in ("cases", "exchanges"):
            damaged = {**original, field: original[field][:-1]}; path.write_text(json.dumps(damaged))
            require(evidence.finalize(root) == 1, "missing " + field + " accepted")
            damaged = {**original, field: original[field] + original[field][:1]}; path.write_text(json.dumps(damaged))
            require(evidence.finalize(root) == 1, "duplicate " + field + " accepted")
        path.write_text(json.dumps(original))
        damaged = json.loads(json.dumps(original)); damaged["cases"][0]["status"] = "FAIL"
        path.write_text(json.dumps(damaged)); require(evidence.finalize(root) == 1, "red case accepted")
        damaged = json.loads(json.dumps(original)); damaged["cases"][0]["detail"] = {"unclassifiedBusinessDifference": True}
        path.write_text(json.dumps(damaged)); require(evidence.finalize(root) == 1, "unclassified business difference accepted")
        path.write_text(json.dumps(original))
        inputs["versions"]["candidate"]["wasm"]["plain"] = "0" * 64
        (root / "compat-inputs.json").write_text(json.dumps(inputs))
        require(evidence.finalize(root) == 1, "wrong Wasm hash accepted")
        inputs["versions"]["candidate"]["wasm"]["plain"] = hashlib.sha256((root / "plain-candidate.wasm").read_bytes()).hexdigest()
        (root / "compat-inputs.json").write_text(json.dumps(inputs))
        require(evidence.finalize(root) == 0, "restored evidence rejected")
        (root / "commands.jsonl").write_text('{"finalizer":"completed"}\n')
        (root / "finalize.log").write_text("completed\n")
        require(json.loads((root / "compat-manifest.json").read_text())["checksums"] != evidence.checksums(root), "self-recording mutation was not detected")
        evidence.seal_checksums(root)
        require(json.loads((root / "compat-manifest.json").read_text())["checksums"] == evidence.checksums(root), "final command/log checksum stale")
        (root / "commands.jsonl").write_text("later mutation\n")
        require(json.loads((root / "compat-manifest.json").read_text())["checksums"] != evidence.checksums(root), "post-finalization mutation hidden")



def sse_no_space_focused_checks():
    """Run the focused verifier/finalizer with injected observations, never Wasm."""
    import contextlib
    import subprocess
    import sys
    from types import SimpleNamespace
    from unittest.mock import patch
    import run_compat
    import verify
    cases = compat_fixture.sse_no_space_cases()
    require(len(cases) == 10 and len({row["id"] for row in cases}) == 10 and sum(row["target"] for row in cases) == 5, "focused matrix incomplete")
    # Actual backend selection must preserve default/control JSON responses.
    for method in ("server/discover", "initialize", "notifications/initialized", "tools/list", "tools/call"):
        for stage in (None, "business", "initialize", "all"):
            h = parse_headers(io.BytesIO(b"Content-Type: application/json\r\n\r\n"))
            if stage:
                h["X-Compat-SSE-Format"], h["X-Compat-SSE-Stage"] = "no-space", stage
            handler = SimpleNamespace(headers=h, command="POST", path="/unified/modern")
            compat_backend.reset()
            with patch.object(compat_backend, "raw_response") as send:
                require(compat_backend.post(handler, c.envelope(False, method=method, tool="echo").decode()), "backend request missing")
                event = compat_backend.state()["events"][-1]
                formatted = bool(stage and method != "notifications/initialized" and
                                 (stage == method or stage == "all" and method != "server/discover" or stage == "business" and method.startswith("tools/")))
                require(("sseFixture" in event) == formatted, "focused format poisoned controls/default")
                if formatted: require(send.call_args.args[3] == "text/event-stream", "focused media mismatch")
    for framing in {row["format"] for row in cases}:
        wire = compat_backend.sse_no_space_wire(b'{"result":{}}', framing)
        require(wire.endswith(b"\r\n\r\n") if framing.endswith("crlf") else wire.endswith(b"\n\n"), "SSE wire incomplete")
    rejected(lambda: compat_backend.sse_no_space_wire(b'{}', "unknown"))
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        result = {"tools": [{"name": "echo", "inputSchema": {"type": "object"}}], "content": [], "number": c.JSONNumber("900719925474099312345"), "value": c.JSONNumber("2")}
        def attach(verifier, fault=None):
            state = {}
            def exchange(port, raw, modern, **options):
                case = next(row for row in cases if row["id"] == verifier.case)
                require(c.loads_typed(raw)["params"]["_meta"]["progressToken"] == "sse-progress", "progressToken omitted")
                require(port == compat_fixture.PORTS[case["route"]] and modern == (case["route"] == "auto-modern"), "focused route changed")
                init_red = verifier.variant == "sse-baseline" and case.get("stage") == "initialize"
                methods = ["server/discover", case["method"]] if modern else ["initialize"] if init_red else ["initialize", "notifications/initialized", case["method"]]
                events = []
                for method in methods:
                    event = {"case": verifier.case, "rpcMethod": method, "kind": "probe" if method == "server/discover" else "control" if method in ("initialize", "notifications/initialized") else "business",
                             "accepted": True, "backendOrigin": "primary", "protocolVersion": c.MODERN if modern else "2025-03-26"}
                    stage = case.get("stage", "business")
                    if method != "notifications/initialized" and (stage == "all" or stage == method or stage == "business" and method.startswith("tools/")):
                        wire = compat_backend.sse_no_space_wire(b'{"jsonrpc":"2.0","id":2,"result":{}}', case["format"])
                        event["sseFixture"] = {"format": case["format"], "stage": method, **c.body_record(wire), "rawBase64": base64.b64encode(wire).decode()}
                    events.append(event)
                state["events"] = events
                failed = verifier.variant == "sse-baseline" and case["target"] or case["format"] == "invalid"
                response = {"jsonrpc": "2.0", "id": "sse-client"}
                if failed: response["error"] = {"code": c.JSONNumber("-32603"), "message": "no data field found in SSE response" if verifier.variant == "sse-baseline" else "invalid JSON response"}
                else: response["result"] = copy.deepcopy(result)
                if fault == "dispatch": events.append(copy.deepcopy(events[-1]))
                elif fault == "id": response["id"] = "wrong"
                elif fault == "number" and not failed: response["result"]["number"] = c.JSONNumber("1")
                elif fault == "stage" and failed: response["error"]["message"] = "unrelated failure"
                verifier.exchanges.append({"case": verifier.case, "accessRequestId": "test-" + verifier.case, "request": c.body_record(raw), "requestHeadersSha256": "same", "httpStatus": 200})
                return 200, {"content-type": "application/json"}, response
            verifier.exchange = exchange
            verifier.backend = lambda reset=False: {"events": []} if reset else state
        for role in ("sse-baseline", "candidate"):
            verifier = Compatibility(role, root); attach(verifier)
            with patch.object(verify, "wait_http", return_value=None):
                require(verifier.run_sse_no_space() == (1 if role == "sse-baseline" else 0), "focused exit semantics changed")
            snapshot = json.loads((root / f"sse-no-space-{role}.json").read_text())
            require(c.complete(cases, snapshot["cases"]) and sum(row["validRedReproduction"] for row in snapshot["cases"]) == (5 if role == "sse-baseline" else 0), "focused red/green misclassified: " + repr(snapshot["cases"]))
        for role, fault in (("candidate", "dispatch"), ("candidate", "id"), ("candidate", "number"), ("sse-baseline", "stage")):
            verifier = Compatibility(role, root); attach(verifier, fault); verifier.case = cases[2]["id"]
            rejected(lambda: verifier.sse_no_space_case(cases[2]))
        inputs = {"candidate": "a" * 40, "sseBaseline": compat_fixture.SSE_NO_SPACE_BASELINE, "sourceStillClean": True, "tokenOverridesUnset": True, "cleanupComplete": True,
                  "verifierExits": {"sse-baseline": 1, "candidate": 0}, "versions": {}, "configs": {}, "harness": {}, "images": {"gateway@sha256:" + "b" * 64: {}, "python@sha256:" + "c" * 64: {}}}
        compat_fixture.sse_no_space_config(root); config = (root / "envoy.yaml").read_bytes()
        compat_fixture.sse_no_space_config(root)
        require((root / "envoy.yaml").read_bytes() == config and b"/runtime/plugin.wasm" in config, "role configuration differs")
        for role, revision in (("sse-baseline", compat_fixture.SSE_NO_SPACE_BASELINE), ("candidate", inputs["candidate"])):
            wasm = root / f"plain-{role}.wasm"; wasm.write_bytes(role.encode())
            inputs["versions"][role] = {"source": revision, "go": "same-go", "moduleFiles": {"go.mod": "same"}, "wasm": {"plain": run_compat.sha(wasm)}}
            cfg = root / f"envoy-{role}.yaml"; cfg.write_bytes(config); inputs["configs"][role] = run_compat.sha(cfg)
            snapshot = json.loads((root / f"sse-no-space-{role}.json").read_text())
            (root / f"gateway-{role}.log").write_text("\n".join("access request_id=" + row["accessRequestId"] + " listener=test" for row in snapshot["exchanges"]))
        require(not run_compat.sse_no_space_errors(root, inputs), "valid focused evidence rejected")
        path = root / "sse-no-space-candidate.json"; original = path.read_text()
        for field in ("cases", "exchanges"):
            for duplicate in (False, True):
                changed = json.loads(original); changed[field] = changed[field] + changed[field][:1] if duplicate else changed[field][:-1]
                path.write_text(json.dumps(changed)); require(bool(run_compat.sse_no_space_errors(root, inputs)), "missing/duplicate observation accepted")
        path.write_text(original)
        changed = json.loads(original)
        next(row for row in changed["cases"] if row["id"] == "sse-no-space/mixed")["detail"]["resultSha256"] = "wrong"
        path.write_text(json.dumps(changed))
        require(bool(run_compat.sse_no_space_errors(root, inputs)), "changed control result accepted")
        path.write_text(original)
        for field, value in (("cleanupComplete", False), ("sseBaseline", "wrong"), ("verifierExits", {"sse-baseline": 0, "candidate": 0})):
            require(bool(run_compat.sse_no_space_errors(root, {**inputs, field: value})), "bad focused identity/cleanup accepted")
        changed = copy.deepcopy(inputs); changed["versions"]["sse-baseline"]["go"] = "different"
        require(bool(run_compat.sse_no_space_errors(root, changed)), "inequivalent toolchain accepted")
        verifier = Compatibility("sse-baseline", root)
        with patch.object(verify, "wait_http", side_effect=RuntimeError("startup failure")):
            require(verifier.run_sse_no_space() == 1 and not any(row["validRedReproduction"] for row in verifier.rows), "startup counted as red")
    for flags in (["--sse-no-space"], ["--sse-baseline", compat_fixture.SSE_NO_SPACE_BASELINE], ["--sse-no-space", "--sse-baseline", "wrong"]):
        with patch.object(sys, "argv", ["run_compat.py", "--output", "/unused", *flags]), contextlib.redirect_stderr(io.StringIO()):
            try: run_compat.main()
            except SystemExit as error: require(error.code == 2, "invalid selection accepted")
            else: raise AssertionError("invalid selection accepted")
    # Exercise real main/finally, injecting failure before any containers start.
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory); harness = root / "harness"; harness.mkdir()
        (harness / "registered_fixture.go.txt").write_text("fixture")
        for cleanup_status in (0, 1):
            output = root / str(cleanup_status); commands = []
            def fake_run(command, log, **options):
                commands.append(command)
                if command[:3] == ["podman", "image", "inspect"]: raise RuntimeError("image failure")
                if options.get("target"): Path(options["target"]).write_text("")
                return subprocess.CompletedProcess(command, cleanup_status if "down" in command else 0, stdout="")
            with patch.object(run_compat, "HERE", harness), patch.object(run_compat, "source_is_clean", return_value=True), \
                 patch.object(run_compat.subprocess, "check_output", return_value="a" * 40), patch.object(run_compat, "recorded_run", side_effect=fake_run), \
                 patch.object(sys, "argv", ["run_compat.py", "--output", str(output), "--sse-no-space", "--sse-baseline", compat_fixture.SSE_NO_SPACE_BASELINE]), contextlib.redirect_stdout(io.StringIO()):
                require(run_compat.main() == 1, "focused runner swallowed image failure")
            recorded = json.loads((output / "compat-inputs.json").read_text())
            require(recorded["cleanupComplete"] == (cleanup_status == 0), "cleanup failure hidden")
            require(any(command[:3] == ["podman", "network", "ls"] for command in commands) and any(command[:3] == ["podman", "volume", "ls"] for command in commands), "cleanup inventories missing")

def main():
    sse_no_space_focused_checks(); strict_peer_checks(); concurrent_control_checks(); nov25_focused_checks(); sse_get_auth_focused_checks(); sse_correlation_checks(); cleanup_failure_checks(); counter_and_legacy_diagnostic_checks(); registry_and_body_checks(); backend_checks(); fixture_checks(); evidence_checks()
    print("unified compatibility fixture/oracle self-tests passed")


if __name__ == "__main__":
    main()
