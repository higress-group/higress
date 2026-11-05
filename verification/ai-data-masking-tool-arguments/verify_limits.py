"""Gateway limits and scope checks. All data and upstream responses are synthetic."""
import gzip
import json
from copy import deepcopy

from runtime_lab import Lab, arguments, sha
from verify_runtime import BASE_CONFIG, parse, request_for


def argument_of_size(size, key='x', value='x'):
    prefix, suffix = '{"' + key + '":"', '"}'
    assert size >= len(prefix + suffix)
    argument = prefix + value * (size - len(prefix + suffix)) + suffix
    assert len(argument.encode()) == size
    return argument


def config(limits, restore=False):
    value = deepcopy(BASE_CONFIG)
    value['tool_arguments'] = {'check': not restore, 'restore': restore, 'limits': limits}
    if restore:
        value['tool_arguments']['restore_allowlist'] = [{'tool': 'lookup', 'paths': ['/email']}]
    return value


def spec_for(arguments, **options):
    return {'calls': [{'name': 'lookup', 'id': 'call-0', 'index': 0, 'arguments': arguments}], **options}


def keymap(spec, replacements=None):
    values = {}
    for ordinal, call in enumerate(spec.get('calls', [])):
        if not isinstance(call.get('arguments'), str):
            continue
        key = (str(call.get('choice', 0)), 'legacy' if call.get('legacy') else 'modern', '0' if call.get('legacy') else str(call.get('index', ordinal)))
        values[key] = replacements.get(ordinal, call['arguments']) if replacements else call['arguments']
    return values


def compress_case(lab, case):
    for suffix in ('request', 'fixture.json', 'body', 'upstream-request', 'upstream-response'):
        path = lab.cases / (case + '.' + suffix)
        if path.exists() and path.stat().st_size > 262144:
            with path.open('rb') as source, gzip.open(str(path) + '.gz', 'wb') as destination:
                while chunk := source.read(1048576):
                    destination.write(chunk)
            path.unlink()


