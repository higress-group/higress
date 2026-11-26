#!/usr/bin/env python3
"""Check the three scoped tag extractors in a rendered or real bootstrap JSON.

This checks regex behavior, not Envoy's actual Prometheus exporter.
"""
import json
import re
import sys
from pathlib import Path

config = json.loads(Path(sys.argv[1]).read_text())
tags = {tag["tag_name"]: tag["regex"] for tag in config["stats_config"]["stats_tags"] if tag.get("tag_name", "").startswith("wujie_")}
assert set(tags) == {"wujie_source", "wujie_model", "wujie_outcome"}, tags
for family in ("requests", "submissions"):
    for source, model, outcome in (("wujieai", "claude-sonnet-4_5", "stream_error"), ("unknown", "unknown", "unknown")):
        base = f"wujie_model_{family}_total"
        raw = f"wasmcustom.{base}.wujie_source={source}.wujie_model={model}.wujie_outcome={outcome}"
        spans = []
        for label, expected in (("source", source), ("model", model), ("outcome", outcome)):
            match = re.search(tags["wujie_" + label], raw)
            assert match and match[2] == expected, (label, raw)
            spans.append(match.span(1))
        extracted = raw
        for start, end in sorted(spans, reverse=True):
            extracted = extracted[:start] + extracted[end:]
        assert extracted == "wasmcustom." + base, extracted
for legacy in ("route.route1.upstream.provider.model.gpt-4.consumer.none.metric.request_success_v1_chat_completions", "http.ingress_http.downstream_rq_total"):
    assert all(re.search(regex, legacy) is None for regex in tags.values()), legacy
print("Scoped tag extraction and legacy-name isolation passed (static check only)")
