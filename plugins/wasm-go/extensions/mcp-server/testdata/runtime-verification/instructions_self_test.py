#!/usr/bin/env python3
"""Focused adversarial checks for the instructions evidence contract."""
import copy
import hashlib
import unittest

import verify
from instructions_contract import case_requests, focused_evidence


def valid_evidence(variant):
    contract = case_requests(variant)
    cases, exchanges, accesses = [], [], []
    for name, expected in contract.items():
        recorded = []
        for item in expected:
            exchange = {"case": name, "accessRequestId": item["accessRequestId"],
                        "request": {"listenerPort": item["port"], "rpcId": item["rpcId"], "rpcMethod": "server/discover",
                                    "mcpMethod": "server/discover", "protocolVersion": verify.MODERN,
                                    "httpMethod": "POST", "path": "/mcp", "instructionsMode": item["mode"]},
                        "response": {"status": 200}}
            if item["raw"]:
                raw = verify.instructions_disconnect_request()
                exchange.update({"transport": "raw-disconnect", "rawRequest": raw.decode(),
                                 "rawRequestSha256": hashlib.sha256(raw).hexdigest(), "response": {"status": "client_disconnected"}})
            recorded.append(exchange)
            code, flags = (0, "DC") if item["raw"] else (200, "-")
            accesses.append(f"access request_id={item['accessRequestId']} listener=instructions-{item['port']} method=POST path={item['accessPath']} code={code} flags={flags}\n")
        cases.append({"case": name, "status": "PASS", "detail": {"clientExchanges": recorded}})
        exchanges.extend(recorded)
    return {"expectedCases": list(contract), "cases": cases, "exchanges": exchanges}, "".join(accesses)


class InstructionsEvidenceTest(unittest.TestCase):
    def test_complete_independently_pinned_cases_and_requests(self):
        for variant, case_count, request_count in (("baseline", 2, 2), ("candidate", 29, 33)):
            snapshot, log = valid_evidence(variant)
            result = focused_evidence(snapshot, variant, log)
            self.assertEqual(result["cases"], case_count)
            self.assertEqual(result["exchangeCount"], request_count)
            self.assertTrue(result["complete"] and result["exchangeLedgerComplete"] and result["accessLedgerComplete"])

    def test_case_substitution_is_rejected_even_with_same_count(self):
        snapshot, log = valid_evidence("candidate")
        snapshot["expectedCases"][0] = "substituted"
        snapshot["cases"][0]["case"] = "substituted"
        self.assertFalse(focused_evidence(snapshot, "candidate", log)["complete"])

    def test_exchange_loss_duplication_and_case_misattribution_are_rejected(self):
        original, log = valid_evidence("candidate")
        for fault in ("missing", "duplicate", "case", "case-detail", "metadata"):
            with self.subTest(fault=fault):
                snapshot = copy.deepcopy(original)
                if fault == "missing": snapshot["exchanges"].pop()
                elif fault == "duplicate": snapshot["exchanges"].append(copy.deepcopy(snapshot["exchanges"][0]))
                elif fault == "case": snapshot["exchanges"][0]["case"] = "field-normal"
                elif fault == "case-detail": snapshot["cases"][0]["detail"]["clientExchanges"] = []
                elif fault == "metadata": snapshot["exchanges"][0]["request"]["rpcMethod"] = "tools/list"
                self.assertFalse(focused_evidence(snapshot, "candidate", log)["exchangeLedgerComplete"])

    def test_raw_disconnect_payload_and_access_are_required(self):
        original, log = valid_evidence("candidate")
        for fault in ("payload", "hash", "response"):
            snapshot = copy.deepcopy(original)
            raw = next(item for item in snapshot["exchanges"] if item.get("transport") == "raw-disconnect")
            if fault == "payload": raw.pop("rawRequest")
            elif fault == "hash": raw["rawRequestSha256"] = "wrong"
            else: raw["response"] = {"status": 200}
            self.assertFalse(focused_evidence(snapshot, "candidate", log)["exchangeLedgerComplete"])
        missing = "".join(line for line in log.splitlines(keepends=True) if "request_id=rv-instructions-disconnect " not in line)
        self.assertFalse(focused_evidence(original, "candidate", missing)["accessLedgerComplete"])

    def test_missing_duplicate_unexpected_or_wrong_listener_access_fails(self):
        snapshot, log = valid_evidence("candidate")
        lines = log.splitlines(keepends=True)
        for broken in ("".join(lines[1:]), log + lines[0], log + "access request_id=unexpected listener=instructions-14000 method=POST path=/instructions code=200 flags=-\n",
                       log + "access request_id=malformed\n",
                       log.replace("listener=instructions-14001", "listener=other", 1),
                       log.replace("path=/mcp", "path=/wrong", 1), log.replace("code=200", "code=503", 1),
                       log.replace("code=0 flags=DC", "code=200 flags=-")):
            self.assertFalse(focused_evidence(snapshot, "candidate", broken)["accessLedgerComplete"])

    def test_rpc_error_must_not_include_normal_instructions_result(self):
        verify.check_instructions_failure(200, {"error": {"code": -32603}}, "rpc-error")
        for value in ("Use the weather tool. 天气", "must not extract", ""):
            with self.subTest(value=value):
                with self.assertRaises(AssertionError):
                    verify.check_instructions_failure(200, {"error": {"code": -32603}, "result": {"instructions": value}}, "rpc-error")

    def test_legacy_bridge_requires_ordinary_headers_and_isolates_controls(self):
        methods = ["initialize", "notifications/initialized", "tools/list", "initialize", "notifications/initialized", "tools/call"]
        sessions = [False, True, True, False, True, True]
        events = [{"rpcMethod": method, "futureParam": None, "cookiePresent": True, "unrelatedCredentialPresent": True,
                   "lastEventIDPresent": False, "internalRoutePresent": False, "authorizationPresent": False,
                   "sessionPresent": session, "fixtureSessionPresent": session} for method, session in zip(methods, sessions)]
        verify.check_modern_to_legacy_events(events)
        for index in range(6):
            for key, value in (("cookiePresent", False), ("unrelatedCredentialPresent", False), ("lastEventIDPresent", True),
                               ("authorizationPresent", True), ("internalRoutePresent", True), ("futureParam", "leak"),
                               ("fixtureSessionPresent", not sessions[index])):
                with self.subTest(index=index, key=key):
                    broken = copy.deepcopy(events)
                    broken[index][key] = value
                    with self.assertRaises(AssertionError): verify.check_modern_to_legacy_events(broken)


if __name__ == "__main__":
    unittest.main()
