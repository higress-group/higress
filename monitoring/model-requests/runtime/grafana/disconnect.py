"""Target the observed fixture timing problem; production monitor is unchanged."""
import http.client
import json
import time
import urllib.request
from pathlib import Path

root = Path(__file__).resolve().parent
ports = json.loads((root.parent / 'gateway-inspect.json').read_text())[0]['NetworkSettings']['Ports']
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


def stats():
    text = urllib.request.urlopen(f'http://127.0.0.1:{admin}/stats/prometheus').read().decode()
    counts = {}
    for outcome in ('interrupted', 'timeout', 'success', 'unknown'):
        counts[outcome] = sum(float(line.rsplit(' ', 1)[1]) for line in text.splitlines() if line.startswith('wujie_model_requests_total{') and 'wujie_source="wujieai"' in line and f'wujie_outcome="{outcome}"' in line)
    return counts


results = []
for index in range(3):
    before = stats()
    conn = http.client.HTTPConnection('127.0.0.1', gateway, timeout=4)
    conn.request('POST', '/v1/disconnect/chat/completions', '{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"disconnect fixture"}]}', {'Content-Type':'application/json','x-test-case':'downstream-close','x-wujie-source':'wujieai','request-id':f'disconnect-target-{index}'})
    response = conn.getresponse()
    # Confirm a real, complete first SSE event before intentionally closing.
    assert response.status == 200, response.status
    first_event = []
    while True:
        line = response.readline()
        assert line, 'Stream ended before first event'
        first_event.append(line)
        if line in (b'\n', b'\r\n'):
            break
    assert b''.join(first_event).startswith(b'data: ')
    response.close()
    conn.close()
    for _ in range(40):
        delta = {key: value - before.get(key, 0) for key, value in stats().items()}
        if sum(delta.values()):
            break
        time.sleep(.05)
    passed = delta == {'interrupted':1, 'timeout':0, 'success':0, 'unknown':0}
    results.append({'iteration':index,'httpStatus':200,'firstEventReceived':True,'routeTimeout':'3s','delta':delta,'passed':passed})
    assert passed, results[-1]
(root / 'disconnect-result.json').write_text(json.dumps(results, indent=2))
print('3 targeted disconnect checks passed: complete first event -> close -> interrupted exactly once, no timeout')
