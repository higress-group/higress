#!/usr/bin/env python3
"""Exercise the auto fixture's real HTTP barriers and irreversible execution ledger.

This checks the harness, not Wasm cancellation. The Envoy matrix observes the
additional downstream/upstream lifecycle gauges before accepting cancellation.
"""

import concurrent.futures
import copy
import http.client
import json
import os
import re
import tempfile
import threading
import unittest
from types import SimpleNamespace
from pathlib import Path
from unittest.mock import patch
from http.server import ThreadingHTTPServer

import auto_backend
import backend
import generate_envoy
import verify
import verify_auto


class AutoFixtureTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), backend.Handler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join(5)

    def request(self, path, value=None, headers=None):
        connection = http.client.HTTPConnection("127.0.0.1", self.server.server_port, timeout=5)
        try:
            connection.request("POST" if value is not None else "GET", path,
                               json.dumps(value).encode() if value is not None else None,
                               {"Content-Type": "application/json", **(headers or {})})
            response = connection.getresponse()
            return response.status, dict(response.getheaders()), response.read()
        finally:
            connection.close()

    def configure(self, mode, barrier=None):
        self.request("/__reset", {})
        self.request("/__auto_config", {"mode": mode, "case": self.id(), "barrier": barrier})

    def rpc(self, method, key="test", headers=None):
        return self.request("/auto/mcp", {"jsonrpc": "2.0", "id": key, "method": method, "params": {}},
                            {"baggage": key, **(headers or {})})

    def state(self):
        return json.loads(self.request("/__state")[2])

    def test_required_cases_are_distinct_and_complete(self):
        names = [name for name, _ in verify_auto.cases(SimpleNamespace())]
        self.assertEqual(len(names), len(set(names)))
        for mode in verify_auto.FAILURES:
            self.assertIn("auto-failure-" + mode, names)
        for stage in (verify_auto.PROBE, verify_auto.INIT, verify_auto.NOTIFY, verify_auto.CALL):
            self.assertIn("auto-cancel-" + stage.replace("/", "-"), names)
        self.assertIn("auto-execution-loss-no-replay", names)
        self.assertIn("auto-fresh-and-concurrent", names)
        self.assertIn("auto-concurrent-legacy-session-isolation", names)

    def test_session_oracle_rejects_swapped_sessions_and_replayed_business(self):
        for fault in (None, verify_auto.NOTIFY, verify_auto.CALL, "replay"):
            with self.subTest(fault=fault):
                self.configure("legacy-session", verify_auto.NOTIFY)
                sessions = {}
                for key in ("alice", "bob"):
                    self.rpc(verify_auto.PROBE, key, {"Authorization": "Bearer auto-" + key})
                    status, headers, _ = self.rpc(verify_auto.INIT, key, {"Authorization": "Bearer auto-" + key})
                    self.assertEqual(status, 200)
                    sessions[key] = headers["Mcp-Session-Id"]
                self.assertNotEqual(sessions["alice"], sessions["bob"])

                def finish(key):
                    for stage in (verify_auto.NOTIFY, verify_auto.CALL):
                        owner = ("bob" if key == "alice" else "alice") if fault == stage else key
                        status, _, _ = self.rpc(stage, key, {"Authorization": "Bearer auto-" + key,
                                                            "Mcp-Session-Id": sessions[owner]})
                        self.assertEqual(status, 202 if stage == verify_auto.NOTIFY else 200)

                with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
                    futures = [pool.submit(finish, key) for key in ("alice", "bob")]
                    try:
                        blocked = verify_auto.wait_for(self.state,
                            lambda value: value["auto"]["waitingRequests"].get(verify_auto.NOTIFY) == ["alice", "bob"],
                            "both legacy sessions blocked")
                        self.assertEqual(blocked["auto"]["executions"], {})
                        self.assertTrue(all(not future.done() for future in futures))
                        self.request("/__auto_release", {"stage": verify_auto.NOTIFY})
                        for future in futures:
                            future.result()
                    finally:
                        self.request("/__auto_release", {"stage": verify_auto.NOTIFY})
                if fault == "replay":
                    self.rpc(verify_auto.CALL, "alice", {"Authorization": "Bearer auto-alice",
                                                       "Mcp-Session-Id": sessions["alice"]})
                state = self.state()
                for session in sessions.values():
                    self.assertNotIn(session, json.dumps(state))
                if fault is None:
                    verify_auto.check_legacy_sessions(state, {"alice": "alice", "bob": "bob"})
                else:
                    with self.assertRaisesRegex(AssertionError, "session does not belong|repeated a phase"):
                        verify_auto.check_legacy_sessions(state, {"alice": "alice", "bob": "bob"})

    def test_phase_release_is_observable_before_reconfiguration(self):
        for stage in (verify_auto.PROBE, verify_auto.INIT, verify_auto.NOTIFY, verify_auto.CALL):
            with self.subTest(stage=stage):
                self.configure("modern", stage)
                with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
                    future = pool.submit(self.rpc, stage)
                    try:
                        verify_auto.wait_for(self.state, lambda value: stage in value["auto"]["waiting"], "fixture barrier")
                        state = self.state()["auto"]
                        self.assertFalse(future.done())
                        self.assertNotIn(stage, state["released"])
                        self.assertNotIn(stage, state["returned"])
                        self.request("/__auto_release", {"stage": stage})
                        self.assertIn(future.result()[0], (200, 202))
                        verify_auto.wait_for(self.state, lambda value: value["auto"]["returned"].get(stage), "response completion")
                        self.assertTrue(self.state()["auto"]["released"][stage])
                    finally:
                        self.request("/__auto_release", {"stage": stage})

    def test_execution_precedes_connection_loss(self):
        self.configure("response-loss")
        with self.assertRaises(http.client.RemoteDisconnected):
            self.rpc(verify_auto.CALL, "executed")
        state = verify_auto.wait_for(self.state, lambda value: value["auto"]["returned"].get(verify_auto.CALL), "closed response")
        self.assertEqual(state["auto"]["executions"], {"executed": 1})
        self.assertEqual([event["rpcMethod"] for event in state["events"]], [verify_auto.CALL])

    def test_capability_and_identity_change_between_requests(self):
        for mode, credential, expected in (("modern", "auto-alice", "alice"), ("legacy", "auto-bob", "bob")):
            self.configure(mode)
            status, _, body = self.rpc(verify_auto.PROBE, headers={"Authorization": "Bearer " + credential})
            self.assertEqual(status, 200)
            response = json.loads(body)
            self.assertIn("result" if mode == "modern" else "error", response)
            self.assertEqual(self.state()["events"][0]["authAlias"], expected)
            self.assertNotIn(credential, json.dumps(self.state()))

    def test_failure_fixtures_keep_discriminating_evidence(self):
        for mode, expected_status in (("modern-ordinary-error", 400), ("trailing-json", 400), ("batch", 400),
                                      ("sse-duplicate", 200), ("sse-missing", 200), ("oversize", 200)):
            with self.subTest(mode=mode):
                self.configure(mode)
                status, headers, body = self.rpc(verify_auto.PROBE)
                self.assertEqual(status, expected_status)
                if mode == "modern-ordinary-error":
                    self.assertEqual(headers["MCP-Protocol-Version"], auto_backend.MODERN)
                elif mode == "oversize":
                    self.assertGreater(len(body), 1 << 20)
                elif mode == "batch":
                    self.assertIsInstance(json.loads(body), list)
                elif mode == "trailing-json":
                    with self.assertRaises(json.JSONDecodeError):
                        json.loads(body)
                else:
                    self.assertEqual(headers["Content-Type"], "text/event-stream")
                    self.assertEqual(body.count(b'"result":'), 2 if mode == "sse-duplicate" else 0)
                self.assertEqual(len(self.state()["events"]), 1)



