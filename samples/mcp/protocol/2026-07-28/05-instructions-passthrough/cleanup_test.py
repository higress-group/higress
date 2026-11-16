"""Exercise the real cleanup function with only external effects stubbed."""
import json
import os
import shlex
import subprocess
import tempfile
import unittest
from pathlib import Path


HERE = Path(__file__).parent


def run_cleanup(script, root, initial_status=0):
    start = script.index("cleanup() {")
    end = script.index("\ntrap cleanup EXIT", start)
    function = script[start:end]
    (root / "restore.json").write_text(json.dumps({"data": {"instructions.txt": "restored\n"}}))
    prefix = "\n".join((
        "MCP_DEMO_EVIDENCE=" + shlex.quote(str(root)), "CASE_DIR=" + shlex.quote(str(HERE)),
        "restore_file=" + shlex.quote(str(root / "restore.json")), "restore_needed=1", "admin=http://unused", "run_id=test", r'''
curl() { printf '{}\n'; }
kubectl() {
  case "$*" in
    *"rollout restart"*) echo restart >>"$MCP_DEMO_EVIDENCE/cleanup-trace.txt" ;;
    *"rollout status"*) echo rollout >>"$MCP_DEMO_EVIDENCE/cleanup-trace.txt" ;;
    *"apply -f"*) echo restore >>"$MCP_DEMO_EVIDENCE/cleanup-trace.txt" ;;
    *"exec deployment/instructions-mcp"*) printf 'restored\n' ;;
    *) printf '{}\n' ;;
  esac
}
wait_gateway_access() { printf '{}\n' >"$MCP_DEMO_EVIDENCE/gateway.log"; }
stop_port_forward() { echo stopped >>"$MCP_DEMO_EVIDENCE/cleanup-trace.txt"; }
'''))
    environment = dict(os.environ)
    environment.pop("GH_TOKEN", None)
    environment.pop("GITHUB_TOKEN", None)
    return subprocess.run(["bash", "-c", prefix + "\n" + function + f"\n(exit {initial_status})\ncleanup\n"],
                          env=environment, capture_output=True, text=True)


class CleanupEvidenceTest(unittest.TestCase):
    def test_evidence_failures_do_not_skip_restore_or_leave_success_completion(self):
        script = (HERE / "verify.sh").read_text()
        for target in ("summary.json", "SHA256SUMS", "invalid-phase"):
            with self.subTest(target=target), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                for phase in ("direct", "on", "off", "changed"):
                    (root / (phase + ".json")).write_text(json.dumps({"phase": phase}))
                if target == "invalid-phase":
                    (root / "on.json").write_text("invalid")
                else:
                    (root / target).mkdir()
                result = run_cleanup(script, root)
                self.assertNotEqual(result.returncode, 0, result.stderr)
                self.assertEqual((root / "cleanup-trace.txt").read_text().splitlines(), ["restore", "restart", "rollout", "stopped"])
                self.assertTrue((root / "completion.txt").read_text().startswith("exit=1;"))

    def test_original_nonzero_exit_survives_evidence_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "summary.json").mkdir()
            result = run_cleanup((HERE / "verify.sh").read_text(), root, initial_status=23)
            self.assertEqual(result.returncode, 23, result.stderr)
            self.assertTrue((root / "completion.txt").read_text().startswith("exit=23;"))
            self.assertIn("stopped", (root / "cleanup-trace.txt").read_text())


if __name__ == "__main__":
    unittest.main()
