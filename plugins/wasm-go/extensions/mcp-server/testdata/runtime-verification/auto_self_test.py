#!/usr/bin/env python3
"""Exercise the auto fixture's real HTTP barriers and irreversible execution ledger.

This checks the harness, not Wasm cancellation. The Envoy matrix observes the
additional downstream/upstream lifecycle gauges before accepting cancellation.
"""

import concurrent.futures
import http.client
import json
import threading
import unittest
from types import SimpleNamespace
from http.server import ThreadingHTTPServer

import auto_backend
import backend
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


if __name__ == "__main__":
    unittest.main()
