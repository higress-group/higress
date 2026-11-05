"""Run pinned gateway comparisons against a synthetic upstream.

Wasm modules must already be built; manifests record their exact hashes.
This functional run does not claim capacity or live-model coverage.
"""
import argparse
import hashlib
import json
import shutil
import socket
import subprocess
import time
import traceback
from pathlib import Path

GATEWAY = 'higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway@sha256:a82de493902012ce6b555ecb2244156f3c73824474d67549144a7efa334d0299'
BUILDER = 'higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/wasm-rust-builder@sha256:31758209b0ab11bb81db4c07789d1b93072c46f63ecb10f1ee88863baf659d90'
BASE_CONFIG = {'deny_openai': True, 'deny_words': ['BLOCKED'], 'replace_roles': [
    {'regex': 'DEMO_SECRET', 'type': 'replace', 'value': 'MASKED_TOKEN', 'restore': True}]}
SOURCE = Path(__file__).resolve().parent


def digest(data):
    return hashlib.sha256(data).hexdigest()


def configurations():
    old = BASE_CONFIG
    allow = [{'tool': 'lookup', 'paths': ['/email']}]
    return {
        'bypass': None,
        'baseline': old,
        'candidate-off': old,
        'candidate-false': {**old, 'tool_arguments': {'check': False, 'restore': False}, 'tool_definitions': {'check': False}},
        'candidate-false-limits': {**old, 'tool_arguments': {'check': False, 'restore': False, 'limits': {'max_call_bytes': 1, 'max_buffer_bytes': 1, 'max_calls': 1}}, 'tool_definitions': {'check': False}},
        'candidate-on': {**old, 'tool_arguments': {'check': True, 'restore': True, 'restore_allowlist': allow}, 'tool_definitions': {'check': True}},
        'candidate-restore-only': {**old, 'tool_arguments': {'restore': True, 'restore_allowlist': allow}},
    }


def envoy_config(network, module, configuration):
    filters = []
    if module:
        filters.append({'name': 'envoy.filters.http.wasm', 'typed_config': {
            '@type': 'type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm',
            'config': {'name': 'ai-data-masking', 'fail_open': False,
                       'configuration': {'@type': 'type.googleapis.com/google.protobuf.StringValue', 'value': json.dumps(configuration)},
                       'vm_config': {'runtime': 'envoy.wasm.runtime.v8', 'code': {'local': {'filename': '/lab/' + module}}}}}})
    filters.append({'name': 'envoy.filters.http.router', 'typed_config': {'@type': 'type.googleapis.com/envoy.extensions.filters.http.router.v3.Router'}})
    manager = {'@type': 'type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager',
               'stat_prefix': 'fixture', 'route_config': {'name': 'fixture', 'virtual_hosts': [{'name': 'fixture', 'domains': ['*'], 'routes': [{'match': {'prefix': '/'}, 'route': {'cluster': 'fixture', 'timeout': '120s'}}]}]},
               'http_filters': filters,
               'access_log': [{'name': 'envoy.access_loggers.file', 'typed_config': {'@type': 'type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog', 'path': '/dev/stdout', 'log_format': {'json_format': {'case': '%REQ(X-TEST-CASE)%', 'status': '%RESPONSE_CODE%', 'bytes': '%BYTES_SENT%', 'flags': '%RESPONSE_FLAGS%', 'duration_ms': '%DURATION%'}}}}]}
    return {'static_resources': {'listeners': [{'name': 'fixture', 'per_connection_buffer_limit_bytes': 33554432, 'address': {'socket_address': {'address': '0.0.0.0', 'port_value': 10000}}, 'filter_chains': [{'filters': [{'name': 'envoy.filters.network.http_connection_manager', 'typed_config': manager}]}]}],
        'clusters': [{'name': 'fixture', 'type': 'STRICT_DNS', 'connect_timeout': '3s', 'load_assignment': {'cluster_name': 'fixture', 'endpoints': [{'lb_endpoints': [{'endpoint': {'address': {'socket_address': {'address': network + '-fixture', 'port_value': 18080}}}}]}]}}]},
        'admin': {'access_log_path': '/tmp/admin.log', 'address': {'socket_address': {'address': '127.0.0.1', 'port_value': 15000}}}}


