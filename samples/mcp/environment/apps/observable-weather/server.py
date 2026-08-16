#!/usr/bin/env python3
"""Protocol-neutral, observable HTTP fixture used by Higress MCP demos."""

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse


LOCK = threading.Lock()
EVENTS = []
SEQUENCE = 0


def record_event(handler):
    global SEQUENCE
    parsed_url = urlparse(handler.path)
    query = {key: values[-1] for key, values in parse_qs(parsed_url.query).items()}
    event = {
        "httpMethod": handler.command,
        "path": parsed_url.path,
        "query": query,
        "requestId": handler.headers.get("X-Request-ID"),
    }
    with LOCK:
        SEQUENCE += 1
        event["seq"] = SEQUENCE
        EVENTS.append(event)


class Handler(BaseHTTPRequestHandler):
    server_version = "mcp-demo-observable-http/1"

    def log_message(self, _format, *_args):
        return

    def send_json(self, status, value):
        data = json.dumps(value, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        parsed_url = urlparse(self.path)
        if parsed_url.path == "/healthz":
            return self.send_json(200, {"ok": True})
        if parsed_url.path == "/__state":
            with LOCK:
                state = {"events": list(EVENTS)}
            return self.send_json(200, state)
        if parsed_url.path == "/weather":
            record_event(self)
            location = (parse_qs(parsed_url.query).get("location") or ["unknown"])[-1]
            return self.send_json(200, {
                "location": location,
                "weather": "sunny",
                "message": f"weather for {location}",
            })
        return self.send_json(404, {"error": "not found"})

    def do_POST(self):
        global EVENTS, SEQUENCE
        if urlparse(self.path).path == "/__reset":
            with LOCK:
                EVENTS = []
                SEQUENCE = 0
            return self.send_json(200, {"reset": True})
        return self.send_json(404, {"error": "not found"})


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
