#!/usr/bin/env python3
"""Finalize sanitized, machine-readable runtime evidence."""

import hashlib
import json
import os
import re
import sys
from collections import Counter
from pathlib import Path

from typed_canonical import DESCRIPTOR_MISMATCH_EXIT, canonical_json_sha256, loads_typed


root = Path(os.environ["RUNTIME_EVIDENCE"])

if os.environ.get("RUNTIME_DESCRIPTOR_SELF_TEST") == "1":
    self_test_manifest = json.loads((root / "corpus-manifest.json").read_text())
    self_test_fixture = os.environ["RUNTIME_DESCRIPTOR_FIXTURE"]
    self_test_actual = loads_typed(Path(os.environ["RUNTIME_DESCRIPTOR_ACTUAL"]).read_bytes())
    self_test_expected = next(
        item["expectedInputSchemaSha256"]
        for item in self_test_manifest["fixtures"]
        if item["fixture"] == self_test_fixture
    )
    sys.exit(0 if canonical_json_sha256(self_test_actual) == self_test_expected else DESCRIPTOR_MISMATCH_EXIT)


matrix_path = root / "matrix.json"
matrix = json.loads(matrix_path.read_text()) if matrix_path.exists() else {"cases": []}

baseline_log = (root / "gateway-baseline.log").read_text(errors="replace") if (root / "gateway-baseline.log").exists() else ""
baseline_state = json.loads((root / "backend-baseline-state.json").read_text()) if (root / "backend-baseline-state.json").exists() else {"events": ["missing"]}
baseline_ok = (
    "requires a primitive type" in baseline_log
    and "plugin start failed" in baseline_log
    and baseline_state.get("events") == []
)
matrix["cases"].append({
    "case": "schema-compatibility-pinned-baseline-is-rejected",
    "status": "PASS" if baseline_ok else "FAIL",
    "detail": {
        "sourceSha": os.environ["BASELINE_SHA"],
        "pluginSha256": os.environ["BASELINE_PLUGIN_SHA256"],
        "configurationAccepted": False,
        "compilerRejection": "enum requires a primitive type",
        "upstreamCalls": 0,
        "backendEvents": {"backend-primary": []},
    } if baseline_ok else {"error": "expected pinned baseline compiler rejection or zero-upstream proof is absent"},
})

corpus_manifest_path = root / "corpus-manifest.json"
corpus_manifest = json.loads(corpus_manifest_path.read_text()) if corpus_manifest_path.exists() else {"fixtures": []}
corpus_runs = {}
for revision in ("oracle", "affected", "candidate"):
    path = root / f"corpus-{revision}.json"
    run = json.loads(path.read_text()) if path.exists() else {"fixtures": []}
    corpus_runs[revision] = {record.get("fixture"): record for record in run.get("fixtures", [])}
expected_corpus_fixtures = {
    "unsupported-semantics", "contradictory-semantics", "byte-limit", "depth-limit", "node-limit",
    "collection-limit", "enum-limit", "numeric-comparison-limit", "mixed-valid-invalid", "rule-level",
}
manifest_fixture_names = {fixture.get("fixture") for fixture in corpus_manifest.get("fixtures", [])}
corpus_complete = (
    manifest_fixture_names == expected_corpus_fixtures
    and all(set(records) == expected_corpus_fixtures for records in corpus_runs.values())
)
if not corpus_complete:
    matrix["cases"].append({
        "case": "schema-compatibility-corpus-completeness",
        "status": "FAIL",
        "detail": {"error": "representative corpus manifest or revision evidence is incomplete"},
    })
