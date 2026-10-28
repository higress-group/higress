#!/usr/bin/env python3
"""Focused fault-injection checks for runtime lifecycle polling and fixtures."""

import io
import json
import os
import subprocess
import sys
import socket
import tempfile
import threading
import urllib.request
from http.client import parse_headers
from pathlib import Path

import backend
import generate_envoy
from instructions_self_test import valid_evidence
import verify


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def check_transient_lds_publish_timeout():
    attempts = iter((socket.timeout("transient"), socket.timeout("transient"), None))
    original = verify.exchange

    def fake_exchange(_url, **_kwargs):
        outcome = next(attempts)
        if outcome is not None:
            raise outcome
        return 200, {}, {"configs": [{"version_info": "candidate-fixture"}]}

    verify.exchange = fake_exchange
    try:
        verify.wait_lds_version("candidate-fixture")
    finally:
        verify.exchange = original


def check_rejection_poll_classification():
    original = verify.read_lds_rejected_count
    attempts = iter((socket.timeout("transient"), 4))

    def transient_then_rejected(_admin_port):
        outcome = next(attempts)
        if isinstance(outcome, Exception):
            raise outcome
        return outcome

    verify.read_lds_rejected_count = transient_then_rejected
    try:
        require(verify.wait_lds_rejected_count(greater_than=3) == 4,
                "transient rejection polling did not recover")
    finally:
        verify.read_lds_rejected_count = original

    verify.read_lds_rejected_count = lambda _admin_port: (_ for _ in ()).throw(socket.timeout("down"))
    try:
        try:
            verify.wait_lds_rejected_count(timeout=0.01)
        except RuntimeError as exc:
            require("checker remained unavailable" in str(exc),
                    f"wrong permanent-checker diagnostic: {exc}")
        else:
            raise AssertionError("permanent admin failure was accepted as an LDS rejection")
    finally:
        verify.read_lds_rejected_count = original

    verify.read_lds_rejected_count = lambda _admin_port: 3
    try:
        try:
            verify.wait_lds_rejected_count(greater_than=3, timeout=0.01)
        except RuntimeError as exc:
            require("did not report an LDS rejection" in str(exc),
                    f"wrong no-rejection diagnostic: {exc}")
        else:
            raise AssertionError("published stats without a rejection increment were accepted")
    finally:
        verify.read_lds_rejected_count = original


def check_corpus_get_fixture():
    with backend.LOCK:
        backend.EVENTS = []
        backend.SEQ = 0
    # BaseHTTPServer performs a host-name lookup during bind; isolate this unit
    # check from developer-machine DNS state.
    original_getfqdn = socket.getfqdn
    socket.getfqdn = lambda _name: "localhost"
    try:
        server = backend.ThreadingHTTPServer(("127.0.0.1", 0), backend.Handler)
    finally:
        socket.getfqdn = original_getfqdn
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with urllib.request.urlopen(
            f"http://127.0.0.1:{server.server_port}/corpus/valid", timeout=2,
        ) as response:
            require(response.status == 200, f"corpus GET returned {response.status}")
            json.load(response)
        with backend.LOCK:
            events = list(backend.EVENTS)
        require(len(events) == 1, f"corpus GET recorded {len(events)} events")
        require(events[0]["httpMethod"] == "GET", f"wrong corpus method: {events[0]}")
        require(events[0]["path"] == "/corpus/valid", f"wrong corpus path: {events[0]}")
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)


def check_partial_corpus_diagnostic():
    original = (
        verify.EVIDENCE, verify.CORPUS_REVISION, verify.CORPUS_PARTIAL_RECORDS,
        verify.CORPUS_CURRENT_RECORD, verify.evidence_snapshot,
    )
    with tempfile.TemporaryDirectory() as directory:
        current = {"fixture": "fault-injected", "actualAcceptance": None}
        verify.EVIDENCE = Path(directory)
        verify.CORPUS_REVISION = "candidate"
        verify.CORPUS_PARTIAL_RECORDS = [current]
        verify.CORPUS_CURRENT_RECORD = current
        verify.evidence_snapshot = lambda: {"backend-primary": []}
        verify.record_corpus_failure(RuntimeError("admin checker fault"))
        observed = json.loads((Path(directory) / "corpus-candidate.json").read_text())
        require(observed["fixtures"][0]["diagnosticError"] == "admin checker fault",
                f"partial diagnostic missing: {observed}")
        require(observed["fixtures"][0]["backendEvents"] == {"backend-primary": []},
                f"partial backend snapshot missing: {observed}")
    (
        verify.EVIDENCE, verify.CORPUS_REVISION, verify.CORPUS_PARTIAL_RECORDS,
        verify.CORPUS_CURRENT_RECORD, verify.evidence_snapshot,
    ) = original



def check_instructions_disconnect_request():
    head, separator, body = verify.instructions_disconnect_request().partition(b"\r\n\r\n")
    require(separator, "disconnect request lacks the HTTP header/body delimiter")
    request_line, _, header_lines = head.partition(b"\r\n")
    require(request_line == b"POST /mcp HTTP/1.1", "disconnect request does not enter the MCP endpoint")
    headers = parse_headers(io.BytesIO(header_lines + b"\r\n\r\n"))
    # Missing Accept caused real Envoy to return 406 before reaching RouteCall.
    # Guard protocol negotiation rather than relaxing the backend assertion.
    require({"application/json", "text/event-stream"}.issubset(
        {value.strip() for value in headers.get("Accept", "").split(",")}),
        "disconnect request cannot negotiate the normal modern media types")
    require(headers.get("Content-Type") == "application/json" and
            headers.get("MCP-Protocol-Version") == verify.MODERN and
            headers.get("Mcp-Method") == "server/discover", "disconnect request fails modern protocol admission")
    require(headers.get("Origin") == "http://" + headers.get("Host", ""), "disconnect origin does not match the fixture host")
    require(int(headers["Content-Length"]) == len(body), "disconnect body framing is incomplete")
    request = json.loads(body)
    require(request["id"] == "disconnect" and request["method"] == "server/discover" and
            request["params"]["_meta"] == verify.modern_meta(), "disconnect request changed the validated envelope")


