"""Fail-closed focused evidence finalization; no inherited runtime PASS."""
import hashlib
import json
import re
from collections import Counter
from pathlib import Path

import compat_contract as contract
from instructions_contract import focused_evidence
import verify_auto


def completeness(expected, rows, key="case"):
    names = list(expected)
    return bool(names) and len(names) == len(set(names)) and Counter(names) == Counter(row.get(key) for row in rows)


def checksums(root):
    root = Path(root)
    return {str(path.relative_to(root)): hashlib.sha256(path.read_bytes()).hexdigest()
            for path in sorted(root.rglob("*")) if path.is_file() and path.name != "compat-manifest.json" and path.suffix != ".wasm"}


def seal_checksums(root):
    root = Path(root)
    path = root / "compat-manifest.json"
    manifest = json.loads(path.read_text())
    manifest["checksums"] = checksums(root)
    path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")


def metadata_errors(root, inputs):
    errors = []
    digest = lambda value: isinstance(value, str) and bool(re.fullmatch(r"[0-9a-f]{64}", value))
    if not re.fullmatch(r"[0-9a-f]{40}", inputs.get("candidate", "")):
        errors.append("candidate revision missing")
    versions = inputs.get("versions", {})
    for variant, source in (("oracle", contract.ORACLE), ("control", contract.CONTROL), ("candidate", inputs.get("candidate"))):
        entry = versions.get(variant, {})
        if entry.get("source") != source or not entry.get("go") or not entry.get("moduleFiles") or not all(digest(value) for value in entry.get("moduleFiles", {}).values()):
            errors.append(variant + " source/toolchain/modules unavailable")
        for flavor in ("plain", "registered"):
            path = root / f"{flavor}-{variant}.wasm"
            if not path.is_file() or entry.get("wasm", {}).get(flavor) != hashlib.sha256(path.read_bytes()).hexdigest():
                errors.append(variant + " " + flavor + " Wasm hash missing/mismatched")
    if not inputs.get("images") or not all("@sha256:" in image for image in inputs.get("images", {})):
        errors.append("immutable image provenance missing")
    if not inputs.get("harness"):
        errors.append("harness provenance missing")
    for relative, wanted in inputs.get("harness", {}).items():
        path = root / "harness" / relative
        if not digest(wanted) or not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != wanted:
            errors.append("harness hash missing/mismatched: " + relative)
    if inputs.get("registeredFixtureSha256") != inputs.get("harness", {}).get("registered_fixture.go.txt"):
        errors.append("shared registered fixture hash missing/mismatched")
    return errors


def core_ledger_errors(expected, snapshot, log):
    errors = []
    exchanges = snapshot.get("exchanges", [])
    if Counter(row.get("case") for row in exchanges) != Counter({case["id"]: contract.exchange_count(case) for case in expected}):
        errors.append("independent downstream exchange counts differ")
    ids = [row.get("accessRequestId") for row in exchanges]
    accesses = re.findall(r"access request_id=(compat-\S+) listener=\S+ method=\S+ path=\S+ code=(\d+) flags=\S+", log)
    if not ids or len(ids) != len(set(ids)) or Counter(ids) != Counter(row[0] for row in accesses) or len(accesses) != sum("access request_id=compat-" in line for line in log.splitlines()):
        errors.append("downstream/access IDs missing or duplicated")
    by_id = {row.get("accessRequestId"): row for row in exchanges}
    if any(int(status) != by_id.get(request_id, {}).get("httpStatus") for request_id, status in accesses):
        errors.append("access status differs from client response")
    return errors


