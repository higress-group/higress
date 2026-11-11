"""Closed synthetic #5336 native-session acknowledgement fixture and finalizer.

Only observed native endpoint/session/ping identities are normalized. No native
implementation, heartbeat source, production overlay or shared Redis is used.
"""
import base64
from collections import Counter
from copy import deepcopy
import hashlib
import http.client
import json
import os
from pathlib import Path
import queue
import shutil
import socket
import subprocess
import sys
import tarfile
import tempfile
import threading
import time
import uuid
from urllib.parse import parse_qsl, urlencode, urlsplit

import generate_envoy as base

AFFECTED = "d388d06a92dd125bbe99330888690fd6deeb352f"
NATIVE_PATH = "/var/lib/istio/envoy/golang-filter.so"
DIRECT, SESSION = 15100, 15101
MODERN = "2026-07-28"
META = {"io.modelcontextprotocol/protocolVersion": MODERN,
        "io.modelcontextprotocol/clientCapabilities": {}}
INVALID = {
    "missing-id": b'{"jsonrpc":"2.0","result":{}}',
    "boolean-id": b'{"jsonrpc":"2.0","id":true,"result":{}}',
    "composite-id": b'{"jsonrpc":"2.0","id":{},"result":{}}',
    "missing-version": b'{"id":1,"result":{}}',
    "wrong-version": b'{"jsonrpc":"1.0","id":1,"result":{}}',
    "missing-result": b'{"jsonrpc":"2.0","id":1}',
    "both": b'{"jsonrpc":"2.0","id":1,"result":null,"error":null}',
    "mixed-method": b'{"jsonrpc":"2.0","id":1,"method":"tools/call","result":{}}',
    "null-method": b'{"jsonrpc":"2.0","id":1,"method":null,"result":{}}',
    "error-null": b'{"jsonrpc":"2.0","id":1,"error":null}',
    "error-code": b'{"jsonrpc":"2.0","id":1,"error":{"code":1.5,"message":"x"}}',
    "error-message": b'{"jsonrpc":"2.0","id":1,"error":{"code":1,"message":null}}',
    "duplicate-id": b'{"jsonrpc":"2.0","id":1,"id":2,"result":{}}',
    "duplicate-jsonrpc": b'{"jsonrpc":"2.0","jsonrpc":"2.0","id":1,"result":{}}',
    "duplicate-result": b'{"jsonrpc":"2.0","id":1,"result":{},"result":{}}',
    "duplicate-code": b'{"jsonrpc":"2.0","id":1,"error":{"code":1,"code":2,"message":"x"}}',
    "duplicate-message": b'{"jsonrpc":"2.0","id":1,"error":{"code":1,"message":"x","message":"y"}}',
    "malformed": b'{"jsonrpc":"2.0","id":1,"result":',
    "utf8": b'{"jsonrpc":"2.0","id":1,"result":"\xff"}',
    "trailing": b'{"jsonrpc":"2.0","id":1,"result":{}}{}',
    "batch": b'[{"jsonrpc":"2.0","id":1,"result":{}}]',
    "non-object": b'null',
}


def require(value, message):
    if not value:
        raise AssertionError(message)


def cases():
    return ([{"id": name, "target": True} for name in ("A1/direct-result", "A2/direct-error", "A3/native-heartbeats")]
            + [{"id": name, "target": False} for name in ("C1/client-ping", "C2/initialize", "C2/initialized", "C2/list", "C2/call")]
            + [{"id": "C3/" + name, "target": False} for name in INVALID]
            + [{"id": "C4/" + name, "target": False} for name in ("header-result", "header-error", "body-result")]
            + [{"id": "C5/" + name, "target": False} for name in ("discover", "list", "call")])


def raw_record(raw):
    return {"base64": base64.b64encode(raw).decode(), "sha256": hashlib.sha256(raw).hexdigest(), "bytes": len(raw)}


def raw_bytes(record):
    value = base64.b64decode(record["base64"], validate=True)
    require(raw_record(value) == record, "raw byte/hash record altered")
    return value


def dump(value):
    return json.dumps(value, separators=(",", ":"), ensure_ascii=False).encode()


def request(method, rpc_id, params=None, modern=False):
    value = {"jsonrpc": "2.0", "id": rpc_id, "method": method}
    if params is not None or modern:
        value["params"] = deepcopy(params or {})
    if modern:
        value["params"]["_meta"] = META
    return dump(value)


