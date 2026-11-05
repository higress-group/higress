#!/usr/bin/env python3
"""Check the wire error and sticky fixture verdict independently of SDK errors."""

import json
import sys
import urllib.error
import urllib.request


def exchange(url, body=None):
    headers = {"Content-Type": "application/json", "Accept": "application/json,text/event-stream",
               "MCP-Protocol-Version": "2026-07-28", "Mcp-Method": "tools/call", "Mcp-Name": "get_weather"}
    request = urllib.request.Request(url, json.dumps(body).encode() if body is not None else None, headers)
    try:
        response = urllib.request.urlopen(request, timeout=10)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        data = response.read()
        try:
            value = json.loads(data)
        except json.JSONDecodeError:
            value = None
        return response.status, value


def check(root):
    rpc_id = "interop-auto-probe-oracle"
    status, response = exchange(root + "/proxy-auto-error", {
        "jsonrpc": "2.0", "id": rpc_id, "method": "tools/call",
        "params": {"name": "get_weather", "arguments": {"location": "New York"},
                   "_meta": {"io.modelcontextprotocol/protocolVersion": "2026-07-28",
                             "io.modelcontextprotocol/clientInfo": {"name": "interop-oracle", "version": "1"},
                             "io.modelcontextprotocol/clientCapabilities": {}}},
    })
    if (status != 400 or not isinstance(response, dict) or response.get("jsonrpc") != "2.0"
            or response.get("id") != rpc_id or response.get("error", {}).get("code") != -32020
            or "result" in response):
        raise AssertionError(f"probe error must be HTTP 400 / JSON-RPC -32020 with matching ID: {status} {response}")
    status, state = exchange(root + "/__fixture_status")
    if status != 200 or state != {"failures": 0}:
        raise AssertionError(f"interop fixture recorded a failure: {status} {state}")


if __name__ == "__main__":
    try:
        check(sys.argv[1])
    except AssertionError as error:
        print(error, file=sys.stderr)
        sys.exit(42)
    print("auto probe oracle: HTTP 400 / JSON-RPC -32020 / matching ID / no fixture failures")