def parse(raw, stream):
    events, done = [], 0
    if stream:
        for event in raw.replace(b'\r\n', b'\n').replace(b'\r', b'\n').split(b'\n\n'):
            data = b'\n'.join(line[5:].lstrip(b' ') for line in event.splitlines() if line.startswith(b'data:'))
            if data.strip() == b'[DONE]':
                done += 1
            elif data:
                try:
                    events.append(json.loads(data))
                except json.JSONDecodeError:
                    events.append({'unparsed': data.decode()})
    else:
        events = [json.loads(raw)]
    arguments, text, finish, names, usage, errors = {}, [], [], {}, [], []
    for event in events:
        if 'usage' in event:
            usage.append(event['usage'])
        if event.get('error'):
            errors.append(event['error'])
        for choice in event.get('choices') or []:
            index = choice.get('index')
            if choice.get('finish_reason') is not None:
                finish.append((index, choice['finish_reason']))
            delta = choice.get('delta', choice.get('message')) or {}
            for field in ('content', 'reasoning_content'):
                if isinstance(delta.get(field), str):
                    text.append((index, field, delta[field]))
            for ordinal, call in enumerate(delta.get('tool_calls') or []):
                call_index = call.get('index', ordinal)
                if call_index is None:
                    call_index = ordinal
                key = (str(index), 'modern', str(int(call_index)))
                function = call.get('function') or {}
                names[key] = names.get(key, '') + (function.get('name') or '')
                if isinstance(function.get('arguments'), str):
                    arguments[key] = arguments.get(key, '') + function['arguments']
            if delta.get('function_call'):
                key = (str(index), 'legacy', '0')
                function = delta['function_call']
                names[key] = names.get(key, '') + (function.get('name') or '')
                if isinstance(function.get('arguments'), str):
                    arguments[key] = arguments.get(key, '') + function['arguments']
    joined = {}
    for index, field, fragment in text:
        joined[(str(index), field)] = joined.get((str(index), field), '') + fragment
    return {'events': events, 'arguments': arguments, 'text': joined, 'finish': finish, 'names': names, 'usage': usage, 'errors': errors, 'done': done}


def request_for(scenario, stream):
    request = {'model': 'public-fixture', 'stream': stream, 'messages': [{'role': 'user', 'content': 'DEMO_SECRET'}], 'tools': [{'type': 'function', 'function': {'name': 'lookup', 'description': 'public fixture', 'parameters': {'type': 'object', 'properties': {'email': {'type': 'string'}}}}}]}
    if scenario == 'definition-deny':
        request['tools'][0]['function']['description'] = 'BLOCKED'
    if scenario == 'request-arguments':
        request['messages'].append({'role': 'assistant', 'content': None, 'tool_calls': [{'function': {'name': 'lookup', 'arguments': '{ "email":"DEMO_SECRET", "n":1e2 }'}}]})
    return request


