#!/usr/bin/env python3
# Copyright (c) 2026 Alibaba Group Holding Limited.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Run the virtual tool set through Higress Envoy and the MCP Inspector."""

import argparse
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request
import uuid


HERE = Path(__file__).resolve().parent
IMAGE = ("higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway"
         "@sha256:5d0618399fa98b9f4ad3c34b58f084b51624e3cc4eccdde72554b636d16b5da4")
INSPECTOR_VERSION = "2.12.0"
MODERN = "2026-07-28"
LEGACY = "2025-03-26"
VERSION_KEY = "io.modelcontextprotocol/protocolVersion"
SOURCES = ("legacy", "modern")
ENTRIES = ("virtual", "empty", "disabled", "unmatched")
PLUGINS = ("mcp-router", "mcp-server")


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


class Echo(BaseHTTPRequestHandler):
    """Two local MCP sources that record the identity received from the gateway."""

    def log_message(self, *_args):
        pass

    def read_body(self):
        if self.headers.get("Transfer-Encoding", "").lower() != "chunked":
            return self.rfile.read(int(self.headers.get("Content-Length", "0")))
        chunks = bytearray()
        while True:
            size = int(self.rfile.readline().partition(b";")[0], 16)
            if size == 0:
                for trailer in self.rfile:
                    if trailer == b"\r\n":
                        break
                return chunks
            chunks.extend(self.rfile.read(size))
            require(self.rfile.read(2) == b"\r\n", "invalid HTTP chunk")

    def do_POST(self):
        request = json.loads(self.read_body())
        source = self.path.rsplit("/", 1)[-1]
        method = request["method"]
        params = request.get("params", {})
        event = {
            "source": source, "path": self.path, "host": self.headers.get("Host"), "method": method,
            "version": self.headers.get("Mcp-Protocol-Version"),
            "methodHeader": self.headers.get("Mcp-Method"), "nameHeader": self.headers.get("Mcp-Name"),
            "bodyVersion": params.get("protocolVersion"),
            "bodyName": params.get("name"), "arguments": params.get("arguments"),
        }
        self.server.events.append(event)
        try:
            result = self.handle_mcp(event, params)
            payload = {"jsonrpc": "2.0", "id": request.get("id"), "result": result}
            status = 202 if result is None else 200
        except (KeyError, RuntimeError, TypeError) as error:
            event["error"] = str(error)
            payload = {"jsonrpc": "2.0", "id": request.get("id"),
                       "error": {"code": -32602, "message": str(error)}}
            status = 400
        body = b"" if status == 202 else json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def handle_mcp(self, event, params):
        source, method = event["source"], event["method"]
        require(source in SOURCES and event["path"] == f"/echo/{source}"
                and event["host"] == f"{source}.echo.test", "unexpected upstream route")
        if source == "legacy":
            require(event["version"] != MODERN and not event["methodHeader"]
                    and not event["nameHeader"] and "_meta" not in params,
                    "Modern identity reached the Legacy source")
            if method == "initialize":
                require(event["bodyVersion"] == LEGACY, "incorrect Legacy initialize version")
                return {"protocolVersion": LEGACY, "capabilities": {"tools": {}},
                        "serverInfo": {"name": "legacy-echo", "version": "1"}}
            if method == "notifications/initialized":
                return None
        else:
            require(event["version"] == MODERN and event["methodHeader"] == method
                    and params["_meta"][VERSION_KEY] == MODERN, "invalid Modern source identity")
            require(event["nameHeader"] == "echo", "incorrect upstream Mcp-Name")
        require(method == "tools/call" and event["bodyName"] == "echo", "incorrect upstream operation")
        message = params["arguments"]["message"]
        require(isinstance(message, str), "message must be a string")
        result = {"content": [{"type": "text", "text": f"{source}: {message}"}],
                  "structuredContent": {"source": source, "tool": "echo", "message": message}}
        if source == "modern":
            result["resultType"] = "complete"
        return result


def address(port):
    return {"socket_address": {"address": "127.0.0.1", "port_value": port}}


def typed(name, **fields):
    return {"@type": "type.googleapis.com/" + name, **fields}


