"""Small live-model run with a predeclared budget and same-request Envoy Tap.

Supply the SSH tunnel separately. Only synthetic secrets are used. Tools are
declared, never executed. Keep raw output private until it has been reviewed.
"""
import base64
import datetime
import json
import subprocess
import time
import urllib.error
import urllib.request
from copy import deepcopy

from runtime_lab import Lab, arguments, sha
from verify_runtime import BASE_CONFIG, parse

TAP_LIMIT = 1048576


def install_tap(config, live):
    if live:
        address = config['static_resources']['clusters'][0]['load_assignment']['endpoints'][0]['lb_endpoints'][0]['endpoint']['address']['socket_address']
        address.update(address='host.docker.internal', port_value=20537)
    manager = config['static_resources']['listeners'][0]['filter_chains'][0]['filters'][0]['typed_config']
    manager['http_filters'].insert(-1, {'name': 'envoy.filters.http.tap', 'typed_config': {
        '@type': 'type.googleapis.com/envoy.extensions.filters.http.tap.v3.Tap',
        'common_config': {'static_config': {'match_config': {'any_match': True}, 'output_config': {
            'sinks': [{'format': 'JSON_BODY_AS_BYTES', 'file_per_tap': {'path_prefix': '/tap/trace'}}],
            'max_buffered_rx_bytes': TAP_LIMIT, 'max_buffered_tx_bytes': TAP_LIMIT, 'streaming': False}}}}})


def body_from_tap(value):
    assert not value.get('truncated', False), 'Tap body truncated'
    encoded = value.get('as_bytes', value.get('asBytes'))
    body = base64.b64decode(encoded) if encoded is not None else value.get('as_string', value.get('asString', '')).encode()
    assert len(body) < TAP_LIMIT, 'Capture at Tap limit; completeness not established'
    return body


def trace_after(directory, previous):
    for _ in range(100):
        paths = [path for path in directory.glob('trace*') if path not in previous]
        if paths:
            assert len(paths) == 1, 'Expected exactly one same-request Tap trace'
            try:
                trace = json.loads(paths[0].read_text())
                return trace.get('http_buffered_trace', trace.get('httpBufferedTrace'))
            except json.JSONDecodeError:
                pass
        time.sleep(.1)
    raise RuntimeError('Same-request Tap trace missing')


def request_for(model, stream, name='lookup', history=False):
    messages = [{'role': 'user', 'content': f'这是工具协议测试。请调用 {name}，把测试标签 DEMO_SECRET 逐字填入 email 和 other 两个字段，不要更改字符。'}]
    if history:
        messages = [
            {'role': 'assistant', 'content': None, 'tool_calls': [{'id': 'history-call', 'type': 'function', 'function': {'name': name, 'arguments': '{"email":"DEMO_SECRET","other":"DEMO_SECRET"}'}}]},
            {'role': 'tool', 'tool_call_id': 'history-call', 'content': 'done'},
            {'role': 'user', 'content': f'这是工具协议测试。请再次调用 {name}，email 和 other 与刚才的参数完全相同。'}]
    value = {'model': model, 'stream': stream, 'messages': messages, 'tools': [{'type': 'function', 'function': {
        'name': name, 'description': 'Echo two synthetic test values; this tool will not be executed.',
        'parameters': {'type': 'object', 'properties': {'email': {'type': 'string'}, 'other': {'type': 'string'}}, 'required': ['email', 'other']}}}],
        'tool_choice': {'type': 'function', 'function': {'name': name}}, 'max_tokens': 256, 'temperature': 0,
        'chat_template_kwargs': {'enable_thinking': False}}
    if stream:
        value['stream_options'] = {'include_usage': True}
    return value


def settings(mode):
    value = deepcopy(BASE_CONFIG)
    if mode.startswith('restore') or mode == 'check-restore':
        value['tool_arguments'] = {'restore': True, 'restore_allowlist': [{'tool': 'lookup', 'paths': ['/absent' if mode == 'restore-other-path' else '/email']}]}
    if mode.startswith('check'):
        value.setdefault('tool_arguments', {})['check'] = True
    return value


