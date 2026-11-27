#!/usr/bin/env python3
"""Reconcile managed-history inventory and served Wasm against public OCI bytes."""

import argparse
import hashlib
import http.client
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
import tarfile
import tempfile
import urllib.error
import urllib.parse
import urllib.request


WASM_MEDIA_TYPE = "application/vnd.module.wasm.content.layer.v1+wasm"
TAR_MEDIA_TYPES = {
    "application/vnd.docker.image.rootfs.diff.tar.gzip",
    "application/vnd.oci.image.layer.v1.tar+gzip",
}


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON key: " + key)
        result[key] = value
    return result


def read_json(path):
    with open(path, encoding="utf-8") as source:
        value = json.load(source, object_pairs_hook=unique_object)
    if not isinstance(value, dict):
        raise ValueError("expected a JSON object: " + str(path))
    return value


def component(value, label):
    if not isinstance(value, str) or not re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}", value):
        raise ValueError("invalid " + label)
    return value


def sha256(value, label, prefix=""):
    if not isinstance(value, str) or not re.fullmatch(prefix + r"[0-9a-f]{64}", value):
        raise ValueError("invalid " + label)
    return value


def public_repository(artifact):
    reference = artifact.get("ociRef")
    if not isinstance(reference, str):
        raise ValueError("missing public ociRef")
    repository, separator, tag = reference.rpartition(":")
    if not separator or not repository or "@" in reference or tag != artifact["version"]:
        raise ValueError("public ociRef tag must equal version: " + reference)
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:/-]*", repository):
        raise ValueError("invalid public OCI repository: " + repository)
    return repository


def add_rows(summary, base_url, role, logical_id, paths, artifact, version_key="version"):
    version = component(artifact.get(version_key), "served version")
    digest = sha256(artifact.get("digest"), "manifest digest", "sha256:")
    expected = artifact.get("wasmSha256")
    if role != "managed-current" or "wasmSha256" in artifact:
        sha256(expected, "Wasm SHA-256 for " + logical_id)
    rows = []
    for path in paths:
        row = {
            "role": role,
            "logicalId": logical_id,
            "path": path,
            "version": version,
            "digest": digest,
            "expectedSha256": expected,
            "actualSha256": None,
            "status": "not-checked",
            "url": base_url + "/plugins/" + path + "/" + version + "/plugin.wasm",
        }
        rows.append(row)
        summary["results"].append(row)
        summary["counts"]["urls"] += 1
    return rows


def prepare(snapshot, lock, base_url, summary):
    plugins = snapshot.get("plugins")
    if not isinstance(plugins, list) or not isinstance(lock.get("plugins"), dict):
        raise ValueError("snapshot plugins must be a list and lock plugins an object")
    managed = []
    for entry in plugins:
        if not isinstance(entry, dict) or not isinstance(entry.get("consumers", {}), dict):
            raise ValueError("invalid snapshot plugin entry")
        mapping = entry.get("consumers", {}).get("pluginServer")
        if not mapping:
            continue
        if not isinstance(mapping, dict):
            raise ValueError("pluginServer consumer mapping must be an object")
        history = entry.get("history", [])
        if not isinstance(history, list):
            raise ValueError("managed history must be a list")
        if not history:
            continue
        logical_id = component(entry.get("logicalId"), "logical id")
        aliases = mapping.get("aliases", [])
        if not isinstance(aliases, list):
            raise ValueError("managed aliases must be a list")
        paths = list(dict.fromkeys(component(path, "served path") for path in [
            mapping.get("httpPath"), mapping.get("inventoryKey"), *aliases,
        ]))
        artifacts = []
        summary["counts"]["managed"] += 1
        summary["counts"]["history"] += len(history)
        for index, artifact in enumerate([entry, *history]):
            if not isinstance(artifact, dict):
                raise ValueError("invalid managed artifact")
            role = "managed-history" if index else "managed-current"
            rows = add_rows(summary, base_url, role, logical_id, paths, artifact)
            artifacts.append((public_repository(artifact), rows))
        managed.append((paths, artifacts))
    for logical_id, entry in lock["plugins"].items():
        component(logical_id, "lock logical id")
        if not isinstance(entry, dict) or not isinstance(entry.get("history", []), list):
            raise ValueError("invalid unmanaged entry or history")
        path = component(entry.get("servedPath"), "unmanaged served path")
        summary["counts"]["unmanaged"] += 1
        for index, artifact in enumerate([entry, *entry.get("history", [])]):
            if not isinstance(artifact, dict):
                raise ValueError("invalid unmanaged artifact")
            role = "unmanaged-history" if index else "unmanaged-current"
            add_rows(summary, base_url, role, logical_id, [path], artifact, "servedVersion")
    seen = set()
    for row in summary["results"]:
        identity = (row["path"], row["version"])
        if identity in seen:
            row["status"] = "failed"
            raise ValueError("duplicate served path+version: " + row["url"])
        seen.add(identity)
    return managed