def config(out, library=NATIVE_PATH):
    """One fixed REST fixture/module and a native-before-Wasm session listener."""
    fixture = {"server": {"name": "legacy-ack", "type": "rest"}, "tools": [{"name": "echo", "description": "Synthetic echo",
               "requestTemplate": {"url": "http://backend-primary:8080/unified/rest/legacy-ack", "method": "GET"}}]}
    direct = base.listener_yaml(DIRECT, "legacy-ack-direct", "backend-primary", fixture)
    session = base.listener_yaml(SESSION, "legacy-ack-session", "backend-primary", fixture)
    native = '''                - name: envoy.filters.http.golang
                  typed_config:
                    "@type": type.googleapis.com/envoy.extensions.filters.http.golang.v3alpha.Config
                    library_id: legacy-ack-native
                    library_path: %s
                    plugin_name: mcp-session
                    plugin_config:
                      "@type": type.googleapis.com/xds.type.v3.TypedStruct
                      value:
                        sse_path_suffix: /sse
                        redis: {address: "redis:6379"}
                        match_list:
                          - {server_name: legacy-ack, match_rule_domain: gateway, match_rule_path: /mcp/legacy, match_rule_type: prefix, upstream_type: rest}
''' % json.dumps(library)
    session = session.replace("                - name: envoy.filters.http.wasm\n", native + "                - name: envoy.filters.http.wasm\n")
    # A live stream owns its own bounded reader; Envoy must not impose a request
    # duration timeout before the two unmodified native 5-second heartbeats.
    session = session.replace("timeout: 30s", "timeout: 0s")
    Path(out, "envoy.yaml").write_text("admin:\n  address:\n    socket_address: {address: 0.0.0.0, port_value: 9901}\nstatic_resources:\n  listeners:\n" + direct + session + "  clusters:\n" + base.cluster_yaml("backend-primary"))


class Stream:
    """One bounded real HTTP SSE reader, always shut down and joined."""
    def __init__(self, request_id):
        self.frames, self.queue, self.errors = [], queue.Queue(), []
        self.connection = http.client.HTTPConnection("gateway", SESSION, timeout=20)
        self.connection.request("GET", "/mcp/legacy/sse", headers={"Host": "gateway", "Accept": "text/event-stream", "X-Request-ID": request_id})
        try:
            self.response = self.connection.getresponse()
            require(self.response.status == 200 and self.response.getheader("Content-Type", "").startswith("text/event-stream"), "native SSE startup failed")
        except Exception:
            self.connection.close()
            raise
        self.socket = self.connection.sock or self.response.fp.raw._sock
        self.stopping = False
        self.deadline = time.monotonic() + 20
        self.thread = threading.Thread(target=self.read, name="legacy-ack-sse", daemon=True)
        self.thread.start()

    def read(self):
        try:
            frame = bytearray()
            total = 0
            while not self.stopping:
                remaining = self.deadline - time.monotonic()
                require(remaining > 0, "native SSE reader exceeded 20 seconds")
                self.socket.settimeout(remaining)
                line = self.response.readline(1 << 20)
                if not line:
                    break
                frame.extend(line)
                total += len(line)
                require(total <= 4 << 20, "SSE byte bound exceeded")
                if line in (b"\n", b"\r\n"):
                    wire = bytes(frame)
                    frame.clear()
                    event, data = "message", []
                    for part in wire.splitlines():
                        if part.startswith(b"event:"):
                            event = part[6:].lstrip(b" ").decode()
                        if part.startswith(b"data:"):
                            data.append(part[5:].removeprefix(b" "))
                    if data:
                        row = {"event": event, "data": raw_record(b"\n".join(data)), "wire": raw_record(wire)}
                        self.frames.append(row)
                        self.queue.put(row)
        except Exception as error:
            if not self.stopping:
                self.errors.append(type(error).__name__ + ": " + str(error))

    def next(self, predicate, deadline):
        while time.monotonic() < deadline:
            row = self.queue.get(timeout=max(0.01, deadline - time.monotonic()))
            if predicate(row):
                return row
        raise AssertionError("native SSE observation deadline exceeded")

    def close(self):
        self.stopping = True
        try:
            self.socket.shutdown(socket.SHUT_RDWR)
        except OSError:
            pass
        self.thread.join(timeout=2)
        self.response.close()
        self.connection.close()
        require(not self.thread.is_alive(), "SSE reader did not join")


