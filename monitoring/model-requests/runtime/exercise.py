"""Run small sequential real-Envoy checks; retain actual stats and failures."""
import http.client
import json
import os
import re
import statistics
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

root = Path(__file__).resolve().parent
container = json.loads(Path(sys.argv[1]).read_text())[0]
ports = container["NetworkSettings"]["Ports"]
for key in ("8080/tcp", "9901/tcp"):
    assert ports[key][0]["HostIp"] == "127.0.0.1"
gateway = int(ports["8080/tcp"][0]["HostPort"])
admin = int(ports["9901/tcp"][0]["HostPort"])
base = f"http://127.0.0.1:{admin}"
for attempt in range(120):
    try:
        urllib.request.urlopen(base + "/ready", timeout=1).read()
        break
    except (OSError, urllib.error.HTTPError):
        time.sleep(0.25)
else:
    raise SystemExit("Envoy did not become ready; inspect gateway.log")


def stats():
    text = urllib.request.urlopen(base + "/stats/prometheus", timeout=2).read().decode()
    result = {}
    for line in text.splitlines():
        if line.startswith("#") or "wujie_model_" not in line:
            continue
        match = re.fullmatch(r'([^\s{]+)\{(.*)\}\s+([0-9.eE+\-]+)', line)
        if not match:
            raise AssertionError("unexpected actual metric: " + line)
        labels = dict(re.findall(r'(\w+)="([^"]*)"', match[2]))
        key = ("submissions" if "submissions" in match[1] else "requests",
               labels.get("wujie_source"), labels.get("wujie_model"), labels.get("wujie_outcome"))
        assert all(key), "missing actual extracted tags: " + line
        result[key] = result.get(key, 0) + float(match[3])
    (root / "actual-stats.txt").write_text(text)
    return result


def request(case, path="/v1/chat/completions", source="wujieai", extra=None, close=False):
    headers = {"Content-Type": "application/json", "x-test-case": case,
               "request-id": case + "-" + path.rsplit("/", 1)[-1], "x-original-api-key": "synthetic-user-key", "wujie_task_id": "999"}
    if source is not None:
        headers["x-wujie-source"] = source
    headers.update(extra or {})
    body = {"model": "gpt-4", "messages": [{"role": "user", "content": "fixture"}], "stream": case.startswith("sse") or close}
    if path == "/v1/responses":
        body = {"model": "gpt-4", "input": "fixture", "stream": True}
    conn = http.client.HTTPConnection("127.0.0.1", gateway, timeout=4)
    try:
        conn.request("POST", path, json.dumps(body), headers)
        response = conn.getresponse()
        status = response.status
        data = response.read(1) if close else response.read()
        return status, data.decode(errors="replace")
    except http.client.IncompleteRead as exc:
        return 200, exc.partial.decode(errors="replace")
    finally:
        conn.close()


results = []
cases = [
    ("success", {}, "success", "wujieai", "gpt-4", "requests"),
    ("success", {"source": None}, "success", "unknown", "gpt-4", "requests"),
    ("success", {"source": "arbitrary"}, "success", "unknown", "gpt-4", "requests"),
    ("http429", {}, "http_error", "wujieai", "gpt-4", "requests"),
    ("http500", {}, "http_error", "wujieai", "gpt-4", "requests"),
    ("sse-normal", {}, "success", "wujieai", "gpt-4", "requests"),
    ("sse-error", {}, "stream_error", "wujieai", "gpt-4", "requests"),
    ("sse-missing", {}, "unknown", "wujieai", "gpt-4", "requests"),
    ("timeout", {}, "timeout", "wujieai", "gpt-4", "requests"),
    ("sse-reset", {}, "interrupted", "wujieai", "gpt-4", "requests"),
    ("downstream-close", {"close": True}, "interrupted", "wujieai", "gpt-4", "requests"),
    ("retry", {}, "success", "wujieai", "gpt-4", "requests"),
    ("success", {"extra": {"x-test-reject": "yes"}}, "http_error", "wujieai", "unknown", "requests"),
    ("success", {"extra": {"x-test-early-reject": "yes"}}, None, "wujieai", "unknown", "requests"),
    ("success", {"path": "/v1/tasks"}, "success", "wujieai", "gpt-4", "submissions"),
    ("sse-missing", {"path": "/v1/responses"}, "unknown", "wujieai", "gpt-4", "requests"),
    ("sse-normal", {"path": "/v1/responses"}, "success", "wujieai", "gpt-4", "requests"),
    ("sse-fixed-missing", {"path": "/v1/responses"}, "unknown", "wujieai", "gpt-4", "requests"),
]
benchmark_only = os.environ.get("OPE9626_BENCH_ONLY") == "1"
if benchmark_only:
    cases = []