affected_corpus_log = (root / "gateway-corpus-affected.log").read_text(errors="replace") if (root / "gateway-corpus-affected.log").exists() else ""
descriptor_mismatch = False
for fixture in corpus_manifest.get("fixtures", []):
    slug = fixture["fixture"]
    records = {revision: corpus_runs[revision].get(slug, {}) for revision in corpus_runs}
    acceptance_ok = all(
        record.get("expectedAcceptance") == fixture["expectedAcceptance"][revision]
        and record.get("actualAcceptance") == fixture["expectedAcceptance"][revision]
        for revision, record in records.items()
    )
    observed_descriptor_hashes = (
        records["candidate"].get("modernDescriptorSha256"),
        records["candidate"].get("legacyDescriptorSha256"),
        records["oracle"].get("legacyDescriptorSha256"),
    )
    descriptor_hashes_present = all(isinstance(value, str) for value in observed_descriptor_hashes)
    descriptor_hashes_match = descriptor_hashes_present and all(
        value == fixture["expectedInputSchemaSha256"] for value in observed_descriptor_hashes
    )
    if descriptor_hashes_present and not descriptor_hashes_match:
        descriptor_mismatch = True
    behavior_ok = (
        records["candidate"].get("modernList") is True
        and records["candidate"].get("modernCallBlocked") is True
        and records["candidate"].get("legacyList") is True
        and records["oracle"].get("legacyList") is True
        and records["affected"].get("legacyList") is False
        and descriptor_hashes_match
    )
    if fixture["legacyMapping"]:
        behavior_ok = behavior_ok and records["candidate"].get("legacyRESTMapping") is True
        behavior_ok = behavior_ok and records["oracle"].get("legacyRESTMapping") is True
    if slug == "mixed-valid-invalid":
        behavior_ok = behavior_ok and records["candidate"].get("validSiblingCallable") is True
        behavior_ok = behavior_ok and records["candidate"].get("validSiblingResultContract") is True
    if slug == "rule-level":
        behavior_ok = behavior_ok and records["candidate"].get("globalList") is True
        behavior_ok = behavior_ok and records["oracle"].get("globalList") is True
    rejection_logged = slug in affected_corpus_log and "plugin start failed" in affected_corpus_log
    fixture_ok = acceptance_ok and behavior_ok and rejection_logged
    matrix["cases"].append({
        "case": f"schema-compatibility-corpus-{slug}",
        "status": "PASS" if fixture_ok else "FAIL",
        "detail": {
            "expectedAcceptance": fixture["expectedAcceptance"],
            "actualAcceptance": {revision: record["actualAcceptance"] for revision, record in records.items()},
            "modernCandidate": {
                "listed": records["candidate"]["modernList"],
                "callBlocked": records["candidate"]["modernCallBlocked"],
            },
            "expectedInputSchemaSha256": fixture["expectedInputSchemaSha256"],
            "modernDescriptorSha256": records["candidate"]["modernDescriptorSha256"],
            "legacyDescriptorSha256": {
                revision: record["legacyDescriptorSha256"] for revision, record in records.items()
            },
            "legacyList": {revision: record["legacyList"] for revision, record in records.items()},
            "legacyRESTMapping": {
                revision: record["legacyRESTMapping"] for revision, record in records.items()
            },
            "validSiblingCallable": records["candidate"]["validSiblingCallable"],
            "validSiblingResultContract": records["candidate"]["validSiblingResultContract"],
            "validSiblingBackendEvents": records["candidate"]["validSiblingBackendEvents"],
            "globalList": {revision: record["globalList"] for revision, record in records.items()},
            "backendEvents": {
                revision: record.get("backendEvents", {}).get("backend-primary", [])
                for revision, record in records.items()
            },
            "affectedRejectionLogged": True,
        } if fixture_ok else {"error": f"corpus evidence incomplete for {slug}"},
    })

oracle_path = root / "oracle-verification.json"
oracle = json.loads(oracle_path.read_text()) if oracle_path.exists() else {}
oracle_log = (root / "gateway-oracle.log").read_text(errors="replace") if (root / "gateway-oracle.log").exists() else ""
oracle_events = oracle.get("backendEvents", {}).get("backend-primary", [])
oracle_ok = (
    oracle.get("configurationAccepted") is True
    and oracle.get("descriptorPreserved") is True
    and oracle.get("legacyRESTMapping") is True
    and len(oracle_events) == 1
    and "plugin start failed" not in oracle_log
)
matrix["cases"].append({
    "case": "schema-compatibility-v2.0.0-oracle-accepts-and-invokes",
    "status": "PASS" if oracle_ok else "FAIL",
    "detail": {
        "sourceSha": os.environ["ORACLE_SHA"],
        "pluginSha256": os.environ["ORACLE_PLUGIN_SHA256"],
        "configurationAccepted": True,
        "descriptorPreserved": True,
        "legacyRESTMapping": True,
        "upstreamCalls": len(oracle_events),
        "backendEvents": {"backend-primary": oracle_events},
        "oracleEvidence": "oracle-verification.json",
    } if oracle_ok else {"error": "v2.0.0 compatibility oracle acceptance, descriptor, or REST mapping proof is absent"},
})

