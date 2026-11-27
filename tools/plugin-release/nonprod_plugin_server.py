#!/usr/bin/env python3
"""Prepare a scratch-only plugin-server context without registry writes."""

import argparse
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile


WASM_MEDIA_TYPE = "application/vnd.module.wasm.content.layer.v1+wasm"
TAR_MEDIA_TYPES = {
    "application/vnd.docker.image.rootfs.diff.tar.gzip",
    "application/vnd.oci.image.layer.v1.tar+gzip",
}
BUILDER_LINE = b"FROM local-${USE_LOCAL_PLUGINS:-false} AS builder"
INJECTION = b"\n".join((
    b"RUN mv /usr/local/bin/oras /usr/local/bin/oras-real",
    b"COPY nonprod-oras /usr/local/bin/oras",
    b"RUN chmod +x /usr/local/bin/oras",
    b"COPY local-oci /opt/local-oci",
))
SHIM = '''#!/usr/bin/env python3
import os
from pathlib import Path
import re
import shutil
import sys

arguments = sys.argv[1:]
if arguments and arguments[0] == "cp":
    if (len(arguments) != 4 or arguments[2] != "--to-oci-layout"
            or not arguments[1] or arguments[1].startswith("-")
            or not arguments[3] or arguments[3].startswith("-")):
        raise SystemExit("nonprod-oras: only cp <source> --to-oci-layout <dir> is allowed")
    digest = arguments[1].rpartition("@")[2]
    if re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
        layout = Path("/opt/local-oci") / digest[7:]
        if (layout / "index.json").is_file():
            destination = Path(arguments[3])
            destination.mkdir(parents=True, exist_ok=True)
            shutil.copytree(layout, destination, dirs_exist_ok=True)
            raise SystemExit(0)
elif arguments != ["version"] and arguments[:2] not in (["manifest", "fetch"], ["blob", "fetch"]):
    raise SystemExit("nonprod-oras: registry writes are forbidden")
os.execv("/usr/local/bin/oras-real", ["/usr/local/bin/oras-real", *arguments])
'''


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON key: " + key)
        result[key] = value
    return result


def load_json(body):
    value = json.loads(body, object_pairs_hook=unique_object)
    if not isinstance(value, dict):
        raise ValueError("expected a JSON object")
    return value


def write_json(path, document):
    path.write_text(json.dumps(document, sort_keys=True, indent=2) + "\n", encoding="utf-8")


def content_hash(body):
    return hashlib.sha256(body).hexdigest()


def checked_digest(value):
    if not isinstance(value, str) or not re.fullmatch(r"sha256:[0-9a-f]{64}", value):
        raise ValueError("invalid OCI digest")
    return value


def component(value):
    if not isinstance(value, str) or not re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}", value):
        raise ValueError("invalid plugin-server inventory key or version")
    return value


def snapshot_input(root, relative, expected_sha):
    path = PurePosixPath(relative)
    if path.is_absolute() or ".." in path.parts or "\\" in relative:
        raise ValueError("snapshot must be a relative path under higress-root")
    source = root / path
    if not source.resolve().is_relative_to(root) or source.is_symlink() or not source.is_file():
        raise ValueError("snapshot must be a regular file under higress-root")
    if not re.fullmatch(r"[0-9a-fA-F]{64}", expected_sha):
        raise ValueError("snapshot SHA-256 must be 64 hex characters")
    body = source.read_bytes()
    if content_hash(body) != expected_sha.lower():
        raise ValueError("snapshot SHA-256 mismatch")
    snapshot = load_json(body)
    version = snapshot.get("gatewayVersion")
    match = re.fullmatch(
        r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
        r"(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?"
        r"(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?", version if isinstance(version, str) else "",
    )
    if not match or any(part.isdigit() and len(part) > 1 and part.startswith("0")
                        for part in (match[4] or "").split(".")):
        raise ValueError("snapshot gatewayVersion must be semver")
    if not isinstance(snapshot.get("sourceCommit"), str) or not re.fullmatch(
        r"[0-9a-fA-F]{40}", snapshot["sourceCommit"],
    ):
        raise ValueError("snapshot sourceCommit must be 40 hex characters")
    if not isinstance(snapshot.get("plugins"), list):
        raise ValueError("snapshot plugins must be a list")
    return body, snapshot


def candidate_repository(artifact):
    reference = artifact["candidateRef"]
    match = re.fullmatch(
        r"([A-Za-z0-9][A-Za-z0-9._:/-]*):([A-Za-z0-9_][A-Za-z0-9_.-]{0,127})@(sha256:[0-9a-f]{64})",
        reference if isinstance(reference, str) else "",
    )
    if not match or match[2] == "latest":
        raise ValueError("invalid digest-pinned candidateRef (tag must not be latest)")
    if match[3] != artifact.get("digest"):
        raise ValueError("candidateRef digest must equal snapshot digest")
    return match[1]