def check_instructions_fixture():
    with tempfile.TemporaryDirectory() as directory:
        original_out = generate_envoy.OUT
        generate_envoy.OUT = Path(directory)
        try:
            generate_envoy.instructions_configs()
        finally:
            generate_envoy.OUT = original_out
        baseline = (Path(directory) / "envoy-instructions-baseline.yaml").read_text()
        candidate = (Path(directory) / "envoy-instructions-candidate.yaml").read_text()
        require("passthroughInstructions" not in baseline, "baseline depends on new flag support")
        require(candidate.count('"passthroughInstructions":true') == 8 and
                candidate.count('"passthroughInstructions":false') == 1, "focused flag coverage changed")
        for marker in ("retry_policy: {retry_on: 5xx, num_retries: 1}", "timeout: 0.2s", "envoy.filters.http.lua"):
            require(marker in candidate, f"missing main-route fixture: {marker}")
    original_getfqdn = socket.getfqdn
    socket.getfqdn = lambda _name: "localhost"
    try:
        server = backend.ThreadingHTTPServer(("127.0.0.1", 0), backend.Handler)
    finally:
        socket.getfqdn = original_getfqdn
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        request_body = {"jsonrpc": "2.0", "id": "original-id", "method": "server/discover", "params": {"_meta": verify.modern_meta()}}
        for mode in ("normal", "limit", "oversize", "missing", "rpc-error", "incomplete", "sse"):
            request = urllib.request.Request(f"http://127.0.0.1:{server.server_port}/instructions",
                                             data=json.dumps(request_body).encode(),
                                             headers={"X-Instructions-Mode": mode, "Content-Type": "application/json"})
            with urllib.request.urlopen(request, timeout=2) as response:
                raw = response.read()
                if mode == "sse":
                    require(response.headers["Content-Type"] == "text/event-stream", "SSE fixture lost its content type")
                    raw = raw.removeprefix(b"data: ").strip()
                value = json.loads(raw)
            require(value["id"] == "original-id", "fixture rewrote original RPC ID")
            result = value["result"]
            require(result["ttlMs"] == 0.5, "unrelated-schema compatibility control disappeared")
            if mode in ("limit", "oversize"):
                require(len(result["instructions"].encode()) == (32768 if mode == "limit" else 32769), "decoded byte boundary changed")
            if mode == "missing":
                require("instructions" not in result, "missing-field control includes instructions")
            if mode == "rpc-error":
                require("error" in value and "instructions" in result, "error/result exclusion control lost")
            if mode == "incomplete":
                require(result["resultType"] == "input_required", "incomplete result control lost")
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)



def check_instructions_evidence_rejects_incomplete_runs():
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        environment = dict(os.environ, RUNTIME_EVIDENCE=directory, RUNTIME_VERIFY_INSTRUCTIONS="1",
                           SOURCE_SHA="1" * 40, SOURCE_TREE_CLEAN="true", INSTRUCTIONS_BASELINE_SHA="2" * 40,
                           PLUGIN_SHA256="3" * 64, INSTRUCTIONS_BASELINE_PLUGIN_SHA256="4" * 64)
        snapshots = {}
        for variant in ("baseline", "candidate"):
            snapshots[variant], log = valid_evidence(variant)
            (root / f"instructions-{variant}.json").write_text(json.dumps(snapshots[variant]))
            (root / f"gateway-instructions-{variant}.log").write_text(log)
        (root / "cleanup-proof.txt").write_text("PASS no containers remain")
        def finalize():
            result = subprocess.run([sys.executable, str(Path(__file__).with_name("finalize_evidence.py"))],
                                    env=environment, capture_output=True, text=True)
            manifest = json.loads((root / "instructions-manifest.json").read_text())
            require(manifest["historicalFullMatrixVerified"] is False, "focused evidence certified full regressions")
            return result.returncode, manifest["passed"]
        require(finalize() == (0, True), "complete focused evidence was rejected")
        access_path = root / "gateway-instructions-candidate.log"
        single_access = access_path.read_text()
        access_path.write_text(single_access * 2)
        require(finalize() == (1, False), "reused request correlation accepted despite repeated access records")
        access_path.write_text(single_access)
        candidate = snapshots["candidate"]
        candidate["cases"].pop()
        (root / "instructions-candidate.json").write_text(json.dumps(candidate))
        require(finalize() == (1, False), "missing focused case accepted")
        (root / "instructions-candidate.json").write_text(json.dumps(snapshots["baseline"]))
        require(finalize() == (1, False), "baseline-only evidence accepted as candidate matrix")
        (root / "instructions-candidate.json").write_text(json.dumps({**candidate, "cases": candidate["cases"] + [{"case": "disconnect-late-callback", "status": "PASS"}]}))
        (root / "gateway-instructions-candidate.log").write_text("")
        require(finalize() == (1, False), "missing route access evidence accepted")


if __name__ == "__main__":
    check_transient_lds_publish_timeout()
    check_rejection_poll_classification()
    check_corpus_get_fixture()
    check_partial_corpus_diagnostic()
    check_instructions_disconnect_request()
    check_instructions_fixture()
    check_instructions_evidence_rejects_incomplete_runs()
    print("runtime orchestration fault-injection self-tests passed")