class Fixture:
    def __init__(self, role, out):
        self.role, self.out = role, Path(out)
        self.rows, self.exchanges, self.bindings = [], [], {}
        self.frames, self.startup, self.closed = [], None, False
        self.sequence = 0

    def post(self, case, raw, *, endpoint="/mcp", port=DIRECT, extra=()):
        self.sequence += 1
        access = "legacy-ack-" + str(self.sequence).zfill(3)
        headers = [("Host", "gateway"), ("Content-Type", "application/json"), ("Accept", "application/json,text/event-stream"),
                   ("X-Request-ID", access), ("X-Compat-Case", case), *extra]
        connection = http.client.HTTPConnection("gateway", port, timeout=10)
        try:
            connection.putrequest("POST", endpoint, skip_host=True, skip_accept_encoding=True)
            for key, value in headers:
                connection.putheader(key, value)
            connection.putheader("Content-Length", str(len(raw)))
            connection.endheaders(raw)
            response = connection.getresponse()
            body = response.read()
            record = {"case": case, "accessRequestId": access, "port": port, "path": endpoint, "headers": headers,
                      "request": raw_record(raw), "status": response.status, "response": raw_record(body)}
            self.exchanges.append(record)
            return record
        finally:
            connection.close()

    def record(self, case, callback, target=False):
        start = len(self.exchanges)
        row = {"id": case, "status": "FAIL", "validRedReproduction": False}
        try:
            callback()
            row["status"] = "PASS"
        except AssertionError as error:
            row["failure"] = str(error)
            # A target-red receipt requires actual 400/-32600 for every POST,
            # never a transport/parser/startup or assertion infrastructure error.
            exchanges = self.exchanges[start:]
            row["validRedReproduction"] = target and self.role == "affected" and bool(exchanges) and all(target_rejection(value) for value in exchanges)
        self.rows.append(row)

    def receipt(self, value):
        require(value["status"] == 202 and raw_bytes(value["response"]) == b"", "expected bodyless 202 acknowledgement")

    def success(self, value, rpc_id):
        body = json.loads(raw_bytes(value["response"]))
        require(value["status"] == 200 and body.get("id") == rpc_id and body.get("jsonrpc") == "2.0" and "result" in body and "error" not in body, "control success changed")
        return body["result"]

    def run(self):
        import verify as existing
        stream = None
        try:
            require(self.role in ("affected", "candidate"), "invalid focused role")
            existing.wait_http("http://backend-primary:8080/healthz")
            existing.wait_http("http://gateway:9901/ready")
            self.record("A1/direct-result", lambda: self.receipt(self.post("A1/direct-result", b'{"jsonrpc":"2.0","id":"direct-result","result":{}}')), True)
            self.record("A2/direct-error", lambda: self.receipt(self.post("A2/direct-error", b'{"jsonrpc":"2.0","id":"direct-error","error":{"code":-32603,"message":"synthetic"}}', extra=(("MCP-Protocol-Version", "2025-03-26"),))), True)
            self.record("C1/client-ping", lambda: require(self.success(self.post("C1/client-ping", request("ping", "client-ping")), "client-ping") == {}, "client ping result changed"))
            for name, raw in INVALID.items():
                def invalid(name=name, raw=raw):
                    value = self.post("C3/" + name, raw)
                    body = json.loads(raw_bytes(value["response"]))
                    code = -32700 if name in ("malformed", "utf8", "trailing") else -32600
                    require(value["status"] == 400 and body.get("error", {}).get("code") == code, "invalid response changed rejection")
                self.record("C3/" + name, invalid)
            for name in ("header-result", "header-error", "body-result"):
                def modern_response(name=name):
                    value = {"jsonrpc": "2.0", "id": "modern-reply", "error" if name == "header-error" else "result": {"code": -1, "message": "synthetic"} if name == "header-error" else {}}
                    if name == "body-result":
                        value["params"] = {"_meta": META}
                    exchange = self.post("C4/" + name, dump(value), extra=() if name == "body-result" else (("MCP-Protocol-Version", MODERN), ("Mcp-Method", "tools/list")))
                    require(target_rejection(exchange), "Modern response rejection changed")
                self.record("C4/" + name, modern_response)
            for name, method in (("discover", "server/discover"), ("list", "tools/list"), ("call", "tools/call")):
                def modern_control(name=name, method=method):
                    params = {"name": "echo", "arguments": {}} if name == "call" else {}
                    extra = [("MCP-Protocol-Version", MODERN), ("Mcp-Method", method)] + ([("Mcp-Name", "echo")] if name == "call" else [])
                    result = self.success(self.post("C5/" + name, request(method, "modern-" + name, params, True), extra=extra), "modern-" + name)
                    check_result(name, result, modern=True)
                self.record("C5/" + name, modern_control)
            stream = Stream("legacy-ack-sse")
            deadline = stream.deadline
            endpoint_frame = stream.next(lambda row: row["event"] == "endpoint", deadline)
            endpoint = raw_bytes(endpoint_frame["data"]).decode()
            parsed = urlsplit(endpoint)
            query = parse_qsl(parsed.query, keep_blank_values=True)
            require(not parsed.scheme and not parsed.netloc and parsed.path == "/mcp/legacy" and len(query) == 1 and query[0][0] == "sessionId" and query[0][1], "native endpoint/session binding invalid")
            self.bindings = {"endpoint": endpoint, "sessionId": query[0][1], "pings": []}
            self.record("C2/initialize", lambda: require(self.success(self.post("C2/initialize", request("initialize", "initialize", {"protocolVersion": "2025-03-26", "capabilities": {}, "clientInfo": {"name": "synthetic", "version": "1"}}), endpoint=endpoint, port=SESSION), "initialize").get("protocolVersion") == "2025-03-26", "Legacy initialize changed"))
            self.record("C2/initialized", lambda: self.receipt(self.post("C2/initialized", b'{"jsonrpc":"2.0","method":"notifications/initialized"}', endpoint=endpoint, port=SESSION)))
            # Drain concurrently via Stream, bind both actual native requests,
            # and submit every reply even when the affected assertions fail.
            acknowledgements = []
            for index in range(2):
                ping = stream.next(lambda row: row["event"] == "message" and json.loads(raw_bytes(row["data"])).get("method") == "ping", deadline)
                value = json.loads(raw_bytes(ping["data"]))
                require(value.get("jsonrpc") == "2.0" and isinstance(value.get("id"), str) and set(value) == {"jsonrpc", "id", "method"}, "unexpected native ping shape")
                reply = dump({"jsonrpc": "2.0", "id": value["id"], "result": {}})
                self.bindings["pings"].append({"frame": stream.frames.index(ping), "id": value["id"], "replySequence": self.sequence + 1})
                acknowledgements.append(self.post("A3/native-heartbeats", reply, endpoint=endpoint, port=SESSION))
            self.record("A3/native-heartbeats", lambda: [self.receipt(value) for value in acknowledgements], True)
            # A3 POSTs precede record(), so qualify its red only with the two
            # measured exchanges rather than a manufactured positive receipt.
            row = self.rows[-1]
            row["validRedReproduction"] = self.role == "affected" and row["status"] == "FAIL" and len(acknowledgements) == 2 and all(target_rejection(value) for value in acknowledgements)
            for name, method in (("list", "tools/list"), ("call", "tools/call")):
                def legacy_control(name=name, method=method):
                    result = self.success(self.post("C2/" + name, request(method, "legacy-" + name, {"name": "echo", "arguments": {}} if name == "call" else {}), endpoint=endpoint, port=SESSION), "legacy-" + name)
                    check_result(name, result)
                self.record("C2/" + name, legacy_control)
            time.sleep(0.25)  # bounded drain for asynchronous native publications
        except Exception as error:
            self.startup = type(error).__name__ + ": " + str(error)
        finally:
            if stream:
                try:
                    stream.close()
                    self.closed = True
                except Exception as error:
                    self.startup = self.startup or str(error)
                self.frames = stream.frames
                if stream.errors:
                    self.startup = self.startup or "; ".join(stream.errors)
            try:
                connection = http.client.HTTPConnection("backend-primary", 8080, timeout=5)
                connection.request("GET", "/__compat_state")
                response = connection.getresponse()
                ledger = json.loads(response.read())
                connection.close()
            except Exception as error:
                ledger = {}
                self.startup = self.startup or str(error)
            snapshot = {"mode": "legacy-response-ack", "variant": self.role, "expectedCases": cases(), "cases": self.rows,
                        "exchanges": self.exchanges, "frames": self.frames, "bindings": self.bindings,
                        "backend": ledger, "readerClosed": self.closed, "startupError": self.startup}
            snapshot["counts"] = observed_counts(snapshot)
            faults = observation_errors(snapshot)
            snapshot["observationErrors"] = faults
            snapshot["passed"] = not faults and all(row["status"] == "PASS" for row in self.rows)
            (self.out / f"legacy-response-{self.role}.json").write_text(json.dumps(snapshot, indent=2, sort_keys=True) + "\n")
        return 0 if snapshot["passed"] else 1


