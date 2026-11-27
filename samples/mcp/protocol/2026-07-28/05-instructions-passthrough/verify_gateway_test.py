import copy
import json
import unittest

from verify_gateway import MissingAccess, correlate


class GatewayCorrelationTest(unittest.TestCase):
    def setUp(self):
        self.run_id = "current-run"
        self.reports = {}
        self.rows = []
        for phase in ("on", "off", "changed"):
            requests, events = [], []
            for index, method in enumerate(("server/discover", "tools/list", "tools/call"), 1):
                marker = f"demo05-{self.run_id}-{phase}-{index}"
                user_agent = "higress-instructions-demo/" + marker
                request_id = f"gateway-generated-{phase}-{index}"
                path = "/instructions/off" if phase == "off" else "/instructions/on"
                requests.append({"method": method, "httpMethod": "POST", "path": path,
                                 "marker": marker, "userAgent": user_agent, "responseCode": 200})
                if phase != "off" or method != "server/discover":
                    events.append({"method": method, "requestId": request_id, "marker": marker, "userAgent": user_agent})
                self.rows.append({"method": "POST", "path": path, "response_code": "200", "request_id": request_id,
                                  "user_agent": user_agent, "route_name": "mcp-demo/mcp-2026-instructions-" + ("off" if phase == "off" else "on"),
                                  "upstream_cluster": "outbound|8080||instructions-mcp.mcp-demo.svc.cluster.local"})
            self.reports[phase] = {"runId": self.run_id, "phase": phase, "httpRequests": requests, "upstream": {"events": events}}

    def log(self, rows):
        return "ordinary non-access log\n" + "\n".join(json.dumps(row) for row in rows)

    def test_regenerated_ids_join_by_current_run_user_agent(self):
        result = correlate(self.reports, self.log(self.rows), self.run_id)
        self.assertEqual(result["status"], "PASS")
        self.assertEqual(len(result["records"]), 9)

    def test_buffered_and_previous_run_records_cannot_pass_early(self):
        with self.assertRaises(MissingAccess):
            correlate(self.reports, self.log(self.rows[:-1]), self.run_id)
        stale = self.log(self.rows).replace("current-run", "old-run")
        with self.assertRaises(MissingAccess):
            correlate(self.reports, stale, self.run_id)
        self.assertEqual(correlate(self.reports, stale + "\n" + self.log(self.rows), self.run_id)["status"], "PASS")

    def test_access_corruption_is_rejected(self):
        for key, value in (("path", "/wrong"), ("response_code", "500"), ("route_name", "other-route"),
                           ("request_id", "client-id-that-did-not-reach-upstream"), ("upstream_cluster", "other-cluster")):
            with self.subTest(key=key):
                rows = copy.deepcopy(self.rows)
                rows[0][key] = value
                with self.assertRaises(ValueError):
                    correlate(self.reports, self.log(rows), self.run_id)
        with self.assertRaises(ValueError):
            correlate(self.reports, self.log(self.rows + [self.rows[0]]), self.run_id)
        reports = copy.deepcopy(self.reports)
        reports["off"]["upstream"]["events"].insert(0, reports["on"]["upstream"]["events"][0])
        with self.assertRaises(ValueError):
            correlate(reports, self.log(self.rows), self.run_id)


if __name__ == "__main__":
    unittest.main()
