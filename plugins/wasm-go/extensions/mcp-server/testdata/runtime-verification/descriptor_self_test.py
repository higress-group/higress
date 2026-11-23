#!/usr/bin/env python3
"""Prepare and validate descriptor-oracle fixtures used by run.sh."""

import json
import sys
from pathlib import Path


FILES = (
    ".descriptor-selftest-structure-good.json",
    ".descriptor-selftest-structure-deleted.json",
    ".descriptor-selftest-array-truncated.json",
    ".descriptor-selftest-number-good.json",
    ".descriptor-selftest-number-as-string.json",
)


def structure_descriptor():
    return {
        "type": "object",
        "properties": {
            "recordId": {"description": "", "type": "string"},
            "page": {"description": "", "type": "integer"},
            "X-Corpus-Flag": {"description": "", "type": "boolean"},
            "payload": {"description": "", "type": "object", "properties": {"amount": {"type": "integer"}}},
            "problem": {"description": "", "type": "array", "items": {"oneOf": []}},
        },
        "required": ["recordId"],
    }


def write_json(path, value):
    path.write_text(json.dumps(value, separators=(",", ":")) + "\n")


def prepare(root):
    from typed_canonical import JSONNumber, canonical_json_sha256, loads_typed

    manifest = json.loads((root / "corpus-manifest.json").read_text())
    expected = {fixture["fixture"]: fixture["expectedInputSchemaSha256"] for fixture in manifest["fixtures"]}

    good = structure_descriptor()
    deleted = json.loads(json.dumps(good))
    del deleted["properties"]["problem"]["items"]
    truncated = json.loads(json.dumps(good))
    truncated["required"] = []
    write_json(root / FILES[0], good)
    write_json(root / FILES[1], deleted)
    write_json(root / FILES[2], truncated)

    numeric_good = b'{"type":"object","properties":{"value":{"type":"number","enum":[1e5000]}}}\n'
    numeric_string = b'{"type":"object","properties":{"value":{"type":"number","enum":["1e5000"]}}}\n'
    (root / FILES[3]).write_bytes(numeric_good)
    (root / FILES[4]).write_bytes(numeric_string)

    business_view = json.loads(numeric_good)
    assert not isinstance(business_view["properties"]["value"]["enum"][0], JSONNumber)
    assert canonical_json_sha256(good) == expected["unsupported-semantics"]
    assert canonical_json_sha256(deleted) != expected["unsupported-semantics"]
    assert canonical_json_sha256(truncated) != expected["unsupported-semantics"]
    assert canonical_json_sha256(loads_typed(numeric_good)) == expected["numeric-comparison-limit"]
    assert canonical_json_sha256(loads_typed(numeric_good)) != canonical_json_sha256(loads_typed(numeric_string))
    print("descriptor canonical self-test fixtures prepared", flush=True)


def cleanup(root):
    for name in FILES:
        path = root / name
        if path.exists():
            path.unlink()


def main():
    if len(sys.argv) != 3 or sys.argv[1] not in ("prepare", "cleanup"):
        raise SystemExit("usage: descriptor_self_test.py prepare|cleanup EVIDENCE_DIR")
    root = Path(sys.argv[2])
    if sys.argv[1] == "prepare":
        prepare(root)
    else:
        cleanup(root)


if __name__ == "__main__":
    main()
