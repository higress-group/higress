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
OUT = ROOT / ("out/oauth-" + os.environ.get("OAUTH_PROBE_VERSION", "v0"))
GATEWAY = "higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway@sha256:5d0618399fa98b9f4ad3c34b58f084b51624e3cc4eccdde72554b636d16b5da4"
HTTPBIN = "docker.io/mccutchen/go-httpbin@sha256:d13433bdac3654c43ff1bef441f38973bfe13513a6f9fbe843a2a49c1787d41e"


def execute(arguments, output, environment=None):
    with output.open("wb") as stream:
        try:
            result = subprocess.run(arguments, stdout=stream, stderr=subprocess.STDOUT, env=environment, timeout=180)
        except (subprocess.TimeoutExpired, OSError) as error:
            stream.write((str(error) + "\n").encode())
            return 124 if isinstance(error, subprocess.TimeoutExpired) else 127
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


def upstream_count(directory=None, name=None):
    response = request("/stats?filter=cluster.httpbin.upstream_rq_total&format=json")
    if directory is not None:
        record_response(directory, name, response)
    if response["status"] != 200:
        raise RuntimeError("upstream counter unavailable: " + str(response))
    stats = json.loads(response["body"])["stats"]
    return sum(stat["value"] for stat in stats if stat["name"] == "cluster.httpbin.upstream_rq_total")


def record_response(directory, name, response):
    (directory / (name + ".body")).write_bytes(response["body"])
    metadata = {key: value for key, value in response.items() if key != "body"}
    metadata["body_sha256"] = hashlib.sha256(response["body"]).hexdigest()
    (directory / (name + ".json")).write_text(json.dumps(metadata, indent=2) + "\n")


def production_echo(response, token, strip_token=False):
    if response["status"] != 200:
        return False
    try:
        headers = {name.lower(): value for name, value in json.loads(response["body"])["headers"].items()}
        return headers.get("x-mse-consumer") == ["probe"] and (not headers.get("authorization") if strip_token else headers.get("authorization") == ["Bearer " + token])
    except (KeyError, ValueError, TypeError):
        return False


