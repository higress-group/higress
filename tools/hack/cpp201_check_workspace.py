#!/usr/bin/env python3
# Copyright (c) 2026 Alibaba Group Holding Ltd.
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

import argparse
import re
from pathlib import Path


SOURCES = (
    "basic_auth",
    "bot_detect",
    "custom_response",
    "hmac_auth",
    "jwt_auth",
    "key_auth",
    "key_rate_limit",
    "oauth",
    "request_block",
)


def require(condition, message):
    if not condition:
        raise ValueError(message)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    args = parser.parse_args()
    root = args.root.resolve()
    cpp = root / "plugins/wasm-cpp"

    builder = (cpp / "Dockerfile").read_text().splitlines()[0]
    require(re.search(r"^ARG BUILDER=\S+@sha256:[0-9a-f]{64}$", builder) is not None, "builder digest is missing")
    require((cpp / ".bazelversion").read_text().strip() == "6.0.0", "unexpected Bazel version")
    for source in SOURCES:
        version_file = cpp / "extensions" / source / "VERSION"
        require(version_file.is_file(), f"missing VERSION for {source}")
        require(version_file.read_text().strip() == "2.0.1", f"wrong VERSION for {source}")

    workflow = (root / ".github/workflows/wasm-cpp-2.0.1-verification.yaml").read_text()
    matrix_section = workflow.split("        source:\n", 1)[1].split("    env:\n", 1)[0]
    matrix_sources = re.findall(r"^\s+- ([a-z_]+)\s*$", matrix_section, re.MULTILINE)
    require(matrix_sources == list(SOURCES), f"matrix is not exactly nine: {matrix_sources}")

    workspace_files = [cpp / "WORKSPACE", *sorted((cpp / "bazel").glob("*.bzl"))]
    archive_count = 0
    for workspace_file in workspace_files:
        text = workspace_file.read_text()
        require("{version}" not in text, f"unresolved archive URL in {workspace_file}")
        for match in re.finditer(r"http_archive\(\n(.*?)\n\s*\)", text, re.DOTALL):
            block = match.group(1)
            name_match = re.search(r'name = "([^"]+)"', block)
            require(name_match is not None, f"unnamed archive in {workspace_file}")
            require(re.search(r'sha256\s*=\s*(?:"|[A-Z][A-Z0-9_]*\s*,)', block) is not None, f"archive {name_match.group(1)} has no SHA-256")
            require("latest" not in block, f"archive {name_match.group(1)} uses latest")
            archive_count += 1
    require(archive_count >= 12, f"incomplete archive scan: {archive_count}")

    pins = (root / "tools/hack/cpp201-runtime-images.pins").read_text().splitlines()
    kind_count = 0
    total_count = 0
    for line in pins:
        if not line or line.startswith("#"):
            continue
        fields = line.split("|")
        require(len(fields) == 3, f"invalid image pin: {line}")
        local_ref, pinned_ref, load_kind = fields
        require("latest" not in local_ref and "latest" not in pinned_ref, f"mutable image pin: {line}")
        require(re.search(r"@sha256:[0-9a-f]{64}$", pinned_ref) is not None, f"image digest missing: {line}")
        require(load_kind in {"true", "false"}, f"invalid kind selector: {line}")
        kind_count += load_kind == "true"
        total_count += 1
    require(total_count == 16 and kind_count == 15, "runtime image inventory mismatch")

    runtime_script = (root / "tools/hack/cpp201-run-e2e.sh").read_text()
    for pinned_image in (
        "docker.io/bitinit/eureka:cpp201-pinned",
        "llm-mock-server:cpp201-pinned",
        "docker.io/curlimages/curl:cpp201-pinned",
    ):
        require(pinned_image in runtime_script, f"missing runtime fixture override: {pinned_image}")
    require("fixture-overrides.diff" in runtime_script, "runtime fixture overrides are not recorded")
    basic_manifest = (root / "test/e2e/conformance/tests/cpp-wasm-basic-auth.yaml").read_text()
    require("file:///opt/plugins/wasm-cpp/extensions/basic_auth/plugin.wasm" in basic_manifest, "basic_auth staging path mismatch")


if __name__ == "__main__":
    main()
