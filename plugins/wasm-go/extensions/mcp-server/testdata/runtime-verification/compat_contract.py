"""Independent case registry and exact JSON boundary helpers for #5281.

Bodies are generated one at a time. Evidence records lengths/hashes, never the
100 MiB payload. Numeric source tokens use the existing typed canonicalizer.
"""
import hashlib
import base64
import json
import re
from collections import Counter

from typed_canonical import JSONNumber, loads_typed

ORACLE = "958b5e7f8e0062d1e26963d008e7be4b707fa715"
CONTROL = "bde990b4bfc7ecb581338e2d0ae858aaedb2fd69"
MODERN = "2026-07-28"
LIMIT = 100 * 1024 * 1024
META = {"io.modelcontextprotocol/protocolVersion": MODERN,
        "io.modelcontextprotocol/clientCapabilities": {}}
IDS = ('"client\\u002did"', "9007199254740991", "9007199254740993", "9223372036854775808", "1.50", "1E+03", "-0", "null", "")
SIZES = (("over-one-mib", (1 << 20) + 1, "arguments"),
         ("ten-mib", 10 << 20, "arguments"),
         ("near-limit", LIMIT - 1, "arguments"),
         ("exact-limit-meta", LIMIT, "meta"),
         ("over-limit-meta", LIMIT + 1, "meta"))


def token(value):
    return value.lexeme if isinstance(value, JSONNumber) else json.dumps(value, ensure_ascii=False, separators=(",", ":"))


