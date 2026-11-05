"""Local capacity observations with exact functional checks; no production sizing claim."""
import concurrent.futures
import http.client
import json
import statistics
import subprocess
import threading
import time
from copy import deepcopy

from runtime_lab import Lab, arguments, sha
from verify_limits import compress_case, keymap
from verify_runtime import BASE_CONFIG, parse, request_for


def sizes(smoke):
    groups = [('4k', 4096, 1), ('256k', 262144, 1), ('1m', 1048576, 1), ('near-total', 1044480, 3)]
    return groups[:1] if smoke else groups


def probe(container):
    command = ['docker', 'exec', container, '/bin/sh', '-c',
        'cat /proc/1/status; cat /sys/fs/cgroup/cpu.stat; cat /sys/fs/cgroup/memory.current']
    result = subprocess.run(command, capture_output=True, text=True)
    row = {'monotonic': time.monotonic(), 'exit_code': result.returncode, 'raw': result.stdout, 'stderr': result.stderr}
    for line in result.stdout.splitlines():
        if line.startswith('VmRSS:'):
            row['rss_bytes'] = int(line.split()[1]) * 1024
        elif line.startswith('usage_usec '):
            row['cpu_usage_usec'] = int(line.split()[1])
        elif line.isdigit():
            row['cgroup_memory_bytes'] = int(line)
    return row


def measured_request(lab, gateway, case, specification, enabled):
    request = request_for('normal', True)
    raw = json.dumps(request).encode()
    lab.save('cases/' + case + '.fixture.json', specification)
    specification_hash = sha((lab.cases / (case + '.fixture.json')).read_bytes())
    (lab.cases / (case + '.request')).write_bytes(raw)
    connection = http.client.HTTPConnection('127.0.0.1', gateway.port, timeout=180)
    started = time.monotonic()
    row = {'case': case, 'request_sha256': sha(raw)}
    pieces, events = [], bytearray()
    try:
        connection.request('POST', '/v1/chat/completions', body=raw, headers={
            'Content-Type': 'application/json', 'X-Test-Case': case, 'X-Delivery': 'events'})
        response = connection.getresponse()
        row['status'] = response.status
        (lab.cases / (case + '.headers')).write_text(json.dumps(response.getheaders()))
        while piece := response.read1(65536):
            pieces.append(piece)
            events.extend(piece)
            while b'\n\n' in events:
                event, _, tail = events.partition(b'\n\n')
                events = bytearray(tail)
                elapsed = time.monotonic() - started
                if b'"name"' in event:
                    row.setdefault('first_metadata_seconds', elapsed)
                if b'"arguments"' in event:
                    row.setdefault('first_arguments_seconds', elapsed)
                    row['last_arguments_seconds'] = elapsed
        body = b''.join(pieces)
        row['complete_response_seconds'] = time.monotonic() - started
        (lab.cases / (case + '.body')).write_bytes(body)
        row.update(body_sha256=sha(body), body_bytes=len(body))
        assert response.status == 200, response.status
        actual = parse(body, True)
        observation = json.loads((lab.cases / (case + '.fixture-observation.json')).read_text())
        assert observation['sha256'] == specification_hash and 'error' not in observation, observation
        expected = keymap(specification)
        if enabled:
            expected = {key: value.replace('MASKED_TOKEN', 'DEMO_SECRET') for key, value in expected.items()}
        assert actual['arguments'] == expected, 'arguments missing, truncated or changed'
        assert actual['names'] == {key: 'lookup' for key in expected}, 'metadata differs'
        assert actual['finish'] == [(0, 'tool_calls')] and actual['done'] == 1, 'termination differs'
        assert actual['usage'] == [{'total_tokens': 20}], 'usage differs'
        assert actual['text'] == {('0', 'content'): 'checking DEMO_SECRET'}, 'content differs'
        upstream = json.loads((lab.cases / (case + '.upstream-request')).read_bytes())
        assert upstream['messages'][0]['content'] == 'MASKED_TOKEN', 'request mask differs'
        row['arguments_sha256'] = {':'.join(key): sha(value.encode()) for key, value in actual['arguments'].items()}
        row['passed'] = True
    except Exception as error:
        row.update(passed=False, error=type(error).__name__ + ': ' + str(error), elapsed_seconds=time.monotonic() - started)
    finally:
        body_path = lab.cases / (case + '.body')
        if not body_path.exists():
            partial = b''.join(pieces)
            body_path.write_bytes(partial)
            row.update(body_sha256=sha(partial), body_bytes=len(partial), partial_response=True)
        connection.close()
    return row


