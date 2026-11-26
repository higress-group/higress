"""Sample task containers and one native browser; stop task on host pressure."""
import json
import os
import re
import shutil
import signal
import subprocess
import sys
import time
from pathlib import Path

root = Path(__file__).resolve().parent
names = ['ope9626-higress-' + suffix for suffix in ('gateway', 'mock', 'prometheus', 'grafana')]
parent = int(sys.argv[1])
while True:
    pressure = subprocess.check_output(['memory_pressure'], text=True)
    free = int(re.search(r'System-wide memory free percentage:\s*(\d+)', pressure)[1])
    record = {'time': time.time(), 'freeGiB': round(shutil.disk_usage(root).free/1024**3, 2), 'memoryFreePercent': free, 'load': os.getloadavg()}
    stats = subprocess.run(['/opt/homebrew/bin/docker', '--context', 'colima-ope10117', 'stats', '--no-stream', '--format', '{{json .}}', *names], text=True, capture_output=True)
    record['containers'] = [json.loads(line) for line in stats.stdout.splitlines() if line.startswith('{')]
    if (root / 'native-pids.json').exists():
        pids = json.loads((root / 'native-pids.json').read_text())
        rows = [list(map(int, line.split())) for line in subprocess.check_output(['ps', '-axo', 'pid=,ppid=,rss='], text=True).splitlines() if line.strip()]
        owned = set(pids.values())
        for _ in range(5):
            owned.update(pid for pid, ppid, _ in rows if ppid in owned)
        record['nativeTaskRssMiB'] = round(sum(rss for pid, _, rss in rows if pid in owned)/1024, 1)
    with (root / 'resources.jsonl').open('a') as file:
        file.write(json.dumps(record) + '\n')
    if record['freeGiB'] < 20 or free < 20 or record['load'][0] > 8:
        (root / 'resource-stop.json').write_text(json.dumps(record))
        os.kill(parent, signal.SIGTERM)
        break
    time.sleep(5)
