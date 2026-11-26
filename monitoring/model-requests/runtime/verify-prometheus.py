"""Check real Prometheus scrape/relabel and the seven unmodified dashboard expressions."""
import json
import subprocess
import time
import urllib.parse
import urllib.request
from pathlib import Path

root = Path(__file__).resolve().parent
container = json.loads((root / 'prometheus-inspect.json').read_text())[0]
port = container['NetworkSettings']['Ports']['9090/tcp'][0]
assert port['HostIp'] == '127.0.0.1'
base = 'http://127.0.0.1:' + port['HostPort']
for _ in range(80):
    try:
        urllib.request.urlopen(base + '/-/ready', timeout=1).read()
        break
    except OSError:
        time.sleep(.25)
else:
    raise SystemExit('Prometheus not ready')
time.sleep(2)


def api(path, params=None):
    return json.loads(urllib.request.urlopen(base + '/api/v1/' + path + '?' + urllib.parse.urlencode(params or {}), timeout=5).read())


targets = api('targets')
assert targets['data']['activeTargets'][0]['health'] == 'up', targets
(root / 'prometheus-targets.json').write_text(json.dumps(targets, indent=2))
series = api('query', {'query': 'wujie_model_requests_total'})
assert series['data']['result'], series
for item in series['data']['result']:
    labels = item['metric']
    assert all(key in labels for key in ('source', 'model', 'outcome', 'instance', 'pod'))
    assert not any(key in labels for key in ('wujie_source', 'wujie_model', 'wujie_outcome'))
(root / 'prometheus-series.json').write_text(json.dumps(series, indent=2))
queries = []
for panel in json.loads((root / 'dashboard.json').read_text())['panels']:
    for target in panel.get('targets', []):
        expr = target['expr'].replace('$source', 'wujieai').replace('$model', '.*')
        result = api('query', {'query': expr})
        assert result['status'] == 'success' and result['data']['result'], (expr, result)
        queries.append({'panel': panel['id'], 'expression': expr, 'result': result})
assert len(queries) == 7
(root / 'prometheus-dashboard-queries.json').write_text(json.dumps(queries, indent=2))
params = urllib.parse.urlencode({'match[]': '{__name__=~"wujie_model_(requests|submissions)_total"}'})
(root / 'prometheus-federation.txt').write_bytes(urllib.request.urlopen(base + '/federate?' + params, timeout=5).read())
subprocess.run(['python3', str(root / 'verify-export.py'), str(root / 'prometheus-federation.txt'), '--canonical'], check=True)
print('Prometheus target up; canonical labels and all 7 dashboard expressions passed')
