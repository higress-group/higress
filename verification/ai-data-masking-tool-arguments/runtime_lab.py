"""Owned Docker resources and evidence for the remaining gateway suites."""
import argparse
import hashlib
import json
import platform
import shutil
import socket
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path

from verify_runtime import BUILDER, GATEWAY, envoy_config


def arguments(description):
    parser = argparse.ArgumentParser(description=description)
    parser.add_argument('--baseline-wasm', type=Path, required=True)
    parser.add_argument('--candidate-wasm', type=Path, required=True)
    parser.add_argument('--baseline-source', required=True)
    parser.add_argument('--candidate-source', required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--resource-prefix', required=True)
    return parser


def sha(data):
    return hashlib.sha256(data).hexdigest()


@dataclass
class Gateway:
    name: str
    container: str
    port: int | None


class Lab:
    def __init__(self, args, suite):
        self.args, self.suite = args, suite
        self.output = args.output.resolve()
        self.output.mkdir(parents=True, exist_ok=False)
        self.cases = self.output / 'cases'
        self.cases.mkdir()
        self.network = args.resource_prefix
        self.network_created = False
        self.containers, self.all_containers, self.ports = [], [], []
        self.commands = []
        self.gateways = []
        source = Path(__file__).resolve().parent
        (self.output / 'source').mkdir()
        for path in source.glob('*.py'):
            shutil.copy2(path, self.output / 'source' / path.name)
        for name, path in [('baseline', args.baseline_wasm), ('candidate', args.candidate_wasm)]:
            shutil.copy2(path, self.output / (name + '.wasm'))
        self.manifest = {'suite': suite, 'gateway': GATEWAY, 'fixture_image': BUILDER, 'baseline_source': args.baseline_source, 'candidate_source': args.candidate_source,
            'baseline_wasm_sha256': sha(args.baseline_wasm.read_bytes()), 'candidate_wasm_sha256': sha(args.candidate_wasm.read_bytes()),
            'scripts_sha256': {path.name: sha(path.read_bytes()) for path in source.glob('*.py')},
            'host_platform': platform.platform(), 'host_arch': platform.machine(), 'workers': 1, 'model_requests': 0, 'tools_executed': 0,
            'limitations': ['Synthetic upstream; no live-model evidence.', 'Upstream writes are not Envoy callback boundaries.', 'Linux/amd64 gateway on the recorded Docker host; no production capacity claim.']}
        self.save('manifest.json', self.manifest)

    def save(self, name, value):
        path = self.output / name
        temporary = path.with_name(path.name + '.next')
        temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n')
        temporary.replace(path)

    def command(self, command, check=True):
        started = time.monotonic()
        result = subprocess.run(command, capture_output=True, text=True)
        self.commands.append({'command': command, 'exit_code': result.returncode, 'elapsed_seconds': time.monotonic() - started})
        self.save('commands.json', self.commands)
        if check and result.returncode:
            raise RuntimeError(result.stdout + result.stderr)
        return result

    def __enter__(self):
        try:
            self.command(['docker', 'network', 'create', self.network])
            self.network_created = True
            source = self.output / 'source'
            fixture = self.command(['docker', 'run', '-d', '--platform', 'linux/amd64', '--name', self.network + '-fixture', '--network', self.network,
                '-v', str(source) + ':/scripts:ro', '-v', str(self.cases) + ':/evidence', '--entrypoint', 'python3', BUILDER, '/scripts/fixture.py', '--evidence', '/evidence']).stdout.strip()
            self.containers.append(fixture)
            self.all_containers.append(fixture)
            self.manifest['docker_host'] = self.command(['docker', 'info', '--format', '{{.OSType}} {{.Architecture}} cpus={{.NCPU}} memory={{.MemTotal}}']).stdout.strip()
            self.save('manifest.json', self.manifest)
            return self
        except BaseException as error:
            self.__exit__(type(error),error,error.__traceback__)
            raise

    def start(self, name, configuration, module='candidate.wasm', modify=None, ready=True, initial_discovery=False, extra_mounts=()):
        config = envoy_config(self.network, module, configuration)
        if modify:
            modify(config)
        self.save(name + '.json', config)
        command = ['docker', 'run', '-d', '--platform', 'linux/amd64', '--name', self.network + '-' + name, '--network', self.network,
            '-v', str(self.output) + ':/lab:ro', '-p', '127.0.0.1::10000']
        for mount in extra_mounts:
            command += ['-v', mount]
        if initial_discovery:
            # Watch and replace inside Linux; Docker Desktop host moves may not
            # generate the inotify event used by filesystem xDS subscriptions.
            command += ['--entrypoint', '/bin/sh', GATEWAY, '-c',
                'cp /lab/update-good-1.json /tmp/extension.json && exec /usr/local/bin/envoy -c "$1" --concurrency 1 --log-level info',
                'gateway', '/lab/' + name + '.json']
        else:
            command += ['--entrypoint', '/usr/local/bin/envoy', GATEWAY, '-c', '/lab/' + name + '.json', '--concurrency', '1', '--log-level', 'info']
        container = self.command(command).stdout.strip()
        self.containers.append(container)
        self.all_containers.append(container)
        port = None
        if ready:
            raw = self.command(['docker', 'port', container, '10000/tcp']).stdout.strip()
            port = int(raw.rsplit(':', 1)[1])
            self.ports.append(port)
            for attempt in range(150):
                result = subprocess.run(['docker', 'exec', container, 'curl', '-sf', 'http://127.0.0.1:15000/ready'], capture_output=True)
                if result.returncode == 0:
                    break
                time.sleep(.2)
            else:
                raise RuntimeError('gateway not ready: ' + name)
        gateway = Gateway(name, container, port)
        self.gateways.append(gateway)
        return gateway

    def request(self, gateway, case, request, scenario='normal', delivery='single'):
        raw = request if isinstance(request, bytes) else json.dumps(request, ensure_ascii=False).encode()
        (self.cases / (case + '.request')).write_bytes(raw)
        result = self.command(['curl', '-sS', '-N', '--connect-timeout', '3', '--max-time', '120', 'http://127.0.0.1:' + str(gateway.port) + '/v1/chat/completions',
            '-H', 'Content-Type: application/json', '-H', 'X-Test-Case: ' + case, '-H', 'X-Scenario: ' + scenario, '-H', 'X-Delivery: ' + delivery,
            '--data-binary', '@' + str(self.cases / (case + '.request')), '-D', str(self.cases / (case + '.headers')), '-o', str(self.cases / (case + '.body')), '-w', '%{http_code}'], check=False)
        body = (self.cases / (case + '.body')).read_bytes() if (self.cases / (case + '.body')).exists() else b''
        upstream = self.cases / (case + '.upstream-request')
        record = {'case': case, 'gateway': gateway.name, 'scenario': scenario, 'delivery': delivery, 'curl_exit': result.returncode, 'status': result.stdout.strip(),
            'request_sha256': sha(raw), 'body_sha256': sha(body), 'upstream_received': upstream.exists()}
        return record, body

    def collect(self, gateway):
        logs = self.command(['docker', 'logs', gateway.container], check=False)
        (self.output / (gateway.name + '.log')).write_text(logs.stdout + logs.stderr)
        for label, endpoint in [('stats', 'stats'), ('config-dump', 'config_dump')]:
            result = self.command(['docker', 'exec', gateway.container, 'curl', '-sf', 'http://127.0.0.1:15000/' + endpoint], check=False)
            (self.output / (gateway.name + '.' + label)).write_text(result.stdout + result.stderr)
        state = self.command(['docker', 'inspect', '--format', '{{json .State}}', gateway.container], check=False)
        (self.output / (gateway.name + '.state.json')).write_text(state.stdout)

    def stop(self, gateway):
        self.collect(gateway)
        self.command(['docker', 'rm', '-f', gateway.container])
        self.containers.remove(gateway.container)
        self.gateways.remove(gateway)

    def __exit__(self, kind, error, _traceback):
        if error:
            self.save('failure.json', {'type': kind.__name__, 'error': str(error)})
        for gateway in list(self.gateways):
            self.collect(gateway)
        if self.all_containers:
            fixture_logs = self.command(['docker', 'logs', self.all_containers[0]], check=False)
            (self.output / 'fixture.log').write_text(fixture_logs.stdout + fixture_logs.stderr)
        for container in self.containers:
            self.command(['docker', 'rm', '-f', container], check=False)
        if self.network_created:
            self.command(['docker', 'network', 'rm', self.network], check=False)
        ports = {}
        for port in self.ports:
            with socket.socket() as connection:
                connection.settimeout(.5)
                ports[str(port)] = connection.connect_ex(('127.0.0.1', port)) != 0
        self.save('cleanup.json', {'containers_absent': {container: subprocess.run(['docker', 'inspect', container], capture_output=True).returncode != 0 for container in self.all_containers},
            'network_absent': subprocess.run(['docker', 'network', 'inspect', self.network], capture_output=True).returncode != 0, 'ports_not_listening': ports})
        files = sorted(path for path in self.output.rglob('*') if path.is_file() and path.name != 'SHA256SUMS')
        (self.output / 'SHA256SUMS').write_text(''.join(sha(path.read_bytes()) + '  ' + str(path.relative_to(self.output)) + '\n' for path in files))