def check_result(name, result, modern=False):
    expected_meta = {"io.modelcontextprotocol/serverInfo": {"name": "legacy-ack", "version": "1.0.0"}}
    core = dict(result)
    if modern:
        require(core.pop("_meta", None) == expected_meta and core.pop("resultType", None) == "complete", "Modern result metadata changed")
        if name in ("discover", "list"):
            require(core.pop("ttlMs", None) == 0 and core.pop("cacheScope", None) == "private", "Modern cache policy changed")
    if name == "discover":
        require(core == {"supportedVersions": ["2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25", MODERN], "capabilities": {"tools": {}}}, "discovery result changed")
    elif name == "list":
        require(core == {"tools": [{"name": "echo", "description": "Synthetic echo", "inputSchema": {"type": "object", "properties": {}}}]}, "tool list changed")
    else:
        require(core.get("isError", False) is False and len(core.get("content", [])) == 1 and core["content"][0].get("type") == "text", "tool call result changed")
        require(json.loads(core["content"][0]["text"]) == {"ok": True}, "REST echo result changed")


def target_rejection(exchange):
    try:
        value = json.loads(raw_bytes(exchange["response"]))
        return exchange["status"] == 400 and value.get("error", {}).get("code") == -32600
    except (ValueError, KeyError, TypeError):
        return False


def normalized(snapshot):
    """Normalize only the bound endpoint/session and top-level ping-reply ID."""
    bindings = snapshot["bindings"]
    endpoint = bindings["endpoint"]
    result = []
    ping_sequences = {row["replySequence"]: row for row in bindings["pings"]}
    for index, exchange in enumerate(snapshot["exchanges"], 1):
        raw = raw_bytes(exchange["request"])
        if index in ping_sequences:
            ping = ping_sequences[index]
            expected = dump({"jsonrpc": "2.0", "id": ping["id"], "result": {}})
            require(raw == expected and exchange["case"] == "A3/native-heartbeats", "ping/reply ID binding altered")
            raw = dump({"jsonrpc": "2.0", "id": "<native-ping-" + str(list(ping_sequences).index(index) + 1) + ">", "result": {}})
        path = exchange["path"]
        if exchange["port"] == SESSION:
            require(path == endpoint, "POST endpoint differs from observed endpoint")
            path = "/mcp/legacy?sessionId=<native-session>"
        result.append({"case": exchange["case"], "port": exchange["port"], "path": path, "headers": exchange["headers"], "request": raw_record(raw)})
    return result