for case, options, outcome, source, model, family in cases:
    before = stats()
    started = time.monotonic()
    status, body = request(case, **options)
    elapsed_ms = (time.monotonic() - started) * 1000
    time.sleep(0.4 if case == "downstream-close" else 0.08)
    after = stats()
    delta = {key: value - before.get(key, 0) for key, value in after.items() if value != before.get(key, 0)}
    expected = {} if outcome is None else {(family, source, model, outcome): 1}
    results.append({"case": case, "options": options, "httpStatus": status,
                    "delta": [{"labels": list(k), "value": v} for k, v in delta.items()],
                    "passed": delta == expected, "latencyMs": elapsed_ms,
                    "response": body if options.get("path") == "/v1/responses" else ""})

time.sleep(0.2)
received = json.loads(urllib.request.urlopen(f"http://127.0.0.1:{gateway}/__reports", timeout=2).read())
(root / "actual-reports.json").write_text(json.dumps(received, indent=2))
for request_id in (() if benchmark_only else ("success-completions", "sse-normal-completions")):
    reports = [r for r in received["reports"] if r["requestId"] == request_id and r["totalToken"] == 15]
    expected_sources = {"wujieai", "unknown"} if request_id == "success-completions" else {"wujieai"}
    passed = bool(reports) and {r["productSource"] for r in reports} == expected_sources
    passed = passed and all(r["inputToken"] == 10 and r["outputToken"] == 5 and r["taskId"] == "999" and r["userKey"] == "synthetic-user-key" for r in reports)
    results.append({"case": "report source/legacy fields " + request_id, "passed": passed, "reports": len(reports)})
if not benchmark_only:
    results.append({"case": "upstream retry attempted twice", "passed": received["attempts"].get("retry-completions") == 2})
if os.environ.get("OPE9626_PROMETHEUS") == "1":
    # Give Prometheus a first sample of each series, then create observable increments.
    time.sleep(2)
    for case in ("success", "sse-error", "sse-missing", "timeout"):
        request(case)
    request("success", path="/v1/tasks")
benchmarks = []
for case in ("success", "sse-normal"):
    for _ in range(3):
        request(case)
    elapsed = []
    for _ in range(20):
        started = time.monotonic()
        status, _ = request(case)
        assert status == 200
        elapsed.append((time.monotonic() - started) * 1000)
    benchmarks.append({"case": case, "requests": len(elapsed), "p50Ms": statistics.median(elapsed), "p95Ms": sorted(elapsed)[18]})
(root / "benchmark.json").write_text(json.dumps({"terminalMetrics": os.environ.get("OPE9626_TERMINAL_METRICS", "1") == "1", "samples": benchmarks}, indent=2))
(root / "actual-config-dump.json").write_bytes(urllib.request.urlopen(base + "/config_dump", timeout=2).read())
(root / "results.json").write_text(json.dumps(results, indent=2))
failures = [r["case"] for r in results if not r["passed"]]
print(json.dumps({"checks": len(results), "failures": failures}, indent=2))
sys.exit(1 if failures else 0)
