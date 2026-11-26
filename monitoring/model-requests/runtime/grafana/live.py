"""Small current-hour real gateway probe, separate from historical display data."""
import http.client
import json
import time
import urllib.request
from pathlib import Path

root = Path(__file__).resolve().parent
runtime = root.parent
ports = json.loads((runtime / 'gateway-inspect.json').read_text())[0]['NetworkSettings']['Ports']
gateway = int(ports['8080/tcp'][0]['HostPort'])
admin = int(ports['9901/tcp'][0]['HostPort'])
for _ in range(160):
    try:
        urllib.request.urlopen(f'http://127.0.0.1:{admin}/ready', timeout=1).read()
        break
    except OSError:
        time.sleep(.25)
else:
    raise SystemExit('Gateway not ready')
def observed(stats, outcome):
    return [line for line in stats.splitlines() if line.startswith('wujie_model_requests_total{') and 'wujie_source="wujieai"' in line and f'wujie_outcome="{outcome}"' in line]


checks = []
for index in range(2):
    for case, outcome in [('success', 'success'), ('sse-normal', 'success'), ('sse-error', 'stream_error'), ('sse-missing', 'unknown')]:
        before = urllib.request.urlopen(f'http://127.0.0.1:{admin}/stats/prometheus').read().decode()
        old_value = sum(float(line.rsplit(' ', 1)[1]) for line in observed(before, outcome))
        conn = http.client.HTTPConnection('127.0.0.1', gateway, timeout=4)
        conn.request('POST', '/v1/chat/completions', json.dumps({'model': 'gpt-4', 'messages': [{'role': 'user', 'content': 'grafana live fixture'}], 'stream': case.startswith('sse')}), {'Content-Type': 'application/json', 'x-test-case': case, 'x-wujie-source': 'wujieai', 'request-id': f'grafana-{index}-{case}'})
        response = conn.getresponse()
        response.read()
        conn.close()
        assert response.status == 200, (case, response.status)
        time.sleep(.1)
        stats = urllib.request.urlopen(f'http://127.0.0.1:{admin}/stats/prometheus').read().decode()
        matching = observed(stats, outcome)
        delta = sum(float(line.rsplit(' ', 1)[1]) for line in matching) - old_value
        assert delta == 1, (case, outcome, delta)
        checks.append({'case': case, 'outcome': outcome, 'httpStatus': response.status, 'counterDelta': delta, 'actualMetric': matching})
    # Let Prometheus sample between two request groups.
    time.sleep(2)
(root / 'live-probe.json').write_text(json.dumps(checks, indent=2))
(runtime / 'actual-stats.txt').write_text(stats)
# Generate actual async submission traffic for its separate query panel.
conn = http.client.HTTPConnection('127.0.0.1', gateway, timeout=4)
conn.request('POST', '/v1/tasks', '{"model":"gpt-4"}', {'Content-Type':'application/json','x-wujie-source':'wujieai'})
response = conn.getresponse(); response.read(); assert response.status==200; conn.close()
print('8 current-hour real Higress HTTP/SSE probes passed')