def bootstrap(listener_port, echo_port, configs):
    filters = []
    for name in PLUGINS:
        filters.append({"name": name, "typed_config": typed(
            "envoy.extensions.filters.http.wasm.v3.Wasm",
            config={"name": name, "root_id": name,
                    "vm_config": {"vm_id": name, "runtime": "envoy.wasm.runtime.v8",
                                  "code": {"local": {"filename": f"/demo/{name}.wasm"}}},
                    "configuration": typed("google.protobuf.StringValue", value=json.dumps(configs[name]))},
        )})
    filters.append({"name": "envoy.filters.http.router",
                    "typed_config": typed("envoy.extensions.filters.http.router.v3.Router")})
    routes = [{"name": f"entry-{name}", "match": {"path": f"/{name}"},
               "direct_response": {"status": 503}} for name in ENTRIES]
    for source in SOURCES:
        for name, path in ((f"source-{source}", f"/route/{source}"), (f"peer-{source}", f"/echo/{source}")):
            routes.append({"name": name, "match": {"path": path},
                           "route": {"cluster": "echo", "timeout": "10s"}})
    routes.append({"name": "ready", "match": {"path": "/healthz"}, "direct_response": {"status": 200}})
    cluster = {"name": "echo", "type": "STATIC", "connect_timeout": "2s", "load_assignment": {
        "cluster_name": "echo", "endpoints": [{"lb_endpoints": [{"endpoint": {"address": address(echo_port)}}]}],
    }}
    access_log = {"name": "envoy.access_loggers.stdout", "typed_config": typed(
        "envoy.extensions.access_loggers.stream.v3.StdoutAccessLog",
        log_format={"text_format_source": {"inline_string":
            "MCP_DEMO %REQ(X-MCP-DEMO-CASE)% %RESPONSE_CODE% %ROUTE_NAME% %REQ(:PATH)% "
            "%RESPONSE_CODE_DETAILS%\n"}},
    )}
    hcm = typed("envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
                stat_prefix="virtual_tools", access_log=[access_log], http_filters=filters,
                route_config={"name": "demo", "virtual_hosts": [{"name": "local", "domains": ["*"], "routes": routes}]})
    return {"static_resources": {"clusters": [cluster], "listeners": [{
        "name": "demo", "address": address(listener_port), "filter_chains": [
            {"filters": [{"name": "envoy.filters.network.http_connection_manager", "typed_config": hcm}]}],
    }]}}


def build_pair(repo, evidence):
    env = dict(os.environ, GOWORK="off", GOTOOLCHAIN="go1.24.4", GOOS="wasip1", GOARCH="wasm")
    for plugin in PLUGINS:
        path = repo / "plugins/wasm-go/extensions" / plugin
        subprocess.run(["go", "build", "-p", "4", "-mod=readonly", "-trimpath", "-buildmode=c-shared",
                        "-o", str(evidence / f"{plugin}.wasm"), "."], cwd=path, env=env, check=True)
    return {name: hashlib.sha256((evidence / f"{name}.wasm").read_bytes()).hexdigest() for name in PLUGINS}


def wait_for_gateway(url):
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(url + "/healthz", timeout=1):
                return
        except urllib.error.HTTPError as error:
            # The global fallback mcp-server configuration can reject a plain GET.
            if error.code == 405:
                return
            raise
        except (OSError, urllib.error.URLError):
            time.sleep(0.2)
    raise RuntimeError("gateway startup timed out")


def inspector(evidence, url, case, method, *args):
    command = ["npx", "--yes", f"@modelcontextprotocol/inspector@{INSPECTOR_VERSION}", "--cli", url,
               "--transport", "http", "--protocol-era", "modern",
               "--header", f"X-Mcp-Demo-Case: {case}", "--method", method, "--format", "json", *args]
    result = subprocess.run(command, capture_output=True, text=True, timeout=120)
    (evidence / f"{case}.json").write_text(result.stdout, encoding="utf-8")
    (evidence / f"{case}.stderr").write_text(result.stderr, encoding="utf-8")
    require(result.returncode == 0, f"Inspector {case} failed ({result.returncode}): {result.stderr}")
    return json.loads(result.stdout)["result"]