def resolve_wasm(oras, repository, digest):
    manifest = subprocess.run(
        [oras, "manifest", "fetch", repository + "@" + digest],
        check=True, capture_output=True, timeout=60,
    )
    document = json.loads(manifest.stdout, object_pairs_hook=unique_object)
    if not isinstance(document, dict) or not isinstance(document.get("layers"), list):
        raise ValueError("OCI manifest must contain layers")
    layers = document["layers"]
    if any(not isinstance(layer, dict) or not isinstance(layer.get("mediaType"), str) for layer in layers):
        raise ValueError("invalid OCI layer descriptor")
    supported = [layer for layer in layers if layer.get("mediaType") in TAR_MEDIA_TYPES | {WASM_MEDIA_TYPE}]
    if len(supported) != 1:
        raise ValueError("OCI manifest must contain exactly one supported Wasm layer")
    layer = supported[0]
    layer_digest = sha256(layer.get("digest"), "layer digest", "sha256:")
    with tempfile.TemporaryDirectory(prefix="snapshot-http-") as temporary:
        blob = Path(temporary) / "layer"
        subprocess.run(
            [oras, "blob", "fetch", repository + "@" + layer_digest, "-o", str(blob)],
            check=True, capture_output=True, timeout=60,
        )
        blob_bytes = blob.read_bytes()
        if hashlib.sha256(blob_bytes).hexdigest() != layer_digest.removeprefix("sha256:"):
            raise ValueError("OCI blob SHA-256 does not match layer digest")
        if layer["mediaType"] == WASM_MEDIA_TYPE:
            body = blob_bytes
        else:
            with tarfile.open(blob, "r:gz") as bundle:
                members = [member for member in bundle.getmembers() if member.name.endswith(".wasm")]
                if len(members) != 1:
                    raise ValueError("OCI tar layer must contain exactly one Wasm file")
                member = members[0]
                path = PurePosixPath(member.name)
                if not member.isfile() or path.is_absolute() or ".." in path.parts or "\\" in member.name:
                    raise ValueError("unsafe OCI tar Wasm member")
                with bundle.extractfile(member) as source:
                    body = source.read()
    if len(body) < 8 or not body.startswith(b"\0asm"):
        raise ValueError("OCI payload is not a complete Wasm header")
    return hashlib.sha256(body).hexdigest()


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, response, code, message, headers, new_url):
        return None


def verify_http(row, opener):
    row["status"] = "failed"
    try:
        with opener.open(row["url"], timeout=30) as response:
            row["httpStatus"] = response.status
            if response.status != 200:
                raise ValueError("non-200 HTTP response: " + row["url"])
            body = response.read()
            row["actualSha256"] = hashlib.sha256(body).hexdigest()
            length = response.headers.get("Content-Length")
            if length is not None and len(body) != int(length):
                raise ValueError("short or inconsistent HTTP body: " + row["url"])
    except urllib.error.HTTPError as error:
        row["httpStatus"] = error.code
        raise ValueError("HTTP %s: %s" % (error.code, row["url"])) from error
    except (OSError, urllib.error.URLError, http.client.HTTPException) as error:
        raise ValueError("HTTP read failed for %s: %s" % (row["url"], error)) from error
    if len(body) < 8 or not body.startswith(b"\0asm"):
        raise ValueError("HTTP body is not a complete Wasm header: " + row["url"])
    if row["actualSha256"] != row["expectedSha256"]:
        raise ValueError("HTTP Wasm SHA-256 mismatch: " + row["url"])
    row["status"] = "ok"


def verify(arguments, summary):
    parsed = urllib.parse.urlsplit(arguments.base_url)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc or parsed.query or parsed.fragment:
        raise ValueError("base URL must be an HTTP(S) URL without query or fragment")
    snapshot = read_json(arguments.snapshot)
    inventory = read_json(arguments.inventory)
    lock = read_json(arguments.lock)
    managed = prepare(snapshot, lock, arguments.base_url.rstrip("/"), summary)
    for paths, artifacts in managed:
        records = []
        for repository, rows in artifacts:
            try:
                computed = resolve_wasm(arguments.oras, repository, rows[0]["digest"])
                for row in rows:
                    if row["expectedSha256"] is not None and row["expectedSha256"] != computed:
                        raise ValueError("snapshot Wasm SHA-256 does not match OCI: " + row["url"])
                    row["expectedSha256"] = computed
            except (OSError, ValueError, EOFError, subprocess.SubprocessError, tarfile.TarError):
                for row in rows:
                    row["status"] = "failed"
                raise
            records.append({
                "version": rows[0]["version"], "digest": rows[0]["digest"], "wasmSha256": computed,
            })
        expected = dict(records[0], history=records[1:])
        for path in paths:
            if inventory.get(path) != expected:
                raise ValueError("snapshot inventory mismatch for managed path: " + path)
    opener = urllib.request.build_opener(NoRedirect())
    for row in summary["results"]:
        verify_http(row, opener)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("snapshot", "inventory", "lock", "base-url", "summary"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--oras", default=os.environ.get("ORAS") or "oras")
    arguments = parser.parse_args()
    if Path(arguments.summary).resolve() in {Path(path).resolve() for path in (
        arguments.snapshot, arguments.inventory, arguments.lock,
    )}:
        print("summary must not overwrite an input", file=sys.stderr)
        return 1
    summary = {"status": "failed", "counts": {"managed": 0, "history": 0, "unmanaged": 0, "urls": 0}, "results": []}
    try:
        verify(arguments, summary)
        summary["status"] = "ok"
    except (OSError, ValueError, EOFError, subprocess.SubprocessError, tarfile.TarError) as error:
        summary["error"] = str(error)
        print("MANAGED_HISTORY_HTTP_FAILED: " + str(error), file=sys.stderr)
    try:
        with open(arguments.summary, "w", encoding="utf-8") as output:
            json.dump(summary, output, indent=2)
            output.write("\n")
    except OSError as error:
        print("cannot write HTTP summary: " + str(error), file=sys.stderr)
        return 1
    if summary["status"] != "ok":
        return 1
    print("MANAGED_HISTORY_HTTP_OK managed={managed} history={history} unmanaged={unmanaged} urls={urls}".format(**summary["counts"]))
    return 0


if __name__ == "__main__":
    sys.exit(main())
