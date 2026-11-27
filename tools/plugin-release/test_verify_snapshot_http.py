#!/usr/bin/env python3
"""Contract tests for the OCI-backed, single-pass HTTP acceptance CLI."""

import base64
import copy
import hashlib
from http.server import BaseHTTPRequestHandler, HTTPServer
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import threading
import unittest


VERIFIER = Path(__file__).with_name("verify_snapshot_http.py")
WASM_MEDIA_TYPE = "application/vnd.module.wasm.content.layer.v1+wasm"
TAR_MEDIA_TYPES = (
    "application/vnd.docker.image.rootfs.diff.tar.gzip",
    "application/vnd.oci.image.layer.v1.tar+gzip",
)
FAKE_ORAS = """#!/usr/bin/env python3
import base64
import json
import os
from pathlib import Path
import sys

arguments = sys.argv[1:]
with open(os.environ["FAKE_ORAS_LOG"], "a") as log:
    log.write(json.dumps(arguments) + "\\n")
with open(os.environ["FAKE_ORAS_FIXTURE"]) as source:
    fixture = json.load(source)
if len(arguments) == 3 and arguments[:2] == ["manifest", "fetch"]:
    print(json.dumps(fixture["manifests"][arguments[2]]))
elif len(arguments) == 5 and arguments[:2] == ["blob", "fetch"] and arguments[3] == "-o":
    Path(arguments[4]).write_bytes(base64.b64decode(fixture["blobs"][arguments[2]]))
else:
    raise SystemExit("unexpected ORAS command: " + repr(arguments))
"""


def content_hash(body):
    return hashlib.sha256(body).hexdigest()


def inventory_record(artifact):
    return {key: artifact[key] for key in ("version", "digest", "wasmSha256")}


def repository(artifact):
    return artifact["ociRef"].rsplit(":", 1)[0]


def lock_record(artifact):
    return {
        "servedVersion": artifact["version"], "repository": repository(artifact),
        "tag": artifact["version"], "digest": artifact["digest"], "wasmSha256": artifact["wasmSha256"],
    }


class VerifySnapshotHTTPTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="test-snapshot-http-")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.oras = self.root / "oras"
        self.oras.write_text(FAKE_ORAS, encoding="utf-8")
        self.oras.chmod(0o755)
        self.oci = {"manifests": {}, "blobs": {}}
        self.payloads = {}
        self.paths = ["filter-bytes", "filter-inventory", "filter-alias"]
        self.current = self.artifact("traffic-filter", "7.4.0")
        self.history = self.artifact("traffic-filter", "6.8.0")
        entry = dict(self.current, logicalId="traffic-filter", history=[self.history], consumers={
            "pluginServer": {
                "httpPath": self.paths[0], "inventoryKey": self.paths[1],
                "aliases": [self.paths[2], self.paths[0], self.paths[1]],
            },
        })
        self.snapshot = {"plugins": [
            entry,
            {"logicalId": "not-a-consumer", "history": [{"unrelated": True}]},
            {"logicalId": "without-history", "consumers": {"pluginServer": {}}},
        ]}
        self.unmanaged = self.artifact("legacy-filter", "5.1.0")
        self.unmanaged_history = self.artifact("legacy-filter", "4.9.0")
        self.lock = {"plugins": {"legacy-filter": dict(
            lock_record(self.unmanaged), logicalKey="legacy-filter", servedPath="legacy-path",
            history=[lock_record(self.unmanaged_history)],
        )}}
        self.inventory = {path: dict(inventory_record(self.current), history=[inventory_record(self.history)])
                          for path in self.paths}
        self.inventory["unrelated-current-only"] = {"version": "unchanged"}
        self.expected = {}
        self.responses = {}
        self.serve("managed-current", "traffic-filter", self.paths, self.current)
        self.serve("managed-history", "traffic-filter", self.paths, self.history)
        self.serve("unmanaged-current", "legacy-filter", ["legacy-path"], self.unmanaged)
        self.serve("unmanaged-history", "legacy-filter", ["legacy-path"], self.unmanaged_history)
        self.requests = []
        self.statuses = {}
        self.lengths = {}
        fixture = self

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                fixture.requests.append(self.path)
                body = fixture.responses.get(self.path, b"not found")
                status = fixture.statuses.get(self.path, 200 if self.path in fixture.responses else 404)
                self.send_response(status)
                self.send_header("Content-Length", str(fixture.lengths.get(self.path, len(body))))
                if status == 302:
                    self.send_header("Location", next(iter(fixture.responses)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, format_string, *arguments):
                pass

        self.server = HTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=lambda: self.server.serve_forever(poll_interval=0.01))
        self.thread.start()
        self.addCleanup(self.stop_server)
        self.base_url = "http://127.0.0.1:%d" % self.server.server_port

    def stop_server(self):
        self.server.shutdown()
        self.thread.join(timeout=5)
        self.server.server_close()

    def artifact(self, logical_id, version, media_type=WASM_MEDIA_TYPE, blob=None):
        body = b"\0asm\x01\0\0\0" + (logical_id + "-" + version).encode()
        if blob is None:
            blob = body
        layer_digest = "sha256:" + content_hash(blob)
        manifest = {"schemaVersion": 2, "layers": [{"mediaType": media_type, "digest": layer_digest, "size": len(blob)}]}
        digest = "sha256:" + content_hash(json.dumps(manifest, sort_keys=True).encode())
        image = "registry.example.test:5000/plugins/" + logical_id
        self.oci["manifests"][image + "@" + digest] = manifest
        self.oci["blobs"][image + "@" + layer_digest] = base64.b64encode(blob).decode()
        self.payloads[digest] = body
        return {"version": version, "ociRef": image + ":" + version, "digest": digest, "wasmSha256": content_hash(body)}

    def serve(self, role, logical_id, paths, artifact):
        for path in paths:
            route = "/plugins/%s/%s/plugin.wasm" % (path, artifact["version"])
            self.responses[route] = self.payloads[artifact["digest"]]
            self.expected[route] = dict(inventory_record(artifact), role=role, logicalId=logical_id, path=path)

    def run_verifier(self, success, use_environment=False, extra_arguments=()):
        self.requests.clear()
        inputs = {"snapshot": self.snapshot, "inventory": self.inventory, "lock": self.lock}
        originals = {}
        arguments = [sys.executable, str(VERIFIER.resolve())]
        for name, value in inputs.items():
            path = self.root / (name + ".json")
            path.write_text(json.dumps(value), encoding="utf-8")
            originals[path] = path.read_bytes()
            arguments.extend(["--" + name, str(path)])
        fixture_path = self.root / "oci.json"
        fixture_path.write_text(json.dumps(self.oci), encoding="utf-8")
        log = self.root / "oras.jsonl"
        log.write_text("", encoding="utf-8")
        summary_path = self.root / "summary.json"
        summary_path.unlink(missing_ok=True)
        arguments.extend(["--base-url", self.base_url + "/", "--summary", str(summary_path)])
        environment = dict(os.environ, ORAS=str(self.oras), FAKE_ORAS_FIXTURE=str(fixture_path), FAKE_ORAS_LOG=str(log))
        if not use_environment:
            environment["ORAS"] = "must-not-be-used"
            arguments.extend(["--oras", str(self.oras)])
        arguments.extend(extra_arguments)
        result = subprocess.run(arguments, capture_output=True, text=True, env=environment, timeout=20)
        self.assertEqual(result.returncode, 0 if success else 1, result.stdout + result.stderr)
        summary = json.loads(summary_path.read_text(encoding="utf-8"))
        self.assertEqual(summary["status"], "ok" if success else "failed")
        self.assertEqual("MANAGED_HISTORY_HTTP_OK" in result.stdout, success)
        for path, original in originals.items():
            self.assertEqual(path.read_bytes(), original)
        calls = [json.loads(line) for line in log.read_text(encoding="utf-8").splitlines()]
        return result, summary, calls

    def test_current_and_history_http_acceptance(self):
        result, summary, calls = self.run_verifier(True)
        self.assertEqual(result.stdout.strip(), "MANAGED_HISTORY_HTTP_OK managed=1 history=1 unmanaged=1 urls=8")
        self.assertEqual(summary["counts"], {"managed": 1, "history": 1, "unmanaged": 1, "urls": 8})
        self.assertEqual(len(summary["results"]), len(self.expected))
        self.assertCountEqual(self.requests, self.expected)
        for row in summary["results"]:
            route = row["url"].removeprefix(self.base_url)
            expected = self.expected[route]
            for key in ("role", "logicalId", "path", "version", "digest"):
                self.assertEqual(row[key], expected[key])
            self.assertEqual(row["expectedSha256"], expected["wasmSha256"])
            self.assertEqual(row["actualSha256"], expected["wasmSha256"])
            self.assertEqual(row["status"], "ok")
            self.assertEqual(row["httpStatus"], 200)
        expected_calls = []
        for artifact in (self.current, self.history):
            reference = repository(artifact) + "@" + artifact["digest"]
            layer = self.oci["manifests"][reference]["layers"][0]["digest"]
            expected_calls.extend([["manifest", "fetch", reference], ["blob", "fetch", repository(artifact) + "@" + layer]])
        self.assertEqual([call[:3] for call in calls], expected_calls)
        self.assertTrue(all(call[3] == "-o" and len(call) == 5 for call in calls if call[0] == "blob"))

    def test_candidate_current_source_uses_candidate_ref(self):
        entry = self.snapshot["plugins"][0]
        candidate_repository = "registry.example.test:5000/candidates/traffic-filter"
        entry["candidateRef"] = candidate_repository + ":q-preparation@" + self.current["digest"]
        public_reference = repository(self.current) + "@" + self.current["digest"]
        candidate_reference = candidate_repository + "@" + self.current["digest"]
        manifest = self.oci["manifests"].pop(public_reference)
        self.oci["manifests"][candidate_reference] = manifest
        layer = manifest["layers"][0]["digest"]
        public_blob = repository(self.current) + "@" + layer
        candidate_blob = candidate_repository + "@" + layer
        self.oci["blobs"][candidate_blob] = self.oci["blobs"].pop(public_blob)

        _, summary, calls = self.run_verifier(True, extra_arguments=("--current-source", "candidate"))
        history_reference = repository(self.history) + "@" + self.history["digest"]
        history_layer = self.oci["manifests"][history_reference]["layers"][0]["digest"]
        expected_calls = [
            ["manifest", "fetch", candidate_reference],
            ["blob", "fetch", candidate_blob],
            ["manifest", "fetch", history_reference],
            ["blob", "fetch", repository(self.history) + "@" + history_layer],
        ]
        self.assertEqual([call[:3] for call in calls], expected_calls)
        self.assertCountEqual(self.requests, self.expected)
        for row in summary["results"]:
            if row["role"].startswith("managed-"):
                expected_reference = candidate_reference if row["role"] == "managed-current" else history_reference
                self.assertEqual(row["sourceRef"], expected_reference)

        self.oci["manifests"][public_reference] = manifest
        self.oci["blobs"][public_blob] = self.oci["blobs"][candidate_blob]
        expected_calls[:2] = [["manifest", "fetch", public_reference], ["blob", "fetch", public_blob]]
        for options in ((), ("--current-source", "candidate")):
            with self.subTest(options=options):
                if options:
                    del entry["candidateRef"]
                _, summary, calls = self.run_verifier(True, extra_arguments=options)
                self.assertEqual([call[:3] for call in calls], expected_calls)
                current_rows = [row for row in summary["results"] if row["role"] == "managed-current"]
                self.assertTrue(all(row["sourceRef"] == public_reference for row in current_rows))

    def test_candidate_ref_digest_mismatch_fails_before_http(self):
        self.snapshot["plugins"][0]["candidateRef"] = (
            "registry.example.test:5000/candidates/traffic-filter:q-preparation@sha256:" + "0" * 64
        )
        result, summary, calls = self.run_verifier(False, extra_arguments=("--current-source", "candidate"))
        self.assertEqual(result.returncode, 1)
        self.assertEqual(summary["status"], "failed")
        self.assertIn("candidateRef digest must equal snapshot digest", summary["error"])
        self.assertEqual(calls, [])
        self.assertEqual(self.requests, [])

    def test_all_managed_includes_current_only_and_aliases(self):
        current_only = self.artifact("current-only-filter", "2.3.0")
        paths = ["current-only-bytes", "current-only-inventory", "current-only-alias"]
        self.snapshot["plugins"].append(dict(current_only, logicalId="current-only-filter", history=[], consumers={
            "pluginServer": {"httpPath": paths[0], "inventoryKey": paths[1], "aliases": [paths[2]]},
        }))
        for path in paths:
            self.inventory[path] = inventory_record(current_only)
        self.serve("managed-current", "current-only-filter", paths, current_only)

        _, summary, calls = self.run_verifier(True, extra_arguments=("--all-managed",))
        self.assertEqual(summary["counts"], {"managed": 2, "history": 1, "unmanaged": 1, "urls": 11})
        self.assertCountEqual(self.requests, self.expected)
        rows = [row for row in summary["results"] if row["logicalId"] == "current-only-filter"]
        self.assertCountEqual([row["path"] for row in rows], paths)
        for row in rows:
            self.assertEqual(row["role"], "managed-current")
            self.assertEqual(row["expectedSha256"], current_only["wasmSha256"])
            self.assertEqual(row["actualSha256"], current_only["wasmSha256"])
            self.assertEqual(row["status"], "ok")
        self.assertIn(["manifest", "fetch", repository(current_only) + "@" + current_only["digest"]], calls)

        _, summary, calls = self.run_verifier(True)
        self.assertEqual(summary["counts"], {"managed": 1, "history": 1, "unmanaged": 1, "urls": 8})
        self.assertFalse(any(row["logicalId"] == "current-only-filter" for row in summary["results"]))
        self.assertCountEqual(self.requests, [
            route for route, expected in self.expected.items() if expected["logicalId"] != "current-only-filter"
        ])
        self.assertFalse(any(call[2].startswith(repository(current_only) + "@") for call in calls))

        for path in paths:
            with self.subTest(inventory_path=path):
                self.inventory[path]["history"] = []
                _, summary, _ = self.run_verifier(False, extra_arguments=("--all-managed",))
                self.assertIn("snapshot inventory mismatch for managed path: " + path, summary["error"])
                self.assertEqual(self.requests, [])
                del self.inventory[path]["history"]

    def test_malformed_plugin_server_mapping_fails_before_reads(self):
        self.snapshot["plugins"][0]["consumers"]["pluginServer"] = "invalid"
        result, summary, calls = self.run_verifier(False)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(summary["status"], "failed")
        self.assertIn("pluginServer consumer mapping must be an object", summary["error"])
        self.assertEqual(calls, [])
        self.assertEqual(self.requests, [])

    def test_nine_managed_entries_produce_eighteen_canonical_urls(self):
        self.snapshot = {"plugins": []}
        self.inventory = {}
        self.expected.clear()
        self.responses.clear()
        paths = ["managed-filter-%d" % number for number in range(1, 10)]
        for path in paths:
            current = self.artifact(path, "7.4.0")
            history = self.artifact(path, "6.8.0")
            self.snapshot["plugins"].append(dict(current, logicalId=path, history=[history], consumers={
                "pluginServer": {"httpPath": path, "inventoryKey": path, "aliases": []},
            }))
            self.inventory[path] = dict(inventory_record(current), history=[inventory_record(history)])
            self.serve("managed-current", path, [path], current)
            self.serve("managed-history", path, [path], history)
        self.serve("unmanaged-current", "legacy-filter", ["legacy-path"], self.unmanaged)
        self.serve("unmanaged-history", "legacy-filter", ["legacy-path"], self.unmanaged_history)

        _, summary, _ = self.run_verifier(True)
        self.assertEqual(summary["counts"], {"managed": 9, "history": 9, "unmanaged": 1, "urls": 20})
        managed_rows = [row for row in summary["results"] if row["role"].startswith("managed-")]
        unmanaged_rows = [row for row in summary["results"] if row["role"].startswith("unmanaged-")]
        self.assertEqual(len(managed_rows), 18)
        self.assertEqual(len(unmanaged_rows), 2)
        self.assertEqual(len(summary["results"]), 20)
        self.assertCountEqual(self.requests, self.expected)
        for path in paths:
            self.assertCountEqual(
                [row["role"] for row in managed_rows if row["path"] == path],
                ["managed-current", "managed-history"],
            )
        for row in summary["results"]:
            route = row["url"].removeprefix(self.base_url)
            expected = self.expected[route]
            for key in ("role", "logicalId", "path", "version", "digest"):
                self.assertEqual(row[key], expected[key])
            self.assertEqual(row["expectedSha256"], expected["wasmSha256"])
            self.assertEqual(row["actualSha256"], expected["wasmSha256"])
            self.assertEqual(row["status"], "ok")
            self.assertEqual(row["httpStatus"], 200)

        with self.subTest(missing_last_history=True):
            last = self.snapshot["plugins"][-1]
            route = "/plugins/%s/%s/plugin.wasm" % (last["consumers"]["pluginServer"]["httpPath"], last["history"][0]["version"])
            del self.responses[route]
            _, summary, _ = self.run_verifier(False)
            self.assertIn(route, self.requests)
            self.assertIn("HTTP 404", summary["error"])
            self.assertIn(self.base_url + route, summary["error"])

    def test_history_hash_and_oci_errors_fail_closed(self):
        snapshot = copy.deepcopy(self.snapshot)
        oci = copy.deepcopy(self.oci)
        reference = repository(self.history) + "@" + self.history["digest"]
        layer = oci["manifests"][reference]["layers"][0]["digest"]
        for mutation in ("wrong-hash", "missing-hash", "tag-drift", "blob-drift", "multiple-layers", "no-wasm-layer", "missing-manifest"):
            with self.subTest(mutation=mutation):
                self.snapshot = copy.deepcopy(snapshot)
                self.oci = copy.deepcopy(oci)
                history = self.snapshot["plugins"][0]["history"][0]
                if mutation == "wrong-hash":
                    history["wasmSha256"] = "0" * 64
                elif mutation == "missing-hash":
                    del history["wasmSha256"]
                elif mutation == "tag-drift":
                    history["ociRef"] = repository(history) + ":incorrect"
                elif mutation == "blob-drift":
                    self.oci["blobs"][repository(history) + "@" + layer] = base64.b64encode(b"corrupt blob").decode()
                elif mutation == "multiple-layers":
                    self.oci["manifests"][reference]["layers"] *= 2
                elif mutation == "no-wasm-layer":
                    self.oci["manifests"][reference]["layers"][0]["mediaType"] = "application/json"
                else:
                    del self.oci["manifests"][reference]
                self.run_verifier(False)
                self.assertEqual(self.requests, [])

    def test_http_errors_fail_closed(self):
        responses = self.responses.copy()
        managed_route = "/plugins/filter-alias/6.8.0/plugin.wasm"
        unmanaged_route = "/plugins/legacy-path/4.9.0/plugin.wasm"
        for mutation in ("404", "wrong-hash", "wrong-magic", "short-header", "truncated-read", "204", "302"):
            with self.subTest(mutation=mutation):
                self.responses = responses.copy()
                self.statuses.clear()
                self.lengths.clear()
                if mutation == "404":
                    del self.responses[managed_route]
                elif mutation == "wrong-hash":
                    self.responses[unmanaged_route] += b"corrupt"
                elif mutation == "wrong-magic":
                    self.responses[unmanaged_route] = b"not Wasm content"
                elif mutation == "short-header":
                    self.responses[managed_route] = b"\0asm"
                elif mutation == "truncated-read":
                    self.lengths[managed_route] = len(self.responses[managed_route]) + 5
                else:
                    self.statuses[managed_route] = int(mutation)
                _, summary, _ = self.run_verifier(False)
                self.assertTrue(any(row["status"] == "failed" for row in summary["results"]))

    def test_inventory_history_must_match_exactly(self):
        older = self.artifact("traffic-filter", "5.2.0")
        self.snapshot["plugins"][0]["history"].append(older)
        for path in self.paths:
            self.inventory[path]["history"].append(inventory_record(older))
        self.serve("managed-history", "traffic-filter", self.paths, older)
        self.run_verifier(True)
        inventory = copy.deepcopy(self.inventory)
        for mutation in ("missing", "reordered", "extra", "version", "digest", "wasmSha256", "extra-field", "missing-path"):
            with self.subTest(mutation=mutation):
                self.inventory = copy.deepcopy(inventory)
                entry = self.inventory["filter-alias"]
                if mutation == "missing":
                    del entry["history"]
                elif mutation == "reordered":
                    entry["history"].reverse()
                elif mutation == "extra":
                    entry["history"].append(inventory_record(self.current))
                elif mutation in ("version", "digest", "wasmSha256"):
                    entry["history"][0][mutation] = "drift"
                elif mutation == "extra-field":
                    entry["unexpected"] = True
                else:
                    del self.inventory["filter-alias"]
                _, summary, _ = self.run_verifier(False)
                self.assertIn("inventory mismatch", summary["error"])
                self.assertEqual(self.requests, [])

    def test_duplicate_served_paths_fail_before_fetches(self):
        lock = copy.deepcopy(self.lock)
        for mutation in ("managed-lock", "unmanaged-current-history"):
            with self.subTest(mutation=mutation):
                self.lock = copy.deepcopy(lock)
                entry = self.lock["plugins"]["legacy-filter"]
                if mutation == "managed-lock":
                    entry["servedPath"] = "filter-alias"
                    entry["history"][0]["servedVersion"] = self.history["version"]
                else:
                    entry["history"][0]["servedVersion"] = entry["servedVersion"]
                _, summary, calls = self.run_verifier(False)
                self.assertIn("duplicate served path+version", summary["error"])
                self.assertEqual(calls, [])
                self.assertEqual(self.requests, [])

    def test_optional_current_hash_is_derived_or_rejected(self):
        entry = self.snapshot["plugins"][0]
        del entry["wasmSha256"]
        _, summary, _ = self.run_verifier(True, use_environment=True)
        current_rows = [row for row in summary["results"] if row["role"] == "managed-current"]
        self.assertEqual(len(current_rows), len(self.paths))
        self.assertTrue(all(row["expectedSha256"] == self.current["wasmSha256"] for row in current_rows))
        entry["wasmSha256"] = "0" * 64
        _, summary, _ = self.run_verifier(False)
        self.assertIn("snapshot Wasm SHA-256 does not match OCI", summary["error"])
        self.assertEqual(self.requests, [])

    def test_legacy_tar_requires_one_safe_wasm(self):
        body = self.payloads[self.history["digest"]]
        for media_type, names, success in (
            (TAR_MEDIA_TYPES[0], ["nested/plugin.wasm"], True),
            (TAR_MEDIA_TYPES[1], ["nested/plugin.wasm"], True),
            (TAR_MEDIA_TYPES[0], ["first.wasm", "second.wasm"], False),
            (TAR_MEDIA_TYPES[0], ["../escape.wasm"], False),
        ):
            with self.subTest(media_type=media_type, names=names):
                archive = io.BytesIO()
                with tarfile.open(fileobj=archive, mode="w:gz") as bundle:
                    for name in names:
                        member = tarfile.TarInfo(name)
                        member.size = len(body)
                        bundle.addfile(member, io.BytesIO(body))
                artifact = self.artifact("traffic-filter", self.history["version"], media_type, archive.getvalue())
                self.snapshot["plugins"][0]["history"] = [artifact]
                for path in self.paths:
                    self.inventory[path]["history"] = [inventory_record(artifact)]
                self.run_verifier(success)


if __name__ == "__main__":
    unittest.main()
