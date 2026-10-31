#!/usr/bin/env python3
"""Exercise Vertex conversion through a real Envoy/proxy-Wasm runtime.

Only Python's standard library is used. All HTTP connections are to loopback.
The supplied Wasm is loaded without rebuilding or altering it.
"""

import argparse
import copy
import http.client
import json
import platform
import socket
import subprocess
import sys
import threading
import time
import traceback
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlsplit


MODEL = "gemini-2.5-flash"
TOKEN = "local-fixture-dummy-key"
PROMPT = "Compare the requested cities."
SIGNATURE = "local-fixture-thought-signature"
PREFIX = "Checking 杭州 and 上海. "
SUFFIX = "Please wait."


def encoded(value):
    return json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode("utf-8")


def save_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def check(checks, name, actual, expected, kind="regression"):
    checks.append({"name": name, "pass": actual == expected, "actual": actual,
                   "expected": expected, "kind": kind})


def scenarios():
    result = []
    for name, same, stream, mixed, split, fragments, count, request_only in [
        ("json-parallel", False, False, False, False, False, 2, False),
        ("json-mixed", False, False, True, False, False, 2, False),
        ("json-same-name", True, False, True, False, False, 2, False),
        ("sse-same-event", False, True, True, False, False, 2, False),
        ("sse-split-events", False, True, True, True, False, 2, False),
        ("sse-same-name", True, True, True, True, False, 2, False),
        ("sse-http-fragments", True, True, True, True, True, 2, False),
        ("request-distinct", False, False, True, False, False, 2, True),
        ("request-same-name", True, False, True, False, False, 2, True),
        ("control-single-json", False, False, False, False, False, 1, False),
        ("control-single-sse", False, True, False, True, True, 1, False),
        ("control-text-json", False, False, False, False, False, 0, False),
        ("control-text-sse", False, True, False, True, True, 0, False),
    ]:
        calls = [
            {"name": "weather", "args": {"city": "Hangzhou"}},
            {"name": "weather" if same else "time", "args": {"city": "Shanghai"}},
        ][:count]
        result.append(dict(name=name, calls=calls, stream=stream, mixed=mixed,
                           split=split, fragments=fragments, request_only=request_only,
                           text=PREFIX + SUFFIX if mixed else ("Hello from Vertex." if not count else ""),
                           kind="stable" if name.startswith("control-") else "regression"))
    return result


def two_round_case(round_number):
    return dict(name="sse-two-tool-rounds", stream=True, mixed=False, split=False,
                fragments=False, request_only=False, text="", kind="regression",
                prompt="Read Hangzhou weather, then recheck Hangzhou and read Shanghai before comparing.",
                round_number=round_number, signature="local-fixture-signature-round-%d" % round_number,
                terminal_event=True, calls=[
                    {"name": "weather", "args": {"city": "Hangzhou"}},
                    {"name": "weather", "args": {"city": "Shanghai"}},
                ][:round_number])


