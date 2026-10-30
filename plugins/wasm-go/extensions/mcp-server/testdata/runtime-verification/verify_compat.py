#!/usr/bin/env python3
"""Execute registered/REST/proxy compatibility cases through real Envoy.

No candidate output generates expectations. The registry and corresponding old
path determine assertions; only named ID/number defects are exempted on oracle.
"""
import hashlib
import base64
import http.client
import json
import os
import sys
import time
from pathlib import Path

import compat_contract as contract
from compat_fixture import PORTS
from typed_canonical import JSONNumber, canonical_json_sha256, loads_typed


def check(condition, message):
    if not condition:
        raise AssertionError(message)


def integer(value):
    return int(value.lexeme) if isinstance(value, JSONNumber) else value


def business_result_sha256(result):
    # Normalize only protocol-owned envelope fields and the named historical
    # opaque number defect. Ordinary result fields and metadata remain compared.
    payload = dict(result)
    for key in ("number", "resultType", "ttlMs", "cacheScope"):
        payload.pop(key, None)
    if isinstance(payload.get("_meta"), dict):
        metadata = dict(payload["_meta"])
        metadata.pop("io.modelcontextprotocol/serverInfo", None)
        if metadata:
            payload["_meta"] = metadata
        else:
            payload.pop("_meta")
    return canonical_json_sha256(payload)


