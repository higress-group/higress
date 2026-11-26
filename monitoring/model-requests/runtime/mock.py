"""Synthetic provider and existing usage-report receiver for remote validation."""
import json
import socket
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

reports = []
attempts = {}
lock = threading.Lock()


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_):
        pass

    def reply(self, status, body, content_type="application/json"):
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        with lock:
            self.reply(200, json.dumps({"reports": reports, "attempts": attempts}).encode())

    def do_POST(self):
        # Envoy forwards a length or uses chunked encoding after provider rewrites.
        if self.headers.get("Transfer-Encoding", "").lower() == "chunked":
            chunks = []
            while True:
                size = int(self.rfile.readline().split(b";", 1)[0], 16)
                if not size:
                    self.rfile.readline()
                    break
                chunks.append(self.rfile.read(size))
                self.rfile.read(2)
            body = b"".join(chunks)
        else:
            body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        if self.path == "/model/usage/report":
            with lock:
                reports.append(json.loads(body))
            self.reply(200, b'{"success":true}')
            return
        case = self.headers.get("x-test-case", "success")
        request_id = self.headers.get("request-id", "")
        with lock:
            attempts[request_id] = attempts.get(request_id, 0) + 1
            attempt = attempts[request_id]
        if case == "timeout":
            time.sleep(1)
        if case in ("http429", "http500") or case == "retry" and attempt == 1:
            self.reply(429 if case == "http429" else 503, b'{"error":{"message":"fixture failure"}}')
            return
        usage = {"model": "gpt-4", "choices": [{"index": 0, "delta": {"content": "fixture"}, "finish_reason": None}], "usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}}
        if case.startswith("sse") or case == "downstream-close":
            if case.startswith("sse-fixed"):
                payload = b"data: " + json.dumps(usage).encode() + b"\n\n"
                if case == "sse-fixed-normal":
                    payload += b"data: [DONE]\n\n"
                self.reply(200, payload, "text/event-stream")
                return
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Transfer-Encoding", "chunked")
            self.end_headers()
            first = b'data: {"error":{"message":"fixture stream error"}}\n\n' if case == "sse-error" else b"data: " + json.dumps(usage).encode() + b"\n\n"
            parts = [first[:9], first[9:]]
            if case not in ("sse-missing", "sse-reset", "downstream-close"):
                parts.extend([b"data: [DO", b"NE]\n\n"])
            for part in parts:
                self.wfile.write(f"{len(part):x}\r\n".encode() + part + b"\r\n")
                self.wfile.flush()
                time.sleep(0.01)
            if case == "sse-reset":
                self.close_connection = True
                self.connection.shutdown(socket.SHUT_RDWR)
                return
            if case == "downstream-close":
                time.sleep(1)
            self.wfile.write(b"0\r\n\r\n")
            self.wfile.flush()
            return
        self.reply(200, json.dumps(usage).encode())


ThreadingHTTPServer(("0.0.0.0", 8081), Handler).serve_forever()