def run_variant(variant, cases):
    production = os.environ.get("OAUTH_PROBE_VERSION") == "production"
    directory = OUT / "runtime" / variant
    directory.mkdir(parents=True, exist_ok=True)
    module = OUT / variant / "plugin.wasm"
    result = {"variant": variant, "module_present": module.is_file(), "loaded": False, "rows": [], "generations": [], "ready_after": [], "errors": [], "cleanup": False}
    if not module.is_file():
        result["errors"].append("module not produced; runtime not-run")
        return result
    harness = directory / "harness"
    harness.mkdir(exist_ok=True)
    shutil.copyfile(module, harness / "plugin.wasm")
    compose_text = (ROOT / "docs/developers/wasm-runtime-verification/compose.yaml").read_text().replace("internal: true", "internal: false")
    (harness / "compose.yaml").write_text(compose_text)
    shutil.copyfile(ROOT / "docs/developers/wasm-runtime-verification/envoy.yaml", harness / "envoy.yaml")
    if production:
        configuration = json.loads((OUT / "fixtures/config.json").read_text())
        text = (harness / "envoy.yaml").read_text().replace('value: "{}"', "value: '" + json.dumps(configuration, sort_keys=True) + "'")
        text = text.replace("FLAGS=%RESPONSE_FLAGS%", "DETAILS=%RESPONSE_CODE_DETAILS% FLAGS=%RESPONSE_FLAGS%")
        text = text.replace("                        - match:", "                        - name: v0\n                          match:")
        routes = "".join("                        - name: " + name + "\n                          match:\n                            prefix: " + prefix + "\n                          route:\n                            cluster: httpbin\n" + ("                            prefix_rewrite: /anything/\n" if name == "v0-strip" else "") for name, prefix in (("forbidden", "/forbidden/"), ("v0-strip", "/strip/")))
        text = text.replace("                      routes:\n", "                      routes:\n" + routes)
        (harness / "envoy.yaml").write_text(text)
    environment = dict(os.environ, HIGRESS_GATEWAY_IMAGE=GATEWAY, HTTPBIN_IMAGE=HTTPBIN, WASM_PATH="./plugin.wasm", ENVOY_PORT="10000", ENVOY_ADMIN_PORT="9901")
    project = "oauth-" + os.environ.get("OAUTH_PROBE_VERSION", "v0") + "-" + os.environ["GITHUB_RUN_ID"] + "-" + variant.replace("_", "-")
    compose = ["docker", "compose", "--project-name", project, "--file", str(harness / "compose.yaml")]
    result["module_sha256"] = hashlib.sha256(module.read_bytes()).hexdigest()
    result["host_sha256"] = hashlib.sha256((harness / "plugin.wasm").read_bytes()).hexdigest()
    result["config_sha256"] = hashlib.sha256((harness / "envoy.yaml").read_bytes()).hexdigest()
    result["fixture_sha256"] = hashlib.sha256((OUT / "fixtures/cases.json").read_bytes()).hexdigest()
    result["project"] = project
    try:
        if execute(compose + ["config"], directory / "compose-config.yaml", environment) != 0:
            raise RuntimeError("compose config failed")
        if execute(compose + ["up", "--detach"], directory / "up.log", environment) != 0:
            raise RuntimeError("compose up failed")
        copy_status = execute(compose + ["cp", "envoy:/etc/envoy/main.wasm", str(directory / "container-plugin.wasm")], directory / "container-copy.log", environment)
        result["container_copy_exit"] = copy_status
        if copy_status != 0:
            raise RuntimeError("container module bytes unavailable")
        result["container_sha256"] = hashlib.sha256((directory / "container-plugin.wasm").read_bytes()).hexdigest()
        result["artifact_host_container_equal"] = result["module_sha256"] == result["host_sha256"] == result["container_sha256"]
        if not result["artifact_host_container_equal"]:
            raise RuntimeError("artifact/host/container bytes differ")
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
                before = upstream_count(directory, name + "-counter-before")
                path = case.get("prefix", "/anything/") + name
                response = request(path, case["token"], name)
                record_response(directory, name, response)
                after = upstream_count(directory, name + "-counter-after")
                expected_valid = case["expected"] == "valid"
                marker = {key.lower(): value for key, value in response["headers"].items()}.get("x-oauth-probe-result")
                expected_status = 403 if case["expected"] == "forbidden" else 200 if expected_valid else 401
                passed = response["status"] == expected_status and after - before == (1 if expected_valid else 0)
                if production:
                    if expected_valid:
                        passed = passed and production_echo(response, case["token"], case.get("strip_token", False))
                    elif expected_status == 401:
                        headers = {key.lower(): value for key, value in response["headers"].items()}
                        passed = passed and response["body"] == b"Invalid Jwt token" and headers.get("www-authenticate") == 'Bearer realm="http://127.0.0.1:10000' + path + '"'
                    else:
                        passed = passed and response["body"] == b"Access Denied" and "www-authenticate" not in {key.lower() for key in response["headers"]}
                elif not expected_valid:
                    passed = passed and marker == case["expected"] and response["body"].startswith(("probe:" + case["expected"] + ":").encode())
                row = {"name": case["name"], "repeat": repeat, "case_id": name, "request_path": path, "expected": case["expected"], "status": response["status"], "marker": marker, "upstream_before": before, "upstream_after": after, "passed": passed}
                result["rows"].append(row)
                if not expected_valid:
                    followup = request("/anything/" + name + "-valid-after", control["token"], name + "-valid-after")
                    record_response(directory, name + "-valid-after", followup)
                    recovered = upstream_count(directory, name + "-counter-valid-after")
                    row["following_valid_status"] = followup["status"]
                    row["following_valid_upstream_after"] = recovered
                    row["following_valid_passed"] = followup["status"] == 200 and recovered - after == 1
                    if production:
                        row["following_valid_passed"] = row["following_valid_passed"] and production_echo(followup, control["token"])
            if production:
                generation_name = str(repeat) + "-token-generation"
                generation_before = upstream_count(directory, generation_name + "-before")
                generated = request("/oauth2/token?grant_type=client_credentials&client_id=probe-client&client_secret=oauth-v0-test-key", "", generation_name)
                record_response(directory, generation_name, generated)
                generation_after = upstream_count(directory, generation_name + "-after")
                generated_token = json.loads(generated["body"]).get("access_token", "") if generated["status"] == 200 else ""
                generated_valid = request("/anything/" + generation_name + "-valid", generated_token, generation_name + "-valid")
                record_response(directory, generation_name + "-valid", generated_valid)
                generation_recovered = upstream_count(directory, generation_name + "-valid-after")
                result["generations"].append({"repeat": repeat, "status": generated["status"], "upstream_delta": generation_after - generation_before, "following_valid_status": generated_valid["status"], "following_valid_delta": generation_recovered - generation_after, "passed": generated["status"] == 200 and bool(generated_token) and generation_after == generation_before and generation_recovered - generation_after == 1 and production_echo(generated_valid, generated_token)})
            ready_after = request("/ready")
            record_response(directory, str(repeat) + "-ready-after", ready_after)
            result["ready_after"].append(ready_after["status"])
        stats = request("/stats?usedonly")
        (directory / "stats.txt").write_bytes(stats["body"])
    except Exception as error:
        result["errors"].append(str(error))
    finally:
        time.sleep(1)
        cleanup_exits = {}
        if production:
            cleanup_exits["stop"] = execute(compose + ["stop", "--timeout", "10"], directory / "stop.log", environment)
        cleanup_exits["logs"] = execute(compose + ["logs", "--no-color", "envoy", "httpbin"], directory / "envoy-upstream.log", environment)
        cleanup_exits["ps_before"] = execute(compose + ["ps", "--all"], directory / "containers-before-cleanup.txt", environment)
        cleanup_exits["down"] = execute(compose + ["down", "--volumes", "--remove-orphans"], directory / "cleanup.log", environment)
        cleanup_exits["compose_ps_after"] = execute(compose + ["ps", "--all", "--quiet"], directory / "containers-after-cleanup.txt", environment)
        cleanup_exits["docker_ps_after"] = execute(["docker", "ps", "--all", "--quiet", "--filter", "label=com.docker.compose.project=" + project], directory / "project-containers-after-cleanup.txt")
        cleanup_exits["networks_after"] = execute(["docker", "network", "ls", "--quiet", "--filter", "label=com.docker.compose.project=" + project], directory / "networks-after-cleanup.txt")
        cleanup_exits["volumes_after"] = execute(["docker", "volume", "ls", "--quiet", "--filter", "label=com.docker.compose.project=" + project], directory / "volumes-after-cleanup.txt")
        admin_closed = request("/ready")
        listener_closed = request("/anything/cleanup", "cleanup", "cleanup")
        record_response(directory, "cleanup-admin", admin_closed)
        record_response(directory, "cleanup-listener", listener_closed)
        result["ports_closed"] = admin_closed["status"] == 0 and listener_closed["status"] == 0
        result["cleanup_command_exits"] = cleanup_exits
        (directory / "cleanup-command-exits.json").write_text(json.dumps(cleanup_exits, indent=2) + "\n")
        result["cleanup"] = all(status == 0 for status in cleanup_exits.values()) and all(not (directory / name).read_text().strip() for name in ("containers-after-cleanup.txt", "project-containers-after-cleanup.txt", "networks-after-cleanup.txt", "volumes-after-cleanup.txt")) and result["ports_closed"]
        log = (directory / "envoy-upstream.log").read_text(errors="replace")
        result["trap_or_vm_failure"] = bool(re.search(r"wasm trap|wasm vm failed|wasm vm now failed|unreachable|Function:.*failed", log, re.IGNORECASE))
        result["catch_log_count"] = len(re.findall(r"OAUTH_V0 case=.*outcome=caught", log))
        result["rejected_log_count"] = len(re.findall(r"OAUTH_V0 case=.*outcome=rejected", log))
        for row in result["rows"]:
            if production:
                row["local_details_observed"] = row["expected"] == "valid" or any("PATH=" + row["request_path"] + " CODE=" + str(row["status"]) + " " in line and "DETAILS=via_wasm::local-verification-plugin::oauth_access_denied " in line for line in log.splitlines())
            row["probe_log_observed"] = "OAUTH_V0 case=" + row["case_id"] + " outcome=" + row["expected"] + " detail=" in log
            if row["expected"] != "valid":
                row["following_valid_log_observed"] = "OAUTH_V0 case=" + row["case_id"] + "-valid-after outcome=valid detail=" in log
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