control_states = {
    variant: json.loads((root / f"backend-control-{variant}-state.json").read_text())
    if (root / f"backend-control-{variant}-state.json").exists() else {"events": ["missing"]}
    for variant in ("oracle", "affected", "candidate")
}
control_logs = {
    variant: (root / f"gateway-control-{variant}.log").read_text(errors="replace")
    if (root / f"gateway-control-{variant}.log").exists() else ""
    for variant in ("oracle", "affected", "candidate")
}
control_rejections = {
    variant: "error parsing URL template" in log and "plugin start failed" in log
    for variant, log in control_logs.items()
}
control_ok = all(control_rejections.values()) and all(
    state.get("events") == [] for state in control_states.values()
)
matrix["cases"].append({
    "case": "malformed-non-schema-control-is-rejected-by-all-revisions",
    "status": "PASS" if control_ok else "FAIL",
    "detail": {
        "revisions": {
            "oracle": os.environ["ORACLE_SHA"],
            "affected": os.environ["BASELINE_SHA"],
            "candidate": os.environ["SOURCE_SHA"],
        },
        "configurationAccepted": {variant: False for variant in control_rejections},
        "rejection": "error parsing URL template",
        "upstreamCalls": 0,
        "backendEvents": {
            variant: {"backend-primary": control_states[variant]["events"]}
            for variant in control_states
        },
    } if control_ok else {"error": f"non-schema rejection proof is incomplete: {control_rejections}"},
})

generation_phases = ("valid-before", "validation-unavailable", "valid-after")
generation_path = root / "generation-transition.json"
generation_transition = json.loads(generation_path.read_text()) if generation_path.exists() else {"generations": []}
generation_records = generation_transition.get("generations", [])
process_before = (root / "generation-process-before.txt").read_text().strip() if (root / "generation-process-before.txt").exists() else "missing-before"
process_after = (root / "generation-process-after.txt").read_text().strip() if (root / "generation-process-after.txt").exists() else "missing-after"
generation_ok = (
    [record.get("phase") for record in generation_records] == list(generation_phases)
    and [record.get("schemaState") for record in generation_records] == ["validated", "validation-unavailable", "validated"]
    and [len(record.get("backendEvents", {}).get("backend-primary", [])) for record in generation_records] == [1, 0, 1]
    and generation_transition.get("ldsVersions") == list(generation_phases)
    and process_before == process_after
    and not process_before.startswith("missing")
)
matrix["cases"].append({
    "case": "schema-compatibility-same-process-dynamic-generation-transition",
    "status": "PASS" if generation_ok else "FAIL",
    "detail": {
        "sequence": [record.get("schemaState") for record in generation_records],
        "backendCallsByGeneration": [len(record["backendEvents"]["backend-primary"]) for record in generation_records],
        "runtimeBoundary": "same Envoy process and Wasm, file-backed LDS config generations",
        "processIdentity": process_before,
        "generationEvidence": "generation-transition.json",
    } if generation_ok else {"error": "same-process valid -> validation-unavailable -> valid LDS evidence is incomplete"},
})

auto_log = (root / "gateway-auto.log").read_text(errors="replace") if (root / "gateway-auto.log").exists() else ""
auto_state = json.loads((root / "backend-auto-state.json").read_text()) if (root / "backend-auto-state.json").exists() else {"events": ["missing"]}
auto_ok = "invalid protocolStrategy value: auto" in auto_log and auto_state.get("events") == []
matrix["cases"].append({
    "case": "proxy-auto-configuration-is-rejected",
    "status": "PASS" if auto_ok else "FAIL",
    "detail": {"configuredValue": "auto", "accepted": False, "upstreamCalls": 0, "backendEvents": {"backend-primary": []}}
    if auto_ok else {"error": "expected configuration rejection absent from sanitized gateway log"},
})
matrix["summary"] = {
    "pass": sum(case["status"] == "PASS" for case in matrix["cases"]),
    "fail": sum(case["status"] != "PASS" for case in matrix["cases"]),
}
matrix_path.write_text(json.dumps(matrix, indent=2, sort_keys=True) + "\n")

client_exchanges = json.loads((root / "client-exchanges.json").read_text()).get("exchanges", [])
expected_request_ids = [exchange.get("accessRequestId") for exchange in client_exchanges]
gateway_log = (root / "gateway.log").read_text(errors="replace") if (root / "gateway.log").exists() else ""
access_request_ids = re.findall(r"access request_id=([^ ]+)", gateway_log)
expected_counts = Counter(expected_request_ids)
access_counts = Counter(access_request_ids)
missing_request_ids = sorted(request_id for request_id in expected_counts if access_counts[request_id] == 0)
duplicate_request_ids = sorted(request_id for request_id, count in access_counts.items() if count != 1)
unexpected_request_ids = sorted(request_id for request_id in access_counts if request_id not in expected_counts)
access_ok = (
    len(expected_request_ids) == len(set(expected_request_ids))
    and len(access_request_ids) == len(expected_request_ids)
    and not missing_request_ids
    and not duplicate_request_ids
    and not unexpected_request_ids
)
access_coverage = {
    "status": "PASS" if access_ok else "FAIL",
    "recordedClientExchangeCount": len(expected_request_ids),
    "accessRecordCount": len(access_request_ids),
    "missingRequestIds": missing_request_ids,
    "duplicateRequestIds": duplicate_request_ids,
    "unexpectedRequestIds": unexpected_request_ids,
}
(root / "access-coverage.json").write_text(json.dumps(access_coverage, indent=2, sort_keys=True) + "\n")