def verify(evidence, base, fixture, container):
    for route in ENTRIES:
        # Inspector's initialize CLI method reports the connection probe for the chosen era.
        result = inspector(evidence, f"{base}/{route}", f"discover-{route}", "initialize")
        require(result["protocolVersion"] == MODERN, f"{route} did not negotiate {MODERN}")
        capabilities = result["capabilities"]
        if route in ("virtual", "empty"):
            require(capabilities.get("tools") == {}, f"{route} is missing the tools capability")
        else:
            require("tools" not in capabilities, f"{route} unexpectedly advertised tools")
    tools = inspector(evidence, f"{base}/virtual", "list-virtual", "tools/list")["tools"]
    require(sorted(tool["name"] for tool in tools) == [f"{source}___echo" for source in SOURCES],
            "virtual list does not contain both composed tools")
    empty = inspector(evidence, f"{base}/empty", "list-empty", "tools/list")
    require(empty.get("tools") == [], "empty tool set must serialize as []")
    for source in SOURCES:
        result = inspector(evidence, f"{base}/virtual", f"call-{source}", "tools/call",
                           "--tool-name", f"{source}___echo", "--tool-arg", "message=hello")
        require(not result.get("isError") and result.get("structuredContent") ==
                {"source": source, "tool": "echo", "message": "hello"}, f"{source} echo result differs")
    require(not any("error" in event for event in fixture.events), "an upstream rejected its request")
    methods = [(event["source"], event["method"]) for event in fixture.events]
    require(methods == [("legacy", "initialize"), ("legacy", "notifications/initialized"),
                        ("legacy", "tools/call"), ("modern", "tools/call")], f"unexpected upstream sequence: {methods}")
    logs = subprocess.check_output(["docker", "logs", container], text=True, stderr=subprocess.STDOUT)
    for route in ENTRIES:
        prefix = f"MCP_DEMO discover-{route} 200 entry-{route} /{route} "
        require(any(line.startswith(prefix) and line.endswith(":server/discover")
                    for line in logs.splitlines()), f"gateway did not observe Modern discovery for {route}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence", type=Path, help="new directory for Wasm files, configuration and captures")
    parser.add_argument("--serve", action="store_true", help="keep the gateway available for manual Inspector use")
    args = parser.parse_args()
    evidence = args.evidence.resolve() if args.evidence else Path(tempfile.mkdtemp(prefix="mcp-inspector-demo-"))
    if args.evidence:
        evidence.mkdir(parents=True)
    print(f"Evidence directory: {evidence}", flush=True)

    repo = Path(subprocess.check_output(["git", "-C", str(HERE), "rev-parse", "--show-toplevel"], text=True).strip())
    summary = {"source": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=repo, text=True).strip(),
               "sourceDirty": bool(subprocess.check_output(["git", "status", "--porcelain"], cwd=repo)),
               "image": IMAGE, "inspector": INSPECTOR_VERSION, "wasmSha256": build_pair(repo, evidence),
               "verified": False}
    configs = json.loads((HERE / "plugins.json").read_text(encoding="utf-8"))
    write_json(evidence / "plugins.json", configs)
    container = f"mcp-inspector-demo-{uuid.uuid4().hex[:12]}"
    with ThreadingHTTPServer(("127.0.0.1", 0), Echo) as fixture:
        fixture.events = []
        thread = threading.Thread(target=fixture.serve_forever, daemon=True)
        thread.start()
        try:
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                port = listener.getsockname()[1]
            write_json(evidence / "envoy.json", bootstrap(port, fixture.server_port, configs))
            subprocess.run(["docker", "run", "--detach", "--name", container, "--network", "host",
                            "-v", f"{evidence}:/demo:ro", "--entrypoint", "/usr/local/bin/envoy", IMAGE,
                            "-c", "/demo/envoy.json", "--disable-hot-restart", "--concurrency", "1",
                            "--log-level", "warning"], check=True, stdout=subprocess.DEVNULL)
            base = f"http://127.0.0.1:{port}"
            wait_for_gateway(base)
            summary["envoy"] = subprocess.check_output(
                ["docker", "exec", container, "/usr/local/bin/envoy", "--version"], text=True).strip()
            summary["endpoint"] = base
            print(f"Inspector URL: {base}/virtual", flush=True)
            if args.serve:
                threading.Event().wait()
            else:
                summary["node"] = subprocess.check_output(["node", "--version"], text=True).strip()
                verify(evidence, base, fixture, container)
                summary["verified"] = True
        finally:
            try:
                with (evidence / "gateway.log").open("w", encoding="utf-8") as logs:
                    subprocess.run(["docker", "logs", container], stdout=logs, stderr=subprocess.STDOUT, timeout=15)
            finally:
                try:
                    subprocess.run(["docker", "rm", "-f", container], capture_output=True, timeout=15)
                finally:
                    fixture.shutdown()
                    thread.join()
                containers = subprocess.check_output(
                    ["docker", "ps", "-a", "--filter", f"name=^/{container}$", "--format", "{{.ID}}"], text=True)
                summary["containerRemoved"] = not containers.strip()
                write_json(evidence / "upstream.json", fixture.events)
                write_json(evidence / "summary.json", summary)
                require(summary["containerRemoved"], f"could not remove demo container {container}")
    if summary["verified"]:
        print("Inspector verification passed for four discovery responses, two catalogs and two echo calls.")


def terminate(_signum, _frame):
    raise KeyboardInterrupt


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, terminate)
    try:
        main()
    except KeyboardInterrupt:
        raise SystemExit(130)
