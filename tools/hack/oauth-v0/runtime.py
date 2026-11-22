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
import os
import re
import shutil
import subprocess
import time
import urllib.error
import urllib.request
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / "out/oauth-v0"
GATEWAY = "higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway@sha256:5d0618399fa98b9f4ad3c34b58f084b51624e3cc4eccdde72554b636d16b5da4"
HTTPBIN = "docker.io/mccutchen/go-httpbin@sha256:d13433bdac3654c43ff1bef441f38973bfe13513a6f9fbe843a2a49c1787d41e"


def execute(arguments, output, environment=None):
    with output.open("wb") as stream:
        result = subprocess.run(arguments, stdout=stream, stderr=subprocess.STDOUT, env=environment, timeout=180)
    return result.returncode


def request(path, token=None, case_name=None):
    headers = {}
    if token is not None:
        headers = {"Authorization": "Bearer " + token, "X-Oauth-Probe-Case": case_name, "X-Request-ID": case_name}
    port = 10000 if token is not None else 9901
    try:
        response = urllib.request.urlopen(urllib.request.Request("http://127.0.0.1:" + str(port) + path, headers=headers), timeout=4)
    except urllib.error.HTTPError as error:
        response = error
    except (urllib.error.URLError, TimeoutError, ConnectionError) as error:
        return {"status": 0, "headers": {}, "body": b"", "error": str(error)}
    with response:
        return {"status": response.status, "headers": dict(response.headers.items()), "body": response.read(), "error": None}


def upstream_count():
    response = request("/stats?filter=cluster.httpbin.upstream_rq_total&format=json")
    if response["status"] != 200:
        raise RuntimeError("upstream counter unavailable: " + str(response))
    stats = json.loads(response["body"])["stats"]
    return sum(stat["value"] for stat in stats if stat["name"] == "cluster.httpbin.upstream_rq_total")


def record_response(directory, name, response):
    (directory / (name + ".body")).write_bytes(response["body"])
    metadata = {key: value for key, value in response.items() if key != "body"}
    metadata["body_sha256"] = hashlib.sha256(response["body"]).hexdigest()
    (directory / (name + ".json")).write_text(json.dumps(metadata, indent=2) + "\n")


def run_variant(variant, cases):
    directory = OUT / "runtime" / variant
    directory.mkdir(parents=True, exist_ok=True)
    module = OUT / variant / "plugin.wasm"
    result = {"variant": variant, "module_present": module.is_file(), "loaded": False, "rows": [], "errors": [], "cleanup": False}
    if not module.is_file():
        result["errors"].append("module not produced; runtime not-run")
        return result
    harness = directory / "harness"
    harness.mkdir(exist_ok=True)
    shutil.copyfile(module, harness / "plugin.wasm")
    compose_text = (ROOT / "docs/developers/wasm-runtime-verification/compose.yaml").read_text().replace("internal: true", "internal: false")
    (harness / "compose.yaml").write_text(compose_text)
    shutil.copyfile(ROOT / "docs/developers/wasm-runtime-verification/envoy.yaml", harness / "envoy.yaml")
    environment = dict(os.environ, HIGRESS_GATEWAY_IMAGE=GATEWAY, HTTPBIN_IMAGE=HTTPBIN, WASM_PATH="./plugin.wasm", ENVOY_PORT="10000", ENVOY_ADMIN_PORT="9901")
    project = "oauth-v0-" + os.environ["GITHUB_RUN_ID"] + "-" + variant.replace("_", "-")
    compose = ["docker", "compose", "--project-name", project, "--file", str(harness / "compose.yaml")]
    result["module_sha256"] = hashlib.sha256(module.read_bytes()).hexdigest()
    result["project"] = project
    try:
        if execute(compose + ["config"], directory / "compose-config.yaml", environment) != 0:
            raise RuntimeError("compose config failed")
        if execute(compose + ["up", "--detach"], directory / "up.log", environment) != 0:
            raise RuntimeError("compose up failed")
        for _ in range(45):
            ready = request("/ready")
            if ready["status"] == 200:
                result["loaded"] = True
                break
            time.sleep(1)
        record_response(directory, "ready", ready)
        if not result["loaded"]:
            raise RuntimeError("current pinned Envoy engine did not load/ready the module")
        if execute(compose + ["exec", "-T", "envoy", "sha256sum", "/etc/envoy/main.wasm"], directory / "mounted-wasm.sha256", environment) != 0:
            raise RuntimeError("mounted module hash unavailable")
        if (directory / "mounted-wasm.sha256").read_text().split()[0] != result["module_sha256"]:
            raise RuntimeError("mounted bytes differ from final build artifact")
        control = cases[0]
        for repeat in (1, 2):
            for case in cases:
                name = str(repeat) + "-" + case["name"]
                before = upstream_count()
                response = request("/anything/" + name, case["token"], name)
                record_response(directory, name, response)
                after = upstream_count()
                expected_valid = case["expected"] == "valid"
                marker = {key.lower(): value for key, value in response["headers"].items()}.get("x-oauth-probe-result")
                passed = response["status"] == (200 if expected_valid else 401) and after - before == (1 if expected_valid else 0)
                if not expected_valid:
                    passed = passed and marker == case["expected"] and response["body"].startswith(("probe:" + case["expected"] + ":").encode())
                row = {"name": case["name"], "repeat": repeat, "expected": case["expected"], "status": response["status"], "marker": marker, "upstream_before": before, "upstream_after": after, "passed": passed}
                result["rows"].append(row)
                if not expected_valid:
                    followup = request("/anything/" + name + "-valid-after", control["token"], name + "-valid-after")
                    record_response(directory, name + "-valid-after", followup)
                    recovered = upstream_count()
                    row["following_valid_status"] = followup["status"]
                    row["following_valid_upstream_after"] = recovered
                    row["following_valid_passed"] = followup["status"] == 200 and recovered - after == 1
            record_response(directory, str(repeat) + "-ready-after", request("/ready"))
        stats = request("/stats?usedonly")
        (directory / "stats.txt").write_bytes(stats["body"])
    except Exception as error:
        result["errors"].append(str(error))
    finally:
        time.sleep(1)
        execute(compose + ["logs", "--no-color", "envoy", "httpbin"], directory / "envoy-upstream.log", environment)
        execute(compose + ["ps", "--all"], directory / "containers-before-cleanup.txt", environment)
        down_status = execute(compose + ["down", "--volumes", "--remove-orphans"], directory / "cleanup.log", environment)
        execute(compose + ["ps", "--all", "--quiet"], directory / "containers-after-cleanup.txt", environment)
        ports_closed = request("/ready")["status"] == 0 and request("/anything/cleanup", "cleanup", "cleanup")["status"] == 0
        result["cleanup"] = down_status == 0 and not (directory / "containers-after-cleanup.txt").read_text().strip() and ports_closed
        log = (directory / "envoy-upstream.log").read_text(errors="replace")
        result["trap_or_vm_failure"] = bool(re.search(r"wasm trap|wasm vm failed|wasm vm now failed|unreachable|Function:.*failed", log, re.IGNORECASE))
        result["catch_log_count"] = len(re.findall(r"OAUTH_V0 case=.*outcome=caught", log))
        result["rejected_log_count"] = len(re.findall(r"OAUTH_V0 case=.*outcome=rejected", log))
    return result


