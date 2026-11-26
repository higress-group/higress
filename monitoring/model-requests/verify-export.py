#!/usr/bin/env python3
"""Validate a REAL per-Pod /stats/prometheus capture, without assuming its prefix.

This reports the actual names/labels; it cannot establish request outcome correctness.
Usage: curl -fsS .../stats/prometheus > /tmp/stats.txt
       python3 verify-export.py /tmp/stats.txt [--canonical]
"""
import argparse
import json
import re
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("capture", type=Path)
parser.add_argument("--canonical", action="store_true", help="validate captured Prometheus federation text after metric relabeling")
args = parser.parse_args()
outcomes = {"success", "http_error", "stream_error", "timeout", "interrupted", "unknown"}
seen = set()
for line in args.capture.read_text().splitlines():
    if line.startswith("#") or not line.strip():
        continue
    match = re.fullmatch(r'([^\s{]+)\{(.*)\}\s+([0-9.eE+\-]+)(?:\s+\d+)?', line)
    if not match or "wujie_model_" not in match[1]:
        continue
    name = match[1]
    if not re.fullmatch(r'.*wujie_model_(requests|submissions)_total', name):
        raise SystemExit(f"Unextracted dimensions in name: {name}")
    labels = {k: json.loads(v) for k, v in re.findall(r'([A-Za-z_][A-Za-z_0-9]*)=("(?:[^"\\]|\\.)*")', match[2])}
    prefix = "" if args.canonical else "wujie_"
    for label in ("source", "model", "outcome"):
        if prefix + label not in labels:
            raise SystemExit(f"Missing {prefix + label}: {name}")
    if labels[prefix + "outcome"] not in outcomes:
        raise SystemExit(f"Unexpected outcome: {labels}")
    if any(not re.fullmatch(r"[A-Za-z0-9_-]{1,64}", labels[prefix + k]) for k in ("source", "model")):
        raise SystemExit(f"Unbounded dimension: {labels}")
    if args.canonical and name not in ("wujie_model_requests_total", "wujie_model_submissions_total"):
        raise SystemExit(f"Name not canonical: {name}")
    if any(k in labels for k in ("uid", "consumer", "request_id", "uri", "error_message")):
        raise SystemExit(f"Unexpected business dimensions: {labels}")
    seen.add((name, tuple(sorted(labels))))
if not seen:
    raise SystemExit("No terminal counters found; run real requests and verify the filter/bootstrap/scrape configuration")
print(json.dumps([{"name": name, "labels": list(labels)} for name, labels in sorted(seen)], indent=2))