def filtered_inputs(server, snapshot):
    managed = set()
    candidates = {}
    mcp_entries = []
    for entry in snapshot["plugins"]:
        if not isinstance(entry, dict) or not isinstance(entry.get("consumers", {}), dict):
            raise ValueError("invalid snapshot plugin entry")
        if "candidateRef" in entry:
            repository = candidate_repository(entry)
            candidates.setdefault(entry["digest"], {"ref": entry["candidateRef"], "repository": repository})
        mapping = entry.get("consumers", {}).get("pluginServer")
        if not mapping:
            continue
        if not isinstance(mapping, dict) or not isinstance(mapping.get("aliases", []), list):
            raise ValueError("invalid pluginServer consumer mapping")
        keys = {component(key) for key in [mapping.get("inventoryKey"), *mapping.get("aliases", [])]}
        managed.update(keys)
        if "mcp-server" in keys:
            mcp_entries.append(entry)
    if len(mcp_entries) != 1:
        raise ValueError("snapshot must have exactly one mcp-server consumer")
    properties = set()
    for raw in (server / "plugins.properties").read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        key, separator, value = line.partition("=")
        key = key.strip()
        if not separator or not key or not value.strip() or key in properties:
            raise ValueError("invalid or duplicate plugins.properties key: " + key)
        properties.add(key)
    lock = load_json((server / "unmanaged-plugins.lock.json").read_bytes())
    if lock.get("schemaVersion") != 1 or not isinstance(lock.get("plugins"), dict):
        raise ValueError("unsupported unmanaged plugin lock schema")
    removed = sorted(set(lock["plugins"]) & managed)
    lock["plugins"] = {key: value for key, value in lock["plugins"].items() if key not in managed}
    expected = properties - managed
    remaining = set(lock["plugins"])
    if remaining != expected:
        raise ValueError("unmanaged lock inventory mismatch: missing=%s extra=%s" % (
            sorted(expected - remaining), sorted(remaining - expected),
        ))
    return managed, removed, lock, candidates, mcp_entries[0]


def run_oras(oras, *arguments):
    return subprocess.run([oras, *arguments], check=True, capture_output=True, timeout=300).stdout


def layout_manifest(layout, digest):
    index_path = layout / "index.json"
    if not index_path.is_file():
        raise ValueError("candidate layout missing index.json: " + digest)
    manifests = load_json(index_path.read_bytes()).get("manifests")
    if (not isinstance(manifests, list) or len(manifests) != 1
            or not isinstance(manifests[0], dict) or manifests[0].get("digest") != digest):
        raise ValueError("candidate layout manifest digest does not equal snapshot digest: " + digest)


def verified_blob(body, digest):
    if content_hash(body) != checked_digest(digest).removeprefix("sha256:"):
        raise ValueError("OCI blob SHA-256 mismatch: " + digest)
    return body


def mcp_wasm(entry, local_oci, oras, temporary):
    digest = checked_digest(entry.get("digest"))
    layout = local_oci / digest[7:]
    if "candidateRef" in entry:
        layout_manifest(layout, digest)
        manifest_body = (layout / "blobs" / "sha256" / digest[7:]).read_bytes()
    else:
        reference = entry.get("ociRef", "")
        repository, separator, tag = reference.rpartition(":")
        if not separator or tag != entry.get("version") or "@" in reference or not re.fullmatch(
            r"[A-Za-z0-9][A-Za-z0-9._:/-]*", repository,
        ):
            raise ValueError("invalid MCP public ociRef")
        manifest_body = run_oras(oras, "manifest", "fetch", repository + "@" + digest)
    manifest = load_json(verified_blob(manifest_body, digest))
    layers = manifest.get("layers")
    if not isinstance(layers, list) or any(not isinstance(layer, dict) for layer in layers):
        raise ValueError("invalid MCP OCI manifest layers")
    supported = [layer for layer in layers if layer.get("mediaType") in TAR_MEDIA_TYPES | {WASM_MEDIA_TYPE}]
    if len(supported) != 1:
        raise ValueError("MCP manifest must have exactly one supported Wasm layer")
    layer = supported[0]
    layer_digest = checked_digest(layer.get("digest"))
    if "candidateRef" in entry:
        body = (layout / "blobs" / "sha256" / layer_digest[7:]).read_bytes()
    else:
        blob = temporary / "mcp-layer"
        run_oras(oras, "blob", "fetch", repository + "@" + layer_digest, "-o", str(blob))
        body = blob.read_bytes()
    verified_blob(body, layer_digest)
    if layer["mediaType"] in TAR_MEDIA_TYPES:
        with tarfile.open(fileobj=io.BytesIO(body), mode="r:gz") as bundle:
            members = [member for member in bundle.getmembers() if member.name.endswith(".wasm")]
            if len(members) != 1:
                raise ValueError("MCP tar layer must contain exactly one Wasm file")
            member = members[0]
            path = PurePosixPath(member.name)
            if not member.isfile() or path.is_absolute() or ".." in path.parts or "\\" in member.name:
                raise ValueError("unsafe MCP tar Wasm member")
            with bundle.extractfile(member) as source:
                body = source.read()
    if len(body) < 8 or not body.startswith(b"\0asm"):
        raise ValueError("MCP artifact is not a complete Wasm header")
    if "wasmSha256" in entry and entry["wasmSha256"] != content_hash(body):
        raise ValueError("MCP Wasm SHA-256 does not match snapshot")
    return body


