#!/usr/bin/env python3

import hashlib
import json
import os

import runtime


CONTROL_SHA256 = "665d335802513e48d338b7895bf0e2d09d324ba412be3c52d91e142117cf1dbc"
FIXTURE_SHA256 = "28fba56d19d6faa7130daad97b45424e29390ed96bb22f773dfc2fbf5bd74a22"
CONFIG_SHA256 = "365eeb6184a0528428d74f4f517e583a93e6c18afb87fdcaf9457b864dc41978"
HELPER = "__throw_exception_with_stack_trace"
SETTINGS = ["-sASSERTIONS=0", "-sEXCEPTION_STACK_TRACES=0"]


def read_json(path):
    return json.loads(path.read_text())


def action_arguments(summary, mnemonic):
    actions = [action for action in summary["executed_actions"] if action["mnemonic"] == mnemonic]
    if len(actions) != 1 or actions[0]["exitCode"] != 0:
        raise RuntimeError("Expected exactly one successful executed probe " + mnemonic)
    return actions[0]["arguments"]


def main():
    if os.environ.get("OAUTH_PROBE_VERSION") != "v1":
        raise SystemExit("V1 requires its dedicated workflow mode")
    output = runtime.OUT
    directory = output / "runtime"
    directory.mkdir(parents=True, exist_ok=True)
    verdict = {"head": os.environ["EXPECTED_HEAD"], "scope": "test-only-v1-diagnostic-contrast", "verdict": "PARTIAL", "production_mechanism_selected": False, "question_answered": False, "variants": [], "proof": {}}
    try:
        cases = read_json(output / "fixtures/cases.json")
        proof = verdict["proof"]
        proof["frozen_inputs"] = len(cases) == 23 and hashlib.sha256((output / "fixtures/cases.json").read_bytes()).hexdigest() == FIXTURE_SHA256 and hashlib.sha256((runtime.ROOT / "docs/developers/wasm-runtime-verification/envoy.yaml").read_bytes()).hexdigest() == CONFIG_SHA256 and (output / "source-head.txt").read_text().strip() == os.environ["EXPECTED_HEAD"]
        for image, name in ((runtime.GATEWAY, "gateway"), (runtime.HTTPBIN, "upstream")):
            if runtime.execute(["docker", "pull", image], directory / (name + "-pull.log")) != 0:
                raise RuntimeError("Pinned image unavailable: " + name)
            if runtime.execute(["docker", "image", "inspect", image], directory / (name + "-inspect.json")) != 0:
                raise RuntimeError("Pinned image identity unavailable: " + name)
        for variant in ("candidate_eh", "candidate_eh_nodiag"):
            result = runtime.run_variant(variant, cases)
            verdict["variants"].append(result)
            (directory / (variant + "-results.json")).write_text(json.dumps(result, indent=2) + "\n")
        control, candidate = verdict["variants"]
        flags = [read_json(output / variant / "flags-summary.json") for variant in ("candidate_eh", "candidate_eh_nodiag")]
        proof["both_actual_eh_actions_and_link_settings"] = all(summary["passed"] for summary in flags)
        comparison = {}
        for mnemonic in ("CppCompile", "CppLink"):
            original = action_arguments(flags[0], mnemonic)
            changed = [argument.replace("candidate_eh_nodiag", "candidate_eh") for argument in action_arguments(flags[1], mnemonic) if argument not in SETTINGS]
            comparison[mnemonic + "_equal_except_target_and_paired_settings"] = original == changed
        verdict["actual_action_comparison"] = comparison
        proof["only_paired_settings_differ"] = all(comparison.values())
        modules = [read_json(output / variant / "module-info.json") for variant in ("candidate_eh", "candidate_eh_nodiag")]
        helper_present = [any(entry["module"] == "env" and entry["name"] == HELPER for entry in module["imports"]) for module in modules]
        verdict["module_comparison"] = [{"variant": variant, "sha256": module["sha256"], "size": module["size"], "diagnostic_helper_import": helper, "sections": module["sections"], "target_features": module["target_features"]} for variant, module, helper in zip(("candidate_eh", "candidate_eh_nodiag"), modules, helper_present)]
        verdict["historical_control_diagnostic"] = {"historical_sha256": CONTROL_SHA256, "current_sha256": modules[0]["sha256"], "equal": modules[0]["sha256"] == CONTROL_SHA256, "cause": "unknown"}
        proof["control_helper_present_candidate_helper_removed_eh_tags_preserved"] = helper_present == [True, False] and all(any(section["id"] == 13 for section in module["sections"]) for module in modules)
        control_log = (directory / "candidate_eh/envoy-upstream.log").read_text(errors="replace")
        proof["control_honest_missing_import_load_failure"] = not control["loaded"] and not control["rows"] and "missing import: env." + HELPER in control_log and control.get("artifact_host_container_equal") is True
        native_rows = [line.split("\t") for line in (output / "native/results.tsv").read_text().splitlines()]
        proof["same_head_native_23_cases"] = (output / "native/build.exit").read_text().strip() == "0" and (output / "native/run.exit").read_text().strip() == "0" and len(native_rows) == 23 and all(len(row) == 5 and row[0] == case["name"] and row[1] == row[2] == case["expected"] and row[4] == "PASS" for row, case in zip(native_rows, cases))
        proof["candidate_artifact_host_container_identity"] = candidate.get("artifact_host_container_equal") is True and candidate.get("module_sha256") == modules[1]["sha256"]
        proof["both_variants_same_config_and_corpus"] = all(result.get("config_sha256") == CONFIG_SHA256 and result.get("fixture_sha256") == FIXTURE_SHA256 for result in (control, candidate))
        expected_rows = [(repeat, case["name"]) for repeat in (1, 2) for case in cases]
        proof["candidate_two_complete_rounds"] = [(row["repeat"], row["name"]) for row in candidate["rows"]] == expected_rows
        proof["candidate_all_http_counter_marker_and_correlated_logs"] = bool(candidate["rows"]) and all(row["passed"] and row.get("probe_log_observed") is True and (row["expected"] == "valid" or (row.get("following_valid_passed") is True and row.get("following_valid_log_observed") is True)) for row in candidate["rows"])
        proof["candidate_loaded_and_remains_healthy_no_trap"] = candidate["loaded"] and candidate["ready_after"] == [200, 200] and not candidate["errors"] and candidate.get("trap_or_vm_failure") is False and candidate.get("catch_log_count") == 40 and candidate.get("rejected_log_count") == 4
        proof["both_project_cleanups_proved"] = all(result["cleanup"] and result.get("ports_closed") is True and result.get("cleanup_command_exits") and all(status == 0 for status in result["cleanup_command_exits"].values()) for result in (control, candidate))
        proof["builder_success"] = (output / "builder.exit").read_text().strip() == "0"
        if all(proof.values()):
            verdict["verdict"] = "VALIDATED"
        elif not candidate["module_present"] or not proof["candidate_loaded_and_remains_healthy_no_trap"] or not proof["candidate_all_http_counter_marker_and_correlated_logs"]:
            verdict["verdict"] = "INVALIDATED"
    except Exception as error:
        verdict["harness_error"] = str(error)
    verdict["diagnostic_cost"] = "This test target loses assertions, implicit/default debug checks and exception stack diagnostics; not a zero-cost single-import removal."
    verdict["limitations"] = "One probe TU, fixed runtime and original finite corpus only; not production OAuth, cross-library unwinding, other runtimes, nine-plugin acceptance or release. V0 remains unchanged."
    (output / "verdict.json").write_text(json.dumps(verdict, indent=2) + "\n")
    print(json.dumps({key: value for key, value in verdict.items() if key != "variants"}, indent=2))


if __name__ == "__main__":
    main()