def normalized_frames(snapshot):
    """Compare remaining SSE bytes/order after the same closed identity binding."""
    endpoint = snapshot["bindings"]["endpoint"].encode()
    pings = {row["frame"]: (index, row) for index, row in enumerate(snapshot["bindings"]["pings"], 1)}
    # Only the two measured affected acknowledgement errors are expected to
    # disappear. All other native/control/business events retain their bytes.
    target_errors = Counter(raw_bytes(row["response"]) for row in snapshot["exchanges"]
                            if row["case"] == "A3/native-heartbeats" and target_rejection(row))
    result = []
    for index, frame in enumerate(snapshot["frames"]):
        wire, data = raw_bytes(frame["wire"]), raw_bytes(frame["data"])
        if frame["event"] == "endpoint":
            require(data == endpoint and wire.count(endpoint) == 1, "endpoint wire binding altered")
            wire = wire.replace(endpoint, b"/mcp/legacy?sessionId=<native-session>", 1)
        elif index in pings:
            order, ping = pings[index]
            token = dump(ping["id"])
            require(wire.count(token) == 1, "native ping token wire binding altered")
            wire = wire.replace(token, dump("<native-ping-" + str(order) + ">"), 1)
        elif target_errors[data]:
            target_errors[data] -= 1
            continue
        result.append(raw_record(wire))
    require(not any(target_errors.values()), "affected acknowledgement error event missing")
    return result


def observed_counts(snapshot):
    """Keep native heartbeats, local controls, tool replies and REST work separate."""
    session = [row for row in snapshot["exchanges"] if row["port"] == SESSION and raw_bytes(row["response"])]
    return {"nativeHeartbeats": len(snapshot["bindings"].get("pings", [])),
            "nativeControlReplies": sum(row["case"] in ("C2/initialize", "C2/initialized") for row in session),
            "nativeBusinessReplies": sum(row["case"] in ("C2/list", "C2/call") for row in session),
            "nativeResponseErrors": sum("error" in json.loads(raw_bytes(row["data"])) for row in snapshot["frames"] if row["event"] == "message"),
            "backendBusiness": len(snapshot["backend"].get("events", []))}


def observation_errors(snapshot):
    errors = []
    try:
        require(snapshot["variant"] in ("affected", "candidate") and not snapshot["startupError"], "startup/environment failure")
        require(snapshot["expectedCases"] == cases() and Counter(row["id"] for row in snapshot["cases"]) == Counter(row["id"] for row in cases()), "missing/duplicate case registry")
        require(snapshot["readerClosed"] is True, "SSE reader/process incomplete")
        exchanges = snapshot["exchanges"]
        require(Counter(row["case"] for row in exchanges) == Counter({row["id"]: 2 if row["id"] == "A3/native-heartbeats" else 1 for row in cases()}), "missing/duplicate exchanges")
        require(len({row["accessRequestId"] for row in exchanges}) == len(exchanges), "duplicate access identity")
        bindings = snapshot["bindings"]
        parsed = urlsplit(bindings["endpoint"])
        require(bindings["sessionId"] and bindings["endpoint"] == "/mcp/legacy?" + urlencode({"sessionId": bindings["sessionId"]}) and not parsed.fragment, "session binding altered")
        endpoints = [frame for frame in snapshot["frames"] if frame["event"] == "endpoint"]
        require(len(endpoints) == 1 and raw_bytes(endpoints[0]["data"]).decode() == bindings["endpoint"], "endpoint event binding altered")
        require(len(bindings["pings"]) == 2 and len({row["id"] for row in bindings["pings"]}) == 2, "two distinct native pings missing")
        for ping in bindings["pings"]:
            frame = snapshot["frames"][ping["frame"]]
            body = json.loads(raw_bytes(frame["data"]))
            require(body == {"jsonrpc": "2.0", "id": ping["id"], "method": "ping"}, "native ping binding altered")
        normalized(snapshot)
        expected_rpc = []
        for exchange in exchanges:
            if exchange["port"] == SESSION and raw_bytes(exchange["response"]):
                expected_rpc.append(raw_bytes(exchange["response"]))
        messages = [raw_bytes(frame["data"]) for frame in snapshot["frames"] if frame["event"] == "message"]
        heartbeats = [raw_bytes(snapshot["frames"][ping["frame"]]["data"]) for ping in bindings["pings"]]
        require(all(frame["event"] in ("endpoint", "message") for frame in snapshot["frames"]), "unexpected SSE event")
        require(Counter(messages) == Counter(expected_rpc + heartbeats), "SSE error/reply pollution or missing native/control/business event")
        require(all(raw_bytes(frame["wire"]) and raw_bytes(frame["data"]) in raw_bytes(frame["wire"]) for frame in snapshot["frames"]), "SSE raw wire altered")
        expected_counts = {"nativeHeartbeats": 2, "nativeControlReplies": 1, "nativeBusinessReplies": 2,
                           "nativeResponseErrors": 2 if snapshot["variant"] == "affected" else 0, "backendBusiness": 2}
        require(snapshot["counts"] == observed_counts(snapshot) == expected_counts, "native/control/business counts changed")
        events = snapshot["backend"]["events"]
        require(len(events) == 2 and Counter(row["case"] for row in events) == Counter({"C2/call": 1, "C5/call": 1}), "unexpected business/control dispatch")
        require(all(row["method"] == "GET" and row["path"] == "/unified/rest/legacy-ack" and row["accepted"] for row in events), "business fixture changed")
        targets = {row["id"] for row in cases() if row["target"]}
        for row in snapshot["cases"]:
            red = snapshot["variant"] == "affected" and row["id"] in targets
            require(row["status"] == ("FAIL" if red else "PASS") and row.get("validRedReproduction") is red, "invalid red/control outcome: " + row["id"])
            if red:
                require(all(target_rejection(value) for value in exchanges if value["case"] == row["id"]), "target failure is not actual 400/-32600")
            if row["id"] in targets and not red:
                require(all(value["status"] == 202 and raw_bytes(value["response"]) == b"" for value in exchanges if value["case"] == row["id"]), "candidate acknowledgement missing")
    except (AssertionError, KeyError, ValueError, TypeError, IndexError) as error:
        errors.append(str(error))
    return errors


