#!/usr/bin/env python3
import argparse
import json
import re
from pathlib import Path


FIXED_SOURCES = {
    "basic-auth": "basic_auth",
    "bot-detect": "bot_detect",
    "custom-response": "custom_response",
    "hmac-auth": "hmac_auth",
    "jwt-auth": "jwt_auth",
    "key-auth": "key_auth",
    "key-rate-limit": "key_rate_limit",
    "oauth": "oauth",
    "request-block": "request_block",
}
EXCLUDED_SOURCES = {"model_mapper", "model_router", "sni_misdirect"}


def matrix(root):
    catalog = json.loads((root / "plugins/release/catalog.json").read_text())
    plugins = [entry for entry in catalog["plugins"] if entry["implementation"] == "cpp"]
    if len(plugins) != 9 or {entry["logicalId"] for entry in plugins} != set(FIXED_SOURCES):
        raise ValueError("catalog must classify exactly the fixed nine C++ plugins")
    extensions = root / "plugins/wasm-cpp/extensions"
    directories = {entry.name for entry in extensions.iterdir() if entry.is_dir()}
    if directories != set(FIXED_SOURCES.values()) | EXCLUDED_SOURCES:
        raise ValueError("unknown or missing classified C++ directory")
    if any(entry.is_symlink() for entry in extensions.iterdir()):
        raise ValueError("symlink C++ extensions are forbidden")
    dockerfile = (root / "plugins/wasm-cpp/Dockerfile").read_text()
    builders = re.findall(r"^ARG BUILDER=(.+)$", dockerfile, re.MULTILINE)
    if len(builders) != 1 or not re.fullmatch(r"[a-z0-9./:_-]+@sha256:[0-9a-f]{64}", builders[0]):
        raise ValueError("builder must remain digest pinned")
    bazel = (root / "plugins/wasm-cpp/.bazelversion").read_text().strip()
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", bazel):
        raise ValueError("Bazel must remain version pinned")
    entries = []
    for plugin in sorted(plugins, key=lambda item: item["logicalId"]):
        source = FIXED_SOURCES[plugin["logicalId"]]
        source_dir = "plugins/wasm-cpp/extensions/" + source
        if plugin["sourceDir"] != source_dir or plugin["image"] != "plugins/" + plugin["logicalId"]:
            raise ValueError("substituted C++ source/image identity")
        if plugin["artifactInputs"] != [source_dir + "/**"] or plugin["sharedInputGroups"] != ["cpp-workspace"]:
            raise ValueError("substituted C++ artifact input selection")
        version = (root / source_dir / "VERSION").read_text().strip()
        if not re.fullmatch(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?", version):
            raise ValueError("invalid C++ VERSION")
        entries.append({"logicalId": plugin["logicalId"], "source": source, "sourceDir": source_dir, "version": version, "builder": builders[0], "bazelVersion": bazel})
    return entries


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    args = parser.parse_args()
    print(json.dumps(matrix(args.root), separators=(",", ":"), sort_keys=True))


if __name__ == "__main__":
    main()
