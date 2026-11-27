#!/usr/bin/env python3
"""Join this demo's SDK observations to buffered public gateway access logs."""
import argparse
import json
from pathlib import Path


class MissingAccess(Exception):
    pass


def correlate(reports, log, run_id):
    expected = {}
    for phase in ("on", "off", "changed"):
        report = reports[phase]
        if report["runId"] != run_id or report["phase"] != phase:
            raise ValueError("SDK report belongs to another run or phase")
        requests = report["httpRequests"]
        if [request["method"] for request in requests] != ["server/discover", "tools/list", "tools/call"]:
            raise ValueError("SDK request sequence changed")
        events = report["upstream"]["events"]
        methods = ["tools/list", "tools/call"] if phase == "off" else ["server/discover", "tools/list", "tools/call"]
        if [event["method"] for event in events] != methods:
            raise ValueError("upstream method sequence changed")
        by_marker = {event["marker"]: event for event in events}
        path = "/instructions/off" if phase == "off" else "/instructions/on"
        for request in requests:
            marker = request["marker"]
            user_agent = request["userAgent"]
            if not marker.startswith(f"demo05-{run_id}-{phase}-") or user_agent != "higress-instructions-demo/" + marker:
                raise ValueError("SDK request correlation changed")
            if request["httpMethod"] != "POST" or request["path"] != path or request["responseCode"] != 200 or user_agent in expected:
                raise ValueError("SDK request path/status or uniqueness changed")
            event = by_marker.get(marker)
            local_discover = phase == "off" and request["method"] == "server/discover"
            if local_discover != (event is None):
                raise ValueError("upstream operation does not match SDK request")
            expected[user_agent] = (phase, request, event)
    rows = {}
    prefix = f"higress-instructions-demo/demo05-{run_id}-"
    for line in log.splitlines():
        try:
            row = json.loads(line)
        except (ValueError, TypeError):
            continue
        if not isinstance(row, dict) or not str(row.get("user_agent", "")).startswith(prefix):
            continue
        user_agent = row["user_agent"]
        if user_agent not in expected or user_agent in rows:
            raise ValueError("unexpected or duplicate current-run gateway access record")
        rows[user_agent] = row
    result = []
    request_ids = set()
    for user_agent, row in rows.items():
        phase, request, event = expected[user_agent]
        route = "mcp-demo/mcp-2026-instructions-" + ("off" if phase == "off" else "on")
        request_id = row.get("request_id")
        if row.get("method") != "POST" or row.get("path") != request["path"] or str(row.get("response_code")) != "200" or row.get("route_name") != route:
            raise ValueError("gateway access path/route/status changed")
        if not request_id or request_id == "-" or request_id in request_ids:
            raise ValueError("gateway request ID absent or duplicated")
        request_ids.add(request_id)
        if event is not None:
            if event["requestId"] != request_id or event["userAgent"] != user_agent or event["method"] != request["method"]:
                raise ValueError("actual gateway ID or marker does not join the upstream SDK event")
            if row.get("upstream_cluster") != "outbound|8080||instructions-mcp.mcp-demo.svc.cluster.local":
                raise ValueError("gateway reached another upstream cluster")
        result.append({"phase": phase, "method": request["method"], "marker": request["marker"],
                       "userAgent": user_agent, "requestId": request_id, "path": row["path"],
                       "route": route, "responseCode": row["response_code"], "upstreamObserved": event is not None})
    if len(rows) != len(expected):
        raise MissingAccess(f"waiting for buffered gateway records: {len(rows)}/{len(expected)}")
    return {"runId": run_id, "status": "PASS", "records": result}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence", type=Path, required=True)
    parser.add_argument("--run-id", required=True)
    args = parser.parse_args()
    reports = {phase: json.loads((args.evidence / (phase + ".json")).read_text()) for phase in ("on", "off", "changed")}
    result = correlate(reports, (args.evidence / "gateway.log").read_text(), args.run_id)
    (args.evidence / "gateway-correlations.json").write_text(json.dumps(result, indent=2) + "\n")


if __name__ == "__main__":
    try:
        main()
    except MissingAccess as exc:
        print(exc)
        raise SystemExit(1)
    except (ValueError, KeyError, OSError, TypeError) as exc:
        print("invalid gateway evidence:", exc)
        raise SystemExit(2)
