"""Build explicitly synthetic historical series for complete-hour browser assertions.

Live gateway data stays under job=higress-fixture in the current hour. This
backfill uses job=hour-fixture and proves visualization, never business volume.
"""
import json
import time
from pathlib import Path

root = Path(__file__).resolve().parent
hour = int(time.time()) // 3600 * 3600
start = hour - 26 * 3600
series = [
    ('wujieai', 'gpt-4', 'success', 6),
    ('wujieai', 'gpt-4', 'http_error', 2),
    ('wujieai', 'gpt-4', 'unknown', 2),
    ('wujieai', 'model-failed', 'http_error', 10),
    ('unknown', 'gpt-4', 'success', 100),
]
lines = ['# TYPE wujie_model_requests_total counter']
for source, model, outcome, mixed_rate in series:
    for minute in range(26 * 60 + 1):
        # Last three hours: mixed, failures only, then no calls; earlier history exposes the left-endpoint bucket.
        old_minutes = min(minute, 23 * 60)
        mixed_minutes = min(max(minute - 23 * 60, 0), 60)
        failed_minutes = min(max(minute - 24 * 60, 0), 60)
        failed_rate = 10 if outcome == 'http_error' else 0
        value = old_minutes / 60 + mixed_rate * mixed_minutes / 60 + failed_rate * failed_minutes / 60
        labels = f'source="{source}",model="{model}",outcome="{outcome}",job="hour-fixture",instance="synthetic-history",pod="history-fixture",namespace="fixture"'
        lines.append(f'wujie_model_requests_total{{{labels}}} {value:.9f} {start + minute * 60}')
lines.append('# TYPE wujie_model_submissions_total counter')
for minute in range(26 * 60 + 1):
    value = min(minute, 23 * 60) / 60 + 4 * min(max(minute - 23 * 60, 0), 60) / 60
    lines.append(f'wujie_model_submissions_total{{source="wujieai",model="gpt-4",outcome="success",job="hour-fixture",instance="synthetic-history",pod="history-fixture",namespace="fixture"}} {value:.9f} {start + minute * 60}')
lines.append('# EOF')
(root / 'history.openmetrics').write_text('\n'.join(lines) + '\n')
(root / 'fixture.json').write_text(json.dumps({'hour': hour, 'mixedEnd': hour - 7200, 'failedEnd': hour - 3600, 'emptyEnd': hour, 'absentEnd': start - 3600, 'job': 'hour-fixture', 'expectedMixed': {'overallSuccessRate': .3, 'gpt4SuccessRate': .6, 'allFailedModelSuccessRate': 0, 'unknownShare': .1}, 'limitation': 'Synthetic historical counters, not shifted real gateway traffic or actual business records.'}, indent=2))