class Compatibility:
    def __init__(self, variant, out):
        self.variant, self.out = variant, Path(out)
        self.rows, self.exchanges = [], []
        self.exchange_counts = {}
        self.case = "readiness"

    def exchange(self, port, raw, modern, tool="raw_summary", method="tools/call", extra=(), host="gateway"):
        count = self.exchange_counts.get(self.case, 0) + 1
        self.exchange_counts[self.case] = count
        request_id = "compat-" + hashlib.sha256(self.case.encode()).hexdigest()[:24] + "-" + str(count)
        headers = [("Host", "mcp.runtime.test"), ("Content-Type", "application/json"),
                   ("Accept", "application/json,text/event-stream"), ("X-Compat-Case", self.case),
                   ("X-Request-ID", request_id),
                   ("X-Ordinary", "first"), ("X-Ordinary", "second")]
        if modern:
            headers += [("MCP-Protocol-Version", contract.MODERN), ("Mcp-Method", method)]
            if method == "tools/call":
                encoded = tool if tool.isascii() else "=?base64?" + base64.b64encode(tool.encode()).decode() + "?="
                headers.append(("Mcp-Name", encoded))
        headers += list(extra)
        connection = http.client.HTTPConnection(host, port, timeout=300)
        begin = time.monotonic()
        try:
            connection.putrequest("POST", "/mcp", skip_host=True, skip_accept_encoding=True)
            for name, value in headers:
                connection.putheader(name, value)
            connection.putheader("Content-Length", str(len(raw)))
            connection.endheaders(raw)
            response = connection.getresponse()
            body, response_headers = response.read(), response.getheaders()
            status = response.status
        finally:
            connection.close()
        try:
            value = loads_typed(body) if body else None
        except ValueError:
            value = None
        self.exchanges.append({"case": self.case, "accessRequestId": request_id, "port": port, "httpStatus": status,
                               "request": contract.body_record(raw), "response": contract.body_record(body),
                               "responseHeaders": {k.lower(): v for k, v in response_headers if k.lower() in ("content-type", "www-authenticate", "retry-after")},
                               "elapsedSecondsDiagnostic": round(time.monotonic() - begin, 3)})
        return status, {k.lower(): v for k, v in response_headers}, value

    def backend(self, reset=False):
        connection = http.client.HTTPConnection("backend-primary", 8080, timeout=10)
        connection.request("POST" if reset else "GET", "/__compat_reset" if reset else "/__compat_state", b"{}" if reset else None)
        response = connection.getresponse()
        value = json.loads(response.read())
        connection.close()
        check(response.status == 200, "backend ledger unavailable")
        return value

    def stats(self, port, modern):
        status, _, response = self.exchange(port, contract.envelope(modern, tool="runtime_counters"), modern, tool="runtime_counters")
        check(status == 200 and response and "result" in response, "registered execution counter unavailable")
        observed = {"generation": response["result"]["generation"], **{key: integer(response["result"][key]) for key in ("creates", "calls")}}
        self.exchanges[-1]["executionCounters"] = observed
        return tuple(observed[key] for key in ("generation", "creates", "calls"))

    def identity(self, response, raw_id, modern, status, headers):
        if modern and not raw_id:
            check(status == (status if status in (401, 403) else 202), "notification status is not an acknowledgement/auth exception")
            check(response is None and self.exchanges[-1]["response"]["utf8Bytes"] == 0, "notification leaked an RPC body")
            if status in (401, 403):
                check(headers.get("www-authenticate") == 'Bearer realm="compat"', "notification lost auth challenge")
            return
        check(isinstance(response, dict) and "id" in response, "determinable client ID missing")
        wanted = loads_typed(raw_id or "0")
        if self.variant == "oracle" and (raw_id in ("1.50", "1E+03", "-0", "null", "9223372036854775808")):
            # D2 fixes these explicitly named historical ID defects. Preserve
            # the observed token in evidence; never use it as candidate oracle.
            check(isinstance(response["id"], JSONNumber), "historical ID compatibility view is not numeric")
            return
        check(type(response["id"]) is type(wanted) and response["id"] == wanted, "client ID token/type was not restored")

    def ledger(self):
        events = [row for row in self.backend()["events"] if row["case"] == self.case]
        return {"events": events, "attempts": {kind: sum(row["kind"] == kind for row in events)
                                                for kind in ("transport", "probe", "control", "business")}}

    def body_case(self, case):
        modern = case["profile"] == "modern"
        raw, arguments = contract.sized_body(modern, case["size"], case["location"])
        check(len(raw) == case["size"], "full UTF-8 body size differs from independent case registry")
        raw.decode("utf-8")
        port = PORTS["http-legacy"] if case["size"] > contract.LIMIT else PORTS["registered-body"]
        status, _, response = self.exchange(port, raw, modern)
        if case["size"] > contract.LIMIT:
            check(status == 413, "complete body above 100 MiB was accepted")
            check(self.ledger()["attempts"]["business"] == 0, "oversized body reached business")
            return {"status": status, "businessAttempts": 0}
        check(status == 200 and response and "result" in response, "admissible full body did not execute")
        result = response["result"]
        check(integer(result["rawLength"]) == len(arguments) and result["rawSha256"] == hashlib.sha256(arguments).hexdigest(), "direct Create bytes changed")
        return {"rawLength": str(integer(result["rawLength"])), "rawSha256": result["rawSha256"]}

    def input_case(self, case):
        modern, shape = case["profile"] == "modern", case["shape"]
        arguments, tool, port = b"{}", "raw_summary", PORTS["registered"]
        if shape == "depth70": arguments = b'{"deep":' + b'[' * 70 + b'0' + b']' * 70 + b'}'
        elif shape == "values65537": arguments = b'{"values":[' + b','.join([b'0'] * 65537) + b']}'
        elif shape == "duplicates": arguments = b'{"choice":1,"choice":2,"nested":{"a":1,"a":2},"number":900719925474099312345}'
        elif shape == "long-name": tool = "n" * 5000
        elif shape == "encoded-name": tool = "界" * 3000
        elif shape == "schema-mismatch": arguments = b'{"schemaOnly":7}'
        elif shape == "allowTools": tool, port = "n" * 5000, PORTS["restricted"]
        elif shape == "local-error": tool = "unregistered-tool"
        raw_id = contract.IDS[case["id_index"]] if "id_index" in case else '"case"'
        raw = contract.envelope(modern, tool=tool, arguments=arguments, rpc_id=raw_id)
        if shape == "invalid": raw = raw[:-1]
        elif shape == "concatenated": raw += b'{}'
        elif shape == "batch": raw = b'[' + raw + b']'
        positive, _, completed = self.exchange(port, contract.envelope(modern), modern)
        check(positive == 200 and completed and "result" in completed, "positive counter control did not execute")
        before = self.stats(port, modern)
        check(bool(before[0]) and before[1] > 0 and before[1] == before[2], "positive execution counter evidence missing")
        status, _, response = self.exchange(port, raw, modern, tool=tool)
        if shape == "local-error": self.identity(response, raw_id, modern, status, {})
        after = self.stats(port, modern)
        check(before[0] == after[0], "registered VM recycled between counter observations")
        if shape in ("invalid", "concatenated", "batch") and self.variant == "oracle":
            return {"basicMessageBoundaryException": True, "observedStatus": status,
                    "createsDelta": after[1] - before[1], "callsDelta": after[2] - before[2],
                    "observedResult": bool(response and "result" in response),
                    "observedError": bool(response and "error" in response)}
        if shape in ("invalid", "concatenated", "batch", "allowTools", "local-error"):
            check(before == after, "rejected request executed Create/Call")
            if shape == "local-error" and modern and not raw_id:
                return {"zeroCreateCall": True, "boundary": "ordinary notification acknowledgement"}
            check(response and "error" in response, "invalid/denied input produced no error")
            if shape == "local-error":
                check(status == 200 and integer(response["error"]["code"]) == -32602, "historical local error changed")
                return {"zeroCreateCall": True, "boundary": "local tool lookup"}
            if shape != "allowTools":
                check(status == 400 and integer(response["error"]["code"]) in (-32700, -32600), "basic-message admission status/code changed")
            return {"zeroCreateCall": True, "boundary": "basic-message" if shape != "allowTools" else "authorization"}
        check(after[1] == before[1] + 1 and after[2] == before[2] + 1, "Create/Call positive increment missing")
        check(status == 200 and response and "result" in response, "historically admissible input rejected")
        result = response["result"]
        check(result["rawSha256"] == hashlib.sha256(arguments).hexdigest(), "raw direct-tool arguments were normalized")
        return {"rawSha256": result["rawSha256"]}

    def descriptors(self, case):
        modern = case["profile"] == "modern"
        status, _, response = self.exchange(PORTS["registered"], contract.envelope(modern, method="tools/list"), modern, method="tools/list")
        check(status == 200 and response and "result" in response, "tool descriptors unavailable")
        tools = {row["name"]: row for row in response["result"]["tools"]}
        check({"raw_summary", "runtime_counters", "numeric_comparison", "n" * 5000, "界" * 3000} <= set(tools), "serializable descriptors filtered")
        summary = tools["raw_summary"]
        check("outputSchema" in summary and summary["inputSchema"]["custom"]["value"] == "serialized", "RawMessage/Marshaler/outputSchema lost")
        digest = canonical_json_sha256(summary)
        status, _, composed = self.exchange(PORTS["composed"], contract.envelope(modern, method="tools/list"), modern, method="tools/list")
        check(status == 200 and composed and "result" in composed, "composed descriptors unavailable")
        composed_summary = next(row for row in composed["result"]["tools"] if row["name"] == "runtime-schema-fixtures___raw_summary")
        check(canonical_json_sha256({**composed_summary, "name": "raw_summary"}) == digest, "composed descriptor lost source fields")
        status, _, rejected = self.exchange(PORTS["composed"], contract.envelope(modern, tool="runtime-schema-fixtures___raw_summary"), modern, tool="runtime-schema-fixtures___raw_summary")
        check(rejected and "error" in rejected and self.ledger()["attempts"]["business"] == 0, "historical composed call rejection changed")
        status, _, rest = self.exchange(PORTS["rest"], contract.envelope(modern, method="tools/list"), modern, method="tools/list")
        rest_descriptors = {row["name"]: row for row in rest["result"]["tools"]}
        names = set(rest_descriptors)
        check({"legacy-false", "legacy-true", "getTransactionRecordListV2"} <= names, "legacyOnly/schema descriptor gate remains")
        for tool in ("legacy-false", "legacy-true"):
            status, _, visible = self.exchange(PORTS["rest"], contract.envelope(modern, tool=tool), modern, tool=tool)
            check(status == 200 and visible and "result" in visible, "legacyOnly still blocks business execution")
        raw = contract.envelope(modern, tool="getTransactionRecordListV2", arguments=b'{"transactionId":"fixture","page":2,"businessType":["SALE"],"payload":{"amount":7}}')
        status, _, converted = self.exchange(PORTS["rest"], raw, modern, tool="getTransactionRecordListV2")
        check(status == 200 and converted and "result" in converted, "historical REST conversion rejected")
        ledger = self.ledger()
        check(ledger["attempts"]["business"] == 1, "REST conversion did not reach its backend")
        request = next(row for row in ledger["events"] if row["kind"] == "business")
        expected_body = loads_typed(b'{"businessType":["SALE"],"payload":{"amount":7}}')
        check(request["method"] == "POST" and request["path"] == "/unified/rest/converted/fixture" and request["query"] == {"fixed": ["yes"], "page": ["2"]}, "REST path/query conversion changed")
        check(request["restFlag"] == "false" and request["argumentsTypedSha256"] == canonical_json_sha256(expected_body), "REST body/header conversion changed")
        return {"descriptorSha256": digest, "restDescriptorSha256": canonical_json_sha256(rest_descriptors),
                "restBusinessAttempts": 1, "restMapping": {key: request[key] for key in ("method", "path", "query", "restFlag", "argumentsTypedSha256")}}

    def builtin(self, case):
        modern, name = case["profile"] == "modern", case["builtin"]
        manifest = json.loads((self.out / "builtin-manifest.json").read_text())[name]
        status, _, response = self.exchange(PORTS[name], contract.envelope(modern, method="tools/list"), modern, method="tools/list")
        check(status == 200 and response and "result" in response, "actual builtin configuration did not load")
        names = {row["name"] for row in response["result"]["tools"]}
        check(set(manifest["tools"]) == names, "actual builtin descriptor visibility differs")
        tool = manifest["tools"][0]
        status, _, response = self.exchange(PORTS[name], contract.envelope(modern, tool=tool, arguments=b'{"url":"https://fixture.invalid","timeout":1}'), modern, tool=tool)
        check(status == 200 and response and "result" in response, "actual builtin tool failed before the fixture route")
        check(self.ledger()["attempts"]["business"] == 1, "actual builtin tool made no observable backend call")
        return {"sourceSha256": manifest["sourceSha256"], "tools": sorted(names), "businessAttempts": 1}

    def proxy_case(self, case):
        modern, route = case["profile"] == "modern", case.get("route", "sse")
        rpc_id = contract.IDS[case["id_index"]] if "id_index" in case else "" if case.get("notification") else '"case"'
        arguments = b'{"number":900719925474099312345,"decimal":1.50,"exponent":1E+03,"choice":1,"choice":2,"nested":{"x":1,"x":2}}'
        method = case.get("method", "tools/call")
        tool = "界" * 3000 if case.get("response") == "encoded-name" else "echo"
        raw = contract.envelope(modern, method=method, tool=tool, arguments=arguments, rpc_id=rpc_id)
        raw = raw.replace(b'"arguments":' + arguments, b'"arguments":' + arguments + b',"arguments":{"ignored":true}', 1)
        if modern:
            raw = raw.replace(b'"io.modelcontextprotocol/clientCapabilities":{}', b'"io.modelcontextprotocol/clientCapabilities":false,"io.modelcontextprotocol/clientInfo":7,"vendor":{"ignored":true},"vendor":{"choice":1,"choice":2}', 1)
            raw = raw[:-2] + b',"ordinary":{"choice":1,"choice":2},"ordinary":{"ignored":true}}}'
        raw = raw[:-1] + b',"params":{"ignored":true}}'
        extra = [("X-Compat-Response", case.get("response", "success"))]
        if case["kind"] == "sse-url":
            endpoint = case["endpoint"]
            route = "sse-path" if endpoint == "path-config" else "sse-path-rewrite" if endpoint == "path-config-absolute" else "sse-rewrite" if endpoint == "host-rewrite" else "sse"
            extra.append(("X-Compat-Endpoint", endpoint))
        if case["kind"] == "sse-auth":
            route = "sse-" + case["credential"]
            extra += [("X-Compat-Endpoint", "cross-authority"), ("Authorization", "Bearer compat-forward"), ("X-Client-Key", "compat-pass")]
        if case["kind"] == "sse-stage-auth":
            extra.append(("X-Compat-Fail-Stage", case["stage"]))
        if case["kind"] == "discovery":
            raw = contract.envelope(True, method="server/discover", rpc_id=rpc_id)
        port, method = (14000, "server/discover") if case["kind"] == "discovery" else (PORTS[route], method)
        status, headers, response = self.exchange(port, raw, modern, tool=tool, method=method, extra=extra)
        evidence = self.ledger()
        if case["kind"] == "sse-url" and case["endpoint"] in ("path-config", "unknownhost"):
            check(status >= 400 or response and "error" in response, "empty-Authority/unknownhost failure was silently guessed into success")
            check(all(row["accepted"] for row in evidence["events"]), "strict peer rejected a failure-boundary request")
            check(evidence["attempts"]["business"] == 0, "failed SSE handshake reached business")
            if case["endpoint"] == "unknownhost":
                check(evidence["attempts"]["control"] == 1 and evidence["events"][-1]["authority"] == "unknownhost", "unknownhost control boundary not reached")
                self.identity(response, rpc_id, modern, status, headers)
            return {"historicalFailureStatus": status, "stages": [{k: row[k] for k in ("kind", "authority", "path", "query")} for row in evidence["events"]]}
        self.identity(response, rpc_id, modern, status, headers)
        mode = case.get("response", "success")
        business_digest = None
        if mode.startswith("http"):
            expected = int(mode[4:])
            check(status == (expected if expected in (401, 403) else 202 if modern and not rpc_id else 200), "business/auth HTTP policy changed")
            if expected in (401, 403): check(headers.get("www-authenticate") == 'Bearer realm="compat"', "auth challenge lost")
            retained_business_headers = not route.startswith("sse") and case["kind"] != "discovery" and expected not in (401, 403) and not (modern and not rpc_id)
            check(headers.get("retry-after") == ("5" if retained_business_headers else None), "corresponding business Retry-After changed")
            check(headers.get("www-authenticate") == ('Bearer realm="compat"' if retained_business_headers or expected in (401, 403) else None), "corresponding business challenge header changed")
            if not (modern and not rpc_id):
                check(response and "error" in response and integer(response["error"]["code"]) == -32603 and "data" not in response["error"], "business HTTP error/data mapping changed")
                if expected not in (401, 403):
                    expected_message = "SSE tool request failed with status " + str(expected) if route.startswith("sse") else "backend " + method + " failed"
                    check(response["error"]["message"] == expected_message, "business HTTP error message changed")
        elif modern and not rpc_id:
            check(status == 202, "ordinary notification completion changed")
        else:
            check(status == 200 and response, "business response unavailable")
            if mode == "rpc-error":
                check(integer(response["error"]["code"]) == -32603 and "data" not in response["error"], "business error code/data policy changed")
                expected_message = "invalid server/discover result" if case["kind"] == "discovery" else "backend tool call failed" if route.startswith("sse") else "invalid tools/list response" if method == "tools/list" else "fixture error"
                check(response["error"]["message"] == expected_message, "historical business error message changed")
            else:
                check("result" in response, "business success rejected")
                if case["kind"] == "discovery":
                    check(response["result"].get("instructions") == "fixture instructions" and "unimplemented" not in response["result"].get("capabilities", {}), "instructions field isolation changed")
                else:
                    result = response["result"]
                    check(isinstance(result.get("number"), JSONNumber), "opaque result number missing")
                    if self.variant != "oracle": check(result["number"].lexeme == "900719925474099312345", "opaque result number rounded")
                    expected_result = loads_typed(b'{"tools":[{"name":"echo","inputSchema":{"type":"object"}}],"content":[],"value":2}')
                    expected_result["recipient"] = next(row["authority"] for row in evidence["events"] if row["kind"] == "business")
                    if mode == "deep":
                        deep = JSONNumber("0")
                        for _ in range(70): deep = [deep]
                        expected_result["deep"] = deep
                    if mode == "over-probe-limit": expected_result["padding"] = "x" * ((1 << 20) + 1)
                    business_digest = business_result_sha256(result)
                    check(business_digest == canonical_json_sha256(expected_result), "successful business result content changed")
        check(all(row["accepted"] for row in evidence["events"]), "strict peer rejected an outbound request")
        if self.variant != "oracle":
            expected_arguments = loads_typed(b'{"number":900719925474099312345,"decimal":1.50,"exponent":1E+03,"choice":2,"nested":{"x":2}}')
            check(all(row.get("argumentsTypedSha256") == canonical_json_sha256(expected_arguments) for row in evidence["events"] if row["rpcMethod"] == "tools/call"), "proxy argument number/duplicate semantics changed")
            check(all(not row["initializeCapabilityKeys"] for row in evidence["events"] if row["rpcMethod"] == "initialize"), "gateway advertised unsupported callbacks")
            if case.get("route") in ("http-modern", "auto-modern"):
                for row in evidence["events"]:
                    if row["kind"] == "business":
                        check(row["ordinaryChoice"] == "2" and row["metaVendorChoice"] == "2", "ordinary/meta carrier selection changed")
        if route.startswith("sse"):
            # The pinned oracle used 2 for either business operation. Preserve
            # its evidence while independently requiring candidate list2/call3.
            expected_business_id = "2" if self.variant == "oracle" or method == "tools/list" else "3"
            for row in evidence["events"]:
                if row["rpcMethod"] == "initialize": check(row["rpcId"] == "1", "SSE initialize ID changed")
                if row["kind"] == "business": check(row["rpcId"] == expected_business_id, "SSE business correlation ID changed")
        if case["kind"] not in ("discovery", "sse-stage-auth"):
            check(evidence["attempts"]["business"] == 1, "business attempt count is not exactly one")
        if case["kind"] == "discovery":
            check(evidence["attempts"] == {"transport": 0, "probe": 1, "control": 0, "business": 0}, "explicit discovery left its one-request scope")
        if case["kind"] == "sse-stage-auth":
            count = {"get": 0, "initialize": 1, "notifications/initialized": 2}[case["stage"]]
            check(evidence["attempts"]["business"] == 0 and evidence["attempts"]["control"] == count, "terminal SSE authentication failure advanced business")
        if case["kind"] == "sse-url":
            endpoint = case["endpoint"]
            expected_get = "fixed.virtual" if endpoint in ("host-rewrite", "path-config-absolute") else "initial.virtual"
            expected_post = "initial.virtual" if endpoint in ("relative", "absolute") else "never-resolve.invalid" if endpoint == "external-domain" else "other.virtual"
            for row in evidence["events"]:
                check(row["backendOrigin"] == "primary", "endpoint changed the actual route backend")
                check(row["authority"] == (expected_get if row["kind"] == "transport" else expected_post), "historical SSE stage Authority changed")
                check(row["path"] == ("/unified/sse" if row["kind"] == "transport" else "/unified/messages"), "historical SSE stage path changed")
                expected_query = ({} if endpoint == "path-config-absolute" else {"initial": ["one"]}) if row["kind"] == "transport" else {"case": [self.case]}
                check(row["query"] == expected_query, "historical SSE stage query changed")
            if response and "result" in response:
                check(response["result"]["recipient"] == expected_post, "same-cluster virtual host recipient did not change")
        if case["kind"] == "sse-auth":
            credential = case["credential"]
            for row in evidence["events"]:
                if credential == "static-header": check(row["upstreamKeyMatches"], "static header key changed recipient/value")
                elif credential == "static-query": check(row["queryCredentialMatches"], "query key changed recipient/value")
                elif credential == "static-authorization": check(row["authorizationAlias"] == "static", "static Authorization changed recipient/value")
                elif credential in ("forward", "passthrough"): check(row["authorizationAlias"] == credential, "authorized credential forwarding changed")
                else: check(row["authorizationAlias"] is None, "removed Authorization restored")
                if credential == "passthrough": check(not row["clientKeyPresent"], "removed downstream key restored")
        if case["kind"].startswith("sse-") and self.variant != "oracle":
            for row in evidence["events"]:
                check([part.strip() for field in row["ordinaryHeaders"] for part in field.split(",")] == ["first", "second"], "ordinary multivalue headers lost")
        projection = {"status": status, "businessAttempts": evidence["attempts"]["business"],
                      "responseHeaders": {name: headers[name] for name in ("retry-after", "www-authenticate") if name in headers}}
        if business_digest is not None: projection["businessResultSha256"] = business_digest
        if response and "error" in response:
            projection.update(errorCode=token_number(response["error"]["code"]), errorMessage=response["error"]["message"])
        return {"projection": projection, "ledger": evidence,
                "clientIDObserved": contract.token(response["id"]) if response and "id" in response else None,
                "precisionExceptions": ["id", "params.arguments.number", "params.arguments.decimal", "params.arguments.exponent", "result.number"] if self.variant == "oracle" else [],
                "retainedFixExceptions": ["SSE ordinary-header multiplicity", "HTTP authentication fidelity"] if self.variant == "oracle" else []}

    def red_case(self, case):
        name = case["red"]
        if name == "modern-over-one-mib": raw, _ = contract.sized_body(True, (1 << 20) + 1, "arguments")
        else: raw = contract.envelope(True, rpc_id="1.50" if name == "fractional-id" else '"case"', tool="legacy-true" if name == "legacyOnly" else "raw_summary")
        port = PORTS["rest"] if name == "legacyOnly" else PORTS["registered"]
        extra = [("Origin", "https://other.example")] if name == "cross-origin" else []
        status, _, response = self.exchange(port, raw, True, tool="legacy-true" if name == "legacyOnly" else "raw_summary", extra=extra)
        expected = {"modern-over-one-mib": 413, "cross-origin": 403, "fractional-id": 400, "schema-call": 200, "legacyOnly": 200}[name]
        if name == "legacyOnly":
            message = 'tool "legacy-true" is unavailable in the modern profile because its input schema cannot be validated: configured as legacy-only'
            result = (response or {}).get("result", {})
            check(status == expected and result.get("isError") is True and result.get("content") == [{"type": "text", "text": message}], "pre-fix legacyOnly tool error result did not reproduce its named gate")
            check(self.ledger()["attempts"]["business"] == 0, "legacyOnly diagnostic reached business")
        else:
            check(status == expected and (name == "modern-over-one-mib" or response and "error" in response), "pre-fix diagnostic did not reproduce its named gate")
        return {"diagnosticRedReproduced": True, "status": status, "notBusinessOracle": True}

    def run(self):
        import verify as existing
        existing.wait_http("http://backend-primary:8080/healthz")
        existing.wait_http("http://gateway:9901/ready")
        expected = contract.registry(self.variant)
        for case in expected:
            self.case = case["id"]
            self.backend(reset=True)
            row = {"id": self.case, "acceptance": case["acceptance"], "status": "FAIL"}
            try:
                kind = case["kind"]
                if kind == "body": detail = self.body_case(case)
                elif kind == "input": detail = self.input_case(case)
                elif kind == "descriptors": detail = self.descriptors(case)
                elif kind == "builtin": detail = self.builtin(case)
                elif kind == "legacy-version":
                    status, _, response = self.exchange(PORTS["registered"], contract.envelope(False), False, extra=[("MCP-Protocol-Version", case["version"])])
                    check(status == 200 and response and "result" in response, "supported Legacy version rejected")
                    detail = {"supportedVersion": case["version"]}
                elif kind == "origin":
                    extra = [] if case["origin"] == "missing" else [("Origin", {"cross-host": "https://other.example", "cross-port": "https://mcp.runtime.test:8443", "null": "null"}[case["origin"]])]
                    modern = case["profile"] == "modern"
                    status, _, response = self.exchange(PORTS["registered"], contract.envelope(modern), modern, extra=extra)
                    check(status == 200 and response and "result" in response, "historical Origin acceptance changed")
                    detail = {"accepted": True}
                elif kind == "red": detail = self.red_case(case)
                else: detail = self.proxy_case(case)
                row.update(status="PASS", detail=detail)
            except Exception as error:
                row["error"] = str(error)
                row["ledger"] = self.ledger()
            self.rows.append(row)
            # Flush each independent case so interruption cannot turn a partial
            # run into a complete-looking result or discard the first failure.
            self.write(expected)
        return 0 if contract.complete(expected, self.rows) and all(row["status"] == "PASS" for row in self.rows) else 1

    def write(self, expected):
        (self.out / f"compat-{self.variant}.json").write_text(json.dumps({"variant": self.variant, "expectedCases": expected,
            "cases": self.rows, "exchanges": self.exchanges}, indent=2, sort_keys=True) + "\n")


def token_number(value):
    return value.lexeme if isinstance(value, JSONNumber) else str(value)


def supplements(variant, out):
    """Retained control/cancellation and instructions oracles remain independent."""
    import verify as existing
    import verify_auto
    existing.RESULTS.clear()
    existing.EXCHANGES.clear()
    expected = [name for name, _ in verify_auto.cases(existing)]
    for name, callback in verify_auto.cases(existing):
        existing.record(name, callback)
    Path(out, f"controls-{variant}.json").write_text(json.dumps({"expectedCases": expected, "cases": existing.RESULTS,
        "exchanges": existing.EXCHANGES}, indent=2, sort_keys=True) + "\n")
    auto_passed = all(row["status"] == "PASS" for row in existing.RESULTS)
    existing.RESULTS.clear()
    existing.EXCHANGES.clear()
    instructions_status = existing.instructions_verification()
    return 0 if auto_passed and instructions_status == 0 else 1


if __name__ == "__main__":
    variant = os.environ["COMPAT_VARIANT"]
    out = os.environ.get("RUNTIME_EVIDENCE", "/evidence")
    if "--supplements" in sys.argv:
        raise SystemExit(supplements(variant, out))
    raise SystemExit(Compatibility(variant, out).run())
