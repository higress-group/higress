"""Request-scoped auto fixtures. All credentials below are public test aliases."""

import json
import socket
import threading

LOCK = threading.Lock()
CONFIG = {"mode": "modern", "case": "unset"}
BARRIERS = {}
RELEASED = {}
RETURNED = {}
EXECUTIONS = {}
MODERN = "2026-07-28"


def control(handler, path, body):
    global CONFIG
    if path == "/__auto_config":
        with LOCK:
            for barrier in BARRIERS.values():
                barrier.set()
            CONFIG = json.loads(body)
            BARRIERS.clear()
            RELEASED.clear()
            RETURNED.clear()
            EXECUTIONS.clear()
        handler.send_json(200, {"configured": True})
        return True
    if path == "/__auto_release":
        stage = json.loads(body)["stage"]
        with LOCK:
            barrier = BARRIERS.get(stage)
            if barrier:
                barrier.set()
        handler.send_json(200, {"released": barrier is not None})
        return True
    return False


def state():
    with LOCK:
        return {"case": CONFIG["case"], "waiting": list(BARRIERS),
                "released": dict(RELEASED), "returned": dict(RETURNED), "executions": dict(EXECUTIONS)}


def event_fields(handler, request):
    params = request.get("params") or {}
    credential = handler.headers.get("Authorization", "")
    with LOCK:
        case = CONFIG["case"]
    return {"autoCase": case, "requestKey": handler.headers.get("baggage"),
            "authority": handler.headers.get("Host"),
            "authAlias": {"Bearer auto-alice": "alice", "Bearer auto-bob": "bob",
                          "Bearer runtime-upstream-token": "fixed", "Bearer auto-tool": "tool"}.get(credential, "none"),
            "modernMetaPresent": "_meta" in params,
            "continuationPresent": "requestState" in params or "inputResponses" in params,
            "sessionAlias": "current" if handler.headers.get("Mcp-Session-Id", "").startswith("auto-session-") else "none"}


def raw_response(handler, status, data, media="application/json", headers=None):
    if isinstance(data, str):
        data = data.encode()
    handler.send_response(status)
    handler.send_header("Content-Type", media)
    handler.send_header("Content-Length", str(len(data)))
    for key, value in (headers or {}).items():
        handler.send_header(key, value)
    handler.end_headers()
    try:
        handler.wfile.write(data)
    except (BrokenPipeError, ConnectionResetError):
        pass


def handle(handler, request):
    try:
        return handle_response(handler, request)
    except (BrokenPipeError, ConnectionResetError):
        # Cancellation/timeout fixtures intentionally let the gateway detach.
        return
    finally:
        with LOCK:
            RETURNED[request.get("method")] = True


