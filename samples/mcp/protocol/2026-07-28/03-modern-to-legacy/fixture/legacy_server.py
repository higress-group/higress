#!/usr/bin/env python3
"""Observable MCP 2025-03-26 fixture for the modern-to-legacy demo."""

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse


LOCK = threading.Lock()
EVENTS = []
SEQUENCE = 0


def header_present(headers, name):
    return bool(headers.get(name))


def record_event(handler, request):
    global SEQUENCE
    params = request.get("params") or {}
    event = {
        "rpcMethod": request.get("method"),
        "rpcId": request.get("id"),
        "toolName": params.get("name"),
        "protocolVersion": handler.headers.get("MCP-Protocol-Version"),
        "mcpMethod": handler.headers.get("Mcp-Method"),
        "mcpName": handler.headers.get("Mcp-Name"),
        "futureParam": handler.headers.get("Mcp-Param-Future"),
        "authorizationPresent": header_present(handler.headers, "Authorization"),
        "cookiePresent": header_present(handler.headers, "Cookie"),
        "sessionPresent": header_present(handler.headers, "Mcp-Session-Id"),
        "lastEventIDPresent": header_present(handler.headers, "Last-Event-ID"),
        "unrelatedCredentialPresent": header_present(handler.headers, "x-unrelated-credential"),
    }
    with LOCK:
        SEQUENCE += 1
        event["seq"] = SEQUENCE
        EVENTS.append(event)


class Handler(BaseHTTPRequestHandler):
    server_version = "mcp-demo-legacy-fixture/1"

    def log_message(self, _format, *_args):
        return

    def send_json(self, status, value, headers=None):
        data = json.dumps(value, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        for name, value in (headers or {}).items():
            self.send_header(name, value)
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        path = urlparse(self.path).path
        if path == "/healthz":
            return self.send_json(200, {"ok": True})
        if path == "/__state":
            with LOCK:
                return self.send_json(200, {"events": list(EVENTS)})
        return self.send_json(404, {"error": "not found"})

    def do_POST(self):
        global EVENTS, SEQUENCE
        path = urlparse(self.path).path
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length).decode("utf-8", "replace")
        if path == "/__reset":
            with LOCK:
                EVENTS = []
                SEQUENCE = 0
            return self.send_json(200, {"reset": True})
        if path != "/legacy":
            return self.send_json(404, {"error": "not found"})

        request = json.loads(body)
        record_event(self, request)
        method = request.get("method")
        rpc_id = request.get("id")
        if method == "initialize":
            return self.send_json(200, {
                "jsonrpc": "2.0",
                "id": rpc_id,
                "result": {
                    "protocolVersion": "2025-03-26",
                    "capabilities": {"tools": {}},
                    "serverInfo": {"name": "fixture-legacy", "version": "1.0.0"},
                },
            }, {"Mcp-Session-Id": "fixture-upstream-session"})
        if method == "notifications/initialized":
            self.send_response(202)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        if method == "tools/list":
            return self.send_json(200, {
                "jsonrpc": "2.0",
                "id": rpc_id,
                "result": {
                    "tools": [{
                        "name": "proxy_echo",
                        "description": "Echo a deterministic fixture value",
                        "inputSchema": {
                            "type": "object",
                            "properties": {"value": {"type": "string"}},
                            "required": ["value"],
                        },
                    }],
                },
            })
        if method == "tools/call":
            arguments = (request.get("params") or {}).get("arguments") or {}
            return self.send_json(200, {
                "jsonrpc": "2.0",
                "id": rpc_id,
                "result": {
                    "content": [{"type": "text", "text": "echo:" + str(arguments.get("value", ""))}],
                    "isError": False,
                },
            })
        return self.send_json(200, {
            "jsonrpc": "2.0",
            "id": rpc_id,
            "error": {"code": -32601, "message": "Method not found"},
        })


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
