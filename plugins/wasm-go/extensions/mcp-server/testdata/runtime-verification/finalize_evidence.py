#!/usr/bin/env python3
"""Finalize sanitized, machine-readable runtime evidence."""

import hashlib
import json
import os
import re
from collections import Counter
from pathlib import Path


root = Path(os.environ["RUNTIME_EVIDENCE"])
matrix_path = root / "matrix.json"
matrix = json.loads(matrix_path.read_text()) if matrix_path.exists() else {"cases": []}
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
        "manifest.json", "matrix.json", "client-exchanges.json", "access-coverage.json", "compose-config.yaml", "envoy.yaml", "envoy-auto.yaml",
        "gateway.log", "gateway-auto.log", "backend-auto-state.json", "backend-primary-final.json", "backend-secondary-final.json",
        "cleanup-proof.txt", "SHA256SUMS",
    ],
    "sanitization": "fake credentials are redacted from textual logs; backend evidence stores only presence/match booleans and never raw credentials or session IDs",
    "cleanup": (lines("cleanup-proof.txt") or ["pending"])[0],
}
(root / "manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")

checksums = []
for path in sorted(root.iterdir()):
    if path.is_file() and path.name not in ("plugin.wasm", "SHA256SUMS"):
        checksums.append(f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}")
(root / "SHA256SUMS").write_text("\n".join(checksums) + "\n")

print(json.dumps({"evidence": str(root), "source_sha": manifest["source_sha"], "plugin_sha256": manifest["plugin_sha256"], "access_coverage": access_coverage["status"], **matrix["summary"]}, sort_keys=True))
raise SystemExit(0 if matrix["summary"]["fail"] == 0 and access_ok else 1)