def finalize(root):
    root = Path(root)
    errors, variants, coverage = [], {}, {}
    try:
        inputs = json.loads((root / "compat-inputs.json").read_text())
        if inputs.get("businessOracle") != contract.ORACLE or inputs.get("controlScopeBaseline") != contract.CONTROL:
            errors.append("source pins differ from the approved baselines")
        if not inputs.get("sourceStillClean") or not inputs.get("tokenOverridesUnset") or not inputs.get("cleanupComplete") or inputs.get("error") or inputs.get("verifierFailed"):
            errors.append("source, command, verifier or cleanup evidence failed")
    except (OSError, ValueError) as error:
        inputs = {}
        errors.append("inputs unavailable: " + str(error))
    errors.extend(metadata_errors(root, inputs))
    for variant in ("oracle", "control", "candidate"):
        try:
            snapshot = json.loads((root / f"compat-{variant}.json").read_text())
            expected = contract.registry(variant)
            rows = snapshot.get("cases", [])
            if snapshot.get("expectedCases") != expected or not contract.complete(expected, rows) or any(row.get("status") != "PASS" for row in rows):
                errors.append(variant + " case registry is incomplete, duplicated, changed or red")
            variants[variant] = {row["id"]: row for row in rows}
            for case in expected:
                row = variants[variant].get(case["id"], {})
                if row.get("acceptance") != case["acceptance"]:
                    errors.append(variant + " acceptance tags differ")
                for acceptance in case["acceptance"]:
                    coverage.setdefault(acceptance, []).append(variant + ":" + case["id"])
            log = (root / f"gateway-{variant}.log").read_text(errors="replace")
            errors.extend(variant + " " + error for error in core_ledger_errors(expected, snapshot, log))
            if variant == "candidate":
                log = (root / "gateway-candidate.log").read_text(errors="replace")
                if "plugin start successfully" not in log:
                    errors.append("candidate Wasm loading marker missing")
        except (OSError, ValueError, KeyError) as error:
            errors.append(variant + " evidence unavailable: " + str(error))
    # Compare only corresponding historical paths. Raw client IDs and numeric
    # tokens have their own positive assertions and narrowly named exceptions.
    differences, classified = [], []
    for case_id, oracle in variants.get("oracle", {}).items():
        candidate = variants.get("candidate", {}).get(case_id)
        if candidate is None:
            differences.append(case_id + ": missing candidate")
            continue
        old, new = oracle.get("detail", {}), candidate.get("detail", {})
        if case_id in {"input/legacy/" + shape for shape in ("invalid", "concatenated", "batch")}:
            if old.get("basicMessageBoundaryException") is True and new == {"zeroCreateCall": True, "boundary": "basic-message"}:
                classified.append({"case": case_id, "exception": "retained basic-message boundary", "observedOracle": old})
                continue
            differences.append(case_id + ": invalid basic-message exception evidence")
            continue
        old, new = old.get("projection", old), new.get("projection", new)
        if old != new:
            differences.append(case_id)
    if differences:
        errors.append("unclassified historical business differences")
    mechanisms = {}
    for variant in ("control", "candidate"):
        try:
            controls = json.loads((root / f"controls-{variant}.json").read_text())
            rows = controls["cases"]
            if controls["expectedCases"] != [name for name, _ in verify_auto.cases(None)] or not completeness(controls["expectedCases"], rows) or any(row.get("status") != "PASS" for row in rows):
                errors.append(variant + " retained control/cancellation cases failed")
            mechanisms[variant] = {row["case"]: row.get("detail", {}).get("upstreamSequence") for row in rows}
            role = "baseline" if variant == "control" else "candidate"
            instructions = json.loads((root / f"instructions-{role}.json").read_text())
            log = (root / f"gateway-{variant}.log").read_text(errors="replace")
            instructions_log = "\n".join(line for line in log.splitlines() if "access request_id=rv-" in line and " listener=instructions-" in line)
            check = focused_evidence(instructions, role, instructions_log)
            if not check["complete"] or not check["exchangeLedgerComplete"] or not check["accessLedgerComplete"]:
                errors.append(variant + " instructions scope/field/budget evidence failed")
            coverage.setdefault("A10", []).extend(variant + ":" + row["case"] for row in rows)
        except (OSError, ValueError, KeyError) as error:
            errors.append(variant + " supplemental evidence unavailable: " + str(error))
    if mechanisms.get("control") != mechanisms.get("candidate"):
        errors.append("retained control sequences differ")
    try:
        routing = json.loads((root / "routing/manifest.json").read_text())
        if routing.get("passed") is not True or routing.get("cleanup_complete") is not True:
            errors.append("routing/header/deadline/lifecycle regression failed")
        coverage["A12"] = ["routing/manifest.json", "controls-candidate.json"]
    except (OSError, ValueError):
        errors.append("routing evidence missing (development-only skip cannot pass)")
    for number in range(1, 13):
        if not coverage.get("A" + str(number)):
            errors.append("acceptance coverage missing: A" + str(number))
    manifest = {"mode": "unified-compatibility-runtime", "passed": not errors, "inputs": inputs,
                "businessOracleRole": "958bLegacy corresponding business paths only",
                "controlBaselineRole": "retained controls/instructions scope; separately labelled diagnostic red reproductions",
                "differences": differences, "classifiedDifferences": classified, "errors": errors, "coverage": coverage,
                "precisionExceptionPaths": ["client id on named historical numeric/null cases", "opaque params.arguments numeric tokens", "opaque result.number"],
                "nativeAndKindAcceptanceClaimed": False,
                "checksums": checksums(root)}
    (root / "compat-manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
    return 0 if manifest["passed"] else 1
