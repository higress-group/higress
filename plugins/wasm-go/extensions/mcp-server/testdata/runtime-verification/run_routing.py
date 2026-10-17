#!/usr/bin/env python3
"""Build pinned source snapshots and compare them in real Envoy/Wasm containers.

Only resources prefixed with this invocation's random name are removed. The
harness uses one immutable image set and one configuration for all variants.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
import uuid

HERE = Path(__file__).resolve().parent
ROOT = Path(subprocess.check_output(["git", "-C", str(HERE), "rev-parse", "--show-toplevel"], text=True).strip())


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    for name in ("oracle", "affected", "candidate", "gateway-image", "output"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--backend-image", default=os.environ.get("ROUTING_BACKEND_IMAGE", "docker.io/library/python:3.12-alpine"))
    parser.add_argument("--engine", default=os.environ.get("CONTAINER_ENGINE", "podman" if shutil.which("podman") else "docker"))
    args = parser.parse_args()
    out = Path(args.output).resolve()
    out.mkdir(parents=True, exist_ok=False)
    # Every subprocess and container consumes this immutable invocation copy.
    # Editing the checkout while a long run is active cannot mix fixture inputs.
    harness = out / "harness"
    harness.mkdir()
    for name in ("run-routing.sh", "run_routing.py", "routing_fixture.py", "routing_backend.py", "verify_routing.py"):
        shutil.copyfile(HERE / name, harness / name)
    log = open(out / "commands.jsonl", "w")
    prefix = "mcp-routing-" + uuid.uuid4().hex[:10]
    resources = []
    manifest = {"harness_head": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip(),
                "fixtures": {p.name: sha(p) for p in sorted(harness.iterdir()) if p.is_file()}, "versions": {}, "images": {}, "resource_prefix": prefix}

    def run(command, *, cwd=None, env=None, allow_failure=False, target=None):
        begin = time.time()
        result = subprocess.run(command, cwd=cwd, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        record = {"command": command, "cwd": str(cwd) if cwd else None, "exit_code": result.returncode, "elapsed_seconds": round(time.time()-begin, 3)}
        log.write(json.dumps(record) + "\n"); log.flush()
        if target: Path(target).write_text(result.stdout)
        if result.returncode and not allow_failure: raise RuntimeError(str(command) + " failed: " + result.stdout[-4000:])
        return result

    def pinned(image):
        if "@sha256:" not in image:
            run([args.engine, "pull", image], target=out / "image-pull.log")
        inspection = json.loads(run([args.engine, "image", "inspect", image]).stdout)[0]
        digests = inspection.get("RepoDigests", [])
        reference = image if "@sha256:" in image else next((x for x in digests if "@sha256:" in x), None)
        if not reference: raise RuntimeError("Cannot pin image " + image)
        manifest["images"][reference] = inspection
        return reference

    success = True
    try:
        gateway, backend = pinned(args.gateway_image), pinned(args.backend_image)
        manifest["toolchain"] = run(["go", "version"]).stdout.strip()
        manifest["engine"] = run([args.engine, "version"]).stdout
        for variant in ("oracle", "affected", "candidate"):
            revision = getattr(args, variant)
            revision = run(["git", "rev-parse", revision + "^{commit}"], cwd=ROOT).stdout.strip()
            variant_out = out / variant; variant_out.mkdir()
            with tempfile.TemporaryDirectory(prefix="mcp-routing-source-") as temporary:
                source = Path(temporary)
                archive = source / "source.tar"
                run(["git", "archive", "--format=tar", "--output=" + str(archive), revision, "plugins/wasm-go"], cwd=ROOT)
                with tarfile.open(archive) as bundle: bundle.extractall(source, filter="data")
                plugin = source / "plugins/wasm-go/extensions/mcp-server"
                env = dict(os.environ, GOOS="wasip1", GOARCH="wasm")
                run(["go", "build", "-trimpath", "-buildmode=c-shared", "-o", str(variant_out / "plugin.wasm"), "."], cwd=plugin, env=env, target=variant_out / "build.log")
                run(["go", "list", "-m", "-json", "all"], cwd=plugin, target=variant_out / "modules.json")
                manifest["versions"][variant] = {"source": revision, "wasm_sha256": sha(variant_out / "plugin.wasm"),
                    "module_files": {str(p.relative_to(source)): sha(p) for p in sorted(source.rglob("go.*")) if p.name in ("go.mod", "go.sum")}}
            run([sys.executable, str(harness / "routing_fixture.py"), str(variant_out / "envoy.json")])
            manifest["versions"][variant]["envoy_sha256"] = sha(variant_out / "envoy.json")
            network = prefix + "-" + variant
            run([args.engine, "network", "create", network]); resources.append(("network", network))
            names = []
            try:
                for name in ("backend", "backend-alt", "wrong"):
                    container = network + "-" + name
                    run([args.engine, "run", "-d", "--name", container, "--network", network, "--network-alias", name,
                         "-v", str(harness) + ":/harness:ro", "-v", str(variant_out) + ":/evidence", "-e", "BACKEND_ID=" + name,
                         backend, "python", "/harness/routing_backend.py"])
                    resources.append(("container", container)); names.append(container)
                container = network + "-gateway"
                run([args.engine, "run", "-d", "--name", container, "--network", network, "--network-alias", "gateway",
                     "-v", str(variant_out) + ":/evidence:ro", "--entrypoint", "/usr/local/bin/envoy", gateway,
                     "-c", "/evidence/envoy.json", "--concurrency", "1", "--log-level", "info"])
                resources.append(("container", container)); names.append(container)
                run([args.engine, "run", "--rm", "--network", network, "-v", str(harness) + ":/harness:ro", "-v", str(variant_out) + ":/evidence",
                     "-e", "ROUTING_VARIANT=" + variant, backend, "python", "/harness/verify_routing.py"], allow_failure=True, target=variant_out / "verify.log")
                rows = json.loads((variant_out / "assertions.json").read_text())
                success = all(row["pass"] for row in rows) and success
            finally:
                for name in names:
                    run([args.engine, "logs", name], allow_failure=True, target=variant_out / (name.rsplit(variant + "-", 1)[-1] + ".log"))
                    run([args.engine, "rm", "-f", name], allow_failure=True)
                run([args.engine, "network", "rm", network], allow_failure=True)
        manifest["passed"] = success
    except Exception as error:
        manifest["error"] = str(error); success = False
    finally:
        for kind, name in reversed(resources):
            run([args.engine, "rm", "-f", name] if kind == "container" else [args.engine, "network", "rm", name], allow_failure=True)
        inventory = run([args.engine, "ps", "-a", "--format", "{{.Names}}"], target=out / "cleanup-containers.txt").stdout
        networks = run([args.engine, "network", "ls", "--format", "{{.Name}}"], target=out / "cleanup-networks.txt").stdout
        manifest["cleanup_complete"] = prefix not in inventory and prefix not in networks
        success = success and manifest["cleanup_complete"]
        log.close()
        manifest["artifacts"] = {str(p.relative_to(out)): sha(p) for p in sorted(out.rglob("*")) if p.is_file()}
        (out / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(json.dumps({"passed": success, "evidence": str(out), "error": manifest.get("error")}))
    return 0 if success else 1


if __name__ == "__main__":
    raise SystemExit(main())
