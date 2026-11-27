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
import hashlib
import json
from pathlib import Path


def read_uleb(data, offset):
    value = 0
    shift = 0
    while True:
        if offset >= len(data) or shift > 63:
            raise ValueError("invalid unsigned LEB128 value")
        byte = data[offset]
        offset += 1
        value |= (byte & 0x7F) << shift
        if byte & 0x80 == 0:
            return value, offset
        shift += 7


def read_name(data, offset):
    length, offset = read_uleb(data, offset)
    end = offset + length
    if end > len(data):
        raise ValueError("truncated WebAssembly name")
    return data[offset:end].decode("utf-8"), end


def read_limits(data, offset):
    flags, offset = read_uleb(data, offset)
    minimum, offset = read_uleb(data, offset)
    maximum = None
    if flags & 1:
        maximum, offset = read_uleb(data, offset)
    return {"min": minimum, "max": maximum}, offset


def parse_imports(data):
    if len(data) < 8 or data[:4] != b"\x00asm" or data[4:8] != b"\x01\x00\x00\x00":
        raise ValueError("not a WebAssembly 1.0 binary")
    offset = 8
    imports = []
    while offset < len(data):
        section_id = data[offset]
        offset += 1
        section_size, offset = read_uleb(data, offset)
        section_end = offset + section_size
        if section_end > len(data):
            raise ValueError("truncated WebAssembly section")
        if section_id != 2:
            offset = section_end
            continue
        count, offset = read_uleb(data, offset)
        for _ in range(count):
            module, offset = read_name(data, offset)
            name, offset = read_name(data, offset)
            kind = data[offset]
            offset += 1
            if kind == 0:
                descriptor, offset = read_uleb(data, offset)
                kind_name = "function"
            elif kind == 1:
                offset += 1
                descriptor, offset = read_limits(data, offset)
                kind_name = "table"
            elif kind == 2:
                descriptor, offset = read_limits(data, offset)
                kind_name = "memory"
            elif kind == 3:
                descriptor = {"value_type": data[offset], "mutable": data[offset + 1]}
                offset += 2
                kind_name = "global"
            elif kind == 4:
                attribute = data[offset]
                type_index, offset = read_uleb(data, offset + 1)
                descriptor = {"attribute": attribute, "type_index": type_index}
                kind_name = "tag"
            else:
                raise ValueError(f"unknown WebAssembly import kind {kind}")
            imports.append({"module": module, "name": name, "kind": kind_name, "descriptor": descriptor})
        if offset != section_end:
            raise ValueError("WebAssembly import section length mismatch")
    return imports


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("wasm", type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    data = args.wasm.read_bytes()
    if not data:
        raise ValueError("empty WebAssembly binary")
    result = {
        "sha256": hashlib.sha256(data).hexdigest(),
        "size": len(data),
        "imports": parse_imports(data),
    }
    args.output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")


if __name__ == "__main__":
    main()