def compare(request, upstream_request, upstream, downstream, mode, stream):
    expected_request = deepcopy(request)
    if mode not in ('direct', 'bypass'):
        for message in expected_request['messages']:
            if isinstance(message.get('content'), str):
                message['content'] = message['content'].replace('DEMO_SECRET', 'MASKED_TOKEN')
            if mode.startswith('check'):
                for call in message.get('tool_calls', []):
                    call['function']['arguments'] = call['function']['arguments'].replace('DEMO_SECRET', 'MASKED_TOKEN')
    assert upstream_request == expected_request, 'Unexpected upstream request rewrite'
    original, actual = parse(upstream, stream), parse(downstream, stream)
    assert not any('unparsed' in event for event in original['events'] + actual['events']), 'Malformed protocol output'
    assert actual['names'] == original['names'], 'Tool name changed'
    assert actual['finish'] == original['finish'] and actual['done'] == original['done'], 'Termination changed'
    assert actual['usage'] == original['usage'] and actual['errors'] == original['errors'], 'Usage/error changed'
    expected_arguments, observed_targets, echo_fields = {}, 0, set()
    target_name = request['tool_choice']['function']['name']
    target_value = 'DEMO_SECRET' if mode in ('direct', 'bypass') else 'MASKED_TOKEN'
    for key, argument in original['arguments'].items():
        changed = argument
        value = json.loads(argument)
        for path in ('email', 'other'):
            if isinstance(value, dict) and value.get(path) == 'MASKED_TOKEN':
                observed_targets += 1
            if original['names'].get(key) == target_name and isinstance(value, dict) and value.get(path) == target_value:
                echo_fields.add(path)
        if mode in ('restore', 'check-restore') and original['names'].get(key) == 'lookup' and isinstance(value, dict) and value.get('email') == 'MASKED_TOKEN':
            # Compare parsed values here: live providers choose their own JSON
            # spacing. Byte preservation is checked by deterministic suites.
            expected_value = {**value, 'email': 'DEMO_SECRET'}
            assert json.loads(actual['arguments'][key]) == expected_value, 'Allowlisted value was not restored in isolation'
            expected_arguments[key] = actual['arguments'][key]
        else:
            expected_arguments[key] = changed
    assert actual['arguments'] == expected_arguments, 'Tool arguments changed outside authorization'
    legacy_map_exists = mode not in ('direct', 'bypass') and any(isinstance(message.get('content'), str) and 'DEMO_SECRET' in message['content'] for message in request['messages'])
    expected_text = {key: text.replace('MASKED_TOKEN', 'DEMO_SECRET') for key, text in original['text'].items()} if legacy_map_exists else original['text']
    assert actual['text'] == expected_text, 'Legacy content scope changed'
    if mode == 'bypass':
        assert upstream == downstream, 'Bypass changed response bytes'
    covered = bool(original['arguments']) and not original['errors'] and {'email', 'other'} <= echo_fields
    # The approved completion contract accepts every present, non-null reason,
    # including stop and the empty string. Target fields still must be observed.
    covered = covered and bool(original['finish']) and (not stream or original['done'] == 1)
    return {'coverage': 'covered' if covered else 'uncovered', 'placeholder_fields_observed': observed_targets,
        'requested_echo_fields_observed': sorted(echo_fields),
        'upstream_calls': len(original['arguments']), 'downstream_calls': len(actual['arguments']),
        'done': actual['done'], 'finish': actual['finish'], 'upstream_errors': original['errors']}


