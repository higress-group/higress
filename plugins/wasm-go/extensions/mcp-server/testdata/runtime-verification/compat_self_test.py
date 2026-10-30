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


def valid_runtime(root):
    def write(name, value): (root / name).write_text(json.dumps(value))
    fixture = root / "harness" / "registered_fixture.go.txt"; fixture.parent.mkdir(); fixture.write_text("same fixture")
    fixture_sha = hashlib.sha256(fixture.read_bytes()).hexdigest()
    inputs = {"candidate": "a" * 40, "businessOracle": c.ORACLE, "controlScopeBaseline": c.CONTROL,
              "sourceStillClean": True, "tokenOverridesUnset": True, "cleanupComplete": True,
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
            write(f"controls-{role}.json", {"expectedCases": names, "cases": [{"case": name, "status": "PASS", "detail": {"upstreamSequence": []}} for name in names]})
            instructions_role = "baseline" if role == "control" else "candidate"
            instructions, access = valid_evidence(instructions_role)
            write(f"instructions-{instructions_role}.json", instructions)
            log.append(access)
        (root / f"gateway-{role}.log").write_text("\n".join(log))
    write("compat-inputs.json", inputs)
    (root / "routing").mkdir(); write("routing/manifest.json", {"passed": True, "cleanup_complete": True})
    return inputs


def evidence_checks():
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory); inputs = valid_runtime(root)
        require(evidence.finalize(root) == 0, "complete focused synthetic evidence rejected")
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


def main():
    strict_peer_checks(); sse_get_auth_focused_checks(); sse_correlation_checks(); cleanup_failure_checks(); counter_and_legacy_diagnostic_checks(); registry_and_body_checks(); backend_checks(); fixture_checks(); evidence_checks()
    print("unified compatibility fixture/oracle self-tests passed")


if __name__ == "__main__":
    main()