def unique_json(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError("duplicate JSON member")
            result[key] = value
        return result
    return json.loads(raw, parse_int=JSONNumber, parse_float=JSONNumber,
                      object_pairs_hook=pairs, parse_constant=lambda _: (_ for _ in ()).throw(ValueError("invalid number")))


def strict_request(raw, headers, modern, check_capabilities=True):
    value = unique_json(raw)
    assert isinstance(value, dict) and value.get("jsonrpc") == "2.0", "invalid RPC object"
    if hasattr(headers, "get_all"):
        for name in ("content-type", "accept", "mcp-protocol-version", "mcp-method", "mcp-name", "mcp-session-id"):
            assert len(headers.get_all(name, [])) <= 1, "duplicate outbound singleton header"
    assert headers.get("content-type", "").split(";", 1)[0] == "application/json", "invalid media"
    if modern:
        assert "application/json" in headers.get("accept", ""), "missing JSON accept"
    method = value.get("method")
    if method != "notifications/initialized":
        rpc_id = value.get("id")
        assert isinstance(rpc_id, str) or (isinstance(rpc_id, JSONNumber) and
               re.fullmatch(r"-?(0|[1-9][0-9]*)", rpc_id.lexeme) and rpc_id.lexeme != "-0" and
               -(1 << 63) <= int(rpc_id.lexeme) < (1 << 63)), "invalid strict upstream ID"
    assert method in ("server/discover", "initialize", "notifications/initialized", "tools/list", "tools/call"), "unsupported RPC method"
    params = value.get("params", {})
    assert isinstance(params, dict), "invalid params object"
    if method == "tools/call":
        assert isinstance(params.get("name"), str) and params["name"], "missing tool name"
    if modern:
        assert headers.get("mcp-protocol-version") == MODERN and headers.get("mcp-method") == method, "identity mismatch"
        meta = params.get("_meta", {})
        assert isinstance(meta, dict), "invalid metadata object"
        assert meta.get("io.modelcontextprotocol/protocolVersion") == MODERN, "version metadata mismatch"
        assert meta.get("io.modelcontextprotocol/clientCapabilities") == {}, "unsupported gateway claims"
        assert (meta.get("io.modelcontextprotocol/clientInfo") or {}).get("name") == "Higress-mcp-proxy", "gateway identity absent"
        if method == "tools/call":
            name = headers.get("mcp-name", "")
            if name.startswith("=?base64?") or name.endswith("?="):
                assert name.startswith("=?base64?") and name.endswith("?="), "invalid encoded name"
                payload = name[9:-2]
                decoded = base64.b64decode(payload, validate=True)
                assert decoded and base64.b64encode(decoded).decode() == payload, "noncanonical encoded name"
                name = decoded.decode("utf-8")
            assert name == params.get("name"), "tool identity mismatch"
        else:
            assert "mcp-name" not in headers, "unexpected tool identity"
    else:
        assert "mcp-method" not in headers and "mcp-name" not in headers, "modern identity leaked to Legacy"
        if method == "initialize" and check_capabilities:
            assert params.get("capabilities") == {}, "unsupported initialize capabilities"
    return value


def envelope(modern, method="tools/call", tool="raw_summary", arguments=b"{}", rpc_id='"case"', meta_padding=b""):
    params = b'"name":' + token(tool).encode() + b',"arguments":' + arguments if method == "tools/call" else b'"cursor":"first"'
    if modern:
        meta = json.dumps(META, separators=(",", ":")).encode()
        if meta_padding:
            meta = meta[:-1] + b',"padding":"' + meta_padding + b'"}'
        params += b',"_meta":' + meta
    elif meta_padding:
        params += b',"_meta":{"padding":"' + meta_padding + b'"}'
    identity = b'' if rpc_id == "" else b',"id":' + rpc_id.encode()
    return b'{"jsonrpc":"2.0"' + identity + b',"method":' + token(method).encode() + b',"params":{' + params + b'}}'


def utf8_padding(size):
    return "界".encode() * (size // 3) + b"x" * (size % 3)


def sized_body(modern, size, location):
    if location == "arguments":
        empty_args = b'{"padding":""}'
        overhead = len(envelope(modern, arguments=empty_args))
        arguments = b'{"padding":"' + utf8_padding(size - overhead) + b'"}'
        return envelope(modern, arguments=arguments), arguments
    # One-byte placeholder makes the metadata carrier present for both eras.
    overhead = len(envelope(modern, meta_padding=b"x")) - 1
    return envelope(modern, meta_padding=utf8_padding(size - overhead)), b"{}"


def registry(variant):
    if variant == "control":
        # This binary is normative only for retained mechanisms/scope. These
        # rows reproduce removed gates diagnostically; no business oracle role.
        return [{"id": "red/" + name, "acceptance": acceptance, "kind": "red", "red": name}
                for name, acceptance in (("modern-over-one-mib", ["A1", "A2"]),
                                         ("schema-call", ["A3"]), ("legacyOnly", ["A4"]),
                                         ("cross-origin", ["A11"]), ("fractional-id", ["A8"]))]
    profiles = ("legacy",) if variant == "oracle" else ("legacy", "modern")
    rows = []
    def add(case_id, acceptance, **fields):
        rows.append({"id": case_id, "acceptance": acceptance, **fields})
    for version in ("2024-11-05", "2025-03-26", "2025-06-18"):
        add("legacy-version/" + version, ["A8", "A12"], kind="legacy-version", profile="legacy", version=version)
    for profile in profiles:
        for name, size, location in SIZES:
            add(f"body/{profile}/{name}", ["A1", "A2", "A8"], kind="body", profile=profile, size=size, location=location)
        for shape in ("depth70", "values65537", "duplicates", "long-name", "encoded-name", "schema-mismatch", "invalid", "concatenated", "batch", "allowTools"):
            add(f"input/{profile}/{shape}", ["A1", "A2", "A3", "A8", "A9", "A10"], kind="input", profile=profile, shape=shape)
        for index in range(len(IDS)):
            add(f"local-id/{profile}/{index}", ["A8", "A9", "A10"], kind="input", profile=profile, shape="local-error", id_index=index)
        add(f"descriptors/{profile}", ["A3", "A4"], kind="descriptors", profile=profile)
        for origin in ("missing", "cross-host", "cross-port", "null"):
            add(f"origin/{profile}/{origin}", ["A11"], kind="origin", profile=profile, origin=origin)
        for builtin in ("e2bdev", "firecrawl"):
            add(f"builtin/{profile}/{builtin}", ["A3", "A4"], kind="builtin", profile=profile, builtin=builtin)
        for route in (("http-legacy", "sse") if variant == "oracle" else ("http-legacy", "http-modern", "auto-modern", "auto-legacy", "sse")):
            if profile == "legacy" and route in ("http-modern", "auto-modern"):
                continue
            for index in range(len(IDS)):
                for response in ("success", "rpc-error"):
                    for method in ("tools/list", "tools/call"):
                        add(f"id/{profile}/{route}/{method}/{index}/{response}", ["A5", "A8", "A9"], kind="id", profile=profile, route=route, method=method, id_index=index, response=response)
            for response in ("missing-media", "custom-media", "duplicates", "deep", "over-probe-limit", "encoded-name", "http400", "http404", "http429", "http500", "http503", "http504", "http401", "http403"):
                if variant == "oracle" and response in ("http401", "http403"):
                    continue  # SPEC4 retains the subsequent authentication fix.
                add(f"business/{profile}/{route}/{response}", ["A6", "A7", "A9"], kind="business", profile=profile, route=route, response=response)
            if profile == "modern":
                for status in ("http401", "http403"):
                    add(f"notification-auth/{route}/{status}", ["A7", "A8"], kind="business", profile=profile, route=route, response=status, notification=True)
        for endpoint in ("relative", "absolute", "cross-authority", "external-domain", "path-config", "path-config-absolute", "host-rewrite", "unknownhost"):
            add(f"sse-url/{profile}/{endpoint}", ["A5", "A10", "A11"], kind="sse-url", profile=profile, endpoint=endpoint)
        for credential in ("static-header", "static-query", "static-authorization", "forward", "passthrough", "removed"):
            add(f"sse-auth/{profile}/{credential}", ["A7", "A11"], kind="sse-auth", profile=profile, credential=credential)
    if variant == "candidate":
        for index in range(len(IDS)):
            for response in ("success", "rpc-error", "http401", "http403"):
                add(f"discovery/id/{index}/{response}", ["A5", "A8"], kind="discovery", profile="modern", id_index=index, response=response)
        for stage in ("get", "initialize", "notifications/initialized"):
            for status in ("http401", "http403"):
                for notification in (False, True):
                    add(f"sse-stage-auth/{stage}/{status}/{notification}", ["A7", "A8", "A10"], kind="sse-stage-auth", profile="modern", route="sse", response=status, notification=notification, stage=stage)
    return rows


def complete(expected, observed):
    expected_ids = [row["id"] for row in expected]
    actual_ids = [row["id"] for row in observed]
    return bool(expected_ids) and len(expected_ids) == len(set(expected_ids)) and Counter(expected_ids) == Counter(actual_ids)


def body_record(raw):
    return {"utf8Bytes": len(raw), "sha256": hashlib.sha256(raw).hexdigest()}


def exchange_count(case):
    """Independent per-case downstream request count, including counter controls."""
    return {"input": 4, "descriptors": 7, "builtin": 2}.get(case["kind"], 1)
