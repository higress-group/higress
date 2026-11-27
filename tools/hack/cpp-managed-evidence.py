#!/usr/bin/env python3
import argparse
import hashlib
import json
import re
from pathlib import Path

from cpp201_wasm_imports import parse_imports


SOURCES = {"basic_auth", "bot_detect", "custom_response", "hmac_auth", "jwt_auth", "key_auth", "key_rate_limit", "oauth", "request_block"}
REQUIRED = {"plugin.wasm", "source-head.txt", "verification-head.txt", "metadata.json", "native-result.txt", "native.log", "build.log", "imports.json", "toolchain.txt", "bazel-version.txt", "targets.txt"}


def verify(directory, source, artifact_source, verification_head, run_id, attempt):
    if not all(re.fullmatch(r"[0-9a-f]{40}", value) for value in (artifact_source, verification_head)):
        raise ValueError("invalid exact source/verification identity")
    sums = {}
    for line in (directory / "SHA256SUMS").read_text().splitlines():
        digest, filename = line.split(maxsplit=1)
        filename = filename.lstrip(" *")
        if filename not in REQUIRED or filename in sums or not re.fullmatch(r"[0-9a-f]{64}", digest):
            raise ValueError("unsafe, duplicate or unexpected artifact hash claim")
        target = directory / filename
        if target.is_symlink() or not target.is_file() or target.stat().st_size == 0:
            raise ValueError("missing or symlink final evidence: " + filename)
        if hashlib.sha256(target.read_bytes()).hexdigest() != digest:
            raise ValueError("artifact evidence hash mismatch: " + filename)
        sums[filename] = digest
    if set(sums) != REQUIRED:
        raise ValueError("incomplete artifact evidence hash set")
    if (directory / "source-head.txt").read_text().strip() != artifact_source or (directory / "verification-head.txt").read_text().strip() != verification_head:
        raise ValueError("artifact source or verification head mismatch")
    metadata = json.loads((directory / "metadata.json").read_text())
    expected = {"source": source, "artifactSourceCommit": artifact_source, "verificationHead": verification_head, "runId": str(run_id), "runAttempt": str(attempt), "wasmSha256": sums["plugin.wasm"]}
    if metadata != expected:
        raise ValueError("artifact head/run/attempt/bytes binding mismatch")
    if (directory / "native-result.txt").read_text().strip() != "success":
        raise ValueError("native test missing, skipped or failed")
    wasm = (directory / "plugin.wasm").read_bytes()
    imports = json.loads((directory / "imports.json").read_text())
    if imports != {"sha256": sums["plugin.wasm"], "size": len(wasm), "imports": parse_imports(wasm)}:
        raise ValueError("Wasm import/magic/hash evidence mismatch")
    return expected


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--artifact-dir", type=Path, required=True)
    parser.add_argument("--head", required=True)
    parser.add_argument("--verification-head", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--run-attempt", required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()
    records = []
    expected_names = {f"cpp-managed-{source}-{args.head}" for source in SOURCES}
    actual_names = {entry.name for entry in args.artifact_dir.iterdir() if entry.is_dir() and entry.name.startswith("cpp-managed-")}
    if actual_names != expected_names:
        raise ValueError("exact-run build artifact set is not the complete fixed nine")
    for source in sorted(SOURCES):
        records.append(verify(args.artifact_dir / f"cpp-managed-{source}-{args.head}", source, args.head, args.verification_head, args.run_id, args.run_attempt))
    args.output_dir.mkdir(parents=True, exist_ok=True)
    (args.output_dir / "native-results.txt").write_text("".join(record["source"] + "\tsuccess\n" for record in records))
    (args.output_dir / "build-index.json").write_text(json.dumps(records, indent=2, sort_keys=True) + "\n")
    print("native=9/9; wasm=9/9; exact-head/run/artifact binding=verified")


if __name__ == "__main__":
    main()
