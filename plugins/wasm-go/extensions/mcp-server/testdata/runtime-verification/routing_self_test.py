#!/usr/bin/env python3
# Copyright (c) 2026 Alibaba Group Holding Ltd.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software distributed
# under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
# CONDITIONS OF ANY KIND, either express or implied.
"""Fast CI checks for routing evidence oracles; no containers or Go builds."""
import copy
import io
import json
from pathlib import Path
import sys
import subprocess
import tempfile
import unittest

import routing_fixture
import run_routing
import verify_routing


class RoutingEvidenceTest(unittest.TestCase):
    def test_verifier_requires_nonempty_boolean_green_rows_and_zero_exit(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "assertions.json"
            path.write_text(json.dumps([{"assertion": "real-check", "pass": True}]))
            self.assertEqual(1, len(run_routing.validate_verification(0, path)))
            # A crash after writing apparently green rows is still a failure.
            with self.assertRaises(RuntimeError): run_routing.validate_verification(7, path)
            for rows in ([], {}, [{"assertion": "red", "pass": False}], [{"assertion": "bad", "pass": "true"}], [{"pass": True}], [None]):
                with self.subTest(rows=rows):
                    path.write_text(json.dumps(rows))
                    with self.assertRaises(RuntimeError): run_routing.validate_verification(0, path)
            path.write_text("truncated JSON")
            with self.assertRaises(ValueError): run_routing.validate_verification(0, path)
            path.unlink()
            with self.assertRaises(FileNotFoundError): run_routing.validate_verification(0, path)

    def test_subprocess_failure_is_recorded_and_cannot_be_silently_accepted(self):
        log = io.StringIO()
        command = [sys.executable, "-c", "print('synthetic-runner-failure'); raise SystemExit(7)"]
        with self.assertRaisesRegex(RuntimeError, "synthetic-runner-failure"):
            run_routing.recorded_run(command, log)
        self.assertEqual(7, json.loads(log.getvalue())["exit_code"])
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "verify.log"
            result = run_routing.recorded_run(command, io.StringIO(), allow_failure=True, target=output)
            self.assertEqual(7, result.returncode)
            self.assertIn("synthetic-runner-failure", output.read_text())
            with self.assertRaises(RuntimeError): run_routing.validate_verification(result.returncode, output)

    def test_json_capture_preserves_provider_banner_and_failure_status(self):
        # Match the Compose capture path: machine-readable stdout and an ANSI
        # external-provider notice on stderr must remain separate artifacts.
        names = {"gateway", "gateway-auto", "gateway-auto-baseline", "gateway-auto-explicit-baseline",
                 "gateway-baseline", "gateway-oracle", "gateway-generation"}
        names.update("gateway-instructions-" + role for role in ("baseline", "candidate"))
        names.update("gateway-" + kind + "-" + role for kind in ("control", "corpus") for role in ("candidate", "affected", "oracle"))
        config = {"services": {name: {"entrypoint": ["/usr/local/bin/envoy"], "command": ["--concurrency", "1"]} for name in names}}
        banner = "\x1b[4m>>>> Executing external compose provider \"fixture-provider\". <<<<\x1b[0m\n"
        script = "import sys; sys.stderr.write(sys.argv[1]); print(sys.argv[2]); raise SystemExit(int(sys.argv[3]))"
        with tempfile.TemporaryDirectory() as temporary:
            output, diagnostics = Path(temporary) / "compose.json", Path(temporary) / "compose.stderr.log"
            for status in (0, 7):
                log = io.StringIO()
                command = [sys.executable, "-c", script, banner, json.dumps(config), str(status)]
                if status:
                    with self.assertRaisesRegex(RuntimeError, "fixture-provider"):
                        run_routing.recorded_run(command, log, target=output, stderr_target=diagnostics)
                else:
                    result = run_routing.recorded_run(command, log, target=output, stderr_target=diagnostics)
                    self.assertEqual(0, result.returncode)
                    self.assertEqual(banner, result.stderr)
                    checked = subprocess.run([sys.executable, str(run_routing.HERE / "compose_config_self_test.py"), str(output)], capture_output=True, text=True)
                    self.assertEqual(0, checked.returncode, checked.stdout + checked.stderr)
                self.assertEqual(config, json.loads(output.read_text()), "provider diagnostics corrupted config JSON")
                self.assertEqual(banner, diagnostics.read_text(), "provider diagnostics were discarded")
                record = json.loads(log.getvalue())
                self.assertEqual(status, record["exit_code"], "failed provider exit was lost")
                self.assertEqual(str(diagnostics), record["stderr_file"])

    def test_runner_startup_failure_retains_manifest_and_nonzero_exit(self):
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "evidence"
            result = subprocess.run([sys.executable, str(run_routing.HERE / "run_routing.py"),
                                     "--oracle", "HEAD", "--affected", "HEAD", "--candidate", "HEAD",
                                     "--gateway-image", "synthetic-image", "--engine", str(Path(temporary) / "missing-engine"),
                                     "--output", str(output)], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            self.assertNotEqual(0, result.returncode, result.stdout)
            manifest = json.loads((output / "manifest.json").read_text())
            self.assertFalse(manifest["passed"])
            self.assertFalse(manifest["cleanup_complete"], "failed inventory is not evidence of cleanup")
            self.assertIn("error", manifest)
            commands = [json.loads(line) for line in (output / "commands.jsonl").read_text().splitlines()]
            self.assertTrue(commands)
            self.assertTrue(all(row["exit_code"] == 127 for row in commands))

    def test_logging_oracle_requires_both_request_and_async_response_payloads(self):
        payload = "route call start\n" + "\n".join(run_routing.LOG_MARKERS) + "\nroute call end"
        self.assertTrue(all(row["pass"] for row in run_routing.log_assertions("ordinary INFO", payload, True)))
        self.assertTrue(all(row["pass"] for row in run_routing.log_assertions(payload, payload, False)))
        for info, debug, safe in ((payload, payload, True), ("", "", True), ("", payload, False),
                                  ("", payload.replace("synthetic-result-marker", ""), True),
                                  ("", payload.replace("route call end", ""), True)):
            with self.subTest(safe=safe, info=info, debug=debug):
                self.assertFalse(all(row["pass"] for row in run_routing.log_assertions(info, debug, safe)))

    def test_route_timeout_oracle_rejects_fast_failures_and_slow_successes(self):
        explicit = {"status": 200, "elapsed": 2.05, "json": {"error": {"code": -32603, "message": "backend tools/call failed"}}}
        auto = {"status": 504, "elapsed": 2.1, "json": {"error": {"code": -32603, "message": "MCP upstream responding: upstream_timeout"}}}
        affected = {"status": 200, "elapsed": 3.01, "json": {"result": {"content": []}}}
        for variant, profile, good in (("oracle", "legacy", explicit), ("review-baseline", "modern", explicit), ("candidate", "modern", explicit), ("candidate", "auto-modern", explicit), ("candidate", "auto-legacy", explicit), ("review-baseline", "auto-modern", auto), ("affected", "modern", affected)):
            self.assertTrue(verify_routing.route_timeout_passes(good, variant, profile))
            for elapsed in (0.01, 5.1):
                bad = copy.deepcopy(good); bad["elapsed"] = elapsed
                self.assertFalse(verify_routing.route_timeout_passes(bad, variant, profile))
        for bad in (affected, {**explicit, "status": 503}, {**explicit, "json": {"error": {"code": -32603, "message": "unrelated error"}}}):
            self.assertFalse(verify_routing.route_timeout_passes(bad, "candidate", "modern"))

    def test_fixture_keeps_route_deadline_and_plugin_timeout_independent(self):
        fixture = routing_fixture.fixture()
        ports = set()
        for listener in fixture["static_resources"]["listeners"]:
            ports.add(listener["address"]["socket_address"]["port_value"])
            config = listener["filter_chains"][0]["filters"][0]["typed_config"]
            if listener["name"] == "log-modern-http":
                self.assertEqual([], config["access_log"], "SDK log probes exclude independent access logging")
            else:
                self.assertTrue(config["access_log"], "routing evidence retains access logs")
            routes = config["route_config"]["virtual_hosts"][0]["routes"]
            self.assertEqual("2s", next(row["route"]["timeout"] for row in routes if row["match"]["prefix"] == "/entry/timeout"))
            self.assertEqual("15s", routes[-1]["route"]["timeout"])
            wasm = next(row for row in config["http_filters"] if row["name"] == "envoy.filters.http.wasm")
            server = json.loads(wasm["typed_config"]["config"]["configuration"]["value"])["server"]
            self.assertNotIn("timeout", server, "retain the default 5s callout regression")
        self.assertEqual({18080, 18081, 18082, 18083, 18084, 18085}, ports)


if __name__ == "__main__":
    unittest.main()