def lines(name):
    path = root / name
    return path.read_text(errors="replace").strip().splitlines() if path.exists() else []


manifest = {
    "source_sha": os.environ["SOURCE_SHA"],
    "source_tree_clean": os.environ.get("SOURCE_TREE_CLEAN") == "true",
    "plugin_sha256": os.environ["PLUGIN_SHA256"],
    "baseline_source_sha": os.environ["BASELINE_SHA"],
    "baseline_plugin_sha256": os.environ["BASELINE_PLUGIN_SHA256"],
    "oracle_source_sha": os.environ["ORACLE_SHA"],
    "oracle_plugin_sha256": os.environ["ORACLE_PLUGIN_SHA256"],
    "corpus_fixture_sha256": os.environ["CORPUS_FIXTURE_SHA256"],
    "corpus_plugin_sha256": {
        "candidate": os.environ["CORPUS_CANDIDATE_PLUGIN_SHA256"],
        "affected": os.environ["CORPUS_AFFECTED_PLUGIN_SHA256"],
        "oracle": os.environ["CORPUS_ORACLE_PLUGIN_SHA256"],
    },
    "gateway_image": "higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:v2.2.3",
    "gateway_resolved_digests": lines("gateway-image-digests.txt"),
    "backend_image": "docker.io/library/python:3.12-alpine",
    "backend_resolved_digests": lines("backend-image-digests.txt"),
    "podman_version": (lines("podman-version.txt") or [""])[0],
    "compose_version": lines("compose-version.txt"),
    "matrix": "matrix.json",
    "client_exchange_count": len(client_exchanges),
    "access_coverage": access_coverage,
    "evidence_index": [
        "manifest.json", "matrix.json", "client-exchanges.json", "access-coverage.json", "compose-config.yaml", "compose-config.json", "envoy.yaml", "envoy-auto.yaml",
        "envoy-baseline.yaml", "envoy-oracle.yaml", "envoy-control-candidate.yaml",
        "envoy-control-affected.yaml", "envoy-control-oracle.yaml", "envoy-generation.yaml", "lds-generation-valid-before.yaml",
        "lds-generation-validation-unavailable.yaml", "lds-generation-valid-after.yaml", "lds-generation-current.yaml",
        "gateway.log", "gateway-auto.log", "gateway-baseline.log", "gateway-oracle.log",
        "gateway-control-candidate.log", "gateway-control-affected.log", "gateway-control-oracle.log", "gateway-generation.log",
        "backend-auto-state.json", "backend-baseline-state.json", "backend-oracle-state.json", "backend-control-state.json",
        "backend-control-candidate-state.json", "backend-control-affected-state.json", "backend-control-oracle-state.json",
        "oracle-verification.json", "generation-transition.json",
        "generation-process-before.txt", "generation-process-after.txt",
        "backend-primary-final.json", "backend-secondary-final.json",
        "lifecycle-diagnostics.log", "cleanup-proof.txt", "SHA256SUMS",
    ] + sorted(path.name for path in root.glob("lds-corpus-*.yaml")) + [
        "corpus-manifest.json", "corpus-candidate.json", "corpus-affected.json", "corpus-oracle.json",
        "gateway-corpus-candidate.log", "gateway-corpus-affected.log", "gateway-corpus-oracle.log",
    ],
    "sanitization": "fake credentials are redacted from textual logs; backend evidence stores only presence/match booleans and never raw credentials or session IDs",
    "cleanup": (lines("cleanup-proof.txt") or ["pending"])[0],
}
(root / "manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")

checksums = []
for path in sorted(root.iterdir()):
    if path.is_file() and path.name not in (
        "plugin.wasm", "baseline-plugin.wasm", "oracle-plugin.wasm",
        "corpus-plugin-candidate.wasm", "corpus-plugin-affected.wasm", "corpus-plugin-oracle.wasm", "SHA256SUMS",
    ):
        checksums.append(f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}")
(root / "SHA256SUMS").write_text("\n".join(checksums) + "\n")

print(json.dumps({"evidence": str(root), "source_sha": manifest["source_sha"], "plugin_sha256": manifest["plugin_sha256"], "access_coverage": access_coverage["status"], **matrix["summary"]}, sort_keys=True))
if descriptor_mismatch:
    raise SystemExit(DESCRIPTOR_MISMATCH_EXIT)
raise SystemExit(0 if matrix["summary"]["fail"] == 0 and access_ok else 1)
