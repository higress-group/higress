#!/usr/bin/env python3
"""Pinned #5281 loop, reusing Compose, bounded lifecycle and evidence finalizer."""
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
import uuid

import compat_contract as contract
from compat_fixture import SSE_NO_SPACE_BASELINE, sse_no_space_cases, sse_no_space_config
from run_routing import ROOT, recorded_run, sha

HERE = Path(__file__).resolve().parent


def clean_environment(extra=None):
    return {**{key: value for key, value in os.environ.items() if key not in ("GH_TOKEN", "GITHUB_TOKEN")},
            "PYTHONDONTWRITEBYTECODE": "1", **(extra or {})}


def source_is_clean(revision):
    return subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip() == revision and not subprocess.check_output(["git", "status", "--porcelain"], cwd=ROOT, text=True).strip()


def sse_no_space_errors(out, inputs):
    """Reject incomplete, stale or non-equivalent focused evidence."""
    from collections import Counter
    errors = []
    def require(value, message):
        if not value: errors.append(message)
    require(not inputs.get("error") and not inputs.get("verifierFailed"), "runner/build/verifier failed")
    require(inputs.get("sourceStillClean") is True and inputs.get("tokenOverridesUnset") is True, "source/token preconditions failed")
    require(inputs.get("cleanupComplete") is True, "run-owned cleanup incomplete")
    require(inputs.get("sseBaseline") == SSE_NO_SPACE_BASELINE, "wrong affected source")
    require(inputs.get("verifierExits") == {"sse-baseline": 1, "candidate": 0}, "baseline red/candidate green process exits missing")
    expected, snapshots = sse_no_space_cases(), {}
    for role, revision in (("sse-baseline", SSE_NO_SPACE_BASELINE), ("candidate", inputs.get("candidate"))):
        try:
            version = inputs["versions"][role]
            require(version["source"] == revision and set(version["wasm"]) == {"plain"}, role + ": source/flavor mismatch")
            require(version["wasm"]["plain"] == sha(out / f"plain-{role}.wasm"), role + ": Wasm hash mismatch")
            snapshot = json.loads((out / f"sse-no-space-{role}.json").read_text())
            snapshots[role] = snapshot
            require(snapshot["variant"] == role and snapshot["expectedCases"] == expected and not snapshot.get("startupError"), role + ": registry/startup mismatch")
            require(contract.complete(expected, snapshot["cases"]), role + ": missing/duplicate case")
            require(Counter(row["id"] for row in expected) == Counter(row["case"] for row in snapshot["exchanges"]), role + ": missing/duplicate exchange")
            access_ids = [row["accessRequestId"] for row in snapshot["exchanges"]]
            require(all(access_ids) and len(access_ids) == len(set(access_ids)), role + ": invalid access identity")
            gateway_log = (out / f"gateway-{role}.log").read_text()
            for request_id in access_ids:
                require(gateway_log.count("access request_id=" + request_id + " ") == 1, role + ": missing/duplicate gateway access")
            rows = {row["id"]: row for row in snapshot["cases"]}
            for case in expected:
                row = rows[case["id"]]
                red = role == "sse-baseline" and case["target"]
                require(row["status"] == ("FAIL" if red else "PASS") and row.get("validRedReproduction") is red, role + ": unexpected outcome " + case["id"])
                require(bool(row.get("detail", {}).get("ledger", {}).get("events")), role + ": unobserved case " + case["id"])
            require(inputs["configs"][role] == sha(out / f"envoy-{role}.yaml"), role + ": config hash mismatch")
        except (KeyError, OSError, ValueError, TypeError) as error:
            errors.append(role + ": incomplete evidence: " + str(error))
    try:
        require(inputs["configs"]["sse-baseline"] == inputs["configs"]["candidate"], "role configs differ")
        before, after = (inputs["versions"][role] for role in ("sse-baseline", "candidate"))
        require(before["go"] == after["go"] and before["moduleFiles"] == after["moduleFiles"], "Go/module inputs differ")
        require(len(inputs["images"]) == 2 and all("@sha256:" in name for name in inputs["images"]), "immutable image pair missing")
        for name, digest in inputs["harness"].items():
            require(sha(out / "harness" / name) == digest, "harness mutated: " + name)
        left, right = (snapshots[role] for role in ("sse-baseline", "candidate"))
        for first, second in zip(left["exchanges"], right["exchanges"]):
            require(all(first[key] == second[key] for key in ("case", "request", "requestHeadersSha256")), "role requests differ")
        for first, second in zip(left["cases"], right["cases"]):
            wire = lambda row: [(event["rpcMethod"], event["sseFixture"]["sha256"]) for event in row["detail"]["ledger"]["events"] if "sseFixture" in event]
            require(wire(first) == wire(second), "backend wire differs: " + first["id"])
            case = next(row for row in expected if row["id"] == first["id"])
            if not case["target"] and case["format"] != "invalid":
                require(first["detail"]["resultSha256"] == second["detail"]["resultSha256"], "control business result differs: " + first["id"])
    except (KeyError, OSError, ValueError, TypeError) as error:
        errors.append("equivalence evidence incomplete: " + str(error))
    return errors


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", default=os.environ.get("RUNTIME_EVIDENCE"), required=not bool(os.environ.get("RUNTIME_EVIDENCE")))
    parser.add_argument("--candidate", default="HEAD")
    parser.add_argument("--oracle", default=os.environ.get("COMPAT_ORACLE_SHA", contract.ORACLE))
    parser.add_argument("--control", default=os.environ.get("COMPAT_CONTROL_SHA", contract.CONTROL))
    parser.add_argument("--gateway-image", default="higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:v2.2.3")
    parser.add_argument("--backend-image", default="docker.io/library/python:3.12-alpine")
    parser.add_argument("--skip-routing", action="store_true", help="Development only; evidence cannot pass A12 without routing")
    parser.add_argument("--sse-no-space", action="store_true", help="Only the closed #5308 plain baseline/candidate matrix")
    parser.add_argument("--sse-baseline", help="Exact affected #5308 source, required with --sse-no-space")
    args = parser.parse_args()
    if args.sse_no_space != bool(args.sse_baseline) or args.sse_no_space and args.sse_baseline != SSE_NO_SPACE_BASELINE:
        parser.error("--sse-no-space requires --sse-baseline " + SSE_NO_SPACE_BASELINE)
    candidate = subprocess.check_output(["git", "rev-parse", args.candidate + "^{commit}"], cwd=ROOT, text=True).strip()
    if args.oracle != contract.ORACLE or args.control != contract.CONTROL:
        parser.error("business oracle/control baseline must match the approved exact pins")
    if not source_is_clean(candidate):
        parser.error("candidate must be the clean exact checkout HEAD")
    out = Path(args.output).resolve()
    out.mkdir(parents=True, exist_ok=True)
    if any(out.iterdir()):
        parser.error("evidence directory must be empty")
    harness = out / "harness"
    shutil.copytree(HERE, harness, ignore=shutil.ignore_patterns("__pycache__", "*.pyc"))
    project = "mcp-compat-" + uuid.uuid4().hex[:10]
    env = clean_environment({"RUNTIME_EVIDENCE": str(out), "COMPOSE_PROJECT_NAME": project,
                             "RUNTIME_SOURCE_ROOT": str(ROOT),
                             "RUNTIME_LIFECYCLE_DIAGNOSTICS": str(out / "lifecycle-diagnostics.log")})
    log = (out / "commands.jsonl").open("w")
    inputs = {"candidate": candidate, "businessOracle": contract.ORACLE, "controlScopeBaseline": contract.CONTROL,
              "nov25Baseline": contract.NOV25_BASELINE, "nov25Configs": {},
              "tokenOverridesUnset": True, "project": project, "versions": {}, "images": {},
              "harness": {str(path.relative_to(harness)): sha(path) for path in sorted(harness.rglob("*")) if path.is_file()},
              "registeredFixtureSha256": sha(harness / "registered_fixture.go.txt"), "routingRequired": not args.skip_routing}
    if args.sse_no_space:
        inputs.update(mode="sse-no-space", sseBaseline=SSE_NO_SPACE_BASELINE, configs={}, verifierExits={})
    failure = None
    verifier_failed = False

    def run(command, **options):
        return recorded_run(command, log, env=env, **options)

    def pinned(image):
        # Cached images are acceptable: record and use the resolved immutable
        # digest for every variant, rather than re-pulling between variants.
        inspection = json.loads(run(["podman", "image", "inspect", image]).stdout)[0]
        reference = image if "@sha256:" in image else next((value for value in inspection.get("RepoDigests", []) if "@sha256:" in value), None)
        if not reference:
            raise RuntimeError("cannot resolve an immutable image digest")
        inputs["images"][reference] = inspection
        return reference

    compose_files = ["-f", str(harness / "compose.yaml"), "-f", str(out / "compose-compat.json")]
    def compose(*arguments, **options):
        return run(["podman", "compose", *compose_files, "-p", project, *arguments], cwd=harness, **options)

    def lifecycle(action):
        script = 'compose_runtime() { podman compose -f "$COMPAT_COMPOSE" -f "$COMPAT_OVERRIDE" -p "$COMPOSE_PROJECT_NAME" "$@"; }; . "$COMPAT_LIFECYCLE"; ' + action
        lifecycle_env = {**env, "COMPAT_COMPOSE": str(harness / "compose.yaml"), "COMPAT_OVERRIDE": str(out / "compose-compat.json"), "COMPAT_LIFECYCLE": str(harness / "lifecycle.sh")}
        return recorded_run(["bash", "-c", script], log, cwd=harness, env=lifecycle_env)

    try:
        gateway, backend = pinned(args.gateway_image), pinned(args.backend_image)
        inputs["python"] = sys.version
        inputs["engine"] = run(["podman", "version"]).stdout
        override = {"services": {"gateway": {"image": gateway}, "backend-primary": {"image": backend}, "backend-secondary": {"image": backend}, "verifier": {"image": backend}}}
        (out / "compose-compat.json").write_text(json.dumps(override, indent=2))
        if not args.sse_no_space:
            run(["go", "run", str(harness / "builtin_fixture.go"), str(ROOT), str(out)], cwd=ROOT / "plugins/wasm-go/extensions/mcp-server", target=out / "builtin-export.log")
        variants = (("sse-baseline", SSE_NO_SPACE_BASELINE), ("candidate", candidate)) if args.sse_no_space else (("oracle", contract.ORACLE), ("control", contract.CONTROL), ("candidate", candidate))
        for variant, revision in variants:
            with tempfile.TemporaryDirectory(prefix="mcp-compat-source-", dir=out) as temporary:
                source = Path(temporary)
                archive = source / "source.tar"
                run(["git", "archive", "--format=tar", "--output=" + str(archive), revision, "plugins/wasm-go"], cwd=ROOT)
                with tarfile.open(archive) as bundle:
                    bundle.extractall(source, filter="data")
                extension = source / "plugins/wasm-go/extensions/mcp-server"
                inputs["versions"][variant] = {"source": revision, "go": run(["go", "version"], cwd=extension).stdout.strip(), "wasm": {},
                    "moduleFiles": {str(path.relative_to(source)): sha(path) for path in sorted(source.rglob("go.*")) if path.name in ("go.mod", "go.sum")}}
                run(["go", "list", "-m", "-json", "all"], cwd=extension, target=out / f"modules-{variant}.json")
                for flavor in (("plain",) if args.sse_no_space else ("plain", "registered")):
                    if flavor == "registered":
                        shutil.copyfile(harness / "registered_fixture.go.txt", extension / "000_runtime_fixture.go")
                    wasm = out / f"{flavor}-{variant}.wasm"
                    build_env = {**env, "GOOS": "wasip1", "GOARCH": "wasm"}
                    recorded_run(["go", "build", "-trimpath", "-buildmode=c-shared", "-o", str(wasm), "."], log, cwd=extension, env=build_env, target=out / f"build-{flavor}-{variant}.log")
                    inputs["versions"][variant]["wasm"][flavor] = sha(wasm)
            if args.sse_no_space:
                shutil.copyfile(out / f"plain-{variant}.wasm", out / "plugin.wasm")
            if variant == "control": shutil.copyfile(out / "plain-control.wasm", out / "instructions-baseline-plugin.wasm")
            if variant == "candidate": shutil.copyfile(out / "plain-candidate.wasm", out / "plugin.wasm")
            if args.sse_no_space:
                sse_no_space_config(out)
                inputs["configs"][variant] = sha(out / "envoy.yaml")
            else:
                run([sys.executable, str(harness / "compat_fixture.py"), str(out), variant])
            shutil.copyfile(out / "envoy.yaml", out / f"envoy-{variant}.yaml")
            # Compose providers may print banners on stderr; keep config JSON
            # pure while retaining those diagnostics in the evidence bundle.
            compose("--profile", "verify", "config", "--format", "json", target=out / f"compose-{variant}.json",
                    stderr_target=out / f"compose-{variant}.stderr.log")
            run([sys.executable, str(harness / "compose_config_self_test.py"), str(out / f"compose-{variant}.json")])
            compose("up", "-d", "backend-primary", "backend-secondary")
            lifecycle("reset_primary_backend")
            compose("up", "-d", "gateway")
            result = compose("--profile", "verify", "run", "--rm", "--no-deps", "-e", "COMPAT_VARIANT=" + variant, "verifier", "python", "/harness/verify_compat.py", *(["--sse-no-space"] if args.sse_no_space else []), allow_failure=True, target=out / f"verify-{variant}.log")
            if args.sse_no_space:
                inputs["verifierExits"][variant] = result.returncode
            verifier_failed |= result.returncode != (1 if args.sse_no_space and variant == "sse-baseline" else 0)
            if not args.sse_no_space and variant != "oracle":
                result = compose("--profile", "verify", "run", "--rm", "--no-deps", "-e", "COMPAT_VARIANT=" + variant,
                    "-e", "RUNTIME_COMPAT_MECHANISM_BASELINE=" + ("1" if variant == "control" else "0"),
                    "-e", "RUNTIME_INSTRUCTIONS_VARIANT=" + ("baseline" if variant == "control" else "candidate"),
                    "verifier", "python", "/harness/verify_compat.py", "--supplements", allow_failure=True, target=out / f"supplements-{variant}.log")
                verifier_failed |= result.returncode != 0
            compose("exec", "-T", "gateway", "/usr/local/bin/envoy", "--version", target=out / f"envoy-{variant}-version.txt")
            lifecycle("stop_runtime_service gateway")
            compose("logs", "--no-color", "gateway", target=out / f"gateway-{variant}.log")
            compose("rm", "-f", "gateway")
        if not args.sse_no_space:
            # The new bug has its own exact pre-fix module. Never retarget the
            # historical business oracle or scope-control baseline above.
            with tempfile.TemporaryDirectory(prefix="mcp-nov25-source-", dir=out) as temporary:
                source = Path(temporary)
                archive = source / "source.tar"
                run(["git", "archive", "--format=tar", "--output=" + str(archive), contract.NOV25_BASELINE, "plugins/wasm-go"], cwd=ROOT)
                with tarfile.open(archive) as bundle:
                    bundle.extractall(source, filter="data")
                extension = source / "plugins/wasm-go/extensions/mcp-server"
                wasm = out / "plain-nov25-baseline.wasm"
                recorded_run(["go", "build", "-trimpath", "-buildmode=c-shared", "-o", str(wasm), "."], log, cwd=extension,
                             env={**env, "GOOS": "wasip1", "GOARCH": "wasm"}, target=out / "build-nov25-baseline.log")
                inputs["versions"]["nov25-baseline"] = {"source": contract.NOV25_BASELINE, "go": run(["go", "version"], cwd=extension).stdout.strip(),
                    "wasm": {"plain": sha(wasm)}, "moduleFiles": {str(path.relative_to(source)): sha(path) for path in sorted(source.rglob("go.*")) if path.name in ("go.mod", "go.sum")}}
            from compat_fixture import nov25_config
            for variant in ("nov25-baseline", "candidate"):
                shutil.copyfile(out / f"plain-{variant}.wasm", out / "plugin.wasm")
                nov25_config(out)
                config = out / f"envoy-nov25-{variant}.yaml"
                shutil.copyfile(out / "envoy.yaml", config)
                inputs["nov25Configs"][variant] = sha(config)
                compose("up", "-d", "backend-primary", "backend-secondary")
                lifecycle("reset_primary_backend")
                compose("up", "-d", "gateway")
                result = compose("--profile", "verify", "run", "--rm", "--no-deps", "-e", "COMPAT_VARIANT=" + variant,
                                 "verifier", "python", "/harness/verify_compat.py", "--nov25", allow_failure=True, target=out / f"verify-nov25-{variant}.log")
                verifier_failed |= result.returncode != 0
                compose("exec", "-T", "gateway", "/usr/local/bin/envoy", "--version", target=out / f"envoy-nov25-{variant}-version.txt")
                lifecycle("stop_runtime_service gateway")
                compose("logs", "--no-color", "gateway", target=out / f"gateway-nov25-{variant}.log")
                compose("rm", "-f", "gateway")
            if not args.skip_routing:
                result = run([sys.executable, str(harness / "run_routing.py"), "--oracle", contract.ORACLE, "--skip-affected", "--candidate", candidate,
                              "--gateway-image", gateway, "--backend-image", backend, "--output", str(out / "routing")], allow_failure=True, target=out / "routing-command.log")
                verifier_failed |= result.returncode != 0
    except Exception as error:
        failure = str(error)
    finally:
        try:
            cleanup = compose("--profile", "verify", "down", "--volumes", "--remove-orphans", allow_failure=True)
            remaining = run(["podman", "ps", "-a", "--filter", "label=com.docker.compose.project=" + project, "--format", "{{.ID}}"], allow_failure=True, target=out / "cleanup-containers.txt")
            inputs["cleanupExitCode"] = cleanup.returncode
            inputs["cleanupComplete"] = cleanup.returncode == 0 and remaining.returncode == 0 and not remaining.stdout.strip()
            if args.sse_no_space:
                networks = run(["podman", "network", "ls", "--filter", "label=com.docker.compose.project=" + project, "--format", "{{.ID}}"], allow_failure=True, target=out / "cleanup-networks.txt")
                volumes = run(["podman", "volume", "ls", "--filter", "label=com.docker.compose.project=" + project, "--format", "{{.Name}}"], allow_failure=True, target=out / "cleanup-volumes.txt")
                inputs["cleanupComplete"] &= networks.returncode == volumes.returncode == 0 and not networks.stdout.strip() and not volumes.stdout.strip()
        except Exception as error:
            inputs["cleanupComplete"] = False
            failure = failure or str(error)
        inputs.update(error=failure, verifierFailed=verifier_failed, sourceStillClean=source_is_clean(candidate))
        (out / "compat-inputs.json").write_text(json.dumps(inputs, indent=2, sort_keys=True) + "\n")
        if args.sse_no_space:
            errors = sse_no_space_errors(out, inputs)
            log.close()
            manifest = {"mode": "sse-no-space", "passed": not errors, "errors": errors,
                        "baselineTargetsPassed": False, "baselineTargetsExpectedRed": not errors,
                        "cleanupComplete": inputs["cleanupComplete"],
                        "checksums": {str(path.relative_to(out)): sha(path) for path in sorted(out.rglob("*")) if path.is_file() and path.name != "sse-no-space-manifest.json"}}
            (out / "sse-no-space-manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
            final_code = int(bool(errors))
        else:
            final_env = {**env, "RUNTIME_VERIFY_COMPAT": "1"}
            final = recorded_run([sys.executable, str(harness / "finalize_evidence.py")], log, env=final_env, allow_failure=True, target=out / "finalize.log")
            log.close()
            # Seal only after the self-recording command boundary has closed.
            from compat_evidence import seal_checksums
            seal_checksums(out)
            final_code = final.returncode
    print(json.dumps({"evidence": str(out), "passed": final_code == 0}))
    return final_code


if __name__ == "__main__":
    raise SystemExit(main())
