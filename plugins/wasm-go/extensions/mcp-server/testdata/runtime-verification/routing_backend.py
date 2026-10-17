#!/usr/bin/env python3
"""Synthetic MCP backend and append-only per-exchange ledger for #4597."""
import hashlib
import fcntl
import json
import os
import queue
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit

LOCK = threading.Lock()
ATTEMPTS = {}
CHANNELS = {}
IDENTITY = os.environ.get("BACKEND_ID", "primary")


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_args):
        pass

    def body(self):
        if self.headers.get("transfer-encoding", "").lower() == "chunked":
            chunks = []
            while True:
                size = int(self.rfile.readline().strip().split(b";", 1)[0], 16)
                if not size:
                    self.rfile.readline()
                    return b"".join(chunks)
                chunks.append(self.rfile.read(size))
                self.rfile.read(2)
        return self.rfile.read(int(self.headers.get("content-length", "0")))

    def ledger(self, operation, raw=b""):
        key = self.headers.get("baggage", "none")
        record = {"key": key, "backend": IDENTITY, "operation": operation,
                  "path": self.path, "headers": list(self.headers.items()),
                  "body_sha256": hashlib.sha256(raw).hexdigest(), "body": raw.decode()}
        with LOCK:
            with open("/evidence/ledger.jsonl", "a+") as output:
                fcntl.flock(output, fcntl.LOCK_EX)
                output.seek(0)
                record["attempt"] = 1 + sum(1 for line in output if (lambda row: row["key"] == key and row["operation"] == operation)(json.loads(line)))
                output.write(json.dumps(record) + "\n")
                output.flush()
                fcntl.flock(output, fcntl.LOCK_UN)
        return record

    def send(self, status, value=None, headers=None, fragmented=False):
        raw = b"" if value is None else value if isinstance(value, bytes) else json.dumps(value).encode()
        self.send_response(status)
        merged_headers = {"content-type": "application/json", **(headers or {})}
        for name, value in merged_headers.items():
            self.send_header(name, value)
        self.send_header("content-length", str(len(raw)))
        self.end_headers()
        try:
            if fragmented and len(raw) > 2:
                self.wfile.write(raw[:len(raw)//2]); self.wfile.flush(); time.sleep(.03)
                self.wfile.write(raw[len(raw)//2:])
            else:
                self.wfile.write(raw)
        except (BrokenPipeError, ConnectionResetError):
            pass

    def do_GET(self):
        if self.path == "/healthz":
            self.send(200, {"ok": True}); return
        if self.path == "/ledger":
            with LOCK:
                try:
                    with open("/evidence/ledger.jsonl") as source: rows = [json.loads(x) for x in source]
                except FileNotFoundError: rows = []
            self.send(200, rows); return
        key = self.headers.get("baggage", "none")
        self.ledger("sse/get")
        channel = queue.Queue()
        with LOCK: CHANNELS[key] = channel
        self.send_response(200)
        self.send_header("content-type", "text/event-stream")
        self.send_header("connection", "close")
        self.end_headers()
        try:
            self.wfile.write(("event: endpoint\ndata: /messages?session=" + key + "\n\n").encode()); self.wfile.flush()
            for _ in range(3):
                message = channel.get(timeout=15)
                self.wfile.write(b"event: message\ndata: " + json.dumps(message).encode() + b"\n\n"); self.wfile.flush()
                if message.get("id") == 3 or "tools" in message.get("result", {}): break
        except (queue.Empty, BrokenPipeError, ConnectionResetError):
            pass
        self.close_connection = True

    def do_POST(self):
        raw = self.body()
        try: request = json.loads(raw)
        except ValueError: self.send(400); return
        method, rpc_id = request.get("method"), request.get("id")
        record = self.ledger(method, raw)
        scenario = str(request.get("params", {}).get("arguments", {}).get("scenario", "normal"))
        result = {}
        if method == "server/discover":
            if urlsplit(self.path).path == "/legacy":
                self.send(404); return
            result = {"resultType": "complete", "supportedVersions": ["2026-07-28"], "capabilities": {"tools": {}},
                      "ttlMs": 0, "cacheScope": "private", "_meta": {"io.modelcontextprotocol/serverInfo": {"name": "routing-fixture", "version": "1"}}}
        elif method == "initialize":
            result = {"protocolVersion": "2025-03-26", "capabilities": {"tools": {}}, "serverInfo": {"name": "routing-fixture", "version": "1"}}
        elif method == "notifications/initialized":
            self.send(202); return
        else:
            if scenario in ("slow", "timeout"): time.sleep(6 if scenario == "slow" else 3)
            if scenario == "disconnect": time.sleep(.1)
            if scenario == "retry" and record["attempt"] == 1:
                self.send(503); return
            if scenario.startswith("status-"):
                status = int(scenario.split("-")[1])
                self.send(status, None if request.get("params", {}).get("arguments", {}).get("empty") else {"jsonrpc": "2.0", "id": rpc_id, "error": {"code": -32603, "message": "fixture rejected"}}, {"www-authenticate": "Bearer realm=fixture", "set-cookie": "upstream-private"}); return
            if scenario == "invalid": self.send(200, b"invalid", {"content-type": "application/octet-stream"}); return
            if method == "tools/list":
                result = {"tools": [{"name": "echo", "inputSchema": {"type": "object"}}, {"name": "hidden", "inputSchema": {"type": "object"}}]}
            else:
                result = {"content": [{"type": "text", "text": "routing-ok"}], "isError": False}
            if self.headers.get("mcp-protocol-version") == "2026-07-28": result["resultType"] = "complete"
        response = {"jsonrpc": "2.0", "id": rpc_id, "result": result}
        query = parse_qs(urlsplit(self.path).query)
        if "session" in query:
            with LOCK: channel = CHANNELS.get(query["session"][0])
            if channel: channel.put(response)
            self.send(202); return
        if scenario == "sse-body" and method.startswith("tools/"):
            self.send(200, b"event: message\ndata: " + json.dumps(response).encode() + b"\n\n", {"content-type": "text/event-stream"}, True); return
        self.send(200, response, {"mcp-session-id": "upstream-session"} if method == "initialize" else {}, scenario == "fragmented")


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
