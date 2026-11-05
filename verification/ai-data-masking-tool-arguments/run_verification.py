"""Dispatch one verification suite; no build, push, issue update or PR creation."""
import argparse
import subprocess
import sys
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__, add_help=False)
parser.add_argument('--suite', required=True, choices=['runtime', 'limits', 'configuration', 'capacity', 'live', 'host-failure', 'red-green'])
args, remaining = parser.parse_known_args()
script = {'host-failure': 'verify_host_failure.py', 'red-green': 'verify_red_green.py'}.get(args.suite, 'verify_' + args.suite + '.py')
raise SystemExit(subprocess.call([sys.executable, '-u', str(Path(__file__).resolve().parent / script), *remaining]))
