#!/usr/bin/env python3
"""Deterministic observable REST and MCP backend for Envoy runtime verification."""

import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse


ORIGIN = os.environ.get("BACKEND_ORIGIN", "primary")
EXPECTED_AUTH = "Bearer runtime-upstream-token"
LOCK = threading.Lock()
EVENTS = []
SEQ = 0


def truthy_header(headers, name):
    return bool(headers.get(name))


def safe_event(handler, body):
    global SEQ
    parsed = {}
    try:
        parsed = json.loads(body) if body else {}
    except json.JSONDecodeError:
        pass
    headers = handler.headers
    event = {
        "origin": ORIGIN,
        "httpMethod": handler.command,
        "path": urlparse(handler.path).path,
        "rpcMethod": parsed.get("method"),
        "rpcId": parsed.get("id"),
        "toolName": (parsed.get("params") or {}).get("name"),
        "protocolVersion": headers.get("MCP-Protocol-Version"),
        "mcpMethod": headers.get("Mcp-Method"),
        "mcpName": headers.get("Mcp-Name"),
        "futureParam": headers.get("Mcp-Param-Future"),
        "authorizationPresent": truthy_header(headers, "Authorization"),
        "authorizationMatchesExplicitPolicy": headers.get("Authorization") == EXPECTED_AUTH,
        "cookiePresent": truthy_header(headers, "Cookie"),
        "sessionPresent": truthy_header(headers, "Mcp-Session-Id"),
        "lastEventIDPresent": truthy_header(headers, "Last-Event-ID"),
        "internalRoutePresent": truthy_header(headers, "x-envoy-allow-mcp-tools"),
        "unrelatedCredentialPresent": truthy_header(headers, "x-unrelated-credential"),
    }
    parsed_url = urlparse(handler.path)
    if parsed_url.path.startswith(("/compat/", "/corpus/")):
        event["compatibilityRequest"] = {
            "query": parse_qs(parsed_url.query),
            "flag": headers.get("X-Compat-Flag") or headers.get("X-Corpus-Flag"),
            "jsonBody": parsed if isinstance(parsed, dict) else {},
        }
    with LOCK:
        SEQ += 1
        event["seq"] = SEQ
        EVENTS.append(event)
    return parsed


def read_request_body(handler):
    length = int(handler.headers.get("Content-Length", "0"))
    if length:
        return handler.rfile.read(length).decode("utf-8", "replace")
    if handler.headers.get("Transfer-Encoding", "").lower() != "chunked":
        return ""
    chunks = []
    while True:
        size_line = handler.rfile.readline().strip().split(b";", 1)[0]
        size = int(size_line, 16)
        if size == 0:
            # Consume the terminating CRLF (fixtures do not send trailers).
            handler.rfile.readline()
            break
        chunks.append(handler.rfile.read(size))
        handler.rfile.read(2)
    return b"".join(chunks).decode("utf-8", "replace")


class Handler(BaseHTTPRequestHandler):
    server_version = "runtime-fixture/1"

    def log_message(self, _format, *_args):
        return

    def send_json(self, status, value, headers=None):
        data = json.dumps(value, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        for name, header_value in (headers or {}).items():
            self.send_header(name, header_value)
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        global SEQ
        parsed_url = urlparse(self.path)
        if parsed_url.path == "/__state":
            with LOCK:
                state = {"origin": ORIGIN, "events": list(EVENTS)}
            return self.send_json(200, state)
        if parsed_url.path == "/healthz":
            return self.send_json(200, {"ok": True, "origin": ORIGIN})
        if parsed_url.path in (
            "/rest/weather", "/v3/weather/weatherInfo", "/compat/health",
            "/corpus/valid",
        ):
            safe_event(self, "")
            query = parse_qs(parsed_url.query)
            city = (query.get("city") or ["unknown"])[0]
            return self.send_json(200, {"origin": ORIGIN, "city": city, "weather": "sunny"})
        return self.send_json(404, {"error": "not found"})

    def do_POST(self):
        global EVENTS, SEQ
        parsed_url = urlparse(self.path)
        body = read_request_body(self)
        if parsed_url.path == "/__reset":
            with LOCK:
                EVENTS = []
                SEQ = 0
            return self.send_json(200, {"reset": True, "origin": ORIGIN})

        request = safe_event(self, body)
        if parsed_url.path.startswith("/compat/"):
            return self.send_json(200, {"ok": True, "path": parsed_url.path, "origin": ORIGIN})
        mode = self.headers.get("Mcp-Param-Test-Mode")
        if mode == "auth401":
            return self.send_json(401, {"error": "fixture unauthorized"}, {"WWW-Authenticate": 'Bearer realm="runtime-fixture"'})
        if mode == "auth403":
            return self.send_json(403, {"error": "fixture forbidden"}, {"WWW-Authenticate": 'Bearer error="insufficient_scope"'})

        method = request.get("method")
        rpc_id = request.get("id")
        if method == "initialize":
            return self.send_json(200, {
                "jsonrpc": "2.0", "id": rpc_id,
                "result": {"protocolVersion": "2025-03-26", "capabilities": {"tools": {}},
                           "serverInfo": {"name": "fixture-legacy", "version": "1.0.0"}},
            }, {"Mcp-Session-Id": "fixture-upstream-session"})
        if method == "notifications/initialized":
            self.send_response(202)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        if method == "tools/list":
            result = {"tools": [{"name": "proxy_echo", "description": "fixture echo", "inputSchema": {"type": "object"}}]}
            if self.headers.get("MCP-Protocol-Version") == "2026-07-28":
                result.update({"resultType": "complete", "ttlMs": 0, "cacheScope": "private",
                               "_meta": {"io.modelcontextprotocol/serverInfo": {"name": "fixture-modern", "version": "1.0.0"}}})
            return self.send_json(200, {"jsonrpc": "2.0", "id": rpc_id, "result": result})
        if method == "tools/call":
            arguments = (request.get("params") or {}).get("arguments") or {}
            result = {"content": [{"type": "text", "text": "echo:" + str(arguments.get("value", ""))}], "isError": False}
            if self.headers.get("MCP-Protocol-Version") == "2026-07-28":
                result.update({"resultType": "complete",
                               "_meta": {"io.modelcontextprotocol/serverInfo": {"name": "fixture-modern", "version": "1.0.0"}}})
            return self.send_json(200, {"jsonrpc": "2.0", "id": rpc_id, "result": result})
        return self.send_json(200, {"jsonrpc": "2.0", "id": rpc_id,
                                    "error": {"code": -32601, "message": "Method not found"}})


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