def final_errors(out, inputs):
    from run_routing import sha
    errors = []
    try:
        require(inputs.get("mode") == "legacy-response-ack" and inputs.get("responseBaseline") == AFFECTED, "mode/affected source mismatch")
        require(not inputs.get("error") and not inputs.get("verifierFailed") and inputs.get("sourceStillClean") is True and inputs.get("tokenOverridesUnset") is True, "runner/source/token preconditions failed")
        require(inputs.get("cleanupComplete") is True and inputs.get("cleanup") == {"containers": [], "networks": [], "volumes": [], "publishedPorts": [], "readerProcesses": []}, "cleanup incomplete")
        require(inputs["verifierExits"] == {"affected": 1, "candidate": 0}, "actual affected red/candidate green exits missing")
        require(len(inputs["images"]) == 3 and all("@sha256:" in ref for ref in inputs["images"]), "immutable image triplet missing")
        require(set(inputs["imageRoles"]) == {"gateway", "backend", "redis"} and set(inputs["imageRoles"].values()) == set(inputs["images"]), "image role binding mismatch")
        require(inputs["native"]["path"] == NATIVE_PATH and len(inputs["native"]["sha256"]) == 64, "measured native library missing")
        snapshots = []
        for role, revision in (("affected", AFFECTED), ("candidate", inputs["candidate"])):
            version = inputs["versions"][role]
            require(version["source"] == revision and set(version["wasm"]) == {"plain"} and version["wasm"]["plain"] == sha(out / f"plain-{role}.wasm"), "source/plain Wasm binding mismatch")
            require(inputs["loaded"][role] == {"wasm": version["wasm"]["plain"], "native": inputs["native"]["sha256"]}, "loaded native/Wasm hash mismatch")
            require(inputs["configs"][role] == sha(out / f"envoy-{role}.yaml"), "config hash mismatch")
            snapshot = json.loads((out / f"legacy-response-{role}.json").read_text())
            require(snapshot["variant"] == role, "snapshot role mismatch")
            errors.extend(role + ": " + error for error in observation_errors(snapshot))
            access = [value["accessRequestId"] for value in snapshot["exchanges"]] + ["legacy-ack-sse"]
            log = (out / f"gateway-{role}.log").read_text()
            require(all(log.count("access request_id=" + identity + " ") == 1 for identity in access), "gateway access identity absent/duplicated")
            snapshots.append(snapshot)
        require(inputs["configs"]["affected"] == inputs["configs"]["candidate"], "role configs differ")
        before, after = (inputs["versions"][role] for role in ("affected", "candidate"))
        require(before["go"] == after["go"] and before["moduleFiles"] == after["moduleFiles"], "compiler/module inputs differ")
        require(normalized(snapshots[0]) == normalized(snapshots[1]), "requests differ beyond bound native identities")
        require(normalized_frames(snapshots[0]) == normalized_frames(snapshots[1]), "SSE bytes/order differ beyond bound native identities and target errors")
        left = {row["case"]: row for row in snapshots[0]["exchanges"]}
        right = {row["case"]: row for row in snapshots[1]["exchanges"]}
        for case in cases():
            if not case["target"]:
                require((left[case["id"]]["status"], left[case["id"]]["response"]) == (right[case["id"]]["status"], right[case["id"]]["response"]), "control response differs: " + case["id"])
        for name, digest in inputs["harness"].items():
            require(sha(out / "harness" / name) == digest, "frozen harness changed: " + name)
    except (AssertionError, KeyError, OSError, ValueError, TypeError) as error:
        errors.append(str(error))
    return errors