def main():
    runtime = OUT / "runtime"
    runtime.mkdir(exist_ok=True)
    cases = json.loads((OUT / "fixtures/cases.json").read_text())
    verdict = {"head": os.environ["EXPECTED_HEAD"], "scope": "test-only-v0-not-production", "production_mechanism_selected": False, "question_answered": False, "verdict": "PARTIAL", "variants": []}
    try:
        for image, name in ((GATEWAY, "gateway"), (HTTPBIN, "upstream")):
            if execute(["docker", "pull", image], runtime / (name + "-pull.log")) != 0:
                raise RuntimeError("pinned image unavailable: " + name)
            execute(["docker", "image", "inspect", image], runtime / (name + "-inspect.json"))
        for variant in ("default_disabled", "candidate_eh"):
            result = run_variant(variant, cases)
            verdict["variants"].append(result)
            (runtime / (variant + "-results.json")).write_text(json.dumps(result, indent=2) + "\n")
        candidate = verdict["variants"][1]
        flags_path = OUT / "candidate_eh/flags-summary.json"
        flags_ok = flags_path.exists() and json.loads(flags_path.read_text())["passed"]
        native_path = OUT / "native/run.exit"
        native_ok = native_path.exists() and native_path.read_text().strip() == "0"
        required_catches = sum(case["expected"] == "caught" for case in cases) * 2
        rows_ok = len(candidate["rows"]) == len(cases) * 2 and all(row["passed"] and row.get("following_valid_passed", True) for row in candidate["rows"])
        runtime_ok = candidate["loaded"] and rows_ok and not candidate["errors"] and not candidate["trap_or_vm_failure"] and candidate["catch_log_count"] >= required_catches and candidate["cleanup"]
        verdict["flags_passed"] = flags_ok
        verdict["native_passed"] = native_ok
        verdict["candidate_runtime_passed"] = runtime_ok
        verdict["required_catches"] = required_catches
        if runtime_ok and flags_ok and native_ok:
            verdict["verdict"] = "VALIDATED"
        elif not candidate["module_present"] or (flags_ok and not runtime_ok):
            verdict["verdict"] = "INVALIDATED"
        verdict["limitations"] = "Test-only parser/catch TU, current pinned runtime and finite corpus only; not a production OAuth fix or mechanism selection."
    except Exception as error:
        verdict["harness_error"] = str(error)
    (OUT / "verdict.json").write_text(json.dumps(verdict, indent=2) + "\n")
    print(json.dumps({key: value for key, value in verdict.items() if key != "variants"}, indent=2))


if __name__ == "__main__":
    main()
