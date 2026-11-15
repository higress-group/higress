"""只供隔离回归：合成 API Key、OpenAI 响应和可控失败，不接触真实上游。"""
import hashlib
import json
import os
import socket
import socketserver
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

EVENTS = []
FAILED = set()
LOCK = threading.Lock()


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_):
        pass

    def reply(self, status, body, content_type="application/json"):
        if not isinstance(body, bytes):
            body = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        with LOCK:
            self.reply(200, list(EVENTS))

    def do_POST(self):
        if self.headers.get("Transfer-Encoding", "").lower() == "chunked":
            chunks = []
            while True:
                size = int(self.rfile.readline().split(b";", 1)[0].strip(), 16)
                if size == 0:
                    while self.rfile.readline().strip():
                        pass
                    break
                chunks.append(self.rfile.read(size))
                assert self.rfile.read(2) == b"\r\n"
            body = b"".join(chunks)
        else:
            body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        data = json.loads(body or b"{}")
        if self.path == "/control/fail":
            with LOCK:
                FAILED.add(data["key"])
            self.reply(200, {"ok": True})
            return
        key = self.headers.get("Authorization", "").removeprefix("Bearer ")
        event = {"key": key, "sha256": hashlib.sha256(body).hexdigest(),
                 "size": len(body), "path": self.path,
                 "request_id": self.headers.get("X-Request-Id")}
        with LOCK:
            EVENTS.append(event)
            failed = key in FAILED
        if failed:
            self.reply(403, {"error": {"message": "synthetic blocked key", "type": "test"}})
            return
        payload = {"id": "chatcmpl-synthetic", "object": "chat.completion", "created": 1,
                   "model": "test-model", "choices": [{"index": 0,
                   "message": {"role": "assistant", "content": key}, "finish_reason": "stop"}],
                   "usage": {"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10}}
        if not data.get("stream"):
            self.reply(200, payload)
            return
        chunk = {"id": "chatcmpl-synthetic", "object": "chat.completion.chunk", "created": 1,
                 "model": "test-model", "choices": [{"index": 0,
                 "delta": {"role": "assistant", "content": key}, "finish_reason": None}]}
        tail = {"id": "chatcmpl-synthetic", "object": "chat.completion.chunk", "created": 1,
                "model": "test-model", "choices": [], "usage": payload["usage"]}
        frames = ("data: " + json.dumps(chunk) + "\n\ndata: " + json.dumps(tail)
                  + "\n\ndata: [DONE]\n\n").encode()
        self.reply(200, frames, "text/event-stream")


class RedisDelay(socketserver.BaseRequestHandler):
    def handle(self):
        with socket.create_connection(("redis", 6379)) as upstream:
            def responses():
                try:
                    while data := upstream.recv(65536):
                        self.request.sendall(data)
                except OSError:
                    pass

            worker = threading.Thread(target=responses, daemon=True)
            worker.start()
            try:
                while data := self.request.recv(65536):
                    time.sleep(float(os.environ.get("REDIS_DELAY_MS", "0")) / 1000)
                    upstream.sendall(data)
            except OSError:
                pass


if __name__ == "__main__":
    proxy = socketserver.ThreadingTCPServer(("0.0.0.0", 6380), RedisDelay)
    proxy.daemon_threads = True
    threading.Thread(target=proxy.serve_forever, daemon=True).start()
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