def prepare(arguments):
    root = Path(arguments.higress_root).resolve()
    server = Path(arguments.plugin_server_root).resolve()
    output = Path(arguments.out).absolute()
    if output.is_symlink():
        raise ValueError("output must not be a symlink")
    output = output.resolve()
    if (not root.is_dir() or not server.is_dir() or output.is_relative_to(root) or output.is_relative_to(server)
            or server.is_relative_to(output) or root.is_relative_to(output)):
        raise ValueError("output must be separate from the source checkouts")
    if output.exists() and (not output.is_dir() or any(output.iterdir())):
        raise ValueError("output must be absent or an empty directory")
    snapshot_bytes, snapshot = snapshot_input(root, arguments.snapshot, arguments.snapshot_sha256)
    managed, removed, lock, candidates, mcp = filtered_inputs(server, snapshot)
    dockerfile = (server / "Dockerfile").read_bytes()
    lines = dockerfile.splitlines(keepends=True)
    matches = [index for index, line in enumerate(lines) if line.rstrip(b"\r\n") == BUILDER_LINE]
    if len(matches) != 1 or b"nonprod-oras" in dockerfile:
        raise ValueError("Dockerfile must contain exactly one unmodified builder stage")
    index = matches[0]
    lines[index] = BUILDER_LINE + b"\n" + INJECTION + b"\n"
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".nonprod-context-", dir=output.parent) as scratch:
        temporary = Path(scratch)
        context = temporary / "context"
        local_oci = temporary / "local-oci"
        local_oci.mkdir()
        for digest, candidate in sorted(candidates.items()):
            layout = local_oci / digest[7:]
            run_oras(arguments.oras, "cp", candidate["ref"], "--to-oci-layout", str(layout))
            layout_manifest(layout, digest)
        body = mcp_wasm(mcp, local_oci, arguments.oras, temporary)
        shutil.copytree(server, context, ignore=shutil.ignore_patterns(".git", "__pycache__", "*.pyc"))
        (context / "snapshot.json").write_bytes(snapshot_bytes)
        write_json(context / "unmanaged-plugins.lock.json", lock)
        shutil.move(str(local_oci), context / "local-oci")
        if not candidates:
            (context / "local-oci" / ".keep").touch()
        for version in ("1.0.0", "2.0.0"):
            target = context / "plugins" / "mcp-server" / version / "plugin.wasm"
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(body)
        (context / "Dockerfile").write_bytes(b"".join(lines))
        shim = context / "nonprod-oras"
        shim.write_text(SHIM, encoding="utf-8")
        shim.chmod(0o755)
        write_json(context / "nonprod-context.json", {
            "schemaVersion": 1,
            "snapshotPath": arguments.snapshot,
            "snapshotSha256": content_hash(snapshot_bytes),
            "gatewayVersion": snapshot["gatewayVersion"],
            "sourceCommit": snapshot["sourceCommit"],
            "managedKeys": sorted(managed),
            "removedLockKeys": removed,
            "remainingLockKeys": sorted(lock["plugins"]),
            "mcpSeed": {"version": component(mcp["version"]), "wasmSha256": content_hash(body),
                        "compatibilityVersions": ["1.0.0", "2.0.0"]},
            "candidateLayouts": candidates,
        })
        if output.exists():
            output.rmdir()
        os.replace(context, output)
    return output


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    command = commands.add_parser("prepare")
    for name in ("higress-root", "plugin-server-root", "snapshot", "snapshot-sha256", "out"):
        command.add_argument("--" + name, required=True)
    command.add_argument("--oras", default="oras")
    arguments = parser.parse_args()
    try:
        output = prepare(arguments)
    except (OSError, ValueError, EOFError, subprocess.SubprocessError, tarfile.TarError) as error:
        print("NONPROD_CONTEXT_FAILED: " + str(error), file=sys.stderr)
        return 1
    print("NONPROD_CONTEXT_OK: " + str(output / "nonprod-context.json"))
    return 0


if __name__ == "__main__":
    sys.exit(main())
