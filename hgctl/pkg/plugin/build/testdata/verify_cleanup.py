# Copyright (c) 2022 Alibaba Group Holding Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Compare actual hgctl CLI cleanup: python3 verify_cleanup.py BASELINE FIXED."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile

results = []
for variant, binary in zip(("baseline", "fixed"), sys.argv[1:], strict=True):
    with tempfile.TemporaryDirectory(prefix="hgctl-cleanup-") as directory:
        root = Path(directory)
        output = root / "output"
        output.mkdir()
        sentinel = output / "sentinel"
        original = b"caller-owned content\n"
        sentinel.write_bytes(original)
        command = [str(Path(binary).resolve()), "plugin", "build", "--input", str(root),
                   "--model-dir", str(root / "missing-model"), "--output-type", "files",
                   "--output-dest", str(output)]
        result = subprocess.run(command, cwd=root, capture_output=True, text=True, timeout=30)
        assert result.returncode != 0, result.stdout + result.stderr
        assert "failed to generate wasm plugin metadata" in result.stdout + result.stderr
        preserved = sentinel.exists() and sentinel.read_bytes() == original
        assert preserved == (variant == "fixed"), result.stdout + result.stderr
        results.append({"variant": variant, "command": command, "exit_code": result.returncode,
                        "sentinel_preserved": preserved, "stdout": result.stdout,
                        "stderr": result.stderr, "binary_sha256": hashlib.sha256(Path(binary).read_bytes()).hexdigest()})
print(json.dumps(results, indent=2))