def run(args):
    """Build separate plain archives, then run the existing bounded Compose loop."""
    # Source discovery and command orchestration are host-only. The verifier
    # imports Fixture in a Python image with neither Git nor a source checkout.
    from run_routing import ROOT, recorded_run, sha
    from run_compat import clean_environment, source_is_clean
    candidate = subprocess.check_output(["git", "rev-parse", args.candidate + "^{commit}"], cwd=ROOT, text=True).strip()
    require(args.response_baseline == AFFECTED and source_is_clean(candidate), "focused source must be exact affected/clean candidate HEAD")
    out = Path(args.output).resolve()
    out.mkdir(parents=True, exist_ok=True)
    require(not any(out.iterdir()), "evidence directory must be empty")
    harness = out / "harness"
    shutil.copytree(Path(__file__).resolve().parent, harness, ignore=shutil.ignore_patterns("__pycache__", "*.pyc"))
    project = "mcp-legacy-ack-" + uuid.uuid4().hex[:10]
    env = clean_environment({"RUNTIME_EVIDENCE": str(out), "COMPOSE_PROJECT_NAME": project, "RUNTIME_SOURCE_ROOT": str(ROOT),
                             "RUNTIME_LIFECYCLE_DIAGNOSTICS": str(out / "lifecycle-diagnostics.log")})
    inputs = {"mode": "legacy-response-ack", "responseBaseline": AFFECTED, "candidate": candidate, "project": project,
              "tokenOverridesUnset": True, "versions": {}, "images": {}, "imageRoles": {}, "configs": {}, "loaded": {}, "verifierExits": {},
              "harness": {str(path.relative_to(harness)): sha(path) for path in sorted(harness.rglob("*")) if path.is_file()},
              "python": sys.version, "command": sys.argv, "cleanupComplete": False}
    log = (out / "commands.jsonl").open("w")
    failure, verifier_failed = None, False

    def command(argv, **options):
        return recorded_run(argv, log, env=env, **options)

    def pinned(role, image):
        inspection = json.loads(command(["podman", "image", "inspect", image], stderr_target=out / f"image-{role}.stderr.log").stdout)[0]
        reference = image if "@sha256:" in image else next((value for value in inspection.get("RepoDigests", []) if "@sha256:" in value), None)
        require(reference, "cannot freeze immutable " + role + " image")
        inputs["images"][reference] = inspection
        inputs["imageRoles"][role] = reference
        return reference

    compose_files = ["-f", str(harness / "compose.yaml"), "-f", str(out / "compose-legacy-response.json")]

    def compose(*arguments, **options):
        return command(["podman", "compose", *compose_files, "-p", project, *arguments], cwd=harness, **options)

    def lifecycle(action):
        script = 'compose_runtime() { podman compose -f "$COMPAT_COMPOSE" -f "$COMPAT_OVERRIDE" -p "$COMPOSE_PROJECT_NAME" "$@"; }; . "$COMPAT_LIFECYCLE"; ' + action
        return recorded_run(["bash", "-c", script], log, cwd=harness,
                            env={**env, "COMPAT_COMPOSE": str(harness / "compose.yaml"), "COMPAT_OVERRIDE": str(out / "compose-legacy-response.json"), "COMPAT_LIFECYCLE": str(harness / "lifecycle.sh")})

    try:
        gateway, backend, redis = (pinned(role, image) for role, image in (("gateway", args.gateway_image), ("backend", args.backend_image), ("redis", args.redis_image)))
        inputs["engine"] = command(["podman", "version"]).stdout
        native_hash = command(["podman", "run", "--rm", "--network", "none", "--entrypoint", "/bin/sh", gateway, "-c", "sha256sum " + NATIVE_PATH], target=out / "native-preflight.log", stderr_target=out / "native-preflight.stderr.log").stdout.split()[0]
        require(len(native_hash) == 64 and all(value in "0123456789abcdef" for value in native_hash), "native library hash unavailable")
        inputs["native"] = {"path": NATIVE_PATH, "sha256": native_hash}
        # Normal Compose paths/images stay unchanged; Redis has no host port or
        # persistent volume and this random project owns every native session.
        override = {"services": {"gateway": {"image": gateway, "depends_on": {"redis": {"condition": "service_healthy"}}},
                    "backend-primary": {"image": backend}, "backend-secondary": {"image": backend}, "verifier": {"image": backend},
                    "redis": {"image": redis, "command": ["redis-server", "--save", "", "--appendonly", "no"],
                              "healthcheck": {"test": ["CMD", "redis-cli", "ping"], "interval": "1s", "timeout": "2s", "retries": 30}}}}
        (out / "compose-legacy-response.json").write_text(json.dumps(override, indent=2) + "\n")
        config(out)
        for role, revision in (("affected", AFFECTED), ("candidate", candidate)):
            with tempfile.TemporaryDirectory(prefix="legacy-ack-source-", dir=out) as temporary:
                source = Path(temporary)
                archive = source / "source.tar"
                command(["git", "archive", "--format=tar", "--output=" + str(archive), revision, "plugins/wasm-go"], cwd=ROOT)
                with tarfile.open(archive) as bundle:
                    bundle.extractall(source, filter="data")
                extension = source / "plugins/wasm-go/extensions/mcp-server"
                version = {"source": revision, "go": command(["go", "version"], cwd=extension, stderr_target=out / f"go-{role}.stderr.log").stdout.strip(), "wasm": {},
                           "moduleFiles": {str(path.relative_to(source)): sha(path) for path in sorted(source.rglob("go.*")) if path.name in ("go.mod", "go.sum")}}
                inputs["versions"][role] = version
                command(["go", "list", "-m", "-json", "all"], cwd=extension, target=out / f"modules-{role}.json", stderr_target=out / f"modules-{role}.stderr.log")
                wasm = out / f"plain-{role}.wasm"
                recorded_run(["go", "build", "-trimpath", "-buildmode=c-shared", "-o", str(wasm), "."], log, cwd=extension,
                             env={**env, "GOOS": "wasip1", "GOARCH": "wasm"}, target=out / f"build-plain-{role}.log")
                version["wasm"]["plain"] = sha(wasm)
            shutil.copyfile(wasm, out / "plugin.wasm")
            shutil.copyfile(out / "envoy.yaml", out / f"envoy-{role}.yaml")
            inputs["configs"][role] = sha(out / f"envoy-{role}.yaml")
            compose("--profile", "verify", "config", "--format", "json", target=out / f"compose-{role}.json", stderr_target=out / f"compose-{role}.stderr.log")
            resolved = json.loads((out / f"compose-{role}.json").read_text())
            require(all(not value.get("ports") for value in resolved["services"].values()), "focused runtime must expose no host ports")
            compose("up", "-d", "backend-primary", "backend-secondary", "redis")
            lifecycle("reset_primary_backend")
            compose("up", "-d", "gateway")
            # Measure the actual mounted Wasm and same native binary inside each
            # running role before accepting any product observation.
            # Compose provider diagnostics belong to stderr evidence, never to
            # the two unmodified sha256sum records consumed by this parser.
            measured = compose("exec", "-T", "gateway", "/bin/sh", "-c", "sha256sum /runtime/plugin.wasm " + NATIVE_PATH,
                               target=out / f"loaded-{role}.txt", stderr_target=out / f"loaded-{role}.stderr.log").stdout.splitlines()
            require(len(measured) == 2, "loaded module/native hash unavailable")
            inputs["loaded"][role] = {"wasm": measured[0].split()[0], "native": measured[1].split()[0]}
            compose("exec", "-T", "gateway", "/usr/local/bin/envoy", "--version", target=out / f"envoy-{role}-version.txt")
            result = compose("--profile", "verify", "run", "--rm", "--no-deps", "-e", "COMPAT_VARIANT=" + role,
                             "verifier", "python", "/harness/verify_compat.py", "--legacy-response-ack", allow_failure=True, target=out / f"verify-{role}.log")
            inputs["verifierExits"][role] = result.returncode
            verifier_failed |= result.returncode != (1 if role == "affected" else 0)
            lifecycle("stop_runtime_service gateway")
            compose("logs", "--no-color", "gateway", target=out / f"gateway-{role}.log")
            compose("rm", "-f", "gateway")
            # Reset the run-owned native/Redis state between the source roles.
            lifecycle("stop_runtime_service redis")
            compose("rm", "-f", "redis")
    except Exception as error:
        failure = type(error).__name__ + ": " + str(error)
        try:
            compose("logs", "--no-color", target=out / "failure-diagnostics.log", allow_failure=True)
        except Exception:
            pass
    finally:
        try:
            cleanup = compose("--profile", "verify", "down", "--volumes", "--remove-orphans", allow_failure=True)
            inventories = {}
            for kind, arguments in (("containers", ["ps", "-a"]), ("networks", ["network", "ls"]), ("volumes", ["volume", "ls"])):
                result = command(["podman", *arguments, "--filter", "label=com.docker.compose.project=" + project, "--format", "{{.Name}}" if kind == "volumes" else "{{.ID}}"], allow_failure=True, target=out / f"cleanup-{kind}.txt", stderr_target=out / f"cleanup-{kind}.stderr.log")
                require(result.returncode == 0, "cleanup inventory failed: " + kind)
                inventories[kind] = result.stdout.split()
            # No host process/listener is created by this mode. The joined
            # reader runs only inside the --rm verifier; zero containers proves
            # no remaining run-owned reader/listener or published host binding.
            inventories.update(publishedPorts=[], readerProcesses=[])
            inputs["cleanup"] = inventories
            inputs["cleanupExitCode"] = cleanup.returncode
            inputs["cleanupComplete"] = cleanup.returncode == 0 and not any(inventories.values())
        except Exception as error:
            failure = failure or str(error)
            inputs["cleanupComplete"] = False
        inputs.update(error=failure, verifierFailed=verifier_failed, sourceStillClean=source_is_clean(candidate))
        (out / "legacy-response-inputs.json").write_text(json.dumps(inputs, indent=2, sort_keys=True) + "\n")
        errors = final_errors(out, inputs)
        log.close()
        manifest = {"mode": "legacy-response-ack", "passed": not errors, "errors": errors, "cleanupComplete": inputs["cleanupComplete"],
                    "affectedTargetsExpectedRed": not errors, "candidateTargetsPassed": not errors,
                    "checksums": {str(path.relative_to(out)): sha(path) for path in sorted(out.rglob("*")) if path.is_file() and path.name != "legacy-response-manifest.json"}}
        (out / "legacy-response-manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
    print(json.dumps({"evidence": str(out), "passed": not errors, "errors": errors}))
    return int(bool(errors))
