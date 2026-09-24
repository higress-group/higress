#!/usr/bin/env python3
"""Type-preserving canonical hashes for JSON descriptors."""

import hashlib
import json
import math
from dataclasses import dataclass


DESCRIPTOR_MISMATCH_EXIT = 42


@dataclass(frozen=True)
class JSONNumber:
    lexeme: str


def loads_typed(raw):
    """Parse JSON while retaining integer/float tokens as typed source text."""
    return json.loads(
        raw,
        parse_int=JSONNumber,
        parse_float=JSONNumber,
        parse_constant=lambda value: (_ for _ in ()).throw(ValueError(f"invalid JSON number {value}")),
    )


def canonical_json_sha256(value):
    """Hash a type-tagged JSON tree; numeric and string domains are distinct."""
    encoded = json.dumps(
        _tagged(value), sort_keys=False, separators=(",", ":"), ensure_ascii=False, allow_nan=False,
    ).encode("utf-8")
    return hashlib.sha256(encoded).hexdigest()


def _tagged(value):
    if value is None:
        return ["null"]
    if isinstance(value, bool):
        return ["boolean", value]
    if isinstance(value, JSONNumber):
        return ["number", value.lexeme]
    if isinstance(value, int):
        return ["number", str(value)]
    if isinstance(value, float):
        if not math.isfinite(value):
            raise ValueError("non-finite JSON number")
        return ["number", json.dumps(value, allow_nan=False, separators=(",", ":"))]
    if isinstance(value, str):
        return ["string", value]
    if isinstance(value, list):
        return ["array", [_tagged(item) for item in value]]
    if isinstance(value, dict):
        if not all(isinstance(key, str) for key in value):
            raise TypeError("JSON object keys must be strings")
        return ["object", [[key, _tagged(value[key])] for key in sorted(value)]]
    raise TypeError(f"unsupported JSON value type: {type(value).__name__}")