def quantiles(rows, field):
    values = sorted(row[field] for row in rows if field in row)
    return {'n': len(values), 'min': min(values), 'median': statistics.median(values),
        'p95_nearest_rank': values[max(0, (95 * len(values) + 99) // 100 - 1)], 'max': max(values)} if values else None


def main():
    parser = arguments(__doc__)
    parser.add_argument('--smoke', action='store_true')
    parser.add_argument('--only-group', help='One named group for diagnosis; not a complete capacity run')
    args = parser.parse_args()
    groups = []
    with Lab(args, 'capacity') as lab:
        lab.manifest.update(only_group=args.only_group, measurement={
            'client': 'Python http.client; one connection per request; no retries',
            'latency': 'Client-observed complete SSE events; first metadata, first and last argument event, HTTP completion',
            'rss': '/proc/1/status VmRSS sampled about every 0.5s; sampled peak, not an instantaneous maximum',
            'cpu': 'Gateway container cgroup cpu.stat usage_usec delta; fixture/client excluded',
            'wasm_memory': 'Record all host Wasm stats; report unavailable if no linear-memory gauge is exposed',
            'isolation': 'New gateway per payload/concurrency/variant; four warm-up requests before measurements',
            'formal_requests_per_group': 4 if args.smoke else 32,
            'limitations': ['Single amd64 worker on Docker Desktop; results do not establish production capacity.',
                'Fixture, client and gateway share this development machine.', 'Latency includes fixture and network time.']})
        lab.save('manifest.json', lab.manifest)
        for label, size, count in sizes(args.smoke):
            prefix, suffix = '{"email":"MASKED_TOKEN","padding":"', '"}'
            argument = prefix + 'x' * (size - len(prefix + suffix)) + suffix
            assert len(argument.encode()) == size
            specification = {'calls': [{'name': 'lookup', 'id': f'call-{index}', 'index': index, 'arguments': argument} for index in range(count)]}
            for concurrency in ((1,) if args.smoke else (1, 8, 32)):
                for variant in ('baseline', 'candidate-off', 'candidate-on'):
                    name = f'{label}-c{concurrency}-{variant}'
                    if args.only_group and name != args.only_group:
                        continue
                    settings = deepcopy(BASE_CONFIG)
                    if variant == 'candidate-on':
                        settings['tool_arguments'] = {'check': True, 'restore': True, 'restore_allowlist': [{'tool': 'lookup', 'paths': ['/email']}]}
                    gateway = lab.start(name, settings, 'baseline.wasm' if variant == 'baseline' else 'candidate.wasm')
                    try:
                        warmups = []
                        for repeat in range(4):
                            case = name + f'-warm-{repeat}'
                            warm = measured_request(lab, gateway, case, specification, variant == 'candidate-on')
                            warmups.append(warm)
                            lab.save(name + '.warmups.json', warmups)
                            assert warm['passed'], warm
                            compress_case(lab, case)
                        samples, stop = [], threading.Event()

                        def sample():
                            while not stop.is_set():
                                samples.append(probe(gateway.container))
                                stop.wait(.5)

                        before = probe(gateway.container)
                        sampler = threading.Thread(target=sample)
                        sampler.start()
                        started = time.monotonic()
                        try:
                            with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as executor:
                                futures = [executor.submit(measured_request, lab, gateway, name + f'-run-{repeat}', specification, variant == 'candidate-on') for repeat in range(4 if args.smoke else 32)]
                                rows = [future.result() for future in futures]
                        finally:
                            stop.set()
                            sampler.join()
                        elapsed = time.monotonic() - started
                        after = probe(gateway.container)
                        samples = [before, *samples, after]
                        group = {'group': name, 'payload_bytes_per_call': size, 'calls': count, 'concurrency': concurrency,
                            'variant': variant, 'warmup_requests': 4, 'formal_requests': len(rows),
                            'passed': sum(row['passed'] for row in rows), 'failed': sum(not row['passed'] for row in rows),
                            'elapsed_seconds': elapsed, 'sampled_peak_rss_bytes': max(row.get('rss_bytes', 0) for row in samples),
                            'sampled_peak_cgroup_memory_bytes': max(row.get('cgroup_memory_bytes', 0) for row in samples),
                            'cpu_usage_usec_delta': after.get('cpu_usage_usec', 0) - before.get('cpu_usage_usec', 0),
                            'latency': {field: quantiles(rows, field) for field in ('first_metadata_seconds', 'first_arguments_seconds', 'last_arguments_seconds', 'complete_response_seconds')}}
                        lab.save(name + '.results.json', rows)
                        lab.save(name + '.samples.json', samples)
                        stats = lab.command(['docker', 'exec', gateway.container, 'curl', '-sf', 'http://127.0.0.1:15000/stats?filter=wasm']).stdout
                        (lab.output / (name + '.wasm-stats')).write_text(stats)
                        for line in stats.splitlines():
                            if any(field in line for field in ('crash_total', 'worker_fail_open_skip_total', 'worker_uninitialized')):
                                assert line.rsplit(': ', 1)[-1] == '0', line
                        group['wasm_memory_gauges'] = [line for line in stats.splitlines() if 'memory' in line.lower()]
                        group['wasm_memory_gauge_available'] = bool(group['wasm_memory_gauges'])
                        groups.append(group)
                        lab.save('results.json', groups)
                        for row in rows:
                            compress_case(lab, row['case'])
                        assert not group['failed'], group
                        print(json.dumps(group), flush=True)
                    finally:
                        lab.stop(gateway)
        lab.save('summary.json', {'groups': len(groups), 'formal_requests': sum(group['formal_requests'] for group in groups),
            'passed': sum(group['passed'] for group in groups), 'failed': sum(group['failed'] for group in groups),
            'full_t5_complete': False, 'production_capacity_established': False, 'models': 0})


if __name__ == '__main__':
    main()
