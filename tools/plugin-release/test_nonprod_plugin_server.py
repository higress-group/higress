#!/usr/bin/env python3
"""Focused scratch-context and read-only ORAS contracts; no Docker required."""

import base64
import copy
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest

import nonprod_plugin_server as harness


FAKE_ORAS = '''#!/usr/bin/env python3
import base64
import json
import os
from pathlib import Path
import sys

arguments = sys.argv[1:]
with open(os.environ["FAKE_ORAS_LOG"], "a") as log:
    log.write(json.dumps(arguments) + "\\n")
fixture = json.loads(Path(os.environ["FAKE_ORAS_FIXTURE"]).read_bytes())
if len(arguments) == 4 and arguments[0] == "cp" and arguments[2] == "--to-oci-layout":
    if ":" in Path(arguments[3]).name:
        raise SystemExit("layout path must not be parsed as an OCI :tag reference")
    artifact = fixture["references"][arguments[1]]
    if fixture.get("layoutMode") == "missing":
        raise SystemExit(0)
    layout = Path(arguments[3])
    layout.mkdir(parents=True, exist_ok=True)
    descriptor = {"digest": artifact["digest"]}
    if fixture.get("layoutMode") == "wrong-digest":
        descriptor["digest"] = "sha256:" + "f" * 64
    manifests = [descriptor] * (2 if fixture.get("layoutMode") == "multiple" else 1)
    (layout / "index.json").write_text(json.dumps({"schemaVersion": 2, "manifests": manifests}))
    (layout / "oci-layout").write_text('{"imageLayoutVersion":"1.0.0"}')
    for digest, body in artifact["blobs"].items():
        blob = layout / "blobs" / "sha256" / digest[7:]
        blob.parent.mkdir(parents=True, exist_ok=True)
        blob.write_bytes(base64.b64decode(body))
elif len(arguments) == 3 and arguments[:2] == ["manifest", "fetch"]:
    sys.stdout.buffer.write(base64.b64decode(fixture["manifests"][arguments[2]]))
elif len(arguments) == 5 and arguments[:2] == ["blob", "fetch"] and arguments[3] == "-o":
    Path(arguments[4]).write_bytes(base64.b64decode(fixture["blobs"][arguments[2]]))
else:
    raise SystemExit("unexpected or writable ORAS command: " + repr(arguments))
'''


def sha256(body):
    return hashlib.sha256(body).hexdigest()


class NonprodPluginServerTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="test-nonprod-server-")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.higress = self.root / "higress"
        self.server = self.root / "plugin-server"
        self.server.mkdir()
        self.snapshot_path = "plugins/release/snapshots/3.0.0.json"
        (self.higress / self.snapshot_path).parent.mkdir(parents=True)
        (self.server / "Dockerfile").write_bytes(
            b"FROM python:3.11 AS local-true\n" + harness.BUILDER_LINE + b"\nRUN python3 pull_plugins.py\n"
        )
        (self.server / "pull_plugins.py").write_bytes(b"unchanged P_CODE reader\n")
        self.properties = b"# source inventory\r\nmcp-server=oci://public.test/plugins/mcp:3.0.0\r\n" \
            b"mcp-alias=oci://public.test/plugins/mcp:3.0.0\ncpp-filter=public.test/plugins/cpp:3.0.0\n" \
            b"legacy=public.test/plugins/legacy:1.0.0\n\n"
        (self.server / "plugins.properties").write_bytes(self.properties)
        (self.server / ".git").mkdir()
        (self.server / ".git" / "config").write_text("not a build input")
        (self.server / "__pycache__").mkdir()
        (self.server / "__pycache__" / "reader.pyc").write_bytes(b"cache")
        (self.server / "other.pyc").write_bytes(b"cache")
        self.lock = {"schemaVersion": 1, "plugins": {
            "mcp-server": {"preserveOnlyInSource": True}, "mcp-alias": {}, "cpp-filter": {},
            "legacy": {"servedVersion": "1.0.0", "history": [{"servedVersion": "0.9.0", "proof": "keep"}]},
        }}
        (self.server / "unmanaged-plugins.lock.json").write_text(json.dumps(self.lock))
        self.fixture = {"references": {}, "manifests": {}, "blobs": {}}
        self.wasm = b"\0asm\x01\0\0\0mcp-fixture"
        self.mcp = self.artifact("mcp", self.wasm)
        self.mcp.update(logicalId="mcp", consumers={"pluginServer": {
            "inventoryKey": "mcp-server", "httpPath": "mcp-bytes", "aliases": ["mcp-alias"],
        }})
        cpp = self.artifact("cpp", b"\0asm\x01\0\0\0cpp-fixture")
        cpp.update(logicalId="cpp", implementation="cpp", consumers={"pluginServer": {
            "inventoryKey": "cpp-filter", "httpPath": "cpp-filter",
        }})
        self.snapshot = {"schemaVersion": 1, "gatewayVersion": "3.0.0", "sourceCommit": "a" * 40,
                         "plugins": [self.mcp, cpp, dict(self.mcp, logicalId="same-digest", consumers={})]}
        self.oras = self.root / "oras"
        self.oras.write_text(FAKE_ORAS)
        self.oras.chmod(0o755)
        self.fixture_path = self.root / "oras.json"
        self.log = self.root / "oras.log"
        self.environment = dict(os.environ, FAKE_ORAS_FIXTURE=str(self.fixture_path), FAKE_ORAS_LOG=str(self.log))

    def artifact(self, name, wasm, legacy=False):
        layer = wasm
        media_type = harness.WASM_MEDIA_TYPE
        if legacy:
            bundle_bytes = io.BytesIO()
            with tarfile.open(fileobj=bundle_bytes, mode="w:gz") as bundle:
                member = tarfile.TarInfo("nested/plugin.wasm")
                member.size = len(wasm)
                bundle.addfile(member, io.BytesIO(wasm))
            layer = bundle_bytes.getvalue()
            media_type = sorted(harness.TAR_MEDIA_TYPES)[0]
        layer_digest = "sha256:" + sha256(layer)
        manifest = json.dumps({"schemaVersion": 2, "layers": [{
            "mediaType": media_type, "digest": layer_digest, "size": len(layer),
        }]}).encode()
        digest = "sha256:" + sha256(manifest)
        repository = "public.test/plugins/" + name
        candidate = "candidate.test/plugins/" + name + ":prepared@" + digest
        self.fixture["references"][candidate] = {"digest": digest, "blobs": {
            digest: base64.b64encode(manifest).decode(), layer_digest: base64.b64encode(layer).decode(),
        }}
        self.fixture["references"][repository + "@" + digest] = self.fixture["references"][candidate]
        self.fixture["manifests"][repository + "@" + digest] = base64.b64encode(manifest).decode()
        self.fixture["blobs"][repository + "@" + layer_digest] = base64.b64encode(layer).decode()
        return {"version": "3.0.0", "ociRef": repository + ":3.0.0", "digest": digest,
                "wasmSha256": sha256(wasm), "candidateRef": candidate}

    def run_prepare(self, name="context", expected_sha=None):
        snapshot_bytes = (json.dumps(self.snapshot, indent=3) + "\n \n").encode()
        (self.higress / self.snapshot_path).write_bytes(snapshot_bytes)
        self.fixture_path.write_text(json.dumps(self.fixture))
        context = self.root / name
        result = subprocess.run([
            sys.executable, str(Path(harness.__file__)), "prepare", "--higress-root", str(self.higress),
            "--plugin-server-root", str(self.server), "--snapshot", self.snapshot_path,
            "--snapshot-sha256", expected_sha or sha256(snapshot_bytes), "--out", str(context),
            "--oras", str(self.oras),
        ], env=self.environment, capture_output=True, text=True, timeout=30)
        return result, context, snapshot_bytes

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def assert_read_only(self):
        for call in self.calls():
            self.assertTrue((len(call) == 4 and call[0] == "cp" and call[2] == "--to-oci-layout")
                            or call[:2] in (["manifest", "fetch"], ["blob", "fetch"]), call)

    def test_exact_snapshot_filtered_lock_and_untouched_source(self):
        before = {path.relative_to(self.server): path.read_bytes()
                  for path in self.server.rglob("*") if path.is_file()}
        (self.root / "context").mkdir()
        result, context, snapshot_bytes = self.run_prepare()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((context / "snapshot.json").read_bytes(), snapshot_bytes)
        self.assertEqual((context / "plugins.properties").read_bytes(), self.properties)
        self.assertEqual((context / "pull_plugins.py").read_bytes(), before[Path("pull_plugins.py")])
        filtered = json.loads((context / "unmanaged-plugins.lock.json").read_bytes())
        self.assertEqual(filtered, {"schemaVersion": 1, "plugins": {"legacy": self.lock["plugins"]["legacy"]}})
        self.assertEqual((context / "unmanaged-plugins.lock.json").read_text(),
                         json.dumps(filtered, sort_keys=True, indent=2) + "\n")
        self.assertEqual(before, {path.relative_to(self.server): path.read_bytes()
                                 for path in self.server.rglob("*") if path.is_file()})
        self.assertFalse((context / ".git").exists())
        self.assertFalse(list(context.rglob("*.pyc")))
        self.assertFalse((context / "__pycache__").exists())
        metadata = json.loads((context / "nonprod-context.json").read_bytes())
        self.assertEqual(metadata["snapshotSha256"], sha256(snapshot_bytes))
        self.assertEqual(metadata["managedKeys"], ["cpp-filter", "mcp-alias", "mcp-server"])
        self.assertEqual(metadata["removedLockKeys"], metadata["managedKeys"])
        self.assertEqual(metadata["remainingLockKeys"], ["legacy"])
        self.assertEqual(len(metadata["candidateLayouts"]), 2)
        self.assertEqual(len(self.calls()), 2)
        self.assert_read_only()

    def test_candidate_mismatch_and_missing_or_wrong_layout_fail_atomically(self):
        original = copy.deepcopy(self.snapshot)
        for mode in ("digest", "latest", "unpinned", "missing", "wrong-digest", "multiple"):
            with self.subTest(mode=mode):
                self.snapshot = copy.deepcopy(original)
                self.fixture.pop("layoutMode", None)
                entry = self.snapshot["plugins"][0]
                if mode == "digest":
                    entry["candidateRef"] = entry["candidateRef"].split("@")[0] + "@sha256:" + "b" * 64
                elif mode == "latest":
                    entry["candidateRef"] = entry["candidateRef"].replace(":prepared@", ":latest@")
                elif mode == "unpinned":
                    entry["candidateRef"] = entry["candidateRef"].split("@")[0]
                else:
                    self.fixture["layoutMode"] = mode
                result, context, _ = self.run_prepare(mode)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(context.exists())
                self.assertFalse(list(self.root.glob(".nonprod-context-*")))
        self.assert_read_only()

    def test_dockerfile_and_executable_shim_only_copy_to_layout(self):
        result, context, _ = self.run_prepare()
        self.assertEqual(result.returncode, 0, result.stderr)
        dockerfile = (context / "Dockerfile").read_bytes()
        self.assertEqual(dockerfile.count(harness.BUILDER_LINE + b"\n" + harness.INJECTION), 1)
        shim = context / "nonprod-oras"
        self.assertTrue(os.access(shim, os.X_OK))
        self.assertEqual(shim.read_text(), harness.SHIM)
        test_shim = self.root / "test-shim"
        test_shim.write_text(shim.read_text().replace('"/opt/local-oci"', json.dumps(str(context / "local-oci")))
                             .replace('"/usr/local/bin/oras-real"', json.dumps(str(self.oras))))
        test_shim.chmod(0o755)
        before = len(self.calls())
        for arguments in (["cp", "source", "remote.test/destination:tag"], ["push", "remote:tag"],
                          ["cp", "source", "--to-oci-layout", "directory", "extra"], ["tag", "source", "latest"]):
            rejected = subprocess.run([str(test_shim), *arguments], env=self.environment,
                                      capture_output=True, timeout=10)
            self.assertNotEqual(rejected.returncode, 0)
        self.assertEqual(len(self.calls()), before)
        destination = self.root / "local-copy"
        source = self.mcp["ociRef"].rsplit(":", 1)[0] + "@" + self.mcp["digest"]
        mapped = subprocess.run([str(test_shim), "cp", source, "--to-oci-layout", str(destination)],
                                env=self.environment, capture_output=True, timeout=10)
        self.assertEqual(mapped.returncode, 0, mapped.stderr)
        self.assertEqual(len(self.calls()), before)
        source_layout = context / "local-oci" / self.mcp["digest"][7:]
        self.assertEqual({path.relative_to(destination): path.read_bytes()
                          for path in destination.rglob("*") if path.is_file()},
                         {path.relative_to(source_layout): path.read_bytes()
                          for path in source_layout.rglob("*") if path.is_file()})
        public = self.artifact("public-history", b"\0asm\x01\0\0\0history")
        self.fixture_path.write_text(json.dumps(self.fixture))
        source = public["ociRef"].rsplit(":", 1)[0] + "@" + public["digest"]
        delegated = subprocess.run([str(test_shim), "cp", source, "--to-oci-layout", str(self.root / "public-copy")],
                                   env=self.environment, capture_output=True, timeout=10)
        self.assertEqual(delegated.returncode, 0, delegated.stderr)
        self.assertEqual(self.calls()[-1][1], source)
        self.assert_read_only()

    def test_mcp_raw_and_legacy_tar_local_and_public(self):
        original = copy.deepcopy(self.snapshot)
        for legacy in (False, True):
            for candidate in (False, True):
                with self.subTest(legacy=legacy, candidate=candidate):
                    self.snapshot = copy.deepcopy(original)
                    entry = self.snapshot["plugins"][0]
                    entry.update(self.artifact("mcp", self.wasm, legacy=legacy))
                    if not candidate:
                        for plugin in self.snapshot["plugins"]:
                            plugin.pop("candidateRef", None)
                        entry.pop("wasmSha256")
                    result, context, _ = self.run_prepare("mcp-%s-%s" % (legacy, candidate))
                    self.assertEqual(result.returncode, 0, result.stderr)
                    for version in ("1.0.0", "2.0.0"):
                        self.assertEqual((context / "plugins" / "mcp-server" / version / "plugin.wasm").read_bytes(), self.wasm)
                    metadata = json.loads((context / "nonprod-context.json").read_bytes())
                    self.assertEqual(metadata["mcpSeed"]["wasmSha256"], sha256(self.wasm))
                    self.assertEqual(metadata["mcpSeed"]["version"], entry["version"])
                    self.assertEqual(list((context / "plugins").iterdir()), [context / "plugins" / "mcp-server"])
                    self.assertEqual((context / "local-oci" / ".keep").exists(), not candidate)
        self.snapshot = copy.deepcopy(original)
        self.snapshot["plugins"][0]["wasmSha256"] = "0" * 64
        result, context, _ = self.run_prepare("bad-mcp-hash")
        self.assertIn("MCP Wasm SHA-256 does not match snapshot", result.stderr)
        self.assertFalse(context.exists())
        self.assertFalse(list(self.root.glob(".nonprod-context-*")))
        self.assert_read_only()

    def test_preflight_failures_leave_no_context_or_oras_calls(self):
        with self.assertRaisesRegex(ValueError, "relative path"):
            harness.snapshot_input(self.higress, "../outside.json", "0" * 64)
        with self.assertRaisesRegex(ValueError, "regular file"):
            harness.snapshot_input(self.higress, "missing.json", "0" * 64)
        result, context, _ = self.run_prepare("bad-sha", "0" * 64)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(context.exists())
        occupied = self.root / "occupied"
        occupied.mkdir()
        (occupied / "sentinel").write_bytes(b"do not overwrite")
        result, context, _ = self.run_prepare("occupied")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(list(context.iterdir()), [context / "sentinel"])
        self.assertEqual((context / "sentinel").read_bytes(), b"do not overwrite")
        for field, value in (("gatewayVersion", "03.0.0"), ("gatewayVersion", "3.0.0-01"),
                             ("sourceCommit", "not-a-commit")):
            with self.subTest(field=field, value=value):
                original = self.snapshot[field]
                self.snapshot[field] = value
                result, context, _ = self.run_prepare("invalid-input")
                self.snapshot[field] = original
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(context.exists())
        dockerfile = (self.server / "Dockerfile").read_bytes()
        (self.server / "Dockerfile").write_bytes(dockerfile + harness.BUILDER_LINE + b"\n")
        result, context, _ = self.run_prepare("duplicate-builder")
        self.assertIn("exactly one unmodified builder stage", result.stderr)
        self.assertFalse(context.exists())
        (self.server / "Dockerfile").write_bytes(dockerfile)
        self.lock["plugins"].pop("legacy")
        self.lock["plugins"]["unexpected"] = {"history": []}
        (self.server / "unmanaged-plugins.lock.json").write_text(json.dumps(self.lock))
        result, context, _ = self.run_prepare("bad-lock")
        self.assertIn("missing=['legacy'] extra=['unexpected']", result.stderr)
        self.assertFalse(context.exists())
        self.assertEqual(self.calls(), [])
        self.assertFalse(list(self.root.glob(".nonprod-context-*")))


if __name__ == "__main__":
    unittest.main()
