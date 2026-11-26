"""Import the repository dashboard into a task-only real Grafana datasource."""
import base64
import json
import time
import urllib.error
import urllib.request
from pathlib import Path

root = Path(__file__).resolve().parent
runtime = root.parent
info = json.loads((root / 'grafana-inspect.json').read_text())[0]
port = info['NetworkSettings']['Ports']['3000/tcp'][0]
assert port['HostIp'] == '127.0.0.1'
base = 'http://127.0.0.1:' + port['HostPort']
password = (root / 'password').read_text().strip()
headers = {'Authorization': 'Basic ' + base64.b64encode(('fixture-admin:' + password).encode()).decode(), 'Content-Type': 'application/json'}


def api(path, body=None):
    request = urllib.request.Request(base + path, None if body is None else json.dumps(body).encode(), headers)
    return json.loads(urllib.request.urlopen(request, timeout=15).read())


for _ in range(160):
    try:
        health = api('/api/health')
        break
    except (OSError, urllib.error.HTTPError):
        time.sleep(.5)
else:
    raise SystemExit('Grafana not ready')
source = api('/api/datasources', {'name': 'OPE-11003 Prometheus', 'uid': 'ope11003-prometheus', 'type': 'prometheus', 'access': 'proxy', 'url': 'http://ope9626-higress-prometheus:9090', 'isDefault': True, 'jsonData': {'httpMethod': 'POST', 'timeInterval': '1s'}})
original = json.loads((runtime / 'dashboard.json').read_text())
result = api('/api/dashboards/import', {'dashboard': original, 'overwrite': True, 'inputs': [{'name': 'DS_PROMETHEUS', 'type': 'datasource', 'pluginId': 'prometheus', 'value': 'ope11003-prometheus'}]})
baseline = json.loads(json.dumps(original))
baseline['time'] = json.loads((root / 'baseline-time.json').read_text())
baseline['uid'] = 'wujie-model-requests-baseline'
baseline['title'] += ' (baseline range validation)'
api('/api/dashboards/import', {'dashboard':baseline, 'overwrite':True, 'inputs':[{'name':'DS_PROMETHEUS','type':'datasource','pluginId':'prometheus','value':'ope11003-prometheus'}]})
stored = api('/api/dashboards/uid/wujie-model-requests')
assert stored['dashboard']['timezone'] == original['timezone']
assert stored['dashboard']['time'] == original['time']
for imported, expected in zip(stored['dashboard']['panels'], original['panels']):
    assert [x['expr'] for x in imported.get('targets', [])] == [x['expr'] for x in expected.get('targets', [])]
assert stored['dashboard']['templating']['list'][0]['current']['value'] == 'wujieai'
(root / 'import-result.json').write_text(json.dumps({'health': health, 'datasourceUid': source['datasource']['uid'], 'import': result, 'stored': stored}, indent=2))
(root / 'browser-config.json').write_text(json.dumps({'base': base, 'dashboard': '/d/wujie-model-requests', 'username': 'fixture-admin'}, indent=2))
print('Real Grafana import passed; dashboard expressions, timezone and default range retained')