def vertex_response(parts, terminal=True):
    candidate = {"index": 0, "content": {"role": "model", "parts": parts}}
    if terminal:
        candidate["finishReason"] = "STOP"
    return {"responseId": "fixture-response", "modelVersion": MODEL,
            "candidates": [candidate],
            "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5, "totalTokenCount": 15}}


def fixture_payload(case):
    parts = []
    if case["mixed"]:
        parts.append({"text": PREFIX})
    for index, call in enumerate(case["calls"]):
        part = {"functionCall": call}
        if index == 0:
            part["thoughtSignature"] = case.get("signature", SIGNATURE)
        parts.append(part)
    if case["mixed"]:
        parts.append({"text": SUFFIX})
    elif not case["calls"]:
        parts.append({"text": case["text"]})
    if not case["stream"]:
        return encoded(vertex_response(parts))
    if case.get("terminal_event"):
        events = [vertex_response(parts, terminal=False), vertex_response([])]
    elif case["split"]:
        events = [vertex_response([part], terminal=False) for part in parts]
        # The terminal event has no call: finish_reason must use prior stream state.
        events.append(vertex_response([]))
    else:
        events = [vertex_response(parts)]
    if "round_number" in case:
        for event in events:
            event["responseId"] = "fixture-round-%d" % case["round_number"]
    return b"".join(b"data: " + encoded(event) + b"\n\n" for event in events)


def tool_output(call, round_number=None):
    result = {"function": call["name"], "city": call["args"].get("city"), "value": "fixture-result"}
    if round_number is not None:
        result["round"] = round_number
    return json.dumps(result, sort_keys=True, separators=(",", ":"))


def tools_for(case):
    return [{"type": "function", "function": {"name": name, "description": "Return fixture data for a city.",
             "parameters": {"type": "object", "properties": {"city": {"type": "string"}}, "required": ["city"]}}}
            for name in dict.fromkeys(call["name"] for call in case["calls"])]


def validate_upstream(body, case, phase):
    checks = []
    contents = body.get("contents", [])
    data_fields = {"text", "inlineData", "fileData", "functionCall", "functionResponse", "executableCode", "codeExecutionResult"}
    invalid_parts = ["contents[%d].parts[%d]" % (turn, index)
                     for turn, content in enumerate(contents)
                     for index, part in enumerate(content.get("parts", []))
                     if len(data_fields.intersection(part)) != 1]
    check(checks, "every-vertex-part-has-one-data-field", invalid_parts, [], "stable")
    first = contents[0] if contents else {}
    check(checks, "initial-user-message", first,
          {"role": "user", "parts": [{"text": case.get("prompt", PROMPT)}]}, "stable")
    if case["calls"]:
        check(checks, "declared-functions", body.get("tools"),
              [{"functionDeclarations": [tool["function"] for tool in tools_for(case)]}], "stable")
    if phase == "initial":
        check(checks, "initial-turn-count", len(contents), 1, "stable")
        return checks
    kind = case["kind"]
    check(checks, "roundtrip-turn-roles", [item.get("role") for item in contents],
          ["user", "model", "user"], kind)
    model_parts = contents[1].get("parts", []) if len(contents) > 1 else []
    actual_calls = [part["functionCall"] for part in model_parts if "functionCall" in part]
    check(checks, "roundtrip-all-calls-and-arguments", actual_calls, case["calls"], kind)
    check(checks, "roundtrip-assistant-text", "".join(p.get("text", "") for p in model_parts),
          case["text"], kind)
    signatures = [part.get("thoughtSignature", "") for part in model_parts if "functionCall" in part]
    check(checks, "roundtrip-call-signatures", signatures,
          [case.get("signature", SIGNATURE)] + [""] * (len(case["calls"]) - 1), kind)
    result_turns = [item for item in contents if any("functionResponse" in p for p in item.get("parts", []))]
    check(checks, "roundtrip-results-single-user-turn", len(result_turns), 1, kind)
    results = [part["functionResponse"] for item in result_turns for part in item.get("parts", [])
               if "functionResponse" in part]
    # The client sends results in reverse order. Vertex positional pairing must
    # nevertheless follow the original calls, including two calls to one name.
    expected = [{"name": call["name"], "response": {"output": tool_output(call, case.get("round_number"))}}
                for call in case["calls"]]
    check(checks, "roundtrip-results-match-call-ids-in-original-order", results, expected, kind)
    return checks


def validate_two_round_history(body):
    checks = []
    contents = body.get("contents", [])
    check(checks, "two-round-turn-roles", [item.get("role") for item in contents],
          ["user", "model", "user", "model", "user"])
    for round_number in (1, 2):
        case = two_round_case(round_number)
        if round_number == 1:
            # A supported single-call history must survive the later tool turn.
            case["kind"] = "stable"
        start = 1 + 2 * (round_number - 1)
        turn_body = dict(body, contents=contents[:1] + contents[start:start + 2])
        for entry in validate_upstream(turn_body, case, "roundtrip"):
            checks.append(dict(entry, name="history-round-%d/%s" % (round_number, entry["name"])))
    return checks


class Fixture(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, output, cases):
        super().__init__(("127.0.0.1", 0), Handler)
        self.output, self.cases, self.records = output, cases, {}


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *args):
        pass

    def read_body(self):
        if self.headers.get("Transfer-Encoding", "").lower() != "chunked":
            return self.rfile.read(int(self.headers["Content-Length"]))
        chunks = []
        while True:
            size = int(self.rfile.readline().split(b";", 1)[0], 16)
            if not size:
                while self.rfile.readline() not in (b"\r\n", b"\n", b""):
                    pass
                return b"".join(chunks)
            chunks.append(self.rfile.read(size))
            if self.rfile.read(2) != b"\r\n":
                raise ValueError("Invalid request chunk delimiter")

    def do_POST(self):
        exchange = self.headers.get("X-Verification-Exchange", "")
        case_name = self.headers.get("X-Verification-Case", "")
        phase = self.headers.get("X-Verification-Phase", "")
        record = {"path": self.path, "headers": list(self.headers.items())}
        self.server.records[exchange] = record
        try:
            case = self.server.cases[case_name]
            directory = self.server.output / exchange
            directory.mkdir(exist_ok=True)
            raw = self.read_body()
            (directory / "upstream-request-body.txt").write_bytes(raw)
            body = json.loads(raw)
            record["body"] = body
            two_rounds = case["name"] == "sse-two-tool-rounds"
            if two_rounds and phase == "final":
                checks = validate_two_round_history(body)
            else:
                request_case = dict(case, kind="stable") if two_rounds else case
                checks = validate_upstream(body, request_case, phase)
            response_case = two_round_case(2) if two_rounds and phase == "round-2" else case
            response_stream = case["stream"] and (phase == "initial" or (two_rounds and phase == "round-2"))
            path = urlsplit(self.path)
            action = "streamGenerateContent" if response_stream else "generateContent"
            check(checks, "vertex-express-path", path.path,
                  "/v1/publishers/google/models/" + MODEL + ":" + action, "stable")
            check(checks, "dummy-key", parse_qs(path.query).get("key"), [TOKEN], "stable")
            if action == "streamGenerateContent":
                check(checks, "sse-query", parse_qs(path.query).get("alt"), ["sse"], "stable")
            record["checks"] = checks
            if any(not entry["pass"] for entry in checks):
                status = 400
                payload = encoded({"error": {"code": 400, "status": "INVALID_ARGUMENT",
                                             "message": "Local fixture rejected the converted request"}})
            else:
                status = 200
                payload = (fixture_payload(response_case) if phase == "initial" or response_stream
                           else encoded(vertex_response([{"text": "Tool results accepted."}])))
            streaming = status == 200 and response_stream
            record.update(status=status, streaming=streaming)
            save_json(directory / "upstream-request.json", record)
            (directory / "upstream-response-body.txt").write_bytes(payload)
            self.send_response(status)
            self.send_header("Content-Type", "text/event-stream" if streaming else "application/json")
            if streaming:
                self.send_header("Transfer-Encoding", "chunked")
            else:
                self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            if streaming:
                if case["fragments"]:
                    sizes = [1, 2, 7, 3, 13, 5, 29]
                    chunks, offset = [], 0
                    while offset < len(payload):
                        size = sizes[len(chunks) % len(sizes)]
                        chunks.append(payload[offset:offset + size])
                        offset += size
                else:
                    chunks = [event + b"\n\n" for event in payload.split(b"\n\n") if event]
                timeline = []
                started = time.monotonic()
                for chunk in chunks:
                    self.wfile.write(("%x\r\n" % len(chunk)).encode() + chunk + b"\r\n")
                    self.wfile.flush()
                    timeline.append({"bytes": len(chunk), "seconds": time.monotonic() - started})
                    time.sleep(0.002 if case["fragments"] else 0.02)
                self.wfile.write(b"0\r\n\r\n")
                self.wfile.flush()
                save_json(directory / "upstream-http-chunks.json", timeline)
            else:
                self.wfile.write(payload)
        except Exception:
            record["error"] = traceback.format_exc()
            self.close_connection = True


