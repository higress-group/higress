#!/usr/bin/env python3

import json
import os
import xml.etree.ElementTree as xml

import runtime


def main():
    if os.environ.get("OAUTH_PROBE_VERSION") != "production":
        raise SystemExit("Production workflow mode required")
    output = runtime.OUT
    directory = output / "runtime"
    directory.mkdir(parents=True, exist_ok=True)
    verdict = {"head": os.environ["EXPECTED_HEAD"], "scope": "official-oauth-production-entry", "verdict": "PARTIAL", "proof": {}}
    try:
        cases = json.loads((output / "fixtures/cases.json").read_text())
        for image, name in ((runtime.GATEWAY, "gateway"), (runtime.HTTPBIN, "upstream")):
            if runtime.execute(["docker", "pull", image], directory / (name + "-pull.log")) != 0 or runtime.execute(["docker", "image", "inspect", image], directory / (name + "-inspect.json")) != 0:
                raise RuntimeError("Pinned image unavailable: " + name)
        result = runtime.run_variant("oauth", cases)
        (directory / "oauth-results.json").write_text(json.dumps(result, indent=2) + "\n")
        verdict["runtime"] = result
        native_runs = []
        for path in sorted((output / "native/testlogs").rglob("test.xml")):
            root = xml.parse(path).getroot()
            native_runs.append({"file": str(path.relative_to(output)), "tests": int(root.attrib.get("tests", 0)), "failures": int(root.attrib.get("failures", 0)), "errors": int(root.attrib.get("errors", 0)), "skipped": int(root.attrib.get("skipped", root.attrib.get("disabled", 0)))})
        verdict["native_runs"] = native_runs
        flags = json.loads((output / "oauth/flags-summary.json").read_text())
        module = json.loads((output / "oauth/module-info.json").read_text())
        verdict["module"] = module
        proof = verdict["proof"]
        proof["exact_head"] = (output / "source-head.txt").read_text().strip() == os.environ["EXPECTED_HEAD"]
        proof["actual_production_compile_link_eh_and_paired_settings"] = flags["passed"]
        proof["helper_absent_eh_tag_present"] = not any(entry["module"] == "env" and entry["name"] == "__throw_exception_with_stack_trace" for entry in module["imports"]) and any(section["id"] == 13 for section in module["sections"])
        proof["production_native_full_suite_twice"] = (output / "native/run.exit").read_text().strip() == "0" and len(native_runs) == 2 and all(run["tests"] > 0 and not run["failures"] and not run["errors"] and not run["skipped"] for run in native_runs)
        proof["artifact_host_container_identity"] = result.get("artifact_host_container_equal") is True and result.get("module_sha256") == module["sha256"]
        proof["two_complete_finite_rounds"] = [(row["repeat"], row["name"]) for row in result["rows"]] == [(repeat, case["name"]) for repeat in (1, 2) for case in cases]
        proof["exact_401_403_valid_headers_bodies_counters_details_and_recovery"] = bool(result["rows"]) and all(row["passed"] and row.get("local_details_observed") is True and (row["expected"] == "valid" or row.get("following_valid_passed") is True) for row in result["rows"])
        proof["production_token_generation_and_valid_control_twice"] = len(result["generations"]) == 2 and all(row["passed"] for row in result["generations"])
        proof["loaded_ready_after_both_rounds_no_trap"] = result["loaded"] and result["ready_after"] == [200, 200] and not result["errors"] and result.get("trap_or_vm_failure") is False
        proof["project_cleanup_exits_empty_resources_closed_ports"] = result["cleanup"] and result.get("ports_closed") is True and bool(result.get("cleanup_command_exits")) and all(status == 0 for status in result["cleanup_command_exits"].values())
        proof["builder_success"] = (output / "builder.exit").read_text().strip() == "0"
        verdict["verdict"] = "VALIDATED" if all(proof.values()) else "INVALIDATED"
    except Exception as error:
        verdict["harness_error"] = str(error)
    verdict["limits"] = "Official OAuth parse boundary only; accepted loss of target assertions/debug checks/exception-stack diagnostics. No entropy failure/label closure, public OCI, downstream 18HTTP or release acceptance."
    (output / "verdict.json").write_text(json.dumps(verdict, indent=2) + "\n")
    print(json.dumps({key: value for key, value in verdict.items() if key != "runtime"}, indent=2))


if __name__ == "__main__":
    main()
