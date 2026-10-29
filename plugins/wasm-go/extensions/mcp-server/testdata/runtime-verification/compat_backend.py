"""Focused #5281 backend using the existing HTTP server and ledger endpoints."""
import hashlib
import json
import queue
import os
import threading
from urllib.parse import parse_qs, quote, urlsplit

from compat_contract import JSONNumber, MODERN, strict_request, token, unique_json
from typed_canonical import canonical_json_sha256, loads_typed

LOCK = threading.Lock()
EVENTS = []
CHANNELS = {}


def reset():
    with LOCK:
        EVENTS.clear()


def state():
    with LOCK:
        return {"events": list(EVENTS)}


def raw_response(handler, status, raw=b"", media="application/json", headers=()):
    handler.send_response(status)
    if media is not None:
        handler.send_header("Content-Type", media)
    handler.send_header("Content-Length", str(len(raw)))
    for name, value in headers:
        handler.send_header(name, value)
    handler.end_headers()
    try:
        handler.wfile.write(raw)
    except (BrokenPipeError, ConnectionResetError):
        pass


def rpc_response(rpc_id, result=None, error=False):
    field = b'"error":{"code":-32099,"message":"fixture error","data":{"discard":true}}' if error else b'"result":' + result
    return b'{"jsonrpc":"2.0","id":' + token(rpc_id).encode() + b',' + field + b'}'


def record(handler, raw, request=None, rejection=None):
    request = request or {}
    method = request.get("method")
    kind = "transport" if handler.command == "GET" else "probe" if method == "server/discover" else "control" if method in ("initialize", "notifications/initialized") else "business"
    parsed = urlsplit(handler.path)
    query = parse_qs(parsed.query)
    auth = handler.headers.get("Authorization", "")
    key = handler.headers.get("X-Compat-Case", "unassociated")
    event = {"case": key, "kind": kind, "method": handler.command, "rpcMethod": method,
             "backendOrigin": os.environ.get("BACKEND_ORIGIN", "primary"),
             "rpcId": token(request["id"]) if "id" in request else None,
             "authority": handler.headers.get("Host", ""), "path": parsed.path,
             "query": {k: ["<synthetic-credential>"] if k == "api_key" else v for k, v in query.items()},
             "queryCredentialMatches": query.get("api_key") == ["compat-static"],
             "authorizationAlias": "static" if auth == "Bearer compat-static" else "forward" if auth == "Bearer compat-forward" else "passthrough" if auth == "Bearer compat-pass" else "other" if auth else None,
             "upstreamKeyMatches": handler.headers.get("X-Upstream-Key") == "compat-static",
             "clientKeyPresent": bool(handler.headers.get("X-Client-Key")),
             "restFlag": handler.headers.get("X-Compat-Flag"),
             "ordinaryHeaders": handler.headers.get_all("X-Ordinary", []),
             "rpcBodyBytes": len(raw), "rpcBodySha256": hashlib.sha256(raw).hexdigest(),
             "accepted": rejection is None, "rejection": rejection,
             "initializeCapabilityKeys": sorted((request.get("params") or {}).get("capabilities", {})) if method == "initialize" else None}
    if request.get("params") is not None:
        event["paramsTypedSha256"] = canonical_json_sha256(request["params"])
        arguments = request["params"].get("arguments")
        if arguments is not None:
            event["argumentsTypedSha256"] = canonical_json_sha256(arguments)
        for key, value in (("ordinaryChoice", request["params"].get("ordinary", {}).get("choice")),
                           ("metaVendorChoice", request["params"].get("_meta", {}).get("vendor", {}).get("choice"))):
            event[key] = token(value)
    with LOCK:
        event["sequence"] = len(EVENTS) + 1
        EVENTS.append(event)
    return event


def get(handler):
    parsed = urlsplit(handler.path)
    if parsed.path == "/__compat_state":
        handler.send_json(200, state())
        return True
    if parsed.path.startswith("/unified/rest/"):
        record(handler, b"", {"method": "rest", "params": {}})
        handler.send_json(200, {"ok": True})
        return True
    if parsed.path != "/unified/sse":
        return False
    # The initial GET intentionally keeps the old main-request path. Its body
    # is not a JSON-RPC POST and must not be put through the strict peer decoder.
    from backend import read_request_body
    read_request_body(handler)
    record(handler, b"")
    if handler.headers.get("X-Compat-Fail-Stage") == "get":
        status = int(handler.headers.get("X-Compat-Response", "http401")[4:])
        raw_response(handler, status, headers=(("WWW-Authenticate", 'Bearer realm="compat"'),))
        return True
    key = handler.headers.get("X-Compat-Case", "unassociated")
    channel = queue.Queue()
    with LOCK:
        CHANNELS[key] = channel
    mode = handler.headers.get("X-Compat-Endpoint", "relative")
    endpoint = "/unified/messages?case=" + quote(key, safe="")
    if mode in ("absolute", "cross-authority", "external-domain", "path-config-absolute", "host-rewrite", "unknownhost"):
        authority = "unknownhost" if mode == "unknownhost" else "initial.virtual" if mode == "absolute" else "never-resolve.invalid" if mode == "external-domain" else "other.virtual"
        endpoint = "http://" + authority + endpoint
    elif mode == "relative":
        endpoint = endpoint.lstrip("/")
    handler.send_response(200)
    handler.send_header("Content-Type", "text/event-stream")
    handler.send_header("Connection", "close")
    handler.end_headers()
    try:
        handler.wfile.write(("event: endpoint\ndata: " + endpoint + "\n\n").encode())
        handler.wfile.flush()
        while True:
            terminal, response = channel.get(timeout=20)
            handler.wfile.write(b"event: message\ndata: " + response + b"\n\n")
            handler.wfile.flush()
            if terminal:
                break
    except (queue.Empty, BrokenPipeError, ConnectionResetError):
        pass
    finally:
        with LOCK:
            CHANNELS.pop(key, None)
        handler.close_connection = True
    return True


