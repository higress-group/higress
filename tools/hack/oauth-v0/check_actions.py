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
import shlex
import sys
from pathlib import Path


def main():
    directory = Path(sys.argv[1])
    variant = directory.name
    if variant not in {"candidate_eh", "candidate_eh_nodiag", "oauth"}:
        raise SystemExit("Unexpected test-only EH variant")
    graph = json.loads((directory / "actions.json").read_text())
    targets = {str(target["id"]): target["label"] for target in graph["targets"]}

    def expand(arguments):
        expanded = []
        for argument in arguments:
            if argument.startswith("@"):
                parameter = directory.parent / "params" / argument[1:]
                if parameter.is_file():
                    expanded.extend(expand(shlex.split(parameter.read_text())))
                else:
                    raise RuntimeError("Missing executed response file: " + argument)
            else:
                expanded.append(argument)
        return expanded

    selected = []
    for action in graph["actions"]:
        label = targets.get(str(action.get("targetId")), "")
        if not label.endswith(":proxy_wasm_" + variant) or action["mnemonic"] not in {"CppCompile", "CppLink"}:
            continue
        arguments = expand(action.get("arguments", []))
        if label.endswith(":proxy_wasm_" + variant) and action["mnemonic"] in {"CppCompile", "CppLink"}:
            selected.append({"label": label, "mnemonic": action["mnemonic"], "raw_arguments": action.get("arguments", []), "param_files": action.get("paramFiles", []), "arguments": arguments, "wasm_exceptions": "-fwasm-exceptions" in arguments})
    source = "extensions/oauth/plugin.cc" if variant == "oauth" else "oauth_parse_probe/probe.cc"
    compile_actions = [action for action in selected if action["mnemonic"] == "CppCompile" and any(source in argument for argument in action["arguments"])]
    link_actions = [action for action in selected if action["mnemonic"] == "CppLink"]
    executed = []
    decoder = json.JSONDecoder()
    text = (directory / "execution.json").read_text()
    cursor = 0
    while cursor < len(text):
        while cursor < len(text) and text[cursor].isspace():
            cursor += 1
        if cursor == len(text):
            break
        record, cursor = decoder.raw_decode(text, cursor)
        if any(("extensions/oauth" if variant == "oauth" else "oauth_parse_probe") in argument and variant in argument for argument in record.get("commandArgs", [])):
            arguments = expand(record.get("commandArgs", []))
            executed.append({"mnemonic": record.get("mnemonic"), "exitCode": record.get("exitCode", 0), "raw_arguments": record.get("commandArgs", []), "arguments": arguments, "wasm_exceptions": "-fwasm-exceptions" in arguments, "no_exceptions": "-fno-exceptions" in arguments})
    executed_compile = [action for action in executed if action["mnemonic"] == "CppCompile"]
    executed_link = [action for action in executed if action["mnemonic"] == "CppLink"]
    passed = bool(compile_actions and link_actions and executed_compile and executed_link) and all(action["wasm_exceptions"] for action in compile_actions + link_actions) and all(action["exitCode"] == 0 and action["wasm_exceptions"] and not action["no_exceptions"] for action in executed_compile + executed_link)
    diagnostic_settings = ["-sASSERTIONS=0", "-sEXCEPTION_STACK_TRACES=0"]
    diagnostic_settings_ok = all(all(setting in action["arguments"] for setting in diagnostic_settings) if variant.endswith("_nodiag") or variant == "oauth" else all(setting not in action["arguments"] for setting in diagnostic_settings) for action in link_actions + executed_link)
    link_o0 = bool(executed_link) and all({argument for argument in action["arguments"] if argument.startswith("-O")} == {"-O0"} for action in link_actions + executed_link)
    passed = passed and diagnostic_settings_ok and link_o0
    (directory / "flags-summary.json").write_text(json.dumps({"passed": passed, "variant": variant, "diagnostic_settings_ok": diagnostic_settings_ok, "link_o0": link_o0, "required_compile_actions": len(compile_actions), "link_actions": len(link_actions), "actions": selected, "executed_actions": executed, "boundary": "Official OAuth plugin.cc production entry." if variant == "oauth" else "Parser and local catch are inline in probe.cc; no arbitrary cross-library C++ unwinding claim."}, indent=2) + "\n")
    if not passed and variant != "oauth":
        raise SystemExit("Required actual probe compile/link actions lack -fwasm-exceptions")


if __name__ == "__main__":
    main()