def main():
    parser = arguments(__doc__)
    parser.add_argument('--smoke', action='store_true')
    args = parser.parse_args()
    records = []
    with Lab(args, 'limits') as lab:
        def group(name, settings, cases, module='candidate.wasm'):
            gateway = lab.start(name, settings, module)
            try:
                for case_number, case_data in enumerate(cases):
                    request = case_data.get('request', request_for('normal', True))
                    specification = case_data.get('spec', spec_for('{}'))
                    expected = case_data.get('expected', 'pass')
                    for stream in case_data.get('streams', (True, False)):
                        request = {**request, 'stream': stream}
                        for delivery in (('single',) if args.smoke else ('single', 'events', 'fragment17', 'prefix1')):
                            for repeat in range(1 if args.smoke else 2):
                                case = f'{name}-{case_number}-{int(stream)}-{delivery}-{repeat}'
                                (lab.cases / (case + '.fixture.json')).write_text(json.dumps(specification, ensure_ascii=False) + '\n')
                                row, body = lab.request(gateway, case, request, delivery=delivery)
                                row.update(expected=expected, stream=stream, repeat=repeat)
                                try:
                                    assert row['curl_exit'] == 0 and row['status'] == '200', row
                                    if expected == 'request-denied':
                                        assert not row['upstream_received'], 'invalid request reached upstream'
                                        assert '已被屏蔽' in body.decode(), 'missing refusal body'
                                    elif expected == 'response-denied':
                                        assert row['upstream_received']
                                        if stream:
                                            actual = parse(body, True)
                                            assert not actual['arguments'] and actual['done'] == 0, 'denied stream leaked parameters/termination'
                                        else:
                                            assert '已被屏蔽' in body.decode()
                                    else:
                                        assert row['upstream_received']
                                        actual = parse(body, stream)
                                        upstream_wire = (lab.cases / (case + '.upstream-response')).read_bytes()
                                        original = parse(upstream_wire, stream)
                                        expected_args = case_data.get('expected_arguments', keymap(specification))
                                        assert actual['arguments'] == expected_args, 'arguments changed or missing'
                                        assert actual['names'] == original['names'], 'tool metadata changed'
                                        assert actual['usage'] == original['usage']
                                        assert actual['finish'] == original['finish'] and actual['done'] == original['done']
                                        assert actual['text'] == {('0', 'content'): case_data.get('expected_text', 'checking DEMO_SECRET')}, 'content scope changed'
                                        upstream = json.loads((lab.cases / (case + '.upstream-request')).read_bytes())
                                        if 'request_argument' in case_data:
                                            value = upstream['messages'][1]['tool_calls'][0]['function']['arguments']
                                            assert value == case_data['request_argument'], 'request argument result differs'
                                        if case_data.get('history'):
                                            assert len(request['messages'][1]['tool_calls']) > 128
                                            assert len(json.dumps(request).encode()) > 4194304
                                            for call in upstream['messages'][1]['tool_calls']:
                                                assert json.loads(call['function']['arguments'])['email'] == 'MASKED_TOKEN'
                                        row['arguments_sha256'] = {':'.join(key): sha(value.encode()) for key, value in actual['arguments'].items()}
                                    row['passed'] = True
                                except Exception as error:
                                    row.update(passed=False, error=str(error))
                                    records.append(row)
                                    lab.save('results.json', records)
                                    raise AssertionError(case + ': ' + str(error)) from error
                                records.append(row)
                                lab.save('results.json', records)
                                compress_case(lab, case)
            finally:
                lab.stop(gateway)
            print(json.dumps({'group': name, 'passed': len(records)}), flush=True)

        for boundary in (64, 1024):
            limits = {'max_call_bytes': boundary, 'max_buffer_bytes': 100000, 'max_calls': 1000}
            cases = []
            for size in (boundary - 1, boundary, boundary + 1):
                argument = argument_of_size(size)
                cases.append({'spec': spec_for(argument), 'expected': 'pass' if size <= boundary else 'response-denied'})
            group(f'input-{boundary}', config(limits), cases)
            cases = []
            for size in (boundary - 1, boundary, boundary + 1):
                request = request_for('request-arguments', True)
                request['messages'][1]['tool_calls'][0]['function']['arguments'] = argument_of_size(size)
                cases.append({'request': request, 'spec': {'calls': []}, 'expected': 'pass' if size <= boundary else 'request-denied', 'expected_text': 'checking MASKED_TOKEN'})
            group(f'request-input-{boundary}', config(limits), cases)
        # Exactly 64 argument bytes + lookup(6) + call-0(6).
        for buffer in (75, 76, 77):
            group(f'total-{buffer}', config({'max_call_bytes': 1024, 'max_buffer_bytes': buffer, 'max_calls': 10}),
                [{'spec': spec_for(argument_of_size(64)), 'expected': 'pass' if buffer >= 76 else 'response-denied'}])
        for maximum in (2, 4):
            cases = []
            for count in (maximum - 1, maximum, maximum + 1):
                calls = [{'name': 'lookup', 'index': index, 'id': 'call-' + str(index), 'arguments': '{}'} for index in range(count)]
                cases.append({'spec': {'calls': calls}, 'expected': 'pass' if count <= maximum else 'response-denied', 'streams': (True,)})
            group(f'count-{maximum}', config({'max_call_bytes': 1024, 'max_buffer_bytes': 100000, 'max_calls': maximum}), cases)
        # Restore expands a four-byte placeholder to 51/52/53 bytes; the
        # containing JSON adds 12 bytes. Request history is restore-only.
        for size in (63, 64, 65):
            settings = config({'max_call_bytes': 64, 'max_buffer_bytes': 10000, 'max_calls': 10}, restore=True)
            settings['replace_roles'] = [{'regex': 'O+', 'type': 'replace', 'value': 'MASK', 'restore': True}]
            request = request_for('normal', True)
            request['messages'][0]['content'] = 'O' * (size - 12)
            changed = '{"email":"' + 'O' * (size - 12) + '"}'
            group(f'output-{size}', settings, [{'request': request, 'spec': spec_for('{"email":"MASK"}', content='public status'),
                'expected': 'pass' if size <= 64 else 'response-denied', 'expected_arguments': {('0', 'modern', '0'): changed}, 'expected_text': 'public status'}])
        # Peak logical retained bytes: input16 + name/id12 + rewritten
        # literal52 + complete rewritten JSON64 = 144.
        for buffer in (143, 144, 145):
            settings = config({'max_call_bytes': 1024, 'max_buffer_bytes': buffer, 'max_calls': 10}, restore=True)
            settings['replace_roles'] = [{'regex': 'O+', 'type': 'replace', 'value': 'MASK', 'restore': True}]
            request = request_for('normal', True)
            request['messages'][0]['content'] = 'O' * 52
            group(f'rewrite-total-{buffer}', settings, [{'request': request, 'spec': spec_for('{"email":"MASK"}', content='public status'), 'expected': 'pass' if buffer >= 144 else 'response-denied',
                'expected_arguments': {('0', 'modern', '0'): '{"email":"' + 'O' * 52 + '"}'}, 'expected_text': 'public status'}])
        # Needed name/id metadata consumes the response budget before arguments.
        for total in (63, 64, 65):
            calls = [{'name': 'lookup', 'index': 0, 'id': 'i' * (total - 6)}]
            group(f'metadata-{total}', config({'max_call_bytes': 1024, 'max_buffer_bytes': 64, 'max_calls': 10}),
                [{'spec': {'calls': calls}, 'expected': 'pass' if total <= 64 else 'response-denied', 'streams': (True,)}])
        # Non-allowlisted restore-only calls do not acquire new byte/count quotas.
        calls = [{'name': 'external', 'index': index, 'id': 'outside-' + str(index), 'arguments': 'not JSON MASKED_TOKEN'} for index in range(130)]
        group('outside-quotas', config({'max_call_bytes': 1, 'max_buffer_bytes': 1, 'max_calls': 1}, restore=True), [{'spec': {'calls': calls}, 'streams': (True,)}])
        if not args.smoke:
            history_argument = '{"email":"DEMO_SECRET","padding":"' + 'x' * 35000 + '"}'
            request = request_for('normal', True)
            request['messages'].append({'role': 'assistant', 'content': None, 'tool_calls': [{'id': f'history-{index}', 'type': 'function', 'function': {'name': 'lookup', 'arguments': history_argument}} for index in range(129)]})
            group('request-long-history', config({'max_call_bytes': 100000, 'max_buffer_bytes': 1, 'max_calls': 1}),
                [{'request': request, 'spec': {'calls': []}, 'streams': (True,), 'history': True, 'expected_text': 'checking MASKED_TOKEN'}])
        lab.save('summary.json', {'passed': len(records), 'failed': 0, 'full_t5_complete': False, 'models': 0})


if __name__ == '__main__':
    main()