def post(handler, body):
    path = urlsplit(handler.path).path
    if path == "/__compat_reset":
        reset()
        handler.send_json(200, {"reset": True})
        return True
    if path.startswith("/unified/rest/"):
        # REST tools own their declared conversions; this path observes their
        # ordinary HTTP request without imposing an MCP envelope on it.
        request = {"method": "rest", "params": {"arguments": loads_typed(body) if body else {}}}
        record(handler, body.encode(), request)
        handler.send_json(200, {"ok": True, "length": len(body.encode())})
        return True
    if not path.startswith("/unified/") and not (path == "/instructions" and handler.headers.get("X-Compat-Case")):
        return False
    headers = handler.headers
    modern = headers.get("mcp-protocol-version") == MODERN
    # Legacy peers retain their historical capabilities policy. Candidate
    # initialize claims are checked independently in the execution ledger.
    try:
        request = strict_request(body, headers, modern, check_capabilities=modern)
    except (AssertionError, ValueError, TypeError) as error:
        record(handler, body.encode(), rejection=str(error))
        raw_response(handler, 400, b'{"error":"strict upstream rejected request"}')
        return True
    record(handler, body.encode(), request)
    method, rpc_id = request.get("method"), request.get("id")
    mode = handler.headers.get("X-Compat-Response", "success")
    if handler.headers.get("Host") == "unknownhost":
        raw_response(handler, 404)
        return True
    fail_stage = handler.headers.get("X-Compat-Fail-Stage")
    if fail_stage and fail_stage != method:
        mode = "success"
    elif not fail_stage and method in ("server/discover", "initialize", "notifications/initialized") and path != "/instructions":
        # A business fixture must never poison the independent probe/handshake.
        mode = "success"
    if mode.startswith("http") and (method.startswith("tools/") or method == "server/discover" or fail_stage == method):
        raw_response(handler, int(mode[4:]), rpc_response(rpc_id, error=True), "application/x-custom",
                     (("Retry-After", "5"), ("WWW-Authenticate", 'Bearer realm="compat"')))
        return True
    if method == "server/discover":
        if path.endswith(("legacy", "auto-legacy")):
            raw_response(handler, 404)
            return True
        result = b'{"resultType":"complete","supportedVersions":["2026-07-28"],"capabilities":{"tools":{}},"ttlMs":0,"cacheScope":"private","_meta":{"io.modelcontextprotocol/serverInfo":{"name":"compat-peer","version":"1"}}}'
        if path == "/instructions":
            result = b'{"resultType":"complete","instructions":"fixture instructions","capabilities":{"unimplemented":{}},"ttlMs":99}'
    elif method == "initialize":
        result = b'{"protocolVersion":"2025-03-26","capabilities":{"tools":{}},"serverInfo":{"name":"compat-peer","version":"1"}}'
    elif method == "notifications/initialized":
        raw_response(handler, 202)
        return True
    else:
        result = b'{"tools":[{"name":"echo","inputSchema":{"type":"object"}}],"content":[],"number":900719925474099312345,"value":1,"value":2,"recipient":' + json.dumps(handler.headers.get("Host", "")).encode() + b'}'
        if mode == "over-probe-limit":
            result = result[:-1] + b',"padding":"' + b"x" * ((1 << 20) + 1) + b'"}'
        if mode == "deep":
            result = result[:-1] + b',"deep":' + b'[' * 70 + b'0' + b']' * 70 + b'}'
    response = rpc_response(rpc_id, result, mode == "rpc-error")
    if path == "/unified/messages":
        key = handler.headers.get("X-Compat-Case", "unassociated")
        with LOCK:
            channel = CHANNELS.get(key)
        if channel:
            channel.put((method.startswith("tools/"), response))
        raw_response(handler, 202)
    else:
        media = None if mode == "missing-media" else "application/x-custom" if mode == "custom-media" else "application/json"
        raw_response(handler, 200, response, media)
    return True