def handle_response(handler, request):
    method = request.get("method")
    rpc_id = request.get("id")
    key = handler.headers.get("baggage", "unkeyed")
    with LOCK:
        config = dict(CONFIG)
        if config.get("barrier") == method:
            barrier = BARRIERS.setdefault(method, threading.Event())
        else:
            barrier = None
    if barrier:
        if not barrier.wait(15):
            return raw_response(handler, 504, b"")
        with LOCK:
            RELEASED[method] = True
    mode = config["mode"]

    def error(code, status=400, data=None, wrong_id=False):
        value = {"jsonrpc": "2.0", "id": "wrong-id" if wrong_id else rpc_id,
                 "error": {"code": code, "message": "fixture error"}}
        if data is not None:
            value["error"]["data"] = data
        handler.send_json(status, value)

    if method == "server/discover":
        if mode == "network":
            handler.close_connection = True
            handler.connection.shutdown(socket.SHUT_RDWR)
            handler.connection.close()
            return
        if mode == "modern-ordinary-error":
            return handler.send_json(400, {"jsonrpc": "2.0", "id": rpc_id, "error": {"code": -32602, "message": "fixture error"}},
                                     {"MCP-Protocol-Version": MODERN})
        if mode in ("trailing-json", "batch"):
            encoded = json.dumps({"jsonrpc": "2.0", "id": rpc_id, "error": {"code": -32601, "message": "fixture"}})
            return raw_response(handler, 400, encoded + " {}" if mode == "trailing-json" else "[" + encoded + "]")
        if mode in ("legacy", "legacy-session", "init-version", "notify-fail", "legacy-continuation"):
            return error(-32601, 200)
        if mode in ("http400", "http404", "http405"):
            return raw_response(handler, int(mode[4:]), b"legacy endpoint", "text/plain")
        if mode in ("401", "403", "429", "500", "302"):
            return raw_response(handler, int(mode), b"fixture error", "text/plain",
                                {"WWW-Authenticate": 'Bearer realm="fixture"', "Retry-After": "2"})
        if mode == "version":
            return error(-32022, data={"requested": MODERN, "supported": ["2025-03-26", "2025-06-18"]})
        if mode == "version-unknown":
            return error(-32022, data={"requested": MODERN, "supported": ["2025-11-25"]})
        if mode == "version-conflict":
            return error(-32022, data={"requested": MODERN, "supported": [MODERN, "2025-03-26"]})
        if mode == "header-mismatch":
            return error(-32020)
        if mode == "missing-capability":
            return error(-32021, data={"requiredCapabilities": {"sampling": {}}})
        if mode == "modern-404":
            return error(-32601, 404)
        if mode == "oversize":
            return raw_response(handler, 200, "x" * ((1 << 20) + 1))
        if mode == "malformed":
            return raw_response(handler, 200, "{invalid")
        result = {"resultType": "complete", "supportedVersions": [MODERN], "capabilities": {"tools": {}},
                  "ttlMs": 0, "cacheScope": "private",
                  "_meta": {"io.modelcontextprotocol/serverInfo": {"name": "auto-fixture", "version": "1"}}}
        if mode == "no-tools":
            result["capabilities"] = {}
        value = {"jsonrpc": "2.0", "id": "wrong-id" if mode == "wrong-id" else rpc_id, "result": result}
        if mode in ("sse", "sse-duplicate", "sse-missing"):
            notification = 'data:{"jsonrpc":"2.0","method":"notifications/progress","params":{"progressToken":"p","progress":1}}\r\n\r\n'
            encoded = json.dumps(value)
            final = "data: {\r\ndata: " + encoded[1:] + "\r\n\r\n"
            frames = ": fixture\r\n\r\n" + notification
            if mode != "sse-missing":
                frames += final
            if mode == "sse-duplicate":
                frames += final
            return raw_response(handler, 200, frames, "text/event-stream")
        return handler.send_json(200, value)
    if method == "initialize":
        version = (request.get("params") or {}).get("protocolVersion", "2025-03-26")
        if mode == "init-version":
            version = "2025-11-25"
        result = {"protocolVersion": version, "capabilities": {"tools": {}},
                  "serverInfo": {"name": "auto-legacy", "version": "1"}}
        headers = {"Mcp-Session-Id": "auto-session-" + key} if mode == "legacy-session" else {}
        return handler.send_json(200, {"jsonrpc": "2.0", "id": rpc_id, "result": result}, headers)
    if method == "notifications/initialized":
        if mode == "notify-fail":
            return error(-32603, 200)
        return raw_response(handler, 202, b"")
    if method in ("tools/list", "tools/call"):
        if method == "tools/call":
            with LOCK:
                EXECUTIONS[key] = EXECUTIONS.get(key, 0) + 1
        if mode == "response-loss":
            handler.close_connection = True
            handler.connection.shutdown(socket.SHUT_RDWR)
            handler.connection.close()
            return
        if mode == "business-version-error":
            return error(-32022, data={"requested": MODERN, "supported": ["2025-03-26"]})
        if mode == "cursor-error":
            return error(-32602, 200)
        result = {"tools": [{"name": "proxy_echo", "inputSchema": {"type": "object"}}]} if method == "tools/list" else {
            "content": [{"type": "text", "text": "auto fixture"}], "opaque": 900719925474099312345}
        if handler.headers.get("MCP-Protocol-Version") == MODERN:
            result["resultType"] = "complete"
            result["_meta"] = {"io.modelcontextprotocol/serverInfo": {"name": "auto-fixture", "version": "1"}}
        value = {"jsonrpc": "2.0", "id": rpc_id, "result": result}
        if mode == "sse":
            return raw_response(handler, 200, "data:" + json.dumps(value) + "\n\n", "text/event-stream")
        return handler.send_json(200, value)
    return error(-32601, 404)
