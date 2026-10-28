import hashlib
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import finish_evidence


class FinalEvidenceTest(unittest.TestCase):
    def make_root(self, directory):
        root = Path(directory)
        for phase in ("direct", "on", "off", "changed"):
            (root / (phase + ".json")).write_text(json.dumps({"phase": phase}))
        return root

    def check_written_status(self, root, expected):
        self.assertTrue((root / "completion.txt").read_text().startswith(f"exit={expected};"))
        if (root / "summary.json").is_file():
            self.assertEqual(json.loads((root / "summary.json").read_text())["exitCode"], expected)
        if (root / "SHA256SUMS").is_file():
            for line in (root / "SHA256SUMS").read_text().splitlines():
                digest, name = line.split("  ", 1)
                self.assertEqual(hashlib.sha256((root / name).read_bytes()).hexdigest(), digest, name)

    def test_success_and_original_failure_are_consistent(self):
        for status in (0, 23):
            with tempfile.TemporaryDirectory() as directory:
                root = self.make_root(directory)
                self.assertEqual(finish_evidence.finish(root, status), status)
                self.check_written_status(root, status)

    def test_invalid_summary_and_checksum_targets_fail(self):
        for name in ("summary.json", "SHA256SUMS"):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as directory:
                root = self.make_root(directory)
                (root / name).mkdir()
                self.assertEqual(finish_evidence.finish(root, 0), 1)
                self.check_written_status(root, 1)

    def test_unwritable_summary_and_checksum_targets_fail(self):
        real_write = finish_evidence.atomic_write
        for name in ("summary.json", "SHA256SUMS"):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as directory:
                root = self.make_root(directory)
                def denied(path, value):
                    if path.name == name:
                        raise PermissionError("injected unwritable target")
                    return real_write(path, value)
                with patch.object(finish_evidence, "atomic_write", denied):
                    self.assertEqual(finish_evidence.finish(root, 0), 1)
                self.check_written_status(root, 1)

    def test_invalid_phase_json_fails_before_final_status(self):
        with tempfile.TemporaryDirectory() as directory:
            root = self.make_root(directory)
            (root / "on.json").write_text("invalid JSON")
            self.assertEqual(finish_evidence.finish(root, 0), 1)
            self.check_written_status(root, 1)


if __name__ == "__main__":
    unittest.main()