def check(variant, scenario, stream, configuration, request, upstream, wire, body):
    enabled = variant in ('candidate-on', 'candidate-restore-only')
    request_denied = variant == 'candidate-on' and scenario == 'definition-deny'
    if request_denied:
        assert upstream is None, 'denied request reached upstream'
        assert '已被屏蔽' in body.decode(), 'missing configured refusal body'
        return {'request_denied': True}
    assert upstream is not None, 'upstream request missing'
    upstream_value = json.loads(upstream)
    original = parse(wire, stream if scenario != 'json-error' else False)
    if scenario == 'json-error':
        assert json.loads(body) == json.loads(wire)
        return {'error_preserved': True}
    response_denied = enabled and scenario in ('malformed-arguments', 'missing-index', 'bad-choice', 'bad-sse')
    response_denied |= variant == 'candidate-on' and scenario in ('escaped-deny', 'deny-value', 'missing-index', 'bad-choice', 'bad-sse', 'nonallowed-malformed')
    if response_denied:
        if stream:
            observed = parse(body, True)
            assert not observed['arguments'] and observed['done'] == 0, 'rejected stream released parameters or DONE'
        else:
            assert '已被屏蔽' in body.decode()
        return {'response_denied': True}
    actual = parse(body, stream)
    assert actual['usage'] == original['usage'], 'usage changed or duplicated'
    assert actual['finish'] == original['finish'], 'finish changed'
    assert actual['done'] == original['done'], 'DONE changed'
    assert actual['errors'] == original['errors'], 'upstream error changed'
    assert actual['names'] == original['names'], 'tool names changed'
    expected_text = original['text']
    legacy_map = 'MASKED_TOKEN' in upstream_value['messages'][0]['content']
    if legacy_map:
        expected_text = {key: value.replace('MASKED_TOKEN', 'DEMO_SECRET') for key, value in expected_text.items()}
    assert actual['text'] == expected_text, 'ordinary content scope changed'
    expected = dict(original['arguments'])
    tool_map = scenario == 'request-arguments' and variant == 'candidate-on'
    if enabled and (legacy_map or tool_map):
        for key, value in expected.items():
            if original['names'].get(key) == 'lookup' and scenario != 'root-string':
                expected[key] = value.replace('"email":"MASKED_TOKEN"', '"email":"DEMO_SECRET"')
    if enabled and scenario == 'upstream-error':
        expected = {}
    assert actual['arguments'] == expected, 'tool arguments differ from expected scope'
    if variant == 'candidate-on' and scenario == 'request-arguments':
        args = upstream_value['messages'][1]['tool_calls'][0]['function']['arguments']
        assert 'DEMO_SECRET' not in args and 'MASKED_TOKEN' in args, 'request arguments were not masked'
    if scenario=='controls':
        for field in (b': keepalive',b'event: message',b'id: public',b'retry: 1000'):
            assert body.count(field)==wire.count(field), 'SSE control field lost'
    if variant == 'bypass':
        assert body == wire, 'bypass body changed'
    return {'arguments_checked': len(expected), 'legacy_map': legacy_map, 'tool_map': tool_map}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--baseline-wasm', type=Path, required=True)
    parser.add_argument('--candidate-wasm', type=Path, required=True)
    parser.add_argument('--baseline-source', required=True)
    parser.add_argument('--candidate-source', required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--resource-prefix', required=True)
    parser.add_argument('--smoke', action='store_true')
    parser.add_argument('--variants', nargs='+', choices=list(configurations()))
    args = parser.parse_args()
    if args.variants and any(name in args.variants for name in ('candidate-off', 'candidate-false', 'candidate-false-limits')) and 'baseline' not in args.variants:
        parser.error('Disabled variants require baseline in the same run for exact comparison')
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    evidence = output / 'cases'
    evidence.mkdir()
    network = args.resource_prefix
    commands, containers, all_containers, records, ports = [], [], [], [], []
    network_created = False

    def run(command, check=True):
        started = time.monotonic()
        result = subprocess.run(command, capture_output=True, text=True)
        commands.append({'command': command, 'exit_code': result.returncode, 'elapsed_seconds': time.monotonic() - started})
        (output / 'commands.json').write_text(json.dumps(commands, indent=2) + '\n')
        if check and result.returncode:
            raise RuntimeError(result.stdout + result.stderr)
        return result.stdout.strip()

    for variant, path in [('baseline', args.baseline_wasm), ('candidate', args.candidate_wasm)]:
        shutil.copyfile(path, output / (variant + '.wasm'))
    manifest = {'gateway': GATEWAY, 'fixture_image': BUILDER, 'baseline_source': args.baseline_source, 'candidate_source': args.candidate_source,
                'baseline_wasm_sha256': digest(args.baseline_wasm.read_bytes()), 'candidate_wasm_sha256': digest(args.candidate_wasm.read_bytes()),
                'fixture_sha256': digest((SOURCE / 'fixture.py').read_bytes()), 'runner_sha256': digest(Path(__file__).read_bytes()),
                'concurrency': 1, 'workers': 1, 'model_requests': 0, 'tools_executed': 0, 'selected_variants': args.variants or list(configurations()),
                'limitations': ['Synthetic upstream; not live-model or capacity evidence.', 'Upstream write boundaries do not imply Envoy callback boundaries.']}
    (output / 'source').mkdir()
    shutil.copy2(SOURCE / 'fixture.py', output / 'source/fixture.py')
    shutil.copy2(Path(__file__), output / 'source/verify_runtime.py')
    (output / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    try:
        run(['docker', 'network', 'create', network])
        network_created = True
        fixture = run(['docker', 'run', '-d', '--platform', 'linux/amd64', '--name', network + '-fixture', '--network', network,
            '-v', str(output / 'source') + ':/scripts:ro', '-v', str(evidence) + ':/evidence', '--entrypoint', 'python3', BUILDER, '/scripts/fixture.py', '--evidence', '/evidence'])
        containers.append(fixture)
        all_containers.append(fixture)
        scenarios = ['normal', 'string-index', 'id-only', 'legacy', 'empty-finish', 'name-suffix', 'null-arguments', 'empty-arguments', 'root-string', 'no-finish', 'http-end', 'controls', 'upstream-error', 'json-error', 'request-arguments', 'definition-deny', 'malformed-arguments', 'escaped-deny', 'deny-value', 'missing-index', 'bad-choice', 'bad-sse', 'nonallowed-malformed']
        if args.smoke:
            scenarios = ['normal', 'legacy', 'definition-deny', 'escaped-deny', 'request-arguments']
        configurations_by_name = configurations()
        for variant, configuration in configurations_by_name.items():
            if args.variants and variant not in args.variants:
                continue
            module = None if configuration is None else 'baseline.wasm' if variant == 'baseline' else 'candidate.wasm'
            config = envoy_config(network, module, configuration)
            (output / (variant + '.json')).write_text(json.dumps(config, indent=2) + '\n')
            container = run(['docker', 'run', '-d', '--platform', 'linux/amd64', '--name', network + '-' + variant, '--network', network,
                '-v', str(output) + ':/lab:ro', '-p', '127.0.0.1::10000', '--entrypoint', '/usr/local/bin/envoy', GATEWAY,
                '-c', '/lab/' + variant + '.json', '--concurrency', '1', '--log-level', 'info'])
            containers.append(container)
            all_containers.append(container)
            port = run(['docker', 'port', container, '10000/tcp']).rsplit(':', 1)[1]
            ports.append(int(port))
            for attempt in range(150):
                ready = subprocess.run(['docker', 'exec', container, 'curl', '-sf', 'http://127.0.0.1:15000/ready'], capture_output=True)
                if ready.returncode == 0:
                    break
                time.sleep(.2)
            else:
                raise RuntimeError('gateway not ready: ' + variant)
            (output / (variant + '.version')).write_text(run(['docker', 'exec', container, '/usr/local/bin/envoy', '--version']) + '\n')
            for scenario in scenarios:
                for stream in ([True, False] if scenario in ('normal', 'legacy', 'json-error', 'malformed-arguments', 'deny-value') else [True]):
                    for delivery in (['single'] if args.smoke else ['single', 'events', 'fragment17', 'prefix1']):
                        for repeat in (range(1) if args.smoke else range(2)):
                            case = f'{variant}-{scenario}-{int(stream)}-{delivery}-{repeat}'
                            request = request_for(scenario, stream)
                            (evidence / (case + '.request.json')).write_text(json.dumps(request) + '\n')
                            command = ['curl', '-sS', '-N', '--connect-timeout', '3', '--max-time', '60', 'http://127.0.0.1:' + port + '/v1/chat/completions',
                                '-H', 'Content-Type: application/json', '-H', 'X-Test-Case: ' + case, '-H', 'X-Scenario: ' + scenario, '-H', 'X-Delivery: ' + delivery,
                                '--data-binary', '@' + str(evidence / (case + '.request.json')), '-D', str(evidence / (case + '.headers')), '-o', str(evidence / (case + '.body')), '-w', '%{http_code}']
                            status = run(command)
                            assert status == '200', (case, status)
                            upstream_path = evidence / (case + '.upstream-request')
                            upstream = upstream_path.read_bytes() if upstream_path.exists() else None
                            wire_path = evidence / (case + '.upstream-response')
                            wire = wire_path.read_bytes() if wire_path.exists() else b''
                            body = (evidence / (case + '.body')).read_bytes()
                            result = {'case': case, 'variant': variant, 'scenario': scenario, 'stream': stream, 'delivery': delivery, 'repeat': repeat, 'status': status,
                                      'request_sha256': digest((evidence / (case + '.request.json')).read_bytes()), 'upstream_request_sha256': digest(upstream) if upstream is not None else None,
                                      'upstream_response_sha256': digest(wire), 'client_body_sha256': digest(body)}
                            try:
                                result.update(check(variant, scenario, stream, configuration, request, upstream, wire, body))
                                result['passed'] = True
                            except Exception as error:
                                result.update(passed=False, error=str(error))
                                records.append(result)
                                (output / 'results.json').write_text(json.dumps(records, indent=2) + '\n')
                                raise AssertionError(case + ': ' + str(error)) from error
                            records.append(result)
                            (output / 'results.json').write_text(json.dumps(records, indent=2) + '\n')
            stats=run(['docker','exec',container,'curl','-sf','http://127.0.0.1:15000/stats?filter=wasm'])
            (output / (variant + '.wasm-stats')).write_text(stats+'\n')
            for line in stats.splitlines():
                if any(name in line for name in ('crash_total','worker_fail_open_skip_total','worker_uninitialized')):
                    assert line.rsplit(': ',1)[-1]=='0',line
            (output / (variant + '.config-dump.json')).write_text(run(['docker', 'exec', container, 'curl', '-sf', 'http://127.0.0.1:15000/config_dump']))
            logs=subprocess.run(['docker','logs',container],capture_output=True,text=True)
            (output / (variant + '.log')).write_text(logs.stdout+logs.stderr)
            run(['docker', 'rm', '-f', container])
            containers.remove(container)
            print(json.dumps({'variant': variant, 'passed_cases': sum(record['variant'] == variant for record in records)}), flush=True)
        for candidate in ('candidate-off', 'candidate-false', 'candidate-false-limits'):
            baseline = {(record['scenario'], record['stream'], record['delivery'], record['repeat']): record for record in records if record['variant'] == 'baseline'}
            for record in records:
                if record['variant'] == candidate:
                    key = record['scenario'], record['stream'], record['delivery'], record['repeat']
                    assert record['client_body_sha256'] == baseline[key]['client_body_sha256'], 'disabled output differs: ' + record['case']
        (output / 'summary.json').write_text(json.dumps({'passed': len(records), 'failed': 0, 'models': 0, 'full_plan_complete': False}, indent=2) + '\n')
    except Exception:
        (output / 'failure.txt').write_text(traceback.format_exc())
        raise
    finally:
        for container in containers:
            result = subprocess.run(['docker', 'logs', container], capture_output=True, text=True)
            (output / ('cleanup-' + container[:12] + '.log')).write_text(result.stdout + result.stderr)
            run(['docker', 'rm', '-f', container], check=False)
        if network_created:
            run(['docker', 'network', 'rm', network], check=False)
        port_status={}
        for port in ports:
            with socket.socket() as connection:
                connection.settimeout(.5)
                port_status[str(port)]=connection.connect_ex(('127.0.0.1',port))!=0
        (output / 'cleanup.json').write_text(json.dumps({'containers_absent': {container: subprocess.run(['docker','inspect',container],capture_output=True).returncode!=0 for container in all_containers}, 'network_absent': subprocess.run(['docker','network','inspect',network],capture_output=True).returncode!=0, 'ports_not_listening':port_status},indent=2)+'\n')
        files = sorted(path for path in output.rglob('*') if path.is_file() and path.name != 'SHA256SUMS')
        (output / 'SHA256SUMS').write_text(''.join(digest(path.read_bytes()) + '  ' + str(path.relative_to(output)) + '\n' for path in files))


if __name__ == '__main__':
    main()