class ExplicitCompatibilityTest(unittest.TestCase):
    def test_modern_proxy_preserves_ordinary_headers_and_stateless_boundaries(self):
        events = [{
            "rpcMethod": method, "mcpMethod": method, "protocolVersion": verify.MODERN,
            "cookiePresent": True, "unrelatedCredentialPresent": True,
            "authorizationPresent": False, "sessionPresent": False,
            "lastEventIDPresent": False, "internalRoutePresent": False,
            "futureParam": None if method == "tools/list" else "opaque-current-call",
        } for method in ("tools/list", "tools/call")]
        verify.check_modern_proxy_events(events)
        for index in (0, 1):
            for key, bad_value in (
                ("cookiePresent", False), ("unrelatedCredentialPresent", False),
                ("authorizationPresent", True), ("sessionPresent", True),
                ("lastEventIDPresent", True), ("internalRoutePresent", True),
                ("protocolVersion", "wrong"), ("mcpMethod", "initialize"),
                ("futureParam", "wrong-scope"),
            ):
                with self.subTest(index=index, field=key):
                    corrupt = copy.deepcopy(events)
                    corrupt[index][key] = bad_value
                    with self.assertRaises(AssertionError):
                        verify.check_modern_proxy_events(corrupt)
        for corrupt in (events[:1], events + [events[1]], list(reversed(events))):
            with self.subTest(sequence=corrupt):
                with self.assertRaises(AssertionError):
                    verify.check_modern_proxy_events(corrupt)

    def test_explicit_differential_rejects_contract_and_access_corruption(self):
        cases = [{"case": name, "status": "PASS", "detail": {
            "backendEvents": {"backend-primary": [{"rpcMethod": "tools/call", "seq": 1}]},
        }} for name in sorted(verify_auto.EXPLICIT_CASE_NAMES)]
        exchanges = [{"case": row["case"], "accessRequestId": f"request-{index}",
                      "request": {"rpcMethod": "tools/call", "rpcId": index},
                      "response": {"status": 200, "headers": {"content-type": "application/json"},
                                   "body": {"result": {"content": ["expected"]}}, "bodySha256": "expected-hash"}}
                     for index, row in enumerate(cases)]
        baseline = {"cases": cases, "exchanges": exchanges}
        log = "".join(f"access request_id={row['accessRequestId']} listener=fixture code=200\n" for row in exchanges)
        self.assertTrue(verify_auto.explicit_baseline_matches(cases, exchanges, baseline, log))
        for fault in ("case-failed", "case-missing", "case-duplicate", "backend-sequence", "request", "response",
                      "response-header", "response-status", "response-hash", "exchange-missing", "request-id"):
            with self.subTest(fault=fault):
                changed_cases, changed_exchanges = copy.deepcopy(cases), copy.deepcopy(exchanges)
                if fault == "case-failed": changed_cases[0]["status"] = "FAIL"
                elif fault == "case-missing": changed_cases.pop()
                elif fault == "case-duplicate": changed_cases.append(copy.deepcopy(changed_cases[0]))
                elif fault == "backend-sequence": changed_cases[0]["detail"]["backendEvents"]["backend-primary"].append({"rpcMethod": "initialize"})
                elif fault == "request": changed_exchanges[0]["request"]["rpcMethod"] = "initialize"
                elif fault == "response": changed_exchanges[0]["response"]["body"]["result"]["content"] = ["corrupt"]
                elif fault == "response-header": changed_exchanges[0]["response"]["headers"]["content-type"] = "text/plain"
                elif fault == "response-status": changed_exchanges[0]["response"]["status"] = 500
                elif fault == "response-hash": changed_exchanges[0]["response"]["bodySha256"] = "wrong-hash"
                elif fault == "exchange-missing": changed_exchanges.pop()
                elif fault == "request-id": changed_exchanges[0]["accessRequestId"] = changed_exchanges[1]["accessRequestId"]
                self.assertFalse(verify_auto.explicit_baseline_matches(changed_cases, changed_exchanges, baseline, log))
        lines = log.splitlines(keepends=True)
        for corrupt_log in ("".join(lines[1:]), log + lines[0], log + "access request_id=unexpected listener=fixture code=200\n"):
            with self.subTest(access=corrupt_log):
                self.assertFalse(verify_auto.explicit_baseline_matches(cases, exchanges, baseline, corrupt_log))

    def test_auto_rejection_and_explicit_compatibility_use_separate_pinned_modules(self):
        here = Path(__file__).parent
        runner = (here / "run.sh").read_text()
        revisions = {name: re.search(rf"^{name}=([0-9a-f]{{40}})$", runner, re.MULTILINE).group(1)
                     for name in ("AUTO_BASELINE_SHA", "EXPLICIT_BASELINE_SHA")}
        self.assertEqual(revisions["AUTO_BASELINE_SHA"], "dc0999326b1a8df4269c13d0cf6ee2b725259f6e")
        self.assertEqual(revisions["EXPLICIT_BASELINE_SHA"], "36f2f1ac5c7305c5a46c296c617b45d1fbacb7d3")
        self.assertNotEqual(revisions["AUTO_BASELINE_SHA"], revisions["EXPLICIT_BASELINE_SHA"])
        with tempfile.TemporaryDirectory() as directory, patch.dict(os.environ, {"RUNTIME_VERIFY_INSTRUCTIONS": "0"}):
            with patch.object(generate_envoy, "OUT", Path(directory)):
                generate_envoy.main()
            rejection = (Path(directory) / "envoy-auto-baseline.yaml").read_text()
            compatibility = (Path(directory) / "envoy-auto-explicit-baseline.yaml").read_text()
            self.assertIn("/runtime/auto-baseline-plugin.wasm", rejection)
            self.assertNotIn("explicit-baseline-plugin.wasm", rejection)
            self.assertIn("/runtime/explicit-baseline-plugin.wasm", compatibility)
            self.assertNotIn("auto-baseline-plugin.wasm", compatibility)
            self.assertIn('"protocolStrategy":"auto"', rejection)
            self.assertNotIn('"protocolStrategy":"auto"', compatibility)


if __name__ == "__main__":
    unittest.main()
