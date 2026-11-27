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

import hashlib
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
from cpp201_wasm_imports import parse_imports, read_name, read_uleb


def main():
    data = Path(sys.argv[1]).read_bytes()
    result = {"sha256": hashlib.sha256(data).hexdigest(), "size": len(data), "imports": parse_imports(data), "sections": [], "target_features": []}
    offset = 8
    while offset < len(data):
        section_id = data[offset]
        size, offset = read_uleb(data, offset + 1)
        end = offset + size
        result["sections"].append({"id": section_id, "size": size})
        if section_id == 0:
            name, cursor = read_name(data, offset)
            if name == "target_features":
                count, cursor = read_uleb(data, cursor)
                for _ in range(count):
                    prefix = chr(data[cursor])
                    feature, cursor = read_name(data, cursor + 1)
                    result["target_features"].append({"prefix": prefix, "name": feature})
        offset = end
    Path(sys.argv[2]).write_text(json.dumps(result, indent=2) + "\n")


if __name__ == "__main__":
    main()
