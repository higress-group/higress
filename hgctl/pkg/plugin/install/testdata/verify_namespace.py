"""Use an isolated kind cluster with the WasmPlugin CRD and two namespaces.
python3 verify_namespace.py BASELINE FIXED KUBECONFIG
Namespaces higress-system and custom-ns must already exist. Deletes only fixtures.
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile

baseline, fixed, kubeconfig = map(lambda p: str(Path(p).resolve()), sys.argv[1:])

def kubectl(*args):
    return subprocess.check_output(["kubectl", "--kubeconfig", kubeconfig, *args], text=True)

results = []
with tempfile.TemporaryDirectory(prefix="hgctl-namespace-") as directory:
    root = Path(directory)
    spec = root / "spec.yaml"
    spec.write_text('spec:\n  configSchema:\n    openAPIV3Schema:\n      type: object\n')
    cases = [("explicit", ["-n", "custom-ns"], None, "custom-ns", "custom-ns"),
             ("omitted-manifest", ["-n", "custom-ns"], None, None, "custom-ns"),
             ("mismatched-manifest", ["-n", "custom-ns"], None, "higress-system", "custom-ns"),
             ("option-file", [], "custom-ns", None, "custom-ns"),
             ("flag-overrides-option", ["-n", "custom-ns"], "higress-system", None, "custom-ns"),
             ("default", [], None, None, "higress-system")]
    for variant, binary in (("baseline", baseline), ("fixed", fixed)):
        for case, flags, configured, manifest_namespace, wanted in cases:
            name = "hgctl-5229-" + variant + "-" + case
            manifest = root / "plugin.json"
            metadata = {"name": name}
            if manifest_namespace:
                metadata["namespace"] = manifest_namespace
            manifest.write_text(json.dumps({"apiVersion": "extensions.higress.io/v1alpha1",
                "kind": "WasmPlugin", "metadata": metadata,
                "spec": {"url": "file:///diagnostic.wasm", "defaultConfig": {}}}))
            option = root / "option.yaml"
            option.write_text("install:\n  namespace: " + configured + "\n" if configured else "{}\n")
            command = [binary, "plugin", "install", "--kubeconfig", kubeconfig,
                       "-y", str(manifest), "-s", str(spec), "-f", str(option), *flags]
            try:
                result = subprocess.run(command, cwd=root, capture_output=True, text=True, timeout=30)
                assert result.returncode == 0, result.stdout + result.stderr
                items = json.loads(kubectl("get", "wasmplugins.extensions.higress.io", "-A", "-o", "json"))["items"]
                actual = [item["metadata"]["namespace"] for item in items if item["metadata"]["name"] == name]
                expected = wanted if variant == "fixed" else "higress-system"
                assert actual == [expected], (case, actual, expected)
                results.append({"variant": variant, "case": case, "wanted": wanted, "actual": actual,
                                "command": command, "exit_code": result.returncode, "stdout": result.stdout,
                                "stderr": result.stderr, "binary_sha256": hashlib.sha256(Path(binary).read_bytes()).hexdigest()})
            finally:
                for namespace in ("custom-ns", "higress-system"):
                    kubectl("delete", "wasmplugins.extensions.higress.io", name, "-n", namespace, "--ignore-not-found")
print(json.dumps(results, indent=2))