def main():
    parser = arguments(__doc__)
    parser.add_argument('--model', default='dsv4')
    parser.add_argument('--fixture', action='store_true', help='Tap harness check only; zero live requests')
    parser.add_argument('--fixture-finish-reason', default='tool_calls')
    args = parser.parse_args()
    ledger, results = [], []
    plan = [('direct', True), ('bypass', True), ('bypass', False), ('baseline', True), ('baseline', False),
        ('off', True), ('off', False), ('restore', True), ('restore', False), ('restore-other-tool', True),
        ('restore-other-path', True), ('check', True), ('check', False), ('check-restore', True)]
    if args.fixture:
        plan = [item for item in plan if item[0] != 'direct']
    with Lab(args, 'live-tap-fixture' if args.fixture else 'live-model') as lab:
        if not args.fixture:
            lab.output.chmod(0o700)
        lab.manifest.update(model='public-fixture' if args.fixture else args.model, model_requests=0,
            model_budget={'health_calls': 0 if args.fixture else 1, 'health_max_tokens': 4,
                'formal_calls': 0 if args.fixture else len(plan), 'formal_max_tokens_per_call': 256,
                'maximum_requested_tokens': 0 if args.fixture else 4 + len(plan) * 256, 'automatic_retries': 0},
            planned_cases=plan, fixture_finish_reason=args.fixture_finish_reason if args.fixture else None, tools_executed=0, tap_capture_limit_bytes=TAP_LIMIT,
            limitations=['Live targets absent from model output are uncovered, not passed.',
                'Different generations are not byte-level A/B comparisons.', 'Raw output requires review before publication.'])
        lab.save('manifest.json', lab.manifest)

        def dispatch(case, body, url, category):
            index = len(ledger)
            ledger.append({'case': case, 'category': category, 'state': 'dispatching',
                'utc': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'max_tokens': body.get('max_tokens'),
                'live_model_request': not args.fixture})
            lab.save('ledger.json', ledger)
            lab.manifest['model_requests'] = sum(row['live_model_request'] for row in ledger)
            lab.save('manifest.json', lab.manifest)
            raw = json.dumps(body, ensure_ascii=False).encode()
            (lab.cases / (case + '.request')).write_bytes(raw)
            result = lab.command(['curl', '-sS', '-N', '--connect-timeout', '5', '--max-time', '120', url + '/v1/chat/completions',
                '-H', 'Content-Type: application/json', '-H', 'X-Test-Case: ' + case,
                '--data-binary', '@' + str(lab.cases / (case + '.request')),
                '-D', str(lab.cases / (case + '.headers')), '-o', str(lab.cases / (case + '.body')), '-w', '%{http_code}'], check=False)
            ledger[index].update(state='returned', curl_exit=result.returncode, http_status=result.stdout.strip())
            lab.save('ledger.json', ledger)
            assert result.returncode == 0 and result.stdout.strip() == '200', {'case': case, 'status': result.stdout, 'curl_exit': result.returncode}
            return (lab.cases / (case + '.body')).read_bytes()

        if not args.fixture:
            with urllib.request.urlopen('http://127.0.0.1:20537/v1/models', timeout=8) as response:
                models_raw = response.read()
            (lab.output / 'models.json').write_bytes(models_raw)
            models = [item['id'] for item in json.loads(models_raw).get('data', [])]
            lab.save('listing.json', {'generation_calls': 0, 'listed_models': models, 'selected_model': args.model})
            assert args.model in models, 'Approved model unavailable; stop before generation'
            health = dispatch('health', {'model': args.model, 'messages': [{'role': 'user', 'content': 'Reply OK.'}],
                'max_tokens': 4, 'stream': False, 'chat_template_kwargs': {'enable_thinking': False}}, 'http://127.0.0.1:20537', 'health')
            health_value = json.loads(health)
            assert not health_value.get('error') and isinstance(health_value.get('choices'), list) and health_value['choices'], 'Model health did not return a completion'
        for ordinal, (mode, stream) in enumerate(plan):
            case = f'{ordinal}-{mode}-{int(stream)}'
            request = request_for('public-fixture' if args.fixture else args.model, stream,
                'external' if mode == 'restore-other-tool' else 'lookup', mode.startswith('check'))
            if mode == 'direct':
                body = dispatch(case, request, 'http://127.0.0.1:20537', 'formal')
                row = compare(request, request, body, body, mode, stream)
                results.append({'case': case, 'mode': mode, **row,
                    'same_request_tap': False, 'body_sha256': sha(body)})
            else:
                tap = lab.output / ('tap-' + case)
                tap.mkdir()
                gateway = lab.start('g-' + case, settings(mode), None if mode == 'bypass' else ('baseline.wasm' if mode == 'baseline' else 'candidate.wasm'),
                    modify=lambda config: install_tap(config, not args.fixture), extra_mounts=[str(tap) + ':/tap'])
                try:
                    if args.fixture:
                        token = 'DEMO_SECRET' if mode == 'bypass' else 'MASKED_TOKEN'
                        lab.save('cases/' + case + '.fixture.json', {'calls': [{'name': request['tool_choice']['function']['name'], 'arguments': json.dumps({'email': token, 'other': token})}], 'content': 'checking ' + token, 'finish_reason': args.fixture_finish_reason})
                    previous = set(tap.glob('trace*'))
                    body = dispatch(case, request, 'http://127.0.0.1:' + str(gateway.port), 'formal')
                    trace = trace_after(tap, previous)
                    upstream_request = body_from_tap(trace['request']['body'])
                    upstream_response = body_from_tap(trace['response']['body'])
                    (lab.cases / (case + '.tap-upstream-request')).write_bytes(upstream_request)
                    (lab.cases / (case + '.tap-upstream-response')).write_bytes(upstream_response)
                    row = compare(request, json.loads(upstream_request), upstream_response, body, mode, stream)
                    results.append({'case': case, 'mode': mode, 'stream': stream, **row, 'same_request_tap': True,
                        'request_capture_bytes': len(upstream_request), 'response_capture_bytes': len(upstream_response),
                        'body_sha256': sha(body), 'upstream_body_sha256': sha(upstream_response)})
                finally:
                    lab.stop(gateway)
            lab.save('results.json', results)
            print(json.dumps(results[-1], ensure_ascii=False), flush=True)
        lab.save('summary.json', {'formal_cases': len(results), 'covered': sum(row['coverage'] == 'covered' for row in results),
            'uncovered': sum(row['coverage'] == 'uncovered' for row in results),
            'real_model_generations_dispatched': sum(row['live_model_request'] for row in ledger),
            'health_calls': sum(row['category'] == 'health' for row in ledger), 'tools_executed': 0, 'full_t6_complete': False})


if __name__ == '__main__':
    main()