def free_port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def envoy_config(wasm, listener, admin, upstream):
    address = lambda port: {"socket_address": {"address": "127.0.0.1", "port_value": port}}
    plugin = {"activeProviderId": "fixture-vertex", "providers": [
        {"id": "fixture-vertex", "type": "vertex", "apiTokens": [TOKEN]}]}
    http = {"@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
            "stat_prefix": "vertex_verification", "codec_type": "AUTO",
            "route_config": {"name": "fixture", "virtual_hosts": [{"name": "fixture", "domains": ["*"],
                "routes": [{"match": {"prefix": "/"}, "route": {"cluster": "fixture", "timeout": "20s"}}]}]},
            "access_log": [{"name": "envoy.access_loggers.stdout", "typed_config": {
                "@type": "type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog"}}],
            "http_filters": [{"name": "envoy.filters.http.wasm", "typed_config": {
                "@type": "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm", "config": {
                    "name": "vertex-verification", "root_id": "vertex-verification",
                    "configuration": {"@type": "type.googleapis.com/google.protobuf.StringValue", "value": json.dumps(plugin)},
                    "vm_config": {"vm_id": "vertex-verification", "runtime": "envoy.wasm.runtime.v8",
                                  "code": {"local": {"filename": str(wasm)}}}}}},
                {"name": "envoy.filters.http.router", "typed_config": {
                    "@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}]}
    return {"admin": {"address": address(admin)}, "static_resources": {
        "listeners": [{"name": "vertex_verification", "address": address(listener), "filter_chains": [
            {"filters": [{"name": "envoy.filters.network.http_connection_manager", "typed_config": http}]}]}],
        "clusters": [{"name": "fixture", "type": "STATIC", "connect_timeout": "2s", "load_assignment": {
            "cluster_name": "fixture", "endpoints": [{"lb_endpoints": [{"endpoint": {"address": address(upstream)}}]}]}}]}}


def request(port, method, path, body=None, headers=None):
    connection = http.client.HTTPConnection("127.0.0.1", port, timeout=25)
    try:
        connection.request(method, path, body=body, headers=headers or {})
        response = connection.getresponse()
        chunks, timeline = [], []
        started = time.monotonic()
        while True:
            chunk = response.read1(4096)
            if not chunk:
                break
            chunks.append(chunk)
            timeline.append({"bytes": len(chunk), "seconds": time.monotonic() - started})
        return response.status, list(response.getheaders()), b"".join(chunks), timeline
    finally:
        connection.close()


def canonical_assistant(case):
    calls = []
    for index, call in enumerate(case["calls"]):
        item = {"id": "fixture-call-" + str(index), "type": "function",
                "function": {"name": call["name"], "arguments": encoded(call["args"]).decode()}}
        if index == 0:
            # Exercise the OpenAI-compatible extra_content form on requests.
            item["extra_content"] = {"google": {"thought_signature": SIGNATURE}}
        calls.append(item)
    return {"role": "assistant", "content": case["text"], "tool_calls": calls}


def collect_assistant(raw, streaming, checks, kind="regression"):
    if not streaming:
        response = json.loads(raw)
        choice = response["choices"][0]
        return choice["message"], [choice.get("finish_reason")]
    assistant, calls, finish, indexes, done = {"role": "assistant", "content": ""}, {}, [], [], 0
    stream = raw.decode("utf-8").replace("\r\n", "\n")
    if not stream.endswith("\n\n"):
        raise ValueError("SSE stream ended with an incomplete event")
    for event in stream.split("\n\n"):
        if not event.strip():
            continue
        data = "\n".join(line[5:].lstrip(" ") for line in event.splitlines() if line.startswith("data:"))
        if done:
            raise ValueError("SSE data arrived after [DONE]; a real client would have stopped reading")
        if data == "[DONE]":
            done += 1
            continue
        if not data:
            raise ValueError("Unexpected non-data SSE event: " + event)
        item = json.loads(data)
        for choice in item.get("choices", []):
            if choice.get("index", 0) != 0:
                raise ValueError("Unexpected candidate index")
            delta = choice.get("delta") or {}
            assistant["content"] += delta.get("content") or ""
            if choice.get("finish_reason"):
                finish.append(choice["finish_reason"])
            for fragment in delta.get("tool_calls") or []:
                index = fragment.get("index", 0)
                indexes.append(index)
                target = calls.setdefault(index, {"function": {"name": "", "arguments": ""}})
                for key in ("id", "type", "thought_signature", "extra_content"):
                    if key in fragment:
                        if key == "id" and key in target and target[key] != fragment[key]:
                            check(checks, "sse-call-id-stable", fragment[key], target[key], kind)
                        target[key] = fragment[key]
                function = fragment.get("function") or {}
                for key in ("name", "arguments"):
                    target["function"][key] += function.get(key) or ""
    check(checks, "sse-done-exactly-once", done, 1, "stable")
    # Complete native functionCall objects are distinct calls, even across events.
    check(checks, "sse-monotonic-call-indexes", indexes, list(range(len(indexes))), kind)
    if calls:
        assistant["tool_calls"] = [calls[index] for index in sorted(calls)]
    return assistant, finish


def validate_assistant(assistant, finish, case, checks):
    kind = case["kind"]
    check(checks, "client-text-parts", assistant.get("content") or "", case["text"], kind)
    calls = assistant.get("tool_calls") or []
    actual = []
    for item in calls:
        function = item.get("function") or {}
        try:
            args = json.loads(function.get("arguments", ""))
        except (ValueError, TypeError):
            args = {"invalid_json": function.get("arguments")}
        actual.append({"name": function.get("name"), "args": args})
    check(checks, "client-all-calls-and-arguments", actual, case["calls"], kind)
    ids = [call.get("id") for call in calls]
    check(checks, "client-nonempty-unique-call-ids", bool(all(ids) and len(set(ids)) == len(ids)), True, kind)
    signatures = [call.get("thought_signature") or call.get("extra_content", {}).get("google", {}).get("thought_signature", "")
                  for call in calls]
    check(checks, "client-call-signatures", signatures,
          ([case.get("signature", SIGNATURE)] + [""] * (len(case["calls"]) - 1)) if case["calls"] else [], kind)
    if case["calls"]:
        check(checks, "client-finish-reason-tool-calls", finish, ["tool_calls"])
    else:
        # Existing streaming text may omit finish_reason; non-tool finish
        # normalization is not the behavior this regression test establishes.
        valid = len(finish) <= 1 if case["stream"] else len(finish) == 1
        check(checks, "client-text-finish-reason", valid and all(reason in ("stop", "STOP") for reason in finish), True, "stable")


def followup(case, assistant, history=None):
    messages = list(history) if history is not None else [{"role": "user", "content": case.get("prompt", PROMPT)}]
    messages.append(assistant)
    for call in reversed(assistant.get("tool_calls") or []):
        function = call.get("function") or {}
        try:
            output = tool_output({"name": function.get("name"), "args": json.loads(function.get("arguments", ""))},
                                 case.get("round_number"))
        except (ValueError, TypeError, AttributeError):
            output = "invalid-tool-arguments"
        messages.append({"role": "tool", "tool_call_id": call.get("id", ""), "content": output})
    return {"model": MODEL, "messages": messages, "stream": False, "tools": tools_for(case)}


def exchange(output, fixture, port, case, repeat, phase, body):
    name = "%02d-%s-%s" % (repeat, case["name"], phase)
    directory = output / name
    directory.mkdir()
    save_json(directory / "client-request.json", body)
    headers = {"Content-Type": "application/json", "X-Verification-Exchange": name,
               "X-Verification-Case": case["name"], "X-Verification-Phase": phase}
    status, response_headers, raw, timeline = request(port, "POST", "/v1/chat/completions", encoded(body), headers)
    (directory / "client-response-body.txt").write_bytes(raw)
    save_json(directory / "client-response.json", {"status": status, "headers": response_headers, "read_chunks": timeline})
    # The fixture writes its checks before sending any bytes.
    upstream = fixture.records.get(name)
    if not upstream or "checks" not in upstream:
        raise RuntimeError("Request did not reach fixture: " + name + "; " + repr(upstream))
    checks = copy.deepcopy(upstream["checks"])
    kind = "stable" if phase == "initial" else case["kind"]
    check(checks, "client-http-status", status, 200, kind)
    return raw, checks, directory


def run_two_tool_rounds(output, fixture, port, case, repeat, checks):
    history = [{"role": "user", "content": case["prompt"]}]
    previous_ids = []
    for round_number in (1, 2):
        round_case = two_round_case(round_number)
        if round_number == 1:
            round_case["kind"] = "stable"
        phase = "initial" if round_number == 1 else "round-2"
        body = {"model": MODEL, "messages": history, "stream": True, "tools": tools_for(round_case)}
        raw, round_checks, directory = exchange(output, fixture, port, case, repeat, phase, body)
        assistant, finish = collect_assistant(raw, True, round_checks, round_case["kind"])
        validate_assistant(assistant, finish, round_case, round_checks)
        ids = [call.get("id") for call in assistant.get("tool_calls") or []]
        if round_number == 2:
            check(round_checks, "new-tool-round-has-new-ids", [value for value in ids if value in previous_ids], [])
        previous_ids.extend(ids)
        save_json(directory / "assembled-assistant.json", assistant)
        # Replay only calls actually returned by this stream. In particular, do
        # not repair baseline's missing second call using the fixture's oracle.
        history = followup(round_case, assistant, history)["messages"]
        for entry in round_checks:
            checks.append(dict(entry, name="round-%d/%s" % (round_number, entry["name"])))
    body = {"model": MODEL, "messages": history, "stream": False, "tools": tools_for(case)}
    raw, final_checks, directory = exchange(output, fixture, port, case, repeat, "final", body)
    checks.extend(final_checks)
    if all(entry["pass"] for entry in final_checks):
        final, _ = collect_assistant(raw, False, checks)
        check(checks, "client-two-tool-rounds-completed", final.get("content"), "Tool results accepted.", "stable")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    default_cases = scenarios()
    available_cases = {case["name"]: case for case in default_cases + [two_round_case(1)]}
    parser.add_argument("--envoy", required=True, type=Path)
    parser.add_argument("--wasm", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--expect", required=True, choices=("baseline", "fixed"))
    parser.add_argument("--repeat", type=int, default=3)
    parser.add_argument("--scenario", action="append", choices=list(available_cases),
                        help="Run only the named scenario; repeat this option to select several. Default: original 13 scenarios.")
    args = parser.parse_args()
    if args.repeat < 1:
        parser.error("--repeat must be at least 1")
    args.envoy, args.wasm, args.output = args.envoy.resolve(), args.wasm.resolve(), args.output.resolve()
    if not args.envoy.is_file() or not args.wasm.is_file():
        parser.error("--envoy and --wasm must refer to existing files")
    # Never overwrite a prior run's evidence, including failed runs.
    args.output.mkdir(parents=True, exist_ok=False)
    matrix = ([case for name, case in available_cases.items() if name in args.scenario]
              if args.scenario else default_cases)
    fixture = Fixture(args.output, {case["name"]: case for case in matrix})
    thread = threading.Thread(target=fixture.serve_forever, name="vertex-fixture", daemon=True)
    thread.start()
    listener, admin = free_port(), free_port()
    while admin == listener:
        admin = free_port()
    config = args.output / "envoy.json"
    save_json(config, envoy_config(args.wasm, listener, admin, fixture.server_port))
    command = [str(args.envoy), "-c", str(config), "--concurrency", "1", "--disable-hot-restart", "--log-level", "info"]
    summary = {"expect": args.expect, "repeat": args.repeat, "cases": [], "errors": [], "command": command}
    process = None
    log_file = None
    try:
        version = subprocess.run([str(args.envoy), "--version"], capture_output=True, text=True, timeout=20, check=True)
        save_json(args.output / "versions.json", {"envoy": version.stdout.strip() or version.stderr.strip(),
            "python": sys.version, "platform": platform.platform(), "wasm_path": str(args.wasm),
            "wasm_bytes": args.wasm.stat().st_size, "envoy_path": str(args.envoy),
            "started_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "ports": {"envoy": listener, "admin": admin, "fixture": fixture.server_port}})
        log_file = (args.output / "envoy.log").open("wb")
        process = subprocess.Popen(command, stdout=log_file, stderr=subprocess.STDOUT)
        deadline = time.monotonic() + 60
        while True:
            if process.poll() is not None:
                raise RuntimeError("Envoy exited during startup; see envoy.log")
            try:
                if request(admin, "GET", "/ready")[0] == 200:
                    break
            except (ConnectionError, OSError, http.client.HTTPException):
                pass
            if time.monotonic() > deadline:
                raise RuntimeError("Envoy did not become ready within 60 seconds")
            time.sleep(0.1)
        for repeat in range(1, args.repeat + 1):
            for case in matrix:
                checks = []
                try:
                    if case["name"] == "sse-two-tool-rounds":
                        run_two_tool_rounds(args.output, fixture, listener, case, repeat, checks)
                    elif case["request_only"]:
                        assistant = canonical_assistant(case)
                    else:
                        body = {"model": MODEL, "messages": [{"role": "user", "content": PROMPT}], "stream": case["stream"]}
                        if case["calls"]:
                            body["tools"] = tools_for(case)
                        raw, initial_checks, directory = exchange(args.output, fixture, listener, case, repeat, "initial", body)
                        checks.extend(initial_checks)
                        assistant, finish = collect_assistant(raw, case["stream"], checks, case["kind"])
                        validate_assistant(assistant, finish, case, checks)
                        save_json(directory / "assembled-assistant.json", assistant)
                    if case["calls"] and case["name"] != "sse-two-tool-rounds":
                        raw, roundtrip_checks, directory = exchange(args.output, fixture, listener, case, repeat,
                                                                     "roundtrip", followup(case, assistant))
                        checks.extend(roundtrip_checks)
                        if all(entry["pass"] for entry in roundtrip_checks):
                            final, _ = collect_assistant(raw, False, checks)
                            check(checks, "client-roundtrip-completed", final.get("content"), "Tool results accepted.", "stable")
                except Exception:
                    summary["errors"].append({"repeat": repeat, "case": case["name"], "error": traceback.format_exc()})
                record = {"repeat": repeat, "case": case["name"], "kind": case["kind"], "checks": checks}
                summary["cases"].append(record)
                failures = [entry["name"] for entry in checks if not entry["pass"]]
                print("%02d %-26s %s" % (repeat, case["name"], "PASS" if not failures else "FAIL: " + ", ".join(failures)), flush=True)
        status, _, raw, _ = request(admin, "GET", "/stats?format=json")
        (args.output / "envoy-stats.json").write_bytes(raw)
        if status != 200:
            raise RuntimeError("Could not read Envoy stats")
    except Exception:
        summary["errors"].append({"error": traceback.format_exc()})
    finally:
        cleanup = {"envoy_pid": process.pid if process else None}
        if process is not None:
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=10)
            cleanup["envoy_returncode"] = process.returncode
            cleanup["envoy_stopped"] = process.poll() is not None
        if log_file:
            log_file.close()
        fixture.shutdown()
        fixture.server_close()
        thread.join(timeout=5)
        cleanup["fixture_stopped"] = not thread.is_alive()
        save_json(args.output / "cleanup.json", cleanup)
        all_checks = [entry for case in summary["cases"] for entry in case["checks"]]
        failed = [entry for entry in all_checks if not entry["pass"]]
        complete = (len(summary["cases"]) == args.repeat * len(matrix) and not summary["errors"]
                    and cleanup.get("envoy_stopped") and cleanup["fixture_stopped"])
        if args.expect == "fixed":
            accepted = complete and not failed
        else:
            # Baseline must reproduce a behavioral failure in every regression
            # scenario, while transport, JSON/SSE parsing, and controls work.
            accepted = complete and not any(entry["kind"] == "stable" for entry in failed)
            accepted = accepted and all(any(not entry["pass"] and entry["kind"] == "regression" for entry in case["checks"])
                                        for case in summary["cases"] if case["kind"] == "regression")
            for case in summary["cases"]:
                if case["case"] == "sse-two-tool-rounds":
                    # A missing finish_reason alone must not stand in for the
                    # second-round call loss this scenario is meant to expose.
                    failed_names = {entry["name"] for entry in case["checks"] if not entry["pass"]}
                    accepted = accepted and {
                        "round-2/client-all-calls-and-arguments",
                        "history-round-2/roundtrip-all-calls-and-arguments",
                    }.issubset(failed_names)
        summary.update(accepted=bool(accepted), checks=len(all_checks), failed_checks=len(failed), cleanup=cleanup)
        save_json(args.output / "summary.json", summary)
    print("Expectation %s: %s; %d cases, %d checks, %d failures, %d infrastructure errors" %
          (args.expect, "MET" if summary["accepted"] else "NOT MET", len(summary["cases"]),
           summary["checks"], summary["failed_checks"], len(summary["errors"])), flush=True)
    return 0 if summary["accepted"] else 1


if __name__ == "__main__":
    sys.exit(main())
