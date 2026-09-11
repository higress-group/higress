"""Auto acceptance oracles; every request is accounted for by backend events."""

import concurrent.futures
import json
import socket
import time

PORT = 10009
PROBE = "server/discover"
INIT = "initialize"
NOTIFY = "notifications/initialized"
CALL = "tools/call"
LIST = "tools/list"

FAILURES = {
    "401": 401, "403": 403, "429": 429, "500": 502, "302": 502,
    "header-mismatch": 400, "missing-capability": 400, "modern-404": 404,
    "version-unknown": 400, "version-conflict": 502,
    "oversize": 502, "malformed": 502, "wrong-id": 502, "no-tools": 502,
    "sse-duplicate": 502, "sse-missing": 502,
    "init-version": 502, "notify-fail": 200, "network": 502,
    "modern-ordinary-error": 400, "trailing-json": 502, "batch": 502,
}


def wait_for(read, predicate, description, timeout=10):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = read()
        if predicate(value):
            return value
        time.sleep(0.02)
    raise AssertionError("timed out waiting for " + description)


def cases(v):
    def configure(mode, barrier=None, reset=True):
        if reset:
            v.backend_reset()
        status, _, _ = v.exchange("http://backend-primary:8080/__auto_config", {
            "case": v.CURRENT_CASE, "mode": mode, "barrier": barrier,
        })
        v.check(status == 200, "auto backend control failed")

    def rpc(method=CALL, key="call", port=PORT, headers=None, extra_params=None, tool="proxy_echo"):
        params = {"_meta": v.modern_meta()}
        if method == CALL:
            params.update({"name": tool, "arguments": {"value": "fixture", "opaque": 900719925474099312345}})
        params.update(extra_params or {})
        rpc_id = f"{v.CURRENT_CASE}-{key}"
        request_headers = {
            "Host": "mcp.runtime.test", "Content-Type": "application/json",
            "Accept": "application/json,text/event-stream", "MCP-Protocol-Version": v.MODERN,
            "Mcp-Method": method, "X-Request-ID": "rv-" + rpc_id, "baggage": key,
        }
        if method == CALL:
            request_headers["Mcp-Name"] = tool
            request_headers["Mcp-Param-Future"] = "current-business"
        request_headers.update(headers or {})
        return v.exchange(f"http://{v.GATEWAY_HOST}:{port}/mcp", {
            "jsonrpc": "2.0", "id": rpc_id, "method": method, "params": params,
        }, request_headers)

    def details(expected=None):
        state = v.backend_state()
        events = state["events"]
        methods = [e["rpcMethod"] for e in events]
        if expected is not None:
            v.check(methods == expected, f"unexpected upstream sequence: {methods}, want {expected}")
        for event in events:
            v.check(event["path"] == "/auto/mcp", f"upstream target changed: {event}")
            v.check(event["authority"] == "backend-primary:8080", f"upstream authority changed: {event}")
            v.check(not event["lastEventIDPresent"] and not event["internalRoutePresent"], "internal/session header leaked")
            if event["rpcMethod"] == PROBE:
                v.check(event["mcpMethod"] == PROBE and event["mcpName"] is None and event["futureParam"] is None,
                        f"business identity leaked to probe: {event}")
                v.check(not event["sessionPresent"], "session leaked into next probe")
            if event["rpcMethod"] in (INIT, NOTIFY) or event["protocolVersion"] != v.MODERN:
                v.check(event["futureParam"] is None, "parameter header crossed legacy boundary")
        return {"upstreamSequence": methods, "backendEvents": {"backend-primary": events}, "backendState": state["auto"]}

    def success(mode):
        def run():
            configure(mode)
            status, _, response = rpc()
            v.check(status == 200 and response.get("result", {}).get("opaque") == 900719925474099312345,
                    f"auto success/precision failure: {status} {response}")
            expected = [PROBE, CALL] if mode in ("modern", "sse") else [PROBE, INIT, NOTIFY, CALL]
            evidence = details(expected)
            if mode == "version":
                v.check(all(event["protocolVersion"] == "2025-06-18" for event in evidence["backendEvents"]["backend-primary"][1:]),
                        "negotiated header version was not retained")
            return evidence
        return run

    def failure(mode):
        def run():
            configure(mode)
            status, headers, response = rpc()
            v.check(status == FAILURES[mode] and "error" in response, f"auto failure {mode}: {status} {response}")
            expected = [PROBE]
            if mode in ("init-version", "notify-fail"):
                expected += [INIT]
            if mode == "notify-fail":
                expected += [NOTIFY]
            lower_headers = {key.lower(): value for key, value in headers.items()}
            if mode in ("401", "403"):
                v.check("www-authenticate" in lower_headers, "authentication challenge was lost")
            if mode == "429":
                v.check(lower_headers.get("retry-after") == "2", "retry-after was lost")
            return details(expected)
        return run

    def fresh_and_concurrent():
        configure("modern")
        for index, mode in enumerate(("modern", "legacy", "modern")):
            configure(mode, reset=False)
            status, _, response = rpc(LIST if index == 0 else CALL, key=f"switch-{index}")
            v.check(status == 200 and "result" in response, "request did not rediscover changed capability")
        evidence = details([PROBE, LIST, PROBE, INIT, NOTIFY, CALL, PROBE, CALL])
        configure("modern")
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            responses = list(pool.map(lambda index: rpc(key=f"concurrent-{index}"), range(4)))
        v.check(all(status == 200 and "result" in response for status, _, response in responses), "concurrent auto request failed")
        concurrent_events = v.backend_state()["events"]
        for index in range(4):
            methods = [e["rpcMethod"] for e in concurrent_events if e["requestKey"] == f"concurrent-{index}"]
            v.check(methods == [PROBE, CALL], "requests shared a probe or business dispatch")
        evidence["concurrentBackendEvents"] = concurrent_events
        return evidence

    def auth_isolation():
        configure("legacy-session")
        for key, port, auth, expected in (("alice", 10011, "Bearer auto-alice", "alice"),
                                          ("bob", 10011, "Bearer auto-bob", "bob"),
                                          ("fixed", 10010, "Bearer auto-alice", "fixed"),
                                          ("override", 10011, "Bearer auto-alice", "tool")):
            status, _, response = rpc(key=key, port=port, headers={"Authorization": auth}, tool="proxy_override" if key == "override" else "proxy_echo")
            v.check(status == 200 and "result" in response, f"auth scenario failed: {response}")
            events = [e for e in v.backend_state()["events"] if e["requestKey"] == key]
            v.check(len(events) == 4 and all(e["authAlias"] == expected for e in events), "authentication snapshot changed")
            v.check([e["sessionAlias"] for e in events] == ["none", "none", "current", "current"], "session scope is wrong")
        return details([PROBE, INIT, NOTIFY, CALL] * 4)

    def boundaries():
        configure("modern")
        status, _, response = rpc(PROBE, key="local")
        v.check(status == 200 and "result" in response, "local discovery failed")
        status, _, response = rpc(key="denied", headers={"x-envoy-allow-mcp-tools": "other"})
        v.check("error" in response, "allowTools rejection missing")
        evidence = details([])
        configure("legacy-continuation")
        status, _, response = rpc(key="continuation", extra_params={"requestState": {"opaque": 1}})
        v.check(status == 400 and response["error"]["code"] == -32602, "continuation was replayed as legacy")
        evidence["continuation"] = details([PROBE, INIT, NOTIFY,])
        configure("cursor-error")
        status, _, response = rpc(LIST, key="cursor", extra_params={"cursor": "opaque-cursor"})
        v.check(response["error"]["code"] == -32602, "cursor error was swallowed")
        evidence["cursor"] = details([PROBE, LIST])
        return evidence

    def no_replay():
        configure("response-loss")
        status, _, response = rpc(key="executed")
        v.check(status == 502 and "error" in response, "lost response did not fail")
        evidence = details([PROBE, CALL])
        v.check(evidence["backendState"]["executions"] == {"executed": 1}, "business replayed after execution")
        configure("business-version-error")
        status, _, response = rpc(key="version-error")
        v.check(status == 400 and response["error"]["code"] == -32022, "business protocol error lost")
        evidence["businessVersionError"] = details([PROBE, CALL])
        return evidence

    def wait_upstream_complete():
        def active():
            _, _, result = v.exchange(f"http://{v.GATEWAY_HOST}:9901/stats?filter=upstream_rq_active&format=json", method="GET")
            return [stat["value"] for stat in result.get("stats", []) if stat["name"] == "cluster.backend-primary.upstream_rq_active"]
        wait_for(active, lambda counts: counts == [0], "upstream request completion")

    def cancel(stage):
        def run():
            configure("legacy" if stage in (INIT, NOTIFY) else "modern", barrier=stage)
            key = "cancel-" + stage.replace("/", "-")
            request_id = "rv-" + v.CURRENT_CASE
            body = json.dumps({"jsonrpc": "2.0", "id": key, "method": CALL,
                               "params": {"name": "proxy_echo", "arguments": {"value": "cancel"}, "_meta": v.modern_meta()}}).encode()
            headers = (f"POST /mcp HTTP/1.1\r\nHost: mcp.runtime.test\r\nContent-Type: application/json\r\n"
                       f"Accept: application/json,text/event-stream\r\nMCP-Protocol-Version: {v.MODERN}\r\n"
                       f"Mcp-Method: {CALL}\r\nMcp-Name: proxy_echo\r\nbaggage: {key}\r\n"
                       f"X-Request-ID: {request_id}\r\nContent-Length: {len(body)}\r\n\r\n").encode()
            sock = socket.create_connection((v.GATEWAY_HOST, PORT), timeout=5)
            try:
                sock.sendall(headers + body)
                wait_for(v.backend_state, lambda state: stage in state["auto"]["waiting"], "upstream phase barrier")
                sock.shutdown(socket.SHUT_RDWR)
                sock.close()
                v.EXCHANGES.append({"case": v.CURRENT_CASE, "accessRequestId": request_id,
                                    "request": {"listenerPort": PORT, "rpcMethod": CALL},
                                    "response": {"status": "client_disconnected"}})
                # Observe the gateway's completed downstream lifecycle before
                # releasing the upstream. No fixed sleep substitutes for it.
                def active():
                    _, _, result = v.exchange(f"http://{v.GATEWAY_HOST}:9901/stats?filter=downstream_rq_active&format=json", method="GET")
                    return [stat["value"] for stat in result.get("stats", []) if stat["name"].startswith("http.proxy-auto.")]
                wait_for(active, lambda counts: bool(counts) and all(count == 0 for count in counts), "downstream cancellation")
                v.exchange("http://backend-primary:8080/__auto_release", {"stage": stage})
                wait_for(v.backend_state, lambda state: state["auto"]["returned"].get(stage), "backend response completion")
                wait_upstream_complete()
                rpc(PROBE, key="after-cancel")  # same listener event-loop fence, local response only
                expected = [PROBE]
                if stage in (INIT, NOTIFY):
                    expected.append(INIT)
                if stage == NOTIFY:
                    expected.append(NOTIFY)
                if stage == CALL:
                    expected.append(CALL)
                evidence = details(expected)
                evidence["cancellationBoundary"] = "backend returned + downstream/upstream active gauges + same-listener fence"
                return evidence
            finally:
                sock.close()
                v.exchange("http://backend-primary:8080/__auto_release", {"stage": stage})
        return run

    def probe_timeout():
        configure("modern", barrier=PROBE)
        try:
            status, _, response = rpc(key="timeout")
            v.check(status in (502, 504) and "error" in response, "probe timeout did not fail")
            v.check(PROBE in v.backend_state()["auto"]["waiting"], "backend did not reach timeout barrier")
            v.exchange("http://backend-primary:8080/__auto_release", {"stage": PROBE})
            wait_for(v.backend_state, lambda state: state["auto"]["returned"].get(PROBE), "timed-out backend completion")
            wait_upstream_complete()
            rpc(PROBE, key="after-timeout")
            return details([PROBE])
        finally:
            v.exchange("http://backend-primary:8080/__auto_release", {"stage": PROBE})

    return (
        [(f"auto-success-{mode}", success(mode)) for mode in ("modern", "legacy", "legacy-session", "version", "http400", "http404", "http405", "sse")]
        + [(f"auto-failure-{mode}", failure(mode)) for mode in FAILURES]
        + [("auto-fresh-and-concurrent", fresh_and_concurrent), ("auto-auth-session-isolation", auth_isolation),
           ("auto-local-permission-continuation-cursor", boundaries), ("auto-execution-loss-no-replay", no_replay), ("auto-probe-timeout", probe_timeout)]
        + [("auto-cancel-" + stage.replace("/", "-"), cancel(stage)) for stage in (PROBE, INIT, NOTIFY, CALL)]
    )
