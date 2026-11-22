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

import json
import sys
from pathlib import Path


def main():
    directory = Path(sys.argv[1])
    graph = json.loads((directory / "actions.json").read_text())
    targets = {str(target["id"]): target["label"] for target in graph["targets"]}
    selected = []
    for action in graph["actions"]:
        label = targets.get(str(action.get("targetId")), "")
        arguments = list(action.get("arguments", []))
        for parameter in action.get("paramFiles", []):
            arguments.extend(parameter.get("arguments", []))
        if label.endswith(":proxy_wasm_candidate_eh") and action["mnemonic"] in {"CppCompile", "CppLink"}:
            selected.append({"label": label, "mnemonic": action["mnemonic"], "arguments": arguments, "wasm_exceptions": "-fwasm-exceptions" in arguments})
    compile_actions = [action for action in selected if action["mnemonic"] == "CppCompile" and any("oauth_parse_probe/probe.cc" in argument for argument in action["arguments"])]
    link_actions = [action for action in selected if action["mnemonic"] == "CppLink"]
    passed = bool(compile_actions and link_actions) and all(action["wasm_exceptions"] for action in compile_actions + link_actions)
    (directory / "flags-summary.json").write_text(json.dumps({"passed": passed, "required_compile_actions": len(compile_actions), "link_actions": len(link_actions), "actions": selected, "boundary": "Parser and local catch are inline in probe.cc; no arbitrary cross-library C++ unwinding claim."}, indent=2) + "\n")
    if not passed:
        raise SystemExit("Required actual probe compile/link actions lack -fwasm-exceptions")


if __name__ == "__main__":
    main()
